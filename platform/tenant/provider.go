// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package tenant

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/resource"
)

type TenantProvider struct {
	db     *gorm.DB
	store  resource.Store[Tenant]
	logger zerolog.Logger
}

func New() *TenantProvider {
	return &TenantProvider{}
}

func (p *TenantProvider) Name() string           { return "tenant" }
func (p *TenantProvider) Version() string        { return "0.1.0" }
func (p *TenantProvider) Features() []string     { return []string{"tenant"} }
func (p *TenantProvider) Dependencies() []string { return nil }

func (p *TenantProvider) Init(ctx provider.Context) error {
	p.db = ctx.DB
	p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()
	p.store = resource.NewGenericStore[Tenant](ctx.DB)

	if err := ctx.DB.AutoMigrate(&Tenant{}); err != nil {
		return err
	}

	p.logger.Info().Msg("tenant provider initialized")
	return nil
}

func (p *TenantProvider) Shutdown(_ context.Context) error {
	p.logger.Info().Msg("tenant provider shutdown")
	return nil
}

func (p *TenantProvider) RegisterRoutes(r chi.Router) {
	r.Route("/tenants", func(r chi.Router) {
		r.Get("/", p.listTenants)
		r.Post("/", p.createTenant)
		r.Route("/{name}", func(r chi.Router) {
			r.Get("/", p.getTenant)
			r.Put("/", p.updateTenant)
			r.Delete("/", p.deleteTenant)
		})
	})
}

var _ provider.Provider = (*TenantProvider)(nil)
var _ provider.APIProvider = (*TenantProvider)(nil)
