// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Action constants for Operation.Action.
const (
	ActionCreate = "create"
	ActionUpdate = "update"
)

// Operation is a single create/update action the LLM wants to perform.
type Operation struct {
	Action string         `json:"action"` // ActionCreate or ActionUpdate
	Table  string         `json:"table"`
	Data   map[string]any `json:"data"`
}

// ParseOperations unmarshals the schema-constrained {"operations": [...]}
// response from the LLM.
func ParseOperations(raw string) ([]Operation, error) {
	cleaned := strings.TrimSpace(raw)

	if cleaned == "" {
		return nil, fmt.Errorf("empty LLM output")
	}

	// UseNumber preserves JSON numbers as json.Number strings instead of
	// float64, avoiding precision loss on large integers (IDs, cents).
	var wrapper struct {
		Operations []Operation `json:"operations"`
	}
	dec := json.NewDecoder(strings.NewReader(cleaned))
	dec.UseNumber()
	if err := dec.Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("parse operations json: %w", err)
	}

	if len(wrapper.Operations) == 0 {
		return nil, fmt.Errorf("no operations found in LLM output")
	}

	return wrapper.Operations, nil
}

// OperationsSchema returns the JSON Schema for structured extraction output.
// The schema constrains model output to {"operations": [...]}, where each
// operation has action, table, and data fields.
func OperationsSchema() map[string]any {
	tables := make([]any, 0, len(ExtractionAllowedOps))
	for t := range ExtractionAllowedOps {
		tables = append(tables, t)
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operations": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []any{"action", "table", "data"},
					"properties": map[string]any{
						"action": map[string]any{
							"type": "string",
							"enum": []any{ActionCreate, ActionUpdate},
						},
						"table": map[string]any{
							"type": "string",
							"enum": tables,
						},
						"data": map[string]any{
							"type": "object",
						},
					},
					"additionalProperties": false,
				},
			},
		},
		"required":             []any{"operations"},
		"additionalProperties": false,
	}
}

// ValidateOperations checks each operation against the allowed tables and
// action types. Returns an error describing the first violation found.
func ValidateOperations(ops []Operation, allowed map[string]AllowedOps) error {
	for i, op := range ops {
		action := strings.ToLower(strings.TrimSpace(op.Action))
		table := strings.ToLower(strings.TrimSpace(op.Table))

		if action != ActionCreate && action != ActionUpdate {
			return fmt.Errorf(
				"operation %d: action must be %q or %q, got %q",
				i, ActionCreate, ActionUpdate, op.Action,
			)
		}

		perms, ok := allowed[table]
		if !ok {
			return fmt.Errorf(
				"operation %d: table %q is not in the allowed set",
				i, op.Table,
			)
		}

		if action == ActionCreate && !perms.Insert {
			return fmt.Errorf(
				"operation %d: create not allowed on table %q",
				i, op.Table,
			)
		}
		if action == ActionUpdate && !perms.Update {
			return fmt.Errorf(
				"operation %d: update not allowed on table %q",
				i, op.Table,
			)
		}

		if len(op.Data) == 0 {
			return fmt.Errorf(
				"operation %d: data must not be empty",
				i,
			)
		}
	}
	return nil
}

// ParseUint extracts a uint from a JSON value (json.Number, float64, or
// string). Returns 0 for nil, negative, or unparsable values.
func ParseUint(v any) uint {
	switch val := v.(type) {
	case json.Number:
		if n, err := strconv.ParseUint(val.String(), 10, strconv.IntSize); err == nil {
			return uint(n)
		}
	case float64:
		if val > 0 && val <= math.MaxUint {
			return uint(val)
		}
	case string:
		if n, err := strconv.ParseUint(strings.TrimSpace(val), 10, strconv.IntSize); err == nil {
			return uint(n)
		}
	}
	return 0
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}
