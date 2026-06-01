// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/valkey-io/valkey-go"
)

const valkeyChannelPrefix = "infractl:events:"

// ValkeyBus is a Valkey pub/sub-backed event bus.
type ValkeyBus struct {
	client valkey.Client
}

var _ Bus = (*ValkeyBus)(nil)

// NewValkeyBus creates a Valkey-backed event bus.
func NewValkeyBus(client valkey.Client) *ValkeyBus {
	return &ValkeyBus{client: client}
}

// Publish serializes the event to JSON and publishes it to a kind-specific
// Valkey channel.
func (b *ValkeyBus) Publish(ctx context.Context, event Event) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	channel := valkeyChannelPrefix + event.Kind
	cmd := b.client.B().Publish().Channel(channel).Message(string(data)).Build()
	return b.client.Do(ctx, cmd).Error()
}

// Subscribe returns a channel that receives events matching the given kinds.
// Pass an empty or nil slice to receive all events via pattern subscribe.
func (b *ValkeyBus) Subscribe(ctx context.Context, kinds []string) (<-chan Event, error) {
	ch := make(chan Event, 100)

	go func() {
		defer close(ch)

		handler := func(msg valkey.PubSubMessage) {
			var evt Event
			if err := json.Unmarshal([]byte(msg.Message), &evt); err != nil {
				return
			}
			select {
			case ch <- evt:
			case <-ctx.Done():
			}
		}

		if len(kinds) == 0 {
			pattern := valkeyChannelPrefix + "*"
			b.client.Receive(ctx, b.client.B().Psubscribe().Pattern(pattern).Build(), handler)
		} else {
			channels := make([]string, len(kinds))
			for i, k := range kinds {
				channels[i] = valkeyChannelPrefix + k
			}
			b.client.Receive(ctx, b.client.B().Subscribe().Channel(channels...).Build(), handler)
		}
	}()

	return ch, nil
}

// Close closes the underlying Valkey client.
func (b *ValkeyBus) Close() {
	b.client.Close()
}
