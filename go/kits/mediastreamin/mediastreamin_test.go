package mediastreamin

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"
)

func chunks(s ...string) [][]byte {
	out := make([][]byte, len(s))
	for i, c := range s {
		out[i] = []byte(c)
	}
	return out
}

func check(t *testing.T, ready [][]byte, gaps []Gap, wantReady [][]byte, wantGaps []Gap) {
	t.Helper()
	if len(ready) != len(wantReady) || (len(ready) > 0 && !reflect.DeepEqual(ready, wantReady)) {
		t.Fatalf("ready = %q, want %q", ready, wantReady)
	}
	if len(gaps) != len(wantGaps) || (len(gaps) > 0 && !reflect.DeepEqual(gaps, wantGaps)) {
		t.Fatalf("gaps = %v, want %v", gaps, wantGaps)
	}
}

func TestBytesTravelAsStandardBase64(t *testing.T) {
	out, _ := json.Marshal(struct {
		Data []byte `json:"data"`
	}{[]byte{0xff, 0x00, 0x10}})
	if string(out) != `{"data":"/wAQ"}` {
		t.Fatalf("got %s", out)
	}
}

func TestInOrderChunksPassStraightThrough(t *testing.T) {
	s := NewChunkSequencer(8)
	r, g := s.Push(0, []byte("a"))
	check(t, r, g, chunks("a"), nil)
	r, g = s.Push(1, []byte("b"))
	check(t, r, g, chunks("b"), nil)
}

func TestAnEarlyChunkWaitsForTheOneBeforeIt(t *testing.T) {
	s := NewChunkSequencer(8)
	r, g := s.Push(1, []byte("b"))
	check(t, r, g, nil, nil)
	r, g = s.Push(0, []byte("a"))
	check(t, r, g, chunks("a", "b"), nil)
}

func TestDuplicatesAndStaleChunksAreDropped(t *testing.T) {
	s := NewChunkSequencer(8)
	s.Push(0, []byte("a"))
	r, g := s.Push(0, []byte("a"))
	check(t, r, g, nil, nil)
	s.Push(2, []byte("c"))
	r, g = s.Push(2, []byte("c"))
	check(t, r, g, nil, nil)
}

func TestAMissingChunkBecomesAGapOnceTheWindowFills(t *testing.T) {
	s := NewChunkSequencer(2)
	s.Push(0, []byte("a"))
	r, g := s.Push(2, []byte("c"))
	check(t, r, g, nil, nil)
	r, g = s.Push(3, []byte("d"))
	check(t, r, g, nil, nil)
	r, g = s.Push(4, []byte("e"))
	check(t, r, g, chunks("c", "d", "e"), []Gap{{1, 2}})
}

func TestFlushReleasesWhatIsHeldAndNamesTheGaps(t *testing.T) {
	s := NewChunkSequencer(8)
	s.Push(0, []byte("a"))
	s.Push(3, []byte("d"))
	r, g := s.Flush()
	check(t, r, g, chunks("d"), []Gap{{1, 3}})
}

func TestTheIntakeFeedsInOrderThenEnds(t *testing.T) {
	var seen []string
	ended := make(chan struct{})
	m := &MediaIntake{
		Sink:  func(_ context.Context, c []byte) error { seen = append(seen, string(c)); return nil },
		OnEnd: func(context.Context) error { close(ended); return nil },
	}
	m.Start(context.Background())
	for _, c := range []string{"1", "2", "3"} {
		m.Feed([]byte(c))
	}
	m.End()
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("never ended")
	}
	if !reflect.DeepEqual(seen, []string{"1", "2", "3"}) {
		t.Fatalf("seen %v", seen)
	}
}

func TestAFullIntakeDropsTheOldest(t *testing.T) {
	var seen []string
	holding := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	m := &MediaIntake{
		MaxChunks: 2,
		Sink: func(_ context.Context, c []byte) error {
			once.Do(func() { close(holding) })
			<-release
			seen = append(seen, string(c))
			return nil
		},
	}
	m.Start(context.Background())
	m.Feed([]byte("1"))
	<-holding // the sink now holds 1
	for _, c := range []string{"2", "3", "4"} {
		m.Feed([]byte(c))
	}
	m.End()
	close(release)
	m.Wait()
	if m.Dropped() != 2 || !reflect.DeepEqual(seen, []string{"1", "4"}) {
		t.Fatalf("dropped %d, seen %v", m.Dropped(), seen)
	}
}

func TestQuietTimeSendsKeepalivesThenGoesIdle(t *testing.T) {
	var mu sync.Mutex
	var quiet []Quiet
	m := &MediaIntake{
		Sink:           func(context.Context, []byte) error { return nil },
		OnQuiet:        func(_ context.Context, q Quiet) error { mu.Lock(); quiet = append(quiet, q); mu.Unlock(); return nil },
		KeepaliveAfter: 20 * time.Millisecond,
		IdleAfter:      70 * time.Millisecond,
	}
	m.Start(context.Background())
	m.Wait()
	keepalives := 0
	for _, q := range quiet {
		if q == Keepalive {
			keepalives++
		}
	}
	if quiet[len(quiet)-1] != Idle || keepalives < 2 {
		t.Fatalf("quiet %v", quiet)
	}
}

func TestStopDropsWhatIsQueued(t *testing.T) {
	release := make(chan struct{})
	ended := false
	m := &MediaIntake{
		Sink:  func(ctx context.Context, _ []byte) error { <-release; return nil },
		OnEnd: func(context.Context) error { ended = true; return nil },
	}
	m.Start(context.Background())
	m.Feed([]byte("1"))
	m.Feed([]byte("2"))
	go func() { time.Sleep(10 * time.Millisecond); close(release) }()
	m.Stop()
	if m.Running() || ended {
		t.Fatalf("running %v, ended %v", m.Running(), ended)
	}
}
