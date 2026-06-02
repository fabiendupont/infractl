// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package webhook

import "github.com/fabiendupont/infractl/resource"

type WebhookSpec struct {
	URL     string   `json:"url"`
	Secret  string   `json:"secret,omitempty"`
	Events  []string `json:"events"`
	Kinds   []string `json:"kinds,omitempty"`
	Active  bool     `json:"active"`
}

type WebhookStatus struct {
	Phase          string `json:"phase"`
	LastDeliveryAt string `json:"last_delivery_at,omitempty"`
	LastStatusCode int    `json:"last_status_code,omitempty"`
	FailureCount   int    `json:"failure_count"`
}

type Webhook struct {
	resource.Resource
	Spec   resource.JSONField[WebhookSpec]   `gorm:"type:jsonb" json:"spec"`
	Status resource.JSONField[WebhookStatus] `gorm:"type:jsonb" json:"status"`
}

func (Webhook) TableName() string { return "webhooks" }

func (w *Webhook) SpecBytes() ([]byte, error) {
	return resource.MarshalSpec(w.Spec.Data)
}
