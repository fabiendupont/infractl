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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// ExternalProviderConfig describes a single provider entry in providers.yaml.
type ExternalProviderConfig struct {
	Name      string   `yaml:"name"`
	Type      string   `yaml:"type"` // "builtin", "external", "discover"
	Socket    string   `yaml:"socket"`
	Directory string   `yaml:"directory"`
	Exclude   []string `yaml:"exclude"`
}

// ProvidersConfig is the top-level providers.yaml structure.
type ProvidersConfig struct {
	Providers []ExternalProviderConfig `yaml:"providers"`
}

// LoadProvidersConfig reads and parses a providers.yaml file.
func LoadProvidersConfig(path string) (*ProvidersConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read providers config %s: %w", path, err)
	}

	var cfg ProvidersConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse providers config %s: %w", path, err)
	}

	return &cfg, nil
}

// DiscoverExternalProviders connects to all external providers described
// in the config. For "external" entries, it dials the specified socket.
// For "discover" entries, it scans the directory for .sock files.
func DiscoverExternalProviders(cfg *ProvidersConfig) ([]*ExternalProvider, error) {
	var providers []*ExternalProvider

	for _, entry := range cfg.Providers {
		switch entry.Type {
		case "external":
			if entry.Socket == "" {
				return nil, fmt.Errorf("external provider %q requires a socket path", entry.Name)
			}
			ep, err := ConnectExternalProvider(entry.Socket)
			if err != nil {
				return nil, fmt.Errorf("failed to connect to external provider %q: %w", entry.Name, err)
			}
			providers = append(providers, ep)
			log.Info().
				Str("provider", ep.Name()).
				Str("socket", entry.Socket).
				Msg("connected to external provider")

		case "discover":
			if entry.Directory == "" {
				return nil, fmt.Errorf("discover entry %q requires a directory", entry.Name)
			}
			discovered, err := discoverFromDirectory(entry.Directory, entry.Exclude)
			if err != nil {
				return nil, fmt.Errorf("failed to discover providers in %s: %w", entry.Directory, err)
			}
			providers = append(providers, discovered...)

		case "builtin":
			// Builtin providers are registered separately; skip.
			continue

		default:
			log.Warn().Str("type", entry.Type).Str("name", entry.Name).Msg("unknown provider type, skipping")
		}
	}

	return providers, nil
}

// discoverFromDirectory scans a directory for .sock files, connects to
// each one, and returns the resulting ExternalProvider instances.
func discoverFromDirectory(dir string, exclude []string) ([]*ExternalProvider, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var providers []*ExternalProvider
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".sock") {
			continue
		}
		if isExcluded(entry.Name(), exclude) {
			log.Debug().Str("socket", entry.Name()).Msg("excluded by pattern")
			continue
		}

		socketPath := filepath.Join(dir, entry.Name())
		ep, err := ConnectExternalProvider(socketPath)
		if err != nil {
			log.Warn().Err(err).Str("socket", socketPath).Msg("failed to connect to discovered socket")
			continue
		}

		providers = append(providers, ep)
		log.Info().
			Str("provider", ep.Name()).
			Str("socket", socketPath).
			Msg("discovered external provider")
	}

	return providers, nil
}

// isExcluded checks if a filename matches any of the exclude patterns.
func isExcluded(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}
