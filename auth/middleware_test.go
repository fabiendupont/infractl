package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTenantUUID = "11111111-1111-1111-1111-111111111111"

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestTenancy_ValidOrgIDHeader(t *testing.T) {
	logic := &GuestTenancyLogic{DefaultTenant: testTenantUUID}
	mw := Tenancy(logic)

	var captured string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := OrgIDFromContext(r.Context())
		require.NoError(t, err)
		captured = orgID.String()
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Org-ID", testTenantUUID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, testTenantUUID, captured)
}

func TestTenancy_NoHeader_UsesDefault(t *testing.T) {
	logic := &GuestTenancyLogic{DefaultTenant: testTenantUUID}
	mw := Tenancy(logic)

	var captured string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := OrgIDFromContext(r.Context())
		require.NoError(t, err)
		captured = orgID.String()
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, testTenantUUID, captured)
}

func TestTenancy_ForbiddenTenant(t *testing.T) {
	// Use DefaultTenancyLogic which reads Subject from context.
	// Set up a Subject that only has access to testTenantUUID.
	logic := &DefaultTenancyLogic{}

	// Wrap with AuthN that provides a subject with limited tenants.
	authn := &GuestAuthenticator{
		GuestUser:   "testuser",
		GuestTenant: testTenantUUID,
	}

	chain := AuthN(authn)(Tenancy(logic)(newTestHandler(t)))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Org-ID", "22222222-2222-2222-2222-222222222222")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestTenancy_InvalidUUID(t *testing.T) {
	logic := &GuestTenancyLogic{DefaultTenant: testTenantUUID}
	mw := Tenancy(logic)

	handler := mw(newTestHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Org-ID", "not-a-uuid")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestOrgIDFromContext_BareContext(t *testing.T) {
	_, err := OrgIDFromContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no org ID in context")
}
