// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package traits

type FirmwareComponent struct {
	Name           string
	CurrentVersion string
	DesiredVersion string
}

type Firmwareable interface {
	FirmwareComponents() []FirmwareComponent
}
