package roomchat

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	chanx "go.chanx.dev/v2"
)

type ChatAuthor struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

// ChatEntry is one persisted chat message.
type ChatEntry struct {
	Body   string     `json:"body"`
	Author ChatAuthor `json:"author"`
	Room   string     `json:"room"`
	ID     string     `json:"id,omitempty"`
	SentAt time.Time  `json:"sent_at,omitzero"`
}

// NewChatEntry is an entry with its id and time filled.
func NewChatEntry(room, body string, author ChatAuthor) ChatEntry {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return ChatEntry{Body: body, Author: author, Room: room, ID: hex.EncodeToString(b), SentAt: time.Now().UTC()}
}

type ChatSendPayload struct {
	Body string `json:"body"`
}

type ChatSendMessage struct {
	chanx.BaseMessage `action:"chat_send" description:"Client posts a message to the room."`
	Payload           ChatSendPayload `json:"payload"`
}

type ChatMessage struct {
	chanx.BaseMessage `action:"chat_message" description:"A message published to the room."`
	Payload           ChatEntry `json:"payload"`
}

type ChatBacklogPayload struct {
	Room    string      `json:"room"`
	Entries []ChatEntry `json:"entries"`
}

type ChatBacklogMessage struct {
	chanx.BaseMessage `action:"chat_backlog" description:"Recent history, sent to a connection when it joins."`
	Payload           ChatBacklogPayload `json:"payload"`
}

type ChatBacklogRequestPayload struct {
	Limit *int `json:"limit,omitempty"`
}

type ChatBacklogRequestMessage struct {
	chanx.BaseMessage `action:"chat_backlog_request" description:"Client asks for history again, without reconnecting."`
	Payload           ChatBacklogRequestPayload `json:"payload,omitzero"`
}
