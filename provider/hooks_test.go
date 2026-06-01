package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHookRunner(t *testing.T) (*Registry, *HookRunner) {
	t.Helper()
	r := NewRegistry()
	logger := zerolog.Nop()
	hr := NewHookRunner(r, logger)
	return r, hr
}

func TestFireSync_HookCalled(t *testing.T) {
	r, hr := newTestHookRunner(t)

	called := false
	r.RegisterHook(SyncHook{
		Feature: "res",
		Event:   "created",
		Handler: func(_ context.Context, payload interface{}) error {
			called = true
			assert.Equal(t, "data", payload)
			return nil
		},
	})

	err := hr.FireSync(context.Background(), "res", "created", "data")
	require.NoError(t, err)
	assert.True(t, called)
}

func TestFireSync_ReturnsError(t *testing.T) {
	r, hr := newTestHookRunner(t)

	r.RegisterHook(SyncHook{
		Feature: "res",
		Event:   "created",
		Handler: func(_ context.Context, _ interface{}) error {
			return errors.New("hook failed")
		},
	})

	err := hr.FireSync(context.Background(), "res", "created", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hook failed")
}

func TestFireSync_NoHooks(t *testing.T) {
	_, hr := newTestHookRunner(t)
	err := hr.FireSync(context.Background(), "nonexistent", "event", nil)
	require.NoError(t, err)
}

func TestFireAsync_ReactionCalled(t *testing.T) {
	r, hr := newTestHookRunner(t)

	done := make(chan struct{})
	r.RegisterReaction(Reaction{
		Feature: "res",
		Event:   "created",
		Callback: func(_ context.Context, payload interface{}) {
			close(done)
		},
	})

	hr.FireAsync(context.Background(), "res", "created", nil)

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("reaction was not called within timeout")
	}
}

func TestFireAsync_PanicRecovered(t *testing.T) {
	r, hr := newTestHookRunner(t)

	done := make(chan struct{})
	r.RegisterReaction(Reaction{
		Feature: "res",
		Event:   "boom",
		Callback: func(_ context.Context, _ interface{}) {
			defer close(done)
			panic("test panic")
		},
	})

	// Should not panic the test.
	hr.FireAsync(context.Background(), "res", "boom", nil)

	select {
	case <-done:
		// panic was recovered, goroutine completed
	case <-time.After(2 * time.Second):
		t.Fatal("reaction goroutine did not complete within timeout")
	}
}
