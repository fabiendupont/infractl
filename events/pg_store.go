// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PgStore is a PostgreSQL-backed event store for audit trail and querying.
// It shares the EventRecord model with PgBus.
type PgStore struct {
	db *gorm.DB
}

var _ Store = (*PgStore)(nil)

// NewPgStore creates a PostgreSQL-backed event store.
func NewPgStore(db *gorm.DB) *PgStore {
	return &PgStore{db: db}
}

// Save persists an event record.
func (s *PgStore) Save(ctx context.Context, event Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	record := EventRecord{
		ID:        event.ID,
		OrgID:     event.OrgID,
		Kind:      event.Kind,
		Name:      event.Name,
		Action:    event.Action,
		Payload:   event.Payload,
		CreatedAt: event.Timestamp,
	}
	return s.db.WithContext(ctx).Create(&record).Error
}

// List returns events matching the given org and filter options.
func (s *PgStore) List(ctx context.Context, orgID uuid.UUID, opts ListOptions) ([]Event, error) {
	query := s.db.WithContext(ctx).Where("org_id = ?", orgID)

	if opts.Kind != "" {
		query = query.Where("kind = ?", opts.Kind)
	}
	if opts.Action != "" {
		query = query.Where("action = ?", opts.Action)
	}

	query = query.Order("created_at DESC")

	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	} else {
		query = query.Limit(100)
	}

	var records []EventRecord
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}

	events := make([]Event, len(records))
	for i, r := range records {
		events[i] = Event{
			ID:        r.ID,
			OrgID:     r.OrgID,
			Kind:      r.Kind,
			Name:      r.Name,
			Action:    r.Action,
			Payload:   r.Payload,
			Timestamp: r.CreatedAt,
		}
	}
	return events, nil
}
