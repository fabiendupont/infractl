// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/fabiendupont/infractl/examples/inventory"
	"github.com/fabiendupont/infractl/provider"
)

func init() {
	provider.RegisterProfileProviders(provider.ProfileDefault, []func() provider.Provider{
		func() provider.Provider { return inventory.New() },
	})
}
