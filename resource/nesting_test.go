// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"testing"
)

func TestHasFinalizer(t *testing.T) {
	r := &Resource{Finalizers: JSONArray{"cleanup.io", "protect.io"}}
	if !r.HasFinalizer("cleanup.io") {
		t.Error("expected HasFinalizer to return true")
	}
	if r.HasFinalizer("missing.io") {
		t.Error("expected HasFinalizer to return false for missing finalizer")
	}
}

func TestParentAccessors(t *testing.T) {
	r := &Resource{}

	if r.GetParent() != nil {
		t.Error("expected nil parent on new resource")
	}

	parent := "parent-resource"
	r.SetParent(&parent)
	if r.GetParent() == nil || *r.GetParent() != "parent-resource" {
		t.Errorf("GetParent = %v, want %q", r.GetParent(), "parent-resource")
	}

	r.SetParent(nil)
	if r.GetParent() != nil {
		t.Error("expected nil parent after clearing")
	}
}
