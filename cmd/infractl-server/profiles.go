// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/fabiendupont/infractl/examples/inventory"
	"github.com/fabiendupont/infractl/platform/catalog"
	"github.com/fabiendupont/infractl/platform/event"
	"github.com/fabiendupont/infractl/platform/organization"
	"github.com/fabiendupont/infractl/platform/policy"
	"github.com/fabiendupont/infractl/platform/secret"
	"github.com/fabiendupont/infractl/platform/task"
	"github.com/fabiendupont/infractl/platform/tenant"
	"github.com/fabiendupont/infractl/platform/webhook"
	"github.com/fabiendupont/infractl/provider"
)

func init() {
	provider.RegisterProfileProviders(provider.ProfileDefault, []func() provider.Provider{
		func() provider.Provider { return tenant.New() },
		func() provider.Provider { return organization.New() },
		func() provider.Provider { return catalog.New() },
		func() provider.Provider { return event.New() },
		func() provider.Provider { return secret.New() },
		func() provider.Provider { return task.New() },
		func() provider.Provider { return webhook.New() },
		func() provider.Provider { return policy.New() },
		func() provider.Provider { return inventory.New() },
	})
}
