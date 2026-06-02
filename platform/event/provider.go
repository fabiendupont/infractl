// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/events"
	"github.com/fabiendupont/infractl/provider"
)

type EventProvider struct {
	store  events.Store
	logger zerolog.Logger
}

func New() *EventProvider {
	return &EventProvider{}
}

func (p *EventProvider) Name() string           { return "event" }
func (p *EventProvider) Version() string        { return "0.1.0" }
func (p *EventProvider) Features() []string     { return []string{"event"} }
func (p *EventProvider) Dependencies() []string { return nil }

func (p *EventProvider) Init(ctx provider.Context) error {
	p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()

	if ctx.DB != nil {
		if err := ctx.DB.AutoMigrate(&events.EventRecord{}); err != nil {
			return err
		}
		p.store = events.NewPgStore(ctx.DB)
	} else {
		p.store = events.NewInMemoryStore()
	}

	p.logger.Info().Msg("event provider initialized")
	return nil
}

func (p *EventProvider) Shutdown(_ context.Context) error {
	p.logger.Info().Msg("event provider shutdown")
	return nil
}

func (p *EventProvider) RegisterRoutes(r chi.Router) {
	r.Route("/events", func(r chi.Router) {
		r.Get("/", p.listEvents)
		r.Get("/{id}", p.getEvent)
	})
}

// Store returns the underlying event store for use by other providers
// that need to query events programmatically.
func (p *EventProvider) Store() events.Store {
	return p.store
}

// SetStore allows injecting a custom event store (for testing or when
// the bus already maintains a store).
func (p *EventProvider) SetStore(s events.Store, _ *gorm.DB) {
	p.store = s
}

var _ provider.Provider = (*EventProvider)(nil)
var _ provider.APIProvider = (*EventProvider)(nil)
