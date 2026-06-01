// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package mixins_test

import (
	"encoding/json"
	"testing"

	"github.com/fabiendupont/infractl/resource/mixins"
)

type composedSpec struct {
	mixins.BMCSpec
	Arch string `json:"arch"`
	CPUs int    `json:"cpus"`
}

type composedStatus struct {
	mixins.BMCStatus
	mixins.LifecycleStatus
}

func TestBMCSpec_JSONRoundTrip(t *testing.T) {
	spec := composedSpec{
		BMCSpec: mixins.BMCSpec{BMCAddr: "192.168.1.100", BMCUser: "admin"},
		Arch:    "x86_64",
		CPUs:    4,
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}

	var decoded composedSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.BMCAddr != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %s", decoded.BMCAddr)
	}
	if decoded.Arch != "x86_64" {
		t.Errorf("expected x86_64, got %s", decoded.Arch)
	}
}

func TestBMCStatus_JSONRoundTrip(t *testing.T) {
	status := composedStatus{
		BMCStatus:       mixins.BMCStatus{PowerState: "on", BMCReachable: true},
		LifecycleStatus: mixins.LifecycleStatus{Phase: "Ready"},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}

	var decoded composedStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.PowerState != "on" {
		t.Errorf("expected on, got %s", decoded.PowerState)
	}
	if !decoded.BMCReachable {
		t.Error("expected BMCReachable to be true")
	}
	if decoded.Phase != "Ready" {
		t.Errorf("expected Ready, got %s", decoded.Phase)
	}
}

func TestNetworkStatus_JSONRoundTrip(t *testing.T) {
	status := mixins.NetworkInterfaceStatus{
		Name:       "eth0",
		MACAddress: "00:11:22:33:44:55",
		Addresses: []mixins.Address{
			{Address: "10.0.0.1", Type: "ipv4", Prefix: 24, Gateway: "10.0.0.254"},
		},
		State: "up",
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}

	var decoded mixins.NetworkInterfaceStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Name != "eth0" {
		t.Errorf("expected eth0, got %s", decoded.Name)
	}
	if len(decoded.Addresses) != 1 || decoded.Addresses[0].Gateway != "10.0.0.254" {
		t.Error("unexpected address round-trip")
	}
}

func TestLifecycleStatus_WithConditions(t *testing.T) {
	status := mixins.LifecycleStatus{
		Phase: "Provisioning",
		Conditions: []mixins.Condition{
			{Type: "BMCReachable", Status: "True"},
			{Type: "OSInstalled", Status: "False", Reason: "Installing", Message: "OS installation in progress"},
		},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}

	var decoded mixins.LifecycleStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if len(decoded.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(decoded.Conditions))
	}
	if decoded.Conditions[1].Reason != "Installing" {
		t.Errorf("expected Installing, got %s", decoded.Conditions[1].Reason)
	}
}

func TestOmitemptyFields(t *testing.T) {
	spec := mixins.BMCSpec{BMCAddr: "192.168.1.1"}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}

	s := string(data)
	if contains(s, "bmc_user") {
		t.Error("empty bmc_user should be omitted")
	}
	if contains(s, "bmc_password") {
		t.Error("empty bmc_password should be omitted")
	}
	if !contains(s, "bmc_addr") {
		t.Error("bmc_addr should be present")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && json.Valid([]byte(s)) && len(substr) > 0 && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
