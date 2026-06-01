// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// SetupPostgres starts a PostgreSQL testcontainer and returns a GORM DB
// handle, the raw DSN string (for pgx connections), and a cleanup function.
func SetupPostgres(t *testing.T) (*gorm.DB, string, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("infractl_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to get connection string: %v", err)
	}

	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{})
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to connect to postgres: %v", err)
	}

	cleanup := func() {
		container.Terminate(ctx)
	}

	return db, dsn, cleanup
}

