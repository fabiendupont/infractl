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

package api

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

// ServerConfig holds the configuration for the API server.
type ServerConfig struct {
	// Addr is the TCP address to listen on (e.g. ":8080").
	Addr string

	// TLSCert is the path to the TLS certificate file. If empty, the server
	// runs without TLS.
	TLSCert string

	// TLSKey is the path to the TLS private key file.
	TLSKey string
}

// Server is the main HTTP server. It wraps a chi router with standard
// middleware and provides lifecycle management.
type Server struct {
	Router chi.Router
	Config ServerConfig
	Logger zerolog.Logger
}

// NewServer creates a Server with standard middleware (request logging,
// panic recovery, and request ID injection) already installed.
func NewServer(cfg ServerConfig, logger zerolog.Logger) *Server {
	r := chi.NewRouter()

	// Install standard middleware stack.
	r.Use(RequestID)
	r.Use(RequestLogger(logger))
	r.Use(Recoverer(logger))

	// Health check endpoint -- always available, no auth.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	return &Server{
		Router: r,
		Config: cfg,
		Logger: logger,
	}
}

// ListenAndServe starts the HTTP(S) server. It blocks until the server
// shuts down or encounters a fatal error.
func (s *Server) ListenAndServe() error {
	addr := s.Config.Addr
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: s.Router,
	}

	if s.Config.TLSCert != "" && s.Config.TLSKey != "" {
		s.Logger.Info().
			Str("addr", addr).
			Msg("starting HTTPS server")

		srv.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		return srv.ListenAndServeTLS(s.Config.TLSCert, s.Config.TLSKey)
	}

	s.Logger.Info().
		Str("addr", addr).
		Msg("starting HTTP server")
	return srv.ListenAndServe()
}

// HTTPServer returns the underlying http.Server for graceful shutdown. The
// caller can use this with signal handling:
//
//	srv := apiServer.HTTPServer()
//	go apiServer.ListenAndServe()
//	<-ctx.Done()
//	srv.Shutdown(context.Background())
func (s *Server) HTTPServer() *http.Server {
	addr := s.Config.Addr
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: s.Router,
	}

	if s.Config.TLSCert != "" && s.Config.TLSKey != "" {
		srv.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	return srv
}

// BaseURL returns the base URL for the server based on its configuration.
func (s *Server) BaseURL() string {
	scheme := "http"
	if s.Config.TLSCert != "" {
		scheme = "https"
	}
	addr := s.Config.Addr
	if addr == "" {
		addr = ":8080"
	}
	return fmt.Sprintf("%s://%s", scheme, addr)
}
