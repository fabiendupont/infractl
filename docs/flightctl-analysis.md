# FlightCtl → infractl Analysis

This document analyzes how infractl could serve as the framework for FlightCtl, mapping each FlightCtl subsystem to its infractl equivalent and identifying gaps.

## Executive Summary

infractl provides a strong foundation for FlightCtl's resource model, store, API, auth, and background work layers — these are nearly identical in design. infractl's existing Valkey queue backend is wire-compatible with FlightCtl's Redis Streams usage. The remaining gaps are domain-specific: fleet rollout orchestration (state machine) and device config templating (Git/OCI dependency resolution). These belong in FlightCtl providers, not framework changes. **No infractl modifications are needed.**

## Resource Model — Direct Match

| FlightCtl | infractl | Match |
|-----------|----------|-------|
| Composite PK (OrgID, Name) | Same | Exact |
| Labels, Annotations | Same | Exact |
| Generation, ResourceVersion | Same | Exact |
| Owner (kind/name reference) | Owner *string + Parent *string | Close |
| Soft delete (DeletedAt) | Same (GORM) | Exact |
| Finalizers | Same | Exact |

FlightCtl resources: Device, Fleet, Repository, TemplateVersion, EnrollmentRequest, CertificateSigningRequest, Catalog, ResourceSync, Event.

All of these fit infractl's `resource.Resource` model. Each would be a provider.

## Store Layer — Close, Needs Adapter

FlightCtl's `GenericStore[P, M, A, AL]` has four type parameters (pointer model, model, API type, API list type) with bidirectional conversion functions. infractl's `GenericStore[R]` is simpler — one type parameter, operates on the domain type directly.

**Gap:** FlightCtl's three-layer conversion (API ↔ Domain ↔ Model) adds flexibility for API versioning (v1alpha1, v1beta1). infractl assumes R is the canonical type.

**Solution:** FlightCtl providers would implement the conversion in their handlers, not in the store. The store operates on the domain type; the API handler converts to/from versioned API types. This is the same pattern OSAC uses.

## API Layer — Same Framework, Different Generation

Both use chi. FlightCtl generates handlers from OpenAPI specs via `oapi-codegen`. infractl uses manual handlers or the generic CRUD factory.

**Gap:** FlightCtl supports parallel API versions (v1alpha1, v1beta1). infractl mounts all routes under one prefix.

**Solution:** FlightCtl can mount versioned chi subrouters:
```go
r.Route("/api/v1alpha1", func(r chi.Router) { ... })
r.Route("/api/v1beta1", func(r chi.Router) { ... })
```
This works with infractl's `APIProvider.RegisterRoutes(chi.Router)` — each versioned handler set is a separate provider or the same provider registering multiple route groups.

## Auth / Tenancy — Close

| FlightCtl | infractl | Match |
|-----------|----------|-------|
| OrgID scoping on every query | Same | Exact |
| Multi-auth (TLS, OIDC, token) | Authenticator interface (Keycloak, Guest) | Close |
| K8s/OpenShift RBAC | Authorizer interface (OPA) | Close |
| Device mTLS auth | ContextAuthenticator | Addressable |

**Gap:** FlightCtl routes auth to different backends per issuer URL. infractl has a single Authenticator. FlightCtl also authenticates devices via mTLS certificate fingerprints.

**Solution:** A `MultiAuthenticator` that delegates to the right backend based on the request. Device mTLS auth would be a `CertificateAuthenticator` implementing infractl's `Authenticator` interface. Both are FlightCtl-specific implementations, not framework changes.

## Background Work — Gap

FlightCtl uses Redis Streams with a queue provider abstraction:

```go
type Provider interface {
    NewQueueConsumer(ctx, queueName) (QueueConsumer, error)
    NewQueueProducer(ctx, queueName) (QueueProducer, error)
}
```

Tasks are dispatched via events: `EventWithOrgId` arrives on a queue → task consumer routes to handler by Kind/Reason. Handlers include fleet rollout, selector matching, device rendering, resource sync.

infractl already has `work.ValkeyQueue` using Valkey Streams (`XADD`/`XREADGROUP`/`XACK`). Valkey is the open-source fork of Redis — wire-compatible, same protocol, same commands. FlightCtl's Redis Streams usage is directly supported by infractl's existing Valkey backend.

**Event-driven task dispatch:** FlightCtl's `dispatchTasks` function routes events to handlers. This maps to infractl's `workflow.DispatchTable` — the resource type + event determines which handler runs. FlightCtl would use `WorkflowProvider.RegisterActions()` to register its task handlers (fleet rollout, device render, etc.) and infractl's dispatcher would route events to them.

## Agent Communication — Addressable

FlightCtl devices call home via HTTP:
- `GET /devices/{name}/rendered` — pull config
- `PUT /devices/{name}/status` — push status

This is standard REST over the device's resource. In infractl:
- The `Get` handler serves rendered config
- The `PartialUpdate` or a custom status endpoint receives device status
- Device auth via `CertificateAuthenticator` (mTLS)

**No framework change needed** — FlightCtl's device transport is just additional API routes on the Device provider.

## Fleet Rollout — Domain-Specific

FlightCtl's rollout orchestration is complex:
- Batched strategy (sequential/parallel)
- Disruption budgets
- RolloutStarted → BatchDispatched → BatchCompleted → RolloutCompleted
- Selector-based device matching → ownership assignment

This is domain logic that belongs in a FlightCtl provider, not in infractl. The provider would:
- Implement `WorkflowProvider` to register fleet rollout actions
- Use `work.Loop` for the rollout state machine
- Use infractl's hook system to react to device status changes

## Config Templating — Domain-Specific

FlightCtl resolves device configs from:
- Git repositories (cloned, specific commit/branch)
- OCI images
- HTTP endpoints
- Secrets

Then templates them with device labels and renders a final spec.

This is FlightCtl-specific and belongs in a provider. infractl provides no templating — nor should it. The provider would use Go's `text/template` or similar, resolve dependencies via its own clients, and store rendered specs via the store.

## Gap Summary

| Gap | Effort | Where |
|-----|--------|-------|
| Multi-auth routing | Small | FlightCtl provider |
| mTLS device auth | Small | FlightCtl provider |
| API version routing | Small | FlightCtl provider |
| Event-driven task dispatch | Already done | infractl workflow.Dispatcher |
| Queue backend (Redis/Valkey) | Already done | infractl work.ValkeyQueue |
| Fleet rollout orchestration | Medium | FlightCtl provider |
| Config templating + rendering | Medium | FlightCtl provider |
| Device agent transport | Small | FlightCtl provider |

**Framework changes needed:** None. Everything is either already in infractl or belongs in FlightCtl-specific providers.

## Migration Path

1. **Phase 1 — Foundation:** Port Device, Fleet, Repository models to infractl providers. Use infractl's store, API, auth. Verify CRUD + tenant isolation works.

2. **Phase 2 — Background work:** Port task dispatch to WorkflowProvider using infractl's ValkeyQueue (wire-compatible with existing Redis). Port fleet rollout as a work loop.

3. **Phase 3 — Agent transport:** Add device status endpoints. Implement mTLS authenticator. Port config rendering.

4. **Phase 4 — Full migration:** Port remaining resources (TemplateVersion, EnrollmentRequest, CSR, Catalog, ResourceSync). Remove old store/API code.

## What FlightCtl Gets from infractl

- Generic CRUD store with optimistic concurrency — no need to maintain GenericStore
- Tenant isolation as a structural guarantee
- Auth middleware (Keycloak, OPA) — plug in, don't reimplement
- Hook system for cross-provider integration
- Prometheus metrics on store + HTTP — automatic
- gRPC support alongside REST — future-proofing
- Platform providers (tenant, organization, event, secret, task, webhook, policy, catalog) — shared with OSAC and NICo
- Workflow dispatch for async operations
- Finalizers and nesting — already built

## What FlightCtl Keeps

- Fleet rollout state machine
- Device config templating + rendering
- Git/OCI/HTTP dependency resolution
- OpenAPI spec-driven handler generation (oapi-codegen)
- Agent-specific HTTP transport
- Redis Streams integration (until infractl adds Redis backend)
