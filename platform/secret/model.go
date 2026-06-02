// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import "github.com/fabiendupont/infractl/resource"

const (
	TypeOpaque = "opaque"
	TypeTLS    = "tls"
	TypeToken  = "token"
)

type SecretSpec struct {
	Type string            `json:"type"`
	Data map[string]string `json:"data,omitempty"`
}

type SecretStatus struct {
	Phase string `json:"phase"`
}

type Secret struct {
	resource.Resource
	Spec   resource.JSONField[SecretSpec]   `gorm:"type:jsonb" json:"spec"`
	Status resource.JSONField[SecretStatus] `gorm:"type:jsonb" json:"status"`
}

func (Secret) TableName() string { return "secrets" }

func (s *Secret) SpecBytes() ([]byte, error) {
	return resource.MarshalSpec(s.Spec.Data)
}

// Redacted returns a copy of the secret with data keys preserved but
// values replaced with "***". Used in list and get responses.
func (s *Secret) Redacted() Secret {
	redacted := *s
	if s.Spec.Data.Data != nil {
		keys := make(map[string]string, len(s.Spec.Data.Data))
		for k := range s.Spec.Data.Data {
			keys[k] = "***"
		}
		redacted.Spec.Data.Data = keys
	}
	return redacted
}
