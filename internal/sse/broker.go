package sse

import (
	"sync"

	"github.com/google/uuid"
)

// Broker is a coalescing pub/sub hub for run events.
// Each subscriber gets a buffered channel of capacity 1; if a notification
// arrives while one is already pending, it is silently dropped — the
// subscriber will still process the latest state when it wakes up.
type Broker struct {
	mu   sync.RWMutex
	subs map[uuid.UUID][]chan struct{}
}

func New() *Broker {
	return &Broker{subs: make(map[uuid.UUID][]chan struct{})}
}

// Subscribe registers a channel for events on runID.
// The returned cancel function must be called (e.g. via defer) to unsubscribe.
func (b *Broker) Subscribe(runID uuid.UUID) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.subs[runID] = append(b.subs[runID], ch)
	b.mu.Unlock()

	return ch, func() {
		b.mu.Lock()
		subs := b.subs[runID]
		for i, c := range subs {
			if c == ch {
				b.subs[runID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(b.subs[runID]) == 0 {
			delete(b.subs, runID)
		}
		b.mu.Unlock()
	}
}

// Notify wakes up all subscribers for runID. If a subscriber already has a
// pending notification it is not duplicated. Safe on a nil Broker (used by
// handler tests that run without SSE).
func (b *Broker) Notify(runID uuid.UUID) {
	if b == nil {
		return
	}
	b.mu.RLock()
	subs := b.subs[runID]
	b.mu.RUnlock()
	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
