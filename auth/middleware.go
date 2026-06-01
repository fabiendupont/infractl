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
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const (
	subjectContextKey contextKey = "auth.subject"
	orgIDContextKey   contextKey = "auth.org_id"

	orgIDHeader = "X-Org-ID"
)

// SubjectFromContext extracts the authenticated Subject stored by the AuthN
// middleware. Returns an error if no subject is present.
func SubjectFromContext(ctx context.Context) (*Subject, error) {
	sub, ok := ctx.Value(subjectContextKey).(*Subject)
	if !ok || sub == nil {
		return nil, errors.New("no authenticated subject in context")
	}
	return sub, nil
}

// ContextWithSubject stores a Subject in the context.
func ContextWithSubject(ctx context.Context, sub *Subject) context.Context {
	return context.WithValue(ctx, subjectContextKey, sub)
}

// ContextWithOrgID stores a resolved organization (tenant) UUID in the context.
func ContextWithOrgID(ctx context.Context, orgID uuid.UUID) context.Context {
	return context.WithValue(ctx, orgIDContextKey, orgID)
}

// OrgIDFromContext extracts the resolved organization (tenant) UUID stored
// by the Tenancy middleware. Returns an error if no org ID is present.
func OrgIDFromContext(ctx context.Context) (uuid.UUID, error) {
	id, ok := ctx.Value(orgIDContextKey).(uuid.UUID)
	if !ok {
		return uuid.UUID{}, errors.New("no org ID in context (Tenancy middleware not configured?)")
	}
	return id, nil
}

// Tenancy returns chi-compatible middleware that resolves the tenant org_id
// for each request. If X-Org-ID is provided, it is validated against the
// subject's allowed tenants. If omitted, the default tenant is used. The
// resolved uuid.UUID is stored in the request context for OrgIDFromContext.
//
// Must run after AuthN (requires Subject in context).
func Tenancy(logic TenancyLogic) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var orgID uuid.UUID

			if raw := r.Header.Get(orgIDHeader); raw != "" {
				parsed, err := uuid.Parse(raw)
				if err != nil {
					http.Error(w, "Bad Request: invalid "+orgIDHeader+": "+err.Error(), http.StatusBadRequest)
					return
				}

				visible, err := logic.DetermineVisibleTenants(r.Context())
				if err != nil {
					http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
					return
				}
				if !visible.IsUniversal() && !visible.Contains(parsed.String()) {
					http.Error(w, "Forbidden: tenant not accessible", http.StatusForbidden)
					return
				}
				orgID = parsed
			} else {
				defaults, err := logic.DetermineDefaultTenants(r.Context())
				if err != nil {
					http.Error(w, "Internal Server Error: "+err.Error(), http.StatusInternalServerError)
					return
				}
				vals := defaults.Values()
				if len(vals) != 1 {
					http.Error(w, "Bad Request: "+orgIDHeader+" header required", http.StatusBadRequest)
					return
				}
				parsed, err := uuid.Parse(vals[0])
				if err != nil {
					http.Error(w, "Internal Server Error: default tenant is not a valid UUID", http.StatusInternalServerError)
					return
				}
				orgID = parsed
			}

			ctx := ContextWithOrgID(r.Context(), orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthN returns chi-compatible middleware that authenticates every request
// using the provided Authenticator. On success the Subject is stored in the
// request context for downstream handlers. On failure a 401 response is
// written and the chain is aborted.
func AuthN(authn Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject, err := authn.Authenticate(r)
			if err != nil {
				http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
				return
			}
			ctx := ContextWithSubject(r.Context(), subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthZ returns chi-compatible middleware that authorizes every request using
// the provided Authorizer. It extracts the Subject from context (set by AuthN),
// determines the resource and action from the request, and calls Authorize.
// On denial a 403 response is written.
//
// The resource is derived from the first path segment after the base path,
// and the action is mapped from the HTTP method:
//
//	GET    -> read
//	POST   -> create
//	PUT    -> update
//	PATCH  -> update
//	DELETE -> delete
func AuthZ(authz Authorizer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject, err := SubjectFromContext(r.Context())
			if err != nil {
				http.Error(w, "Forbidden: "+err.Error(), http.StatusForbidden)
				return
			}

			resource := extractResource(r.URL.Path)
			action := methodToAction(r.Method)

			if err := authz.Authorize(r.Context(), subject, resource, action); err != nil {
				http.Error(w, "Forbidden: "+err.Error(), http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// methodToAction maps HTTP methods to authorization action names.
func methodToAction(method string) string {
	switch method {
	case http.MethodGet:
		return "read"
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// extractResource pulls the first meaningful path segment as the resource
// name. For example, "/api/v1/networks/my-net" yields "networks".
func extractResource(path string) string {
	// Skip leading slash and find first non-empty segment.
	segments := splitPath(path)
	// Skip common API prefixes like "api" and version segments like "v1".
	for _, seg := range segments {
		if seg == "api" || seg == "" {
			continue
		}
		if len(seg) > 0 && seg[0] == 'v' && len(seg) <= 3 {
			continue
		}
		return seg
	}
	if len(segments) > 0 {
		return segments[len(segments)-1]
	}
	return ""
}

// splitPath splits a URL path into non-empty segments.
func splitPath(path string) []string {
	var segments []string
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i > start {
				segments = append(segments, path[start:i])
			}
			start = i + 1
		}
	}
	return segments
}
