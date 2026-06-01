// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/fabiendupont/infractl/resource"
	"github.com/fabiendupont/infractl/tests/testutil"
)

// TestSpec is a minimal spec for store integration tests.
type TestSpec struct {
	Value string `json:"value"`
}

// TestStatus is a minimal status for store integration tests.
type TestStatus struct {
	Phase string `json:"phase"`
}

// TestResource is a domain resource used exclusively by store integration tests.
type TestResource struct {
	resource.Resource
	Spec   resource.JSONField[TestSpec]   `gorm:"type:jsonb" json:"spec"`
	Status resource.JSONField[TestStatus] `gorm:"type:jsonb" json:"status"`
}

func (TestResource) TableName() string { return "test_resources" }

func (r *TestResource) SpecBytes() ([]byte, error) {
	return resource.MarshalSpec(r.Spec.Data)
}

var (
	orgA = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	orgB = uuid.MustParse("00000000-0000-0000-0000-000000000002")
)

func TestStoreCreateAndGet(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	if err := db.AutoMigrate(&TestResource{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	store := resource.NewGenericStore[TestResource](db)
	ctx := context.Background()

	res := &TestResource{
		Resource: resource.Resource{
			OrgID: orgA,
			Name:  "test-create",
		},
		Spec:   resource.JSONField[TestSpec]{Data: TestSpec{Value: "hello"}},
		Status: resource.JSONField[TestStatus]{Data: TestStatus{Phase: "ready"}},
	}

	if err := store.Create(ctx, res); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Get(ctx, orgA, "test-create")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Name != "test-create" {
		t.Errorf("Name = %q, want %q", got.Name, "test-create")
	}
	if got.OrgID != orgA {
		t.Errorf("OrgID = %v, want %v", got.OrgID, orgA)
	}
	if got.Spec.Data.Value != "hello" {
		t.Errorf("Spec.Value = %q, want %q", got.Spec.Data.Value, "hello")
	}
	if got.Status.Data.Phase != "ready" {
		t.Errorf("Status.Phase = %q, want %q", got.Status.Data.Phase, "ready")
	}
	if got.ResourceVersion != 1 {
		t.Errorf("ResourceVersion = %d, want 1", got.ResourceVersion)
	}
	if got.Generation != 1 {
		t.Errorf("Generation = %d, want 1", got.Generation)
	}
}

func TestStoreCreateDuplicate(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	if err := db.AutoMigrate(&TestResource{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	store := resource.NewGenericStore[TestResource](db)
	ctx := context.Background()

	res := &TestResource{
		Resource: resource.Resource{OrgID: orgA, Name: "dup"},
		Spec:     resource.JSONField[TestSpec]{Data: TestSpec{Value: "v1"}},
	}
	if err := store.Create(ctx, res); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	dup := &TestResource{
		Resource: resource.Resource{OrgID: orgA, Name: "dup"},
		Spec:     resource.JSONField[TestSpec]{Data: TestSpec{Value: "v2"}},
	}
	err := store.Create(ctx, dup)
	if !errors.Is(err, resource.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestStoreGetNotFound(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	if err := db.AutoMigrate(&TestResource{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	store := resource.NewGenericStore[TestResource](db)
	ctx := context.Background()

	_, err := store.Get(ctx, orgA, "does-not-exist")
	if !errors.Is(err, resource.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreListPagination(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	if err := db.AutoMigrate(&TestResource{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	store := resource.NewGenericStore[TestResource](db)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		res := &TestResource{
			Resource: resource.Resource{OrgID: orgA, Name: fmt.Sprintf("item-%02d", i)},
			Spec:     resource.JSONField[TestSpec]{Data: TestSpec{Value: fmt.Sprintf("v%d", i)}},
		}
		if err := store.Create(ctx, res); err != nil {
			t.Fatalf("Create item-%02d: %v", i, err)
		}
	}

	// First page.
	page1, err := store.List(ctx, orgA, resource.ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("List page1: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("page1 items = %d, want 2", len(page1.Items))
	}
	if page1.Total != 5 {
		t.Errorf("page1 total = %d, want 5", page1.Total)
	}
	if page1.Continue == "" {
		t.Fatal("page1 Continue is empty, expected a token")
	}

	// Second page using the continue token.
	page2, err := store.List(ctx, orgA, resource.ListOptions{Limit: 2, Continue: page1.Continue})
	if err != nil {
		t.Fatalf("List page2: %v", err)
	}
	if len(page2.Items) != 2 {
		t.Fatalf("page2 items = %d, want 2", len(page2.Items))
	}

	// Verify no overlap between pages.
	if page1.Items[0].Name == page2.Items[0].Name {
		t.Errorf("page2 first item %q overlaps with page1", page2.Items[0].Name)
	}
}

func TestStoreListWithFilter(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	if err := db.AutoMigrate(&TestResource{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	store := resource.NewGenericStore[TestResource](db)
	ctx := context.Background()

	for _, name := range []string{"alpha", "beta", "gamma"} {
		res := &TestResource{
			Resource: resource.Resource{OrgID: orgA, Name: name},
			Spec:     resource.JSONField[TestSpec]{Data: TestSpec{Value: name}},
		}
		if err := store.Create(ctx, res); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	list, err := store.List(ctx, orgA, resource.ListOptions{Filter: "name=beta"})
	if err != nil {
		t.Fatalf("List with filter: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("filtered items = %d, want 1", len(list.Items))
	}
	if list.Items[0].Name != "beta" {
		t.Errorf("filtered item name = %q, want %q", list.Items[0].Name, "beta")
	}
}

func TestStoreUpdateResourceVersion(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	if err := db.AutoMigrate(&TestResource{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	store := resource.NewGenericStore[TestResource](db)
	ctx := context.Background()

	res := &TestResource{
		Resource: resource.Resource{OrgID: orgA, Name: "rv-test"},
		Spec:     resource.JSONField[TestSpec]{Data: TestSpec{Value: "original"}},
	}
	if err := store.Create(ctx, res); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Get(ctx, orgA, "rv-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ResourceVersion != 1 {
		t.Fatalf("initial RV = %d, want 1", got.ResourceVersion)
	}

	got.Spec.Data.Value = "updated"
	if err := store.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got2, err := store.Get(ctx, orgA, "rv-test")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got2.ResourceVersion != 2 {
		t.Errorf("updated RV = %d, want 2", got2.ResourceVersion)
	}
}

func TestStoreUpdateStaleResourceVersion(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	if err := db.AutoMigrate(&TestResource{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	store := resource.NewGenericStore[TestResource](db)
	ctx := context.Background()

	res := &TestResource{
		Resource: resource.Resource{OrgID: orgA, Name: "stale-rv"},
		Spec:     resource.JSONField[TestSpec]{Data: TestSpec{Value: "v1"}},
	}
	if err := store.Create(ctx, res); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Get(ctx, orgA, "stale-rv")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Set a wrong ResourceVersion to simulate a stale client.
	got.ResourceVersion = 999
	err = store.Update(ctx, got)
	if !errors.Is(err, resource.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestStoreGenerationTracking(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	if err := db.AutoMigrate(&TestResource{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	store := resource.NewGenericStore[TestResource](db)
	ctx := context.Background()

	res := &TestResource{
		Resource: resource.Resource{OrgID: orgA, Name: "gen-track"},
		Spec:     resource.JSONField[TestSpec]{Data: TestSpec{Value: "original"}},
		Status:   resource.JSONField[TestStatus]{Data: TestStatus{Phase: "init"}},
	}
	if err := store.Create(ctx, res); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Update without changing spec: generation should stay the same.
	got, err := store.Get(ctx, orgA, "gen-track")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got.Status.Data.Phase = "running"
	if err := store.Update(ctx, got); err != nil {
		t.Fatalf("Update (status only): %v", err)
	}
	afterStatusUpdate, err := store.Get(ctx, orgA, "gen-track")
	if err != nil {
		t.Fatalf("Get after status update: %v", err)
	}
	if afterStatusUpdate.Generation != 1 {
		t.Errorf("Generation after status-only update = %d, want 1", afterStatusUpdate.Generation)
	}

	// Update with spec change: generation should increment.
	afterStatusUpdate.Spec.Data.Value = "changed"
	if err := store.Update(ctx, afterStatusUpdate); err != nil {
		t.Fatalf("Update (spec change): %v", err)
	}
	afterSpecUpdate, err := store.Get(ctx, orgA, "gen-track")
	if err != nil {
		t.Fatalf("Get after spec update: %v", err)
	}
	if afterSpecUpdate.Generation != 2 {
		t.Errorf("Generation after spec change = %d, want 2", afterSpecUpdate.Generation)
	}
}

func TestStoreDeleteAndGetNotFound(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	if err := db.AutoMigrate(&TestResource{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	store := resource.NewGenericStore[TestResource](db)
	ctx := context.Background()

	res := &TestResource{
		Resource: resource.Resource{OrgID: orgA, Name: "to-delete"},
		Spec:     resource.JSONField[TestSpec]{Data: TestSpec{Value: "bye"}},
	}
	if err := store.Create(ctx, res); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, orgA, "to-delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(ctx, orgA, "to-delete")
	if !errors.Is(err, resource.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStoreTenantIsolation(t *testing.T) {
	db, _, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	if err := db.AutoMigrate(&TestResource{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	store := resource.NewGenericStore[TestResource](db)
	ctx := context.Background()

	res := &TestResource{
		Resource: resource.Resource{OrgID: orgA, Name: "tenant-test"},
		Spec:     resource.JSONField[TestSpec]{Data: TestSpec{Value: "orgA-data"}},
	}
	if err := store.Create(ctx, res); err != nil {
		t.Fatalf("Create in orgA: %v", err)
	}

	// List in orgB should return empty.
	listB, err := store.List(ctx, orgB, resource.ListOptions{})
	if err != nil {
		t.Fatalf("List orgB: %v", err)
	}
	if len(listB.Items) != 0 {
		t.Errorf("orgB items = %d, want 0", len(listB.Items))
	}

	// List in orgA should find the resource.
	listA, err := store.List(ctx, orgA, resource.ListOptions{})
	if err != nil {
		t.Fatalf("List orgA: %v", err)
	}
	if len(listA.Items) != 1 {
		t.Errorf("orgA items = %d, want 1", len(listA.Items))
	}
	if len(listA.Items) > 0 && listA.Items[0].Name != "tenant-test" {
		t.Errorf("orgA item name = %q, want %q", listA.Items[0].Name, "tenant-test")
	}
}
