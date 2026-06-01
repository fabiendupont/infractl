// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type mockStore struct{}

func (m *mockStore) Create(_ context.Context, _ *struct{}) error                      { return nil }
func (m *mockStore) Get(_ context.Context, _ uuid.UUID, _ string) (*struct{}, error)  { return nil, nil }
func (m *mockStore) List(_ context.Context, _ uuid.UUID, _ ListOptions) (*List[struct{}], error) {
	return &List[struct{}]{}, nil
}
func (m *mockStore) Update(_ context.Context, _ *struct{}) error                      { return nil }
func (m *mockStore) Delete(_ context.Context, _ uuid.UUID, _ string) error            { return nil }

func TestInstrumentedStoreRecordsMetrics(t *testing.T) {
	store := NewInstrumentedStore[struct{}](&mockStore{}, "test_resources")
	ctx := context.Background()
	orgID := uuid.New()

	store.Create(ctx, &struct{}{})
	store.Get(ctx, orgID, "test")
	store.List(ctx, orgID, ListOptions{})
	store.Update(ctx, &struct{}{})
	store.Delete(ctx, orgID, "test")

	count := testutil.CollectAndCount(storeOpDuration)
	if count == 0 {
		t.Error("expected metrics to be recorded, got 0")
	}
}
