// Copyright 2025 Fabien Dupont
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/fabiendupont/infractl/events"
	"github.com/fabiendupont/infractl/work"
)

// Context is what the core provides to providers at initialization.
// Config is typed as interface{} because the concrete config type lives
// in an internal package; providers type-assert to the config they need.
type Context struct {
	// DB is the shared GORM database connection.
	DB *gorm.DB

	// Config is the application configuration. Providers type-assert
	// to the concrete config struct they expect.
	Config interface{}

	// Registry is the provider registry. Providers use it to look up
	// other providers or register hooks during Init.
	Registry *Registry

	// Hooks provides hook firing capabilities. Created by the main
	// binary from the Registry. Nil when running without hook support.
	Hooks *HookRunner

	// Logger is the structured logger for this provider context.
	Logger zerolog.Logger

	// APIPrefix is the route prefix for API endpoints (e.g., "/api/v1").
	// Set by the core before calling Init on API providers.
	APIPrefix string

	// Bus is the event bus for publishing and subscribing to resource
	// lifecycle events. Nil when running without event support.
	Bus events.Bus

	// Queue is the task queue for async background work. Nil when
	// running without queue support.
	Queue work.Queue
}
