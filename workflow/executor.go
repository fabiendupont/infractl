// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import "context"

// RunStatus represents the state of a workflow execution.
type RunStatus string

const (
	RunPending   RunStatus = "pending"
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
)

// Run represents a workflow execution instance.
type Run struct {
	ID      string
	Status  RunStatus
	Outputs map[string]interface{}
	Error   string
}

// Executor submits handlers for execution and polls for completion.
// Implementations are backend-specific: AAP, Temporal, in-process, etc.
type Executor interface {
	Submit(ctx context.Context, handler Handler, input map[string]interface{}) (*Run, error)
	Poll(ctx context.Context, runID string) (*Run, error)
	Cancel(ctx context.Context, runID string) error
}
