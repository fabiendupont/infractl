// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

// Phase defines when a handler runs relative to the main action.
type Phase string

const (
	PhasePre  Phase = "pre"
	PhaseMain Phase = "main"
	PhasePost Phase = "post"
)

// Handler maps a resource lifecycle event to an executor-specific action.
// The Ref field is opaque to infractl — the executor interprets it as a
// Temporal workflow ID, AAP role name, Go function name, etc.
type Handler struct {
	ResourceType string
	Event        string
	Phase        Phase
	Priority     int
	Ref          string
	Metadata     map[string]string
}

// phaseOrder returns an integer for sorting phases: pre < main < post.
func phaseOrder(p Phase) int {
	switch p {
	case PhasePre:
		return 0
	case PhaseMain:
		return 1
	case PhasePost:
		return 2
	default:
		return 3
	}
}
