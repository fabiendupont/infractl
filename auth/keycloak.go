// Copyright 2025 The infractl Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// KeycloakConfig holds configuration for the Keycloak JWT authenticator.
type KeycloakConfig struct {
	// RealmURL is the Keycloak realm base URL,
	// e.g. "https://keycloak.example.com/realms/infractl".
	RealmURL string

	// ClientID is the expected audience claim. If empty, audience
	// validation is skipped.
	ClientID string

	// TenantClaim is the JWT claim containing tenant IDs.
	// Defaults to "org_id".
	TenantClaim string

	// UserClaim is the JWT claim containing the user identifier.
	// Defaults to "preferred_username", falls back to "sub".
	UserClaim string

	// JWKSCacheTTL is how long to cache JWKS keys.
	// Defaults to 15 minutes.
	JWKSCacheTTL time.Duration
}

type jwksCache struct {
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

// KeycloakAuthenticator validates JWT Bearer tokens issued by a Keycloak
// realm. It fetches and caches the realm's JWKS signing keys.
type KeycloakAuthenticator struct {
	config     KeycloakConfig
	mu         sync.RWMutex
	jwks       *jwksCache
	httpClient *http.Client
}

// NewKeycloakAuthenticator creates a KeycloakAuthenticator with the given
// config. It applies defaults for unset optional fields.
func NewKeycloakAuthenticator(config KeycloakConfig) *KeycloakAuthenticator {
	if config.TenantClaim == "" {
		config.TenantClaim = "org_id"
	}
	if config.UserClaim == "" {
		config.UserClaim = "preferred_username"
	}
	if config.JWKSCacheTTL == 0 {
		config.JWKSCacheTTL = 15 * time.Minute
	}
	config.RealmURL = strings.TrimRight(config.RealmURL, "/")
	return &KeycloakAuthenticator{
		config:     config,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Authenticate extracts and validates a JWT Bearer token from the request's
// Authorization header and returns the authenticated Subject.
func (k *KeycloakAuthenticator) Authenticate(r *http.Request) (*Subject, error) {
	token, err := extractBearerToken(r)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed JWT: expected 3 parts")
	}

	// Decode header to get kid.
	headerJSON, err := base64URLDecode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("malformed JWT header: %w", err)
	}
	var header struct {
		Kid string `json:"kid"`
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("malformed JWT header: %w", err)
	}
	if header.Kid == "" {
		return nil, errors.New("JWT header missing kid")
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported signing algorithm: %s", header.Alg)
	}

	// Look up signing key.
	key, err := k.getSigningKey(header.Kid)
	if err != nil {
		return nil, fmt.Errorf("signing key lookup: %w", err)
	}

	// Verify signature.
	if err := verifyRS256(parts[0]+"."+parts[1], parts[2], key); err != nil {
		return nil, fmt.Errorf("token signature verification failed: %w", err)
	}

	// Decode and validate claims.
	claimsJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("malformed JWT payload: %w", err)
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("malformed JWT payload: %w", err)
	}

	if err := k.validateStandardClaims(claims); err != nil {
		return nil, err
	}

	user, err := k.extractUser(claims)
	if err != nil {
		return nil, err
	}

	tenants, err := k.extractTenants(claims)
	if err != nil {
		return nil, err
	}

	return &Subject{
		User:    user,
		Tenants: NewTenantSet(tenants...),
	}, nil
}

func extractBearerToken(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", errors.New("missing Authorization header")
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return "", errors.New("Authorization header must use Bearer scheme")
	}
	token := auth[len(prefix):]
	if token == "" {
		return "", errors.New("empty Bearer token")
	}
	return token, nil
}

func (k *KeycloakAuthenticator) validateStandardClaims(claims map[string]interface{}) error {
	now := time.Now().Unix()

	if exp, ok := claims["exp"].(float64); ok {
		if int64(exp) < now {
			return errors.New("token expired")
		}
	} else {
		return errors.New("token missing exp claim")
	}

	if iat, ok := claims["iat"].(float64); ok {
		if int64(iat) > now+60 {
			return errors.New("token issued in the future")
		}
	}

	if iss, ok := claims["iss"].(string); ok {
		if iss != k.config.RealmURL {
			return fmt.Errorf("invalid issuer: got %q, want %q", iss, k.config.RealmURL)
		}
	} else {
		return errors.New("token missing iss claim")
	}

	if k.config.ClientID != "" {
		if err := validateAudience(claims, k.config.ClientID); err != nil {
			return err
		}
	}

	return nil
}

func validateAudience(claims map[string]interface{}, clientID string) error {
	aud, ok := claims["aud"]
	if !ok {
		return errors.New("token missing aud claim")
	}
	switch v := aud.(type) {
	case string:
		if v != clientID {
			return fmt.Errorf("invalid audience: got %q, want %q", v, clientID)
		}
	case []interface{}:
		for _, a := range v {
			if s, ok := a.(string); ok && s == clientID {
				return nil
			}
		}
		return fmt.Errorf("audience does not contain %q", clientID)
	default:
		return errors.New("invalid aud claim type")
	}
	return nil
}

func (k *KeycloakAuthenticator) extractUser(claims map[string]interface{}) (string, error) {
	if user, ok := claims[k.config.UserClaim].(string); ok && user != "" {
		return user, nil
	}
	if k.config.UserClaim != "preferred_username" {
		// UserClaim was explicitly configured; don't fall back.
		return "", fmt.Errorf("user claim %q not found in token", k.config.UserClaim)
	}
	if sub, ok := claims["sub"].(string); ok && sub != "" {
		return sub, nil
	}
	return "", errors.New("no user identity in token")
}

func (k *KeycloakAuthenticator) extractTenants(claims map[string]interface{}) ([]string, error) {
	raw, ok := claims[k.config.TenantClaim]
	if !ok {
		return nil, fmt.Errorf("tenant claim %q not found in token", k.config.TenantClaim)
	}
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, fmt.Errorf("tenant claim %q is empty", k.config.TenantClaim)
		}
		return []string{v}, nil
	case []interface{}:
		tenants := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok || s == "" {
				continue
			}
			tenants = append(tenants, s)
		}
		if len(tenants) == 0 {
			return nil, fmt.Errorf("tenant claim %q contains no valid entries", k.config.TenantClaim)
		}
		return tenants, nil
	default:
		return nil, fmt.Errorf("tenant claim %q has unsupported type", k.config.TenantClaim)
	}
}

// getSigningKey returns the RSA public key for the given kid, fetching or
// refreshing the JWKS cache as needed.
func (k *KeycloakAuthenticator) getSigningKey(kid string) (*rsa.PublicKey, error) {
	k.mu.RLock()
	if k.jwks != nil && time.Since(k.jwks.fetchedAt) < k.config.JWKSCacheTTL {
		if key, ok := k.jwks.keys[kid]; ok {
			k.mu.RUnlock()
			return key, nil
		}
	}
	k.mu.RUnlock()

	// Cache miss or expired -- refresh.
	k.mu.Lock()
	defer k.mu.Unlock()

	// Double-check after acquiring write lock.
	if k.jwks != nil && time.Since(k.jwks.fetchedAt) < k.config.JWKSCacheTTL {
		if key, ok := k.jwks.keys[kid]; ok {
			return key, nil
		}
	}

	cache, err := k.fetchJWKS()
	if err != nil {
		return nil, err
	}
	k.jwks = cache

	key, ok := cache.keys[kid]
	if !ok {
		return nil, fmt.Errorf("signing key %q not found in JWKS", kid)
	}
	return key, nil
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (k *KeycloakAuthenticator) fetchJWKS() (*jwksCache, error) {
	url := k.config.RealmURL + "/protocol/openid-connect/certs"
	resp, err := k.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching JWKS from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading JWKS response: %w", err)
	}

	var jwksResp jwksResponse
	if err := json.Unmarshal(body, &jwksResp); err != nil {
		return nil, fmt.Errorf("parsing JWKS response: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, k := range jwksResp.Keys {
		if k.Kty != "RSA" {
			continue
		}
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	return &jwksCache{
		keys:      keys,
		fetchedAt: time.Now(),
	}, nil
}

func parseRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64URLDecode(nB64)
	if err != nil {
		return nil, fmt.Errorf("decoding modulus: %w", err)
	}
	eBytes, err := base64URLDecode(eB64)
	if err != nil {
		return nil, fmt.Errorf("decoding exponent: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() {
		return nil, errors.New("exponent too large")
	}

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func verifyRS256(signingInput, signatureB64 string, key *rsa.PublicKey) error {
	signature, err := base64URLDecode(signatureB64)
	if err != nil {
		return fmt.Errorf("decoding signature: %w", err)
	}

	// RS256 = RSASSA-PKCS1-v1_5 with SHA-256
	h := sha256.New()
	h.Write([]byte(signingInput))
	hashed := h.Sum(nil)

	return rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed, signature)
}
