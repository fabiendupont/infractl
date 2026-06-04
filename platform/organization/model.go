// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package organization

import "github.com/fabiendupont/infractl/resource"

type OrganizationSpec struct {
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
}

type OrganizationStatus struct {
	Phase      string `json:"phase"` // Pending, Active, Failed, Deleting
	Message    string `json:"message,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
}

type Organization struct {
	resource.Resource
	Spec   resource.JSONField[OrganizationSpec]   `gorm:"type:jsonb" json:"spec"`
	Status resource.JSONField[OrganizationStatus] `gorm:"type:jsonb" json:"status"`
}

func (Organization) TableName() string { return "organizations" }

func (o *Organization) SpecBytes() ([]byte, error) {
	return resource.MarshalSpec(o.Spec.Data)
}
