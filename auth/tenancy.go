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

package auth

import (
	"context"
	"fmt"
)

// TenancyLogic determines the tenant sets available to a subject for
// different operations. Assignable tenants are those a subject may write to,
// default tenants are used when no explicit tenant is specified, and visible
// tenants are those a subject may read.
type TenancyLogic interface {
	// DetermineAssignableTenants returns the tenants the current subject may
	// create or modify resources in.
	DetermineAssignableTenants(ctx context.Context) (TenantSet, error)

	// DetermineDefaultTenants returns the tenants used when the caller does
	// not specify an explicit tenant scope.
	DetermineDefaultTenants(ctx context.Context) (TenantSet, error)

	// DetermineVisibleTenants returns the tenants whose resources the current
	// subject may list and read.
	DetermineVisibleTenants(ctx context.Context) (TenantSet, error)
}

// DefaultTenancyLogic reads the subject from context and returns its tenant
// set for all three operations. This is the standard implementation for
// authenticated users.
type DefaultTenancyLogic struct{}

func (d *DefaultTenancyLogic) DetermineAssignableTenants(ctx context.Context) (TenantSet, error) {
	sub, err := SubjectFromContext(ctx)
	if err != nil {
		return TenantSet{}, err
	}
	return sub.Tenants, nil
}

func (d *DefaultTenancyLogic) DetermineDefaultTenants(ctx context.Context) (TenantSet, error) {
	sub, err := SubjectFromContext(ctx)
	if err != nil {
		return TenantSet{}, err
	}
	return sub.Tenants, nil
}

func (d *DefaultTenancyLogic) DetermineVisibleTenants(ctx context.Context) (TenantSet, error) {
	sub, err := SubjectFromContext(ctx)
	if err != nil {
		return TenantSet{}, err
	}
	return sub.Tenants, nil
}

// GuestTenancyLogic always returns a fixed single-tenant set. It is used
// for unauthenticated or guest access where all operations are scoped to a
// single default tenant.
type GuestTenancyLogic struct {
	DefaultTenant string
}

func (g *GuestTenancyLogic) DetermineAssignableTenants(_ context.Context) (TenantSet, error) {
	if g.DefaultTenant == "" {
		return TenantSet{}, fmt.Errorf("guest tenancy: no default tenant configured")
	}
	return NewTenantSet(g.DefaultTenant), nil
}

func (g *GuestTenancyLogic) DetermineDefaultTenants(_ context.Context) (TenantSet, error) {
	if g.DefaultTenant == "" {
		return TenantSet{}, fmt.Errorf("guest tenancy: no default tenant configured")
	}
	return NewTenantSet(g.DefaultTenant), nil
}

func (g *GuestTenancyLogic) DetermineVisibleTenants(_ context.Context) (TenantSet, error) {
	if g.DefaultTenant == "" {
		return TenantSet{}, fmt.Errorf("guest tenancy: no default tenant configured")
	}
	return NewTenantSet(g.DefaultTenant), nil
}
