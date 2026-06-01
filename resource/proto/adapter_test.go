// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package proto

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fabiendupont/infractl/resource"
)

func TestMetadataRoundTrip(t *testing.T) {
	owner := "admin"
	now := time.Now().Truncate(time.Microsecond)
	orgID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	original := resource.Resource{
		OrgID:           orgID,
		Name:            "test-resource",
		Labels:          resource.JSONMap{"env": "prod"},
		Annotations:     resource.JSONMap{"note": "important"},
		Finalizers:      resource.JSONArray{"cleanup.infractl.io"},
		Generation:      3,
		ResourceVersion: 7,
		Owner:           &owner,
		Creator:         "alice",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	proto := MetadataToProto(&original)

	if proto.GetName() != "test-resource" {
		t.Errorf("Name = %q, want %q", proto.GetName(), "test-resource")
	}
	if proto.GetOrgId() != orgID.String() {
		t.Errorf("OrgId = %q, want %q", proto.GetOrgId(), orgID.String())
	}
	if proto.GetGeneration() != 3 {
		t.Errorf("Generation = %d, want 3", proto.GetGeneration())
	}
	if proto.GetResourceVersion() != 7 {
		t.Errorf("ResourceVersion = %d, want 7", proto.GetResourceVersion())
	}
	if proto.GetOwner() != "admin" {
		t.Errorf("Owner = %q, want %q", proto.GetOwner(), "admin")
	}
	if proto.GetCreator() != "alice" {
		t.Errorf("Creator = %q, want %q", proto.GetCreator(), "alice")
	}
	if proto.GetLabels()["env"] != "prod" {
		t.Errorf("Labels[env] = %q, want %q", proto.GetLabels()["env"], "prod")
	}
	if len(proto.GetFinalizers()) != 1 || proto.GetFinalizers()[0] != "cleanup.infractl.io" {
		t.Errorf("Finalizers = %v", proto.GetFinalizers())
	}

	roundTripped := MetadataFromProto(proto)

	if roundTripped.OrgID != orgID {
		t.Errorf("roundTripped OrgID = %v, want %v", roundTripped.OrgID, orgID)
	}
	if roundTripped.Name != "test-resource" {
		t.Errorf("roundTripped Name = %q", roundTripped.Name)
	}
	if roundTripped.Generation != 3 {
		t.Errorf("roundTripped Generation = %d", roundTripped.Generation)
	}
	if roundTripped.ResourceVersion != 7 {
		t.Errorf("roundTripped ResourceVersion = %d", roundTripped.ResourceVersion)
	}
	if *roundTripped.Owner != "admin" {
		t.Errorf("roundTripped Owner = %q", *roundTripped.Owner)
	}
	if roundTripped.Creator != "alice" {
		t.Errorf("roundTripped Creator = %q", roundTripped.Creator)
	}
	if roundTripped.Labels["env"] != "prod" {
		t.Errorf("roundTripped Labels[env] = %q", roundTripped.Labels["env"])
	}
	if len(roundTripped.Finalizers) != 1 {
		t.Errorf("roundTripped Finalizers = %v", roundTripped.Finalizers)
	}
}
