// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Event represents a resource lifecycle event.
type Event struct {
	ID        string    `json:"id"`
	OrgID     uuid.UUID `json:"org_id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
	Payload   []byte    `json:"payload,omitempty"`
}

// Bus publishes and subscribes to resource lifecycle events.
type Bus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(ctx context.Context, kinds []string) (<-chan Event, error)
}

// InMemoryBus is a development-only event bus backed by Go channels.
// For production use PostgreSQL NOTIFY/LISTEN.
type InMemoryBus struct {
	mu          sync.RWMutex
	subscribers []subscriber
}

type subscriber struct {
	kinds map[string]bool
	ch    chan Event
}

// NewInMemoryBus creates an in-memory event bus.
func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{}
}

// Publish sends an event to all matching subscribers.
func (b *InMemoryBus) Publish(_ context.Context, event Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subscribers {
		if len(sub.kinds) == 0 || sub.kinds[event.Kind] {
			select {
			case sub.ch <- event:
			default:
			}
		}
	}
	return nil
}

// Subscribe returns a channel that receives events matching the given kinds.
// Pass an empty slice to receive all events.
func (b *InMemoryBus) Subscribe(_ context.Context, kinds []string) (<-chan Event, error) {
	ch := make(chan Event, 100)
	kindSet := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		kindSet[k] = true
	}

	b.mu.Lock()
	b.subscribers = append(b.subscribers, subscriber{kinds: kindSet, ch: ch})
	b.mu.Unlock()

	return ch, nil
}
