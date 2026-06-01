# Source Project Mapping

This document maps each infractl framework component to its source implementation in the three reference projects: FlightCtl, OSAC, and NICo. Use this as the primary reference when implementing or modifying framework components.

## Component Mapping

| Framework Component | infractl Package | Source Project | Source File | Key Pattern |
|---|---|---|---|---|
| Generic store | `resource/` | FlightCtl | `internal/store/generic.go` (440 lines) | `GenericStore` with Go type parameters over GORM; Create/Get/List/Update/Delete with automatic OrgID scoping |
| Base resource model | `resource/` | FlightCtl | `internal/store/model/resource.go` | Composite primary key `(OrgID, Name)`; JSONB columns for Labels and Annotations; soft delete via `DeletedAt` |
| JSON field wrappers | `resource/` | FlightCtl | `internal/store/model/jsonfield.go` | `JSONField[T]` for typed JSONB columns; `JSONMap[K,V]` for key-value JSONB; implements `database/sql` Scanner/Valuer |
| Store interface | `resource/` | FlightCtl | `internal/store/store.go` | `DataStore` interface aggregating per-resource stores; single point of access for all persistence |
| Auth middleware | `auth/` | FlightCtl | `internal/auth/middleware.go` | chi middleware extracting credentials, running authn+authz, populating context with Subject |
| Generic DAO | `resource/` | OSAC | `internal/database/dao/generic_dao.go` (380 lines) | Builder pattern for query construction; CEL-inspired filter expressions translated to SQL WHERE clauses |
| Tenancy logic | `auth/` | OSAC | `internal/auth/tenancy_logic.go` | `TenancyLogic` interface with `DetermineAssignableTenants`, `DetermineDefaultTenants`, `DetermineVisibleTenants` |
| Auth subject | `auth/` | OSAC | `internal/auth/auth_subject.go` | `Subject` struct with `User` string and `Tenants` set; universal set representation for admin users |
| Event notifier | `events/` | OSAC | `internal/database/database_notifier.go` | PostgreSQL NOTIFY/LISTEN with protobuf-encoded payloads; channel-per-resource-type naming |
| Work loop | `work/` | OSAC | `internal/work/work_loop.go` (215 lines) | Interval-based function execution with kick-to-wake channel for immediate re-execution |
| Provider interface | `provider/` | NICo | `provider/provider.go` | `Provider` base interface; optional `APIProvider`, `WorkflowProvider`, `MigrationProvider` capability interfaces |
| Provider registry | `provider/` | NICo | `provider/registry.go` (198 lines) | Topological sort via DFS for dependency resolution; feature conflict detection on registration |
| Provider context | `provider/` | NICo | `provider/context.go` | `Context` struct passed to `Init()` containing DB, Logger, HookRunner, Config; shared services for providers |
| Hook system | `provider/` | NICo | `provider/hooks.go` | `SyncHook` (inline, can abort) + `Reaction` (async, fire-and-forget); `HookRunner` with `FireSync`/`FireAsync` |
| Profiles | `provider/` | NICo | `provider/profiles.go` | Environment-driven provider selection via `INFRACTL_PROFILE`; YAML profile definitions |
| External providers | `provider/` | NICo | `provider/external.go` (251 lines) | gRPC sidecar protocol over Unix domain sockets; proxy for HTTP requests and hook events |
| Capability discovery | `provider/` | NICo | `provider/discovery.go` | `providers.yaml` manifest for external provider sockets; `GET /capabilities` endpoint returning feature status map |
| Minimal provider example | `examples/inventory/` | NICo | `providers/firmware/provider.go` (49 lines) | Minimal `Provider` + `APIProvider` implementation; demonstrates interface compliance pattern |
| Add-On model | (design reference) | OSAC | `enhancement-proposals/enhancements/osac-addon/README.md` | Ansible collection convention for infrastructure add-ons; packaging and discovery patterns |

## Source Project Overview

### FlightCtl

FlightCtl is an edge device management platform. Its internal packages provide mature implementations of generic data storage, resource modeling, and authentication middleware.

Key characteristics of FlightCtl's codebase relevant to infractl:
- Heavy use of Go generics for type-safe store operations
- GORM as the ORM layer with PostgreSQL
- chi router for HTTP handling
- oapi-codegen for OpenAPI-first API development
- Composite `(OrgID, Name)` keys for multi-tenant resource identity

### OSAC (Open Sovereign AI Cloud)

OSAC is a fulfillment system for provisioning Kubernetes clusters and compute instances. Its contributions to infractl center on multi-tenancy, event notification, and asynchronous work patterns.

Key characteristics:
- Sophisticated multi-tenant model with three-dimensional tenant scoping
- CEL-inspired filter expressions for flexible querying
- PostgreSQL NOTIFY/LISTEN for event propagation
- Work loop pattern for background reconciliation
- Builder pattern for composable query construction

### NICo

NICo is a GPU infrastructure management platform. Its provider framework is the most directly reusable component, providing the extension architecture that infractl adopts.

Key characteristics:
- Provider-based architecture with 14 providers in production
- Approximately 2,000 lines of provider framework code
- Topological dependency resolution for provider initialization
- SyncHook + Reaction pattern for cross-provider integration
- gRPC sidecar protocol for external (out-of-process) providers
- Environment-driven deployment profiles

## Design Decisions by Source

The following table explains why specific source implementations were chosen over alternatives when multiple projects provided similar functionality.

| Decision | Chosen Source | Rationale |
|---|---|---|
| Store layer | FlightCtl | Most mature generic store with comprehensive optimistic concurrency and GORM integration |
| Filter expressions | OSAC | CEL-inspired syntax is more expressive than FlightCtl's label-only filtering |
| Tenancy model | OSAC | Three-dimensional model (assignable/default/visible) covers more access patterns than simple OrgID scoping |
| Provider framework | NICo | Production-proven with 14 providers; includes hooks, profiles, and external provider support |
| Auth middleware | FlightCtl | Clean chi middleware pattern with pluggable backends |
| Event system | OSAC | PostgreSQL NOTIFY avoids external message broker dependency |
| Work loop | OSAC | Simple, effective pattern that covers most background processing needs without a full job framework |
