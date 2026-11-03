// Package app holds the WebSocket consumers. After copit add, list the kit's topics here.
package app

import (
	"context"
	"os"

	chanx "go.chanx.dev/v2"
	"go.chanx.dev/layers/redis/v2"
	"go.chanx.dev/layers/v2"
	"go.chanx.dev/layers/v2/inmemory"
	"go.chanx.dev/v2/asyncapi"

	"app/internal/wskits/notification"
)

var Info = asyncapi.Info{Title: "chanx app", Version: "0.1.0", Description: "WebSocket API of this app."}

// User is what Authenticate returns; the kits read UserID.
type User struct{ ID string }

func (u User) UserID() string { return u.ID }

type NotificationConsumer struct{}

// Authenticate is demo identity from ?as=<name>. Replace with your auth.
func (NotificationConsumer) Authenticate(_ *chanx.Context, r *chanx.Request) (any, error) {
	if who := r.Query.Get("as"); who != "" {
		return User{ID: who}, nil
	}
	return nil, nil
}

func (NotificationConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{
		notification.UserNotificationTopic{},
		notification.BroadcastNotificationTopic{},
	}
}

func (NotificationConsumer) HandlePing(*chanx.Context, chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

func New() *chanx.Engine {
	engine := chanx.New(chanx.Config{Layers: Layers()})
	engine.Handle("/ws/notifications", NotificationConsumer{}, chanx.Name("notifications"),
		chanx.Description("Notifications for the connected user, and for everyone."))
	return engine
}

// Layers is Redis when REDIS_URL is set (needed across processes), in memory otherwise.
func Layers() layers.Set {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		return layers.Set{"default": inmemory.New()}
	}
	l, err := redis.NewPubSub(redis.PubSubOptions{Hosts: []string{url}, Prefix: "chanx-app"})
	if err != nil {
		panic(err)
	}
	return layers.Set{"default": l}
}

// Notify reaches one user's connections, or everyone when user is empty.
func Notify(ctx context.Context, pub *chanx.Publisher, title, user string) error {
	payload := notification.NotificationPayload{Title: title}
	if user != "" {
		return notification.NotifyUser(ctx, pub, user, payload)
	}
	return notification.NotifyAll(ctx, pub, payload)
}

// NotifyRequest is the body of POST /api/notify.
type NotifyRequest struct {
	Title string `json:"title"`
	User  string `json:"user"`
}
