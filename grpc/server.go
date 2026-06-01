// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"crypto/tls"
	"fmt"
	"net"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// ServerConfig holds the configuration for the gRPC server.
type ServerConfig struct {
	Addr    string
	TLSCert string
	TLSKey  string
}

// Server wraps a gRPC server with lifecycle management.
type Server struct {
	GRPCServer *grpc.Server
	Config     ServerConfig
	Logger     zerolog.Logger
	Health     *health.Server
}

// NewServer creates a gRPC server with health check and reflection
// services pre-registered. Auth interceptors are added via options.
func NewServer(cfg ServerConfig, logger zerolog.Logger, opts ...grpc.ServerOption) *Server {
	var serverOpts []grpc.ServerOption

	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to load TLS credentials")
		}
		creds := credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})
		serverOpts = append(serverOpts, grpc.Creds(creds))
	}

	serverOpts = append(serverOpts, opts...)
	srv := grpc.NewServer(serverOpts...)

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthSrv)
	reflection.Register(srv)

	return &Server{
		GRPCServer: srv,
		Config:     cfg,
		Logger:     logger,
		Health:     healthSrv,
	}
}

// ListenAndServe starts the gRPC server on the configured address.
func (s *Server) ListenAndServe() error {
	addr := s.Config.Addr
	if addr == "" {
		addr = ":9090"
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.Logger.Info().Str("addr", addr).Msg("starting gRPC server")
	return s.GRPCServer.Serve(lis)
}

// GracefulStop gracefully stops the gRPC server.
func (s *Server) GracefulStop() {
	s.Health.Shutdown()
	s.GRPCServer.GracefulStop()
}
