// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import "context"

// OrgProvisionResult holds the output of provisioning an organization
// in an identity provider.
type OrgProvisionResult struct {
	ExternalID   string
	AdminUserID  string
	AdminSecret  string // only populated on initial creation, not stored
}

// IdentityProvider manages organizations in an external identity system.
// Implementations handle the specifics of Keycloak, Dex, Okta, etc.
// A no-op implementation is provided for development.
type IdentityProvider interface {
	ProvisionOrganization(ctx context.Context, orgName, displayName string) (*OrgProvisionResult, error)
	DeprovisionOrganization(ctx context.Context, orgName string) error
}

// NoOpIdentityProvider is a development-only implementation that does
// nothing. Use when no external identity system is configured.
type NoOpIdentityProvider struct{}

func (p *NoOpIdentityProvider) ProvisionOrganization(_ context.Context, orgName, _ string) (*OrgProvisionResult, error) {
	return &OrgProvisionResult{ExternalID: orgName}, nil
}

func (p *NoOpIdentityProvider) DeprovisionOrganization(_ context.Context, _ string) error {
	return nil
}
