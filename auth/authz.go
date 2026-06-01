// Copyright 2025 The infractl Authors.
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

package auth

import (
	"context"
)

// Authorizer decides whether a subject is permitted to perform an action on
// a resource kind. Implementations may consult OPA policies, RBAC rules, or
// other policy engines.
type Authorizer interface {
	// Authorize returns nil if the subject is allowed to perform the action
	// on the given resource kind, or a non-nil error describing the denial.
	Authorize(ctx context.Context, subject *Subject, resource, action string) error
}

// AllowAllAuthorizer permits every request. Use this only for development
// and testing -- never in production.
type AllowAllAuthorizer struct{}

// Authorize always returns nil (permit).
func (a *AllowAllAuthorizer) Authorize(_ context.Context, _ *Subject, _, _ string) error {
	return nil
}
