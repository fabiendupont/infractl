// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/fabiendupont/infractl/auth"
	"github.com/fabiendupont/infractl/resource"
)

func (p *CatalogProvider) listCatalogItems(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}

	// Filter to published items unless ?all=true is specified.
	filter := r.URL.Query().Get("filter")
	if r.URL.Query().Get("all") != "true" && filter == "" {
		filter = "published=true"
	}

	list, err := p.store.List(r.Context(), orgID, resource.ListOptions{
		Limit:    limit,
		Continue: r.URL.Query().Get("continue"),
		Filter:   filter,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (p *CatalogProvider) getCatalogItem(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := chi.URLParam(r, "name")

	item, err := p.store.Get(r.Context(), orgID, name)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "catalog item not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (p *CatalogProvider) createCatalogItem(w http.ResponseWriter, r *http.Request) {
	var item CatalogItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	item.OrgID = orgID
	item.Status = resource.JSONField[CatalogItemStatus]{
		Data: CatalogItemStatus{Phase: "Active"},
	}

	if err := p.store.Create(r.Context(), &item); err != nil {
		if errors.Is(err, resource.ErrAlreadyExists) {
			http.Error(w, "catalog item already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, item)
}

func (p *CatalogProvider) updateCatalogItem(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := chi.URLParam(r, "name")

	existing, err := p.store.Get(r.Context(), orgID, name)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "catalog item not found", http.StatusNotFound)
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

func (p *CatalogProvider) deleteCatalogItem(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := chi.URLParam(r, "name")

	if err := p.store.Delete(r.Context(), orgID, name); err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			http.Error(w, "catalog item not found", http.StatusNotFound)
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
