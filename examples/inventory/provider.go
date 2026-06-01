// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/resource"
)

type InventoryProvider struct {
	db     *gorm.DB
	store  resource.Store[Machine]
	logger zerolog.Logger
}

func New() *InventoryProvider {
	return &InventoryProvider{}
}

func (p *InventoryProvider) Name() string           { return "inventory" }
func (p *InventoryProvider) Version() string        { return "0.1.0" }
func (p *InventoryProvider) Features() []string     { return []string{"inventory"} }
func (p *InventoryProvider) Dependencies() []string { return nil }

func (p *InventoryProvider) Init(ctx provider.Context) error {
	p.db = ctx.DB
	p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()

	p.store = resource.NewGenericStore[Machine](ctx.DB)

	if err := ctx.DB.AutoMigrate(&Machine{}); err != nil {
		return err
	}

	p.logger.Info().Msg("inventory provider initialized")
	return nil
}

func (p *InventoryProvider) Shutdown(_ context.Context) error {
	p.logger.Info().Msg("inventory provider shutdown")
	return nil
}

func (p *InventoryProvider) RegisterRoutes(r chi.Router) {
	r.Route("/machines", func(r chi.Router) {
		r.Get("/", p.listMachines)
		r.Post("/", p.createMachine)
		r.Route("/{name}", func(r chi.Router) {
			r.Get("/", p.getMachine)
			r.Put("/", p.updateMachine)
			r.Delete("/", p.deleteMachine)
		})
	})
}

var _ provider.Provider = (*InventoryProvider)(nil)
var _ provider.APIProvider = (*InventoryProvider)(nil)
