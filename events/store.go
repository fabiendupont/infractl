// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// ListOptions controls event query filtering and pagination.
type ListOptions struct {
	Limit  int
	Offset int
	Kind   string
	Action string
}

// Store persists events for audit trail and querying.
type Store interface {
	Save(ctx context.Context, event Event) error
	List(ctx context.Context, orgID uuid.UUID, opts ListOptions) ([]Event, error)
}

// InMemoryStore is a development-only event store.
type InMemoryStore struct {
	mu     sync.RWMutex
	events []Event
}

// NewInMemoryStore creates an in-memory event store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{}
}

// Save appends an event to the store.
func (s *InMemoryStore) Save(_ context.Context, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

// List returns events matching the given org and filter options.
func (s *InMemoryStore) List(_ context.Context, orgID uuid.UUID, opts ListOptions) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Event
	for _, e := range s.events {
		if e.OrgID != orgID {
			continue
		}
		if opts.Kind != "" && e.Kind != opts.Kind {
			continue
		}
		if opts.Action != "" && e.Action != opts.Action {
			continue
		}
		result = append(result, e)
	}

	if opts.Offset > 0 && opts.Offset < len(result) {
		result = result[opts.Offset:]
	}
	if opts.Limit > 0 && opts.Limit < len(result) {
		result = result[:opts.Limit]
	}

	return result, nil
}
