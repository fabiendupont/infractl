// Copyright 2025 Fabien Dupont
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

package provider

import (
	"context"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc"
)

// Provider is the contract every infractl provider implements.
// Each provider declares a name, version, features it offers, and
// dependencies on other providers. The lifecycle is Init then Shutdown.
type Provider interface {
	Name() string
	Version() string
	Features() []string
	Dependencies() []string
	Init(ctx Context) error
	Shutdown(ctx context.Context) error
}

// APIProvider adds REST routes to the chi router. Providers that serve
// HTTP endpoints implement this interface in addition to Provider.
type APIProvider interface {
	Provider
	RegisterRoutes(r chi.Router)
}

// GRPCProvider registers gRPC services with the server. Providers that
// serve gRPC endpoints implement this interface in addition to Provider.
type GRPCProvider interface {
	Provider
	RegisterServices(s *grpc.Server)
}

// WorkflowProvider registers async workflows with a workflow engine.
// The engine is provider-defined — Temporal, AAP, in-process, or other.
// The registry type is intentionally interface{} to avoid coupling
// infractl to a specific workflow engine.
type WorkflowProvider interface {
	Provider
	RegisterWorkflows(registry interface{})
}

// MigrationProvider manages its own DB tables. The returned fs.FS must
// contain SQL migration files that the core migration runner applies.
type MigrationProvider interface {
	Provider
	MigrationSource() fs.FS
}
