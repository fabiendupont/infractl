// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

func TestMetricsEndpoint(t *testing.T) {
	logger := zerolog.Nop()
	srv := NewServer(ServerConfig{}, logger)

	// Hit healthz to generate a metric.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz returned %d, want 200", rec.Code)
	}

	// Scrape the metrics endpoint.
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rec, req)

	body := rec.Body.String()

	if !strings.Contains(body, "infractl_http_requests_total") {
		t.Error("metrics output missing infractl_http_requests_total")
	}
	if !strings.Contains(body, "infractl_http_request_duration_seconds") {
		t.Error("metrics output missing infractl_http_request_duration_seconds")
	}
}
