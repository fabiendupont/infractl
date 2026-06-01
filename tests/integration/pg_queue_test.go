// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/fabiendupont/infractl/tests/testutil"
	"github.com/fabiendupont/infractl/work"
)

func TestPgQueueEnqueueAndClaim(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	q, err := work.NewPgQueue(db, logger)
	if err != nil {
		t.Fatalf("NewPgQueue: %v", err)
	}

	ctx := context.Background()

	task := work.Task{
		ID:    "task-001",
		Kind:  "provision",
		OrgID: orgA,
		Name:  "vm-1",
	}
	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	claimed, err := q.Claim(ctx, []string{"provision"})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if claimed.ID != "task-001" {
		t.Errorf("claimed ID = %q, want %q", claimed.ID, "task-001")
	}
	if claimed.Kind != "provision" {
		t.Errorf("claimed Kind = %q, want %q", claimed.Kind, "provision")
	}
	if claimed.Name != "vm-1" {
		t.Errorf("claimed Name = %q, want %q", claimed.Name, "vm-1")
	}
	if claimed.Status != work.TaskRunning {
		t.Errorf("claimed Status = %q, want %q", claimed.Status, work.TaskRunning)
	}
}

func TestPgQueueClaimNoTasks(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	q, err := work.NewPgQueue(db, logger)
	if err != nil {
		t.Fatalf("NewPgQueue: %v", err)
	}

	ctx := context.Background()

	_, err = q.Claim(ctx, nil)
	if !errors.Is(err, work.ErrNoTasks) {
		t.Fatalf("expected ErrNoTasks, got %v", err)
	}
}

func TestPgQueueClaimRespectsKindFilter(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	q, err := work.NewPgQueue(db, logger)
	if err != nil {
		t.Fatalf("NewPgQueue: %v", err)
	}

	ctx := context.Background()

	task := work.Task{
		ID:    "task-kind-a",
		Kind:  "a",
		OrgID: orgA,
		Name:  "item-a",
	}
	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Claim with kind=["b"] should find nothing.
	_, err = q.Claim(ctx, []string{"b"})
	if !errors.Is(err, work.ErrNoTasks) {
		t.Fatalf("expected ErrNoTasks for kind b, got %v", err)
	}
}

func TestPgQueueCompleteSetsStatus(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	q, err := work.NewPgQueue(db, logger)
	if err != nil {
		t.Fatalf("NewPgQueue: %v", err)
	}

	ctx := context.Background()

	task := work.Task{
		ID:    "task-complete",
		Kind:  "provision",
		OrgID: orgA,
		Name:  "vm-complete",
	}
	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.Claim(ctx, nil); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := q.Complete(ctx, "task-complete"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	var record work.TaskRecord
	if err := db.First(&record, "id = ?", "task-complete").Error; err != nil {
		t.Fatalf("query task record: %v", err)
	}
	if record.Status != string(work.TaskCompleted) {
		t.Errorf("Status = %q, want %q", record.Status, work.TaskCompleted)
	}
}

func TestPgQueueFailWithRetry(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	q, err := work.NewPgQueue(db, logger)
	if err != nil {
		t.Fatalf("NewPgQueue: %v", err)
	}

	ctx := context.Background()

	task := work.Task{
		ID:    "task-retry",
		Kind:  "provision",
		OrgID: orgA,
		Name:  "vm-retry",
	}
	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.Claim(ctx, nil); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if err := q.Fail(ctx, "task-retry", "transient error"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	var record work.TaskRecord
	if err := db.First(&record, "id = ?", "task-retry").Error; err != nil {
		t.Fatalf("query task record: %v", err)
	}
	if record.Status != string(work.TaskPending) {
		t.Errorf("Status = %q, want %q (should be requeued)", record.Status, work.TaskPending)
	}
	if record.RetryAfter == nil {
		t.Error("RetryAfter is nil, expected a value")
	}
}

func TestPgQueueFailExhaustsMaxAttempts(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	q, err := work.NewPgQueue(db, logger)
	if err != nil {
		t.Fatalf("NewPgQueue: %v", err)
	}

	ctx := context.Background()

	task := work.Task{
		ID:    "task-exhaust",
		Kind:  "provision",
		OrgID: orgA,
		Name:  "vm-exhaust",
	}
	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Set max_attempts=1 directly in the DB so the first fail exhausts retries.
	if err := db.Model(&work.TaskRecord{}).Where("id = ?", "task-exhaust").
		Update("max_attempts", 1).Error; err != nil {
		t.Fatalf("update max_attempts: %v", err)
	}

	if _, err := q.Claim(ctx, nil); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if err := q.Fail(ctx, "task-exhaust", "permanent error"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	var record work.TaskRecord
	if err := db.First(&record, "id = ?", "task-exhaust").Error; err != nil {
		t.Fatalf("query task record: %v", err)
	}
	if record.Status != string(work.TaskFailed) {
		t.Errorf("Status = %q, want %q", record.Status, work.TaskFailed)
	}
}

func TestPgQueueConcurrentClaim(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	q, err := work.NewPgQueue(db, logger)
	if err != nil {
		t.Fatalf("NewPgQueue: %v", err)
	}

	ctx := context.Background()

	// Enqueue 5 tasks.
	for i := 0; i < 5; i++ {
		task := work.Task{
			ID:    uuid.New().String(),
			Kind:  "concurrent",
			OrgID: orgA,
			Name:  "concurrent-task",
		}
		if err := q.Enqueue(ctx, task); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	// Spawn 10 goroutines, each trying to claim one task.
	var (
		wg       sync.WaitGroup
		claimed  atomic.Int32
		noTasks  atomic.Int32
	)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine needs its own GORM session for transaction isolation.
			_, claimErr := q.Claim(ctx, []string{"concurrent"})
			if claimErr == nil {
				claimed.Add(1)
			} else if errors.Is(claimErr, work.ErrNoTasks) {
				noTasks.Add(1)
			} else {
				t.Errorf("unexpected Claim error: %v", claimErr)
			}
		}()
	}

	wg.Wait()

	if claimed.Load() != 5 {
		t.Errorf("claimed = %d, want 5", claimed.Load())
	}
	if noTasks.Load() != 5 {
		t.Errorf("noTasks = %d, want 5", noTasks.Load())
	}

	// Verify all 5 tasks are now in "running" status.
	var runningCount int64
	if err := db.Model(&work.TaskRecord{}).
		Where("kind = ? AND status = ?", "concurrent", string(work.TaskRunning)).
		Count(&runningCount).Error; err != nil {
		t.Fatalf("count running tasks: %v", err)
	}
	if runningCount != 5 {
		t.Errorf("running tasks in DB = %d, want 5", runningCount)
	}
}
