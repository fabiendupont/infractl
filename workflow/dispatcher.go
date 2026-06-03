// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// ReactionRegistrar accepts async reaction registrations. This avoids
// importing the provider package, breaking the import cycle.
type ReactionRegistrar interface {
	RegisterReactionFunc(feature, event string, callback func(ctx context.Context, payload interface{}))
}

// Dispatcher wires resource lifecycle events to workflow execution.
// It reads from a DispatchTable and delegates to an Executor. Tracks
// in-flight runs and polls for completion via StartPolling.
type Dispatcher struct {
	table    *DispatchTable
	executor Executor
	logger   zerolog.Logger

	mu      sync.Mutex
	tracked map[string]TrackedRun
}

// NewDispatcher creates a dispatcher backed by the given table and executor.
func NewDispatcher(table *DispatchTable, executor Executor, logger zerolog.Logger) *Dispatcher {
	return &Dispatcher{
		table:    table,
		executor: executor,
		logger:   logger.With().Str("component", "dispatcher").Logger(),
		tracked:  make(map[string]TrackedRun),
	}
}

// DispatchOpts identifies the resource being dispatched for tracking.
type DispatchOpts struct {
	OrgID uuid.UUID
	Name  string
}

// Dispatch executes all handlers for a resource lifecycle event in phase
// order. Pre-phase handlers run synchronously and can abort the operation
// by returning an error. The main-phase handler is submitted to the
// executor. Post-phase handlers run after main completes.
//
// Returns the Run from the main-phase handler, or nil if no main handler
// is registered. If opts is provided, the run is tracked for status polling.
func (d *Dispatcher) Dispatch(ctx context.Context, resourceType, event string, input map[string]interface{}, opts ...DispatchOpts) (*Run, error) {
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

		if run.Status != RunCompleted && run.Status != RunFailed && len(opts) > 0 {
			d.mu.Lock()
			d.tracked[run.ID] = TrackedRun{
				RunID:        run.ID,
				ResourceType: resourceType,
				OrgID:        opts[0].OrgID,
				Name:         opts[0].Name,
				Event:        event,
				SubmittedAt:  time.Now(),
			}
			d.mu.Unlock()
		}
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
				opts := extractDispatchOpts(payload)
				run, err := d.Dispatch(ctx, resourceType, "create", input, opts)
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
				opts := extractDispatchOpts(payload)
				run, err := d.Dispatch(ctx, resourceType, "delete", input, opts)
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

// StartPolling begins a background loop that polls all tracked runs
// for completion. When a run reaches a terminal state, the callback
// is invoked and the run is removed from tracking. The loop runs
// until the context is cancelled.
func (d *Dispatcher) StartPolling(ctx context.Context, interval time.Duration, callback StatusCallback) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		d.logger.Info().Dur("interval", interval).Msg("status polling started")

		for {
			select {
			case <-ctx.Done():
				d.logger.Info().Msg("status polling stopped")
				return
			case <-ticker.C:
				d.pollOnce(ctx, callback)
			}
		}
	}()
}

// InFlightCount returns the number of runs currently being tracked.
func (d *Dispatcher) InFlightCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.tracked)
}

func (d *Dispatcher) pollOnce(ctx context.Context, callback StatusCallback) {
	d.mu.Lock()
	runs := make([]TrackedRun, 0, len(d.tracked))
	for _, tr := range d.tracked {
		runs = append(runs, tr)
	}
	d.mu.Unlock()

	for _, tr := range runs {
		run, err := d.executor.Poll(ctx, tr.RunID)
		if err != nil {
			d.logger.Warn().Err(err).Str("run_id", tr.RunID).Msg("poll failed")
			continue
		}

		if run.Status != RunCompleted && run.Status != RunFailed {
			continue
		}

		d.logger.Info().
			Str("run_id", tr.RunID).
			Str("resource_type", tr.ResourceType).
			Str("name", tr.Name).
			Str("status", string(run.Status)).
			Msg("run completed")

		if callback != nil {
			if err := callback(ctx, tr, run); err != nil {
				d.logger.Error().Err(err).Str("run_id", tr.RunID).Msg("status callback failed")
			}
		}

		d.mu.Lock()
		delete(d.tracked, tr.RunID)
		d.mu.Unlock()
	}
}

// resourceIdentifier is satisfied by any type that has GetOrgID and GetName
// (e.g., resource.Resource). Avoids importing the resource package.
type resourceIdentifier interface {
	GetOrgID() uuid.UUID
	GetName() string
}

func extractDispatchOpts(payload interface{}) DispatchOpts {
	if ri, ok := payload.(resourceIdentifier); ok {
		return DispatchOpts{OrgID: ri.GetOrgID(), Name: ri.GetName()}
	}
	if m, ok := payload.(map[string]interface{}); ok {
		var opts DispatchOpts
		if orgID, ok := m["org_id"].(string); ok {
			opts.OrgID, _ = uuid.Parse(orgID)
		}
		if name, ok := m["name"].(string); ok {
			opts.Name = name
		}
		return opts
	}
	return DispatchOpts{}
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
