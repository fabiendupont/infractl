// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/work"
)

type TaskProvider struct {
	db     *gorm.DB
	logger zerolog.Logger
}

func New() *TaskProvider {
	return &TaskProvider{}
}

func (p *TaskProvider) Name() string           { return "task" }
func (p *TaskProvider) Version() string        { return "0.1.0" }
func (p *TaskProvider) Features() []string     { return []string{"task"} }
func (p *TaskProvider) Dependencies() []string { return nil }

func (p *TaskProvider) Init(ctx provider.Context) error {
	p.db = ctx.DB
	p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()

	if err := ctx.DB.AutoMigrate(&work.TaskRecord{}); err != nil {
		return err
	}

	p.logger.Info().Msg("task provider initialized")
	return nil
}

func (p *TaskProvider) Shutdown(_ context.Context) error {
	p.logger.Info().Msg("task provider shutdown")
	return nil
}

func (p *TaskProvider) RegisterRoutes(r chi.Router) {
	r.Route("/tasks", func(r chi.Router) {
		r.Get("/", p.listTasks)
		r.Get("/{id}", p.getTask)
		r.Post("/{id}/cancel", p.cancelTask)
	})
}

var _ provider.Provider = (*TaskProvider)(nil)
var _ provider.APIProvider = (*TaskProvider)(nil)
