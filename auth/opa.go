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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
)

// OPAConfig holds configuration for the OPA-based authorizer.
type OPAConfig struct {
	// PolicyDir is the directory containing .rego policy files.
	PolicyDir string

	// DefaultPolicy is the Rego package path for the allow rule.
	// Defaults to "data.infractl.authz.allow" if empty.
	DefaultPolicy string
}

// OPAAuthorizer evaluates Rego policies to decide whether a subject is
// permitted to perform an action on a resource kind.
type OPAAuthorizer struct {
	config   OPAConfig
	compiler *ast.Compiler
	mu       sync.RWMutex
}

// NewOPAAuthorizer creates an OPAAuthorizer by loading and compiling all
// .rego files from the configured policy directory.
func NewOPAAuthorizer(config OPAConfig) (*OPAAuthorizer, error) {
	if config.DefaultPolicy == "" {
		config.DefaultPolicy = "data.infractl.authz.allow"
	}

	a := &OPAAuthorizer{config: config}
	if err := a.loadPolicies(); err != nil {
		return nil, err
	}
	return a, nil
}

// Authorize evaluates the loaded Rego policies against the given subject,
// resource, and action. Returns nil if the request is allowed, or an error
// describing the denial.
func (a *OPAAuthorizer) Authorize(_ context.Context, subject *Subject, resource, action string) error {
	input := map[string]interface{}{
		"subject": map[string]interface{}{
			"user":    subject.User,
			"tenants": subject.Tenants.Values(),
		},
		"resource": resource,
		"action":   action,
	}

	a.mu.RLock()
	compiler := a.compiler
	a.mu.RUnlock()

	query := rego.New(
		rego.Query(a.config.DefaultPolicy),
		rego.Compiler(compiler),
		rego.Input(input),
	)

	rs, err := query.Eval(context.Background())
	if err != nil {
		return fmt.Errorf("policy evaluation failed: %w", err)
	}

	if len(rs) > 0 && len(rs[0].Expressions) > 0 {
		if allowed, ok := rs[0].Expressions[0].Value.(bool); ok && allowed {
			return nil
		}
	}

	return fmt.Errorf("access denied: %s %s", action, resource)
}

// ReloadPolicies re-reads and recompiles policies from the configured
// directory, allowing hot-reloading without restart.
func (a *OPAAuthorizer) ReloadPolicies() error {
	return a.loadPolicies()
}

func (a *OPAAuthorizer) loadPolicies() error {
	modules := map[string]*ast.Module{}

	entries, err := os.ReadDir(a.config.PolicyDir)
	if err != nil {
		return fmt.Errorf("reading policy directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".rego") {
			continue
		}
		path := filepath.Join(a.config.PolicyDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading policy file %s: %w", entry.Name(), err)
		}
		parsed, err := ast.ParseModule(entry.Name(), string(data))
		if err != nil {
			return fmt.Errorf("parsing policy file %s: %w", entry.Name(), err)
		}
		modules[entry.Name()] = parsed
	}

	compiler := ast.NewCompiler()
	compiler.Compile(modules)
	if compiler.Failed() {
		return fmt.Errorf("compiling policies: %v", compiler.Errors)
	}

	a.mu.Lock()
	a.compiler = compiler
	a.mu.Unlock()

	return nil
}
