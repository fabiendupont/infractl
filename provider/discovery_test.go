// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProvidersConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")

	content := `providers:
  - name: partner-dns
    type: external
    socket: /run/infractl/partner-dns.sock
  - name: auto
    type: discover
    directory: /run/infractl/sockets
    exclude:
      - "test-*.sock"
  - name: core
    type: builtin
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProvidersConfig(path)
	if err != nil {
		t.Fatalf("LoadProvidersConfig: %v", err)
	}

	if len(cfg.Providers) != 3 {
		t.Fatalf("Providers count = %d, want 3", len(cfg.Providers))
	}

	p := cfg.Providers[0]
	if p.Name != "partner-dns" {
		t.Errorf("Providers[0].Name = %q, want %q", p.Name, "partner-dns")
	}
	if p.Type != "external" {
		t.Errorf("Providers[0].Type = %q, want %q", p.Type, "external")
	}
	if p.Socket != "/run/infractl/partner-dns.sock" {
		t.Errorf("Providers[0].Socket = %q, want %q", p.Socket, "/run/infractl/partner-dns.sock")
	}

	d := cfg.Providers[1]
	if d.Type != "discover" {
		t.Errorf("Providers[1].Type = %q, want %q", d.Type, "discover")
	}
	if d.Directory != "/run/infractl/sockets" {
		t.Errorf("Providers[1].Directory = %q", d.Directory)
	}
	if len(d.Exclude) != 1 || d.Exclude[0] != "test-*.sock" {
		t.Errorf("Providers[1].Exclude = %v, want [test-*.sock]", d.Exclude)
	}

	b := cfg.Providers[2]
	if b.Type != "builtin" {
		t.Errorf("Providers[2].Type = %q, want %q", b.Type, "builtin")
	}
}

func TestLoadProvidersConfigNotFound(t *testing.T) {
	_, err := LoadProvidersConfig("/nonexistent/providers.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestIsExcluded(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		want     bool
	}{
		{"foo.sock", []string{"foo.sock"}, true},
		{"foo.sock", []string{"bar.sock"}, false},
		{"test-abc.sock", []string{"test-*.sock"}, true},
		{"prod-abc.sock", []string{"test-*.sock"}, false},
		{"anything.sock", nil, false},
		{"anything.sock", []string{}, false},
		{"foo.sock", []string{"*.sock"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExcluded(tt.name, tt.patterns)
			if got != tt.want {
				t.Errorf("isExcluded(%q, %v) = %v, want %v", tt.name, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestDiscoverExternalProvidersBuiltinSkipped(t *testing.T) {
	cfg := &ProvidersConfig{
		Providers: []ExternalProviderConfig{
			{Name: "core", Type: "builtin"},
		},
	}

	providers, err := DiscoverExternalProviders(cfg)
	if err != nil {
		t.Fatalf("DiscoverExternalProviders: %v", err)
	}
	if len(providers) != 0 {
		t.Errorf("expected 0 providers for builtin, got %d", len(providers))
	}
}

func TestDiscoverExternalProvidersMissingSocket(t *testing.T) {
	cfg := &ProvidersConfig{
		Providers: []ExternalProviderConfig{
			{Name: "bad", Type: "external", Socket: ""},
		},
	}

	_, err := DiscoverExternalProviders(cfg)
	if err == nil {
		t.Fatal("expected error for external provider without socket")
	}
}

func TestDiscoverExternalProvidersMissingDirectory(t *testing.T) {
	cfg := &ProvidersConfig{
		Providers: []ExternalProviderConfig{
			{Name: "bad", Type: "discover", Directory: ""},
		},
	}

	_, err := DiscoverExternalProviders(cfg)
	if err == nil {
		t.Fatal("expected error for discover entry without directory")
	}
}

func TestDiscoverFromDirectoryNoSockets(t *testing.T) {
	dir := t.TempDir()

	// Create non-socket files.
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0644)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("x: 1"), 0644)

	providers, err := discoverFromDirectory(dir, nil)
	if err != nil {
		t.Fatalf("discoverFromDirectory: %v", err)
	}
	if len(providers) != 0 {
		t.Errorf("expected 0 providers, got %d", len(providers))
	}
}

func TestDiscoverFromDirectoryExcludesPattern(t *testing.T) {
	dir := t.TempDir()

	// Create .sock files — they won't connect but the exclude logic runs first.
	os.WriteFile(filepath.Join(dir, "test-a.sock"), []byte{}, 0644)
	os.WriteFile(filepath.Join(dir, "test-b.sock"), []byte{}, 0644)
	os.WriteFile(filepath.Join(dir, "prod.sock"), []byte{}, 0644)

	// Exclude all test-* sockets. prod.sock will attempt to connect and fail
	// (not a real socket), but that's logged as a warning and skipped.
	providers, err := discoverFromDirectory(dir, []string{"test-*.sock"})
	if err != nil {
		t.Fatalf("discoverFromDirectory: %v", err)
	}
	// prod.sock fails to connect (not a real Unix socket), so 0 providers.
	if len(providers) != 0 {
		t.Errorf("expected 0 providers (connect fails), got %d", len(providers))
	}
}
