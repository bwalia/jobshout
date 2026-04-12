package integration

import (
	"context"
	"sync"
)

const defaultBufferSize = 256

// Bus is an in-process event bus backed by Go channels.
// Each EventType has its own broadcast list of subscriber channels.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]chan TaskEvent
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[EventType][]chan TaskEvent),
	}
}

// Subscribe returns a channel that receives events of the given type.
// The caller is responsible for draining the channel to avoid blocking publishers.
func (b *Bus) Subscribe(topic EventType) <-chan TaskEvent {
	ch := make(chan TaskEvent, defaultBufferSize)
	b.mu.Lock()
	b.subscribers[topic] = append(b.subscribers[topic], ch)
	b.mu.Unlock()
	return ch
}

// Publish sends an event to all subscribers of the event's type.
// Non-blocking: if a subscriber's channel is full, the event is dropped for that subscriber.
func (b *Bus) Publish(_ context.Context, event TaskEvent) {
	b.mu.RLock()
	subs := b.subscribers[event.Type]
	b.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// subscriber is slow — drop rather than block the publisher
		}
	}
}
