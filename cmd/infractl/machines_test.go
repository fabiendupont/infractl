// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMachinesList(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, "/api/v1/machines") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(machineListResponse{
			Items: []machineResource{
				{Name: "web-01", Spec: machineSpec{Arch: "x86_64", CPUs: 4, MemoryMB: 8192}, Status: machineStatus{Phase: "Ready"}},
				{Name: "web-02", Spec: machineSpec{Arch: "arm64", CPUs: 8, MemoryMB: 16384}, Status: machineStatus{Phase: "Pending"}},
			},
			Total: 2,
		})
	}))
	defer ts.Close()

	server = ts.URL
	orgID = "00000000-0000-0000-0000-000000000001"
	output = "table"

	cmd := newMachinesListCmd()
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	if !strings.Contains(out, "web-01") {
		t.Error("output missing web-01")
	}
	if !strings.Contains(out, "web-02") {
		t.Error("output missing web-02")
	}
	if !strings.Contains(out, "NAME") {
		t.Error("output missing table header")
	}
}

func TestMachinesListJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"name":"web-01"}],"total":1}`))
	}))
	defer ts.Close()

	server = ts.URL
	orgID = "00000000-0000-0000-0000-000000000001"
	output = "json"

	cmd := newMachinesListCmd()
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	if !strings.Contains(out, `"web-01"`) {
		t.Errorf("JSON output missing web-01: %s", out)
	}
}

func TestMachinesGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/machines/test-machine" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(machineResource{
			Name:  "test-machine",
			OrgID: "00000000-0000-0000-0000-000000000001",
			Spec:  machineSpec{Arch: "x86_64", CPUs: 4, MemoryMB: 8192},
		})
	}))
	defer ts.Close()

	server = ts.URL
	orgID = "00000000-0000-0000-0000-000000000001"
	output = "table"

	cmd := newMachinesGetCmd()
	cmd.SetArgs([]string{"test-machine"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	if !strings.Contains(out, "test-machine") {
		t.Error("output missing test-machine")
	}
	if !strings.Contains(out, "x86_64") {
		t.Error("output missing arch")
	}
}

func TestMachinesCreate(t *testing.T) {
	var receivedBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"new-machine"}`))
	}))
	defer ts.Close()

	server = ts.URL
	orgID = "00000000-0000-0000-0000-000000000001"
	output = "table"

	tmpFile, err := os.CreateTemp("", "machine-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(`{"name":"new-machine","spec":{"arch":"x86_64","cpus":4}}`)
	tmpFile.Close()

	cmd := newMachinesCreateCmd()
	cmd.SetArgs([]string{"-f", tmpFile.Name()})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	if !strings.Contains(out, "new-machine") {
		t.Errorf("output missing new-machine: %s", out)
	}
	if len(receivedBody) == 0 {
		t.Error("server received empty body")
	}
}

func TestMachinesDelete(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/machines/doomed" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	server = ts.URL
	orgID = "00000000-0000-0000-0000-000000000001"
	output = "table"

	cmd := newMachinesDeleteCmd()
	cmd.SetArgs([]string{"doomed"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})

	if !strings.Contains(out, "doomed") {
		t.Errorf("output missing doomed: %s", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}
