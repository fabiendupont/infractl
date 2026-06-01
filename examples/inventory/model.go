// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"github.com/fabiendupont/infractl/resource"
	"github.com/fabiendupont/infractl/resource/mixins"
	"github.com/fabiendupont/infractl/resource/traits"
)

type MachineSpec struct {
	mixins.BMCSpec
	Arch     string `json:"arch,omitempty"`
	CPUs     int    `json:"cpus,omitempty"`
	MemoryMB int    `json:"memory_mb,omitempty"`
	DiskGB   int    `json:"disk_gb,omitempty"`
}

type MachineStatus struct {
	mixins.BMCStatus
	mixins.LifecycleStatus
	Message string `json:"message,omitempty"`
}

type Machine struct {
	resource.Resource
	Spec   resource.JSONField[MachineSpec]   `gorm:"type:jsonb" json:"spec"`
	Status resource.JSONField[MachineStatus] `gorm:"type:jsonb" json:"status"`
}

func (Machine) TableName() string { return "machines" }

func (m *Machine) SpecBytes() ([]byte, error) {
	return resource.MarshalSpec(m.Spec.Data)
}

func (m *Machine) BMCAddress() string {
	return m.Spec.Data.BMCAddr
}

func (m *Machine) PowerState() traits.PowerState {
	switch m.Status.Data.PowerState {
	case "on":
		return traits.PowerOn
	case "off":
		return traits.PowerOff
	default:
		return traits.PowerUnknown
	}
}

func (m *Machine) Phase() traits.Phase {
	return traits.Phase(m.Status.Data.LifecycleStatus.Phase)
}

func (m *Machine) Conditions() []traits.Condition {
	var out []traits.Condition
	for _, c := range m.Status.Data.LifecycleStatus.Conditions {
		out = append(out, traits.Condition{
			Type:    c.Type,
			Status:  c.Status,
			Reason:  c.Reason,
			Message: c.Message,
		})
	}
	return out
}

var _ traits.Provisionable = (*Machine)(nil)
var _ traits.Lifecycleable = (*Machine)(nil)
