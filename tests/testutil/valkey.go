// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/valkey-io/valkey-go"
)

// SetupValkey starts a Valkey testcontainer and returns a connected client
// and a cleanup function.
func SetupValkey(t *testing.T) (valkey.Client, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "valkey/valkey:8-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("failed to start valkey container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to get valkey host: %v", err)
	}

	port, err := container.MappedPort(ctx, "6379")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to get valkey port: %v", err)
	}

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{host + ":" + port.Port()},
	})
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to create valkey client: %v", err)
	}

	cleanup := func() {
		client.Close()
		container.Terminate(ctx)
	}

	return client, cleanup
}
