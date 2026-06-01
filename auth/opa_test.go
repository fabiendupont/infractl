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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPolicy = `package infractl.authz

default allow := false

allow if {
	input.action == "read"
}

allow if {
	input.action != "read"
	input.subject.user == "admin"
}
`

func writePolicy(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

func TestOPAAuthorizer_LoadsPolicies(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "authz.rego", testPolicy)

	authorizer, err := NewOPAAuthorizer(OPAConfig{PolicyDir: dir})
	require.NoError(t, err)
	assert.NotNil(t, authorizer)
}

func TestOPAAuthorizer_DefaultDeny(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "empty.rego", `package infractl.authz
default allow := false
`)

	authorizer, err := NewOPAAuthorizer(OPAConfig{PolicyDir: dir})
	require.NoError(t, err)

	subject := &Subject{User: "someone", Tenants: NewTenantSet("t1")}
	err = authorizer.Authorize(context.Background(), subject, "networks", "create")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestOPAAuthorizer_AllowRead(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "authz.rego", testPolicy)

	authorizer, err := NewOPAAuthorizer(OPAConfig{PolicyDir: dir})
	require.NoError(t, err)

	subject := &Subject{User: "viewer", Tenants: NewTenantSet("t1")}
	err = authorizer.Authorize(context.Background(), subject, "networks", "read")
	assert.NoError(t, err)
}

func TestOPAAuthorizer_DenyWriteForNonAdmin(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "authz.rego", testPolicy)

	authorizer, err := NewOPAAuthorizer(OPAConfig{PolicyDir: dir})
	require.NoError(t, err)

	subject := &Subject{User: "guest", Tenants: NewTenantSet("t1")}
	err = authorizer.Authorize(context.Background(), subject, "networks", "create")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestOPAAuthorizer_AllowWriteForAdmin(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "authz.rego", testPolicy)

	authorizer, err := NewOPAAuthorizer(OPAConfig{PolicyDir: dir})
	require.NoError(t, err)

	subject := &Subject{User: "admin", Tenants: NewTenantSet("t1")}
	err = authorizer.Authorize(context.Background(), subject, "networks", "create")
	assert.NoError(t, err)
}

func TestOPAAuthorizer_InvalidPolicyDir(t *testing.T) {
	_, err := NewOPAAuthorizer(OPAConfig{PolicyDir: "/nonexistent/path"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading policy directory")
}

func TestOPAAuthorizer_EmptyPolicyDir(t *testing.T) {
	dir := t.TempDir()

	authorizer, err := NewOPAAuthorizer(OPAConfig{PolicyDir: dir})
	require.NoError(t, err)

	subject := &Subject{User: "admin", Tenants: NewTenantSet("t1")}
	err = authorizer.Authorize(context.Background(), subject, "networks", "read")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestOPAAuthorizer_ReloadPolicies(t *testing.T) {
	dir := t.TempDir()

	// Start with a deny-all policy.
	writePolicy(t, dir, "authz.rego", `package infractl.authz
default allow := false
`)

	authorizer, err := NewOPAAuthorizer(OPAConfig{PolicyDir: dir})
	require.NoError(t, err)

	subject := &Subject{User: "viewer", Tenants: NewTenantSet("t1")}
	err = authorizer.Authorize(context.Background(), subject, "networks", "read")
	assert.Error(t, err, "should deny before reload")

	// Replace with a policy that allows reads.
	writePolicy(t, dir, "authz.rego", testPolicy)

	require.NoError(t, authorizer.ReloadPolicies())

	err = authorizer.Authorize(context.Background(), subject, "networks", "read")
	assert.NoError(t, err, "should allow after reload")
}

func TestOPAAuthorizer_DefaultPolicyPath(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "authz.rego", testPolicy)

	authorizer, err := NewOPAAuthorizer(OPAConfig{PolicyDir: dir})
	require.NoError(t, err)
	assert.Equal(t, "data.infractl.authz.allow", authorizer.config.DefaultPolicy)
}

func TestOPAAuthorizer_CustomPolicyPath(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "custom.rego", `package custom.policy
default allow := false
allow if { input.action == "read" }
`)

	authorizer, err := NewOPAAuthorizer(OPAConfig{
		PolicyDir:     dir,
		DefaultPolicy: "data.custom.policy.allow",
	})
	require.NoError(t, err)

	subject := &Subject{User: "viewer", Tenants: NewTenantSet("t1")}
	err = authorizer.Authorize(context.Background(), subject, "networks", "read")
	assert.NoError(t, err)
}
