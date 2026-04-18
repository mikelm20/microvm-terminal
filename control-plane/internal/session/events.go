package session

import "sync"

// EventBus is a simple in-memory fan-out: publish an event, every currently-
// subscribed channel receives a copy. Slow subscribers drop messages, they
// don't back-pressure the publisher. One bus per Session.
type EventBus struct {
	mu          sync.Mutex
	subscribers map[chan GuestEvent]struct{}

	// history lets a late subscriber get the events fired so far (bounded).
	history []GuestEvent
}

const historyCap = 256

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[chan GuestEvent]struct{}),
	}
}

func (b *EventBus) Publish(e GuestEvent) {
	b.mu.Lock()
	b.history = append(b.history, e)
	if len(b.history) > historyCap {
		b.history = b.history[len(b.history)-historyCap:]
	}
	subs := make([]chan GuestEvent, 0, len(b.subscribers))
	for c := range b.subscribers {
		subs = append(subs, c)
	}
	b.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- e:
		default: // drop for slow subscriber
		}
	}
}

// Subscribe returns a history snapshot and a channel of future events. Call
// the returned cleanup to unsubscribe.
func (b *EventBus) Subscribe(buffer int) ([]GuestEvent, <-chan GuestEvent, func()) {
	b.mu.Lock()
	snap := make([]GuestEvent, len(b.history))
	copy(snap, b.history)
	ch := make(chan GuestEvent, buffer)
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return snap, ch, func() {
		b.mu.Lock()
		delete(b.subscribers, ch)
		b.mu.Unlock()
		close(ch)
	}
}
