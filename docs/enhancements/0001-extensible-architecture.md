# IEP-0001: Extensible Architecture for infractl

- **Status**: Provisional
- **Authors**: Fabien Dupont
- **Created**: 2025-05-15

## Summary

infractl is a shared Go framework for building infrastructure management services. It extracts common patterns (generic CRUD stores, authentication/tenancy middleware, event systems, API scaffolding, async work dispatch) from three production projects into a single reusable core, while domain-specific functionality lives in pluggable providers.

## Motivation

Three infrastructure management projects -- FlightCtl (edge device management), OSAC (sovereign cloud fulfillment), and NICo (GPU infrastructure) -- each independently implement the same foundational patterns:

- Generic CRUD stores with PostgreSQL/JSONB (440-2,000 lines each)
- Authentication and multi-tenant authorization middleware (2,000-8,500 lines each)
- Event notification systems (200-700 lines each)
- API server scaffolding with code generation (1,000-3,000 lines each)
- Background work loops and task dispatch (200-350 lines each)

This duplication means security patches, database upgrades, and auth provider changes must be applied independently across three codebases. Bug fixes in one project's generic store do not benefit the others. Each team bears the maintenance cost of infrastructure code that is identical in purpose but slightly different in implementation.

NICo's extensible-architecture branch demonstrates that a provider-based framework with approximately 2,000 lines of core code can support 14 production providers spanning networking, compute, health monitoring, firmware management, service catalog, and fulfillment. This validates the architectural pattern at scale.

## Goals

1. **Single framework for common infrastructure**: Generic CRUD, auth, tenancy, events, and async work maintained once, used by all projects.
2. **Provider interface as the extension point**: All domain-specific logic (device management, network provisioning, GPU scheduling) lives in providers that implement a standard interface.
3. **Cross-provider integration via hooks**: Sync hooks (pre/post CRUD, can abort operations) and async reactions (fire-and-forget callbacks) enable loose coupling between providers.
4. **External provider protocol**: gRPC sidecar protocol over Unix domain sockets allows out-of-process providers for partners, different languages, or crash isolation.
5. **Deployment profiles**: Environment-driven provider selection enables composable products from the same binary.

## Non-Goals

1. **Not a Kubernetes operator framework**: Providers that need controller-runtime should use it directly. infractl manages the API/store/auth layers, not the reconciliation loop.
2. **Not a workflow engine**: Providers bring their own orchestration (Temporal, AAP, controller-runtime). The core provides work loops for simple background tasks.
3. **Not a UI framework**: infractl provides the API layer. UI components (PatternFly dashboards, CLIs) are built separately on top of the API.
4. **Not an opinionated business logic layer**: The framework enforces structural patterns (tenancy scoping, resource lifecycle) but does not dictate domain logic.

## Design

### Provider Interface

Every provider implements a minimal contract:

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

Optional capability interfaces extend the base:

- **APIProvider**: Adds REST routes via chi router
- **MigrationProvider**: Manages database schema migrations

### Registry

The registry manages provider lifecycle:

1. **Registration**: Stores providers, maps features, detects conflicts (two providers claiming the same feature).
2. **Dependency Resolution**: DFS-based topological sort detects circular dependencies and missing providers.
3. **Initialization**: Providers init in dependency order, receiving a Context with shared services (DB, logger, hook runner, config).
4. **Shutdown**: Reverse initialization order for clean teardown.

### Hook System

Two hook types enable cross-provider integration:

- **SyncHook**: Runs inline within the caller's execution context. Pre-hooks can return an error to abort the operation. Post-hooks that fail cause the caller to fail.
- **Reaction**: Fires asynchronously after the operation. Non-blocking -- errors are logged but do not affect the caller. Used for side effects (monitoring, billing, audit).

Hooks are registered by feature and event name (e.g., "compute:pre-create-instance"). The HookRunner provides FireSync and FireAsync methods.

### Resource Model

All resources share a common base:

- **OrgID** (UUID): Tenant identifier, part of composite primary key
- **Name** (string): Unique within org, part of composite primary key
- **Labels/Annotations**: JSONB key-value maps for metadata
- **Generation/ResourceVersion**: Optimistic concurrency control
- **Spec/Status**: Typed JSONB fields (via Go generics) for resource-specific data
- **CreatedAt/UpdatedAt/DeletedAt**: Lifecycle timestamps with soft delete

The GenericStore provides CRUD operations that automatically scope all queries by OrgID, preventing cross-tenant data leakage.

### Tenancy Model

Three dimensions of tenant scoping (from OSAC):

- **Assignable**: Tenants to which the user can assign new resources
- **Default**: Tenant auto-selected when creating without explicit specification
- **Visible**: Tenants whose resources the user can read (superset of assignable)

Special tenants: "system" (internal, invisible to users) and "shared" (platform resources, read-only for all).

### Deployment Profiles

The INFRACTL_PROFILE environment variable selects which providers load at startup:

```yaml
profiles:
  edge:      [device, fleet, updates, firmware]
  cloud:     [compute, networking, storage, dns, tenancy]
  gpu:       [inventory, firmware, topology, scheduling, monitoring]
  minimal:   [inventory]
```

This enables shipping a single binary that serves different use cases based on configuration.

## Migration Path

### Phase 1: Framework extraction
Extract core patterns from FlightCtl's internal packages into the infractl module. Validate that the abstractions work for FlightCtl's existing resource types (Device, Fleet, Repository).

### Phase 2: FlightCtl refactoring
Refactor FlightCtl to import infractl for store, auth, and events. FlightCtl's device management, fleet orchestration, and agent protocol become providers.

### Phase 3: OSAC providers
Build OSAC's resource types (Cluster, ComputeInstance, VirtualNetwork, Subnet, SecurityGroup, PublicIP) as infractl providers. Retire the standalone fulfillment-service.

### Phase 4: NCP providers
Build GPU-specific providers (GPU compute, InfiniBand fabric, DPU lifecycle, health monitoring) for the NCP Red Hat Stack use case.

## Risks

1. **Abstraction mismatch**: The framework may not fit all three projects equally. Mitigation: start with FlightCtl (most mature) and validate with one additional project before committing all three.

2. **Migration disruption**: Refactoring existing projects to use infractl interrupts active development. Mitigation: phase the migration, starting with new features built as providers while existing code continues working.

3. **Interface evolution**: The Provider interface may need breaking changes as new use cases emerge. Mitigation: version the interface (Provider/v1) and support adapter patterns for backward compatibility.

4. **Performance overhead**: Generic abstractions may introduce overhead compared to hand-tuned implementations. Mitigation: benchmark critical paths (CRUD operations, auth middleware) and optimize the framework's hot paths.

## Graduation Criteria

- At least two projects (FlightCtl + one other) running on infractl core in production
- External provider protocol validated with at least one partner sidecar implementation
- Integration test suite covering: provider lifecycle, hook execution, tenancy isolation, concurrent CRUD operations
- Documentation: architecture guide, provider development tutorial, migration guide for each adopting project
