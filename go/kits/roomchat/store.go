package roomchat

import (
	"context"
	"sync"
)

// MessageStore persists room history. Implement it against your database; the
// in-memory default is for demos and tests.
type MessageStore interface {
	Append(ctx context.Context, entry ChatEntry) error
	// Backlog is up to limit recent messages, oldest first.
	Backlog(ctx context.Context, room string, limit int) ([]ChatEntry, error)
}

// InMemoryMessageStore is process-local, bounded history: not durable, not shared.
type InMemoryMessageStore struct {
	MaxPerRoom int

	mu    sync.Mutex
	rooms map[string][]ChatEntry
}

func NewInMemoryMessageStore() *InMemoryMessageStore {
	return &InMemoryMessageStore{MaxPerRoom: 200, rooms: map[string][]ChatEntry{}}
}

func (s *InMemoryMessageStore) Append(_ context.Context, entry ChatEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := append(s.rooms[entry.Room], entry)
	if over := len(entries) - s.MaxPerRoom; over > 0 {
		entries = entries[over:]
	}
	s.rooms[entry.Room] = entries
	return nil
}

func (s *InMemoryMessageStore) Backlog(_ context.Context, room string, limit int) ([]ChatEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.rooms[room]
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return append([]ChatEntry{}, entries...), nil
}

// Clear drops all history, for tests.
func (s *InMemoryMessageStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.rooms)
}
