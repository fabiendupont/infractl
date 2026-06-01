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
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	providerv1 "github.com/fabiendupont/infractl/provider/proto/infractl/provider/v1"
)

// ExternalProvider wraps a gRPC connection to a provider sidecar process.
// It implements Provider and APIProvider, proxying HTTP requests over gRPC
// using the ExternalProvider service defined in provider.proto.
type ExternalProvider struct {
	info       *providerv1.ProviderInfo
	client     providerv1.ExternalProviderClient
	conn       *grpc.ClientConn
	socketPath string
	routes     []*providerv1.Route
	hooks      []SyncHook
	reactions  []Reaction
}

// Name returns the provider's name as reported by the sidecar.
func (p *ExternalProvider) Name() string {
	return p.info.GetName()
}

// Version returns the provider's version as reported by the sidecar.
func (p *ExternalProvider) Version() string {
	return p.info.GetVersion()
}

// Features returns the features this provider offers.
func (p *ExternalProvider) Features() []string {
	return p.info.GetFeatures()
}

// Dependencies returns the providers this one depends on.
func (p *ExternalProvider) Dependencies() []string {
	return p.info.GetDependencies()
}

// Init initializes the external provider by calling the sidecar's Init
// RPC, fetching declared routes via GetRoutes, and registering hooks
// via GetHookRegistrations.
func (p *ExternalProvider) Init(ctx Context) error {
	// Call the sidecar's Init RPC with the API prefix and config.
	config := make(map[string]string)
	_, err := p.client.Init(context.Background(), &providerv1.InitRequest{
		ApiPrefix: ctx.APIPrefix,
		Config:    config,
	})
	if err != nil {
		return fmt.Errorf("external provider %q Init RPC failed: %w", p.Name(), err)
	}

	// Fetch routes from the sidecar.
	routesResp, err := p.client.GetRoutes(context.Background(), &providerv1.GetRoutesRequest{})
	if err != nil {
		return fmt.Errorf("external provider %q GetRoutes RPC failed: %w", p.Name(), err)
	}
	p.routes = routesResp.GetRoutes()

	// Fetch and register hook registrations from the sidecar.
	hookResp, err := p.client.GetHookRegistrations(context.Background(), &providerv1.GetHookRegistrationsRequest{})
	if err != nil {
		return fmt.Errorf("external provider %q GetHookRegistrations RPC failed: %w", p.Name(), err)
	}

	for _, reg := range hookResp.GetRegistrations() {
		if reg.GetSync() {
			hook := SyncHook{
				Feature: reg.GetFeature(),
				Event:   reg.GetEvent(),
				Handler: p.makeSyncHookHandler(reg.GetFeature(), reg.GetEvent()),
			}
			p.hooks = append(p.hooks, hook)
			if ctx.Registry != nil {
				ctx.Registry.RegisterHook(hook)
			}
		} else {
			reaction := Reaction{
				Feature:  reg.GetFeature(),
				Event:    reg.GetEvent(),
				Callback: p.makeAsyncHookHandler(reg.GetFeature(), reg.GetEvent()),
			}
			p.reactions = append(p.reactions, reaction)
			if ctx.Registry != nil {
				ctx.Registry.RegisterReaction(reaction)
			}
		}
	}

	log.Info().
		Str("provider", p.Name()).
		Str("version", p.Version()).
		Str("socket", p.socketPath).
		Int("routes", len(p.routes)).
		Int("hooks", len(p.hooks)).
		Int("reactions", len(p.reactions)).
		Msg("external provider initialized")
	return nil
}

// Shutdown calls the sidecar's Shutdown RPC and closes the gRPC connection.
func (p *ExternalProvider) Shutdown(ctx context.Context) error {
	if p.client != nil {
		if _, err := p.client.Shutdown(ctx, &providerv1.ShutdownRequest{}); err != nil {
			log.Warn().Err(err).Str("provider", p.Name()).Msg("sidecar shutdown RPC failed")
		}
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// RegisterRoutes registers proxy handlers for each route declared by
// the sidecar. Each route proxies the HTTP request to the sidecar
// via the HandleHTTP gRPC call and writes the response back.
func (p *ExternalProvider) RegisterRoutes(r chi.Router) {
	for _, route := range p.routes {
		method := route.GetMethod()
		path := route.GetPath()
		handler := p.proxyHandler

		switch method {
		case http.MethodGet:
			r.Get(path, handler)
		case http.MethodPost:
			r.Post(path, handler)
		case http.MethodPut:
			r.Put(path, handler)
		case http.MethodPatch:
			r.Patch(path, handler)
		case http.MethodDelete:
			r.Delete(path, handler)
		default:
			r.HandleFunc(path, handler)
		}

		log.Debug().
			Str("provider", p.Name()).
			Str("method", method).
			Str("path", path).
			Msg("registered external route")
	}
}

// proxyHandler forwards an HTTP request to the sidecar over gRPC
// using the HandleHTTP RPC and writes the response back to the caller.
func (p *ExternalProvider) proxyHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Convert HTTP headers to a flat map. For headers with multiple
	// values, only the first value is forwarded.
	headers := make(map[string]string, len(r.Header))
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	grpcReq := &providerv1.HTTPRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: headers,
		Body:    body,
		Query:   r.URL.RawQuery,
	}

	grpcResp, err := p.client.HandleHTTP(r.Context(), grpcReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("external provider RPC failed: %v", err), http.StatusBadGateway)
		return
	}

	// Write response headers from the sidecar.
	for k, v := range grpcResp.GetHeaders() {
		w.Header().Set(k, v)
	}
	w.WriteHeader(int(grpcResp.GetStatusCode()))
	w.Write(grpcResp.GetBody())
}

// makeSyncHookHandler creates a closure that forwards sync hook
// invocations to the sidecar via the HandleSyncHook gRPC call.
func (p *ExternalProvider) makeSyncHookHandler(feature, event string) func(ctx context.Context, payload interface{}) error {
	return func(ctx context.Context, payload interface{}) error {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal hook payload: %w", err)
		}

		result, err := p.client.HandleSyncHook(ctx, &providerv1.HookEvent{
			Feature: feature,
			Event:   event,
			Payload: payloadBytes,
		})
		if err != nil {
			return fmt.Errorf("sync hook RPC failed for %s:%s: %w", feature, event, err)
		}
		if !result.GetSuccess() {
			return fmt.Errorf("sync hook %s:%s rejected: %s", feature, event, result.GetError())
		}
		return nil
	}
}

// makeAsyncHookHandler creates a closure that forwards async hook
// (reaction) invocations to the sidecar via the HandleSyncHook gRPC
// call. Errors are logged but do not propagate to the caller.
func (p *ExternalProvider) makeAsyncHookHandler(feature, event string) func(ctx context.Context, payload interface{}) {
	return func(ctx context.Context, payload interface{}) {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Error().Err(err).
				Str("provider", p.Name()).
				Str("feature", feature).
				Str("event", event).
				Msg("failed to marshal async hook payload")
			return
		}

		_, err = p.client.HandleSyncHook(ctx, &providerv1.HookEvent{
			Feature: feature,
			Event:   event,
			Payload: payloadBytes,
		})
		if err != nil {
			log.Error().Err(err).
				Str("provider", p.Name()).
				Str("feature", feature).
				Str("event", event).
				Msg("async hook RPC failed")
		}
	}
}

// ConnectExternalProvider dials a Unix domain socket, creates the gRPC
// client, and calls GetInfo to populate the provider metadata from the
// sidecar. Returns a connected ExternalProvider ready for Init.
func ConnectExternalProvider(socketPath string) (*ExternalProvider, error) {
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return net.DialTimeout("unix", socketPath, 0)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to external provider at %s: %w", socketPath, err)
	}

	client := providerv1.NewExternalProviderClient(conn)

	// Call GetInfo to populate provider metadata from the sidecar.
	info, err := client.GetInfo(context.Background(), &providerv1.GetInfoRequest{})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to get info from external provider at %s: %w", socketPath, err)
	}

	return &ExternalProvider{
		info:       info,
		client:     client,
		conn:       conn,
		socketPath: socketPath,
	}, nil
}
