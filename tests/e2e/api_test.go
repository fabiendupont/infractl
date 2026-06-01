// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fabiendupont/infractl/api"
	"github.com/fabiendupont/infractl/auth"
	"github.com/fabiendupont/infractl/examples/inventory"
	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/tests/testutil"
)

const (
	defaultTenant   = "00000000-0000-0000-0000-000000000001"
	alternativeTenant = "00000000-0000-0000-0000-000000000002"
	forbiddenTenant = "99999999-9999-9999-9999-999999999999"
)

// setupTestServer starts a PostgreSQL test container, initializes the
// inventory provider, wires up auth middleware, and returns the httptest
// server URL plus a cleanup function.
func setupTestServer(t *testing.T) (string, func()) {
	t.Helper()

	db, _, pgCleanup := testutil.SetupPostgres(t)

	logger := zerolog.Nop()

	reg := provider.NewRegistry()
	require.NoError(t, reg.Register(inventory.New()))
	require.NoError(t, reg.ResolveDependencies())

	hooks := provider.NewHookRunner(reg, logger)

	require.NoError(t, reg.InitAll(provider.Context{
		DB:        db,
		Registry:  reg,
		Hooks:     hooks,
		Logger:    logger,
		APIPrefix: "/api/v1",
	}))

	srv := api.NewServer(api.ServerConfig{Addr: ":0"}, logger)

	// Mount auth middleware on the API subrouter.
	srv.Router.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.AuthN(&auth.GuestAuthenticator{}))
		r.Use(auth.Tenancy(&auth.GuestTenancyLogic{
			DefaultTenant: defaultTenant,
		}))
		r.Use(auth.AuthZ(&auth.AllowAllAuthorizer{}))

		for _, ap := range reg.APIProviders() {
			ap.RegisterRoutes(r)
		}
	})

	ts := httptest.NewServer(srv.Router)

	cleanup := func() {
		ts.Close()
		pgCleanup()
	}

	return ts.URL, cleanup
}

// doRequest is a helper to send HTTP requests with optional body and headers.
func doRequest(t *testing.T, method, url string, body interface{}, headers map[string]string) *http.Response {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reqBody)
	require.NoError(t, err)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

// readBody reads and returns the response body as bytes, closing it.
func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return b
}

func TestHealthz(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	resp := doRequest(t, http.MethodGet, baseURL+"/healthz", nil, nil)
	body := readBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "ok")
}

func TestCreateMachine(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]interface{}{
		"name": "test-machine",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "x86_64",
				"cpus": 4,
			},
		},
	}

	resp := doRequest(t, http.MethodPost, baseURL+"/api/v1/machines", payload, map[string]string{
		"X-Org-ID": defaultTenant,
	})
	body := readBody(t, resp)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "test-machine", result["name"])
	assert.EqualValues(t, 1, result["resource_version"])
	assert.EqualValues(t, 1, result["generation"])
}

func TestGetMachine(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	// Create a machine first.
	payload := map[string]interface{}{
		"name": "test-machine",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "x86_64",
				"cpus": 4,
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/machines", payload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// GET the created machine.
	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/machines/test-machine", nil, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "test-machine", result["name"])
}

func TestUpdateMachine(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	// Create a machine.
	createPayload := map[string]interface{}{
		"name": "test-machine",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "x86_64",
				"cpus": 4,
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/machines", createPayload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// Update the machine with correct resource_version.
	updatePayload := map[string]interface{}{
		"name":             "test-machine",
		"org_id":           defaultTenant,
		"resource_version": 1,
		"generation":       1,
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "x86_64",
				"cpus": 8,
			},
		},
	}
	resp := doRequest(t, http.MethodPut, baseURL+"/api/v1/machines/test-machine", updatePayload, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.EqualValues(t, 2, result["resource_version"])
	assert.EqualValues(t, 2, result["generation"])
}

func TestUpdateMachineStaleResourceVersion(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	// Create a machine.
	createPayload := map[string]interface{}{
		"name": "test-machine",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "x86_64",
				"cpus": 4,
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/machines", createPayload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// First update to bump resource_version to 2.
	updatePayload1 := map[string]interface{}{
		"name":             "test-machine",
		"org_id":           defaultTenant,
		"resource_version": 1,
		"generation":       1,
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "x86_64",
				"cpus": 8,
			},
		},
	}
	firstUpdateResp := doRequest(t, http.MethodPut, baseURL+"/api/v1/machines/test-machine", updatePayload1, headers)
	readBody(t, firstUpdateResp)
	require.Equal(t, http.StatusOK, firstUpdateResp.StatusCode)

	// Second update with stale resource_version=1 (now stale, DB has 2).
	stalePayload := map[string]interface{}{
		"name":             "test-machine",
		"org_id":           defaultTenant,
		"resource_version": 1,
		"generation":       1,
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "x86_64",
				"cpus": 16,
			},
		},
	}
	resp := doRequest(t, http.MethodPut, baseURL+"/api/v1/machines/test-machine", stalePayload, headers)
	body := readBody(t, resp)

	// The inventory handler currently returns 500 for all update errors.
	// This test expects 409 Conflict, which is the correct behavior.
	// If this fails, the updateMachine handler needs to map ErrConflict to 409.
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "body: %s", string(body))
}

func TestDeleteMachine(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	// Create a machine.
	createPayload := map[string]interface{}{
		"name": "test-machine",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "x86_64",
				"cpus": 4,
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/machines", createPayload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// Delete the machine.
	resp := doRequest(t, http.MethodDelete, baseURL+"/api/v1/machines/test-machine", nil, headers)
	readBody(t, resp)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestGetMachineAfterDelete(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	// Create then delete a machine.
	createPayload := map[string]interface{}{
		"name": "test-machine",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "x86_64",
				"cpus": 4,
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/machines", createPayload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	deleteResp := doRequest(t, http.MethodDelete, baseURL+"/api/v1/machines/test-machine", nil, headers)
	readBody(t, deleteResp)
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	// GET after delete should return 404.
	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/machines/test-machine", nil, headers)
	readBody(t, resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestCreateMachineWithoutOrgID(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	// POST without X-Org-ID header -- should use default tenant.
	payload := map[string]interface{}{
		"name": "default-tenant-machine",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "arm64",
				"cpus": 2,
			},
		},
	}
	resp := doRequest(t, http.MethodPost, baseURL+"/api/v1/machines", payload, nil)
	body := readBody(t, resp)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "default-tenant-machine", result["name"])
	assert.Equal(t, defaultTenant, result["org_id"])
}

func TestTenantIsolation(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headersA := map[string]string{"X-Org-ID": defaultTenant}

	// Create a machine in org A (the default tenant).
	createPayload := map[string]interface{}{
		"name": "org-a-machine",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"arch": "x86_64",
				"cpus": 4,
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/machines", createPayload, headersA)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// GET with org B (alternative tenant) should return 403 because the
	// GuestTenancyLogic only allows the default tenant.
	headersB := map[string]string{"X-Org-ID": alternativeTenant}
	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/machines/org-a-machine", nil, headersB)
	readBody(t, resp)

	// GuestTenancyLogic's visible tenants only include the default tenant,
	// so requesting with a different org returns 403 Forbidden.
	// If the tenancy logic allowed both tenants, this would return 404
	// because the machine doesn't exist in org B.
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestForbiddenOrgID(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	// GET with a forbidden (not in guest tenant set) X-Org-ID should return 403.
	headers := map[string]string{"X-Org-ID": forbiddenTenant}
	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/machines/anything", nil, headers)
	readBody(t, resp)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
