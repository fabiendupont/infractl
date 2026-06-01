// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package mixins

type Address struct {
	Address string `json:"address"`
	Type    string `json:"type"`
	Gateway string `json:"gateway,omitempty"`
	Prefix  int    `json:"prefix"`
}

type NetworkInterfaceSpec struct {
	Name       string `json:"name"`
	MACAddress string `json:"mac_address,omitempty"`
}

type NetworkInterfaceStatus struct {
	Name       string    `json:"name"`
	MACAddress string    `json:"mac_address"`
	Addresses  []Address `json:"addresses,omitempty"`
	State      string    `json:"state"`
}
