package audiostreamout

import (
	"context"
	"strings"
	"sync"
	"unicode"
)

type Utterance struct {
	Text       string
	DurationMs int
	PlayedMs   int
	Finished   bool
}

// HeardText is the part of text heard after playedMs, cut at a word.
func HeardText(text string, durationMs, playedMs int) string {
	if durationMs <= 0 {
		return ""
	}
	if playedMs >= durationMs {
		return text
	}
	runes := []rune(text)
	cut := len(runes) * playedMs / durationMs
	head := string(runes[:cut])
	if cut < len(runes) && !unicode.IsSpace(runes[cut]) && strings.Contains(head, " ") {
		head = head[:strings.LastIndex(head, " ")]
	}
	return strings.TrimRightFunc(head, unicode.IsSpace)
}

// PlaybackStore records what was said and how much was heard.
type PlaybackStore interface {
	Spoken(ctx context.Context, session, utteranceID, text string) error
	Finished(ctx context.Context, session, utteranceID string, durationMs int) error
	Mark(ctx context.Context, session, utteranceID string, playedMs int) error
	Get(ctx context.Context, session, utteranceID string) (*Utterance, error)
}

// InMemoryPlaybackStore keeps the last Keep utterances per session.
type InMemoryPlaybackStore struct {
	Keep int

	mu       sync.Mutex
	sessions map[string]*recent
}

type recent struct {
	order []string
	byID  map[string]*Utterance
}

func NewInMemoryPlaybackStore() *InMemoryPlaybackStore {
	return &InMemoryPlaybackStore{Keep: 50, sessions: map[string]*recent{}}
}

func (s *InMemoryPlaybackStore) session(name string) *recent {
	r := s.sessions[name]
	if r == nil {
		r = &recent{byID: map[string]*Utterance{}}
		s.sessions[name] = r
	}
	return r
}

func (s *InMemoryPlaybackStore) Spoken(_ context.Context, session, utteranceID, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.session(session)
	if _, seen := r.byID[utteranceID]; !seen {
		r.order = append(r.order, utteranceID)
	}
	r.byID[utteranceID] = &Utterance{Text: text}
	for len(r.order) > s.Keep {
		delete(r.byID, r.order[0])
		r.order = r.order[1:]
	}
	return nil
}

func (s *InMemoryPlaybackStore) Finished(_ context.Context, session, utteranceID string, durationMs int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.session(session).byID[utteranceID]; u != nil {
		u.DurationMs, u.Finished = durationMs, true
	}
	return nil
}

func (s *InMemoryPlaybackStore) Mark(_ context.Context, session, utteranceID string, playedMs int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.session(session).byID[utteranceID]; u != nil {
		u.PlayedMs = max(u.PlayedMs, playedMs)
	}
	return nil
}

func (s *InMemoryPlaybackStore) Get(_ context.Context, session, utteranceID string) (*Utterance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u := s.session(session).byID[utteranceID]; u != nil {
		copied := *u
		return &copied, nil
	}
	return nil, nil
}
