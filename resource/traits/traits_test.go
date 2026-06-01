// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package traits_test

import (
	"testing"

	"github.com/fabiendupont/infractl/resource/traits"
)

type mockResource struct{}

func (m *mockResource) BMCAddress() string          { return "192.168.1.100" }
func (m *mockResource) PowerState() traits.PowerState { return traits.PowerOn }
func (m *mockResource) Phase() traits.Phase          { return traits.PhaseReady }
func (m *mockResource) Conditions() []traits.Condition {
	return []traits.Condition{{Type: "Ready", Status: "True"}}
}
func (m *mockResource) NetworkInterfaces() []traits.NetworkInterface {
	return []traits.NetworkInterface{{Name: "eth0", MACAddress: "00:11:22:33:44:55"}}
}
func (m *mockResource) FirmwareComponents() []traits.FirmwareComponent {
	return []traits.FirmwareComponent{{Name: "bios", CurrentVersion: "1.0", DesiredVersion: "1.1"}}
}
func (m *mockResource) HealthStatus() traits.HealthStatus { return traits.HealthHealthy }
func (m *mockResource) HealthMessage() string             { return "all good" }

var _ traits.Provisionable = (*mockResource)(nil)
var _ traits.Lifecycleable = (*mockResource)(nil)
var _ traits.Networkable = (*mockResource)(nil)
var _ traits.Firmwareable = (*mockResource)(nil)
var _ traits.Healthable = (*mockResource)(nil)

func TestTraitComposition(t *testing.T) {
	var r interface{} = &mockResource{}

	if p, ok := r.(traits.Provisionable); ok {
		if p.BMCAddress() != "192.168.1.100" {
			t.Errorf("expected 192.168.1.100, got %s", p.BMCAddress())
		}
	} else {
		t.Error("mockResource should implement Provisionable")
	}

	if l, ok := r.(traits.Lifecycleable); ok {
		if l.Phase() != traits.PhaseReady {
			t.Errorf("expected Ready, got %s", l.Phase())
		}
		if len(l.Conditions()) != 1 {
			t.Errorf("expected 1 condition, got %d", len(l.Conditions()))
		}
	} else {
		t.Error("mockResource should implement Lifecycleable")
	}

	if n, ok := r.(traits.Networkable); ok {
		ifaces := n.NetworkInterfaces()
		if len(ifaces) != 1 || ifaces[0].Name != "eth0" {
			t.Error("unexpected network interfaces")
		}
	} else {
		t.Error("mockResource should implement Networkable")
	}

	if f, ok := r.(traits.Firmwareable); ok {
		comps := f.FirmwareComponents()
		if len(comps) != 1 || comps[0].Name != "bios" {
			t.Error("unexpected firmware components")
		}
	} else {
		t.Error("mockResource should implement Firmwareable")
	}

	if h, ok := r.(traits.Healthable); ok {
		if h.HealthStatus() != traits.HealthHealthy {
			t.Errorf("expected Healthy, got %s", h.HealthStatus())
		}
	} else {
		t.Error("mockResource should implement Healthable")
	}
}

func TestPartialTraitImplementation(t *testing.T) {
	type bmcOnly struct{}
	bmc := &bmcOnly{}

	if _, ok := interface{}(bmc).(traits.Provisionable); ok {
		t.Error("bmcOnly should not implement Provisionable")
	}
	if _, ok := interface{}(bmc).(traits.Networkable); ok {
		t.Error("bmcOnly should not implement Networkable")
	}
}
