// Package presence tracks who is present in a scope, and tells everyone when that changes.
package presence

import (
	"context"

	chanx "go.chanx.dev/v2"
)

// DefaultStore is the roster a PresenceTopic without a Store uses.
var DefaultStore PresenceStore = NewInMemoryPresenceStore()

// PresenceTopic is presence:<scope>. A subscriber gets presence_state with the roster,
// then presence_join and presence_leave; a member with several connections joins once.
type PresenceTopic struct {
	chanx.BaseTopic `pattern:"presence:{scope}" description:"Who is present in a scope."`
	// Store holds the roster; DefaultStore when nil.
	Store PresenceStore
	// Member describes the connection; by default from its user, else anonymous.
	Member func(conn *chanx.Conn) PresenceMember
}

func (t PresenceTopic) store() PresenceStore {
	if t.Store != nil {
		return t.Store
	}
	return DefaultStore
}

func (t PresenceTopic) member(conn *chanx.Conn) PresenceMember {
	if t.Member != nil {
		return t.Member(conn)
	}
	return MemberOf(conn)
}

// MemberOf is the default member: a string user, or one with UserID and UserName
// methods; anonymous, by connection, otherwise.
func MemberOf(conn *chanx.Conn) PresenceMember {
	id, name := "", ""
	switch user := conn.User.(type) {
	case string:
		id, name = user, user
	case interface{ UserID() string }:
		id, name = user.UserID(), user.UserID()
		if named, ok := user.(interface{ UserName() string }); ok {
			name = named.UserName()
		}
	}
	if id == "" {
		id, name = conn.ChannelName, "anonymous"
	}
	return PresenceMember{ID: id, Name: &name}
}

func (PresenceTopic) Passthrough() []chanx.Message {
	return []chanx.Message{PresenceJoinMessage{}, PresenceLeaveMessage{}}
}

func (t PresenceTopic) OnSubscribe(ctx *chanx.Context) error {
	member := t.member(ctx.Conn)
	joined, err := t.store().Add(ctx, ctx.Params["scope"], ctx.Conn.ChannelName, member)
	if err != nil {
		return err
	}
	state, err := t.state(ctx, ctx.Params["scope"])
	if err != nil {
		return err
	}
	if err := ctx.Push(state); err != nil {
		return err
	}
	if !joined {
		return nil
	}
	return ctx.BroadcastTopic(PresenceJoinMessage{Payload: PresenceEventPayload{Scope: ctx.Params["scope"], Member: member}})
}

func (t PresenceTopic) OnUnsubscribe(ctx *chanx.Context) error {
	departed, err := t.store().Discard(ctx, ctx.Params["scope"], ctx.Conn.ChannelName)
	if err != nil || departed == nil {
		return err
	}
	return ctx.BroadcastTopic(PresenceLeaveMessage{Payload: PresenceEventPayload{Scope: ctx.Params["scope"], Member: *departed}})
}

func (t PresenceTopic) HandlePresenceRequest(ctx *chanx.Context, _ PresenceRequestMessage) (PresenceStateMessage, error) {
	return t.state(ctx, ctx.Params["scope"])
}

func (t PresenceTopic) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandlePresenceRequest": {Summary: "Request the roster", Description: "Return who is currently present, without reconnecting."},
	}
}

func (t PresenceTopic) state(ctx context.Context, scope string) (PresenceStateMessage, error) {
	members, err := t.store().Members(ctx, scope)
	return PresenceStateMessage{Payload: PresenceStatePayload{Scope: scope, Members: members}}, err
}

// MembersOf is who is present in scope, callable from anywhere.
func (t PresenceTopic) MembersOf(ctx context.Context, scope string) ([]PresenceMember, error) {
	return t.store().Members(ctx, scope)
}
