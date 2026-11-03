// Package roomchat is a chat room: replayable history, delivered live.
package roomchat

import (
	"context"

	chanx "go.chanx.dev/v2"
)

// DefaultStore is the history a RoomChatTopic without a Store uses.
var DefaultStore MessageStore = NewInMemoryMessageStore()

// RoomChatTopic is chat:<room>. A subscriber gets chat_backlog with recent messages,
// then chat_message as they arrive.
type RoomChatTopic struct {
	chanx.BaseTopic `pattern:"chat:{room}" description:"Chat for one room."`
	// Store holds the history; DefaultStore when nil. The default is not durable.
	Store MessageStore
	// BacklogLimit caps the history sent on subscribe; 50 when zero.
	BacklogLimit int
	// Author is who a connection posts as; by default from its user, else anonymous.
	Author func(conn *chanx.Conn) ChatAuthor
}

func (t RoomChatTopic) store() MessageStore {
	if t.Store != nil {
		return t.Store
	}
	return DefaultStore
}

func (t RoomChatTopic) limit(requested *int) int {
	switch {
	case requested != nil && *requested > 0:
		return *requested
	case t.BacklogLimit > 0:
		return t.BacklogLimit
	default:
		return 50
	}
}

func (t RoomChatTopic) author(conn *chanx.Conn) ChatAuthor {
	if t.Author != nil {
		return t.Author(conn)
	}
	return AuthorOf(conn)
}

// AuthorOf is the default author: a string user, or one with UserID and UserName
// methods; anonymous, by connection, otherwise.
func AuthorOf(conn *chanx.Conn) ChatAuthor {
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
	return ChatAuthor{ID: id, Name: &name}
}

func (RoomChatTopic) Passthrough() []chanx.Message { return []chanx.Message{ChatMessage{}} }

func (t RoomChatTopic) OnSubscribe(ctx *chanx.Context) error {
	backlog, err := t.backlog(ctx, ctx.Params["room"], nil)
	if err != nil {
		return err
	}
	return ctx.Push(backlog)
}

func (t RoomChatTopic) HandleChatBacklogRequest(ctx *chanx.Context, m ChatBacklogRequestMessage) (ChatBacklogMessage, error) {
	return t.backlog(ctx, ctx.Params["room"], m.Payload.Limit)
}

// HandleChatSend persists first and publishes second, so an unsaved message is never shown.
func (t RoomChatTopic) HandleChatSend(ctx *chanx.Context, m ChatSendMessage) error {
	entry := NewChatEntry(ctx.Params["room"], m.Payload.Body, t.author(ctx.Conn))
	if err := t.store().Append(ctx, entry); err != nil {
		return err
	}
	return ctx.BroadcastTopic(ChatMessage{Payload: entry})
}

func (RoomChatTopic) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandleChatBacklogRequest": {Summary: "Request history", Description: "Return recent messages again, without reconnecting."},
		"HandleChatSend":           {Summary: "Post a chat message", Description: "Persist a message and publish it to everyone in the room.", Replies: []chanx.Message{ChatMessage{}}},
	}
}

func (t RoomChatTopic) backlog(ctx context.Context, room string, limit *int) (ChatBacklogMessage, error) {
	entries, err := t.store().Backlog(ctx, room, t.limit(limit))
	return ChatBacklogMessage{Payload: ChatBacklogPayload{Room: room, Entries: entries}}, err
}

// PostToRoom persists and delivers a message to room, with no connection involved.
func (t RoomChatTopic) PostToRoom(ctx context.Context, p *chanx.Publisher, room, body string, author ChatAuthor) (ChatEntry, error) {
	entry := NewChatEntry(room, body, author)
	if err := t.store().Append(ctx, entry); err != nil {
		return ChatEntry{}, err
	}
	topic, err := chanx.FormatTopic[RoomChatTopic](chanx.Params{"room": room})
	if err != nil {
		return ChatEntry{}, err
	}
	return entry, chanx.BroadcastTopic[RoomChatTopic](ctx, p, topic, ChatMessage{Payload: entry})
}

// History is recent messages for room, callable from anywhere.
func (t RoomChatTopic) History(ctx context.Context, room string, limit int) ([]ChatEntry, error) {
	return t.store().Backlog(ctx, room, limit)
}
