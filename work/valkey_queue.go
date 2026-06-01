// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package work

import (
	"context"
	"errors"

	"github.com/valkey-io/valkey-go"
)

var errValkeyQueueNotImplemented = errors.New("valkey queue: not implemented")

// ValkeyQueue is a Valkey-backed task queue. This is a stub that validates
// the Queue interface; the implementation is not yet wired.
// A real implementation would use Valkey Streams or list-based patterns.
type ValkeyQueue struct {
	client valkey.Client
}

var _ Queue = (*ValkeyQueue)(nil)

// NewValkeyQueue creates a Valkey-backed task queue stub.
func NewValkeyQueue(client valkey.Client) *ValkeyQueue {
	return &ValkeyQueue{client: client}
}

// Enqueue would XADD the task to a Valkey stream.
func (q *ValkeyQueue) Enqueue(_ context.Context, _ Task) error {
	return errValkeyQueueNotImplemented
}

// Claim would XREADGROUP from a Valkey consumer group.
func (q *ValkeyQueue) Claim(_ context.Context, _ []string) (*Task, error) {
	return nil, errValkeyQueueNotImplemented
}

// Complete would XACK the task in the Valkey stream.
func (q *ValkeyQueue) Complete(_ context.Context, _ string) error {
	return errValkeyQueueNotImplemented
}

// Fail would re-enqueue or dead-letter the task.
func (q *ValkeyQueue) Fail(_ context.Context, _ string, _ string) error {
	return errValkeyQueueNotImplemented
}
