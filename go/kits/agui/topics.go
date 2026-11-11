// Package agui serves the AG-UI protocol over chanx-go, speaking the Python kit's ag-ui@2.
package agui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"sync"

	chanx "go.chanx.dev/v2"
)

// Defaults for an AgUiTopic without its own stores.
var (
	DefaultRunEventStore  RunEventStore  = NewInMemoryRunEventStore()
	DefaultActiveRunStore ActiveRunStore = NewInMemoryActiveRunStore()
)

// Stream is a run's events; a non-nil error ends it as RUN_ERROR.
type Stream func(ctx context.Context, input RunAgentInput) iter.Seq2[Event, error]

// AgUiTopic is agui:thread:<thread_id>. Set RunAgent, or RunEvents for a provider
// that emits its own lifecycle; a stream should stop when ctx is done.
type AgUiTopic struct {
	chanx.BaseTopic `pattern:"agui:thread:{thread_id}" description:"The AG-UI protocol for one thread."`
	// RunAgent yields content events; RUN_STARTED and RUN_FINISHED are added around them.
	RunAgent Stream
	// RunEvents yields the whole stream, lifecycle included.
	RunEvents Stream
	// Broadcast sends each run to every connection on the thread, replaying it to late joiners.
	Broadcast bool
	// Transcript is what a reconnecting client is shown first, usually a MESSAGES_SNAPSHOT.
	Transcript     func(ctx context.Context, threadID string) (Event, error)
	RunEventStore  RunEventStore
	ActiveRunStore ActiveRunStore
}

// ThreadBusyError is returned when a thread already has a run in flight.
type ThreadBusyError struct{ ThreadID, ActiveRunID string }

func (e *ThreadBusyError) Error() string {
	return fmt.Sprintf("Thread %s is already running %s.", e.ThreadID, e.ActiveRunID)
}

type run struct {
	cancel context.CancelCauseFunc
	owner  string
}

// errLeft stops a run whose only watcher left: there is no one to tell it ended.
var errLeft = errors.New("agui: the run's connection left")

// runs are this process's runs in flight, by thread and run id.
var runs = struct {
	sync.Mutex
	m map[[2]string]run
}{m: map[[2]string]run{}}

func newRunID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (t AgUiTopic) events() RunEventStore {
	if t.RunEventStore != nil {
		return t.RunEventStore
	}
	return DefaultRunEventStore
}

func (t AgUiTopic) active() ActiveRunStore {
	if t.ActiveRunStore != nil {
		return t.ActiveRunStore
	}
	return DefaultActiveRunStore
}

// Events is a run's whole stream: RunEvents, or RunAgent inside the run's lifecycle.
func (t AgUiTopic) Events(ctx context.Context, in RunAgentInput) iter.Seq2[Event, error] {
	if t.RunEvents != nil {
		return t.RunEvents(ctx, in)
	}
	return func(yield func(Event, error) bool) {
		if t.RunAgent == nil {
			yield(nil, fmt.Errorf("agui: AgUiTopic needs RunAgent or RunEvents"))
			return
		}
		if !yield(RunStarted(in.ThreadID, in.RunID), nil) {
			return
		}
		for event, err := range t.RunAgent(ctx, in) {
			if !yield(event, err) || err != nil {
				return
			}
		}
		yield(RunFinished(in.ThreadID, in.RunID), nil)
	}
}

// drive runs a claimed run to its end and releases the thread, however it ends.
func (t AgUiTopic) drive(parent context.Context, in RunAgentInput, emit func(Event) error, owner string) {
	ctx, cancel := context.WithCancelCause(parent)
	key := [2]string{in.ThreadID, in.RunID}
	runs.Lock()
	runs.m[key] = run{cancel, owner}
	runs.Unlock()
	defer func() {
		cancel(nil)
		runs.Lock()
		delete(runs.m, key)
		runs.Unlock()
		_ = t.active().End(context.Background(), in.ThreadID, in.RunID)
	}()

	var failure error
	for event, err := range t.Events(ctx, in) {
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			failure = err
			break
		}
		if emit(event) != nil {
			return
		}
	}
	// A cancelled run did not produce what was asked, so it ends as an error.
	switch {
	case errors.Is(context.Cause(ctx), errLeft):
	case ctx.Err() != nil:
		_ = emit(RunError("Run cancelled."))
	case failure != nil:
		_ = emit(RunError(failure.Error()))
	}
}

func (t AgUiTopic) broadcastRunEvent(ctx context.Context, p *chanx.Publisher, threadID string, event Event) error {
	store := t.events()
	if event.Type() == "RUN_STARTED" {
		if err := store.Clear(ctx, threadID); err != nil {
			return err
		}
	}
	seq, err := store.Append(ctx, threadID, event)
	if err != nil {
		return err
	}
	err = chanx.BroadcastTopic[AgUiTopic](ctx, p, "agui:thread:"+threadID, AgUiEventMessage{Payload: event}, chanx.Seq(seq))
	if event.Type() == "RUN_FINISHED" || event.Type() == "RUN_ERROR" {
		_ = store.Clear(ctx, threadID)
	}
	return err
}

func (AgUiTopic) Passthrough() []chanx.Message { return []chanx.Message{AgUiEventMessage{}} }

// OnSubscribe sends state first, then the run in flight: AG-UI cannot join a stream part-way.
func (t AgUiTopic) OnSubscribe(ctx *chanx.Context) error {
	threadID := ctx.Params["thread_id"]
	if t.Transcript != nil {
		event, err := t.Transcript(ctx, threadID)
		if err != nil {
			return err
		}
		if event != nil {
			if err := ctx.Push(AgUiEventMessage{Payload: event}); err != nil {
				return err
			}
		}
	}
	if !t.Broadcast {
		return nil
	}
	replay, err := t.events().Replay(ctx, threadID)
	if err != nil {
		return err
	}
	for _, se := range replay {
		at := *ctx
		at.Seq = &se.Seq
		if err := at.Push(AgUiEventMessage{Payload: se.Event}); err != nil {
			return err
		}
	}
	return nil
}

// OnUnsubscribe stops this socket's runs; a broadcast run belongs to the thread instead.
func (t AgUiTopic) OnUnsubscribe(ctx *chanx.Context) error {
	if t.Broadcast {
		return nil
	}
	runs.Lock()
	defer runs.Unlock()
	for key, r := range runs.m {
		if key[0] == ctx.Params["thread_id"] && r.owner == ctx.Conn.ChannelName {
			r.cancel(errLeft)
		}
	}
	return nil
}

func (t AgUiTopic) HandleAgUiRun(ctx *chanx.Context, m AgUiRunMessage) error {
	in := m.Payload
	in.ThreadID = ctx.Params["thread_id"]
	if in.RunID == "" {
		in.RunID = newRunID()
	}
	holder, err := t.active().Begin(ctx, in.ThreadID, in.RunID)
	if err != nil {
		return err
	}
	if holder != "" {
		// Only the asking connection hears this; the run in flight carries on.
		return ctx.Send(AgUiEventMessage{Payload: RunError(fmt.Sprintf("Thread is already running %s.", holder))})
	}
	if t.Broadcast {
		pub := ctx.Publisher()
		go t.drive(context.Background(), in, func(e Event) error {
			return t.broadcastRunEvent(context.Background(), pub, in.ThreadID, e)
		}, "")
		return nil
	}
	push := *ctx
	go t.drive(ctx.Conn.Context(), in, func(e Event) error {
		return push.Push(AgUiEventMessage{Payload: e})
	}, ctx.Conn.ChannelName)
	return nil
}

func (AgUiTopic) HandleAgUiCancel(ctx *chanx.Context, m AgUiCancelMessage) error {
	runs.Lock()
	defer runs.Unlock()
	if r, ok := runs.m[[2]string{ctx.Params["thread_id"], m.Payload.RunID}]; ok {
		r.cancel(nil)
	}
	return nil
}

func (AgUiTopic) Operations() map[string]chanx.Operation {
	replies := []chanx.Message{AgUiEventMessage{}}
	return map[string]chanx.Operation{
		"HandleAgUiRun":    {Summary: "Run the agent", Description: "Run an AG-UI agent and stream its events back.", Replies: replies},
		"HandleAgUiCancel": {Summary: "Cancel the run", Description: "Stop the run in flight on this thread.", Replies: replies},
	}
}

// StartRun runs the agent on threadID from the server; it needs Broadcast.
func (t AgUiTopic) StartRun(ctx context.Context, p *chanx.Publisher, threadID string, in RunAgentInput) (string, error) {
	if !t.Broadcast {
		return "", fmt.Errorf("agui: StartRun needs Broadcast, since no connection asked for this run")
	}
	in.ThreadID = threadID
	if in.RunID == "" {
		in.RunID = newRunID()
	}
	holder, err := t.active().Begin(ctx, threadID, in.RunID)
	if err != nil {
		return "", err
	}
	if holder != "" {
		return "", &ThreadBusyError{threadID, holder}
	}
	go t.drive(context.Background(), in, func(e Event) error {
		return t.broadcastRunEvent(context.Background(), p, threadID, e)
	}, "")
	return in.RunID, nil
}

// CancelRun stops this process's run on threadID, any run when runID is empty.
func (AgUiTopic) CancelRun(threadID, runID string) bool {
	runs.Lock()
	defer runs.Unlock()
	stopped := false
	for key, r := range runs.m {
		if key[0] == threadID && (runID == "" || key[1] == runID) {
			r.cancel(nil)
			stopped = true
		}
	}
	return stopped
}

// Running reports whether a run on threadID is in flight in this process.
func (AgUiTopic) Running(threadID string) bool {
	runs.Lock()
	defer runs.Unlock()
	for key := range runs.m {
		if key[0] == threadID {
			return true
		}
	}
	return false
}

// EmitToThread emits into a thread from outside; recorded for replay while a run is in flight.
func (t AgUiTopic) EmitToThread(ctx context.Context, p *chanx.Publisher, threadID string, event Event) error {
	if t.Broadcast {
		replay, err := t.events().Replay(ctx, threadID)
		if err != nil {
			return err
		}
		if event.Type() == "RUN_STARTED" || len(replay) > 0 {
			return t.broadcastRunEvent(ctx, p, threadID, event)
		}
	}
	return chanx.BroadcastTopic[AgUiTopic](ctx, p, "agui:thread:"+threadID, AgUiEventMessage{Payload: event})
}

// AgUiRunTopic is agui:run:<run_id>, for work off the connection to emit into one run.
type AgUiRunTopic struct {
	chanx.BaseTopic `pattern:"agui:run:{run_id}" description:"One run's events."`
}

func (AgUiRunTopic) Passthrough() []chanx.Message { return []chanx.Message{AgUiEventMessage{}} }

// EmitToRun emits an event into one run from outside the connection.
func EmitToRun(ctx context.Context, p *chanx.Publisher, runID string, event Event) error {
	return chanx.BroadcastTopic[AgUiRunTopic](ctx, p, "agui:run:"+runID, AgUiEventMessage{Payload: event})
}
