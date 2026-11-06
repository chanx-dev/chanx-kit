// Package notification fans notifications out to live connections, one topic per audience.
package notification

import (
	"context"

	chanx "go.chanx.dev/v2"
)

// Acks is the behaviour every notification topic shares.
type Acks struct {
	// OnAcked records that the client saw these notifications. No-op when nil.
	OnAcked func(ctx *chanx.Context, ids []string) error
}

func (a Acks) HandleNotificationAck(ctx *chanx.Context, m NotificationAckMessage) (NotificationAckedMessage, error) {
	if a.OnAcked != nil {
		if err := a.OnAcked(ctx, m.Payload.IDs); err != nil {
			return NotificationAckedMessage{}, err
		}
	}
	return NotificationAckedMessage{Payload: m.Payload}, nil
}

func (Acks) Passthrough() []chanx.Message { return []chanx.Message{NotificationMessage{}} }

// UserNotificationTopic is one user's notifications: only that user may subscribe.
type UserNotificationTopic struct {
	chanx.BaseTopic `pattern:"notification:user:{user_id}" description:"Notifications addressed to one user."`
	Acks
	// UserID is the connection's user id; by default a string user, or its UserID().
	UserID func(conn *chanx.Conn) string
}

func (t UserNotificationTopic) Authorize(ctx *chanx.Context, p chanx.Params) (bool, error) {
	id := t.currentUserID(ctx.Conn)
	return id != "" && id == p["user_id"], nil
}

func (t UserNotificationTopic) currentUserID(conn *chanx.Conn) string {
	if t.UserID != nil {
		return t.UserID(conn)
	}
	switch user := conn.User.(type) {
	case string:
		return user
	case interface{ UserID() string }:
		return user.UserID()
	default:
		return ""
	}
}

// SubjectNotificationTopic is a subject users opt into.
type SubjectNotificationTopic struct {
	chanx.BaseTopic `pattern:"notification:subject:{subject}" description:"Notifications for a subject users opt into."`
	Acks
}

// BroadcastNotificationTopic reaches everyone subscribed.
type BroadcastNotificationTopic struct {
	chanx.BaseTopic `pattern:"notification:all" description:"Notifications addressed to everyone."`
	Acks
}

// NotifyUser sends a notification to every live connection of one user.
func NotifyUser(ctx context.Context, p *chanx.Publisher, userID string, n NotificationPayload) error {
	return notify[UserNotificationTopic](ctx, p, chanx.Params{"user_id": userID}, n)
}

// NotifySubject sends a notification to everyone subscribed to subject.
func NotifySubject(ctx context.Context, p *chanx.Publisher, subject string, n NotificationPayload) error {
	return notify[SubjectNotificationTopic](ctx, p, chanx.Params{"subject": subject}, n)
}

// NotifyAll sends a notification to every subscribed client.
func NotifyAll(ctx context.Context, p *chanx.Publisher, n NotificationPayload) error {
	return notify[BroadcastNotificationTopic](ctx, p, nil, n)
}

func notify[T chanx.TopicHandler](ctx context.Context, p *chanx.Publisher, params chanx.Params, n NotificationPayload) error {
	topic, err := chanx.FormatTopic[T](params)
	if err != nil {
		return err
	}
	return chanx.BroadcastTopic[T](ctx, p, topic, NotificationMessage{Payload: n.filled()})
}
