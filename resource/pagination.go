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

package resource

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// ContinueToken holds the state needed to resume a paginated List query.
type ContinueToken struct {
	// Offset is the row offset into the full result set.
	Offset int `json:"o"`

	// ResourceVersion pins the query to a specific point-in-time snapshot.
	// This prevents phantom reads when rows are inserted between pages.
	ResourceVersion int64 `json:"rv,omitempty"`
}

// EncodeContinueToken serializes a ContinueToken to a URL-safe opaque string.
func EncodeContinueToken(token ContinueToken) string {
	b, _ := json.Marshal(token)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeContinueToken deserializes an opaque continue string back into a
// ContinueToken. Returns an error if the token is malformed.
func DecodeContinueToken(s string) (ContinueToken, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return ContinueToken{}, fmt.Errorf("invalid continue token encoding: %w", err)
	}
	var token ContinueToken
	if err := json.Unmarshal(data, &token); err != nil {
		return ContinueToken{}, fmt.Errorf("invalid continue token payload: %w", err)
	}
	return token, nil
}
