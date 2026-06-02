// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package tenant

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/fabiendupont/infractl/resource"
)

func (p *TenantProvider) listTenants(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}

	list, err := p.store.List(r.Context(), resource.SystemTenantID, resource.ListOptions{
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

func (p *TenantProvider) getTenant(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	t, err := p.store.Get(r.Context(), resource.SystemTenantID, name)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "tenant not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, t)
}

func (p *TenantProvider) createTenant(w http.ResponseWriter, r *http.Request) {
	var t Tenant
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	t.OrgID = resource.SystemTenantID
	t.Status = resource.JSONField[TenantStatus]{
		Data: TenantStatus{Phase: "Pending"},
	}

	if err := p.store.Create(r.Context(), &t); err != nil {
		if errors.Is(err, resource.ErrAlreadyExists) {
			http.Error(w, "tenant already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, t)
}

func (p *TenantProvider) updateTenant(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	existing, err := p.store.Get(r.Context(), resource.SystemTenantID, name)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "tenant not found", http.StatusNotFound)
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

func (p *TenantProvider) deleteTenant(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	if err := p.store.Delete(r.Context(), resource.SystemTenantID, name); err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "tenant not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, resource.ErrFinalizersPending) {
			w.WriteHeader(http.StatusAccepted)
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
