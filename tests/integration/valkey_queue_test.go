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

	"github.com/fabiendupont/infractl/tests/testutil"
	"github.com/fabiendupont/infractl/work"
)

func TestValkeyQueueEnqueueAndClaim(t *testing.T) {
	client, cleanup := testutil.SetupValkey(t)
	t.Cleanup(cleanup)

	q := work.NewValkeyQueue(client)
	ctx := context.Background()

	task := work.Task{
		ID:   "vtask-001",
		Kind: "provision",
		Name: "vm-1",
	}
	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	claimed, err := q.Claim(ctx, []string{"provision"})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed.ID != "vtask-001" {
		t.Errorf("claimed ID = %q, want %q", claimed.ID, "vtask-001")
	}
	if claimed.Kind != "provision" {
		t.Errorf("claimed Kind = %q, want %q", claimed.Kind, "provision")
	}
	if claimed.Status != work.TaskRunning {
		t.Errorf("claimed Status = %q, want %q", claimed.Status, work.TaskRunning)
	}
}

func TestValkeyQueueClaimNoTasks(t *testing.T) {
	client, cleanup := testutil.SetupValkey(t)
	t.Cleanup(cleanup)

	q := work.NewValkeyQueue(client)
	ctx := context.Background()

	_, err := q.Claim(ctx, []string{"nonexistent"})
	if !errors.Is(err, work.ErrNoTasks) {
		t.Fatalf("expected ErrNoTasks, got %v", err)
	}
}

func TestValkeyQueueCompleteSetsStatus(t *testing.T) {
	client, cleanup := testutil.SetupValkey(t)
	t.Cleanup(cleanup)

	q := work.NewValkeyQueue(client)
	ctx := context.Background()

	task := work.Task{
		ID:   "vtask-complete",
		Kind: "provision",
		Name: "vm-complete",
	}
	if err := q.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := q.Claim(ctx, []string{"provision"}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if err := q.Complete(ctx, "vtask-complete"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// After complete, claiming again should find nothing.
	_, err := q.Claim(ctx, []string{"provision"})
	if !errors.Is(err, work.ErrNoTasks) {
		t.Fatalf("expected ErrNoTasks after complete, got %v", err)
	}
}

func TestValkeyQueueConcurrentClaim(t *testing.T) {
	client, cleanup := testutil.SetupValkey(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	// Each goroutine needs its own queue instance with a unique consumer ID.
	for i := 0; i < 5; i++ {
		q := work.NewValkeyQueue(client)
		task := work.Task{
			ID:   uuid.New().String(),
			Kind: "concurrent",
			Name: "concurrent-task",
		}
		if err := q.Enqueue(ctx, task); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	var (
		wg      sync.WaitGroup
		claimed atomic.Int32
		noTasks atomic.Int32
	)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q := work.NewValkeyQueue(client)
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
}
