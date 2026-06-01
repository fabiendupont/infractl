// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"

	"github.com/google/uuid"
)

// LifecycleHandler emits standard CRUD lifecycle events to an event bus.
type LifecycleHandler struct {
	bus Bus
}

// NewLifecycleHandler creates a handler that publishes to the given bus.
func NewLifecycleHandler(bus Bus) *LifecycleHandler {
	return &LifecycleHandler{bus: bus}
}

// OnCreate emits a "created" event for the given resource.
func (h *LifecycleHandler) OnCreate(ctx context.Context, kind, name string, orgID uuid.UUID) {
	h.bus.Publish(ctx, Event{
		OrgID:  orgID,
		Kind:   kind,
		Name:   name,
		Action: "created",
	})
}

// OnUpdate emits an "updated" event for the given resource.
func (h *LifecycleHandler) OnUpdate(ctx context.Context, kind, name string, orgID uuid.UUID) {
	h.bus.Publish(ctx, Event{
		OrgID:  orgID,
		Kind:   kind,
		Name:   name,
		Action: "updated",
	})
}

// OnDelete emits a "deleted" event for the given resource.
func (h *LifecycleHandler) OnDelete(ctx context.Context, kind, name string, orgID uuid.UUID) {
	h.bus.Publish(ctx, Event{
		OrgID:  orgID,
		Kind:   kind,
		Name:   name,
		Action: "deleted",
	})
}
