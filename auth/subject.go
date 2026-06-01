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

// Subject represents an authenticated identity and the set of tenants it
// can access. It is placed into request context by the AuthN middleware.
type Subject struct {
	// User is the unique identifier of the authenticated user (e.g. email,
	// service account name, or OIDC subject claim).
	User string

	// Tenants is the set of tenants this subject is authorized to access.
	Tenants TenantSet
}

// TenantSet represents a set of tenant identifiers. A universal set grants
// access to all tenants (used for platform administrators).
type TenantSet struct {
	universal bool
	tenants   map[string]struct{}
}

// NewTenantSet creates a TenantSet containing the given tenant IDs.
func NewTenantSet(ids ...string) TenantSet {
	ts := TenantSet{tenants: make(map[string]struct{}, len(ids))}
	for _, id := range ids {
		ts.tenants[id] = struct{}{}
	}
	return ts
}

// UniversalTenantSet returns a TenantSet that contains all tenants.
func UniversalTenantSet() TenantSet {
	return TenantSet{universal: true}
}

// Contains reports whether the set includes the given tenant ID.
func (ts TenantSet) Contains(id string) bool {
	if ts.universal {
		return true
	}
	_, ok := ts.tenants[id]
	return ok
}

// Add inserts a tenant ID into the set. It is a no-op on a universal set.
func (ts *TenantSet) Add(id string) {
	if ts.universal {
		return
	}
	if ts.tenants == nil {
		ts.tenants = make(map[string]struct{})
	}
	ts.tenants[id] = struct{}{}
}

// Values returns all tenant IDs in the set. Returns nil for a universal set.
func (ts TenantSet) Values() []string {
	if ts.universal {
		return nil
	}
	vals := make([]string, 0, len(ts.tenants))
	for id := range ts.tenants {
		vals = append(vals, id)
	}
	return vals
}

// IsUniversal reports whether this is a universal (all-tenants) set.
func (ts TenantSet) IsUniversal() bool {
	return ts.universal
}

// Len returns the number of tenants in the set, or -1 for a universal set.
func (ts TenantSet) Len() int {
	if ts.universal {
		return -1
	}
	return len(ts.tenants)
}
