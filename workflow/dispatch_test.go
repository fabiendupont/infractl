// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func TestDispatchTableLookupSortsByPhaseAndPriority(t *testing.T) {
	table := NewDispatchTable()
	table.Register(Handler{ResourceType: "machine", Event: "create", Phase: PhasePost, Priority: 10, Ref: "post-10"})
	table.Register(Handler{ResourceType: "machine", Event: "create", Phase: PhasePre, Priority: 20, Ref: "pre-20"})
	table.Register(Handler{ResourceType: "machine", Event: "create", Phase: PhaseMain, Priority: 100, Ref: "main-100"})
	table.Register(Handler{ResourceType: "machine", Event: "create", Phase: PhasePre, Priority: 10, Ref: "pre-10"})

	handlers := table.Lookup("machine", "create")

	if len(handlers) != 4 {
		t.Fatalf("expected 4 handlers, got %d", len(handlers))
	}

	expected := []string{"pre-10", "pre-20", "main-100", "post-10"}
	for i, h := range handlers {
		if h.Ref != expected[i] {
			t.Errorf("handlers[%d].Ref = %q, want %q", i, h.Ref, expected[i])
		}
	}
}

func TestDispatchTableLookupEmpty(t *testing.T) {
	table := NewDispatchTable()
	handlers := table.Lookup("nonexistent", "create")
	if len(handlers) != 0 {
		t.Errorf("expected 0 handlers, got %d", len(handlers))
	}
}

func TestDispatchTableResourceTypes(t *testing.T) {
	table := NewDispatchTable()
	table.Register(Handler{ResourceType: "machine", Event: "create", Phase: PhaseMain, Ref: "m"})
	table.Register(Handler{ResourceType: "network", Event: "create", Phase: PhaseMain, Ref: "n"})
	table.Register(Handler{ResourceType: "machine", Event: "delete", Phase: PhaseMain, Ref: "md"})

	types := table.ResourceTypes()
	if len(types) != 2 {
		t.Fatalf("expected 2 resource types, got %d", len(types))
	}
	if types[0] != "machine" || types[1] != "network" {
		t.Errorf("types = %v, want [machine, network]", types)
	}
}

func TestLocalExecutorSubmitSuccess(t *testing.T) {
	exec := NewLocalExecutor()
	exec.Register("greet", func(_ context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"message": "hello " + input["name"].(string)}, nil
	})

	run, err := exec.Submit(context.Background(), Handler{Ref: "greet"}, map[string]interface{}{"name": "world"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if run.Status != RunCompleted {
		t.Errorf("Status = %q, want %q", run.Status, RunCompleted)
	}
	if run.Outputs["message"] != "hello world" {
		t.Errorf("Outputs[message] = %v, want %q", run.Outputs["message"], "hello world")
	}
}

func TestLocalExecutorSubmitFailure(t *testing.T) {
	exec := NewLocalExecutor()
	exec.Register("fail", func(_ context.Context, _ map[string]interface{}) (map[string]interface{}, error) {
		return nil, fmt.Errorf("intentional failure")
	})

	run, err := exec.Submit(context.Background(), Handler{Ref: "fail"}, nil)
	if err != nil {
		t.Fatalf("Submit should not error, got: %v", err)
	}
	if run.Status != RunFailed {
		t.Errorf("Status = %q, want %q", run.Status, RunFailed)
	}
	if run.Error != "intentional failure" {
		t.Errorf("Error = %q", run.Error)
	}
}

func TestLocalExecutorSubmitUnknownRef(t *testing.T) {
	exec := NewLocalExecutor()
	_, err := exec.Submit(context.Background(), Handler{Ref: "missing"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown ref")
	}
}

func TestDispatcherPrePhaseAborts(t *testing.T) {
	table := NewDispatchTable()
	table.Register(Handler{ResourceType: "vm", Event: "create", Phase: PhasePre, Ref: "validate"})
	table.Register(Handler{ResourceType: "vm", Event: "create", Phase: PhaseMain, Ref: "provision"})

	exec := NewLocalExecutor()
	exec.Register("validate", func(_ context.Context, _ map[string]interface{}) (map[string]interface{}, error) {
		return nil, fmt.Errorf("quota exceeded")
	})
	exec.Register("provision", func(_ context.Context, _ map[string]interface{}) (map[string]interface{}, error) {
		t.Error("main handler should not run when pre-phase aborts")
		return nil, nil
	})

	logger := zerolog.Nop()
	dispatcher := NewDispatcher(table, exec, logger)

	_, err := dispatcher.Dispatch(context.Background(), "vm", "create", nil)
	if err == nil {
		t.Fatal("expected error from pre-phase rejection")
	}
}

func TestDispatcherFullLifecycle(t *testing.T) {
	table := NewDispatchTable()
	table.Register(Handler{ResourceType: "vm", Event: "create", Phase: PhasePre, Ref: "check"})
	table.Register(Handler{ResourceType: "vm", Event: "create", Phase: PhaseMain, Ref: "provision"})
	table.Register(Handler{ResourceType: "vm", Event: "create", Phase: PhasePost, Ref: "notify"})

	var order []string

	exec := NewLocalExecutor()
	exec.Register("check", func(_ context.Context, _ map[string]interface{}) (map[string]interface{}, error) {
		order = append(order, "pre")
		return nil, nil
	})
	exec.Register("provision", func(_ context.Context, _ map[string]interface{}) (map[string]interface{}, error) {
		order = append(order, "main")
		return map[string]interface{}{"vm_id": "vm-123"}, nil
	})
	exec.Register("notify", func(_ context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		order = append(order, "post")
		if input["vm_id"] != "vm-123" {
			t.Errorf("post handler did not receive main outputs: vm_id = %v", input["vm_id"])
		}
		return nil, nil
	})

	logger := zerolog.Nop()
	dispatcher := NewDispatcher(table, exec, logger)

	run, err := dispatcher.Dispatch(context.Background(), "vm", "create", map[string]interface{}{"name": "test"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if run.Status != RunCompleted {
		t.Errorf("Run.Status = %q, want %q", run.Status, RunCompleted)
	}
	if run.Outputs["vm_id"] != "vm-123" {
		t.Errorf("Run.Outputs[vm_id] = %v", run.Outputs["vm_id"])
	}

	expectedOrder := []string{"pre", "main", "post"}
	if len(order) != 3 {
		t.Fatalf("execution order = %v, want %v", order, expectedOrder)
	}
	for i, phase := range expectedOrder {
		if order[i] != phase {
			t.Errorf("order[%d] = %q, want %q", i, order[i], phase)
		}
	}
}

func TestDispatcherNoHandlers(t *testing.T) {
	table := NewDispatchTable()
	exec := NewLocalExecutor()
	logger := zerolog.Nop()
	dispatcher := NewDispatcher(table, exec, logger)

	run, err := dispatcher.Dispatch(context.Background(), "nothing", "create", nil)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if run != nil {
		t.Errorf("expected nil run for no handlers, got %+v", run)
	}
}

// asyncExecutor simulates an async executor where Submit returns Pending
// and Poll returns the final status after a configured number of polls.
type asyncExecutor struct {
	mu         sync.Mutex
	runs       map[string]*Run
	pollCounts map[string]int
	readyAfter int
}

func newAsyncExecutor(readyAfter int) *asyncExecutor {
	return &asyncExecutor{
		runs:       make(map[string]*Run),
		pollCounts: make(map[string]int),
		readyAfter: readyAfter,
	}
}

func (e *asyncExecutor) Submit(_ context.Context, h Handler, _ map[string]interface{}) (*Run, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	run := &Run{ID: "async-" + h.Ref, Status: RunPending}
	e.runs[run.ID] = run
	return run, nil
}

func (e *asyncExecutor) Poll(_ context.Context, runID string) (*Run, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pollCounts[runID]++
	if e.pollCounts[runID] >= e.readyAfter {
		return &Run{
			ID:      runID,
			Status:  RunCompleted,
			Outputs: map[string]interface{}{"result": "done"},
		}, nil
	}
	return &Run{ID: runID, Status: RunRunning}, nil
}

func (e *asyncExecutor) Cancel(_ context.Context, _ string) error { return nil }

func TestStatusPolling(t *testing.T) {
	table := NewDispatchTable()
	table.Register(Handler{
		ResourceType: "Server", Event: "create",
		Phase: PhaseMain, Ref: "provision",
	})

	exec := newAsyncExecutor(2)
	logger := zerolog.Nop()
	dispatcher := NewDispatcher(table, exec, logger)

	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	run, err := dispatcher.Dispatch(context.Background(), "Server", "create",
		map[string]interface{}{"name": "test"},
		DispatchOpts{OrgID: orgID, Name: "my-server"},
	)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if run.Status != RunPending {
		t.Errorf("initial status = %q, want %q", run.Status, RunPending)
	}
	if dispatcher.InFlightCount() != 1 {
		t.Errorf("in-flight = %d, want 1", dispatcher.InFlightCount())
	}

	var callbackCalled bool
	var callbackTracked TrackedRun
	var callbackRun *Run

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dispatcher.StartPolling(ctx, 50*time.Millisecond, func(_ context.Context, tracked TrackedRun, r *Run) error {
		callbackCalled = true
		callbackTracked = tracked
		callbackRun = r
		return nil
	})

	// Wait for polling to detect completion.
	deadline := time.After(3 * time.Second)
	for {
		if dispatcher.InFlightCount() == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for polling to complete")
		case <-time.After(25 * time.Millisecond):
		}
	}

	if !callbackCalled {
		t.Fatal("callback was not called")
	}
	if callbackTracked.ResourceType != "Server" {
		t.Errorf("tracked ResourceType = %q, want %q", callbackTracked.ResourceType, "Server")
	}
	if callbackTracked.Name != "my-server" {
		t.Errorf("tracked Name = %q, want %q", callbackTracked.Name, "my-server")
	}
	if callbackTracked.OrgID != orgID {
		t.Errorf("tracked OrgID = %v, want %v", callbackTracked.OrgID, orgID)
	}
	if callbackRun.Status != RunCompleted {
		t.Errorf("run status = %q, want %q", callbackRun.Status, RunCompleted)
	}
}
