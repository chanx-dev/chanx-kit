package presence

import chanx "go.chanx.dev/v2"

// PresenceMember is someone present in a scope; ID identifies them across connections.
type PresenceMember struct {
	ID   string         `json:"id"`
	Name *string        `json:"name,omitempty"`
	Data map[string]any `json:"data,omitempty"`
}

type PresenceStatePayload struct {
	Scope   string           `json:"scope"`
	Members []PresenceMember `json:"members"`
}

type PresenceEventPayload struct {
	Scope  string         `json:"scope"`
	Member PresenceMember `json:"member"`
}

type PresenceStateMessage struct {
	chanx.BaseMessage `action:"presence_state" description:"Full member list, sent to a connection when it joins."`
	Payload           PresenceStatePayload `json:"payload"`
}

type PresenceJoinMessage struct {
	chanx.BaseMessage `action:"presence_join" description:"Someone became present in the scope."`
	Payload           PresenceEventPayload `json:"payload"`
}

type PresenceLeaveMessage struct {
	chanx.BaseMessage `action:"presence_leave" description:"Someone is no longer present in the scope."`
	Payload           PresenceEventPayload `json:"payload"`
}

type PresenceRequestMessage struct {
	chanx.BaseMessage `action:"presence_request" description:"Client asks for the roster again, without reconnecting."`
}
