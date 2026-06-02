// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	// ErrParentNotFound is returned when a resource references a parent
	// that does not exist.
	ErrParentNotFound = errors.New("parent resource not found")

	// ErrCircularParent is returned when setting a parent would create
	// a circular dependency in the resource hierarchy.
	ErrCircularParent = errors.New("circular parent reference")

	// ErrHasChildren is returned when deleting a resource that has
	// child resources referencing it as parent.
	ErrHasChildren = errors.New("resource has children")
)

// ValidateParent checks that a parent reference is valid: the parent
// exists in the same org and table, and assigning it would not create
// a cycle. The model parameter is a zero-value instance of the resource
// type (used to determine the table name).
func ValidateParent(ctx context.Context, db *gorm.DB, orgID uuid.UUID, name string, parentName string, model interface{}) error {
	if parentName == name {
		return ErrCircularParent
	}

	var count int64
	if err := db.WithContext(ctx).
		Model(model).
		Where("org_id = ? AND name = ?", orgID, parentName).
		Count(&count).Error; err != nil {
		return fmt.Errorf("checking parent existence: %w", err)
	}
	if count == 0 {
		return ErrParentNotFound
	}

	// Walk the ancestor chain to detect cycles.
	visited := map[string]bool{name: true}
	current := parentName
	for current != "" {
		if visited[current] {
			return ErrCircularParent
		}
		visited[current] = true

		var parent *string
		err := db.WithContext(ctx).
			Model(model).
			Select("parent").
			Where("org_id = ? AND name = ?", orgID, current).
			Scan(&parent).Error
		if err != nil {
			return fmt.Errorf("walking ancestor chain: %w", err)
		}
		if parent == nil {
			break
		}
		current = *parent
	}

	return nil
}

// HasChildren checks whether any resource in the same org and table
// references the given name as its parent.
func HasChildren(ctx context.Context, db *gorm.DB, orgID uuid.UUID, name string, model interface{}) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).
		Model(model).
		Where("org_id = ? AND parent = ?", orgID, name).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("checking children: %w", err)
	}
	return count > 0, nil
}

// ListChildren returns the names of all direct children of the given
// resource in the same org and table.
func ListChildren(ctx context.Context, db *gorm.DB, orgID uuid.UUID, name string, model interface{}) ([]string, error) {
	var names []string
	if err := db.WithContext(ctx).
		Model(model).
		Select("name").
		Where("org_id = ? AND parent = ?", orgID, name).
		Scan(&names).Error; err != nil {
		return nil, fmt.Errorf("listing children: %w", err)
	}
	return names, nil
}
