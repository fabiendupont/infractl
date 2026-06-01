// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package traits

type Phase string

const (
	PhasePending      Phase = "Pending"
	PhaseProvisioning Phase = "Provisioning"
	PhaseReady        Phase = "Ready"
	PhaseError        Phase = "Error"
	PhaseDeleting     Phase = "Deleting"
)

type Condition struct {
	Type    string
	Status  string // "True", "False", "Unknown"
	Reason  string
	Message string
}

type Lifecycleable interface {
	Phase() Phase
	Conditions() []Condition
}
