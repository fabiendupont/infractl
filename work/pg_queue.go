// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package work

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TaskRecord is the GORM model for the tasks table.
type TaskRecord struct {
	ID          string     `gorm:"primaryKey"`
	Kind        string     `gorm:"index"`
	OrgID       uuid.UUID  `gorm:"type:uuid"`
	Name        string
	Payload     []byte
	Status      string     `gorm:"index"`
	Error       string
	Attempts    int
	MaxAttempts int
	CreatedAt   time.Time
	ClaimedAt   *time.Time
	CompletedAt *time.Time
	RetryAfter  *time.Time `gorm:"index"`
}

func (TaskRecord) TableName() string { return "tasks" }

// PgQueue is a PostgreSQL-backed task queue using SELECT ... FOR UPDATE
// SKIP LOCKED for concurrent-safe claiming.
type PgQueue struct {
	db     *gorm.DB
	logger zerolog.Logger
}

var _ Queue = (*PgQueue)(nil)

// NewPgQueue creates a PostgreSQL-backed task queue. It auto-migrates the
// tasks table on construction.
func NewPgQueue(db *gorm.DB, logger zerolog.Logger) (*PgQueue, error) {
	if err := db.AutoMigrate(&TaskRecord{}); err != nil {
		return nil, err
	}
	return &PgQueue{
		db:     db,
		logger: logger.With().Str("component", "pg_queue").Logger(),
	}, nil
}

// Enqueue adds a task to the queue with status pending.
func (q *PgQueue) Enqueue(ctx context.Context, task Task) error {
	if task.ID == "" {
		task.ID = uuid.New().String()
	}

	record := TaskRecord{
		ID:          task.ID,
		Kind:        task.Kind,
		OrgID:       task.OrgID,
		Name:        task.Name,
		Payload:     task.Payload,
		Status:      string(TaskPending),
		Attempts:    0,
		MaxAttempts: 3,
		CreatedAt:   time.Now(),
	}
	return q.db.WithContext(ctx).Create(&record).Error
}

// Claim finds the first pending task matching the given kinds using
// FOR UPDATE SKIP LOCKED, marks it as running, and returns it.
func (q *PgQueue) Claim(ctx context.Context, kinds []string) (*Task, error) {
	var record TaskRecord

	err := q.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("status = ?", string(TaskPending)).
			Where("retry_after IS NULL OR retry_after <= ?", time.Now())

		if len(kinds) > 0 {
			query = query.Where("kind IN ?", kinds)
		}

		result := query.Order("created_at ASC").
			Limit(1).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			First(&record)

		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				return ErrNoTasks
			}
			return result.Error
		}

		now := time.Now()
		record.Status = string(TaskRunning)
		record.ClaimedAt = &now
		record.Attempts++

		return tx.Save(&record).Error
	})

	if err != nil {
		return nil, err
	}

	return &Task{
		ID:        record.ID,
		Kind:      record.Kind,
		OrgID:     record.OrgID,
		Name:      record.Name,
		Payload:   record.Payload,
		Status:    TaskStatus(record.Status),
		CreatedAt: record.CreatedAt,
	}, nil
}

// Complete marks a task as completed.
func (q *PgQueue) Complete(ctx context.Context, taskID string) error {
	now := time.Now()
	result := q.db.WithContext(ctx).Model(&TaskRecord{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":       string(TaskCompleted),
			"completed_at": &now,
		})
	if result.RowsAffected == 0 {
		return ErrNoTasks
	}
	return result.Error
}

// Fail marks a task as failed. If the task has remaining retry attempts,
// it is requeued with exponential backoff.
func (q *PgQueue) Fail(ctx context.Context, taskID string, reason string) error {
	var record TaskRecord
	if err := q.db.WithContext(ctx).First(&record, "id = ?", taskID).Error; err != nil {
		return err
	}

	if record.Attempts < record.MaxAttempts {
		backoff := time.Duration(math.Min(
			float64(time.Second*5*time.Duration(1<<record.Attempts)),
			float64(5*time.Minute),
		))
		retryAt := time.Now().Add(backoff)
		return q.db.WithContext(ctx).Model(&record).Updates(map[string]interface{}{
			"status":      string(TaskPending),
			"retry_after": &retryAt,
			"error":       reason,
		}).Error
	}

	return q.db.WithContext(ctx).Model(&record).Updates(map[string]interface{}{
		"status": string(TaskFailed),
		"error":  reason,
	}).Error
}

// StartRecovery periodically resets stale running tasks (claimed longer than
// staleTimeout ago) back to pending.
func (q *PgQueue) StartRecovery(ctx context.Context, staleTimeout, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-staleTimeout)
				result := q.db.WithContext(ctx).Model(&TaskRecord{}).
					Where("status = ? AND claimed_at < ?", string(TaskRunning), cutoff).
					Updates(map[string]interface{}{
						"status":     string(TaskPending),
						"claimed_at": nil,
					})
				if result.Error != nil {
					q.logger.Error().Err(result.Error).Msg("stale task recovery failed")
				} else if result.RowsAffected > 0 {
					q.logger.Info().Int64("recovered", result.RowsAffected).Msg("reset stale tasks to pending")
				}
			}
		}
	}()
}
