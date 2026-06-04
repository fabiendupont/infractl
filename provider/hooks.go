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
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
)

// HookFirer is the interface that activities use to fire hooks. It is
// implemented by HookRunner. Consumers accept this interface rather
// than the concrete HookRunner to avoid tight coupling.
type HookFirer interface {
	FireSync(ctx context.Context, feature, event string, payload interface{}) error
	FireAsync(ctx context.Context, feature, event string, payload interface{})
}

// Verify HookRunner implements HookFirer at compile time.
var _ HookFirer = (*HookRunner)(nil)

// SyncHook runs inline in the caller's execution context. Pre-hooks can
// return an error to abort the operation. Post-hooks that fail cause the
// calling operation to fail.
type SyncHook struct {
	Feature string
	Event   string
	Handler func(ctx context.Context, payload interface{}) error
}

// Reaction fires an asynchronous callback after an event. Non-blocking --
// the source operation does not wait for the callback to complete. Errors
// are logged but do not fail the caller.
type Reaction struct {
	Feature  string
	Event    string
	Callback func(ctx context.Context, payload interface{})
}

// hookKey creates a lookup key from feature+event.
func hookKey(feature, event string) string {
	return feature + ":" + event
}

// hookRegistry stores sync hooks and async reactions.
type hookRegistry struct {
	mu        sync.RWMutex
	hooks     map[string][]SyncHook
	reactions map[string][]Reaction
}

func newHookRegistry() *hookRegistry {
	return &hookRegistry{
		hooks:     make(map[string][]SyncHook),
		reactions: make(map[string][]Reaction),
	}
}

// RegisterHook adds a sync hook for a feature+event combination.
func (r *Registry) RegisterHook(hook SyncHook) {
	r.hooks.mu.Lock()
	defer r.hooks.mu.Unlock()
	key := hookKey(hook.Feature, hook.Event)
	r.hooks.hooks[key] = append(r.hooks.hooks[key], hook)
}

// RegisterReaction adds an async reaction for a feature+event combination.
func (r *Registry) RegisterReaction(reaction Reaction) {
	r.hooks.mu.Lock()
	defer r.hooks.mu.Unlock()
	key := hookKey(reaction.Feature, reaction.Event)
	r.hooks.reactions[key] = append(r.hooks.reactions[key], reaction)
}

// RegisterReactionFunc is a convenience method that satisfies the
// workflow.ReactionRegistrar interface without requiring callers to
// construct a Reaction struct.
func (r *Registry) RegisterReactionFunc(feature, event string, callback func(ctx context.Context, payload interface{})) {
	r.RegisterReaction(Reaction{
		Feature:  feature,
		Event:    event,
		Callback: callback,
	})
}

// HookRunner provides hook firing capabilities to operations. It wraps
// the hook registry and exposes sync and async firing methods.
type HookRunner struct {
	hooks  *hookRegistry
	logger zerolog.Logger
}

// NewHookRunner creates a HookRunner from a registry and logger.
func NewHookRunner(registry *Registry, logger zerolog.Logger) *HookRunner {
	return &HookRunner{
		hooks:  registry.hooks,
		logger: logger,
	}
}

// FireSync runs all sync hooks registered for the given feature+event.
// Returns the first error if any hook fails. For pre-hooks, this aborts
// the operation. For post-hooks, this causes the caller to fail.
func (hr *HookRunner) FireSync(ctx context.Context, feature, event string, payload interface{}) error {
	if hr == nil {
		return nil
	}
	hr.hooks.mu.RLock()
	hooks := hr.hooks.hooks[hookKey(feature, event)]
	hr.hooks.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook.Handler(ctx, payload); err != nil {
			return fmt.Errorf("hook %s:%s failed: %w", feature, event, err)
		}
	}
	return nil
}

// FireAsync runs all registered reactions for the given feature+event.
// Non-blocking -- each reaction runs in its own goroutine. Errors are
// logged but do not fail the caller.
func (hr *HookRunner) FireAsync(ctx context.Context, feature, event string, payload interface{}) {
	if hr == nil {
		return
	}
	hr.hooks.mu.RLock()
	reactions := hr.hooks.reactions[hookKey(feature, event)]
	hr.hooks.mu.RUnlock()

	for _, reaction := range reactions {
		go func(r Reaction) {
			defer func() {
				if rec := recover(); rec != nil {
					hr.logger.Error().
						Str("feature", r.Feature).
						Str("event", r.Event).
						Interface("panic", rec).
						Msg("reaction panicked")
				}
			}()
			r.Callback(context.Background(), payload)
		}(reaction)
	}
}
