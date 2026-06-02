// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/resource"
)

type SecretProvider struct {
	db     *gorm.DB
	store  resource.Store[Secret]
	logger zerolog.Logger
}

func New() *SecretProvider {
	return &SecretProvider{}
}

func (p *SecretProvider) Name() string           { return "secret" }
func (p *SecretProvider) Version() string        { return "0.1.0" }
func (p *SecretProvider) Features() []string     { return []string{"secret"} }
func (p *SecretProvider) Dependencies() []string { return nil }

func (p *SecretProvider) Init(ctx provider.Context) error {
	p.db = ctx.DB
	p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()
	p.store = resource.NewGenericStore[Secret](ctx.DB)

	if err := ctx.DB.AutoMigrate(&Secret{}); err != nil {
		return err
	}

	p.logger.Info().Msg("secret provider initialized")
	return nil
}

func (p *SecretProvider) Shutdown(_ context.Context) error {
	p.logger.Info().Msg("secret provider shutdown")
	return nil
}

func (p *SecretProvider) RegisterRoutes(r chi.Router) {
	r.Route("/secrets", func(r chi.Router) {
		r.Get("/", p.listSecrets)
		r.Post("/", p.createSecret)
		r.Route("/{name}", func(r chi.Router) {
			r.Get("/", p.getSecret)
			r.Put("/", p.updateSecret)
			r.Delete("/", p.deleteSecret)
			r.Get("/reveal", p.revealSecret)
		})
	})
}

var _ provider.Provider = (*SecretProvider)(nil)
var _ provider.APIProvider = (*SecretProvider)(nil)
