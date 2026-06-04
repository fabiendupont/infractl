// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package organization

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/fabiendupont/infractl/resource"
)

func (p *OrganizationProvider) listOrganizations(w http.ResponseWriter, r *http.Request) {
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

func (p *OrganizationProvider) getOrganization(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	org, err := p.store.Get(r.Context(), resource.SystemTenantID, name)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "organization not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, org)
}

func (p *OrganizationProvider) createOrganization(w http.ResponseWriter, r *http.Request) {
	var org Organization
	if err := json.NewDecoder(r.Body).Decode(&org); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	org.OrgID = resource.SystemTenantID
	org.Status = resource.JSONField[OrganizationStatus]{
		Data: OrganizationStatus{Phase: "Pending"},
	}

	if err := p.store.Create(r.Context(), &org); err != nil {
		if errors.Is(err, resource.ErrAlreadyExists) {
			http.Error(w, "organization already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if p.hooks != nil {
		p.hooks.FireAsync(r.Context(), "Organization", "post_create", &org)
	}

	writeJSON(w, http.StatusCreated, org)
}

func (p *OrganizationProvider) updateOrganization(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	existing, err := p.store.Get(r.Context(), resource.SystemTenantID, name)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "organization not found", http.StatusNotFound)
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

func (p *OrganizationProvider) deleteOrganization(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	if err := p.store.Delete(r.Context(), resource.SystemTenantID, name); err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "organization not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, resource.ErrFinalizersPending) {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if p.hooks != nil {
		p.hooks.FireAsync(r.Context(), "Organization", "post_delete", map[string]interface{}{
			"org_id": resource.SystemTenantID.String(), "name": name,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
