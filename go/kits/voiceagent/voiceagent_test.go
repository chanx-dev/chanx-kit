package voiceagent

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"sync/atomic"
	"testing"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"

	"go.chanx.dev/kit/kits/agui"
	"go.chanx.dev/kit/kits/audiostreamout"
	"go.chanx.dev/kit/kits/mediastreamout"
)

func TestSentencesAreReleasedAsTheyComplete(t *testing.T) {
	b := SentenceBuffer{MinChars: 1}
	if got := b.Feed("Hello there. How are"); !reflect.DeepEqual(got, []string{"Hello there."}) {
		t.Fatalf("got %v", got)
	}
	if got := b.Feed(" you? Fine"); !reflect.DeepEqual(got, []string{"How are you?"}) {
		t.Fatalf("got %v", got)
	}
	if b.Flush() != "Fine" {
		t.Fatal("flush lost the rest")
	}
}

func TestTinySentencesWaitForTheNext(t *testing.T) {
	b := SentenceBuffer{MinChars: 12}
	if got := b.Feed("Hi. I am well. "); !reflect.DeepEqual(got, []string{"Hi. I am well."}) {
		t.Fatalf("got %v", got)
	}
}

func TestLineBreaksEndSentences(t *testing.T) {
	b := SentenceBuffer{MinChars: 1}
	if got := b.Feed("first line\nsecond"); !reflect.DeepEqual(got, []string{"first line"}) {
		t.Fatalf("got %v", got)
	}
}

// echo replies to the last message, a word at a time.
func echo(_ context.Context, in agui.RunAgentInput) iter.Seq2[agui.Event, error] {
	return func(yield func(agui.Event, error) bool) {
		last, _ := in.Messages[len(in.Messages)-1]["content"].(string)
		reply := fmt.Sprintf("You said %s. I heard %d messages.", last, len(in.Messages))
		_ = yield(agui.TextMessageStart("m"), nil) &&
			yield(agui.TextMessageContent("m", reply), nil) &&
			yield(agui.TextMessageEnd("m"), nil)
	}
}

// hold streams a reply, then waits to be cancelled.
func hold(ctx context.Context, _ agui.RunAgentInput) iter.Seq2[agui.Event, error] {
	return func(yield func(agui.Event, error) bool) {
		if yield(agui.TextMessageStart("m"), nil) && yield(agui.TextMessageContent("m", "A long reply that keeps going. "), nil) {
			<-ctx.Done()
		}
	}
}

// pcm says each text as 250 ms of silence.
func pcm(context.Context, string, string, audiostreamout.SpeechFormat) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) { yield(make([]byte, 12000), nil) }
}

type agentConsumer struct{ topics []chanx.TopicHandler }

func (c agentConsumer) Topics() []chanx.TopicHandler { return c.topics }

var counter atomic.Int64

func setup(t *testing.T, run agui.Stream) (*Agent, *chanxtest.Client, string) {
	t.Helper()
	session := fmt.Sprintf("v%d", counter.Add(1))
	engine := chanx.New(chanx.Config{})
	agent := &Agent{
		Agent: agui.AgUiTopic{RunAgent: run, RunEventStore: agui.NewInMemoryRunEventStore(), ActiveRunStore: agui.NewInMemoryActiveRunStore()},
		Synthesizer: audiostreamout.SynthesizerTopic{
			Synthesize: pcm, ReplayStore: mediastreamout.NewInMemoryReplayStore(), PlaybackStore: audiostreamout.NewInMemoryPlaybackStore(),
		},
	}
	engine.Handle("/ws", agentConsumer{[]chanx.TopicHandler{agent.AgUiTopic(), agent.Synthesizer}})
	agent.Publisher = engine.Publisher()
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe("agui:thread:" + session)
	c.Subscribe("speak:" + session)
	return agent, c, session
}

func until(t *testing.T, c *chanxtest.Client, eventType string) agui.Event {
	t.Helper()
	for {
		m, _ := chanxtest.ReceiveUntilMessage[agui.AgUiEventMessage](c)
		if m.Payload.Type() == eventType {
			return m.Payload
		}
	}
}

func TestWhatTheUserSaysIsAnsweredOutLoud(t *testing.T) {
	agent, c, session := setup(t, echo)
	if _, err := agent.Respond(context.Background(), session, "hello"); err != nil {
		t.Fatal(err)
	}
	start, _ := chanxtest.ReceiveUntilMessage[audiostreamout.SpeechAudioStartMessage](c)
	if start.Payload.Text != "You said hello." {
		t.Fatalf("spoke %q", start.Payload.Text)
	}
}

func TestTheConversationIsRememberedBetweenTurns(t *testing.T) {
	agent, c, session := setup(t, echo)
	ctx := context.Background()
	_, _ = agent.Respond(ctx, session, "one")
	until(t, c, "RUN_FINISHED")
	agent.Synthesizer.WaitUntilSpoken(session)
	_, _ = agent.Respond(ctx, session, "two")
	until(t, c, "RUN_STARTED")
	content := until(t, c, "TEXT_MESSAGE_CONTENT")
	if content["delta"] != "You said two. I heard 3 messages." {
		t.Fatalf("second reply %v", content["delta"])
	}
}

func TestAProviderKeepingItsOwnHistoryGetsTheNewTurnOnly(t *testing.T) {
	agent, c, session := setup(t, echo)
	agent.OwnHistory = true
	ctx := context.Background()
	_, _ = agent.Respond(ctx, session, "one")
	until(t, c, "RUN_FINISHED")
	_, _ = agent.Respond(ctx, session, "two")
	until(t, c, "RUN_STARTED")
	if content := until(t, c, "TEXT_MESSAGE_CONTENT"); content["delta"] != "You said two. I heard 1 messages." {
		t.Fatalf("reply %v", content["delta"])
	}
}

func TestSpeakingOverTheAgentInterruptsIt(t *testing.T) {
	agent, c, session := setup(t, hold)
	ctx := context.Background()
	_, _ = agent.Respond(ctx, session, "talk")
	start, _ := chanxtest.ReceiveUntilMessage[audiostreamout.SpeechAudioStartMessage](c)
	_ = agent.Synthesizer.Playback().Mark(ctx, session, start.Payload.UtteranceID, 5000)

	interrupted, err := agent.Interrupt(ctx, session)
	if err != nil || !interrupted {
		t.Fatalf("interrupted %v, %v", interrupted, err)
	}
	custom := until(t, c, "CUSTOM")
	if heard := custom["value"].(map[string]any)["heard"]; heard != "A long reply that keeps going." {
		t.Fatalf("heard %v", heard)
	}
	turns, _ := agent.Memory.Load(ctx, session)
	if len(turns) != 2 || turns[1]["content"] != "A long reply that keeps going." {
		t.Fatalf("remembered %v", turns)
	}
}

func TestInterruptingAQuietAgentDoesNothing(t *testing.T) {
	agent, _, session := setup(t, echo)
	interrupted, err := agent.Interrupt(context.Background(), session)
	if err != nil || interrupted {
		t.Fatalf("interrupted %v, %v", interrupted, err)
	}
}

func TestInterruptingANewRunKeepsThePreviousReply(t *testing.T) {
	agent, c, session := setup(t, echo)
	ctx := context.Background()
	_, _ = agent.Respond(ctx, session, "one")
	until(t, c, "RUN_FINISHED")
	agent.Synthesizer.WaitUntilSpoken(session)

	agent.Agent.RunAgent = hold
	_, _ = agent.Respond(ctx, session, "two")
	until(t, c, "TEXT_MESSAGE_CONTENT")
	if interrupted, err := agent.Interrupt(ctx, session); err != nil || !interrupted {
		t.Fatalf("interrupted %v, %v", interrupted, err)
	}
	turns, _ := agent.Memory.Load(ctx, session)
	if len(turns) != 3 || turns[1]["content"] != "You said one. I heard 1 messages." || turns[2]["content"] != "two" {
		t.Fatalf("remembered %v", turns)
	}
}
