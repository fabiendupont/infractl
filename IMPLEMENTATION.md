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

## What's Next

Potential future work (not yet prioritized):

- **Run integration/e2e tests end-to-end** with a running Podman/Docker daemon to validate against real PostgreSQL
- **Initial git commit** — the entire codebase is currently untracked
- **Example external provider sidecar** — a minimal out-of-process provider to demonstrate the gRPC protocol
- **CI pipeline** — GitHub Actions or similar for build, vet, unit tests, and integration tests
- **Documentation** — godoc, usage guide, provider authoring guide
- **Additional event bus backends** — NATS, Kafka
- **Metrics and observability** — Prometheus metrics, OpenTelemetry tracing

## How to Work

1. Read the referenced source files for patterns
2. Implement with tests
3. Run `go build ./... && go vet ./... && go test ./...` before considering it done
4. Keep implementations minimal — match the existing code style (no comments unless the "why" is non-obvious, no unnecessary abstractions)
