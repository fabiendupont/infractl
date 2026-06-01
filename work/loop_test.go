package work

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestLoop_RunCallsWorkFunc(t *testing.T) {
	var count atomic.Int32
	logger := zerolog.Nop()

	loop := NewLoop("test", func(_ context.Context) error {
		count.Add(1)
		return nil
	}, 50*time.Millisecond, logger)

	ctx, cancel := context.WithCancel(context.Background())

	go loop.Run(ctx)

	// Wait for at least 2 invocations within 2x interval.
	deadline := time.After(500 * time.Millisecond)
	for {
		if count.Load() >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected at least 2 calls, got %d", count.Load())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
}

func TestLoop_Kick(t *testing.T) {
	var count atomic.Int32
	logger := zerolog.Nop()

	loop := NewLoop("test-kick", func(_ context.Context) error {
		count.Add(1)
		return nil
	}, 10*time.Second, logger) // very long interval

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	// First call happens immediately. Wait for it.
	deadline := time.After(1 * time.Second)
	for count.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("first call did not happen")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// Kick to trigger immediate re-execution instead of waiting 10s.
	loop.Kick()

	deadline = time.After(1 * time.Second)
	for count.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("kick did not trigger re-execution, count=%d", count.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	assert.GreaterOrEqual(t, count.Load(), int32(2))
}

func TestLoop_ContextCancellation(t *testing.T) {
	logger := zerolog.Nop()
	var count atomic.Int32

	loop := NewLoop("test-cancel", func(_ context.Context) error {
		count.Add(1)
		return nil
	}, 50*time.Millisecond, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Let it run once.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// loop exited
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not stop after context cancellation")
	}
}
