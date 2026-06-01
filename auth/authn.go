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
	"net/http"
)

// Authenticator extracts an authenticated Subject from an HTTP request.
// Implementations may inspect headers, cookies, or tokens.
type Authenticator interface {
	Authenticate(r *http.Request) (*Subject, error)
}

// ContextAuthenticator extracts an authenticated Subject from a context.
// Used by gRPC interceptors where no *http.Request is available.
type ContextAuthenticator interface {
	AuthenticateContext(ctx context.Context) (*Subject, error)
}

// GuestAuthenticator always returns a fixed guest subject. Use this for
// development or single-tenant deployments where authentication is not
// required.
type GuestAuthenticator struct {
	// GuestUser is the user identity assigned to all requests.
	// Defaults to "guest" if empty.
	GuestUser string

	// GuestTenant is the tenant ID assigned to the guest subject.
	// Defaults to "default" if empty.
	GuestTenant string
}

// Authenticate returns a guest Subject regardless of request content.
func (g *GuestAuthenticator) Authenticate(_ *http.Request) (*Subject, error) { return g.guest() }

// AuthenticateContext returns a guest Subject regardless of context content.
func (g *GuestAuthenticator) AuthenticateContext(_ context.Context) (*Subject, error) { return g.guest() }

func (g *GuestAuthenticator) guest() (*Subject, error) {
	user := g.GuestUser
	if user == "" {
		user = "guest"
	}
	tenant := g.GuestTenant
	if tenant == "" {
		tenant = "00000000-0000-0000-0000-000000000001"
	}
	return &Subject{
		User:    user,
		Tenants: NewTenantSet(tenant),
	}, nil
}
