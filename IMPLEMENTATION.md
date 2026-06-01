# Implementation Plan

This document tracks the implementation progress of infractl. All foundational tasks are complete. The framework compiles, unit tests pass, and integration/e2e tests are written (require PostgreSQL via testcontainers to run).

Read `docs/source-projects.md` for exact file paths to reference implementations in FlightCtl, OSAC, and NICo that informed each task.

## Current State

- **Compiles**: All packages build and pass `go vet`
- **Unit tests pass**: auth/, provider/, resource/, resource/mixins/, resource/traits/, work/
- **Integration/e2e tests written**: tests/integration/ and tests/e2e/ — require Podman or Docker for testcontainers-go PostgreSQL
- **All core packages implemented**: resource/ (model, store, filter, pagination, mixins, traits), api/ (server, middleware, handlers, OpenAPI), auth/ (authn, authz, tenancy, Keycloak, OPA), provider/ (registry, hooks, profiles, discovery, external via gRPC), events/ (in-memory bus, PostgreSQL bus, Valkey bus, event store), work/ (loops, queue)
- **Entry points**: cmd/infractl-server/ (server binary), cmd/infractl/ (CLI with machines and capabilities commands)
- **Reference provider**: examples/inventory/ (model, handler, provider, OpenAPI spec)

## Completed Tasks

### Task 1: cmd/infractl-server entry point — DONE

`cmd/infractl-server/main.go` and `profiles.go` wire config parsing, GORM/PostgreSQL, provider registry, profile-based provider loading, API server with auth middleware, and graceful shutdown.

### Task 2: GenericStore — DONE

`resource/store.go` implements tenant-scoped queries, optimistic concurrency (ResourceVersion), generation tracking, soft delete, cursor-based pagination, and filter support. Additionally, `resource/mixins/` and `resource/traits/` provide composable resource behaviors.

### Task 3: Filter expressions — DONE

`resource/filter.go` supports comparison operators (`=`, `!=`, `>`, `<`, `>=`, `<=`), logical operators (`AND`, `OR`), and label selectors. Translates to parameterized GORM `Where` clauses.

### Task 4: PostgreSQL-backed event bus — DONE

`events/pg_bus.go` implements NOTIFY/LISTEN with event persistence via `events/pg_store.go`. Additionally, `events/valkey_bus.go` provides a Valkey (Redis-compatible) bus alternative.

### Task 5: PostgreSQL-backed task queue — DONE

`work/queue.go` implements database-backed task queue with `SELECT ... FOR UPDATE SKIP LOCKED`, retry with backoff, and stale task recovery. Integration tests cover concurrent claim, retry exhaustion, and kind filtering.

### Task 6: External provider gRPC protocol — DONE

`provider/proto/infractl/provider/v1/provider.proto` defines 7 RPCs. Generated Go code in `provider.pb.go` and `provider_grpc.pb.go`. `provider/external.go` fully implements `ConnectExternalProvider` (Unix socket dial + GetInfo), Init (route/hook registration), HTTP-to-gRPC proxying, and sync/async hook forwarding.

### Task 7: Integration tests — DONE

`tests/integration/` covers store CRUD, pagination, filtering, optimistic concurrency, generation tracking, soft delete, tenant isolation, PostgreSQL event bus, and task queue. `tests/e2e/` covers full HTTP stack (healthz, CRUD, tenant isolation, auth). All use testcontainers-go for PostgreSQL.

### Task 8: CLI tool — DONE

`cmd/infractl/main.go` with cobra commands for `machines` (list, get, create, delete) and `capabilities`.

### Task 9: Keycloak authenticator — DONE

`auth/keycloak.go` validates JWT Bearer tokens, fetches JWKS from the realm's well-known endpoint, extracts user and tenant claims, and caches keys with TTL. Unit tests in `auth/keycloak_test.go`.

### Task 10: OPA authorizer — DONE

`auth/opa.go` loads Rego policies, evaluates the `allow` rule with `{subject, resource, action}` input, and caches compiled policies. Unit tests in `auth/opa_test.go`.

## OSAC Gap Closure — DONE

All 8 gaps identified in the OSAC compatibility analysis have been closed:

1. **Store metrics** — `InstrumentedStore[R]` with Prometheus histogram per operation
2. **Attribution logic** — `AttributionLogic` interface with `SubjectAttributionLogic` and `GuestAttributionLogic`
3. **Finalizers** — `Finalizers` field on Resource, `DeletionTimestamp`, finalizer-aware Delete
4. **CEL-style filters** — Extended filter parser to accept `==`, `&&`, `||` alongside classic syntax
5. **Field masking** — `PartialUpdate` method on Store for field-level updates with optimistic concurrency
6. **gRPC server** — `grpc/` package with Server, Gateway, auth interceptors, `GRPCProvider` interface
7. **Protobuf adapter** — Shared `Metadata` proto message, `MetadataToProto`/`MetadataFromProto` converters
8. **Generic gRPC server factory** — `GenericServiceHandler[R]` generating CRUD handlers from Store + Adapter

## What's Next

Potential future work (not yet prioritized):

- **Build an OSAC resource as an infractl provider** — pick one resource type (e.g., Tenant) and implement it as a gRPC provider using the new framework to validate the integration end-to-end
- **Example external provider sidecar** — a minimal out-of-process provider to demonstrate the gRPC protocol
- **OpenTelemetry tracing** — distributed tracing for requests across REST and gRPC
- **Additional event bus backends** — NATS, Kafka
- **Full CEL support** — replace the extended parser with `cel-go` AST-based translation for complex expressions

## How to Work

1. Read the referenced source files for patterns
2. Implement with tests
3. Run `go build ./... && go vet ./... && go test ./...` before considering it done
4. Keep implementations minimal — match the existing code style (no comments unless the "why" is non-obvious, no unnecessary abstractions)
