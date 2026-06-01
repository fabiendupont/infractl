// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"net/http"
	"sync"
)

// SpecRegistry collects OpenAPI spec fragments from providers and merges
// them into a single specification served at GET /openapi.json.
type SpecRegistry struct {
	mu        sync.RWMutex
	fragments map[string][]byte
}

// NewSpecRegistry creates an empty spec registry.
func NewSpecRegistry() *SpecRegistry {
	return &SpecRegistry{
		fragments: make(map[string][]byte),
	}
}

// Register adds an OpenAPI fragment contributed by a provider. The fragment
// is raw YAML or JSON bytes from the provider's openapi.yaml file.
func (r *SpecRegistry) Register(providerName string, fragment []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fragments[providerName] = fragment
}

// Handler returns an HTTP handler that serves the list of registered
// providers and their spec availability at GET /openapi.json.
func (r *SpecRegistry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		r.mu.RLock()
		defer r.mu.RUnlock()

		providers := make(map[string]int)
		for name, frag := range r.fragments {
			providers[name] = len(frag)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"providers": providers,
		})
	}
}
