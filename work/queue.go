// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package work

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TaskStatus represents the state of a queued task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
)

// Task represents a unit of async work.
type Task struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	OrgID     uuid.UUID  `json:"org_id"`
	Name      string     `json:"name"`
	Payload   []byte     `json:"payload,omitempty"`
	Status    TaskStatus `json:"status"`
	Error     string     `json:"error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Queue manages async task lifecycle: enqueue, claim, complete, fail.
type Queue interface {
	Enqueue(ctx context.Context, task Task) error
	Claim(ctx context.Context, kinds []string) (*Task, error)
	Complete(ctx context.Context, taskID string) error
	Fail(ctx context.Context, taskID string, reason string) error
}

// ErrNoTasks is returned by Claim when no tasks are available.
var ErrNoTasks = errors.New("no tasks available")

// InMemoryQueue is a development-only task queue.
type InMemoryQueue struct {
	mu    sync.Mutex
	tasks []*Task
}

// NewInMemoryQueue creates an in-memory task queue.
func NewInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{}
}

// Enqueue adds a task to the queue with status Pending.
func (q *InMemoryQueue) Enqueue(_ context.Context, task Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if task.ID == "" {
		task.ID = uuid.New().String()
	}
	task.Status = TaskPending
	task.CreatedAt = time.Now()
	q.tasks = append(q.tasks, &task)
	return nil
}

// Claim finds the first pending task matching the given kinds, marks it
// as running, and returns it.
func (q *InMemoryQueue) Claim(_ context.Context, kinds []string) (*Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	kindSet := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		kindSet[k] = true
	}

	for _, t := range q.tasks {
		if t.Status != TaskPending {
			continue
		}
		if len(kindSet) > 0 && !kindSet[t.Kind] {
			continue
		}
		t.Status = TaskRunning
		return t, nil
	}

	return nil, ErrNoTasks
}

// Complete marks a task as completed.
func (q *InMemoryQueue) Complete(_ context.Context, taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, t := range q.tasks {
		if t.ID == taskID {
			t.Status = TaskCompleted
			return nil
		}
	}
	return errors.New("task not found: " + taskID)
}

// Fail marks a task as failed with the given reason.
func (q *InMemoryQueue) Fail(_ context.Context, taskID string, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, t := range q.tasks {
		if t.ID == taskID {
			t.Status = TaskFailed
			t.Error = reason
			return nil
		}
	}
	return errors.New("task not found: " + taskID)
}
