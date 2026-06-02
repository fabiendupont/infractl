# infractl Architecture

infractl is a shared Go framework for building infrastructure management services. It extracts common patterns from three production projects -- FlightCtl (edge device management), OSAC (sovereign cloud provisioning), and NICo (GPU infrastructure management) -- into a single reusable module.

This document describes the core layers, their responsibilities, and the design decisions behind them.

## Core Layers

### resource/ -- Generic Resource Model and CRUD Store

The resource layer provides a uniform data model and persistence interface for all managed infrastructure objects.

#### Base Resource Struct

Every resource in infractl embeds a common base structure:

```go
type Resource struct {
    OrgID           uuid.UUID       // Tenant identifier, scopes all queries
    Name            string          // Unique within an org
    Labels          JSONMap          // Key-value pairs for filtering
    Annotations     JSONMap          // Arbitrary metadata
    Generation      int64           // Incremented on spec changes
    ResourceVersion int64           // Incremented on any write (optimistic concurrency)
    CreatedAt       time.Time
    UpdatedAt       time.Time
    DeletedAt       *time.Time      // Soft delete
}
```

Spec and Status are stored as JSONB columns in PostgreSQL. Go generics parameterize the concrete types:

```go
type Machine struct {
    resource.Resource
    Spec   MachineSpec   `gorm:"type:jsonb"`
    Status MachineStatus `gorm:"type:jsonb"`
}
```

The composite key is `(OrgID, Name)`, enforcing name uniqueness within a tenant while allowing the same name across tenants.

#### GenericStore

`GenericStore[R Resource]` provides typed CRUD operations over any resource that embeds `resource.Resource`:

- **Create** -- inserts with `OrgID` set by the caller, sets initial `Generation` and `ResourceVersion` to 1. Returns `ErrAlreadyExists` on duplicate `(OrgID, Name)`
- **Get** -- retrieves by `(OrgID, Name)`, returns `ErrNotFound` for missing or soft-deleted resources
- **List** -- paginated listing with filter expressions, cursor-based pagination, and label selectors
- **Update** -- conditional update using `ResourceVersion` for optimistic concurrency; returns `ErrConflict` on version mismatch; increments `Generation` if spec changed (detected via `GenerationTracker` interface)
- **Delete** -- soft delete by setting `DeletedAt`, preserving audit trail

All store operations automatically scope queries to the caller's `OrgID`, making tenant isolation a structural guarantee rather than a per-query concern.

#### Filter Expressions

List operations accept filter strings that the store translates to parameterized SQL WHERE clauses:

```
name = "web-01"                       // Exact match
name != "temp"                        // Not equal
labels.env = "production"             // Label selector
name = "web-01" AND labels.env = "prod"  // Compound with AND
name = "a" OR name = "b"             // Compound with OR
```

Supported operators: `=`, `!=`, `>`, `<`, `>=`, `<=`. Logical operators: `AND`, `OR`. Label selectors use the `labels.key = value` syntax.

#### Pagination

The store uses cursor-based pagination with continue tokens rather than offset-based pagination. This avoids the consistency problems of OFFSET (missed or duplicate items when data changes between pages) and performs better on large datasets. Each list response includes a `continueToken` that encodes the position for the next page.

#### Derivation

This layer synthesizes patterns from:
- FlightCtl's `internal/store/generic.go` (440 lines): GenericStore with type parameters over GORM
- FlightCtl's `internal/store/model/resource.go`: composite key `(OrgID, Name)`, JSONB labels/annotations
- FlightCtl's `internal/store/model/jsonfield.go`: `JSONField[T]` and `JSONMap[K,V]` wrappers
- OSAC's `internal/database/dao/generic_dao.go` (380 lines): builder pattern, CEL filter translation

### api/ -- HTTP Server Scaffolding

The API layer provides the HTTP server skeleton, routing, middleware, and generic handler factories.

#### Router and Middleware

infractl uses chi as its HTTP router. The middleware chain executes in this order for every request:

1. **Panic recovery** -- catches panics, logs stack trace, returns 500
2. **Request logging** -- structured log entry with method, path, status, duration
3. **Metrics** -- Prometheus histogram for request duration, counter for status codes
4. **Authentication** -- extracts and validates credentials, populates context with Subject
5. **Tenant resolution** -- determines OrgID from Subject, sets it in context

#### Generic CRUD Handler Factory

Given a resource type and its store, the handler factory generates standard REST handlers:

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/{resources}` | List | Paginated list with filters |
| GET | `/{resources}/{name}` | Get | Single resource by name |
| POST | `/{resources}` | Create | Create new resource |
| PUT | `/{resources}/{name}` | Update | Full resource update |
| DELETE | `/{resources}/{name}` | Delete | Soft delete |

Providers can override any generated handler or add custom endpoints alongside the standard ones.

#### OpenAPI Fragment Registration

Each provider contributes an OpenAPI spec fragment describing its endpoints. The API layer merges all fragments into a unified OpenAPI specification served at `/openapi.json`. This keeps API documentation co-located with the provider that implements it.

#### Derivation

- FlightCtl's oapi-codegen pattern (generated handlers from OpenAPI spec)
- NICo's Echo-based provider route registration (providers register their own routes)

### auth/ -- Authentication, Authorization, Tenancy

The auth layer handles identity verification, access control, and tenant scoping.

#### Authentication (AuthN)

Authentication is pluggable through the `Authenticator` interface. Supported backends:

- **Keycloak OIDC** -- validates JWT tokens against a Keycloak realm, extracts user identity and group memberships, caches JWKS keys with TTL
- **Guest mode** -- development-only backend that accepts all requests with a configurable default identity

Each authenticator produces a `Subject` that flows through the rest of the request.

#### Subject Model

```go
type Subject struct {
    User    string      // Authenticated user identifier
    Tenants TenantSet   // Set of OrgIDs the user belongs to
}
```

`TenantSet` supports a universal set representation for admin users who have access to all tenants, avoiding the need to enumerate every tenant ID.

#### Authorization (AuthZ)

Authorization evaluates policies of the form `(subject, resource, action) -> allow|deny`. The policy engine is pluggable, supporting OPA (Open Policy Agent) evaluation or simple role-based rules.

#### Tenancy Model

The tenancy system defines three dimensions for multi-tenant access:

- **Assignable tenants** -- the set of tenants to which a user can assign ownership of new resources
- **Default tenants** -- the tenant(s) automatically assigned when a user creates a resource without specifying one
- **Visible tenants** -- the set of tenants whose resources a user can read (superset of assignable)

Two special tenants exist:
- **System tenant** -- internal resources (migrations, system state) invisible to regular users
- **Shared tenant** -- resources visible to all users in read-only mode (e.g., base images, network classes)

The `TenancyLogic` interface encapsulates these rules:

```go
type TenancyLogic interface {
    DetermineAssignableTenants(subject Subject) TenantSet
    DetermineDefaultTenants(subject Subject) TenantSet
    DetermineVisibleTenants(subject Subject) TenantSet
}
```

#### Derivation

- OSAC's `internal/auth/tenancy_logic.go`: TenancyLogic interface with the three-dimensional model
- OSAC's `internal/auth/auth_subject.go`: Subject with universal set support
- FlightCtl's `internal/auth/middleware.go`: chi middleware pattern for authn+authz

### provider/ -- Provider Framework

The provider framework is infractl's extension mechanism. All domain-specific logic lives in providers; the core framework is intentionally generic.

#### Provider Interface

Every provider implements the base `Provider` interface:

```go
type Provider interface {
    Name() string
    Version() string
    Features() []string
    Dependencies() []string
    Init(ctx Context) error
    Shutdown(ctx context.Context) error
}
```

- **Name** -- unique identifier for the provider (e.g., `"inventory"`, `"networking"`)
- **Version** -- semantic version of the provider
- **Features** -- capabilities this provider supplies (e.g., `"compute"`, `"dns"`)
- **Dependencies** -- features this provider requires from other providers
- **Init** -- called during startup with a `Context` containing the database, logger, hook runner, and other shared services
- **Shutdown** -- called during graceful shutdown

#### Capability Interfaces

Providers optionally implement additional interfaces to contribute specific capabilities:

```go
type APIProvider interface {
    Provider
    RegisterRoutes(r chi.Router)
}

type GRPCProvider interface {
    Provider
    RegisterServices(s *grpc.Server)
}

type WorkflowProvider interface {
    Provider
    RegisterWorkflows(registry interface{})
}

type MigrationProvider interface {
    Provider
    MigrationSource() fs.FS
}
```

- **APIProvider** -- contributes HTTP routes to the chi REST API server
- **GRPCProvider** -- contributes gRPC services to the gRPC server
- **WorkflowProvider** -- registers async workflows with a workflow engine (Temporal, AAP, in-process, or other). The registry type is intentionally `interface{}` to avoid coupling infractl to a specific engine.
- **MigrationProvider** -- contributes database migrations (returns an `fs.FS` containing SQL migration files)

#### Registry

The provider registry handles:

1. **Registration** -- providers register themselves; the registry detects feature conflicts (two providers claiming the same feature)
2. **Dependency resolution** -- DFS-based topological sort ensures providers initialize in dependency order
3. **Lifecycle management** -- init in resolved order, shutdown in reverse order

#### Profiles

Deployment profiles select which providers are active for a given deployment. The `INFRACTL_PROFILE` environment variable selects the active profile. Profiles are defined in YAML:

```yaml
profiles:
  edge:
    providers: [inventory, fleet, updates]
  cloud:
    providers: [compute, networking, storage, dns]
  gpu:
    providers: [inventory, firmware, topology, scheduling]
```

This allows the same binary to serve different products by activating different provider sets.

#### Hook System

Hooks enable cross-provider integration without direct dependencies:

- **SyncHook** -- inline execution; the hook handler can inspect the operation and return an error to abort it. Example: a storage provider blocks instance deletion if volumes are still attached.
- **Reaction** -- asynchronous fire-and-forget execution via a callback. Example: a monitoring provider reacts to instance creation by setting up dashboards.

```go
type HookFirer interface {
    FireSync(ctx context.Context, feature, event string, payload interface{}) error
    FireAsync(ctx context.Context, feature, event string, payload interface{})
}
```

Hooks are registered on the provider registry by feature and event name:

```go
registry.RegisterHook(provider.SyncHook{
    Feature: "machines",
    Event:   "pre_delete",
    Handler: func(ctx context.Context, payload interface{}) error { ... },
})

registry.RegisterReaction(provider.Reaction{
    Feature:  "machines",
    Event:    "post_create",
    Callback: func(ctx context.Context, payload interface{}) { ... },
})
```

Sync hooks run in the caller's transaction; if any hook returns an error, the entire operation rolls back. Reactions are dispatched after the transaction commits.

#### External Providers

For out-of-process or partner-contributed providers, infractl supports a gRPC sidecar protocol over Unix domain sockets. The core process discovers external providers via a `providers.yaml` manifest and communicates with them over the socket. External providers implement the same `Provider` interface semantics but in a separate process, enabling:

- Language-agnostic provider implementation
- Independent deployment and versioning
- Crash isolation from the core process

#### Capability Discovery

`GET /capabilities` returns a map of all registered features and their status (provider name, version, health). This endpoint enables clients and operators to discover what a given deployment supports.

#### Derivation

- NICo's `provider/provider.go`: Provider, APIProvider, WorkflowProvider, MigrationProvider interfaces
- NICo's `provider/registry.go` (198 lines): topological sort, feature conflict detection
- NICo's `provider/context.go`: ProviderContext with shared services
- NICo's `provider/hooks.go`: SyncHook + Reaction, HookRunner
- NICo's `provider/profiles.go`: environment-driven provider selection
- NICo's `provider/external.go` (251 lines): gRPC sidecar protocol over Unix domain sockets
- NICo's `provider/discovery.go`: providers.yaml, socket discovery

### events/ -- Event System

The event system provides real-time notifications and an audit trail for resource lifecycle changes.

#### PostgreSQL NOTIFY/LISTEN

infractl uses PostgreSQL's built-in NOTIFY/LISTEN mechanism for event propagation. When a resource is created, updated, or deleted, the store emits a notification on a channel named after the resource type. Listeners in the same or other processes receive these notifications in near-real-time without polling.

#### CRUD Lifecycle Events

The event system defines hooks at each stage of the resource lifecycle:

- **PreCreate / PostCreate** -- before and after resource insertion
- **PreUpdate / PostUpdate** -- before and after resource modification
- **PreDelete / PostDelete** -- before and after resource soft-deletion

Pre-hooks can abort the operation by returning an error. Post-hooks receive the committed state.

#### Event Store

All events are persisted to an event store table for audit purposes. Each event record includes the resource type, resource name, OrgID, action, actor (Subject), timestamp, and a snapshot of the change.

#### Derivation

- OSAC's `internal/database/database_notifier.go`: PostgreSQL NOTIFY with protobuf payloads
- NICo's `provider/hooks.go`: hook lifecycle pattern

### work/ -- Async Task Execution

The work layer handles background processing that cannot complete within an HTTP request lifecycle.

#### Work Loop

The work loop runs a function at a configurable interval. It supports a kick-to-wake mechanism: when an event arrives that requires immediate processing, the loop can be kicked to skip the remaining sleep interval and execute immediately.

```go
type WorkLoop struct {
    interval time.Duration
    fn       func(ctx context.Context) error
    kick     chan struct{}
}
```

This pattern avoids both the waste of tight polling and the latency of long intervals.

#### Task Queue

For discrete units of work, the task queue provides DB-backed job management:

- **Enqueue** -- add a task with a type, payload, and optional scheduling constraints
- **Claim** -- atomically claim the next available task (prevents duplicate processing across replicas)
- **Complete** -- mark a task as successfully finished
- **Fail** -- mark a task as failed with an error message and optional retry policy

The database backing ensures tasks survive process restarts and can be distributed across multiple worker replicas.

#### Derivation

- OSAC's `internal/work/work_loop.go` (215 lines): interval-based execution with kick-to-wake
- FlightCtl's `internal/tasks/consumer.go`: task queue with claim semantics

## Provider Lifecycle

Providers move through a well-defined lifecycle during application startup and shutdown:

```
register -> resolve dependencies -> init (in dependency order) -> run -> shutdown (reverse order)
```

1. **Register** -- each provider registers itself with the registry, declaring its name, features, and dependencies
2. **Resolve** -- the registry performs topological sort on the dependency graph; circular dependencies cause a startup error
3. **Init** -- providers initialize in dependency order, receiving a `Context` with shared services (database, logger, hooks)
4. **Run** -- the application serves requests, routing to the appropriate provider handlers
5. **Shutdown** -- on SIGTERM/SIGINT, providers shut down in reverse initialization order, allowing dependent providers to drain before their dependencies disappear

## Resource Lifecycle

Resources follow a standard state machine:

```
pending -> progressing -> ready
                       -> failed -> progressing (retry)
                       -> deleting
```

- **Pending** -- resource created, awaiting processing
- **Progressing** -- provider is actively reconciling the resource toward its desired state
- **Ready** -- resource matches its desired state and is operational
- **Failed** -- reconciliation encountered an error; may transition back to progressing on retry
- **Deleting** -- soft-delete initiated, provider is cleaning up external resources before final removal

Providers set these states via the resource's Status field. The framework does not enforce state transitions but provides the model; providers implement the reconciliation logic.

## Tenancy Model

All resources are scoped by `OrgID`, which serves as the tenant identifier.

### Three Dimensions

The tenancy model defines three overlapping sets for each authenticated user:

- **Assignable** -- tenants to which the user can assign new resources. A project administrator might be assignable to their project's tenant only.
- **Default** -- the tenant automatically selected when creating a resource without explicit tenant specification. Typically a single tenant for regular users.
- **Visible** -- tenants whose resources the user can read. Always a superset of assignable. Administrators might see all tenants.

### Special Tenants

- **System** -- holds internal framework state (migration records, system configuration). Never visible to regular users. Only the framework itself and system administrators interact with system-tenant resources.
- **Shared** -- holds platform-provided resources visible to all users in read-only mode. Examples include base VM images, network classes, or public IP pools. Users can reference shared-tenant resources but cannot modify or delete them.

### Query Scoping

Every database query passes through the store layer, which injects `WHERE org_id IN (visible_tenants)` for reads and `WHERE org_id = assignable_tenant` for writes. This scoping is automatic and cannot be bypassed by providers, eliminating an entire class of tenant isolation bugs.
