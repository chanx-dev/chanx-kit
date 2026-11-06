package presence

import (
	"context"
	"slices"
	"sync"
)

// PresenceStore backs the roster of each scope. The in-memory default is wrong once
// you run two processes: use the redis-presence-store kit or your own.
type PresenceStore interface {
	// Add records a connection; true when the member became newly present, so one
	// user with three tabs produces one join.
	Add(ctx context.Context, scope, connID string, member PresenceMember) (bool, error)
	// Discard forgets a connection; the member when that was their last one.
	Discard(ctx context.Context, scope, connID string) (*PresenceMember, error)
	// Members is everyone present in scope, once each.
	Members(ctx context.Context, scope string) ([]PresenceMember, error)
}

type entry struct {
	connID string
	member PresenceMember
}

// InMemoryPresenceStore is process-local: rosters are incomplete with several processes.
type InMemoryPresenceStore struct {
	mu     sync.Mutex
	scopes map[string][]entry
}

func NewInMemoryPresenceStore() *InMemoryPresenceStore {
	return &InMemoryPresenceStore{scopes: map[string][]entry{}}
}

func (s *InMemoryPresenceStore) Add(_ context.Context, scope, connID string, member PresenceMember) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.scopes[scope]
	present := slices.ContainsFunc(entries, func(e entry) bool { return e.member.ID == member.ID })
	entries = slices.DeleteFunc(entries, func(e entry) bool { return e.connID == connID })
	s.scopes[scope] = append(entries, entry{connID, member})
	return !present, nil
}

func (s *InMemoryPresenceStore) Discard(_ context.Context, scope, connID string) (*PresenceMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.scopes[scope]
	i := slices.IndexFunc(entries, func(e entry) bool { return e.connID == connID })
	if i < 0 {
		return nil, nil
	}
	member := entries[i].member
	entries = slices.Delete(entries, i, i+1)
	if len(entries) == 0 {
		delete(s.scopes, scope)
		return &member, nil
	}
	s.scopes[scope] = entries
	if slices.ContainsFunc(entries, func(e entry) bool { return e.member.ID == member.ID }) {
		return nil, nil
	}
	return &member, nil
}

func (s *InMemoryPresenceStore) Members(_ context.Context, scope string) ([]PresenceMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := []PresenceMember{}
	for _, e := range s.scopes[scope] {
		if !slices.ContainsFunc(members, func(m PresenceMember) bool { return m.ID == e.member.ID }) {
			members = append(members, e.member)
		}
	}
	return members, nil
}

// Clear drops all state, for tests.
func (s *InMemoryPresenceStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.scopes)
}
