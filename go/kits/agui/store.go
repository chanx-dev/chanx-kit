package agui

import (
	"context"
	"sync"
)

// RunEventStore buffers one run per thread for replay; seq restarts with each run.
type RunEventStore interface {
	Append(ctx context.Context, threadID string, event Event) (int64, error)
	Replay(ctx context.Context, threadID string) ([]SeqEvent, error)
	Clear(ctx context.Context, threadID string) error
}

type SeqEvent struct {
	Seq   int64
	Event Event
}

// ActiveRunStore records which run holds each thread.
type ActiveRunStore interface {
	// Begin claims the thread for runID; the run already holding it, or "" on success.
	Begin(ctx context.Context, threadID, runID string) (string, error)
	// End releases the thread if runID still holds it.
	End(ctx context.Context, threadID, runID string) error
}

// InMemoryRunEventStore is process-local and bounded.
type InMemoryRunEventStore struct {
	MaxEventsPerRun int

	mu   sync.Mutex
	runs map[string][]SeqEvent
}

func NewInMemoryRunEventStore() *InMemoryRunEventStore {
	return &InMemoryRunEventStore{MaxEventsPerRun: 2000, runs: map[string][]SeqEvent{}}
}

func (s *InMemoryRunEventStore) Append(_ context.Context, threadID string, event Event) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buffered := s.runs[threadID]
	seq := int64(1)
	if n := len(buffered); n > 0 {
		seq = buffered[n-1].Seq + 1
	}
	if len(buffered) >= s.MaxEventsPerRun {
		buffered = buffered[1:]
	}
	s.runs[threadID] = append(buffered, SeqEvent{seq, event})
	return seq, nil
}

func (s *InMemoryRunEventStore) Replay(_ context.Context, threadID string) ([]SeqEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SeqEvent{}, s.runs[threadID]...), nil
}

func (s *InMemoryRunEventStore) Clear(_ context.Context, threadID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runs, threadID)
	return nil
}

// InMemoryActiveRunStore is process-local.
type InMemoryActiveRunStore struct {
	mu     sync.Mutex
	active map[string]string
}

func NewInMemoryActiveRunStore() *InMemoryActiveRunStore {
	return &InMemoryActiveRunStore{active: map[string]string{}}
}

func (s *InMemoryActiveRunStore) Begin(_ context.Context, threadID, runID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if holder, ok := s.active[threadID]; ok {
		return holder, nil
	}
	s.active[threadID] = runID
	return "", nil
}

func (s *InMemoryActiveRunStore) End(_ context.Context, threadID, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[threadID] == runID {
		delete(s.active, threadID)
	}
	return nil
}
