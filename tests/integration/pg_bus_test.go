// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/fabiendupont/infractl/events"
	"github.com/fabiendupont/infractl/tests/testutil"
)

func TestPgBusPublishInsertsRecord(t *testing.T) {
	db, dsn, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	bus, err := events.NewPgBus(db, dsn, logger)
	if err != nil {
		t.Fatalf("NewPgBus: %v", err)
	}
	defer bus.Close()

	ctx := context.Background()
	evt := events.Event{
		ID:     "evt-001",
		OrgID:  orgA,
		Kind:   "machines",
		Name:   "machine-1",
		Action: "created",
	}

	if err := bus.Publish(ctx, evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	var record events.EventRecord
	if err := db.First(&record, "id = ?", "evt-001").Error; err != nil {
		t.Fatalf("query event record: %v", err)
	}
	if record.Kind != "machines" {
		t.Errorf("Kind = %q, want %q", record.Kind, "machines")
	}
	if record.Name != "machine-1" {
		t.Errorf("Name = %q, want %q", record.Name, "machine-1")
	}
	if record.Action != "created" {
		t.Errorf("Action = %q, want %q", record.Action, "created")
	}
}

func TestPgBusPublishSubscribeRoundTrip(t *testing.T) {
	db, dsn, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	bus, err := events.NewPgBus(db, dsn, logger)
	if err != nil {
		t.Fatalf("NewPgBus: %v", err)
	}
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := bus.Subscribe(ctx, nil)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Give the LISTEN goroutine time to connect.
	time.Sleep(500 * time.Millisecond)

	evt := events.Event{
		OrgID:  orgA,
		Kind:   "machines",
		Name:   "machine-2",
		Action: "updated",
	}
	if err := bus.Publish(ctx, evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case received := <-ch:
		if received.Kind != "machines" {
			t.Errorf("received Kind = %q, want %q", received.Kind, "machines")
		}
		if received.Name != "machine-2" {
			t.Errorf("received Name = %q, want %q", received.Name, "machine-2")
		}
		if received.Action != "updated" {
			t.Errorf("received Action = %q, want %q", received.Action, "updated")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestPgBusKindFilter(t *testing.T) {
	db, dsn, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	bus, err := events.NewPgBus(db, dsn, logger)
	if err != nil {
		t.Fatalf("NewPgBus: %v", err)
	}
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe only to "machines" events.
	ch, err := bus.Subscribe(ctx, []string{"machines"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Publish a "networks" event (should be filtered out).
	if err := bus.Publish(ctx, events.Event{
		OrgID:  orgA,
		Kind:   "networks",
		Name:   "net-1",
		Action: "created",
	}); err != nil {
		t.Fatalf("Publish networks: %v", err)
	}

	// Publish a "machines" event (should be received).
	if err := bus.Publish(ctx, events.Event{
		OrgID:  orgA,
		Kind:   "machines",
		Name:   "machine-3",
		Action: "created",
	}); err != nil {
		t.Fatalf("Publish machines: %v", err)
	}

	select {
	case received := <-ch:
		if received.Kind != "machines" {
			t.Errorf("received Kind = %q, want %q", received.Kind, "machines")
		}
		if received.Name != "machine-3" {
			t.Errorf("received Name = %q, want %q", received.Name, "machine-3")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for machines event")
	}

	// Verify no extra event arrives (the networks event should have been dropped).
	select {
	case extra := <-ch:
		t.Fatalf("unexpected extra event: %+v", extra)
	case <-time.After(500 * time.Millisecond):
		// Expected: no more events.
	}
}

func TestPgBusMultipleSubscribers(t *testing.T) {
	db, dsn, cleanup := testutil.SetupPostgres(t)
	t.Cleanup(cleanup)

	logger := zerolog.Nop()
	bus, err := events.NewPgBus(db, dsn, logger)
	if err != nil {
		t.Fatalf("NewPgBus: %v", err)
	}
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch1, err := bus.Subscribe(ctx, nil)
	if err != nil {
		t.Fatalf("Subscribe ch1: %v", err)
	}
	ch2, err := bus.Subscribe(ctx, nil)
	if err != nil {
		t.Fatalf("Subscribe ch2: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	evt := events.Event{
		ID:     uuid.New().String(),
		OrgID:  orgA,
		Kind:   "machines",
		Name:   "machine-multi",
		Action: "created",
	}
	if err := bus.Publish(ctx, evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	for i, ch := range []<-chan events.Event{ch1, ch2} {
		select {
		case received := <-ch:
			if received.Name != "machine-multi" {
				t.Errorf("subscriber %d: Name = %q, want %q", i, received.Name, "machine-multi")
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}
