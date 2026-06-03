// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// LocalFunc is a function that can be executed by the LocalExecutor.
type LocalFunc func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)

// LocalExecutor runs handlers as in-process Go functions. Handlers are
// registered by ref name. Submit runs the function synchronously and
// returns a completed (or failed) Run immediately.
type LocalExecutor struct {
	mu    sync.RWMutex
	funcs map[string]LocalFunc
}

// NewLocalExecutor creates an in-process executor.
func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{
		funcs: make(map[string]LocalFunc),
	}
}

// Register associates a ref name with a function.
func (e *LocalExecutor) Register(ref string, fn LocalFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.funcs[ref] = fn
}

// Submit runs the handler's function synchronously. Returns a completed
// Run on success or a failed Run on error.
func (e *LocalExecutor) Submit(ctx context.Context, handler Handler, input map[string]interface{}) (*Run, error) {
	e.mu.RLock()
	fn, ok := e.funcs[handler.Ref]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("local executor: no function registered for ref %q", handler.Ref)
	}

	run := &Run{
		ID:     uuid.New().String(),
		Status: RunRunning,
	}

	outputs, err := fn(ctx, input)
	if err != nil {
		run.Status = RunFailed
		run.Error = err.Error()
		return run, nil
	}

	run.Status = RunCompleted
	run.Outputs = outputs
	return run, nil
}

// Poll returns the run as-is since local execution is synchronous.
// The run is always in a terminal state (completed or failed).
func (e *LocalExecutor) Poll(_ context.Context, _ string) (*Run, error) {
	return nil, fmt.Errorf("local executor: poll not supported (execution is synchronous)")
}

// Cancel is a no-op for local execution since Submit blocks until completion.
func (e *LocalExecutor) Cancel(_ context.Context, _ string) error {
	return nil
}

var _ Executor = (*LocalExecutor)(nil)
