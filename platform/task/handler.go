// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package task

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/fabiendupont/infractl/auth"
	"github.com/fabiendupont/infractl/work"
)

func (p *TaskProvider) listTasks(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}

	query := p.db.WithContext(r.Context()).
		Where("org_id = ?", orgID).
		Order("created_at DESC").
		Limit(limit)

	if kind := r.URL.Query().Get("kind"); kind != "" {
		query = query.Where("kind = ?", kind)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var records []work.TaskRecord
	if err := query.Find(&records).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": records,
		"total": len(records),
	})
}

func (p *TaskProvider) getTask(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := chi.URLParam(r, "id")

	var record work.TaskRecord
	if err := p.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", id, orgID).
		First(&record).Error; err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, record)
}

func (p *TaskProvider) cancelTask(w http.ResponseWriter, r *http.Request) {
	orgID, err := auth.OrgIDFromContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := chi.URLParam(r, "id")

	result := p.db.WithContext(r.Context()).
		Model(&work.TaskRecord{}).
		Where("id = ? AND org_id = ? AND status IN ?", id, orgID,
			[]string{string(work.TaskPending), string(work.TaskRunning)}).
		Update("status", string(work.TaskFailed))

	if result.Error != nil {
		http.Error(w, result.Error.Error(), http.StatusInternalServerError)
		return
	}
	if result.RowsAffected == 0 {
		http.Error(w, "task not found or already completed", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
