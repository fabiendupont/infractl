# infractl

Extensible framework for building infrastructure management services.

## Motivation

Three infrastructure management projects -- FlightCtl (edge devices), OSAC (sovereign cloud), and NICo (GPU infrastructure) -- each reimplement the same foundational patterns: generic CRUD stores with PostgreSQL/JSONB, auth and tenancy middleware, event-driven lifecycle hooks, REST API scaffolding, and background work loops. infractl extracts these patterns into a shared framework so teams focus exclusively on domain logic, implemented as pluggable providers.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  infractl core                   │
│  ┌──────────┐ ┌──────┐ ┌──────┐ ┌────────────┐ │
│  │ resource/ │ │ api/ │ │auth/ │ │  provider/  │ │
│  │  store    │ │server│ │tenant│ │  registry   │ │
│  └──────────┘ └──────┘ └──────┘ │  hooks      │ │
│  ┌──────────┐ ┌──────┐          │  profiles   │ │
│  │ events/  │ │work/ │          └────────────┘ │
│  │  bus     │ │ loop │                          │
│  └──────────┘ └──────┘                          │
└─────────────────────────────────────────────────┘
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

The core provides generic resource storage, API scaffolding, auth/tenancy enforcement, an event bus, and background work loops. Domain functionality is implemented in **providers** that register resource types, route handlers, and lifecycle hooks with the core.

## Quick Start

A minimal provider registers a resource type and its API routes:

```go
package inventory

import (
	"github.com/fabiendupont/infractl/provider"
	"github.com/fabiendupont/infractl/resource"
)

const ProviderName = "inventory"

type Provider struct {
	store *resource.Store
}

func init() {
	provider.Register(ProviderName, &Provider{})
}

func (p *Provider) Init(ctx provider.Context) error {
	p.store = ctx.NewStore("hosts", HostSpec{}, HostStatus{})
	ctx.HandleCRUD("/api/v1/hosts", p.store)
	return nil
}

func (p *Provider) Start(ctx provider.Context) error {
	return nil
}

func (p *Provider) Stop() error {
	return nil
}

type HostSpec struct {
	Hostname string `json:"hostname"`
	Address  string `json:"address"`
}

type HostStatus struct {
	State string `json:"state"`
}
```

Import the provider in a profile to include it in a build:

```go
package main

import (
	_ "github.com/fabiendupont/infractl/examples/inventory"
	"github.com/fabiendupont/infractl/api"
)

func main() {
	api.Run()
}
```

## Documentation

| Document | Description |
|----------|-------------|
| [CLAUDE.md](CLAUDE.md) | Development instructions and conventions |
| [docs/source-projects.md](docs/source-projects.md) | Mapping of framework components to source implementations |
| [docs/enhancements/](docs/enhancements/) | Design proposals and RFCs |
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
