// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"testing"
)

func TestSubjectAttributionLogic(t *testing.T) {
	logic := &SubjectAttributionLogic{}

	ctx := ContextWithSubject(context.Background(), &Subject{User: "alice"})
	creator, err := logic.DetermineAssignedCreator(ctx)
	if err != nil {
		t.Fatalf("DetermineAssignedCreator: %v", err)
	}
	if creator != "alice" {
		t.Errorf("creator = %q, want %q", creator, "alice")
	}
}

func TestSubjectAttributionLogicNoSubject(t *testing.T) {
	logic := &SubjectAttributionLogic{}

	_, err := logic.DetermineAssignedCreator(context.Background())
	if err == nil {
		t.Fatal("expected error for missing subject")
	}
}

func TestGuestAttributionLogic(t *testing.T) {
	logic := &GuestAttributionLogic{Creator: "system"}

	creator, err := logic.DetermineAssignedCreator(context.Background())
	if err != nil {
		t.Fatalf("DetermineAssignedCreator: %v", err)
	}
	if creator != "system" {
		t.Errorf("creator = %q, want %q", creator, "system")
	}
}
