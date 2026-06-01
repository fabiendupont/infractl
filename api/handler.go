// Copyright 2025 The infractl Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/fabiendupont/infractl/auth"
	"github.com/fabiendupont/infractl/resource"
)

// RegisterCRUDRoutes registers standard REST endpoints for a resource type R
// under the given base path (resourceName). The routes are:
//
//	GET    /{resourceName}         List
//	GET    /{resourceName}/{name}  Get
//	POST   /{resourceName}         Create
//	PUT    /{resourceName}/{name}  Update
//	DELETE /{resourceName}/{name}  Delete
func RegisterCRUDRoutes[R any](router chi.Router, store resource.Store[R], resourceName string) {
	h := &crudHandler[R]{
		store:        store,
		resourceName: resourceName,
	}

	router.Route("/"+resourceName, func(r chi.Router) {
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/{name}", h.get)
		r.Put("/{name}", h.update)
		r.Delete("/{name}", h.delete)
	})
}

type crudHandler[R any] struct {
	store        resource.Store[R]
	resourceName string
}

func (h *crudHandler[R]) list(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	opts := resource.ListOptions{
		Filter:   r.URL.Query().Get("filter"),
		Sort:     r.URL.Query().Get("sort"),
		Continue: r.URL.Query().Get("continue"),
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit parameter")
			return
		}
		opts.Limit = limit
	}

	list, err := h.store.List(r.Context(), orgID, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (h *crudHandler[R]) get(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	res, err := h.store.Get(r.Context(), orgID, name)
	if err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			writeError(w, http.StatusNotFound, h.resourceName+" not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (h *crudHandler[R]) create(w http.ResponseWriter, r *http.Request) {
	var res R
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if err := h.store.Create(r.Context(), &res); err != nil {
		if errors.Is(err, resource.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, h.resourceName+" already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, res)
}

func (h *crudHandler[R]) update(w http.ResponseWriter, r *http.Request) {
	var res R
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if err := h.store.Update(r.Context(), &res); err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			writeError(w, http.StatusNotFound, h.resourceName+" not found")
			return
		}
		if errors.Is(err, resource.ErrConflict) {
			writeError(w, http.StatusConflict, "resource version conflict")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (h *crudHandler[R]) delete(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := h.store.Delete(r.Context(), orgID, name); err != nil {
		if errors.Is(err, resource.ErrNotFound) {
			writeError(w, http.StatusNotFound, h.resourceName+" not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeJSON serializes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
