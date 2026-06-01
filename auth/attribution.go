// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import "context"

// AttributionLogic determines the creator to assign to newly created resources.
type AttributionLogic interface {
	DetermineAssignedCreator(ctx context.Context) (string, error)
}

// SubjectAttributionLogic assigns the authenticated user as the creator.
type SubjectAttributionLogic struct{}

func (l *SubjectAttributionLogic) DetermineAssignedCreator(ctx context.Context) (string, error) {
	subject, err := SubjectFromContext(ctx)
	if err != nil {
		return "", err
	}
	return subject.User, nil
}

// GuestAttributionLogic always assigns a fixed creator string.
type GuestAttributionLogic struct {
	Creator string
}

func (l *GuestAttributionLogic) DetermineAssignedCreator(_ context.Context) (string, error) {
	return l.Creator, nil
}
