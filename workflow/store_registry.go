// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// StatusUpdater updates a resource's status fields by org and name.
// This avoids importing resource.Store (which would create import cycles).
type StatusUpdater interface {
	UpdateStatus(ctx context.Context, orgID uuid.UUID, name string, fields map[string]interface{}) error
}

// StoreRegistry maps resource types to their StatusUpdater. Providers
// register their updater during Init; the polling callback looks up
// the right one by resource type.
type StoreRegistry struct {
	mu     sync.RWMutex
	stores map[string]StatusUpdater
}

// NewStoreRegistry creates an empty store registry.
func NewStoreRegistry() *StoreRegistry {
	return &StoreRegistry{stores: make(map[string]StatusUpdater)}
}

// Register associates a resource type with its status updater.
func (r *StoreRegistry) Register(resourceType string, updater StatusUpdater) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stores[resourceType] = updater
}

// Lookup returns the updater for the given resource type, or nil.
func (r *StoreRegistry) Lookup(resourceType string) StatusUpdater {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stores[resourceType]
}

// MakeStatusCallback creates a StatusCallback that uses the registry
// to update resource status when workflows complete or fail.
func MakeStatusCallback(registry *StoreRegistry) StatusCallback {
	return func(ctx context.Context, tracked TrackedRun, run *Run) error {
		updater := registry.Lookup(tracked.ResourceType)
		if updater == nil {
			return fmt.Errorf("no store registered for resource type %q", tracked.ResourceType)
		}

		phase := "Ready"
		message := ""
		if run.Status == RunFailed {
			phase = "Failed"
			message = run.Error
		}

		return updater.UpdateStatus(ctx, tracked.OrgID, tracked.Name, map[string]interface{}{
			"phase":   phase,
			"message": message,
		})
	}
}
