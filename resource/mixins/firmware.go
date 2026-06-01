// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package mixins

type FirmwareSpec struct {
	DesiredVersion string `json:"desired_version,omitempty"`
}

type FirmwareStatus struct {
	CurrentVersion string `json:"current_version,omitempty"`
	UpdateState    string `json:"update_state,omitempty"`
}
