// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package organization

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/auth"
	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/resource"
)

type OrganizationProvider struct {
	db       *gorm.DB
	store    resource.Store[Organization]
	idp      auth.IdentityProvider
	hooks    provider.HookFirer
	logger   zerolog.Logger
}

func New() *OrganizationProvider {
	return &OrganizationProvider{}
}

func (p *OrganizationProvider) Name() string           { return "organization" }
func (p *OrganizationProvider) Version() string        { return "0.1.0" }
func (p *OrganizationProvider) Features() []string     { return []string{"organization"} }
func (p *OrganizationProvider) Dependencies() []string { return []string{"tenant"} }

func (p *OrganizationProvider) Init(ctx provider.Context) error {
	p.db = ctx.DB
	p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()
	p.store = resource.NewGenericStore[Organization](ctx.DB)

	p.idp = ctx.IdentityProvider
	if p.idp == nil {
		p.idp = &auth.NoOpIdentityProvider{}
	}
	if ctx.Hooks != nil {
		p.hooks = ctx.Hooks
	}

	if err := ctx.DB.AutoMigrate(&Organization{}); err != nil {
		return err
	}

	// Register IDP sync hooks.
	if ctx.Registry != nil {
		ctx.Registry.RegisterReaction(provider.Reaction{
			Feature:  "Organization",
			Event:    "post_create",
			Callback: p.onPostCreate,
		})
		ctx.Registry.RegisterReaction(provider.Reaction{
			Feature:  "Organization",
			Event:    "post_delete",
			Callback: p.onPostDelete,
		})
	}

	p.logger.Info().Msg("organization provider initialized")
	return nil
}

func (p *OrganizationProvider) Shutdown(_ context.Context) error {
	p.logger.Info().Msg("organization provider shutdown")
	return nil
}

func (p *OrganizationProvider) RegisterRoutes(r chi.Router) {
	r.Route("/organizations", func(r chi.Router) {
		r.Get("/", p.listOrganizations)
		r.Post("/", p.createOrganization)
		r.Route("/{name}", func(r chi.Router) {
			r.Get("/", p.getOrganization)
			r.Put("/", p.updateOrganization)
			r.Delete("/", p.deleteOrganization)
		})
	})
}

// onPostCreate provisions the organization in the identity provider
// and updates the status to Active or Failed.
func (p *OrganizationProvider) onPostCreate(ctx context.Context, payload interface{}) {
	org, ok := payload.(*Organization)
	if !ok {
		return
	}

	displayName := org.Spec.Data.DisplayName
	if displayName == "" {
		displayName = org.Name
	}

	result, err := p.idp.ProvisionOrganization(ctx, org.Name, displayName)
	if err != nil {
		p.logger.Error().Err(err).Str("org", org.Name).Msg("IDP provisioning failed")
		p.updateStatus(ctx, org, "Failed", err.Error(), "")
		return
	}

	p.updateStatus(ctx, org, "Active", "", result.ExternalID)
	p.logger.Info().Str("org", org.Name).Str("external_id", result.ExternalID).Msg("organization provisioned in IDP")
}

// onPostDelete deprovisions the organization from the identity provider.
func (p *OrganizationProvider) onPostDelete(ctx context.Context, payload interface{}) {
	if m, ok := payload.(map[string]interface{}); ok {
		if name, ok := m["name"].(string); ok {
			if err := p.idp.DeprovisionOrganization(ctx, name); err != nil {
				p.logger.Error().Err(err).Str("org", name).Msg("IDP deprovisioning failed")
			}
		}
	}
}

func (p *OrganizationProvider) updateStatus(ctx context.Context, org *Organization, phase, message, externalID string) {
	p.db.WithContext(ctx).
		Table("organizations").
		Where("org_id = ? AND name = ? AND deleted_at IS NULL", org.OrgID, org.Name).
		Updates(map[string]interface{}{
			"status": resource.JSONField[OrganizationStatus]{Data: OrganizationStatus{
				Phase:      phase,
				Message:    message,
				ExternalID: externalID,
			}},
		})
}

var _ provider.Provider = (*OrganizationProvider)(nil)
var _ provider.APIProvider = (*OrganizationProvider)(nil)
