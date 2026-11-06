# room-chat

A chat room: persisted history replayed on connect, plus a live roster. Speaks
**chat@1**, like the Python kit, so the `chat` UI kit works unchanged.

```bash
copit add @chanx-kit-go/room-chat   # installs presence too
```

## Use it

List the topic on a consumer, next to presence for the roster:

```go
func (RoomConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{roomchat.RoomChatTopic{}, presence.PresenceTopic{}}
}
```

A client subscribes to `chat:<room>` and receives `chat_backlog` with recent history,
then `chat_message` for anything posted afterwards. Posting persists before it
publishes, so a message that fails to save is never shown as delivered.

## Post without a connection

```go
chat := roomchat.RoomChatTopic{}
chat.PostToRoom(ctx, engine.Publisher(), "general", "deploy finished",
	roomchat.ChatAuthor{ID: "bot"})
entries, _ := chat.History(ctx, "general", 20)
```

## Customise

| Field | Default | Purpose |
|---|---|---|
| `Author` | `AuthorOf`: a string user, or its `UserID()` and `UserName()` | Who the connection posts as |
| `Store` | `DefaultStore`, in memory | Where history lives |
| `BacklogLimit` | `50` | How much history a joiner gets |

## Production: replace the store

`InMemoryMessageStore` is bounded and per process: history is lost on restart and
differs between replicas. Implement `MessageStore` against your database.

## Messages

| Action | Direction | Payload |
|---|---|---|
| `chat_send` | client → server | `body` |
| `chat_message` | server → client | `body`, `author`, `room`, `id`, `sent_at` |
| `chat_backlog` | server → client | `room`, `entries` |
| `chat_backlog_request` | client → server | `limit` |
