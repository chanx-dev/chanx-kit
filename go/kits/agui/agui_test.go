package agui

import (
	"context"
	"fmt"
	"iter"
	"testing"
	"time"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"
)

func reply(context.Context, RunAgentInput) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		_ = yield(TextMessageContent("m", "hi"), nil) && yield(TextMessageContent("m", " there"), nil)
	}
}

func forever(ctx context.Context, _ RunAgentInput) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		if yield(TextMessageContent("m", "working"), nil) {
			<-ctx.Done()
		}
	}
}

type agentConsumer struct{ topic AgUiTopic }

func (c agentConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{c.topic, AgUiRunTopic{}}
}

func setup(t *testing.T, topic AgUiTopic) *chanx.Engine {
	t.Helper()
	topic.RunEventStore, topic.ActiveRunStore = NewInMemoryRunEventStore(), NewInMemoryActiveRunStore()
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", agentConsumer{topic})
	return engine
}

func join(t *testing.T, engine *chanx.Engine) *chanxtest.Client {
	t.Helper()
	c := chanxtest.Connect(t, engine, "/ws")
	c.Subscribe("agui:thread:t1")
	return c
}

func start(c *chanxtest.Client, runID string) {
	c.Send(AgUiRunMessage{Payload: RunAgentInput{ThreadID: "t1", RunID: runID, Messages: []map[string]any{}, Tools: []map[string]any{}, Context: []map[string]any{}}}, chanxtest.Topic("agui:thread:t1"))
}

func next(t *testing.T, c *chanxtest.Client) Event {
	t.Helper()
	m, _ := chanxtest.ReceiveUntilMessage[AgUiEventMessage](c)
	return m.Payload
}

func TestARunStreamsInsideItsLifecycle(t *testing.T) {
	c := join(t, setup(t, AgUiTopic{RunAgent: reply}))

	start(c, "r1")

	for _, want := range []string{"RUN_STARTED", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_CONTENT", "RUN_FINISHED"} {
		if got := next(t, c).Type(); got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	}
}

func TestASecondRunIsRefusedAndCancelEndsTheFirst(t *testing.T) {
	c := join(t, setup(t, AgUiTopic{RunAgent: forever}))
	start(c, "r1")
	next(t, c)
	next(t, c)

	start(c, "r2")
	if e := next(t, c); e["message"] != "Thread is already running r1." {
		t.Fatalf("refusal %v", e)
	}

	c.Send(AgUiCancelMessage{Payload: AgUiCancel{RunID: "r1"}}, chanxtest.Topic("agui:thread:t1"))
	if e := next(t, c); e.Type() != "RUN_ERROR" || e["message"] != "Run cancelled." {
		t.Fatalf("cancel %v", e)
	}
}

func TestLeavingStopsTheSocketsRun(t *testing.T) {
	topic := AgUiTopic{RunAgent: forever}
	c := join(t, setup(t, topic))
	start(c, "r1")
	next(t, c)
	next(t, c)

	c.Unsubscribe("agui:thread:t1")

	deadline := time.Now().Add(2 * time.Second)
	for topic.Running("t1") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if topic.Running("t1") {
		t.Fatal("the run outlived its only watcher")
	}
}

func TestABroadcastRunIsReplayedWithSeq(t *testing.T) {
	engine := setup(t, AgUiTopic{RunAgent: forever, Broadcast: true})
	first := join(t, engine)
	start(first, "r1")
	next(t, first)
	next(t, first)

	late := chanxtest.Connect(t, engine, "/ws")
	late.Subscribe("agui:thread:t1")

	for i, want := range []string{"RUN_STARTED", "TEXT_MESSAGE_CONTENT"} {
		f := late.Receive()
		var m AgUiEventMessage
		if err := f.Decode(&m); err != nil || m.Payload.Type() != want || fmt.Sprint(f["seq"]) != fmt.Sprint(i+1) {
			t.Fatalf("replayed %s", f)
		}
	}
}

func TestAServerRunAndEmitToRun(t *testing.T) {
	topic := AgUiTopic{RunAgent: reply, Broadcast: true}
	engine := setup(t, topic)
	c := join(t, engine)
	c.Subscribe("agui:run:r9")

	if _, err := topic.StartRun(context.Background(), engine.Publisher(), "t1", RunAgentInput{RunID: "r9"}); err != nil {
		t.Fatal(err)
	}
	for next(t, c).Type() != "RUN_FINISHED" {
	}
	if err := EmitToRun(context.Background(), engine.Publisher(), "r9", TextMessageContent("m", "from a worker")); err != nil {
		t.Fatal(err)
	}
	if f := c.ReceiveTopic("agui:run:r9"); f.Action() != "ag_ui_event" {
		t.Fatalf("got %s", f)
	}
}
