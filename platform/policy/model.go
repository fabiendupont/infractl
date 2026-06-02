// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package policy

import "github.com/fabiendupont/infractl/resource"

// Rule defines a single permission: a subject pattern may perform
// specified actions on specified resource kinds.
type Rule struct {
	Subjects  []string `json:"subjects"`
	Resources []string `json:"resources"`
	Actions   []string `json:"actions"`
	Effect    string   `json:"effect"` // allow, deny
}

type PolicySpec struct {
	Description string `json:"description,omitempty"`
	Rules       []Rule `json:"rules"`
}

type PolicyStatus struct {
	Phase string `json:"phase"`
}

type Policy struct {
	resource.Resource
	Spec   resource.JSONField[PolicySpec]   `gorm:"type:jsonb" json:"spec"`
	Status resource.JSONField[PolicyStatus] `gorm:"type:jsonb" json:"status"`
}

func (Policy) TableName() string { return "policies" }

func (p *Policy) SpecBytes() ([]byte, error) {
	return resource.MarshalSpec(p.Spec.Data)
}
