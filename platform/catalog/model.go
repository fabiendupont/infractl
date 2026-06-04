// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package catalog

import "github.com/fabiendupont/infractl/resource"

// FieldDefinition constrains a field in the resource spec. Locked fields
// have a fixed value set by the platform admin; unlocked fields can be
// changed by the tenant within optional validation rules.
type FieldDefinition struct {
	Field   string      `json:"field"`
	Locked  bool        `json:"locked"`
	Default interface{} `json:"default,omitempty"`
	Value   interface{} `json:"value,omitempty"`
}

// ProvisioningMetadata binds a CatalogItem to an Ansible collection
// and ResourceAction.
type ProvisioningMetadata struct {
	Collection     string `json:"collection"`
	ResourceAction string `json:"resource_action"`
}

// CatalogItemSpec defines the tenant-facing offering.
type CatalogItemSpec struct {
	Title            string               `json:"title"`
	Description      string               `json:"description,omitempty"`
	ResourceType     string               `json:"resource_type"`
	Published        bool                 `json:"published"`
	Provisioning     ProvisioningMetadata `json:"provisioning"`
	FieldDefinitions []FieldDefinition    `json:"field_definitions,omitempty"`
}

// CatalogItemStatus tracks the catalog item state.
type CatalogItemStatus struct {
	Phase string `json:"phase"` // Active, Deprecated
}

// CatalogItem binds a tenant-facing offering to provisioning logic.
// It defines what tenants see (title, field constraints, published state)
// and how resources are provisioned (collection + resource_action).
type CatalogItem struct {
	resource.Resource
	Spec   resource.JSONField[CatalogItemSpec]   `gorm:"type:jsonb" json:"spec"`
	Status resource.JSONField[CatalogItemStatus] `gorm:"type:jsonb" json:"status"`
}

func (CatalogItem) TableName() string { return "catalog_items" }

func (c *CatalogItem) SpecBytes() ([]byte, error) {
	return resource.MarshalSpec(c.Spec.Data)
}
