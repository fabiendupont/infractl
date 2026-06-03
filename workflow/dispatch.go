// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"fmt"
	"sort"
	"sync"
)

// DispatchTable maps (resource_type, event) pairs to ordered handlers.
// Thread-safe for concurrent registration and lookup.
type DispatchTable struct {
	mu      sync.RWMutex
	actions map[string][]Handler
}

// NewDispatchTable creates an empty dispatch table.
func NewDispatchTable() *DispatchTable {
	return &DispatchTable{
		actions: make(map[string][]Handler),
	}
}

func dispatchKey(resourceType, event string) string {
	return fmt.Sprintf("%s.%s", resourceType, event)
}

// Register adds a handler to the dispatch table.
func (t *DispatchTable) Register(h Handler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := dispatchKey(h.ResourceType, h.Event)
	t.actions[key] = append(t.actions[key], h)
}

// Lookup returns all handlers for the given resource type and event,
// sorted by phase (pre → main → post) then by priority within each phase.
func (t *DispatchTable) Lookup(resourceType, event string) []Handler {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key := dispatchKey(resourceType, event)
	handlers := make([]Handler, len(t.actions[key]))
	copy(handlers, t.actions[key])

	sort.Slice(handlers, func(i, j int) bool {
		pi, pj := phaseOrder(handlers[i].Phase), phaseOrder(handlers[j].Phase)
		if pi != pj {
			return pi < pj
		}
		return handlers[i].Priority < handlers[j].Priority
	})

	return handlers
}

// ResourceTypes returns all distinct resource types that have registered handlers.
func (t *DispatchTable) ResourceTypes() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	seen := make(map[string]bool)
	for key := range t.actions {
		for _, h := range t.actions[key] {
			seen[h.ResourceType] = true
		}
	}

	types := make([]string, 0, len(seen))
	for rt := range seen {
		types = append(types, rt)
	}
	sort.Strings(types)
	return types
}
