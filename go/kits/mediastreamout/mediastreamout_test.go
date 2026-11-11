package mediastreamout

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	chanx "go.chanx.dev/v2"
)

type log struct {
	mu    sync.Mutex
	items []string
}

func (l *log) add(s string) { l.mu.Lock(); l.items = append(l.items, s); l.mu.Unlock() }
func (l *log) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.items...)
}

func TestJobsForAKeyRunInOrderOneAtATime(t *testing.T) {
	var jobs SerialJobs
	var seen log
	for _, name := range []string{"a", "b", "c"} {
		jobs.Submit("k", func(context.Context) error {
			seen.add(name + "-start")
			time.Sleep(5 * time.Millisecond)
			seen.add(name + "-end")
			return nil
		})
	}
	jobs.Wait("k")
	want := []string{"a-start", "a-end", "b-start", "b-end", "c-start", "c-end"}
	if !reflect.DeepEqual(seen.get(), want) || jobs.Busy("k") {
		t.Fatalf("seen %v, busy %v", seen.get(), jobs.Busy("k"))
	}
}

func TestKeysRunIndependently(t *testing.T) {
	var jobs SerialJobs
	release := make(chan struct{})
	jobs.Submit("slow", func(context.Context) error { <-release; return nil })
	done := make(chan struct{})
	jobs.Submit("fast", func(context.Context) error { close(done); return nil })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fast waited for slow")
	}
	close(release)
	jobs.Wait("slow")
}

func TestCancelStopsTheCurrentJobAndDropsTheQueue(t *testing.T) {
	var jobs SerialJobs
	var seen log
	started := make(chan struct{})
	jobs.Submit("k", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		seen.add("stopped")
		return ctx.Err()
	})
	jobs.Submit("k", func(context.Context) error { seen.add("queued ran"); return nil })
	<-started
	if !jobs.Cancel("k") {
		t.Fatal("cancel stopped nothing")
	}
	time.Sleep(10 * time.Millisecond)
	if !reflect.DeepEqual(seen.get(), []string{"stopped"}) || jobs.Busy("k") {
		t.Fatalf("seen %v", seen.get())
	}
	if jobs.Cancel("k") {
		t.Fatal("a second cancel stopped something")
	}
}

func TestAFailingJobDoesNotStopTheNext(t *testing.T) {
	var jobs SerialJobs
	var seen log
	jobs.Submit("k", func(context.Context) error { return errors.New("boom") })
	jobs.Submit("k", func(context.Context) error { seen.add("next"); return nil })
	jobs.Wait("k")
	if !reflect.DeepEqual(seen.get(), []string{"next"}) {
		t.Fatalf("seen %v", seen.get())
	}
}

func TestReplayNumbersFromOneAndRestartsAfterClear(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryReplayStore()
	for range 2 {
		_, _ = store.Append(ctx, "k", chanx.PongMessage{})
	}
	entries, _ := store.Replay(ctx, "k")
	if len(entries) != 2 || entries[0].Seq != 1 || entries[1].Seq != 2 {
		t.Fatalf("entries %v", entries)
	}
	_ = store.Clear(ctx, "k")
	if seq, _ := store.Append(ctx, "k", chanx.PongMessage{}); seq != 1 {
		t.Fatalf("seq after clear %d", seq)
	}
}

func TestReplayIsBounded(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryReplayStore()
	store.MaxMessages = 2
	for range 3 {
		_, _ = store.Append(ctx, "k", chanx.PongMessage{})
	}
	entries, _ := store.Replay(ctx, "k")
	if len(entries) != 2 || entries[0].Seq != 2 || entries[1].Seq != 3 {
		t.Fatalf("entries %v", entries)
	}
}
