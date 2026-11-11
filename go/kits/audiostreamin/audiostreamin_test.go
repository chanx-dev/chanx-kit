package audiostreamin

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"
)

// fakeStream records what it is sent and, on Finish, says one final per chunk received.
type fakeStream struct {
	mu         sync.Mutex
	sent       [][]byte
	keepalives int
	closed     bool
	frames     chan Frame
	once       sync.Once
}

func newFakeStream() *fakeStream { return &fakeStream{frames: make(chan Frame, 64)} }

func (f *fakeStream) Send(_ context.Context, audio []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, audio)
	return nil
}

func (f *fakeStream) Keepalive(context.Context) error {
	f.mu.Lock()
	f.keepalives++
	f.mu.Unlock()
	return nil
}

func (f *fakeStream) Finish(context.Context) error {
	f.mu.Lock()
	var text []string
	for _, chunk := range f.sent {
		text = append(text, string(chunk))
	}
	f.mu.Unlock()
	f.frames <- SpeechStartedFrame{AtMs: 0}
	f.frames <- FinalFrame{Text: strings.Join(text, " "), StartMs: 0, EndMs: 100}
	f.frames <- UtteranceEndFrame{}
	f.stop()
	return nil
}

func (f *fakeStream) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	f.stop()
	return nil
}

func (f *fakeStream) stop() { f.once.Do(func() { close(f.frames) }) }

func (f *fakeStream) Frames(ctx context.Context) iter.Seq2[Frame, error] {
	return func(yield func(Frame, error) bool) {
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-f.frames:
				if !ok || !yield(frame, nil) {
					return
				}
			}
		}
	}
}

func (f *fakeStream) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

type opened struct {
	mu      sync.Mutex
	streams []*fakeStream
}

func (o *opened) open(context.Context, AudioStart) (TranscriberStream, error) {
	s := newFakeStream()
	o.mu.Lock()
	o.streams = append(o.streams, s)
	o.mu.Unlock()
	return s, nil
}

func (o *opened) last() *fakeStream {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.streams[len(o.streams)-1]
}

type voiceConsumer struct{ topic TranscriberTopic }

func (c voiceConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{c.topic, TranscriptTopic{}}
}

func (voiceConsumer) Authenticate(_ *chanx.Context, r *chanx.Request) (any, error) {
	return r.Query.Get("as"), nil
}

func connect(t *testing.T, topic TranscriberTopic, user string) *chanxtest.Client {
	t.Helper()
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", voiceConsumer{topic})
	return chanxtest.Connect(t, engine, "/ws", chanxtest.Query("as", user))
}

const topic = "transcribe:s1"

func join(t *testing.T, topic TranscriberTopic, user string) *chanxtest.Client {
	t.Helper()
	c := connect(t, topic, user)
	c.Subscribe("transcribe:s1")
	chanxtest.ReceiveUntilMessage[AudioConfigMessage](c)
	return c
}

func start(c *chanxtest.Client) {
	c.Send(AudioStartMessage{Payload: AudioStart{Encoding: PCM16, SampleRate: 24000, Channels: 1}}, chanxtest.Topic(topic))
}

func chunk(c *chanxtest.Client, index int, data string) {
	c.Send(AudioChunkMessage{Payload: AudioChunk{Index: index, Data: []byte(data)}}, chanxtest.Topic(topic))
}

func errorOf(t *testing.T, c *chanxtest.Client) TranscriberError {
	t.Helper()
	m, _ := chanxtest.ReceiveUntilMessage[TranscriberErrorMessage](c)
	return m.Payload
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition never held")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTheFormatIsSentOnSubscribe(t *testing.T) {
	c := connect(t, TranscriberTopic{OpenStream: (&opened{}).open}, "ann")
	c.Subscribe(topic)
	m, _ := chanxtest.ReceiveUntilMessage[AudioConfigMessage](c)
	if m.Payload != DefaultAudioConfig {
		t.Fatalf("config %+v", m.Payload)
	}
}

func TestSpeechBecomesAnUtterance(t *testing.T) {
	var o opened
	c := join(t, TranscriberTopic{OpenStream: o.open}, "ann")
	start(c)
	chunk(c, 0, "hello")
	chunk(c, 1, "world")
	c.Send(AudioEndMessage{}, chanxtest.Topic(topic))

	chanxtest.ReceiveUntilMessage[SpeechStartedMessage](c)
	final, _ := chanxtest.ReceiveUntilMessage[TranscriptFinalMessage](c)
	end, _ := chanxtest.ReceiveUntilMessage[UtteranceEndMessage](c)
	if final.Payload.Text != "hello world" || end.Payload.UtteranceID != final.Payload.UtteranceID {
		t.Fatalf("final %+v, end %+v", final.Payload, end.Payload)
	}
}

func TestWatchersReceiveTheTranscript(t *testing.T) {
	var o opened
	topicDef := TranscriberTopic{OpenStream: o.open}
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", voiceConsumer{topicDef})
	speaker := chanxtest.Connect(t, engine, "/ws", chanxtest.Query("as", "ann"))
	watcher := chanxtest.Connect(t, engine, "/ws", chanxtest.Query("as", "bob"))
	watcher.Subscribe("transcript:s1")
	speaker.Subscribe(topic)
	chanxtest.ReceiveUntilMessage[AudioConfigMessage](speaker)

	start(speaker)
	chunk(speaker, 0, "hi")
	speaker.Send(AudioEndMessage{}, chanxtest.Topic(topic))
	final, _ := chanxtest.ReceiveUntilMessage[TranscriptFinalMessage](watcher)
	if final.Payload.Text != "hi" {
		t.Fatalf("watcher heard %+v", final.Payload)
	}
}

func TestAFormatOtherThanTheConfiguredOneIsRefused(t *testing.T) {
	c := join(t, TranscriberTopic{OpenStream: (&opened{}).open}, "ann")
	c.Send(AudioStartMessage{Payload: AudioStart{Encoding: PCM16, SampleRate: 16000, Channels: 1}}, chanxtest.Topic(topic))
	if e := errorOf(t, c); e.Code != BadFormat {
		t.Fatalf("error %+v", e)
	}
}

func TestAudioBeforeAudioStartIsRefused(t *testing.T) {
	c := join(t, TranscriberTopic{OpenStream: (&opened{}).open}, "ann")
	chunk(c, 0, "early")
	if e := errorOf(t, c); e.Code != NotStarted {
		t.Fatalf("error %+v", e)
	}
}

func TestLostChunksAreReported(t *testing.T) {
	c := join(t, TranscriberTopic{OpenStream: (&opened{}).open, ReorderWindow: 1}, "ann")
	start(c)
	chunk(c, 0, "a")
	chunk(c, 2, "c")
	chunk(c, 3, "d")
	if e := errorOf(t, c); e.Code != AudioGap || !strings.Contains(e.Message, "1 to 1") {
		t.Fatalf("error %+v", e)
	}
}

func TestStreamsPerUserAreLimited(t *testing.T) {
	var o opened
	topicDef := TranscriberTopic{OpenStream: o.open, MaxSessionsPerUser: 1}
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", voiceConsumer{topicDef})
	first := chanxtest.Connect(t, engine, "/ws", chanxtest.Query("as", "ann"))
	second := chanxtest.Connect(t, engine, "/ws", chanxtest.Query("as", "ann"))
	for _, c := range []*chanxtest.Client{first, second} {
		c.Subscribe(topic)
		chanxtest.ReceiveUntilMessage[AudioConfigMessage](c)
	}
	start(first)
	eventually(t, func() bool { o.mu.Lock(); defer o.mu.Unlock(); return len(o.streams) == 1 })
	start(second)
	if e := errorOf(t, second); e.Code != LimitReached {
		t.Fatalf("error %+v", e)
	}
}

func TestASilentStreamIsClosed(t *testing.T) {
	var o opened
	c := join(t, TranscriberTopic{OpenStream: o.open, IdleTimeout: 50 * time.Millisecond, KeepaliveEvery: Off}, "ann")
	start(c)
	if e := errorOf(t, c); e.Code != LimitReached || !strings.Contains(e.Message, "No audio") {
		t.Fatalf("error %+v", e)
	}
	eventually(t, func() bool { return o.last().isClosed() })
}

func TestQuietTimeKeepsTheProviderAlive(t *testing.T) {
	var o opened
	c := join(t, TranscriberTopic{OpenStream: o.open, KeepaliveEvery: 20 * time.Millisecond, IdleTimeout: Off}, "ann")
	start(c)
	eventually(t, func() bool {
		o.mu.Lock()
		defer o.mu.Unlock()
		if len(o.streams) == 0 {
			return false
		}
		s := o.streams[0]
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.keepalives >= 2
	})
}

func TestAStreamIsBoundedInTime(t *testing.T) {
	var o opened
	c := join(t, TranscriberTopic{OpenStream: o.open, MaxSession: 50 * time.Millisecond, IdleTimeout: Off}, "ann")
	start(c)
	if e := errorOf(t, c); e.Code != LimitReached || !strings.Contains(e.Message, "limited to") {
		t.Fatalf("error %+v", e)
	}
	eventually(t, func() bool { return o.last().isClosed() })
}

func TestANewAudioStartReplacesTheStream(t *testing.T) {
	var o opened
	c := join(t, TranscriberTopic{OpenStream: o.open}, "ann")
	start(c)
	eventually(t, func() bool { o.mu.Lock(); defer o.mu.Unlock(); return len(o.streams) == 1 })
	first := o.last()
	start(c)
	eventually(t, func() bool { o.mu.Lock(); defer o.mu.Unlock(); return len(o.streams) == 2 })
	eventually(t, first.isClosed)
}

func TestLeavingClosesTheProviderStream(t *testing.T) {
	var o opened
	c := join(t, TranscriberTopic{OpenStream: o.open}, "ann")
	start(c)
	eventually(t, func() bool { o.mu.Lock(); defer o.mu.Unlock(); return len(o.streams) == 1 })
	c.Unsubscribe(topic)
	eventually(t, o.last().isClosed)
}

func TestAProviderThatRefusesIsReported(t *testing.T) {
	refuse := func(context.Context, AudioStart) (TranscriberStream, error) {
		return nil, errors.Join(ErrProviderUnavailable, errors.New("no key"))
	}
	c := join(t, TranscriberTopic{OpenStream: refuse}, "ann")
	start(c)
	if e := errorOf(t, c); e.Code != ProviderUnavailable {
		t.Fatalf("error %+v", e)
	}
	// The refused stream did not count against the user's limit.
	start(c)
	if e := errorOf(t, c); e.Code != ProviderUnavailable {
		t.Fatalf("second error %+v", e)
	}
}

func TestAnUnconfiguredProviderSaysSoOnSubscribe(t *testing.T) {
	c := connect(t, TranscriberTopic{OpenStream: (&opened{}).open, ProviderAvailable: func() bool { return false }}, "ann")
	c.Subscribe(topic)
	if e := errorOf(t, c); e.Code != ProviderUnavailable {
		t.Fatalf("error %+v", e)
	}
}

func TestFinalHooksSeeTheText(t *testing.T) {
	var o opened
	heard := make(chan string, 1)
	c := join(t, TranscriberTopic{
		OpenStream: o.open,
		OnFinal: func(_ context.Context, s Scope, f TranscriptFinal) error {
			heard <- s.Session + ":" + f.Text
			return nil
		},
	}, "ann")
	start(c)
	chunk(c, 0, "hey")
	c.Send(AudioEndMessage{}, chanxtest.Topic(topic))
	select {
	case got := <-heard:
		if got != "s1:hey" {
			t.Fatalf("hook got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hook never ran")
	}
}
