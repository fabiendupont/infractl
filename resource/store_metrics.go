// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
)

var storeOpDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "infractl",
		Subsystem: "store",
		Name:      "operation_duration_seconds",
		Help:      "Duration of store operations in seconds.",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
	},
	[]string{"table", "operation", "error"},
)

func init() {
	prometheus.MustRegister(storeOpDuration)
}

// InstrumentedStore wraps a Store with Prometheus metrics.
type InstrumentedStore[R any] struct {
	inner Store[R]
	table string
}

// NewInstrumentedStore wraps the given store with Prometheus instrumentation.
// The table name is used as the "table" label on all metrics.
func NewInstrumentedStore[R any](inner Store[R], table string) *InstrumentedStore[R] {
	return &InstrumentedStore[R]{inner: inner, table: table}
}

func (s *InstrumentedStore[R]) Create(ctx context.Context, resource *R) error {
	start := time.Now()
	err := s.inner.Create(ctx, resource)
	s.record("create", start, err)
	return err
}

func (s *InstrumentedStore[R]) Get(ctx context.Context, orgID uuid.UUID, name string) (*R, error) {
	start := time.Now()
	r, err := s.inner.Get(ctx, orgID, name)
	s.record("get", start, err)
	return r, err
}

func (s *InstrumentedStore[R]) List(ctx context.Context, orgID uuid.UUID, opts ListOptions) (*List[R], error) {
	start := time.Now()
	l, err := s.inner.List(ctx, orgID, opts)
	s.record("list", start, err)
	return l, err
}

func (s *InstrumentedStore[R]) Update(ctx context.Context, resource *R) error {
	start := time.Now()
	err := s.inner.Update(ctx, resource)
	s.record("update", start, err)
	return err
}

func (s *InstrumentedStore[R]) Delete(ctx context.Context, orgID uuid.UUID, name string) error {
	start := time.Now()
	err := s.inner.Delete(ctx, orgID, name)
	s.record("delete", start, err)
	return err
}

func (s *InstrumentedStore[R]) record(op string, start time.Time, err error) {
	code := ""
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			code = pgErr.Code
		} else {
			code = "error"
		}
	}
	storeOpDuration.WithLabelValues(s.table, op, code).Observe(time.Since(start).Seconds())
}
