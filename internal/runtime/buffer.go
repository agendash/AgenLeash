package runtime

import (
	"sync"
	"time"
)

type EventBuffer struct {
	mu       sync.RWMutex
	capacity int
	seq      uint64
	events   []Event
}

func NewEventBuffer(capacity int) *EventBuffer {
	if capacity <= 0 {
		capacity = 128
	}
	return &EventBuffer{
		capacity: capacity,
		events:   make([]Event, 0, capacity),
	}
}

func (b *EventBuffer) Append(event Event) Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.seq++
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	event.Sequence = b.seq

	if len(b.events) == b.capacity {
		copy(b.events, b.events[1:])
		b.events[len(b.events)-1] = event
		return event
	}

	b.events = append(b.events, event)
	return event
}

func (b *EventBuffer) LatestSequence() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.seq
}

func (b *EventBuffer) Snapshot() []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]Event, len(b.events))
	copy(out, b.events)
	return out
}

func (b *EventBuffer) Since(sequence uint64) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.events) == 0 {
		return nil
	}

	out := make([]Event, 0, len(b.events))
	for _, event := range b.events {
		if event.Sequence > sequence {
			out = append(out, event)
		}
	}
	return out
}
