// provides an in-process event bus for pipeline phase transition notifications.

package events

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Event represents a phase transition notification emitted by the pipeline.
type Event struct {
	Event     string `json:"event"`     // "phase-start" | "phase-complete" | "phase-fail" | "checkpoint" | "abandon"
	Phase     string `json:"phase"`     // e.g. "phase-3"
	SpecName  string `json:"specName"`  // from state.SpecName
	Workspace string `json:"workspace"` // absolute path passed to the tool
	Timestamp string `json:"timestamp"` // RFC3339 UTC
	Outcome   string `json:"outcome"`   // "in_progress" | "completed" | "failed" | "awaiting_human" | "abandoned"
}

const subscriberBufferSize = 64

// EventBus manages a set of subscriber channels and broadcasts published events to all of them.
// Concurrent Publish calls are safe and do not serialize against each other.
// Subscribe and Unsubscribe serialize against all other operations.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan Event
	nextID      atomic.Uint64
}

// NewEventBus constructs a new, empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string]chan Event),
	}
}

// Subscribe registers a new subscriber and returns its unique ID and a read-only channel.
// The channel has a buffer of 64 events. Events are dropped (not delivered) when the buffer is full.
func (b *EventBus) Subscribe() (id string, ch <-chan Event) {
	rawID := b.nextID.Add(1)
	id = fmt.Sprintf("sub-%d", rawID)
	c := make(chan Event, subscriberBufferSize)

	b.mu.Lock()
	b.subscribers[id] = c
	b.mu.Unlock()

	return id, c
}

// Unsubscribe removes the subscriber with the given ID and closes its channel.
// Calling Unsubscribe with an unknown ID is a no-op.
func (b *EventBus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	c, ok := b.subscribers[id]
	if !ok {
		return
	}
	delete(b.subscribers, id)
	close(c)
}

// Publish broadcasts e to all current subscribers using non-blocking sends.
// Events are dropped for any subscriber whose channel buffer is full.
// Publish acquires only a read lock so concurrent Publish calls do not serialize.
func (b *EventBus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- e:
		default:
			// subscriber is slow; drop the event rather than blocking
		}
	}
}
