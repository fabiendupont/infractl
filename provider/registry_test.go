package provider

import (
	"context"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider is a minimal Provider implementation for testing.
type mockProvider struct {
	name         string
	version      string
	features     []string
	dependencies []string
	initFunc     func(Context) error
	shutdownFunc func(context.Context) error
}

func (m *mockProvider) Name() string            { return m.name }
func (m *mockProvider) Version() string          { return m.version }
func (m *mockProvider) Features() []string       { return m.features }
func (m *mockProvider) Dependencies() []string   { return m.dependencies }
func (m *mockProvider) Init(ctx Context) error {
	if m.initFunc != nil {
		return m.initFunc(ctx)
	}
	return nil
}
func (m *mockProvider) Shutdown(ctx context.Context) error {
	if m.shutdownFunc != nil {
		return m.shutdownFunc(ctx)
	}
	return nil
}

// mockAPIProvider implements both Provider and APIProvider.
type mockAPIProvider struct {
	mockProvider
}

func (m *mockAPIProvider) RegisterRoutes(_ chi.Router) {}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	p := &mockProvider{name: "test", version: "1.0", features: []string{"f1"}}
	require.NoError(t, r.Register(p))

	got, ok := r.Get("test")
	assert.True(t, ok)
	assert.Equal(t, "test", got.Name())
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	p := &mockProvider{name: "test", version: "1.0"}
	require.NoError(t, r.Register(p))

	err := r.Register(&mockProvider{name: "test", version: "2.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_RegisterConflictingFeature(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&mockProvider{name: "a", features: []string{"shared"}}))

	err := r.Register(&mockProvider{name: "b", features: []string{"shared"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already provided by")
}

func TestRegistry_ResolveDependencies_LinearChain(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&mockProvider{name: "A", dependencies: []string{"B"}}))
	require.NoError(t, r.Register(&mockProvider{name: "B", dependencies: []string{"C"}}))
	require.NoError(t, r.Register(&mockProvider{name: "C"}))

	require.NoError(t, r.ResolveDependencies())
	assert.Equal(t, []string{"C", "B", "A"}, r.order)
}

func TestRegistry_ResolveDependencies_Circular(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&mockProvider{name: "A", dependencies: []string{"B"}}))
	require.NoError(t, r.Register(&mockProvider{name: "B", dependencies: []string{"A"}}))

	err := r.ResolveDependencies()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestRegistry_ResolveDependencies_MissingDep(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&mockProvider{name: "A", dependencies: []string{"missing"}}))

	err := r.ResolveDependencies()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestRegistry_InitAll_DependencyOrder(t *testing.T) {
	r := NewRegistry()
	var order []string

	require.NoError(t, r.Register(&mockProvider{
		name:         "A",
		dependencies: []string{"B"},
		initFunc:     func(_ Context) error { order = append(order, "A"); return nil },
	}))
	require.NoError(t, r.Register(&mockProvider{
		name:     "B",
		initFunc: func(_ Context) error { order = append(order, "B"); return nil },
	}))

	require.NoError(t, r.InitAll(Context{}))
	assert.Equal(t, []string{"B", "A"}, order)
}

func TestRegistry_ShutdownAll_ReverseOrder(t *testing.T) {
	r := NewRegistry()
	var order []string

	require.NoError(t, r.Register(&mockProvider{
		name:         "A",
		dependencies: []string{"B"},
		shutdownFunc: func(_ context.Context) error { order = append(order, "A"); return nil },
	}))
	require.NoError(t, r.Register(&mockProvider{
		name:         "B",
		shutdownFunc: func(_ context.Context) error { order = append(order, "B"); return nil },
	}))

	require.NoError(t, r.ResolveDependencies())
	require.NoError(t, r.ShutdownAll(context.Background()))
	assert.Equal(t, []string{"A", "B"}, order)
}

func TestRegistry_APIProviders(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&mockProvider{name: "plain"}))
	require.NoError(t, r.Register(&mockAPIProvider{mockProvider: mockProvider{name: "api"}}))

	require.NoError(t, r.ResolveDependencies())
	apis := r.APIProviders()
	require.Len(t, apis, 1)
	assert.Equal(t, "api", apis[0].Name())
}
