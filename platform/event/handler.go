// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/fabiendupont/infractl/auth"
	"github.com/fabiendupont/infractl/events"
)

func (p *EventProvider) listEvents(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	opts := events.ListOptions{
		Limit:  limit,
		Offset: offset,
		Kind:   r.URL.Query().Get("kind"),
		Action: r.URL.Query().Get("action"),
	}

	evts, err := p.store.List(r.Context(), orgID, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": evts,
		"total": len(evts),
	})
}

func (p *EventProvider) getEvent(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := chi.URLParam(r, "id")

	evts, err := p.store.List(r.Context(), orgID, events.ListOptions{Limit: 1})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, e := range evts {
		if e.ID == id {
			writeJSON(w, http.StatusOK, e)
			return
		}
	}

	http.Error(w, "event not found", http.StatusNotFound)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
