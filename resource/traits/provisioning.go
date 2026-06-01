// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package traits

type PowerState string

const (
	PowerOn      PowerState = "on"
	PowerOff     PowerState = "off"
	PowerUnknown PowerState = "unknown"
)

type Provisionable interface {
	BMCAddress() string
	PowerState() PowerState
}
