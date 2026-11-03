package notification

import (
	"context"
	"testing"
	"time"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"
)

type appConsumer struct{ acked *[]string }

func (appConsumer) Authenticate(_ *chanx.Context, r *chanx.Request) (any, error) {
	return r.Query.Get("as"), nil
}

func (c appConsumer) Topics() []chanx.TopicHandler {
	acks := Acks{OnAcked: func(_ *chanx.Context, ids []string) error {
		*c.acked = append(*c.acked, ids...)
		return nil
	}}
	return []chanx.TopicHandler{
		UserNotificationTopic{Acks: acks},
		SubjectNotificationTopic{Acks: acks},
		BroadcastNotificationTopic{Acks: acks},
	}
}

func setup(t *testing.T) (*chanx.Engine, *[]string) {
	t.Helper()
	acked := &[]string{}
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", appConsumer{acked: acked})
	return engine, acked
}

func TestNotifyUserReachesOnlyThatUser(t *testing.T) {
	engine, _ := setup(t)
	ann := chanxtest.Connect(t, engine, "/ws?as=ann")
	bob := chanxtest.Connect(t, engine, "/ws?as=bob")
	ann.Subscribe("notification:user:ann")
	bob.Subscribe("notification:user:bob")

	err := NotifyUser(context.Background(), engine.Publisher(), "ann", NotificationPayload{Title: "Build finished"})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := chanxtest.ReceiveUntilMessage[NotificationMessage](ann)
	if got.Payload.Title != "Build finished" || got.Payload.Level != LevelInfo || got.Payload.ID == "" || got.Payload.CreatedAt.IsZero() {
		t.Fatalf("defaults not filled: %+v", got.Payload)
	}
	bob.ExpectNothing(100 * time.Millisecond)
}

func TestAnotherUsersTopicIsRefused(t *testing.T) {
	engine, _ := setup(t)
	ann := chanxtest.Connect(t, engine, "/ws?as=ann")

	ann.SendJSON(map[string]any{"action": "subscribe", "topic": "notification:user:bob"})

	f := ann.Receive()
	if f.Action() != "error" || f.Payload().(map[string]any)["type"] != "https://chanx.dev/errors/unauthorized" {
		t.Fatalf("got %s", f)
	}
}

func TestAnonymousCannotSubscribeToAUserTopic(t *testing.T) {
	engine, _ := setup(t)
	anon := chanxtest.Connect(t, engine, "/ws")

	anon.SendJSON(map[string]any{"action": "subscribe", "topic": "notification:user:ann"})

	if f := anon.Receive(); f.Action() != "error" {
		t.Fatalf("got %s", f)
	}
}

func TestBroadcastAndSubjectReachSubscribers(t *testing.T) {
	engine, _ := setup(t)
	ann := chanxtest.Connect(t, engine, "/ws?as=ann")
	ann.Subscribe("notification:all")
	ann.Subscribe("notification:subject:deploys")
	ctx, pub := context.Background(), engine.Publisher()

	if err := NotifyAll(ctx, pub, NotificationPayload{Title: "Maintenance"}); err != nil {
		t.Fatal(err)
	}
	if f := ann.ReceiveTopic("notification:all"); f.Action() != "notification" {
		t.Fatalf("got %s", f)
	}
	if err := NotifySubject(ctx, pub, "deploys", NotificationPayload{Title: "Shipped", Level: LevelSuccess}); err != nil {
		t.Fatal(err)
	}
	if f := ann.ReceiveTopic("notification:subject:deploys"); f.Payload().(map[string]any)["level"] != "success" {
		t.Fatalf("got %s", f)
	}
}

func TestAckIsRecordedAndConfirmed(t *testing.T) {
	engine, acked := setup(t)
	ann := chanxtest.Connect(t, engine, "/ws?as=ann")
	ann.Subscribe("notification:user:ann")

	replies := ann.Request(NotificationAckMessage{Payload: NotificationAckPayload{IDs: []string{"n1", "n2"}}}, chanxtest.Topic("notification:user:ann"))

	if replies[0].Action() != "notification_acked" {
		t.Fatalf("got %v", replies)
	}
	if len(*acked) != 2 || (*acked)[0] != "n1" {
		t.Fatalf("acked %v", *acked)
	}
}
