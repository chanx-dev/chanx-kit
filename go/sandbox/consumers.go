package main

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	chanx "go.chanx.dev/v2"

	"go.chanx.dev/kit/kits/agui"
	"go.chanx.dev/kit/kits/notification"
	"go.chanx.dev/kit/kits/presence"
	"go.chanx.dev/kit/kits/roomchat"
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

// AgentConsumer speaks AG-UI with a scripted reply: the sandbox has no model.
type AgentConsumer struct{ demoAuth }

func (AgentConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{agui.AgUiTopic{RunAgent: cannedReply}, agui.AgUiRunTopic{}}
}

func cannedReply(ctx context.Context, in agui.RunAgentInput) iter.Seq2[agui.Event, error] {
	return func(yield func(agui.Event, error) bool) {
		last := ""
		if n := len(in.Messages); n > 0 {
			last, _ = in.Messages[n-1]["content"].(string)
		}
		reply := fmt.Sprintf("You said: %q. This sandbox has no model credentials, so these AG-UI "+
			"events are produced locally to show the protocol travelling over a chanx websocket.", last)
		if !yield(agui.TextMessageStart("m-"+in.RunID), nil) {
			return
		}
		for _, word := range strings.Fields(reply) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(40 * time.Millisecond):
			}
			if !yield(agui.TextMessageContent("m-"+in.RunID, word+" "), nil) {
				return
			}
		}
		yield(agui.TextMessageEnd("m-"+in.RunID), nil)
	}
}

func mount(engine *chanx.Engine) {
	engine.Handle("/ws/notifications", NotificationConsumer{}, chanx.Name("notifications"),
		chanx.Description("Fan-out notifications for the connected user."))
	engine.Handle("/ws/rooms/{room}", RoomConsumer{}, chanx.Name("room"),
		chanx.Description("Room chat with history and a live roster."))
	engine.Handle("/ws/agent", AgentConsumer{}, chanx.Name("agent"),
		chanx.Description("AG-UI protocol over a websocket."))
	mountVoice(engine)
}
