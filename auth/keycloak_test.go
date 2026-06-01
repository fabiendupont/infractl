package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustGenerateRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func signRS256(signingInput string, key *rsa.PrivateKey) (string, error) {
	h := sha256.New()
	h.Write([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h.Sum(nil))
	if err != nil {
		return "", err
	}
	return base64URLEncode(sig), nil
}

func buildJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]interface{}) string {
	t.Helper()
	header := map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid}
	headerJSON, err := json.Marshal(header)
	require.NoError(t, err)
	claimsJSON, err := json.Marshal(claims)
	require.NoError(t, err)

	signingInput := base64URLEncode(headerJSON) + "." + base64URLEncode(claimsJSON)
	sig, err := signRS256(signingInput, key)
	require.NoError(t, err)
	return signingInput + "." + sig
}

func jwksJSON(t *testing.T, kid string, pub *rsa.PublicKey) []byte {
	t.Helper()
	resp := map[string]interface{}{
		"keys": []map[string]string{
			{
				"kid": kid,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64URLEncode(pub.N.Bytes()),
				"e":   base64URLEncode(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	return b
}

func newTestKeycloak(t *testing.T, key *rsa.PrivateKey, kid string) (*KeycloakAuthenticator, string) {
	t.Helper()
	jwks := jwksJSON(t, kid, &key.PublicKey)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/protocol/openid-connect/certs" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(jwks)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	kc := NewKeycloakAuthenticator(KeycloakConfig{
		RealmURL:     srv.URL,
		JWKSCacheTTL: 5 * time.Minute,
	})
	return kc, srv.URL
}

func validClaims(realmURL string) map[string]interface{} {
	now := time.Now().Unix()
	return map[string]interface{}{
		"iss":                realmURL,
		"exp":                float64(now + 3600),
		"iat":                float64(now - 60),
		"sub":                "user-uuid-123",
		"preferred_username": "testuser",
		"org_id":             "tenant-1",
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		wantErr string
	}{
		{"valid", "Bearer abc123", ""},
		{"missing header", "", "missing Authorization header"},
		{"wrong scheme", "Basic abc123", "must use Bearer scheme"},
		{"empty token", "Bearer ", "empty Bearer token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			token, err := extractBearerToken(req)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "abc123", token)
			}
		})
	}
}

func TestParseRSAPublicKey(t *testing.T) {
	key := mustGenerateRSAKey(t)
	nB64 := base64URLEncode(key.PublicKey.N.Bytes())
	eB64 := base64URLEncode(big.NewInt(int64(key.PublicKey.E)).Bytes())

	pub, err := parseRSAPublicKey(nB64, eB64)
	require.NoError(t, err)
	assert.Equal(t, key.PublicKey.N, pub.N)
	assert.Equal(t, key.PublicKey.E, pub.E)
}

func TestJWKSFetchAndCache(t *testing.T) {
	kid := "test-kid-1"
	key := mustGenerateRSAKey(t)
	kc, _ := newTestKeycloak(t, key, kid)

	// First fetch populates cache.
	pub, err := kc.getSigningKey(kid)
	require.NoError(t, err)
	assert.Equal(t, key.PublicKey.N, pub.N)

	// Second call uses cache (no network).
	pub2, err := kc.getSigningKey(kid)
	require.NoError(t, err)
	assert.Equal(t, pub, pub2)
}

func TestJWKSCacheExpiry(t *testing.T) {
	kid := "test-kid-2"
	key := mustGenerateRSAKey(t)
	kc, _ := newTestKeycloak(t, key, kid)
	kc.config.JWKSCacheTTL = 1 * time.Millisecond

	// Populate cache.
	_, err := kc.getSigningKey(kid)
	require.NoError(t, err)

	// Wait for expiry.
	time.Sleep(5 * time.Millisecond)

	// Should refetch.
	pub, err := kc.getSigningKey(kid)
	require.NoError(t, err)
	assert.Equal(t, key.PublicKey.N, pub.N)
}

func TestAuthenticate_ValidToken(t *testing.T) {
	kid := "kid-valid"
	key := mustGenerateRSAKey(t)
	kc, realmURL := newTestKeycloak(t, key, kid)

	claims := validClaims(realmURL)
	token := buildJWT(t, key, kid, claims)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	subject, err := kc.Authenticate(req)
	require.NoError(t, err)
	assert.Equal(t, "testuser", subject.User)
	assert.True(t, subject.Tenants.Contains("tenant-1"))
}

func TestAuthenticate_ExpiredToken(t *testing.T) {
	kid := "kid-expired"
	key := mustGenerateRSAKey(t)
	kc, realmURL := newTestKeycloak(t, key, kid)

	claims := validClaims(realmURL)
	claims["exp"] = float64(time.Now().Unix() - 60)
	token := buildJWT(t, key, kid, claims)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := kc.Authenticate(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token expired")
}

func TestAuthenticate_WrongIssuer(t *testing.T) {
	kid := "kid-issuer"
	key := mustGenerateRSAKey(t)
	kc, _ := newTestKeycloak(t, key, kid)

	claims := validClaims("https://wrong-issuer.example.com")
	token := buildJWT(t, key, kid, claims)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := kc.Authenticate(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid issuer")
}

func TestAuthenticate_AudienceValidation(t *testing.T) {
	kid := "kid-aud"
	key := mustGenerateRSAKey(t)
	kc, realmURL := newTestKeycloak(t, key, kid)
	kc.config.ClientID = "my-client"

	t.Run("valid audience string", func(t *testing.T) {
		claims := validClaims(realmURL)
		claims["aud"] = "my-client"
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		subject, err := kc.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, "testuser", subject.User)
	})

	t.Run("valid audience array", func(t *testing.T) {
		claims := validClaims(realmURL)
		claims["aud"] = []string{"other-client", "my-client"}
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		subject, err := kc.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, "testuser", subject.User)
	})

	t.Run("wrong audience", func(t *testing.T) {
		claims := validClaims(realmURL)
		claims["aud"] = "wrong-client"
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		_, err := kc.Authenticate(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid audience")
	})

	t.Run("missing audience", func(t *testing.T) {
		claims := validClaims(realmURL)
		delete(claims, "aud")
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		_, err := kc.Authenticate(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing aud claim")
	})
}

func TestAuthenticate_UserExtraction(t *testing.T) {
	kid := "kid-user"
	key := mustGenerateRSAKey(t)
	kc, realmURL := newTestKeycloak(t, key, kid)

	t.Run("preferred_username", func(t *testing.T) {
		claims := validClaims(realmURL)
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		subject, err := kc.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, "testuser", subject.User)
	})

	t.Run("falls back to sub", func(t *testing.T) {
		claims := validClaims(realmURL)
		delete(claims, "preferred_username")
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		subject, err := kc.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, "user-uuid-123", subject.User)
	})

	t.Run("custom user claim", func(t *testing.T) {
		kc2, _ := newTestKeycloak(t, key, kid)
		kc2.config.UserClaim = "email"

		claims := validClaims(kc2.config.RealmURL)
		claims["email"] = "test@example.com"
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		subject, err := kc2.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, "test@example.com", subject.User)
	})

	t.Run("no user identity", func(t *testing.T) {
		claims := validClaims(realmURL)
		delete(claims, "preferred_username")
		delete(claims, "sub")
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		_, err := kc.Authenticate(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no user identity")
	})
}

func TestAuthenticate_TenantExtraction(t *testing.T) {
	kid := "kid-tenant"
	key := mustGenerateRSAKey(t)
	kc, realmURL := newTestKeycloak(t, key, kid)

	t.Run("single string tenant", func(t *testing.T) {
		claims := validClaims(realmURL)
		claims["org_id"] = "tenant-a"
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		subject, err := kc.Authenticate(req)
		require.NoError(t, err)
		assert.True(t, subject.Tenants.Contains("tenant-a"))
		assert.Equal(t, 1, subject.Tenants.Len())
	})

	t.Run("array of tenants", func(t *testing.T) {
		claims := validClaims(realmURL)
		claims["org_id"] = []string{"tenant-a", "tenant-b"}
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		subject, err := kc.Authenticate(req)
		require.NoError(t, err)
		assert.True(t, subject.Tenants.Contains("tenant-a"))
		assert.True(t, subject.Tenants.Contains("tenant-b"))
		assert.Equal(t, 2, subject.Tenants.Len())
	})

	t.Run("missing tenant claim", func(t *testing.T) {
		claims := validClaims(realmURL)
		delete(claims, "org_id")
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		_, err := kc.Authenticate(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tenant claim")
	})

	t.Run("empty string tenant", func(t *testing.T) {
		claims := validClaims(realmURL)
		claims["org_id"] = ""
		token := buildJWT(t, key, kid, claims)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		_, err := kc.Authenticate(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})
}

func TestAuthenticate_InvalidSignature(t *testing.T) {
	kid := "kid-sig"
	key := mustGenerateRSAKey(t)
	kc, realmURL := newTestKeycloak(t, key, kid)

	// Sign with a different key.
	wrongKey := mustGenerateRSAKey(t)
	claims := validClaims(realmURL)
	token := buildJWT(t, wrongKey, kid, claims)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := kc.Authenticate(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature verification failed")
}

func TestAuthenticate_MalformedToken(t *testing.T) {
	kid := "kid-malformed"
	key := mustGenerateRSAKey(t)
	kc, _ := newTestKeycloak(t, key, kid)

	tests := []struct {
		name  string
		token string
	}{
		{"not enough parts", "abc.def"},
		{"too many parts", "a.b.c.d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)

			_, err := kc.Authenticate(req)
			require.Error(t, err)
		})
	}
}

func TestAuthenticate_UnknownKid(t *testing.T) {
	kid := "kid-known"
	key := mustGenerateRSAKey(t)
	kc, realmURL := newTestKeycloak(t, key, kid)

	claims := validClaims(realmURL)
	token := buildJWT(t, key, "kid-unknown", claims)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := kc.Authenticate(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signing key")
}

func TestNewKeycloakAuthenticator_Defaults(t *testing.T) {
	kc := NewKeycloakAuthenticator(KeycloakConfig{
		RealmURL: "https://keycloak.example.com/realms/test/",
	})
	assert.Equal(t, "org_id", kc.config.TenantClaim)
	assert.Equal(t, "preferred_username", kc.config.UserClaim)
	assert.Equal(t, 15*time.Minute, kc.config.JWKSCacheTTL)
	assert.Equal(t, "https://keycloak.example.com/realms/test", kc.config.RealmURL)
}

func TestAuthenticate_JWKSEndpointError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	kc := NewKeycloakAuthenticator(KeycloakConfig{RealmURL: srv.URL})

	key := mustGenerateRSAKey(t)
	claims := validClaims(srv.URL)
	token := buildJWT(t, key, "some-kid", claims)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err := kc.Authenticate(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestAuthenticate_NoAuthorizationHeader(t *testing.T) {
	kid := "kid-noauth"
	key := mustGenerateRSAKey(t)
	kc, _ := newTestKeycloak(t, key, kid)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, err := kc.Authenticate(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing Authorization header")
}

func TestAuthenticate_WithAuthNMiddleware(t *testing.T) {
	kid := "kid-mw"
	key := mustGenerateRSAKey(t)
	kc, realmURL := newTestKeycloak(t, key, kid)

	claims := validClaims(realmURL)
	token := buildJWT(t, key, kid, claims)

	var captured *Subject
	handler := AuthN(kc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub, err := SubjectFromContext(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		captured = sub
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, captured)
	assert.Equal(t, "testuser", captured.User)
	assert.True(t, captured.Tenants.Contains("tenant-1"))
}

func TestAuthenticate_MiddlewareUnauthorized(t *testing.T) {
	kid := "kid-mw-unauth"
	key := mustGenerateRSAKey(t)
	kc, _ := newTestKeycloak(t, key, kid)

	handler := AuthN(kc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
