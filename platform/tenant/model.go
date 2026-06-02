// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package tenant

import "github.com/fabiendupont/infractl/resource"

type TenantSpec struct {
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
}

type TenantStatus struct {
	Phase   string `json:"phase"`
	Message string `json:"message,omitempty"`
}

type Tenant struct {
	resource.Resource
	Spec   resource.JSONField[TenantSpec]   `gorm:"type:jsonb" json:"spec"`
	Status resource.JSONField[TenantStatus] `gorm:"type:jsonb" json:"status"`
}

func (Tenant) TableName() string { return "tenants" }

func (t *Tenant) SpecBytes() ([]byte, error) {
	return resource.MarshalSpec(t.Spec.Data)
}
