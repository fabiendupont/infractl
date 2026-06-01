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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var (
	// ErrNotFound is returned when a resource lookup finds no matching row.
	ErrNotFound = errors.New("resource not found")

	// ErrAlreadyExists is returned when a Create would violate the primary key.
	ErrAlreadyExists = errors.New("resource already exists")

	// ErrConflict is returned when an Update fails optimistic concurrency
	// checks (the ResourceVersion in the request doesn't match the DB).
	ErrConflict = errors.New("resource version conflict")
)

// ListOptions controls pagination, filtering, and sorting for List operations.
type ListOptions struct {
	// Limit is the maximum number of items to return. Zero means server default.
	Limit int

	// Continue is an opaque pagination token from a previous List response.
	Continue string

	// Filter is a simple field=value filter expression.
	// CEL-based filtering is planned for a future release.
	Filter string

	// Sort specifies the ordering, e.g. "name asc" or "created_at desc".
	Sort string
}

// List holds a page of results and pagination metadata.
type List[R any] struct {
	Items    []R    `json:"items"`
	Continue string `json:"continue,omitempty"`
	Total    int64  `json:"total"`
}

// Store is the generic persistence interface for domain resources. Every
// query is scoped to an OrgID to enforce tenant isolation at the data layer.
type Store[R any] interface {
	Create(ctx context.Context, resource *R) error
	Get(ctx context.Context, orgID uuid.UUID, name string) (*R, error)
	List(ctx context.Context, orgID uuid.UUID, opts ListOptions) (*List[R], error)
	Update(ctx context.Context, resource *R) error
	Delete(ctx context.Context, orgID uuid.UUID, name string) error
}

// GenericStore is a GORM-backed implementation of Store[R]. It assumes that R
// embeds resource.Resource (or has org_id and name columns as composite PK).
type GenericStore[R any] struct {
	db *gorm.DB
}

// NewGenericStore creates a store backed by the given GORM database handle.
func NewGenericStore[R any](db *gorm.DB) *GenericStore[R] {
	return &GenericStore[R]{db: db}
}

// Create inserts a new resource. Returns ErrAlreadyExists if the composite
// primary key (org_id, name) already exists.
func (s *GenericStore[R]) Create(ctx context.Context, resource *R) error {
	result := s.db.WithContext(ctx).Create(resource)
	if result.Error != nil {
		if isDuplicateKeyError(result.Error) {
			return ErrAlreadyExists
		}
		return fmt.Errorf("creating resource: %w", result.Error)
	}
	return nil
}

// Get retrieves a single resource by org and name. Returns ErrNotFound when
// no matching row exists (soft-deleted rows are excluded by GORM).
func (s *GenericStore[R]) Get(ctx context.Context, orgID uuid.UUID, name string) (*R, error) {
	var resource R
	result := s.db.WithContext(ctx).
		Where("org_id = ? AND name = ?", orgID, name).
		First(&resource)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting resource: %w", result.Error)
	}
	return &resource, nil
}

// List returns a paginated list of resources scoped to the given org.
func (s *GenericStore[R]) List(ctx context.Context, orgID uuid.UUID, opts ListOptions) (*List[R], error) {
	query := s.db.WithContext(ctx).Where("org_id = ?", orgID)

	// Apply simple field=value filter.
	if opts.Filter != "" {
		clause, args, err := ParseFilter(opts.Filter)
		if err != nil {
			return nil, fmt.Errorf("parsing filter: %w", err)
		}
		query = query.Where(clause, args...)
	}

	// Count total before pagination.
	var total int64
	if err := query.Model(new(R)).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("counting resources: %w", err)
	}

	// Apply sorting.
	if opts.Sort != "" {
		query = query.Order(opts.Sort)
	} else {
		query = query.Order("name ASC")
	}

	// Apply pagination.
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	query = query.Limit(limit)

	if opts.Continue != "" {
		token, err := DecodeContinueToken(opts.Continue)
		if err != nil {
			return nil, fmt.Errorf("decoding continue token: %w", err)
		}
		query = query.Offset(token.Offset)
	}

	var items []R
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("listing resources: %w", err)
	}

	// Build continue token if there are more results.
	var continueToken string
	offset := 0
	if opts.Continue != "" {
		token, _ := DecodeContinueToken(opts.Continue)
		offset = token.Offset
	}
	nextOffset := offset + len(items)
	if int64(nextOffset) < total {
		continueToken = EncodeContinueToken(ContinueToken{Offset: nextOffset})
	}

	return &List[R]{
		Items:    items,
		Continue: continueToken,
		Total:    total,
	}, nil
}

// Update persists changes to an existing resource with optimistic concurrency
// control. The resource's ResourceVersion must match the current DB value or
// ErrConflict is returned. Generation is incremented when the spec changes
// (if the resource implements GenerationTracker).
func (s *GenericStore[R]) Update(ctx context.Context, res *R) error {
	accessor, ok := any(res).(ResourceAccessor)
	if !ok {
		result := s.db.WithContext(ctx).Save(res)
		if result.Error != nil {
			return fmt.Errorf("updating resource: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	}

	orgID := accessor.GetOrgID()
	name := accessor.GetName()
	clientVersion := accessor.GetResourceVersion()

	if tracker, ok := any(res).(GenerationTracker); ok {
		var existing R
		if err := s.db.WithContext(ctx).
			Where("org_id = ? AND name = ?", orgID, name).
			First(&existing).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("fetching existing resource: %w", err)
		}
		if oldTracker, ok := any(&existing).(GenerationTracker); ok {
			oldSpec, err1 := oldTracker.SpecBytes()
			newSpec, err2 := tracker.SpecBytes()
			if err1 == nil && err2 == nil && !bytes.Equal(oldSpec, newSpec) {
				accessor.SetGeneration(accessor.GetGeneration() + 1)
			}
		}
	}

	accessor.SetResourceVersion(clientVersion + 1)

	result := s.db.WithContext(ctx).
		Model(res).
		Where("org_id = ? AND name = ? AND resource_version = ?", orgID, name, clientVersion).
		Select("*").
		Updates(res)
	if result.Error != nil {
		return fmt.Errorf("updating resource: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	return nil
}

// marshalSpec is a helper for implementing GenerationTracker on resources
// that use JSONField[T] for their spec.
func MarshalSpec(v any) ([]byte, error) {
	return json.Marshal(v)
}

// isDuplicateKeyError checks whether the error is a PostgreSQL unique violation
// (SQLSTATE 23505). GORM's ErrDuplicatedKey is not reliably returned by the
// pgx driver, so we inspect the underlying pgconn.PgError directly.
func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	return false
}

// Delete soft-deletes a resource by org and name. Returns ErrNotFound if the
// resource does not exist.
func (s *GenericStore[R]) Delete(ctx context.Context, orgID uuid.UUID, name string) error {
	result := s.db.WithContext(ctx).
		Where("org_id = ? AND name = ?", orgID, name).
		Delete(new(R))
	if result.Error != nil {
		return fmt.Errorf("deleting resource: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
