package mediastreamout

import (
	"context"
	"sync"

	chanx "go.chanx.dev/v2"
)

type Entry struct {
	Seq     int64
	Message chanx.Message
}

// ReplayStore buffers a stream per key; sequence numbers restart after Clear.
type ReplayStore interface {
	// Append returns the message's sequence number, from 1.
	Append(ctx context.Context, key string, message chanx.Message) (int64, error)
	Replay(ctx context.Context, key string) ([]Entry, error)
	Clear(ctx context.Context, key string) error
}

// InMemoryReplayStore is process-local and bounded.
type InMemoryReplayStore struct {
	MaxMessages int

	mu      sync.Mutex
	streams map[string][]Entry
}

func NewInMemoryReplayStore() *InMemoryReplayStore {
	return &InMemoryReplayStore{MaxMessages: 3000, streams: map[string][]Entry{}}
}

func (s *InMemoryReplayStore) Append(_ context.Context, key string, message chanx.Message) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buffered := s.streams[key]
	seq := int64(1)
	if n := len(buffered); n > 0 {
		seq = buffered[n-1].Seq + 1
	}
	// A late joiner sees the dropped start as a gap.
	if len(buffered) >= s.MaxMessages {
		buffered = buffered[1:]
	}
	s.streams[key] = append(buffered, Entry{seq, message})
	return seq, nil
}

func (s *InMemoryReplayStore) Replay(_ context.Context, key string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Entry(nil), s.streams[key]...), nil
}

func (s *InMemoryReplayStore) Clear(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streams, key)
	return nil
}
