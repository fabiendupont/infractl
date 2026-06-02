// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/resource"
)

type PolicyProvider struct {
	db     *gorm.DB
	store  resource.Store[Policy]
	logger zerolog.Logger
}

func New() *PolicyProvider {
	return &PolicyProvider{}
}

func (p *PolicyProvider) Name() string           { return "policy" }
func (p *PolicyProvider) Version() string        { return "0.1.0" }
func (p *PolicyProvider) Features() []string     { return []string{"policy"} }
func (p *PolicyProvider) Dependencies() []string { return nil }

func (p *PolicyProvider) Init(ctx provider.Context) error {
	p.db = ctx.DB
	p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()
	p.store = resource.NewGenericStore[Policy](ctx.DB)

	if err := ctx.DB.AutoMigrate(&Policy{}); err != nil {
		return err
	}

	p.logger.Info().Msg("policy provider initialized")
	return nil
}

func (p *PolicyProvider) Shutdown(_ context.Context) error {
	p.logger.Info().Msg("policy provider shutdown")
	return nil
}

func (p *PolicyProvider) RegisterRoutes(r chi.Router) {
	r.Route("/policies", func(r chi.Router) {
		r.Get("/", p.listPolicies)
		r.Post("/", p.createPolicy)
		r.Route("/{name}", func(r chi.Router) {
			r.Get("/", p.getPolicy)
			r.Put("/", p.updatePolicy)
			r.Delete("/", p.deletePolicy)
		})
	})
}

var _ provider.Provider = (*PolicyProvider)(nil)
var _ provider.APIProvider = (*PolicyProvider)(nil)
