package redispresencestore

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"go.chanx.dev/kit/presence"
)

func setup(t *testing.T) (*RedisPresenceStore, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	return New(redis.NewClient(&redis.Options{Addr: server.Addr()})), server
}

func member(id string) presence.PresenceMember { return presence.PresenceMember{ID: id} }

func TestASecondConnectionOfAMemberIsNotANewJoin(t *testing.T) {
	store, _ := setup(t)
	ctx := context.Background()

	first, _ := store.Add(ctx, "lobby", "c1", member("ann"))
	second, _ := store.Add(ctx, "lobby", "c2", member("ann"))

	if !first || second {
		t.Fatalf("joins %v %v", first, second)
	}
	members, _ := store.Members(ctx, "lobby")
	if len(members) != 1 {
		t.Fatalf("members %+v", members)
	}
}

func TestTheLastConnectionLeaving(t *testing.T) {
	store, _ := setup(t)
	ctx := context.Background()
	_, _ = store.Add(ctx, "lobby", "c1", member("ann"))
	_, _ = store.Add(ctx, "lobby", "c2", member("ann"))

	if left, _ := store.Discard(ctx, "lobby", "c1"); left != nil {
		t.Fatalf("left with a tab open: %+v", left)
	}
	left, _ := store.Discard(ctx, "lobby", "c2")
	if left == nil || left.ID != "ann" {
		t.Fatalf("left %+v", left)
	}
	if again, _ := store.Discard(ctx, "lobby", "c2"); again != nil {
		t.Fatalf("discarded twice: %+v", again)
	}
}

func TestTheRosterExpiresWithoutWrites(t *testing.T) {
	store, server := setup(t)
	ctx := context.Background()
	_, _ = store.Add(ctx, "lobby", "c1", member("ann"))

	server.FastForward(time.Hour + time.Second)

	if members, _ := store.Members(ctx, "lobby"); len(members) != 0 {
		t.Fatalf("members %+v", members)
	}
}

func TestItReadsWhatThePythonKitWrites(t *testing.T) {
	store, server := setup(t)
	server.HSet("chanx-kit:presence:lobby", "py-channel", `{"id":"bob","name":"Bob","data":{}}`)

	members, err := store.Members(context.Background(), "lobby")

	if err != nil || len(members) != 1 || *members[0].Name != "Bob" {
		t.Fatalf("members %+v %v", members, err)
	}
}
