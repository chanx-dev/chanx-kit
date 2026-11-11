package mediastreamin

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Quiet is why an intake heard nothing for a while.
type Quiet string

const (
	// Keepalive asks the consumer to keep its provider connection open.
	Keepalive Quiet = "keepalive"
	// Idle means nothing arrived for too long; the intake stops after it.
	Idle Quiet = "idle"
)

// MediaIntake feeds chunks to Sink in order, dropping the oldest when full.
type MediaIntake struct {
	Sink      func(context.Context, []byte) error
	OnEnd     func(context.Context) error
	OnQuiet   func(context.Context, Quiet) error
	MaxChunks int
	// Zero turns it off.
	KeepaliveAfter time.Duration
	IdleAfter      time.Duration

	mu      sync.Mutex
	queue   [][]byte
	ended   bool
	dropped int
	wake    chan struct{}
	cancel  context.CancelFunc
	done    chan struct{}
}

func (m *MediaIntake) Start(ctx context.Context) {
	if m.MaxChunks <= 0 {
		m.MaxChunks = 50
	}
	ctx, m.cancel = context.WithCancel(ctx)
	m.wake = make(chan struct{}, 1)
	m.done = make(chan struct{})
	go m.run(ctx)
}

func (m *MediaIntake) Running() bool {
	if m.done == nil {
		return false
	}
	select {
	case <-m.done:
		return false
	default:
		return true
	}
}

func (m *MediaIntake) Dropped() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dropped
}

func (m *MediaIntake) Feed(chunk []byte) {
	m.mu.Lock()
	if len(m.queue) >= m.MaxChunks {
		m.queue = m.queue[1:]
		m.dropped++
		slog.Warn("media intake full; dropped the oldest chunk", "dropped", m.dropped)
	}
	m.queue = append(m.queue, chunk)
	m.mu.Unlock()
	m.signal()
}

// End calls OnEnd after the queued chunks.
func (m *MediaIntake) End() {
	m.mu.Lock()
	// The end takes a slot, as a chunk would.
	if len(m.queue) >= m.MaxChunks {
		m.queue = m.queue[1:]
		m.dropped++
	}
	m.ended = true
	m.mu.Unlock()
	m.signal()
}

// Stop drops the queue and waits for the intake; not from inside a callback.
func (m *MediaIntake) Stop() {
	if m.cancel == nil {
		return
	}
	m.cancel()
	<-m.done
}

func (m *MediaIntake) Wait() {
	if m.done != nil {
		<-m.done
	}
}

func (m *MediaIntake) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *MediaIntake) next() (chunk []byte, have, ended bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.queue) > 0 {
		chunk, m.queue = m.queue[0], m.queue[1:]
		return chunk, true, false
	}
	return nil, false, m.ended
}

func (m *MediaIntake) run(ctx context.Context) {
	defer close(m.done)
	last := time.Now()
	for ctx.Err() == nil {
		chunk, have, ended := m.next()
		if have {
			last = time.Now()
			if m.Sink(ctx, chunk) != nil {
				return
			}
			continue
		}
		if ended {
			if m.OnEnd != nil {
				_ = m.OnEnd(ctx)
			}
			return
		}

		wait := m.KeepaliveAfter
		if m.IdleAfter > 0 {
			if remaining := m.IdleAfter - time.Since(last); wait <= 0 || remaining < wait {
				wait = max(remaining, 0)
			}
		}
		var timeout <-chan time.Time
		var timer *time.Timer
		if wait > 0 || m.IdleAfter > 0 {
			timer = time.NewTimer(wait)
			timeout = timer.C
		}
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
			if timer != nil {
				timer.Stop()
			}
		case <-timeout:
			if m.IdleAfter > 0 && time.Since(last) >= m.IdleAfter {
				if m.OnQuiet != nil {
					_ = m.OnQuiet(ctx, Idle)
				}
				return
			}
			if m.OnQuiet != nil {
				_ = m.OnQuiet(ctx, Keepalive)
			}
		}
	}
}
