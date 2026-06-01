// Copyright 2025 The infractl Authors
// SPDX-License-Identifier: Apache-2.0

package mixins

type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type LifecycleStatus struct {
	Phase      string      `json:"phase"`
	Conditions []Condition `json:"conditions,omitempty"`
}
