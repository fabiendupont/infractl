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
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
)

// Registry manages provider lifecycle: registration, dependency resolution,
// initialization, and shutdown.
type Registry struct {
	providers map[string]Provider
	features  map[string]string // feature name -> provider name
	order     []string          // init order after dependency resolution
	hooks     *hookRegistry
}

// NewRegistry creates a new provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		features:  make(map[string]string),
		hooks:     newHookRegistry(),
	}
}

// Register adds a provider to the registry and maps its features.
// Returns an error if the provider name is already registered or if
// any of its features conflict with an existing provider.
func (r *Registry) Register(p Provider) error {
	name := p.Name()
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider %q already registered", name)
	}
	r.providers[name] = p
	for _, f := range p.Features() {
		if existing, ok := r.features[f]; ok {
			return fmt.Errorf("feature %q already provided by %q, cannot register %q", f, existing, name)
		}
		r.features[f] = name
	}
	return nil
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// FeatureProvider returns the provider registered for a given feature.
func (r *Registry) FeatureProvider(feature string) (Provider, bool) {
	name, ok := r.features[feature]
	if !ok {
		return nil, false
	}
	return r.providers[name], true
}

// ResolveDependencies performs a topological sort of providers based on
// their declared dependencies using depth-first search. Returns an error
// on circular or missing dependencies.
func (r *Registry) ResolveDependencies() error {
	// State: 0=unvisited, 1=in-progress, 2=done
	visited := make(map[string]int)
	var order []string

	var visit func(name string) error
	visit = func(name string) error {
		switch visited[name] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("circular dependency detected involving %q", name)
		}
		visited[name] = 1

		p, ok := r.providers[name]
		if !ok {
			return fmt.Errorf("missing dependency: provider %q not registered", name)
		}

		for _, dep := range p.Dependencies() {
			if _, ok := r.providers[dep]; !ok {
				return fmt.Errorf("provider %q depends on %q, which is not registered", name, dep)
			}
			if err := visit(dep); err != nil {
				return err
			}
		}

		visited[name] = 2
		order = append(order, name)
		return nil
	}

	for name := range r.providers {
		if err := visit(name); err != nil {
			return err
		}
	}

	r.order = order
	return nil
}

// InitAll initializes all providers in dependency order.
func (r *Registry) InitAll(ctx Context) error {
	if r.order == nil {
		if err := r.ResolveDependencies(); err != nil {
			return err
		}
	}
	for _, name := range r.order {
		if err := r.providers[name].Init(ctx); err != nil {
			return fmt.Errorf("failed to initialize provider %q: %w", name, err)
		}
	}
	return nil
}

// ShutdownAll shuts down all providers in reverse dependency order.
func (r *Registry) ShutdownAll(ctx context.Context) error {
	for i := len(r.order) - 1; i >= 0; i-- {
		if err := r.providers[r.order[i]].Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown provider %q: %w", r.order[i], err)
		}
	}
	return nil
}

// APIProviders returns all registered providers that implement APIProvider,
// in dependency-resolved order.
func (r *Registry) APIProviders() []APIProvider {
	var result []APIProvider
	for _, name := range r.order {
		if ap, ok := r.providers[name].(APIProvider); ok {
			result = append(result, ap)
		}
	}
	return result
}

// GRPCProviders returns all registered providers that implement
// GRPCProvider, in dependency-resolved order.
func (r *Registry) GRPCProviders() []GRPCProvider {
	var result []GRPCProvider
	for _, name := range r.order {
		if gp, ok := r.providers[name].(GRPCProvider); ok {
			result = append(result, gp)
		}
	}
	return result
}

// WorkflowProviders returns all registered providers that implement
// WorkflowProvider, in dependency-resolved order.
func (r *Registry) WorkflowProviders() []WorkflowProvider {
	var result []WorkflowProvider
	for _, name := range r.order {
		if wp, ok := r.providers[name].(WorkflowProvider); ok {
			result = append(result, wp)
		}
	}
	return result
}

// MigrationProviders returns all registered providers that implement
// MigrationProvider, in dependency-resolved order.
func (r *Registry) MigrationProviders() []MigrationProvider {
	var result []MigrationProvider
	for _, name := range r.order {
		if mp, ok := r.providers[name].(MigrationProvider); ok {
			result = append(result, mp)
		}
	}
	return result
}

// RegisterExternalProviders loads providers.yaml and registers any external
// provider sidecars it describes. The config file is optional -- if it does
// not exist, no external providers are registered and nil is returned.
func (r *Registry) RegisterExternalProviders(configPath string) error {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Debug().Str("path", configPath).Msg("no external providers config found")
		return nil
	}

	cfg, err := LoadProvidersConfig(configPath)
	if err != nil {
		return err
	}

	providers, err := DiscoverExternalProviders(cfg)
	if err != nil {
		return err
	}

	for _, p := range providers {
		if err := r.Register(p); err != nil {
			return fmt.Errorf("failed to register external provider %q: %w", p.Name(), err)
		}
	}

	log.Info().Int("count", len(providers)).Msg("external providers registered")
	return nil
}
