// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ResourceRef declares a reference from one resource type to another.
// Providers register these during Init; the framework automatically
// validates them via pre_create sync hooks.
type ResourceRef struct {
	Source   string         // source resource type (e.g., "Subnet")
	Field   string         // field path (e.g., "spec.virtual_network")
	Target  string         // target resource type (e.g., "VirtualNetwork")
	Table   string         // target DB table name (e.g., "virtual_networks")
	Required bool          // if true, the field must be non-empty
	Extract RefExtractor   // extracts the referenced name from the payload
}

// RefExtractor returns the org_id and referenced name from a resource
// payload. If the reference is not set, return empty name.
type RefExtractor func(payload interface{}) (orgID uuid.UUID, refName string)

// refRegistry holds all declared resource references.
type refRegistry struct {
	mu   sync.RWMutex
	refs []ResourceRef
}

// RegisterRef declares a resource reference and automatically registers
// a pre_create sync hook that validates the referenced resource exists.
func (r *Registry) RegisterRef(ref ResourceRef, db *gorm.DB) {
	r.refs.mu.Lock()
	r.refs.refs = append(r.refs.refs, ref)
	r.refs.mu.Unlock()

	r.RegisterHook(SyncHook{
		Feature: ref.Source,
		Event:   "pre_create",
		Handler: makeRefValidator(ref, db),
	})
}

func makeRefValidator(ref ResourceRef, db *gorm.DB) func(ctx context.Context, payload interface{}) error {
	return func(ctx context.Context, payload interface{}) error {
		if ref.Extract == nil {
			return nil
		}

		orgID, refName := ref.Extract(payload)
		if refName == "" {
			if ref.Required {
				return fmt.Errorf("%s is required", ref.Field)
			}
			return nil
		}

		var count int64
		if err := db.WithContext(ctx).
			Table(ref.Table).
			Where("org_id = ? AND name = ? AND deleted_at IS NULL", orgID, refName).
			Count(&count).Error; err != nil {
			return fmt.Errorf("validating %s: %w", ref.Field, err)
		}
		if count == 0 {
			return fmt.Errorf("%s %q not found (referenced by %s)", ref.Target, refName, ref.Field)
		}
		return nil
	}
}
