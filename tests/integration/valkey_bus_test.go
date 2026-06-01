// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/fabiendupont/infractl/events"
	"github.com/fabiendupont/infractl/tests/testutil"
)

func TestValkeyBusPublishSubscribeRoundTrip(t *testing.T) {
	client, cleanup := testutil.SetupValkey(t)
	t.Cleanup(cleanup)

	bus := events.NewValkeyBus(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := bus.Subscribe(ctx, nil)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	evt := events.Event{
		OrgID:  orgA,
		Kind:   "machines",
		Name:   "machine-1",
		Action: "created",
	}
	if err := bus.Publish(ctx, evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case received := <-ch:
		if received.Kind != "machines" {
			t.Errorf("Kind = %q, want %q", received.Kind, "machines")
		}
		if received.Name != "machine-1" {
			t.Errorf("Name = %q, want %q", received.Name, "machine-1")
		}
		if received.Action != "created" {
			t.Errorf("Action = %q, want %q", received.Action, "created")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestValkeyBusKindFilter(t *testing.T) {
	client, cleanup := testutil.SetupValkey(t)
	t.Cleanup(cleanup)

	bus := events.NewValkeyBus(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := bus.Subscribe(ctx, []string{"machines"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	if err := bus.Publish(ctx, events.Event{
		OrgID: orgA, Kind: "networks", Name: "net-1", Action: "created",
	}); err != nil {
		t.Fatalf("Publish networks: %v", err)
	}

	if err := bus.Publish(ctx, events.Event{
		OrgID: orgA, Kind: "machines", Name: "machine-2", Action: "created",
	}); err != nil {
		t.Fatalf("Publish machines: %v", err)
	}

	select {
	case received := <-ch:
		if received.Kind != "machines" {
			t.Errorf("Kind = %q, want %q", received.Kind, "machines")
		}
		if received.Name != "machine-2" {
			t.Errorf("Name = %q, want %q", received.Name, "machine-2")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for machines event")
	}

	select {
	case extra := <-ch:
		t.Fatalf("unexpected extra event: %+v", extra)
	case <-time.After(500 * time.Millisecond):
	}
}

func TestValkeyBusMultipleSubscribers(t *testing.T) {
	client, cleanup := testutil.SetupValkey(t)
	t.Cleanup(cleanup)

	bus := events.NewValkeyBus(client)

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
		OrgID: orgA, Kind: "machines", Name: "machine-multi", Action: "created",
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
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}
