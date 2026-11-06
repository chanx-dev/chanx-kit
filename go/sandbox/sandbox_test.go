package main

import (
	"slices"
	"testing"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"
	"go.chanx.dev/v2/chanxtest"

	"go.chanx.dev/kit/kits/presence"
)

func TestTheUIsChannelsAndTopicsArePublished(t *testing.T) {
	engine := chanx.New(chanx.Config{})
	mount(engine)

	doc, err := asyncapi.Generate(engine, info)
	if err != nil {
		t.Fatal(err)
	}
	channels, _ := doc.Get("channels")
	keys := channels.(*asyncapi.Map).Keys()
	for _, want := range []string{"notifications", "notification_user_notification_topic", "room", "room_room_chat_topic", "room_presence_topic"} {
		if !slices.Contains(keys, want) {
			t.Errorf("no channel %s in %v", want, keys)
		}
	}
}

func TestQueryIdentityReachesTheKits(t *testing.T) {
	engine := chanx.New(chanx.Config{})
	mount(engine)
	ann := chanxtest.Connect(t, engine, "/ws/rooms/lobby?as=ann")

	ann.Subscribe("presence:lobby")

	state, _ := chanxtest.ReceiveUntilMessage[presence.PresenceStateMessage](ann)
	if m := state.Payload.Members[0]; m.ID != "ann" || *m.Name != "Ann" {
		t.Fatalf("member %+v", m)
	}
}

func TestEveryFrameworkIsServed(t *testing.T) {
	for _, name := range []string{"nethttp", "chi", "gin", "echo", "fiber"} {
		if frameworks[name] == nil {
			t.Errorf("no %s", name)
		}
	}
}
