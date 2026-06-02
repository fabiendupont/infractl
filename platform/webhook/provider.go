// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/events"
	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/resource"
)

type WebhookProvider struct {
	db     *gorm.DB
	store  resource.Store[Webhook]
	bus    events.Bus
	logger zerolog.Logger
}

func New() *WebhookProvider {
	return &WebhookProvider{}
}

func (p *WebhookProvider) Name() string           { return "webhook" }
func (p *WebhookProvider) Version() string        { return "0.1.0" }
func (p *WebhookProvider) Features() []string     { return []string{"webhook"} }
func (p *WebhookProvider) Dependencies() []string { return nil }

func (p *WebhookProvider) Init(ctx provider.Context) error {
	p.db = ctx.DB
	p.bus = ctx.Bus
	p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()
	p.store = resource.NewGenericStore[Webhook](ctx.DB)

	if err := ctx.DB.AutoMigrate(&Webhook{}); err != nil {
		return err
	}

	if p.bus != nil {
		go p.startDeliveryLoop(ctx.Logger)
	}

	p.logger.Info().Msg("webhook provider initialized")
	return nil
}

func (p *WebhookProvider) Shutdown(_ context.Context) error {
	p.logger.Info().Msg("webhook provider shutdown")
	return nil
}

func (p *WebhookProvider) RegisterRoutes(r chi.Router) {
	r.Route("/webhooks", func(r chi.Router) {
		r.Get("/", p.listWebhooks)
		r.Post("/", p.createWebhook)
		r.Route("/{name}", func(r chi.Router) {
			r.Get("/", p.getWebhook)
			r.Put("/", p.updateWebhook)
			r.Delete("/", p.deleteWebhook)
		})
	})
}

// startDeliveryLoop subscribes to all events and delivers matching ones
// to registered webhooks.
func (p *WebhookProvider) startDeliveryLoop(logger zerolog.Logger) {
	ctx := context.Background()
	ch, err := p.bus.Subscribe(ctx, nil)
	if err != nil {
		logger.Error().Err(err).Msg("webhook: failed to subscribe to event bus")
		return
	}

	for evt := range ch {
		p.deliverEvent(ctx, evt)
	}
}

func (p *WebhookProvider) deliverEvent(ctx context.Context, evt events.Event) {
	var webhooks []Webhook
	if err := p.db.WithContext(ctx).
		Where("org_id = ?", evt.OrgID).
		Find(&webhooks).Error; err != nil {
		p.logger.Error().Err(err).Msg("failed to query webhooks")
		return
	}

	for _, wh := range webhooks {
		if !wh.Spec.Data.Active {
			continue
		}
		if !matchesFilter(wh.Spec.Data, evt) {
			continue
		}

		go p.deliver(ctx, &wh, evt)
	}
}

func matchesFilter(spec WebhookSpec, evt events.Event) bool {
	if len(spec.Events) > 0 {
		found := false
		for _, e := range spec.Events {
			if e == evt.Action || e == "*" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(spec.Kinds) > 0 {
		found := false
		for _, k := range spec.Kinds {
			if k == evt.Kind || k == "*" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (p *WebhookProvider) deliver(_ context.Context, wh *Webhook, evt events.Event) {
	body, err := json.Marshal(evt)
	if err != nil {
		p.logger.Error().Err(err).Str("webhook", wh.Name).Msg("failed to marshal event")
		return
	}

	req, err := http.NewRequest("POST", wh.Spec.Data.URL, io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		p.logger.Error().Err(err).Str("webhook", wh.Name).Msg("failed to create request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)

	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
		resp.Body.Close()
	}

	p.db.Model(&Webhook{}).
		Where("org_id = ? AND name = ?", wh.OrgID, wh.Name).
		Updates(map[string]interface{}{
			"status": resource.JSONField[WebhookStatus]{Data: WebhookStatus{
				Phase:          "Active",
				LastDeliveryAt: time.Now().Format(time.RFC3339),
				LastStatusCode: statusCode,
				FailureCount:   failureCount(wh.Status.Data.FailureCount, err, statusCode),
			}},
		})

	if err != nil {
		p.logger.Warn().Err(err).Str("webhook", wh.Name).Msg("delivery failed")
	}
}

func failureCount(current int, err error, statusCode int) int {
	if err != nil || statusCode >= 400 {
		return current + 1
	}
	return 0
}

var _ provider.Provider = (*WebhookProvider)(nil)
var _ provider.APIProvider = (*WebhookProvider)(nil)
