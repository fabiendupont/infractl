// Copyright 2025 The infractl Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resource

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ResourceAccessor provides typed access to the base Resource fields from a
// generic R in GenericStore. The Resource struct satisfies this interface.
type ResourceAccessor interface {
	GetOrgID() uuid.UUID
	GetName() string
	GetResourceVersion() int64
	SetResourceVersion(int64)
	GetGeneration() int64
	SetGeneration(int64)
}

// GenerationTracker is an optional interface that domain resources can
// implement to enable automatic Generation bumps when spec content changes.
// Return the raw JSON bytes of the spec field.
type GenerationTracker interface {
	SpecBytes() ([]byte, error)
}

// Resource is the base model embedded by all domain resources. It provides
// multi-tenant scoping via OrgID, optimistic concurrency via Generation and
// ResourceVersion, and soft-delete support via DeletedAt.
type Resource struct {
	OrgID           uuid.UUID      `gorm:"primaryKey;type:uuid" json:"org_id"`
	Name            string         `gorm:"primaryKey" json:"name"`
	Labels          JSONMap        `gorm:"type:jsonb" json:"labels,omitempty"`
	Annotations     JSONMap        `gorm:"type:jsonb" json:"annotations,omitempty"`
	Generation      int64          `json:"generation"`
	ResourceVersion int64          `json:"resource_version"`
	Owner           *string        `json:"owner,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// BeforeCreate bumps Generation and ResourceVersion on initial insert.
func (r *Resource) BeforeCreate(_ *gorm.DB) error {
	if r.Generation == 0 {
		r.Generation = 1
	}
	if r.ResourceVersion == 0 {
		r.ResourceVersion = 1
	}
	return nil
}

// BeforeUpdate is a no-op hook. ResourceVersion is managed by GenericStore
// to support optimistic concurrency control.
func (r *Resource) BeforeUpdate(_ *gorm.DB) error {
	return nil
}

func (r *Resource) GetOrgID() uuid.UUID        { return r.OrgID }
func (r *Resource) GetName() string             { return r.Name }
func (r *Resource) GetResourceVersion() int64   { return r.ResourceVersion }
func (r *Resource) SetResourceVersion(v int64)  { r.ResourceVersion = v }
func (r *Resource) GetGeneration() int64        { return r.Generation }
func (r *Resource) SetGeneration(v int64)       { r.Generation = v }

// JSONMap stores a string-to-string map as PostgreSQL JSONB.
type JSONMap map[string]string

// Value implements driver.Valuer for GORM/database serialization.
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshaling JSONMap: %w", err)
	}
	return b, nil
}

// Scan implements sql.Scanner for GORM/database deserialization.
func (m *JSONMap) Scan(src interface{}) error {
	if src == nil {
		*m = nil
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("JSONMap.Scan: unsupported type %T", src)
	}
	return json.Unmarshal(data, m)
}

// JSONField wraps an arbitrary struct so it can be stored as PostgreSQL JSONB.
// The type parameter T must be JSON-serializable.
type JSONField[T any] struct {
	Data T
}

// Value implements driver.Valuer for GORM/database serialization.
func (f JSONField[T]) Value() (driver.Value, error) {
	b, err := json.Marshal(f.Data)
	if err != nil {
		return nil, fmt.Errorf("marshaling JSONField: %w", err)
	}
	return b, nil
}

// Scan implements sql.Scanner for GORM/database deserialization.
func (f *JSONField[T]) Scan(src interface{}) error {
	if src == nil {
		var zero T
		f.Data = zero
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("JSONField.Scan: unsupported type %T", src)
	}
	return json.Unmarshal(data, &f.Data)
}
