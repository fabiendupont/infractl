// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/fabiendupont/infractl/auth"
	"github.com/fabiendupont/infractl/resource"
)

func (p *WebhookProvider) listWebhooks(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}

	list, err := p.store.List(r.Context(), orgID, resource.ListOptions{
		Limit:    limit,
		Continue: r.URL.Query().Get("continue"),
		Filter:   r.URL.Query().Get("filter"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (p *WebhookProvider) getWebhook(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := chi.URLParam(r, "name")

	wh, err := p.store.Get(r.Context(), orgID, name)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "webhook not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, wh)
}

func (p *WebhookProvider) createWebhook(w http.ResponseWriter, r *http.Request) {
	var wh Webhook
	if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wh.OrgID = orgID

	if wh.Spec.Data.URL == "" {
		http.Error(w, "spec.url is required", http.StatusBadRequest)
		return
	}

	wh.Status = resource.JSONField[WebhookStatus]{
		Data: WebhookStatus{Phase: "Active"},
	}

	if err := p.store.Create(r.Context(), &wh); err != nil {
		if errors.Is(err, resource.ErrAlreadyExists) {
			http.Error(w, "webhook already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, wh)
}

func (p *WebhookProvider) updateWebhook(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := chi.URLParam(r, "name")

	existing, err := p.store.Get(r.Context(), orgID, name)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "webhook not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(existing); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := p.store.Update(r.Context(), existing); err != nil {
		if errors.Is(err, resource.ErrConflict) {
			http.Error(w, "resource version conflict", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, existing)
}

func (p *WebhookProvider) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := chi.URLParam(r, "name")

	if err := p.store.Delete(r.Context(), orgID, name); err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "webhook not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
