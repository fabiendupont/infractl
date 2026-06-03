// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
)

// Dispatcher wires resource lifecycle events to workflow execution.
// It reads from a DispatchTable and delegates to an Executor.
type Dispatcher struct {
	table    *DispatchTable
	executor Executor
	logger   zerolog.Logger
}

// NewDispatcher creates a dispatcher backed by the given table and executor.
func NewDispatcher(table *DispatchTable, executor Executor, logger zerolog.Logger) *Dispatcher {
	return &Dispatcher{
		table:    table,
		executor: executor,
		logger:   logger.With().Str("component", "dispatcher").Logger(),
	}
}

// Dispatch executes all handlers for a resource lifecycle event in phase
// order. Pre-phase handlers run synchronously and can abort the operation
// by returning an error. The main-phase handler is submitted to the
// executor. Post-phase handlers run after main completes.
//
// Returns the Run from the main-phase handler, or nil if no main handler
// is registered.
func (d *Dispatcher) Dispatch(ctx context.Context, resourceType, event string, input map[string]interface{}) (*Run, error) {
	handlers := d.table.Lookup(resourceType, event)
	if len(handlers) == 0 {
		return nil, nil
	}

	d.logger.Debug().
		Str("resource_type", resourceType).
		Str("event", event).
		Int("handlers", len(handlers)).
		Msg("dispatching")

	// Pre-phase: synchronous, can abort.
	for _, h := range handlers {
		if h.Phase != PhasePre {
			continue
		}
		run, err := d.executor.Submit(ctx, h, input)
		if err != nil {
			return nil, fmt.Errorf("pre-phase handler %q failed to submit: %w", h.Ref, err)
		}
		if run.Status == RunFailed {
			return nil, fmt.Errorf("pre-phase handler %q rejected: %s", h.Ref, run.Error)
		}
	}

	// Main phase: submit and track.
	var mainRun *Run
	for _, h := range handlers {
		if h.Phase != PhaseMain {
			continue
		}
		run, err := d.executor.Submit(ctx, h, input)
		if err != nil {
			return nil, fmt.Errorf("main-phase handler %q failed to submit: %w", h.Ref, err)
		}
		mainRun = run
		break
	}

	// Post-phase: fire-and-forget.
	for _, h := range handlers {
		if h.Phase != PhasePost {
			continue
		}
		postInput := input
		if mainRun != nil && mainRun.Outputs != nil {
			postInput = mergeInputs(input, mainRun.Outputs)
		}
		run, err := d.executor.Submit(ctx, h, postInput)
		if err != nil {
			d.logger.Warn().Err(err).Str("handler", h.Ref).Msg("post-phase handler failed to submit")
			continue
		}
		if run.Status == RunFailed {
			d.logger.Warn().Str("handler", h.Ref).Str("error", run.Error).Msg("post-phase handler failed")
		}
	}

	return mainRun, nil
}

func mergeInputs(base, overlay map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(base)+len(overlay))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return merged
}
