// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCapabilities(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"inventory":"inventory","compute":"cloud"}`))
	}))
	defer ts.Close()

	server = ts.URL
	output = "table"

	cmd := newCapabilitiesCmd()
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	if !strings.Contains(out, "FEATURE") {
		t.Error("output missing table header")
	}
	if !strings.Contains(out, "inventory") {
		t.Error("output missing inventory feature")
	}
	if !strings.Contains(out, "compute") {
		t.Error("output missing compute feature")
	}
}
