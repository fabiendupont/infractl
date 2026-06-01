// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/fabiendupont/infractl/auth"
	"github.com/fabiendupont/infractl/resource"
)

// ResourceAdapter converts between a Go resource type R and its protobuf
// representation. Providers implement this for each resource type.
type ResourceAdapter[R any] interface {
	ToProto(r *R) interface{}
	FromProto(msg interface{}) (*R, error)
	ToProtoList(items []R, continueToken string, total int64) interface{}
}

// GenericServiceHandler provides CRUD operations for a resource type over
// gRPC. Providers embed this and delegate their gRPC method implementations
// to its methods.
type GenericServiceHandler[R any] struct {
	Store       resource.Store[R]
	Adapter     ResourceAdapter[R]
	Tenancy     auth.TenancyLogic
	Attribution auth.AttributionLogic
}

// Create handles a gRPC Create request.
func (h *GenericServiceHandler[R]) Create(ctx context.Context, msg interface{}) (interface{}, error) {
	r, err := h.Adapter.FromProto(msg)
	if err != nil {
		return nil, err
	}

	orgID, err := h.resolveOrgID(ctx)
	if err != nil {
		return nil, err
	}

	if accessor, ok := any(r).(resource.ResourceAccessor); ok {
		if accessor.GetOrgID() == uuid.Nil {
			accessor.(*resource.Resource).OrgID = orgID
		}
	}

	if h.Attribution != nil {
		creator, err := h.Attribution.DetermineAssignedCreator(ctx)
		if err == nil {
			if res, ok := any(r).(interface{ SetCreator(string) }); ok {
				res.SetCreator(creator)
			}
		}
	}

	if err := h.Store.Create(ctx, r); err != nil {
		return nil, mapStoreError(err)
	}

	return h.Adapter.ToProto(r), nil
}

// Get handles a gRPC Get request.
func (h *GenericServiceHandler[R]) Get(ctx context.Context, name string) (interface{}, error) {
	orgID, err := h.resolveOrgID(ctx)
	if err != nil {
		return nil, err
	}

	r, err := h.Store.Get(ctx, orgID, name)
	if err != nil {
		return nil, mapStoreError(err)
	}

	return h.Adapter.ToProto(r), nil
}

// List handles a gRPC List request.
func (h *GenericServiceHandler[R]) List(ctx context.Context, opts resource.ListOptions) (interface{}, error) {
	orgID, err := h.resolveOrgID(ctx)
	if err != nil {
		return nil, err
	}

	list, err := h.Store.List(ctx, orgID, opts)
	if err != nil {
		return nil, mapStoreError(err)
	}

	return h.Adapter.ToProtoList(list.Items, list.Continue, list.Total), nil
}

// Update handles a gRPC Update request.
func (h *GenericServiceHandler[R]) Update(ctx context.Context, msg interface{}) (interface{}, error) {
	r, err := h.Adapter.FromProto(msg)
	if err != nil {
		return nil, err
	}

	if err := h.Store.Update(ctx, r); err != nil {
		return nil, mapStoreError(err)
	}

	return h.Adapter.ToProto(r), nil
}

// PartialUpdate handles a gRPC partial update request with field masking.
func (h *GenericServiceHandler[R]) PartialUpdate(ctx context.Context, orgID uuid.UUID, name string, resourceVersion int64, fields map[string]interface{}) (interface{}, error) {
	if err := h.Store.PartialUpdate(ctx, orgID, name, resourceVersion, fields); err != nil {
		return nil, mapStoreError(err)
	}

	r, err := h.Store.Get(ctx, orgID, name)
	if err != nil {
		return nil, mapStoreError(err)
	}

	return h.Adapter.ToProto(r), nil
}

// Delete handles a gRPC Delete request.
func (h *GenericServiceHandler[R]) Delete(ctx context.Context, name string) error {
	orgID, err := h.resolveOrgID(ctx)
	if err != nil {
		return err
	}

	err = h.Store.Delete(ctx, orgID, name)
	if err != nil && !errors.Is(err, resource.ErrFinalizersPending) {
		return mapStoreError(err)
	}
	return nil
}

func (h *GenericServiceHandler[R]) resolveOrgID(ctx context.Context) (uuid.UUID, error) {
	orgID, err := auth.OrgIDFromContext(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return orgID, nil
}

func mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, resource.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, resource.ErrAlreadyExists):
		return ErrAlreadyExists
	case errors.Is(err, resource.ErrConflict):
		return ErrConflict
	default:
		return err
	}
}
