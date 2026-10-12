package me

import "sync"

// Bus is a tiny SSE fan-out broadcaster.
type Bus struct {
	mu   sync.Mutex
	subs map[chan []byte]bool
}

func NewBus() *Bus { return &Bus{subs: map[chan []byte]bool{}} }

func (b *Bus) subscribe() chan []byte {
	ch := make(chan []byte, 64)
	b.mu.Lock()
	b.subs[ch] = true
	b.mu.Unlock()
	return ch
}

func (b *Bus) unsubscribe(ch chan []byte) {
	b.mu.Lock()
	if b.subs[ch] {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *Bus) publish(msg []byte) {
	b.mu.Lock()
	for ch := range b.subs {
		select {
		case ch <- msg:
		default: // drop for slow clients
		}
	}
	b.mu.Unlock()
}

func (b *Bus) hasSubs() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs) > 0
}
