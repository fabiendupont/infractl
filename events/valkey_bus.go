// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"

	"github.com/valkey-io/valkey-go"
)

var errValkeyBusNotImplemented = errors.New("valkey bus: not implemented")

// ValkeyBus is a Valkey pub/sub-backed event bus. This is a stub that
// validates the Bus interface; the implementation is not yet wired.
type ValkeyBus struct {
	client valkey.Client
}

var _ Bus = (*ValkeyBus)(nil)

// NewValkeyBus creates a Valkey-backed event bus stub.
func NewValkeyBus(client valkey.Client) *ValkeyBus {
	return &ValkeyBus{client: client}
}

// Publish would PUBLISH the event JSON to a Valkey channel.
func (b *ValkeyBus) Publish(_ context.Context, _ Event) error {
	return errValkeyBusNotImplemented
}

// Subscribe would SUBSCRIBE to a Valkey channel and forward events.
func (b *ValkeyBus) Subscribe(_ context.Context, _ []string) (<-chan Event, error) {
	return nil, errValkeyBusNotImplemented
}
