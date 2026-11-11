package audiostreamout

import (
	"context"
	"fmt"
	"iter"
	"testing"
	"time"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"

	"go.chanx.dev/kit/kits/mediastreamout"
)

// tone yields n bytes per word, in pieces that split samples.
func tone(n int) Synthesize {
	return func(_ context.Context, text string, _ string, _ SpeechFormat) iter.Seq2[[]byte, error] {
		return func(yield func([]byte, error) bool) {
			audio := make([]byte, n)
			if !yield(audio[:n/2+1], nil) {
				return
			}
			yield(audio[n/2+1:], nil)
		}
	}
}

// slow yields one piece, then waits for release or cancel.
func slow(release chan struct{}) Synthesize {
	return func(ctx context.Context, _ string, _ string, _ SpeechFormat) iter.Seq2[[]byte, error] {
		return func(yield func([]byte, error) bool) {
			if !yield(make([]byte, 4800), nil) {
				return
			}
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}
}

type speakConsumer struct{ topic SynthesizerTopic }

func (c speakConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

var sessions int

func setup(t *testing.T, topic SynthesizerTopic) (*chanx.Engine, SynthesizerTopic, string) {
	t.Helper()
	sessions++
	topic.ReplayStore, topic.PlaybackStore = mediastreamout.NewInMemoryReplayStore(), NewInMemoryPlaybackStore()
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", speakConsumer{topic})
	return engine, topic, fmt.Sprintf("speak:s%d", sessions)
}

func listen(t *testing.T, engine *chanx.Engine, name string) *chanxtest.Client {
	t.Helper()
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe(name)
	return c
}

func session(name string) string { return name[len("speak:"):] }

func receive[T chanx.Message](c *chanxtest.Client) T {
	m, _ := chanxtest.ReceiveUntilMessage[T](c)
	return m
}

func TestTextBecomesOneUtteranceOfPCMAudio(t *testing.T) {
	engine, topic, name := setup(t, SynthesizerTopic{Synthesize: tone(9600)})
	c := listen(t, engine, name)
	c.Send(SpeakMessage{Payload: Speak{Text: "hello"}}, chanxtest.Topic(name))

	start := receive[SpeechAudioStartMessage](c)
	end := receive[SpeechAudioEndMessage](c)
	if start.Payload.Text != "hello" || start.Payload.SampleRate != 24000 || end.Payload.DurationMs != 200 {
		t.Fatalf("start %+v, end %+v", start.Payload, end.Payload)
	}
	topic.WaitUntilSpoken(session(name))
}

func TestAudioIsCutIntoBoundedWholeFrames(t *testing.T) {
	engine, _, name := setup(t, SynthesizerTopic{Synthesize: tone(24001), MaxChunkMs: 100})
	c := listen(t, engine, name)
	c.Send(SpeakMessage{Payload: Speak{Text: "hello"}}, chanxtest.Topic(name))
	_, frames := chanxtest.ReceiveUntilMessage[SpeechAudioEndMessage](c)
	total := 0
	for _, f := range frames {
		if f.Action() != "audio_chunk" {
			continue
		}
		var m SpeechAudioChunkMessage
		_ = f.Decode(&m)
		if n := len(m.Payload.Data); n%2 != 0 || n > 4800 {
			t.Fatalf("chunk of %d bytes", n)
		}
		total += len(m.Payload.Data)
	}
	if total != 24000 {
		t.Fatalf("sent %d bytes, want whole samples only", total)
	}
}

func TestUtterancesPlayInTheOrderTheyWereQueued(t *testing.T) {
	engine, topic, name := setup(t, SynthesizerTopic{Synthesize: tone(480)})
	c := listen(t, engine, name)
	ctx := context.Background()
	for _, text := range []string{"one", "two", "three"} {
		if _, err := topic.Speak(ctx, engine.Publisher(), session(name), text, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"one", "two", "three"} {
		if got := receive[SpeechAudioStartMessage](c).Payload.Text; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

func TestEveryListenerHearsIt(t *testing.T) {
	engine, _, name := setup(t, SynthesizerTopic{Synthesize: tone(480)})
	asker, other := listen(t, engine, name), listen(t, engine, name)
	asker.Send(SpeakMessage{Payload: Speak{Text: "hi all"}}, chanxtest.Topic(name))
	if receive[SpeechAudioStartMessage](other).Payload.Text != "hi all" {
		t.Fatal("the other listener missed it")
	}
}

func TestAListenerJoiningMidUtteranceIsReplayedFromItsStart(t *testing.T) {
	release := make(chan struct{})
	engine, topic, name := setup(t, SynthesizerTopic{Synthesize: slow(release)})
	first := listen(t, engine, name)
	first.Send(SpeakMessage{Payload: Speak{Text: "long one"}}, chanxtest.Topic(name))
	receive[SpeechAudioChunkMessage](first)

	late := listen(t, engine, name)
	if receive[SpeechAudioStartMessage](late).Payload.Text != "long one" {
		t.Fatal("no replayed start")
	}
	if receive[SpeechAudioChunkMessage](late).Payload.Index != 0 {
		t.Fatal("replay did not begin at the first chunk")
	}
	close(release)
	topic.WaitUntilSpoken(session(name))
}

func TestClearStopsTheUtteranceAndDropsTheQueue(t *testing.T) {
	release := make(chan struct{})
	engine, topic, name := setup(t, SynthesizerTopic{Synthesize: slow(release)})
	c := listen(t, engine, name)
	ctx := context.Background()
	id, _ := topic.Speak(ctx, engine.Publisher(), session(name), "first", "", "")
	_, _ = topic.Speak(ctx, engine.Publisher(), session(name), "second", "", "")
	receive[SpeechAudioChunkMessage](c)

	c.Send(SpeakClearMessage{}, chanxtest.Topic(name))
	clear := receive[SpeechClearMessage](c)
	if clear.Payload.UtteranceID == nil || *clear.Payload.UtteranceID != id || topic.IsSpeaking(session(name)) {
		t.Fatalf("clear %+v, speaking %v", clear.Payload, topic.IsSpeaking(session(name)))
	}
	c.ExpectNothing(50 * time.Millisecond)
}

func TestEmptyTextIsRefused(t *testing.T) {
	engine, _, name := setup(t, SynthesizerTopic{Synthesize: tone(480)})
	c := listen(t, engine, name)
	c.Send(SpeakMessage{Payload: Speak{Text: "  "}}, chanxtest.Topic(name))
	if e := receive[SynthesizerErrorMessage](c).Payload; e.Code != BadRequest {
		t.Fatalf("error %+v", e)
	}
}

func TestTextOverTheLimitIsRefused(t *testing.T) {
	engine, _, name := setup(t, SynthesizerTopic{Synthesize: tone(480), MaxTextChars: 3})
	c := listen(t, engine, name)
	c.Send(SpeakMessage{Payload: Speak{Text: "four"}}, chanxtest.Topic(name))
	if e := receive[SynthesizerErrorMessage](c).Payload; e.Code != BadRequest {
		t.Fatalf("error %+v", e)
	}
}

func TestARefusingProviderIsReportedAndTheUtteranceStillEnds(t *testing.T) {
	refuse := func(context.Context, string, string, SpeechFormat) iter.Seq2[[]byte, error] {
		return func(yield func([]byte, error) bool) { yield(nil, fmt.Errorf("%w: no key", ErrProviderUnavailable)) }
	}
	engine, _, name := setup(t, SynthesizerTopic{Synthesize: refuse})
	c := listen(t, engine, name)
	c.Send(SpeakMessage{Payload: Speak{Text: "hello"}}, chanxtest.Topic(name))
	if e := receive[SynthesizerErrorMessage](c).Payload; e.Code != ProviderUnavailable {
		t.Fatalf("error %+v", e)
	}
	receive[SpeechAudioEndMessage](c)
}

func TestAnUnconfiguredProviderAnswersTheAsker(t *testing.T) {
	engine, _, name := setup(t, SynthesizerTopic{Synthesize: tone(480), ProviderAvailable: func() bool { return false }})
	c := listen(t, engine, name)
	c.Send(SpeakMessage{Payload: Speak{Text: "hello"}}, chanxtest.Topic(name))
	if e := receive[SynthesizerErrorMessage](c).Payload; e.Code != ProviderUnavailable {
		t.Fatalf("error %+v", e)
	}
}

func TestPlaybackMarksAreRecorded(t *testing.T) {
	engine, topic, name := setup(t, SynthesizerTopic{Synthesize: tone(480)})
	c := listen(t, engine, name)
	c.Send(SpeakMessage{Payload: Speak{Text: "hello"}}, chanxtest.Topic(name))
	id := receive[SpeechAudioStartMessage](c).Payload.UtteranceID
	receive[SpeechAudioEndMessage](c)
	c.Send(PlaybackMarkMessage{Payload: PlaybackMark{UtteranceID: id, PlayedMs: 7}}, chanxtest.Topic(name))
	deadline := time.Now().Add(time.Second)
	for {
		u, _ := topic.Playback().Get(context.Background(), session(name), id)
		if u != nil && u.PlayedMs == 7 && u.Finished {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("utterance %+v", u)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestHeardTextCutsAtAWord(t *testing.T) {
	for played, want := range map[int]string{
		0: "", 500: "The quick brown", 1000: "The quick brown fox jumps over", 5000: "The quick brown fox jumps over",
	} {
		if got := HeardText("The quick brown fox jumps over", 1000, played); got != want {
			t.Errorf("played %d: %q, want %q", played, got, want)
		}
	}
	if HeardText("anything", 0, 100) != "" {
		t.Error("an unmeasured utterance was heard")
	}
}
