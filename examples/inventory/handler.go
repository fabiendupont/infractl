// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/fabiendupont/infractl/auth"
	"github.com/fabiendupont/infractl/resource"
	"github.com/fabiendupont/infractl/resource/mixins"
)

func (p *InventoryProvider) listMachines(w http.ResponseWriter, r *http.Request) {
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

func (p *InventoryProvider) getMachine(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := chi.URLParam(r, "name")

	machine, err := p.store.Get(r.Context(), orgID, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, machine)
}

func (p *InventoryProvider) createMachine(w http.ResponseWriter, r *http.Request) {
	var machine Machine
	if err := json.NewDecoder(r.Body).Decode(&machine); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	machine.OrgID = orgID
	machine.Status = resource.JSONField[MachineStatus]{
		Data: MachineStatus{LifecycleStatus: mixins.LifecycleStatus{Phase: "Pending"}},
	}

	if err := p.store.Create(r.Context(), &machine); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	writeJSON(w, http.StatusCreated, machine)
}

func (p *InventoryProvider) updateMachine(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := chi.URLParam(r, "name")

	existing, err := p.store.Get(r.Context(), orgID, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
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

func (p *InventoryProvider) deleteMachine(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := chi.URLParam(r, "name")

	if err := p.store.Delete(r.Context(), orgID, name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
