// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"encoding/json"
	"fmt"
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

// OperationPreviewRow holds the column-value pairs from an Operation for
// rendering as a mini table in the extraction overlay.
type OperationPreviewRow struct {
	Table   string
	Op      string // "create" or "update"
	RowID   uint   // nonzero for update (from data["id"] or separate field)
	Columns []string
	Values  []string
}

// ParseOperations strips code fences and unmarshals a JSON array of operations.
func ParseOperations(raw string) ([]Operation, error) {
	cleaned := StripCodeFences(raw)
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return nil, fmt.Errorf("empty LLM output")
	}

	var ops []Operation
	if err := json.Unmarshal([]byte(cleaned), &ops); err != nil {
		return nil, fmt.Errorf("parse operations json: %w", err)
	}

	if len(ops) == 0 {
		return nil, fmt.Errorf("no operations found in LLM output")
	}

	return ops, nil
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

// OperationPreview extracts column-value pairs from an Operation for display.
func OperationPreview(op Operation) *OperationPreviewRow {
	if len(op.Data) == 0 {
		return nil
	}

	row := &OperationPreviewRow{
		Table: op.Table,
		Op:    op.Action,
	}

	// Extract row ID from "id" key if present (for updates).
	if idVal, ok := op.Data["id"]; ok {
		row.RowID = parseUintFromAny(idVal)
	}

	// Sort keys for deterministic display. Exclude "id" from column list
	// since it's shown in the header.
	keys := sortedKeys(op.Data)
	for _, k := range keys {
		if k == "id" {
			continue
		}
		row.Columns = append(row.Columns, k)
		row.Values = append(row.Values, formatValue(op.Data[k]))
	}

	return row
}

// parseUintFromAny extracts a uint from a JSON value (float64 or string).
func parseUintFromAny(v any) uint {
	switch val := v.(type) {
	case float64:
		if val > 0 {
			return uint(val)
		}
	case string:
		if n, err := strconv.ParseUint(val, 10, 64); err == nil {
			return uint(n)
		}
	}
	return 0
}

// formatValue converts a JSON value to a display string.
func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort for small maps.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
