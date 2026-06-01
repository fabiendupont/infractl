// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrNotFound      = status.Error(codes.NotFound, "resource not found")
	ErrAlreadyExists = status.Error(codes.AlreadyExists, "resource already exists")
	ErrConflict      = status.Error(codes.Aborted, "resource version conflict")
)
