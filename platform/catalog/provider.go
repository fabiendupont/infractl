// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/resource"
)

type CatalogProvider struct {
	db     *gorm.DB
	store  resource.Store[CatalogItem]
	logger zerolog.Logger
}

func New() *CatalogProvider {
	return &CatalogProvider{}
}

func (p *CatalogProvider) Name() string           { return "catalog" }
func (p *CatalogProvider) Version() string        { return "0.1.0" }
func (p *CatalogProvider) Features() []string     { return []string{"catalog"} }
func (p *CatalogProvider) Dependencies() []string { return nil }

func (p *CatalogProvider) Init(ctx provider.Context) error {
	p.db = ctx.DB
	p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()
	p.store = resource.NewGenericStore[CatalogItem](ctx.DB)

	if err := ctx.DB.AutoMigrate(&CatalogItem{}); err != nil {
		return err
	}

	p.logger.Info().Msg("catalog provider initialized")
	return nil
}

func (p *CatalogProvider) Shutdown(_ context.Context) error {
	p.logger.Info().Msg("catalog provider shutdown")
	return nil
}

func (p *CatalogProvider) RegisterRoutes(r chi.Router) {
	r.Route("/catalog-items", func(r chi.Router) {
		r.Get("/", p.listCatalogItems)
		r.Post("/", p.createCatalogItem)
		r.Route("/{name}", func(r chi.Router) {
			r.Get("/", p.getCatalogItem)
			r.Put("/", p.updateCatalogItem)
			r.Delete("/", p.deleteCatalogItem)
		})
	})
}

var _ provider.Provider = (*CatalogProvider)(nil)
var _ provider.APIProvider = (*CatalogProvider)(nil)
