# presence

Track who is present in a room, document or tenant, and tell everyone when that changes.
Speaks **presence@1**, like the Python kit, so the `presence` UI kit works unchanged.

```bash
copit add @chanx-kit-go/presence
```

## Use it

List the topic on a consumer:

```go
func (RoomConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{presence.PresenceTopic{}}
}
```

A client subscribes to `presence:<scope>` and receives `presence_state` with the full
roster once, then `presence_join` / `presence_leave` as it changes. Join and leave reach
the whole scope, the member who triggered them included.

Presence is keyed on the member, not the connection: three tabs of one user produce one
join, and the leave fires when the last one closes.

## Customise

```go
presence.PresenceTopic{
	Store: redisStore,
	Member: func(conn *chanx.Conn) presence.PresenceMember {
		user := conn.User.(*User)
		return presence.PresenceMember{ID: user.ID, Name: &user.FullName,
			Data: map[string]any{"avatar": user.AvatarURL}}
	},
}
```

| Field | Default | Purpose |
|---|---|---|
| `Member` | `MemberOf`: a string user, or its `UserID()` and `UserName()` | Who is present |
| `Store` | `DefaultStore`, in memory | Where the roster lives |

`MembersOf(ctx, scope)` reads the roster from anywhere.

## Production: replace the store

`InMemoryPresenceStore` is per process: with two replicas each sees half the room.
Implement `PresenceStore` against shared storage.

## Messages

| Action | Direction | Payload |
|---|---|---|
| `presence_state` | server → client | `scope`, `members` |
| `presence_join` | server → client | `scope`, `member` |
| `presence_leave` | server → client | `scope`, `member` |
| `presence_request` | client → server | none |
