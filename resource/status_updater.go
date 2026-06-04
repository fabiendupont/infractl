// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StatusUpdater updates the status JSONB column of a resource by org
// and name. Used by the workflow polling callback to set phase/message
// when workflows complete.
type StatusUpdater struct {
	db    *gorm.DB
	table string
}

// NewStatusUpdater creates a status updater for the given table.
func NewStatusUpdater(db *gorm.DB, table string) *StatusUpdater {
	return &StatusUpdater{db: db, table: table}
}

// UpdateStatus sets the status fields on a resource. The fields map
// is serialized to JSON and written to the status JSONB column.
func (u *StatusUpdater) UpdateStatus(ctx context.Context, orgID uuid.UUID, name string, fields map[string]interface{}) error {
	status, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("marshaling status: %w", err)
	}

	result := u.db.WithContext(ctx).
		Table(u.table).
		Where("org_id = ? AND name = ? AND deleted_at IS NULL", orgID, name).
		Updates(map[string]interface{}{
			"status":     string(status),
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("updating status in %s: %w", u.table, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
