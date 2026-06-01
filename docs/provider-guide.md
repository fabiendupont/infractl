# Provider Guide

This guide walks through building an infractl provider from scratch. By the end, you will have a provider that exposes a REST API for managing a custom resource type, integrates with cross-provider hooks, and can be deployed via profiles.

The inventory/machine example in `examples/inventory/` serves as the reference implementation throughout.

## 1. Minimal Provider

A minimal provider implements the `Provider` interface: six methods, no external dependencies.

```go
// providers/inventory/provider.go
package inventory

import (
    "context"

    "github.com/rs/zerolog"

    "github.com/fabiendupont/infractl/provider"
)

type InventoryProvider struct {
    logger zerolog.Logger
}

func New() *InventoryProvider {
    return &InventoryProvider{}
}

func (p *InventoryProvider) Name() string           { return "inventory" }
func (p *InventoryProvider) Version() string        { return "0.1.0" }
func (p *InventoryProvider) Features() []string     { return []string{"inventory"} }
func (p *InventoryProvider) Dependencies() []string { return nil }

func (p *InventoryProvider) Init(ctx provider.Context) error {
    p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()
    p.logger.Info().Msg("inventory provider initialized")
    return nil
}

func (p *InventoryProvider) Shutdown(_ context.Context) error {
    p.logger.Info().Msg("inventory provider shutdown")
    return nil
}

// Compile-time interface check
var _ provider.Provider = (*InventoryProvider)(nil)
```

Register the provider in your application's main:

```go
func main() {
    registry := provider.NewRegistry()
    registry.Register(inventory.New())
    // ... start server
}
```

### What each method does

- **Name()** returns a unique string identifying this provider. No two registered providers may share a name.
- **Version()** returns a semantic version string. Used in capability discovery responses.
- **Features()** declares the capabilities this provider supplies. Other providers can depend on these feature names. The registry rejects duplicate feature claims.
- **Dependencies()** lists feature names this provider requires. The registry ensures those features are available and initializes their providers first.
- **Init()** receives a `provider.Context` containing shared services. Perform setup here: open connections, run migrations, register hooks. Return an error to abort startup.
- **Shutdown()** is called during graceful shutdown, in reverse initialization order. Clean up connections, flush buffers, cancel background work.

## 2. Adding REST Endpoints

To serve HTTP endpoints, implement the `APIProvider` interface by adding a `RegisterRoutes` method.

```go
import "github.com/go-chi/chi/v5"

func (p *InventoryProvider) RegisterRoutes(r chi.Router) {
    r.Route("/machines", func(r chi.Router) {
        r.Get("/", p.listMachines)
        r.Post("/", p.createMachine)
        r.Route("/{name}", func(r chi.Router) {
            r.Get("/", p.getMachine)
            r.Put("/", p.updateMachine)
            r.Delete("/", p.deleteMachine)
        })
    })
}

var _ provider.APIProvider = (*InventoryProvider)(nil)
```

Routes are mounted under the API prefix (e.g., `/api/v1/machines`). The chi router is shared across all providers, so path conflicts between providers will cause a startup panic.

### Handler implementation

Handlers follow standard `net/http` conventions. The tenant OrgID is available from the request context, injected by the auth middleware:

```go
func (p *InventoryProvider) listMachines(w http.ResponseWriter, r *http.Request) {
    orgID, err := auth.OrgIDFromContext(r.Context())
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
    if limit <= 0 {
        limit = 100
    }

    list, err := p.store.List(r.Context(), orgID, resource.ListOptions{
        Limit:    limit,
        Continue: r.URL.Query().Get("continue"),
        Filter:   r.URL.Query().Get("filter"),
    })
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    writeJSON(w, http.StatusOK, list)
}

func (p *InventoryProvider) getMachine(w http.ResponseWriter, r *http.Request) {
    orgID, err := auth.OrgIDFromContext(r.Context())
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    name := chi.URLParam(r, "name")

    machine, err := p.store.Get(r.Context(), orgID, name)
    if err != nil {
        if errors.Is(err, resource.ErrNotFound) {
            http.Error(w, "not found", http.StatusNotFound)
            return
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    writeJSON(w, http.StatusOK, machine)
}
```

### OpenAPI fragment

Create an OpenAPI fragment alongside your provider code. The framework merges all fragments at startup:

```yaml
# providers/inventory/openapi.yaml
paths:
  /machines:
    get:
      summary: List machines
      operationId: listMachines
      parameters:
        - name: filter
          in: query
          schema:
            type: string
        - name: continue
          in: query
          schema:
            type: string
        - name: limit
          in: query
          schema:
            type: integer
      responses:
        '200':
          description: Machine list
    post:
      summary: Create a machine
      operationId: createMachine
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Machine'
      responses:
        '201':
          description: Machine created
```

## 3. Adding a Resource Type

Define a model struct that embeds `resource.Resource` and adds typed Spec and Status fields.

### Model definition

```go
// providers/inventory/model.go
package inventory

import "github.com/fabiendupont/infractl/resource"

type MachineSpec struct {
    Arch     string `json:"arch,omitempty"`
    CPUs     int    `json:"cpus,omitempty"`
    MemoryMB int    `json:"memory_mb,omitempty"`
    DiskGB   int    `json:"disk_gb,omitempty"`
}

type MachineStatus struct {
    Phase   string `json:"phase"`
    Message string `json:"message,omitempty"`
}

type Machine struct {
    resource.Resource
    Spec   resource.JSONField[MachineSpec]   `gorm:"type:jsonb" json:"spec"`
    Status resource.JSONField[MachineStatus] `gorm:"type:jsonb" json:"status"`
}

func (Machine) TableName() string { return "machines" }

// SpecBytes enables automatic Generation tracking on spec changes.
func (m *Machine) SpecBytes() ([]byte, error) {
    return resource.MarshalSpec(m.Spec.Data)
}
```

### Store registration

Create a typed store in the provider's `Init` method:

```go
func (p *InventoryProvider) Init(ctx provider.Context) error {
    p.db = ctx.DB
    p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()

    // Create a typed store for Machine resources
    p.store = resource.NewGenericStore[Machine](ctx.DB)

    // Run database migration for the Machine table
    if err := ctx.DB.AutoMigrate(&Machine{}); err != nil {
        return err
    }

    p.logger.Info().Msg("inventory provider initialized")
    return nil
}
```

The `GenericStore[Machine]` provides Create, Get, List, Update, and Delete operations that are automatically scoped to the caller's OrgID and handle optimistic concurrency via ResourceVersion.

### Using the MigrationProvider interface

For production use, prefer versioned migrations over AutoMigrate:

```go
//go:embed migrations/*.sql
var inventoryMigrations embed.FS

func (p *InventoryProvider) MigrationSource() fs.FS {
    return inventoryMigrations
}

var _ provider.MigrationProvider = (*InventoryProvider)(nil)
```

## 4. Cross-Provider Hooks

Hooks let providers interact without importing each other. A storage provider can block machine deletion; a monitoring provider can react to machine creation.

### Registering a sync hook

Sync hooks run inline within the caller's transaction. Returning an error aborts the operation.

```go
// In a storage provider that needs to prevent machine deletion
// when volumes are still attached:

func (p *StorageProvider) Init(ctx provider.Context) error {
    // ...

    ctx.Registry.RegisterHook(provider.SyncHook{
        Feature: "machines",
        Event:   "pre_delete",
        Handler: p.blockDeleteIfVolumesAttached,
    })

    return nil
}

func (p *StorageProvider) blockDeleteIfVolumesAttached(
    ctx context.Context, payload interface{},
) error {
    machine, ok := payload.(*Machine)
    if !ok {
        return nil
    }

    volumes, err := p.volumeStore.ListByMachine(ctx, machine.OrgID, machine.Name)
    if err != nil {
        return fmt.Errorf("checking volumes: %w", err)
    }

    if len(volumes) > 0 {
        return fmt.Errorf(
            "cannot delete machine %q: %d volumes still attached",
            machine.Name, len(volumes),
        )
    }

    return nil
}
```

### Registering an async reaction

Reactions fire after the transaction commits. They run asynchronously and cannot abort the original operation.

```go
// In a monitoring provider that sets up dashboards for new machines:

func (p *MonitoringProvider) Init(ctx provider.Context) error {
    // ...

    ctx.Registry.RegisterReaction(provider.Reaction{
        Feature:  "machines",
        Event:    "post_create",
        Callback: p.setupDashboard,
    })

    return nil
}

func (p *MonitoringProvider) setupDashboard(
    ctx context.Context, payload interface{},
) {
    machine, ok := payload.(*Machine)
    if !ok {
        return
    }
    p.logger.Info().
        Str("machine", machine.Name).
        Msg("setting up monitoring dashboard")
}
```

### Hook naming convention

Hook names follow the pattern `{resource_type}.{pre|post}_{action}`:

| Hook | When it fires |
|------|---------------|
| `machine.pre_create` | Before inserting a machine |
| `machine.post_create` | After inserting a machine (committed) |
| `machine.pre_update` | Before updating a machine |
| `machine.post_update` | After updating a machine (committed) |
| `machine.pre_delete` | Before soft-deleting a machine |
| `machine.post_delete` | After soft-deleting a machine (committed) |

## 5. Deployment Profiles

Profiles control which providers are active in a given deployment. Define profiles in a YAML configuration file:

```yaml
# profiles.yaml
profiles:
  edge:
    description: "FlightCtl edge device management"
    providers:
      - inventory
      - fleet
      - updates
      - firmware

  cloud:
    description: "OSAC sovereign cloud provisioning"
    providers:
      - compute
      - networking
      - storage
      - dns
      - tenancy

  gpu:
    description: "NICo GPU infrastructure management"
    providers:
      - inventory
      - firmware
      - topology
      - scheduling
      - monitoring

  minimal:
    description: "Development and testing"
    providers:
      - inventory
```

Select the active profile via the `INFRACTL_PROFILE` environment variable:

```bash
INFRACTL_PROFILE=edge ./infractl-server
```

When no profile is set, all registered providers are activated. This is useful during development but should not be used in production.

### Adding a provider to a profile

1. Implement the `Provider` interface (see section 1)
2. Register the provider in the application's main function
3. Add the provider name to the desired profile(s) in `profiles.yaml`
4. If the provider depends on features from other providers, ensure those providers are also in the profile

The registry validates at startup that all dependencies within the active profile are satisfiable. A missing dependency causes a startup error with a clear message indicating which feature is needed and which provider requires it.

## 6. External Provider (gRPC Sidecar)

For providers that must run out-of-process -- different language, independent release cycle, crash isolation, or partner-contributed -- infractl supports a gRPC sidecar protocol over Unix domain sockets.

### Architecture

```
infractl-server <--Unix socket--> external-provider-sidecar
```

The core process discovers external providers via a `providers.yaml` manifest:

```yaml
# providers.yaml
external:
  - name: partner-dns
    socket: /run/infractl/partner-dns.sock
    features:
      - dns
    dependencies: []
```

### Implementing an external provider

The external provider runs as a separate process and implements the infractl provider gRPC service:

```protobuf
service ExternalProvider {
    rpc GetInfo(GetInfoRequest) returns (ProviderInfo);
    rpc Init(InitRequest) returns (InitResponse);
    rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
    rpc GetRoutes(GetRoutesRequest) returns (GetRoutesResponse);
    rpc HandleHTTP(HTTPRequest) returns (HTTPResponse);
    rpc GetHookRegistrations(GetHookRegistrationsRequest) returns (HookRegistrationList);
    rpc HandleSyncHook(HookEvent) returns (HookResult);
}
```

The core process proxies HTTP requests and hook events to the external provider over the socket. From the perspective of the rest of the system, external providers are indistinguishable from built-in providers.

### Running an external provider

1. Build the external provider as a standalone binary
2. Configure it in `providers.yaml` with its socket path
3. Start the external provider process before (or alongside) infractl-server
4. infractl-server discovers the socket and integrates the provider

The external provider must create and listen on the Unix domain socket before infractl-server attempts to connect. In Kubernetes deployments, both processes typically run as containers in the same pod, sharing a volume for the socket.

### Error handling

If an external provider's socket is unavailable at startup, the registry returns an error and the server does not start. If the connection drops during runtime, requests routed to that provider return 503 (Service Unavailable) until the connection is re-established. The core process periodically attempts to reconnect.

## Complete Example

The `examples/inventory/` directory contains a complete, runnable provider implementation:

- `provider.go` -- Provider interface implementation with route registration
- Uses `resource.NewGenericStore[Machine]` for typed CRUD
- Registers routes under `/machines` with standard REST verbs
- Demonstrates both `provider.Provider` and `provider.APIProvider` interface satisfaction via compile-time checks

To run the example, start the server with a PostgreSQL database:

```bash
INFRACTL_DB_DSN="host=localhost user=infractl dbname=infractl sslmode=disable" \
    go run ./cmd/infractl-server/
```

This starts the server with the default profile (which includes the inventory provider), serving machine CRUD endpoints under `/api/v1/machines`.

Test it with curl:

```bash
# Create a machine
curl -X POST http://localhost:8080/api/v1/machines \
  -H "Content-Type: application/json" \
  -d '{"name":"test-machine","spec":{"data":{"arch":"x86_64","cpus":4}}}'

# List machines
curl http://localhost:8080/api/v1/machines

# Get a specific machine
curl http://localhost:8080/api/v1/machines/test-machine
```
