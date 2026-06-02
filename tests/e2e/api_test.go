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
	"github.com/fabiendupont/infractl/platform/event"
	"github.com/fabiendupont/infractl/platform/policy"
	"github.com/fabiendupont/infractl/platform/secret"
	"github.com/fabiendupont/infractl/platform/task"
	"github.com/fabiendupont/infractl/platform/tenant"
	"github.com/fabiendupont/infractl/platform/webhook"
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
	require.NoError(t, reg.Register(tenant.New()))
	require.NoError(t, reg.Register(event.New()))
	require.NoError(t, reg.Register(secret.New()))
	require.NoError(t, reg.Register(task.New()))
	require.NoError(t, reg.Register(webhook.New()))
	require.NoError(t, reg.Register(policy.New()))
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

// --- Tenant provider tests ---

func TestCreateTenant(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]interface{}{
		"name": "test-tenant",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"display_name": "Test Tenant",
				"description":  "A test tenant",
			},
		},
	}

	resp := doRequest(t, http.MethodPost, baseURL+"/api/v1/tenants", payload, nil)
	body := readBody(t, resp)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "test-tenant", result["name"])
}

func TestGetTenant(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]interface{}{
		"name": "test-tenant",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"display_name": "Test Tenant",
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/tenants", payload, nil)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/tenants/test-tenant", nil, nil)
	body := readBody(t, resp)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "test-tenant", result["name"])
}

func TestListTenants(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	for _, name := range []string{"tenant-a", "tenant-b"} {
		payload := map[string]interface{}{
			"name": name,
			"spec": map[string]interface{}{
				"data": map[string]interface{}{
					"display_name": name,
				},
			},
		}
		createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/tenants", payload, nil)
		readBody(t, createResp)
		require.Equal(t, http.StatusCreated, createResp.StatusCode)
	}

	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/tenants", nil, nil)
	body := readBody(t, resp)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	items, ok := result["items"].([]interface{})
	require.True(t, ok)
	assert.Len(t, items, 2)
}

func TestDeleteTenant(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	payload := map[string]interface{}{
		"name": "test-tenant",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"display_name": "Test Tenant",
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/tenants", payload, nil)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	resp := doRequest(t, http.MethodDelete, baseURL+"/api/v1/tenants/test-tenant", nil, nil)
	readBody(t, resp)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	getResp := doRequest(t, http.MethodGet, baseURL+"/api/v1/tenants/test-tenant", nil, nil)
	readBody(t, getResp)
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

// --- Secret provider tests ---

func TestCreateSecret(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	payload := map[string]interface{}{
		"name": "test-secret",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"type": "opaque",
				"data": map[string]interface{}{
					"username": "admin",
					"password": "s3cret",
				},
			},
		},
	}

	resp := doRequest(t, http.MethodPost, baseURL+"/api/v1/secrets", payload, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "test-secret", result["name"])

	spec := result["spec"].(map[string]interface{})
	data := spec["data"].(map[string]interface{})
	secretData := data["data"].(map[string]interface{})
	assert.Equal(t, "***", secretData["username"])
	assert.Equal(t, "***", secretData["password"])
}

func TestGetSecretRedacted(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	payload := map[string]interface{}{
		"name": "test-secret",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"type": "opaque",
				"data": map[string]interface{}{
					"api_key": "abc123",
				},
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/secrets", payload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/secrets/test-secret", nil, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	spec := result["spec"].(map[string]interface{})
	data := spec["data"].(map[string]interface{})
	secretData := data["data"].(map[string]interface{})
	assert.Equal(t, "***", secretData["api_key"])
}

func TestRevealSecret(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	payload := map[string]interface{}{
		"name": "test-secret",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"type": "opaque",
				"data": map[string]interface{}{
					"api_key": "abc123",
				},
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/secrets", payload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/secrets/test-secret/reveal", nil, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	spec := result["spec"].(map[string]interface{})
	data := spec["data"].(map[string]interface{})
	secretData := data["data"].(map[string]interface{})
	assert.Equal(t, "abc123", secretData["api_key"])
}

func TestDeleteSecret(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	payload := map[string]interface{}{
		"name": "test-secret",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"type": "opaque",
				"data": map[string]interface{}{
					"key": "value",
				},
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/secrets", payload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	resp := doRequest(t, http.MethodDelete, baseURL+"/api/v1/secrets/test-secret", nil, headers)
	readBody(t, resp)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	getResp := doRequest(t, http.MethodGet, baseURL+"/api/v1/secrets/test-secret", nil, headers)
	readBody(t, getResp)
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

// --- Policy provider tests ---

func TestCreatePolicy(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	payload := map[string]interface{}{
		"name": "test-policy",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"description": "Allow all machines",
				"rules": []map[string]interface{}{
					{
						"subjects":  []string{"*"},
						"resources": []string{"machines"},
						"actions":   []string{"get", "list", "create"},
						"effect":    "allow",
					},
				},
			},
		},
	}

	resp := doRequest(t, http.MethodPost, baseURL+"/api/v1/policies", payload, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "test-policy", result["name"])
}

func TestGetPolicy(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	payload := map[string]interface{}{
		"name": "test-policy",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"description": "Allow all",
				"rules": []map[string]interface{}{
					{
						"subjects":  []string{"*"},
						"resources": []string{"*"},
						"actions":   []string{"*"},
						"effect":    "allow",
					},
				},
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/policies", payload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/policies/test-policy", nil, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "test-policy", result["name"])
}

func TestDeletePolicy(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	payload := map[string]interface{}{
		"name": "test-policy",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"description": "Temp policy",
				"rules":       []map[string]interface{}{},
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/policies", payload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	resp := doRequest(t, http.MethodDelete, baseURL+"/api/v1/policies/test-policy", nil, headers)
	readBody(t, resp)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	getResp := doRequest(t, http.MethodGet, baseURL+"/api/v1/policies/test-policy", nil, headers)
	readBody(t, getResp)
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

// --- Event provider tests ---

func TestListEvents(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/events", nil, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["items"])
}

// --- Task provider tests ---

func TestListTasks(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/tasks", nil, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["items"])
}

// --- Webhook provider tests ---

func TestCreateWebhook(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	payload := map[string]interface{}{
		"name": "test-webhook",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"url":    "https://example.com/hook",
				"events": []string{"created", "updated"},
				"active": true,
			},
		},
	}

	resp := doRequest(t, http.MethodPost, baseURL+"/api/v1/webhooks", payload, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusCreated, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "test-webhook", result["name"])
}

func TestGetWebhook(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	payload := map[string]interface{}{
		"name": "test-webhook",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"url":    "https://example.com/hook",
				"events": []string{"created"},
				"active": true,
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/webhooks", payload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	resp := doRequest(t, http.MethodGet, baseURL+"/api/v1/webhooks/test-webhook", nil, headers)
	body := readBody(t, resp)

	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "test-webhook", result["name"])
}

func TestDeleteWebhook(t *testing.T) {
	baseURL, cleanup := setupTestServer(t)
	defer cleanup()

	headers := map[string]string{"X-Org-ID": defaultTenant}

	payload := map[string]interface{}{
		"name": "test-webhook",
		"spec": map[string]interface{}{
			"data": map[string]interface{}{
				"url":    "https://example.com/hook",
				"events": []string{"created"},
				"active": true,
			},
		},
	}
	createResp := doRequest(t, http.MethodPost, baseURL+"/api/v1/webhooks", payload, headers)
	readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	resp := doRequest(t, http.MethodDelete, baseURL+"/api/v1/webhooks/test-webhook", nil, headers)
	readBody(t, resp)

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	getResp := doRequest(t, http.MethodGet, baseURL+"/api/v1/webhooks/test-webhook", nil, headers)
	readBody(t, getResp)
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}
