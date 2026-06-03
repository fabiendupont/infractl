# infractl

Extensible framework for building infrastructure management services.

## Motivation

Three infrastructure management projects -- FlightCtl (edge devices), OSAC (sovereign cloud), and NICo (GPU infrastructure) -- each reimplement the same foundational patterns: generic CRUD stores with PostgreSQL/JSONB, auth and tenancy middleware, event-driven lifecycle hooks, REST API scaffolding, and background work loops. infractl extracts these patterns into a shared framework so teams focus exclusively on domain logic, implemented as pluggable providers.

## Architecture

```
┌───────────────────────────────────────────────────────────┐
│                      infractl core                        │
│  ┌──────────┐ ┌──────┐ ┌──────┐ ┌────────────┐ ┌──────┐ │
│  │ resource/ │ │ api/ │ │auth/ │ │  provider/  │ │ grpc/│ │
│  │  store    │ │server│ │tenant│ │  registry   │ │server│ │
│  │  proto    │ └──────┘ └──────┘ │  hooks      │ │ gw   │ │
│  └──────────┘                    │  profiles   │ └──────┘ │
│  ┌──────────┐ ┌──────┐          └────────────┘           │
│  │ events/  │ │work/ │ ┌───────────┐ ┌──────────────┐    │
│  │  bus     │ │ loop │ │ workflow/ │ │  platform/   │    │
│  └──────────┘ │queue │ │ dispatch  │ │ tenant event │    │
│               └──────┘ │ executor  │ │ secret task  │    │
│                         └───────────┘ │ webhook policy│   │
│                                       └──────────────┘    │
└───────────────────────────────────────────────────────────┘
                          │
            ┌─────────────┼─────────────┐
            ▼             ▼             ▼
     ┌─────────────┐ ┌──────────┐ ┌──────────┐
     │   edge/     │ │  cloud/  │ │   gpu/   │
     │  device     │ │ network  │ │ compute  │
     │  fleet      │ │ cluster  │ │ fabric   │
     │  (FlightCtl)│ │  (OSAC)  │ │  (NICo)  │
     └─────────────┘ └──────────┘ └──────────┘
```

The core provides generic resource storage, API scaffolding, auth/tenancy enforcement, an event bus, background work loops, workflow dispatch, gRPC serving, and built-in platform providers for common infrastructure concerns. Domain functionality is implemented in **providers** that register resource types, route handlers, workflow actions, and lifecycle hooks with the core.

## Packages

| Package | Description |
|---------|-------------|
| `resource/` | Generic resource model and CRUD store (GORM/PostgreSQL, JSONB spec/status, parent nesting, finalizers, soft delete) |
| `resource/proto/` | Protobuf adapter — shared `Metadata` message, `MetadataToProto`/`MetadataFromProto` converters |
| `api/` | HTTP server, middleware, generic CRUD handlers (chi router, OpenAPI, Prometheus metrics) |
| `auth/` | Authentication (Keycloak, `ContextAuthenticator` for gRPC), authorization (OPA), tenancy, attribution |
| `provider/` | Provider interface, registry, hooks, profiles, discovery, external gRPC sidecar protocol |
| `events/` | Event bus (in-memory, PostgreSQL NOTIFY, Valkey PUBLISH) and event store |
| `work/` | Background work loops, PostgreSQL-backed task queue with retry and stale recovery |
| `workflow/` | Dispatch table, executor interface, local executor, dispatcher (wires hooks to execution) |
| `grpc/` | gRPC server, REST gateway, auth interceptors, generic service handler |
| `platform/` | Built-in providers: tenant, event, secret, task, webhook, policy |

## Quick Start

A provider defines a resource type and registers HTTP routes:

```go
package inventory

import (
    "context"

    "github.com/go-chi/chi/v5"
    "github.com/rs/zerolog"
    "gorm.io/gorm"

    "github.com/fabiendupont/infractl/provider"
    "github.com/fabiendupont/infractl/resource"
)

type InventoryProvider struct {
    db     *gorm.DB
    store  resource.Store[Machine]
    logger zerolog.Logger
}

func New() *InventoryProvider { return &InventoryProvider{} }

func (p *InventoryProvider) Name() string           { return "inventory" }
func (p *InventoryProvider) Version() string        { return "0.1.0" }
func (p *InventoryProvider) Features() []string     { return []string{"inventory"} }
func (p *InventoryProvider) Dependencies() []string { return nil }

func (p *InventoryProvider) Init(ctx provider.Context) error {
    p.db = ctx.DB
    p.logger = ctx.Logger.With().Str("provider", p.Name()).Logger()
    p.store = resource.NewGenericStore[Machine](ctx.DB)
    return ctx.DB.AutoMigrate(&Machine{})
}

func (p *InventoryProvider) Shutdown(_ context.Context) error { return nil }

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

See `examples/inventory/` for the complete reference implementation including model definition, handler implementations, and OpenAPI spec.

## Build / Run / Test

```bash
# Build all packages
go build ./...

# Run unit tests (no external dependencies)
go test $(go list ./... | grep -v /tests/) -count=1

# Run integration and e2e tests (requires Docker or Podman)
# With Podman, set DOCKER_HOST and disable Ryuk:
export DOCKER_HOST=unix:///run/user/1000/podman/podman.sock
export TESTCONTAINERS_RYUK_DISABLED=true
go test ./tests/integration/ ./tests/e2e/ -v -count=1

# Start the server (requires PostgreSQL)
INFRACTL_DB_DSN="host=localhost user=infractl dbname=infractl sslmode=disable" \
    go run ./cmd/infractl-server/

# Use the CLI
go run ./cmd/infractl/ machines list
go run ./cmd/infractl/ capabilities
```

## Platform Providers

infractl ships with six built-in providers that cover common infrastructure management concerns:

| Provider | Description |
|----------|-------------|
| `platform/tenant` | System tenant (well-known UUID), global tenant CRUD |
| `platform/event` | Read-only access over persisted event records |
| `platform/secret` | Typed secrets with redacted GET and `/reveal` endpoint |
| `platform/task` | Read-only view over task records with `/cancel` |
| `platform/webhook` | Event subscriptions and delivery loop |
| `platform/policy` | RBAC rules managed as resources |

## Workflow Dispatch

The `workflow/` package provides a dispatch table and executor model for running actions triggered by resource lifecycle events. A `DispatchTable` maps (ResourceType, Event, Phase) to prioritized handlers. The `Dispatcher` wires lifecycle hooks to an `Executor` (local in-process or remote via the AAP executor). Phases are pre, main, and post -- sync hooks can abort, async hooks fire after commit.

## Consumer Projects

| Project | Description |
|---------|-------------|
| [osac-infractl](https://github.com/osac-project/osac-workspace/tree/main/osac-infractl) | 10 domain providers for sovereign cloud provisioning, built on infractl |
| [infractl-executor-aap](https://github.com/fabiendupont/infractl-executor-aap) | AAP Controller executor with OAuth2, mTLS, retry, and circuit breaker |

## Documentation

| Document | Description |
|----------|-------------|
| [docs/architecture.md](docs/architecture.md) | Core layers, design decisions, and derivation from source projects |
| [docs/provider-guide.md](docs/provider-guide.md) | Step-by-step guide to building a provider |
| [docs/source-projects.md](docs/source-projects.md) | Mapping of framework components to source implementations |
| [docs/enhancements/](docs/enhancements/) | Design proposals and RFCs |
| [CLAUDE.md](CLAUDE.md) | Development instructions and conventions |
| [IMPLEMENTATION.md](IMPLEMENTATION.md) | Implementation progress and task tracking |
| [examples/inventory/](examples/inventory/) | Reference provider implementation |

## Source Projects

infractl distills patterns from three production systems:

| Project | Domain | Repository |
|---------|--------|------------|
| FlightCtl | Edge device management | [flightctl/flightctl](https://github.com/flightctl/flightctl) |
| OSAC | Sovereign cloud provisioning | [osac-project/fulfillment-service](https://github.com/osac-project/fulfillment-service) |
| NICo | GPU infrastructure | [NVIDIA/ncx-infra-controller-rest](https://github.com/NVIDIA/ncx-infra-controller-rest) |

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
