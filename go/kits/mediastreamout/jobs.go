// Package mediastreamout streams media out: serial producers and replay.
package mediastreamout

import (
	"context"
	"log/slog"
	"sync"
)

type Job func(ctx context.Context) error

// SerialJobs runs jobs one at a time per key, in order.
type SerialJobs struct {
	mu    sync.Mutex
	lanes map[string]*lane
}

type lane struct {
	queue  []Job
	cancel context.CancelFunc
	done   chan struct{}
}

func (s *SerialJobs) Submit(key string, job Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lanes == nil {
		s.lanes = map[string]*lane{}
	}
	l := s.lanes[key]
	if l == nil {
		l = &lane{done: make(chan struct{})}
		s.lanes[key] = l
		go s.work(key, l)
	}
	l.queue = append(l.queue, job)
}

func (s *SerialJobs) Busy(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lanes[key] != nil
}

// Cancel stops the running job and drops the queued ones; not from inside a job.
func (s *SerialJobs) Cancel(key string) bool {
	s.mu.Lock()
	l := s.lanes[key]
	if l == nil {
		s.mu.Unlock()
		return false
	}
	delete(s.lanes, key)
	l.queue = nil
	if l.cancel != nil {
		l.cancel()
	}
	s.mu.Unlock()
	<-l.done
	return true
}

// Wait blocks until key's jobs have run. For tests.
func (s *SerialJobs) Wait(key string) {
	s.mu.Lock()
	l := s.lanes[key]
	s.mu.Unlock()
	if l != nil {
		<-l.done
	}
}

func (s *SerialJobs) work(key string, l *lane) {
	defer close(l.done)
	for {
		s.mu.Lock()
		if len(l.queue) == 0 || s.lanes[key] != l {
			if s.lanes[key] == l {
				delete(s.lanes, key)
			}
			s.mu.Unlock()
			return
		}
		job := l.queue[0]
		l.queue = l.queue[1:]
		ctx, cancel := context.WithCancel(context.Background())
		l.cancel = cancel
		s.mu.Unlock()

		err := job(ctx)
		cancelled := ctx.Err() != nil
		cancel()
		if err != nil && !cancelled {
			slog.Error("media job failed", "key", key, "error", err)
		}
		if cancelled {
			return
		}
	}
}
