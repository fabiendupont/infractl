// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

// Package proto provides an adapter layer for mapping between infractl's
// Go resource structs and protobuf messages. Providers that serve gRPC
// endpoints use this to convert between the store layer (Go structs) and
// the transport layer (proto messages).
package proto

import (
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fabiendupont/infractl/resource"
	resourcev1 "github.com/fabiendupont/infractl/resource/proto/infractl/resource/v1"
)

// MetadataToProto converts a resource.Resource's metadata fields to a
// protobuf Metadata message.
func MetadataToProto(r *resource.Resource) *resourcev1.Metadata {
	m := &resourcev1.Metadata{
		OrgId:           r.OrgID.String(),
		Name:            r.Name,
		Generation:      r.Generation,
		ResourceVersion: r.ResourceVersion,
		Creator:         r.Creator,
		CreatedAt:       timestamppb.New(r.CreatedAt),
		UpdatedAt:       timestamppb.New(r.UpdatedAt),
	}

	if r.Parent != nil {
		m.Parent = r.Parent
	}
	if r.Owner != nil {
		m.Owner = *r.Owner
	}

	if r.Labels != nil {
		m.Labels = map[string]string(r.Labels)
	}
	if r.Annotations != nil {
		m.Annotations = map[string]string(r.Annotations)
	}
	if r.Finalizers != nil {
		m.Finalizers = []string(r.Finalizers)
	}

	if r.DeletionTimestamp != nil {
		m.DeletionTimestamp = timestamppb.New(*r.DeletionTimestamp)
	}

	return m
}

// MetadataFromProto converts a protobuf Metadata message back to
// resource.Resource fields. The caller is responsible for embedding the
// returned Resource into their domain type.
func MetadataFromProto(m *resourcev1.Metadata) resource.Resource {
	r := resource.Resource{
		Name:            m.GetName(),
		Generation:      m.GetGeneration(),
		ResourceVersion: m.GetResourceVersion(),
		Creator:         m.GetCreator(),
	}

	if id, err := uuid.Parse(m.GetOrgId()); err == nil {
		r.OrgID = id
	}

	if m.Parent != nil {
		r.Parent = m.Parent
	}
	if m.GetOwner() != "" {
		owner := m.GetOwner()
		r.Owner = &owner
	}

	if m.GetLabels() != nil {
		r.Labels = resource.JSONMap(m.GetLabels())
	}
	if m.GetAnnotations() != nil {
		r.Annotations = resource.JSONMap(m.GetAnnotations())
	}
	if m.GetFinalizers() != nil {
		r.Finalizers = resource.JSONArray(m.GetFinalizers())
	}

	if m.GetCreatedAt() != nil {
		r.CreatedAt = m.GetCreatedAt().AsTime()
	}
	if m.GetUpdatedAt() != nil {
		r.UpdatedAt = m.GetUpdatedAt().AsTime()
	}
	if m.GetDeletionTimestamp() != nil {
		t := m.GetDeletionTimestamp().AsTime()
		r.DeletionTimestamp = &t
	}

	return r
}

// ListOptionsFromProto converts a protobuf ListRequest to resource.ListOptions.
func ListOptionsFromProto(req *resourcev1.ListRequest) resource.ListOptions {
	return resource.ListOptions{
		Limit:    int(req.GetLimit()),
		Continue: req.GetContinue(),
		Filter:   req.GetFilter(),
		Sort:     req.GetSort(),
	}
}

// OrgIDFromProto parses the org_id string from a proto message into a uuid.UUID.
func OrgIDFromProto(orgID string) (uuid.UUID, error) {
	return uuid.Parse(orgID)
}

// TimestampToProto converts a Go time.Time to a protobuf Timestamp.
// Returns nil for zero time.
func TimestampToProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
