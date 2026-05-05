package event

import (
	"sync"
)

type Hub struct {
	mu     sync.RWMutex
	feeds  map[string]*sessionFeed
	buffer int
}

type sessionFeed struct {
	mu     sync.RWMutex
	recent []Event
	subs   map[chan Event]struct{}
	buffer int
	closed bool
}

type Subscription struct {
	Recent []Event
	C      <-chan Event

	close func()
}

func NewHub(buffer int) *Hub {
	if buffer <= 0 {
		buffer = 32
	}
	return &Hub{
		feeds:  make(map[string]*sessionFeed),
		buffer: buffer,
	}
}

func (h *Hub) Publish(sessionID string, evt Event) {
	if sessionID == "" {
		return
	}

	feed := h.feed(sessionID)
	feed.publish(evt)
}

func (h *Hub) Snapshot(sessionID string) []Event {
	feed := h.feed(sessionID)
	return feed.snapshot()
}

func (h *Hub) Subscribe(sessionID string) *Subscription {
	feed := h.feed(sessionID)
	return feed.subscribe()
}

func (h *Hub) Close(sessionID string) {
	h.mu.Lock()
	feed := h.feeds[sessionID]
	delete(h.feeds, sessionID)
	h.mu.Unlock()

	if feed != nil {
		feed.close()
	}
}

func (h *Hub) feed(sessionID string) *sessionFeed {
	h.mu.RLock()
	feed := h.feeds[sessionID]
	h.mu.RUnlock()
	if feed != nil {
		return feed
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if feed = h.feeds[sessionID]; feed != nil {
		return feed
	}

	feed = &sessionFeed{
		subs:   make(map[chan Event]struct{}),
		buffer: h.buffer,
	}
	h.feeds[sessionID] = feed
	return feed
}

func (f *sessionFeed) publish(evt Event) {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	if f.buffer > 0 {
		f.recent = append(f.recent, evt)
		if len(f.recent) > f.buffer {
			drop := len(f.recent) - f.buffer
			copy(f.recent, f.recent[drop:])
			f.recent = f.recent[:f.buffer]
		}
	}
	subs := make([]chan Event, 0, len(f.subs))
	for ch := range f.subs {
		subs = append(subs, ch)
	}
	f.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (f *sessionFeed) snapshot() []Event {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make([]Event, len(f.recent))
	copy(out, f.recent)
	return out
}

func (f *sessionFeed) subscribe() *Subscription {
	ch := make(chan Event, 64)

	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		close(ch)
		return &Subscription{Recent: nil, C: ch, close: func() {}}
	}

	recent := make([]Event, len(f.recent))
	copy(recent, f.recent)
	f.subs[ch] = struct{}{}
	f.mu.Unlock()

	return &Subscription{
		Recent: recent,
		C:      ch,
		close: func() {
			f.mu.Lock()
			if _, ok := f.subs[ch]; ok {
				delete(f.subs, ch)
				close(ch)
			}
			f.mu.Unlock()
		},
	}
}

func (f *sessionFeed) close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	subs := make([]chan Event, 0, len(f.subs))
	for ch := range f.subs {
		subs = append(subs, ch)
	}
	f.subs = nil
	f.mu.Unlock()

	for _, ch := range subs {
		close(ch)
	}
}

func (s *Subscription) Close() {
	if s != nil && s.close != nil {
		s.close()
	}
}
