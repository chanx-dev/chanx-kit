package main

import (
	"strings"

	chanx "go.chanx.dev/v2"

	"go.chanx.dev/kit/notification"
	"go.chanx.dev/kit/presence"
	"go.chanx.dev/kit/roomchat"
)

// demoUser is what a real Authenticate would return. The kits read identity from
// UserID and UserName, so they are listed unchanged.
type demoUser struct{ id string }

func (u demoUser) UserID() string { return u.id }

func (u demoUser) UserName() string { return strings.ToUpper(u.id[:1]) + u.id[1:] }

// demoAuth stands in for auth: ?as=<name>. Anonymous without it.
type demoAuth struct{}

func (demoAuth) Authenticate(_ *chanx.Context, r *chanx.Request) (any, error) {
	if who := r.Query.Get("as"); who != "" {
		return demoUser{id: who}, nil
	}
	return nil, nil
}

func (demoAuth) HandlePing(*chanx.Context, chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// NotificationConsumer is the notification kit: one user's stream, a subject, everyone.
type NotificationConsumer struct{ demoAuth }

func (NotificationConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{
		notification.UserNotificationTopic{},
		notification.SubjectNotificationTopic{},
		notification.BroadcastNotificationTopic{},
	}
}

// RoomConsumer lists two independent kits side by side: chat and the roster.
type RoomConsumer struct{ demoAuth }

func (RoomConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{roomchat.RoomChatTopic{}, presence.PresenceTopic{}}
}

func mount(engine *chanx.Engine) {
	engine.Handle("/ws/notifications", NotificationConsumer{}, chanx.Name("notifications"),
		chanx.Description("Fan-out notifications for the connected user."))
	engine.Handle("/ws/rooms/{room}", RoomConsumer{}, chanx.Name("room"),
		chanx.Description("Room chat with history and a live roster."))
}
