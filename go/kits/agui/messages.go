package agui

import (
	"github.com/google/jsonschema-go/jsonschema"
	chanx "go.chanx.dev/v2"
)

// RunAgentInput is AG-UI's run request; its schema is the contract's.
type RunAgentInput struct {
	ThreadID       string           `json:"threadId"`
	RunID          string           `json:"runId"`
	ParentRunID    *string          `json:"parentRunId,omitempty"`
	State          any              `json:"state,omitempty"`
	Messages       []map[string]any `json:"messages"`
	Tools          []map[string]any `json:"tools"`
	Context        []map[string]any `json:"context"`
	ForwardedProps any              `json:"forwardedProps,omitempty"`
}

func (RunAgentInput) JSONSchema(s *jsonschema.Schema) { *s = *contractSchema("RunAgentInput", true) }

// AgUiCancel names the run to stop, so a late cancel cannot stop the next one.
type AgUiCancel struct {
	RunID string `json:"runId"`
}

func (AgUiCancel) JSONSchema(s *jsonschema.Schema) { *s = *contractSchema("AgUiCancel", true) }

// Event is one AG-UI event, any member of the protocol's Event union.
type Event map[string]any

func (Event) JSONSchema(s *jsonschema.Schema) { *s = *contractSchema("AgUiEventMessage", false) }

// Type is the event's discriminator, such as RUN_STARTED.
func (e Event) Type() string { t, _ := e["type"].(string); return t }

type AgUiRunMessage struct {
	chanx.BaseMessage `action:"ag_ui_run" description:"Client asks the agent to run; the payload is AG-UI's own RunAgentInput."`
	Payload           RunAgentInput `json:"payload"`
}

type AgUiCancelMessage struct {
	chanx.BaseMessage `action:"ag_ui_cancel" description:"Client asks for the run in flight to stop."`
	Payload           AgUiCancel `json:"payload"`
}

type AgUiEventMessage struct {
	chanx.BaseMessage `action:"ag_ui_event" description:"One AG-UI event on its way to the client, as the protocol's Event union."`
	Payload           Event `json:"payload"`
}

func RunStarted(threadID, runID string) Event {
	return Event{"type": "RUN_STARTED", "threadId": threadID, "runId": runID}
}

func RunFinished(threadID, runID string) Event {
	return Event{"type": "RUN_FINISHED", "threadId": threadID, "runId": runID}
}

func RunError(message string) Event { return Event{"type": "RUN_ERROR", "message": message} }

func TextMessageStart(messageID string) Event {
	return Event{"type": "TEXT_MESSAGE_START", "messageId": messageID, "role": "assistant"}
}

func TextMessageContent(messageID, delta string) Event {
	return Event{"type": "TEXT_MESSAGE_CONTENT", "messageId": messageID, "delta": delta}
}

func TextMessageEnd(messageID string) Event {
	return Event{"type": "TEXT_MESSAGE_END", "messageId": messageID}
}
