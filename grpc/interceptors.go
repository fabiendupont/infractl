// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fabiendupont/infractl/auth"
)

// AuthInterceptor returns a gRPC unary interceptor that enforces
// authentication, tenancy, and authorization using the same interfaces
// as the chi HTTP middleware.
func AuthInterceptor(
	authn auth.ContextAuthenticator,
	tenancy auth.TenancyLogic,
	authz auth.Authorizer,
	logger zerolog.Logger,
) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		subject, err := authn.AuthenticateContext(ctx)
		if err != nil {
			logger.Warn().Err(err).Str("method", info.FullMethod).Msg("authentication failed")
			return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		ctx = auth.ContextWithSubject(ctx, subject)

		defaults, err := tenancy.DetermineDefaultTenants(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "default tenant resolution failed: %v", err)
		}

		defaultTenants := defaults.Values()
		if len(defaultTenants) > 0 {
			parsed, err := uuid.Parse(defaultTenants[0])
			if err != nil {
				return nil, status.Errorf(codes.Internal, "invalid tenant UUID: %v", err)
			}
			ctx = auth.ContextWithOrgID(ctx, parsed)
		}

		if err := authz.Authorize(ctx, subject, info.FullMethod, "invoke"); err != nil {
			return nil, status.Errorf(codes.PermissionDenied, "access denied: %v", err)
		}

		return handler(ctx, req)
	}
}

// StreamAuthInterceptor returns a gRPC stream interceptor for auth.
func StreamAuthInterceptor(
	authn auth.ContextAuthenticator,
	tenancy auth.TenancyLogic,
	authz auth.Authorizer,
	logger zerolog.Logger,
) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		subject, err := authn.AuthenticateContext(ctx)
		if err != nil {
			logger.Warn().Err(err).Str("method", info.FullMethod).Msg("authentication failed")
			return status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		ctx = auth.ContextWithSubject(ctx, subject)

		defaults, err := tenancy.DetermineDefaultTenants(ctx)
		if err != nil {
			return status.Errorf(codes.Internal, "default tenant resolution failed: %v", err)
		}

		defaultTenants := defaults.Values()
		if len(defaultTenants) > 0 {
			parsed, err := uuid.Parse(defaultTenants[0])
			if err != nil {
				return status.Errorf(codes.Internal, "invalid tenant UUID: %v", err)
			}
			ctx = auth.ContextWithOrgID(ctx, parsed)
		}

		if err := authz.Authorize(ctx, subject, info.FullMethod, "invoke"); err != nil {
			return status.Errorf(codes.PermissionDenied, "access denied: %v", err)
		}

		wrapped := &wrappedStream{ServerStream: ss, ctx: ctx}
		return handler(srv, wrapped)
	}
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}
