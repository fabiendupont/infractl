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
	"fmt"
	"strings"
)

// jsonbPrefixes lists the field prefixes that map to JSONB columns and use
// the ->> operator for key access.
var jsonbPrefixes = []string{"labels.", "annotations."}

// operators lists the supported comparison operators in order from longest
// to shortest so that ">=" is matched before ">".
var operators = []string{">=", "<=", "!=", ">", "<", "="}

// ParseFilter parses a filter expression into a GORM WHERE clause.
//
// Supported syntax:
//
//	"field=value"                 single equality
//	"field!=value"                inequality
//	"field>value"                 greater than (also >=, <, <=)
//	"field1=v1,field2=v2"         multiple terms with AND (comma)
//	"field1=v1 AND field2=v2"     multiple terms with AND (keyword)
//	"field1=v1 OR field2=v2"      OR groups
//	"labels.key=value"            JSONB label access (labels->>'key' = ?)
//	"annotations.key=value"       JSONB annotation access
//
// Field names are validated against a conservative allowlist of characters
// to prevent SQL injection. Values are always parameterized.
func ParseFilter(expr string) (clause string, args []interface{}, err error) {
	if expr == "" {
		return "", nil, nil
	}

	// Split by " OR " first to get OR groups.
	orGroups := splitOR(expr)
	orClauses := make([]string, 0, len(orGroups))

	for _, group := range orGroups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}

		// Split each OR group by "," or " AND " to get AND terms.
		andTerms := splitAND(group)
		andClauses := make([]string, 0, len(andTerms))

		for _, term := range andTerms {
			term = strings.TrimSpace(term)
			if term == "" {
				continue
			}

			tc, tv, terr := parseTerm(term)
			if terr != nil {
				return "", nil, terr
			}
			andClauses = append(andClauses, tc)
			args = append(args, tv)
		}

		if len(andClauses) == 0 {
			continue
		}

		if len(andClauses) == 1 {
			orClauses = append(orClauses, andClauses[0])
		} else {
			orClauses = append(orClauses, strings.Join(andClauses, " AND "))
		}
	}

	if len(orClauses) == 0 {
		return "", nil, nil
	}

	if len(orClauses) == 1 {
		return orClauses[0], args, nil
	}

	// Wrap each OR group in parentheses.
	for i, c := range orClauses {
		orClauses[i] = "(" + c + ")"
	}
	return strings.Join(orClauses, " OR "), args, nil
}

// splitOR splits an expression by the " OR " keyword. We split on " OR "
// (with surrounding spaces) to avoid matching "OR" inside field names or values.
func splitOR(expr string) []string {
	return splitKeyword(expr, " OR ")
}

// splitAND splits an expression by "," or " AND ". Commas are checked first
// for backwards compatibility; if no commas are present, " AND " is used.
func splitAND(expr string) []string {
	// If the expression contains commas, split on commas.
	// If it contains " AND ", split on that.
	// If it contains both, we need to handle them together.
	// Strategy: replace " AND " with "," then split on ",".
	normalized := replaceKeyword(expr, " AND ", ",")
	return strings.Split(normalized, ",")
}

// replaceKeyword replaces all occurrences of a case-sensitive keyword in the
// string. This is a simple replacement — no attempt to handle quoting.
func replaceKeyword(s, keyword, replacement string) string {
	return strings.ReplaceAll(s, keyword, replacement)
}

// splitKeyword splits a string by a case-sensitive keyword.
func splitKeyword(s, keyword string) []string {
	return strings.Split(s, keyword)
}

// parseTerm parses a single "field op value" term and returns the SQL clause
// fragment and the parameterized value.
func parseTerm(term string) (clause string, value interface{}, err error) {
	field, op, val, err := extractFieldOpValue(term)
	if err != nil {
		return "", nil, err
	}

	if field == "" {
		return "", nil, fmt.Errorf("filter term %q: empty field name", term)
	}

	if !isValidFieldName(field) {
		return "", nil, fmt.Errorf("filter term %q: invalid field name %q", term, field)
	}

	sqlField := fieldToSQL(field)
	return fmt.Sprintf("%s %s ?", sqlField, op), val, nil
}

// extractFieldOpValue splits a term like "field>=value" into its three parts.
// It finds the earliest operator position, preferring longer operators when
// multiple operators start at the same index (e.g., ">=" over ">").
func extractFieldOpValue(term string) (field, op, value string, err error) {
	bestIdx := -1
	bestOp := ""

	for _, candidate := range operators {
		idx := strings.Index(term, candidate)
		if idx < 0 {
			continue
		}
		// Pick the operator with the smallest index. If tied, pick
		// the longer one (operators are sorted longest-first, so the
		// first match at a given index wins).
		if bestIdx < 0 || idx < bestIdx || (idx == bestIdx && len(candidate) > len(bestOp)) {
			bestIdx = idx
			bestOp = candidate
		}
	}

	if bestIdx < 0 {
		return "", "", "", fmt.Errorf("filter term %q: missing operator (expected one of =, !=, >, <, >=, <=)", term)
	}

	field = strings.TrimSpace(term[:bestIdx])
	value = strings.TrimSpace(term[bestIdx+len(bestOp):])
	op = bestOp
	return field, op, value, nil
}

// fieldToSQL converts a field name to a SQL column reference. For JSONB
// fields (labels.*, annotations.*), it generates the ->> accessor syntax.
func fieldToSQL(field string) string {
	for _, prefix := range jsonbPrefixes {
		if strings.HasPrefix(field, prefix) {
			column := strings.TrimSuffix(prefix, ".")
			key := field[len(prefix):]
			return fmt.Sprintf("%s->>'%s'", column, key)
		}
	}
	return field
}

// isValidFieldName checks that a field name contains only safe characters:
// letters, digits, underscores, and dots (for nested JSONB access).
func isValidFieldName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '.') {
			return false
		}
	}
	return true
}
