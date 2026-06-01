# CLAUDE.md

## Project Context

infractl is a shared Go framework for building infrastructure management services. It extracts common patterns from three production projects:

- **FlightCtl** (edge device management) -- github.com/flightctl/flightctl
- **OSAC fulfillment-service** (sovereign cloud provisioning) -- github.com/osac-project/fulfillment-service
- **NICo** (GPU infrastructure management) -- github.com/NVIDIA/ncx-infra-controller-rest (extensible-architecture branch)

Each of these projects reimplements the same foundational patterns: generic CRUD stores, auth/tenancy middleware, event systems, API scaffolding, and background work loops. infractl extracts these into a shared framework so teams only write domain logic as providers.

## Architecture

Provider-based extensible platform. The core provides:

- **Generic resource store** -- GORM/PostgreSQL with JSONB for spec/status fields
- **REST API scaffolding** -- OpenAPI 3.0 specs + oapi-codegen + chi router
- **Auth/tenancy middleware** -- Authentication, authorization, org-scoped isolation
- **Event system** -- PostgreSQL NOTIFY with sync and async hooks
- **Work loops** -- Background task processing and reconciliation queues

Domain functionality lives in **providers** that implement a standard interface and register themselves with the core registry. Providers are composed into deployable binaries via compile-time profiles.

## Build / Test / Lint

```bash
go build ./...          # Build all packages
go test ./...           # Run all tests
go vet ./...            # Static analysis
```

## Critical Rules

- All resources MUST include tenant scoping (`org_id` in every query). Never skip tenant isolation.
- The provider interface is the extension point -- never hardcode domain logic in core packages.
- Use GORM for database access. Spec and status fields use PostgreSQL JSONB columns.
- OpenAPI 3.0 specs define the API -- providers register route fragments with the core router.
- Sign off all commits: `git commit -s`
- Add AI attribution trailer when AI-assisted:
  ```
  Assisted-by: Claude Code <noreply@anthropic.com>
  ```

## Package Structure

```
infractl/
  resource/             Generic resource model and CRUD store
                        Defines the base Resource type with metadata, spec (JSONB),
                        and status (JSONB). Provides a Store interface for typed
                        CRUD operations backed by GORM/PostgreSQL.

  api/                  HTTP server, middleware, generic CRUD handlers
                        Chi-based router with OpenAPI integration. Provides reusable
                        handler factories for standard CRUD endpoints. Middleware
                        chain handles auth, tenancy, logging, and error mapping.

  auth/                 Authentication, authorization, tenancy
                        Token validation, RBAC policy evaluation, and org_id
                        extraction/enforcement. Pluggable backends for different
                        identity providers.

  provider/             Provider interface, registry, hooks, profiles
                        Defines the Provider interface that domain packages implement.
                        Registry manages provider lifecycle (init, start, stop).
                        Profiles select which providers compile into a given binary.
                        Hook points allow providers to react to resource events.

  events/               Event bus and CRUD lifecycle hooks
                        PostgreSQL NOTIFY/LISTEN-based event bus. Sync hooks run
                        in-transaction and can abort operations. Async hooks fire
                        after commit for reactions and side effects.

  work/                 Background work loops and task queues
                        Periodic reconciliation loops and one-shot task queues.
                        Used by providers for background processing like status
                        polling, cleanup, and external system synchronization.

  examples/inventory/   Reference provider implementation
                        Minimal working provider that demonstrates registration,
                        route fragments, store usage, and hook integration.
```

## Implementation Plan

See `IMPLEMENTATION.md` for a prioritized task list of what needs to be implemented. Tasks are ordered by dependency — start from the top.

## Reference Source Projects

See `docs/source-projects.md` for a detailed mapping of framework components to their source implementations in FlightCtl, OSAC, and NICo. Use this when implementing or refactoring core packages to understand the design rationale and edge cases each source project handles.

## Key Design Principles

1. **Small core, pluggable providers.** The framework handles plumbing. All domain logic -- resource types, validation rules, business workflows -- lives in providers.

2. **Compile-time provider registration via profiles.** A profile is a list of providers that get compiled into a single binary. No runtime plugin loading.

3. **Service interfaces for cross-provider access.** Providers access other providers through well-defined service interfaces, never through direct database queries or internal types.

4. **Hook-driven integration.** Sync hooks run within the database transaction and can abort the operation (e.g., validation). Async hooks fire after commit for reactions and side effects (e.g., triggering provisioning). Hooks are the primary integration mechanism between providers and between providers and external systems.

5. **External providers via gRPC sidecar protocol.** Providers that cannot be compiled in (different language, closed source, separate lifecycle) communicate over a standard gRPC sidecar protocol.
