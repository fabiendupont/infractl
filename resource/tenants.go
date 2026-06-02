// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import "github.com/google/uuid"

// Well-known tenant UUIDs used by the framework.
var (
	// SystemTenantID scopes internal framework resources (tenants,
	// policies, system configuration). Not visible to regular users.
	SystemTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000000")

	// SharedTenantID scopes platform-provided resources visible to all
	// users in read-only mode (e.g., base images, network classes).
	SharedTenantID = uuid.MustParse("00000000-0000-0000-0000-ffffffffffff")
)
