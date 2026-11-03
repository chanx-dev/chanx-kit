package notification

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	chanx "go.chanx.dev/v2"
)

type NotificationLevel string

const (
	LevelInfo    NotificationLevel = "info"
	LevelSuccess NotificationLevel = "success"
	LevelWarning NotificationLevel = "warning"
	LevelError   NotificationLevel = "error"
)

func (NotificationLevel) JSONSchema(s *jsonschema.Schema) {
	s.Enum = []any{"info", "success", "warning", "error"}
}

type NotificationPayload struct {
	Title     string            `json:"title"`
	Body      *string           `json:"body,omitempty"`
	Level     NotificationLevel `json:"level,omitempty"`
	ID        string            `json:"id,omitempty"`
	CreatedAt time.Time         `json:"created_at,omitzero"`
	Data      map[string]any    `json:"data,omitempty"`
}

// filled sets the defaults a sent notification always carries.
func (p NotificationPayload) filled() NotificationPayload {
	if p.Level == "" {
		p.Level = LevelInfo
	}
	if p.ID == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		p.ID = hex.EncodeToString(b)
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.Data == nil {
		p.Data = map[string]any{}
	}
	return p
}

type NotificationMessage struct {
	chanx.BaseMessage `action:"notification" description:"A notification delivered to the client."`
	Payload           NotificationPayload `json:"payload"`
}

type NotificationAckPayload struct {
	IDs []string `json:"ids"`
}

type NotificationAckMessage struct {
	chanx.BaseMessage `action:"notification_ack" description:"Client acknowledges that notifications were seen."`
	Payload           NotificationAckPayload `json:"payload"`
}

type NotificationAckedMessage struct {
	chanx.BaseMessage `action:"notification_acked" description:"Server confirms an acknowledgement was recorded."`
	Payload           NotificationAckPayload `json:"payload"`
}
