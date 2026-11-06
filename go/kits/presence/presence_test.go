package presence

import (
	"context"
	"testing"
	"time"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/chanxtest"
)

type roomConsumer struct{ topic PresenceTopic }

func (roomConsumer) Authenticate(_ *chanx.Context, r *chanx.Request) (any, error) {
	return r.Query.Get("as"), nil
}

func (c roomConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{c.topic} }

func setup(t *testing.T) (*chanx.Engine, PresenceTopic) {
	t.Helper()
	topic := PresenceTopic{Store: NewInMemoryPresenceStore()}
	engine := chanx.New(chanx.Config{})
	engine.Handle("/ws", roomConsumer{topic: topic})
	return engine, topic
}

func join(t *testing.T, engine *chanx.Engine, user string) (*chanxtest.Client, PresenceStatePayload) {
	t.Helper()
	c := chanxtest.Connect(t, engine, "/ws?as="+user)
	c.Subscribe("presence:lobby")
	state, _ := chanxtest.ReceiveUntilMessage[PresenceStateMessage](c)
	return c, state.Payload
}

func ids(members []PresenceMember) []string {
	out := []string{}
	for _, m := range members {
		out = append(out, m.ID)
	}
	return out
}

func TestJoinSendsTheRosterAndAnnouncesTheNewcomer(t *testing.T) {
	engine, _ := setup(t)
	ann, state := join(t, engine, "ann")
	if got := ids(state.Members); len(got) != 1 || got[0] != "ann" {
		t.Fatalf("roster %v", got)
	}
	_, _ = chanxtest.ReceiveUntilMessage[PresenceJoinMessage](ann)

	_, state = join(t, engine, "bob")

	if got := ids(state.Members); len(got) != 2 || got[1] != "bob" {
		t.Fatalf("roster %v", got)
	}
	joined, _ := chanxtest.ReceiveUntilMessage[PresenceJoinMessage](ann)
	if joined.Payload.Member.ID != "bob" || *joined.Payload.Member.Name != "bob" {
		t.Fatalf("join %+v", joined.Payload)
	}
}

func TestASecondTabNeitherJoinsNorLeaves(t *testing.T) {
	engine, _ := setup(t)
	bob, _ := join(t, engine, "bob")
	_, _ = chanxtest.ReceiveUntilMessage[PresenceJoinMessage](bob)

	tab, state := join(t, engine, "ann")
	_, _ = chanxtest.ReceiveUntilMessage[PresenceJoinMessage](tab)
	_, _ = chanxtest.ReceiveUntilMessage[PresenceJoinMessage](bob)
	second, _ := join(t, engine, "ann")
	if len(state.Members) != 2 {
		t.Fatalf("roster %v", ids(state.Members))
	}
	bob.ExpectNothing(100 * time.Millisecond)

	second.Close()
	bob.ExpectNothing(100 * time.Millisecond)

	tab.Unsubscribe("presence:lobby")
	left, _ := chanxtest.ReceiveUntilMessage[PresenceLeaveMessage](bob)
	if left.Payload.Member.ID != "ann" {
		t.Fatalf("leave %+v", left.Payload)
	}
}

func TestDisconnectAnnouncesTheLeave(t *testing.T) {
	engine, topic := setup(t)
	bob, _ := join(t, engine, "bob")
	_, _ = chanxtest.ReceiveUntilMessage[PresenceJoinMessage](bob)
	ann, _ := join(t, engine, "ann")
	_, _ = chanxtest.ReceiveUntilMessage[PresenceJoinMessage](bob)

	ann.Close()

	left, _ := chanxtest.ReceiveUntilMessage[PresenceLeaveMessage](bob)
	if left.Payload.Member.ID != "ann" {
		t.Fatalf("leave %+v", left.Payload)
	}
	members, _ := topic.MembersOf(context.Background(), "lobby")
	if got := ids(members); len(got) != 1 || got[0] != "bob" {
		t.Fatalf("members %v", got)
	}
}

func TestPresenceRequestReturnsTheRoster(t *testing.T) {
	engine, _ := setup(t)
	ann, _ := join(t, engine, "ann")
	_, _ = chanxtest.ReceiveUntilMessage[PresenceJoinMessage](ann)

	replies := ann.Request(PresenceRequestMessage{}, chanxtest.Topic("presence:lobby"))

	var state PresenceStateMessage
	if err := replies[0].Decode(&state); err != nil || ids(state.Payload.Members)[0] != "ann" {
		t.Fatalf("got %v", replies)
	}
}

func TestAnAnonymousConnectionIsPresentByConnection(t *testing.T) {
	engine, _ := setup(t)

	_, state := join(t, engine, "")

	if m := state.Members[0]; m.ID == "" || *m.Name != "anonymous" {
		t.Fatalf("member %+v", m)
	}
}
