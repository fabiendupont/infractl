// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package mixins

type BMCSpec struct {
	BMCAddr     string `json:"bmc_addr,omitempty"`
	BMCUser     string `json:"bmc_user,omitempty"`
	BMCPassword string `json:"bmc_password,omitempty"`
}

type BMCStatus struct {
	PowerState   string `json:"power_state,omitempty"`
	BMCReachable bool   `json:"bmc_reachable"`
}
