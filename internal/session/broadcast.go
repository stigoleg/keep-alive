package session

import "sync"

const subscriberBuffer = 64

// broadcaster fans events out to subscribers without ever blocking the
// publisher: a full subscriber loses its oldest event.
type broadcaster struct {
	mu     sync.Mutex
	subs   map[chan Event]struct{}
	closed bool
}

func (b *broadcaster) init() { b.subs = map[chan Event]struct{}{} }

func (b *broadcaster) subscribe() (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(ch)
		return ch, func() {}
	}
	b.subs[ch] = struct{}{}
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
}

func (b *broadcaster) publish(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
			continue
		default:
		}
		select { // drop the oldest
		case <-ch:
		default:
		}
		select {
		case ch <- ev:
		default:
		}
	}
}

func (b *broadcaster) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.subs {
		close(ch)
	}
	b.subs = nil
}
