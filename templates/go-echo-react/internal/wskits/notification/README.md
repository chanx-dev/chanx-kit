# notification

Fan out notifications to a user's live connections from anywhere in your app: an HTTP
handler, a background worker, another service. Speaks **notification@1**, like the Python
kit, so the `notification` UI kit works unchanged.

```bash
copit add @chanx-kit-go/notification
```

## Use it

List the topics on a consumer:

```go
import "example.com/app/internal/wskits/notification"

type AppConsumer struct{}

func (AppConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{
		notification.UserNotificationTopic{},
		notification.BroadcastNotificationTopic{},
	}
}
```

A client subscribes to the audiences it wants (`notification:user:<id>`,
`notification:subject:<name>`, `notification:all`). Asking for another user's
notifications is refused: the user id is the connection's `User`, as returned by
`Authenticate`, when it is a string or has a `UserID() string` method.

## Send a notification

From anywhere with a publisher on the same layers:

```go
pub := engine.Publisher()

notification.NotifyUser(ctx, pub, user.ID, notification.NotificationPayload{
	Title: "Build finished", Level: notification.LevelSuccess,
})
notification.NotifyAll(ctx, pub, notification.NotificationPayload{Title: "Maintenance at 22:00"})
```

`id`, `created_at` and `level` are filled when left empty.

## Customise

Set the fields, or edit the copied code:

```go
notification.UserNotificationTopic{
	UserID: func(conn *chanx.Conn) string { return conn.User.(*User).ID },
	Acks: notification.Acks{OnAcked: func(ctx *chanx.Context, ids []string) error {
		return store.MarkRead(ctx, ids)
	}},
}
```

| Field | Default | Purpose |
|---|---|---|
| `UserID` | `Conn.User` as a string, or its `UserID()` | Whose notifications this connection gets |
| `Acks.OnAcked` | no-op | Persist read state |

## Messages

| Action | Direction | Payload |
|---|---|---|
| `notification` | server → client | `title`, `body`, `level`, `id`, `created_at`, `data` |
| `notification_ack` | client → server | `ids` |
| `notification_acked` | server → client | `ids` |
