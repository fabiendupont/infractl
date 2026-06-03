// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
)

// ReactionRegistrar accepts async reaction registrations. This avoids
// importing the provider package, breaking the import cycle.
type ReactionRegistrar interface {
	RegisterReactionFunc(feature, event string, callback func(ctx context.Context, payload interface{}))
}

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

// RegisterHooks registers async reactions so that post_create and
// post_delete events trigger workflow dispatch. Call this after all
// WorkflowProviders have registered their actions.
func (d *Dispatcher) RegisterHooks(registrar ReactionRegistrar) {
	for _, rt := range d.table.ResourceTypes() {
		resourceType := rt

		if len(d.table.Lookup(resourceType, "create")) > 0 {
			registrar.RegisterReactionFunc(resourceType, "post_create", func(ctx context.Context, payload interface{}) {
				input := map[string]interface{}{"resource": payload}
				run, err := d.Dispatch(ctx, resourceType, "create", input)
				if err != nil {
					d.logger.Error().Err(err).Str("resource_type", resourceType).Msg("create dispatch failed")
					return
				}
				if run != nil {
					d.logger.Info().Str("resource_type", resourceType).Str("run_id", run.ID).Msg("create workflow submitted")
				}
			})
		}

		if len(d.table.Lookup(resourceType, "delete")) > 0 {
			registrar.RegisterReactionFunc(resourceType, "post_delete", func(ctx context.Context, payload interface{}) {
				input := map[string]interface{}{"resource": payload}
				run, err := d.Dispatch(ctx, resourceType, "delete", input)
				if err != nil {
					d.logger.Error().Err(err).Str("resource_type", resourceType).Msg("delete dispatch failed")
					return
				}
				if run != nil {
					d.logger.Info().Str("resource_type", resourceType).Str("run_id", run.ID).Msg("delete workflow submitted")
				}
			})
		}
	}

	d.logger.Info().
		Int("resource_types", len(d.table.ResourceTypes())).
		Msg("workflow dispatch hooks registered")
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
