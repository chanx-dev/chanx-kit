package roomchat

import (
	"context"
	"testing"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"
)

type roomConsumer struct{ topic RoomChatTopic }

func (roomConsumer) Authenticate(_ *chanx.Context, r *chanx.Request) (any, error) {
	return r.Query.Get("as"), nil
}

func (c roomConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

func setup(t *testing.T) (*chanx.Engine, RoomChatTopic) {
	t.Helper()
	topic := RoomChatTopic{Store: NewInMemoryMessageStore(), BacklogLimit: 2}
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", roomConsumer{topic: topic})
	return engine, topic
}

func join(t *testing.T, engine *chanx.Engine, user string) (*chanxtest.Client, []ChatEntry) {
	t.Helper()
	c := chanxtest.Connect(t, engine, "/ws?as="+user)
	c.Subscribe("chat:lobby")
	backlog, _ := chanxtest.ReceiveUntilMessage[ChatBacklogMessage](c)
	return c, backlog.Payload.Entries
}

func TestASentMessageReachesTheRoomWithItsAuthor(t *testing.T) {
	engine, _ := setup(t)
	ann, _ := join(t, engine, "ann")
	bob, _ := join(t, engine, "bob")

	ann.Send(ChatSendMessage{Payload: ChatSendPayload{Body: "hi"}}, chanxtest.Topic("chat:lobby"))

	got, _ := chanxtest.ReceiveUntilMessage[ChatMessage](bob)
	if got.Payload.Body != "hi" || got.Payload.Author.ID != "ann" || got.Payload.ID == "" || got.Payload.Room != "lobby" {
		t.Fatalf("got %+v", got.Payload)
	}
}

func TestJoiningReplaysTheBacklogUpToTheLimit(t *testing.T) {
	engine, topic := setup(t)
	ctx, pub := context.Background(), engine.Publisher()
	for _, body := range []string{"one", "two", "three"} {
		if _, err := topic.PostToRoom(ctx, pub, "lobby", body, ChatAuthor{ID: "bot"}); err != nil {
			t.Fatal(err)
		}
	}

	_, entries := join(t, engine, "ann")

	if len(entries) != 2 || entries[0].Body != "two" || entries[1].Body != "three" {
		t.Fatalf("backlog %+v", entries)
	}
}

func TestBacklogRequestHonoursItsLimit(t *testing.T) {
	engine, topic := setup(t)
	ctx, pub := context.Background(), engine.Publisher()
	for _, body := range []string{"one", "two", "three"} {
		_, _ = topic.PostToRoom(ctx, pub, "lobby", body, ChatAuthor{ID: "bot"})
	}
	ann, _ := join(t, engine, "ann")
	one := 1

	replies := ann.Request(ChatBacklogRequestMessage{Payload: ChatBacklogRequestPayload{Limit: &one}}, chanxtest.Topic("chat:lobby"))

	var backlog ChatBacklogMessage
	if err := replies[0].Decode(&backlog); err != nil || len(backlog.Payload.Entries) != 1 || backlog.Payload.Entries[0].Body != "three" {
		t.Fatalf("got %v", replies)
	}
}

func TestPostToRoomReachesSubscribers(t *testing.T) {
	engine, topic := setup(t)
	ann, _ := join(t, engine, "ann")

	_, err := topic.PostToRoom(context.Background(), engine.Publisher(), "lobby", "deployed", ChatAuthor{ID: "ci"})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := chanxtest.ReceiveUntilMessage[ChatMessage](ann)
	if got.Payload.Author.ID != "ci" {
		t.Fatalf("got %+v", got.Payload)
	}
}
