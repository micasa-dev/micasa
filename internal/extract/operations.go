// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Action is a typed string enum for extraction operations.
type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"

	documentsTable = "documents"
)

// JSON Schema vocabulary keys used when building OperationsSchema.
const (
	schemaKeyType                 = "type"
	schemaKeyProperties           = "properties"
	schemaKeyRequired             = "required"
	schemaKeyItems                = "items"
	schemaKeyAnyOf                = "anyOf"
	schemaKeyEnum                 = "enum"
	schemaKeyAdditionalProperties = "additionalProperties"
)

// Property names in the extraction-output schema (mirrors Operation's JSON tags
// and the top-level "operations"/"document" wrapper fields).
const (
	schemaKeyOperations = "operations"
	schemaKeyDocument   = "document"
	schemaKeyAction     = "action"
	schemaKeyTable      = "table"
	schemaKeyData       = "data"
)

// Operation is a single create/update action the LLM wants to perform.
type Operation struct {
	Action Action         `json:"action"`
	Table  string         `json:"table"`
	Data   map[string]any `json:"data"`
}

// ParseOperations unmarshals the schema-constrained
// {"operations": [...], "document": {...}} response from the LLM.
// The optional "document" field is synthesized into a regular Operation
// with Table "documents" so downstream consumers see a uniform slice.
func ParseOperations(raw string) ([]Operation, error) {
	cleaned := strings.TrimSpace(raw)

	if cleaned == "" {
		return nil, errors.New("empty LLM output")
	}

	// UseNumber preserves JSON numbers as json.Number strings instead of
	// float64, avoiding precision loss on large integers (IDs, cents).
	var wrapper struct {
		Operations []Operation `json:"operations"`
		Document   *Operation  `json:"document"`
	}
	dec := json.NewDecoder(strings.NewReader(cleaned))
	dec.UseNumber()
	if err := dec.Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("parse operations json: %w", err)
	}

	ops := wrapper.Operations
	if wrapper.Document != nil {
		wrapper.Document.Table = documentsTable
		ops = append(ops, *wrapper.Document)
	}

	if len(ops) == 0 {
		return nil, errors.New("no operations found in LLM output")
	}

	return ops, nil
}

// OperationsSchema returns the JSON Schema for structured extraction output.
// The schema uses anyOf to define precise per-table column schemas, so the
// LLM is constrained to produce only valid column names and types for each
// {action, table} combination. Document operations live in a separate
// top-level "document" field (singular object) rather than the array.
func OperationsSchema() map[string]any {
	schema := map[string]any{
		schemaKeyType: "object",
		schemaKeyProperties: map[string]any{
			schemaKeyOperations: map[string]any{
				schemaKeyType: "array",
				schemaKeyItems: map[string]any{
					schemaKeyAnyOf: operationVariants(),
				},
			},
			schemaKeyDocument: map[string]any{
				schemaKeyAnyOf: documentVariants(),
			},
		},
		schemaKeyRequired:             []any{schemaKeyOperations},
		schemaKeyAdditionalProperties: false,
	}
	return schema
}

// operationVariants returns the anyOf branches for non-document tables.
// Each branch constrains table to a single value and data to the exact
// columns that table's commit function consumes.
func operationVariants() []any {
	var variants []any
	for _, op := range ExtractionOps {
		if op.Table == documentsTable {
			continue
		}
		variants = append(variants, buildVariant(op))
	}
	return variants
}

// documentVariants returns the anyOf branches for the document table only.
func documentVariants() []any {
	var variants []any
	for _, op := range ExtractionOps {
		if op.Table != documentsTable {
			continue
		}
		variants = append(variants, buildDocumentVariant(op))
	}
	return variants
}

// buildDataSchema constructs the JSON Schema for the "data" property
// from a flattened TableOp's columns.
func buildDataSchema(op TableOp) map[string]any {
	dataProps := make(map[string]any, len(op.Columns))
	var required []any
	for _, fc := range op.Columns {
		prop := map[string]any{schemaKeyType: string(fc.Type)}
		if len(fc.Enum) > 0 {
			prop[schemaKeyEnum] = fc.Enum
		}
		dataProps[fc.Name] = prop
		if fc.Required {
			required = append(required, fc.Name)
		}
	}

	dataSchema := map[string]any{
		schemaKeyType:                 "object",
		schemaKeyProperties:           dataProps,
		schemaKeyAdditionalProperties: false,
	}
	if len(required) > 0 {
		dataSchema[schemaKeyRequired] = required
	}
	return dataSchema
}

// buildVariant constructs a single anyOf branch for an operation (non-document).
func buildVariant(op TableOp) map[string]any {
	return map[string]any{
		schemaKeyType:     "object",
		schemaKeyRequired: []any{schemaKeyAction, schemaKeyTable, schemaKeyData},
		schemaKeyProperties: map[string]any{
			schemaKeyAction: map[string]any{
				schemaKeyType: "string",
				schemaKeyEnum: []any{op.Action},
			},
			schemaKeyTable: map[string]any{
				schemaKeyType: "string",
				schemaKeyEnum: []any{op.Table},
			},
			schemaKeyData: buildDataSchema(op),
		},
		schemaKeyAdditionalProperties: false,
	}
}

// buildDocumentVariant constructs a single anyOf branch for a document
// operation. Unlike buildVariant, it has no "table" property (implied).
func buildDocumentVariant(op TableOp) map[string]any {
	return map[string]any{
		schemaKeyType:     "object",
		schemaKeyRequired: []any{schemaKeyAction, schemaKeyData},
		schemaKeyProperties: map[string]any{
			schemaKeyAction: map[string]any{
				schemaKeyType: "string",
				schemaKeyEnum: []any{op.Action},
			},
			schemaKeyData: buildDataSchema(op),
		},
		schemaKeyAdditionalProperties: false,
	}
}

// ValidateOperations checks each operation against the allowed tables and
// action types. Returns an error describing the first violation found.
func ValidateOperations(ops []Operation, allowed map[string]AllowedOps) error {
	for i, op := range ops {
		action := Action(strings.ToLower(strings.TrimSpace(string(op.Action))))
		table := strings.ToLower(strings.TrimSpace(op.Table))

		switch action {
		case ActionCreate, ActionUpdate:
		default:
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

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}
