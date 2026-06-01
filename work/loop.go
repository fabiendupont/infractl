// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

// Package work provides background task execution patterns adapted from
// OSAC's work/work_loop.go and FlightCtl's tasks/consumer.go.
package work

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// Loop executes a function repeatedly at a configurable interval. It supports
// kick-to-wake for immediate re-execution and graceful shutdown via context
// cancellation.
type Loop struct {
	name     string
	workFunc func(ctx context.Context) error
	interval time.Duration
	kick     chan struct{}
	logger   zerolog.Logger
}

// NewLoop creates a work loop that calls workFunc every interval.
func NewLoop(name string, workFunc func(ctx context.Context) error, interval time.Duration, logger zerolog.Logger) *Loop {
	return &Loop{
		name:     name,
		workFunc: workFunc,
		interval: interval,
		kick:     make(chan struct{}, 1),
		logger:   logger.With().Str("loop", name).Logger(),
	}
}

// Run starts the work loop. It blocks until the context is cancelled.
func (l *Loop) Run(ctx context.Context) {
	l.logger.Info().Dur("interval", l.interval).Msg("starting work loop")

	for {
		start := time.Now()

		if err := l.workFunc(ctx); err != nil {
			l.logger.Error().Err(err).Msg("work function failed")
		}

		if ctx.Err() != nil {
			l.logger.Info().Msg("work loop stopped")
			return
		}

		l.sleep(ctx, time.Since(start))
		if ctx.Err() != nil {
			l.logger.Info().Msg("work loop stopped")
			return
		}
	}
}

// Kick interrupts the current sleep and triggers immediate re-execution.
// Non-blocking: if a kick is already pending, the call is a no-op.
func (l *Loop) Kick() {
	select {
	case l.kick <- struct{}{}:
	default:
	}
}

func (l *Loop) sleep(ctx context.Context, elapsed time.Duration) {
	remaining := l.interval - elapsed
	if remaining <= 0 {
		return
	}

	timer := time.NewTimer(remaining)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-l.kick:
	case <-timer.C:
	}
}
