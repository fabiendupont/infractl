// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/api"
	"github.com/fabiendupont/infractl/auth"
	"github.com/fabiendupont/infractl/events"
	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/work"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	dsn := os.Getenv("INFRACTL_DB_DSN")
	if dsn == "" {
		logger.Fatal().Msg("INFRACTL_DB_DSN is required")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}

	registry := provider.NewRegistry()

	profile := provider.GetProfile()
	factories := provider.GetProfileProviders(profile)
	if factories == nil {
		logger.Fatal().Str("profile", profile).Msg("no providers registered for profile")
	}
	for _, factory := range factories {
		if err := registry.Register(factory()); err != nil {
			logger.Fatal().Err(err).Msg("failed to register provider")
		}
	}

	if configPath := os.Getenv("INFRACTL_PROVIDERS_CONFIG"); configPath != "" {
		if err := registry.RegisterExternalProviders(configPath); err != nil {
			logger.Fatal().Err(err).Msg("failed to register external providers")
		}
	}

	bus, err := events.NewPgBus(db, dsn, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create event bus")
	}

	queue, err := work.NewPgQueue(db, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create task queue")
	}

	hooks := provider.NewHookRunner(registry, logger)
	provCtx := provider.Context{
		DB:        db,
		Registry:  registry,
		Hooks:     hooks,
		Logger:    logger,
		APIPrefix: "/api/v1",
		Bus:       bus,
		Queue:     queue,
	}

	if err := registry.InitAll(provCtx); err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize providers")
	}

	serverCfg := api.ServerConfig{
		Addr:    os.Getenv("INFRACTL_ADDR"),
		TLSCert: os.Getenv("INFRACTL_TLS_CERT"),
		TLSKey:  os.Getenv("INFRACTL_TLS_KEY"),
	}
	srv := api.NewServer(serverCfg, logger)

	srv.Router.Group(func(r chi.Router) {
		r.Use(auth.AuthN(&auth.GuestAuthenticator{}))
		r.Use(auth.Tenancy(&auth.GuestTenancyLogic{
			DefaultTenant: "00000000-0000-0000-0000-000000000001",
		}))
		r.Use(auth.AuthZ(&auth.AllowAllAuthorizer{}))

		r.Route("/api/v1", func(r chi.Router) {
			for _, ap := range registry.APIProviders() {
				ap.RegisterRoutes(r)
			}

			r.Get("/capabilities", capabilitiesHandler(registry))
		})
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	bus.StartCleanup(ctx, 7*24*time.Hour, 1*time.Hour)
	queue.StartRecovery(ctx, 15*time.Minute, 1*time.Minute)

	httpSrv := srv.HTTPServer()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	<-ctx.Done()
	logger.Info().Msg("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("HTTP server shutdown error")
	}

	if err := registry.ShutdownAll(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("provider shutdown error")
	}

	bus.Close()
}

func capabilitiesHandler(registry *provider.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		caps := make(map[string]string)
		for _, ap := range registry.APIProviders() {
			p := ap.(provider.Provider)
			for _, f := range p.Features() {
				caps[f] = p.Name()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(caps)
	}
}
