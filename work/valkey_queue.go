// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package work

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/valkey-io/valkey-go"
)

const (
	valkeyStreamPrefix = "infractl:tasks:"
	valkeyGroupName    = "workers"
)

// ValkeyQueue is a Valkey Streams-backed task queue using consumer groups
// for concurrent-safe claiming.
type ValkeyQueue struct {
	client     valkey.Client
	consumerID string

	mu       sync.Mutex
	claimed  map[string]claimedTask // taskID -> stream + message ID
}

type claimedTask struct {
	stream string
	msgID  string
}

var _ Queue = (*ValkeyQueue)(nil)

// NewValkeyQueue creates a Valkey-backed task queue.
func NewValkeyQueue(client valkey.Client) *ValkeyQueue {
	return &ValkeyQueue{
		client:     client,
		consumerID: uuid.New().String(),
		claimed:    make(map[string]claimedTask),
	}
}

// Enqueue adds a task to the Valkey stream for its kind.
func (q *ValkeyQueue) Enqueue(ctx context.Context, task Task) error {
	if task.ID == "" {
		task.ID = uuid.New().String()
	}
	task.Status = TaskPending
	task.CreatedAt = time.Now()

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshaling task: %w", err)
	}

	stream := valkeyStreamPrefix + task.Kind
	q.ensureGroup(ctx, stream)

	cmd := q.client.B().Xadd().Key(stream).Id("*").FieldValue().FieldValue("id", task.ID).FieldValue("data", string(data)).Build()
	return q.client.Do(ctx, cmd).Error()
}

// Claim reads the next pending message from streams matching the given kinds
// using a consumer group.
func (q *ValkeyQueue) Claim(ctx context.Context, kinds []string) (*Task, error) {
	if len(kinds) == 0 {
		return nil, ErrNoTasks
	}

	for _, kind := range kinds {
		stream := valkeyStreamPrefix + kind
		q.ensureGroup(ctx, stream)

		cmd := q.client.B().Xreadgroup().
			Group(valkeyGroupName, q.consumerID).
			Count(1).
			Streams().Key(stream).Id(">").
			Build()

		resp := q.client.Do(ctx, cmd)
		if resp.Error() != nil {
			continue
		}

		messages, err := resp.AsXRead()
		if err != nil || len(messages) == 0 {
			continue
		}

		for streamKey, msgs := range messages {
			if len(msgs) == 0 {
				continue
			}
			msg := msgs[0]

			data, ok := msg.FieldValues["data"]
			if !ok {
				q.ackAndDel(ctx, streamKey, msg.ID)
				continue
			}

			var task Task
			if err := json.Unmarshal([]byte(data), &task); err != nil {
				q.ackAndDel(ctx, streamKey, msg.ID)
				continue
			}

			task.Status = TaskRunning

			q.mu.Lock()
			q.claimed[task.ID] = claimedTask{stream: streamKey, msgID: msg.ID}
			q.mu.Unlock()

			return &task, nil
		}
	}

	return nil, ErrNoTasks
}

// Complete acknowledges and deletes the task from the stream.
func (q *ValkeyQueue) Complete(ctx context.Context, taskID string) error {
	q.mu.Lock()
	ct, ok := q.claimed[taskID]
	if ok {
		delete(q.claimed, taskID)
	}
	q.mu.Unlock()

	if !ok {
		return fmt.Errorf("task %q not found in claimed set", taskID)
	}

	q.ackAndDel(ctx, ct.stream, ct.msgID)
	return nil
}

// Fail acknowledges the task and removes it from the stream.
func (q *ValkeyQueue) Fail(ctx context.Context, taskID string, _ string) error {
	return q.Complete(ctx, taskID)
}

func (q *ValkeyQueue) ensureGroup(ctx context.Context, stream string) {
	cmd := q.client.B().XgroupCreate().Key(stream).Group(valkeyGroupName).Id("0").Mkstream().Build()
	q.client.Do(ctx, cmd)
}

func (q *ValkeyQueue) ackAndDel(ctx context.Context, stream, msgID string) {
	ack := q.client.B().Xack().Key(stream).Group(valkeyGroupName).Id(msgID).Build()
	q.client.Do(ctx, ack)
	del := q.client.B().Xdel().Key(stream).Id(msgID).Build()
	q.client.Do(ctx, del)
}
