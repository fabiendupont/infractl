// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package traits

type AddressType string

const (
	AddressIPv4 AddressType = "ipv4"
	AddressIPv6 AddressType = "ipv6"
)

type Address struct {
	Address string
	Type    AddressType
	Gateway string
	Prefix  int
}

type NetworkInterface struct {
	Name       string
	MACAddress string
	Addresses  []Address
}

type Networkable interface {
	NetworkInterfaces() []NetworkInterface
}
