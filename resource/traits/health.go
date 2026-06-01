// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package traits

type HealthStatus string

const (
	HealthHealthy   HealthStatus = "Healthy"
	HealthDegraded  HealthStatus = "Degraded"
	HealthUnhealthy HealthStatus = "Unhealthy"
	HealthUnknown   HealthStatus = "Unknown"
)

type Healthable interface {
	HealthStatus() HealthStatus
	HealthMessage() string
}
