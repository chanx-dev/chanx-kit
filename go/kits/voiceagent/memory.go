package voiceagent

import (
	"context"
	"sync"
)

// Turn is an AG-UI user or assistant message.
type Turn = map[string]any

// Memory keeps the conversation, since no client sends it.
type Memory interface {
	Load(ctx context.Context, threadID string) ([]Turn, error)
	Save(ctx context.Context, threadID string, turns []Turn) error
}

// InMemoryMemory is process-local and bounded per thread.
type InMemoryMemory struct {
	MaxTurns int

	mu      sync.Mutex
	threads map[string][]Turn
}

func NewInMemoryMemory() *InMemoryMemory {
	return &InMemoryMemory{MaxTurns: 100, threads: map[string][]Turn{}}
}

func (m *InMemoryMemory) Load(_ context.Context, threadID string) ([]Turn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Turn(nil), m.threads[threadID]...), nil
}

func (m *InMemoryMemory) Save(_ context.Context, threadID string, turns []Turn) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.threads[threadID] = append([]Turn(nil), turns[max(0, len(turns)-m.MaxTurns):]...)
	return nil
}
