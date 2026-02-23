// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ParseOperations ---

func TestParseOperations_Valid(t *testing.T) {
	raw := `[
		{"action": "create", "table": "vendors", "data": {"name": "Garcia Plumbing"}},
		{"action": "update", "table": "documents", "data": {"title": "Invoice", "notes": "Repair"}}
	]`
	ops, err := ParseOperations(raw)
	require.NoError(t, err)
	require.Len(t, ops, 2)

	assert.Equal(t, "create", ops[0].Action)
	assert.Equal(t, "vendors", ops[0].Table)
	assert.Equal(t, "Garcia Plumbing", ops[0].Data["name"])

	assert.Equal(t, "update", ops[1].Action)
	assert.Equal(t, "documents", ops[1].Table)
	assert.Equal(t, "Invoice", ops[1].Data["title"])
}

func TestParseOperations_RejectsCodeFences(t *testing.T) {
	raw := "```json\n" + `[{"action": "create", "table": "vendors", "data": {"name": "Test"}}]` + "\n```"
	_, err := ParseOperations(raw)
	assert.Error(t, err, "code fences should be rejected; use llm.WithJSON() to constrain output")
}

func TestParseOperations_Empty(t *testing.T) {
	_, err := ParseOperations("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty LLM output")
}

func TestParseOperations_InvalidJSON(t *testing.T) {
	_, err := ParseOperations("I don't understand the question")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse operations json")
}

func TestParseOperations_EmptyArray(t *testing.T) {
	_, err := ParseOperations("[]")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no operations found")
}

// --- ValidateOperations ---

var testAllowedOps = map[string]AllowedOps{
	"documents":         {Update: true},
	"vendors":           {Insert: true},
	"quotes":            {Insert: true},
	"maintenance_items": {Insert: true},
	"appliances":        {Insert: true},
}

func TestValidateOperations_Valid(t *testing.T) {
	ops := []Operation{
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "Test"}},
		{Action: "update", Table: "documents", Data: map[string]any{"title": "Doc"}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	assert.NoError(t, err)
}

func TestValidateOperations_InvalidAction(t *testing.T) {
	ops := []Operation{
		{Action: "delete", Table: "vendors", Data: map[string]any{"id": 1}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "action must be")
}

func TestValidateOperations_UnknownTable(t *testing.T) {
	ops := []Operation{
		{Action: "create", Table: "users", Data: map[string]any{"name": "Test"}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the allowed set")
}

func TestValidateOperations_CreateOnUpdateOnlyTable(t *testing.T) {
	ops := []Operation{
		{Action: "create", Table: "documents", Data: map[string]any{"title": "X"}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create not allowed")
}

func TestValidateOperations_UpdateOnInsertOnlyTable(t *testing.T) {
	ops := []Operation{
		{Action: "update", Table: "vendors", Data: map[string]any{"name": "X"}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update not allowed")
}

func TestValidateOperations_EmptyData(t *testing.T) {
	ops := []Operation{
		{Action: "create", Table: "vendors", Data: map[string]any{}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data must not be empty")
}

// --- OperationPreview ---

func TestOperationPreview_Create(t *testing.T) {
	op := Operation{
		Action: "create",
		Table:  "vendors",
		Data:   map[string]any{"name": "Garcia Plumbing"},
	}
	row := OperationPreview(op)
	require.NotNil(t, row)
	assert.Equal(t, "vendors", row.Table)
	assert.Equal(t, "create", row.Op)
	assert.Equal(t, uint(0), row.RowID)
	assert.Equal(t, []string{"name"}, row.Columns)
	assert.Equal(t, []string{"Garcia Plumbing"}, row.Values)
}

func TestOperationPreview_UpdateWithID(t *testing.T) {
	op := Operation{
		Action: "update",
		Table:  "documents",
		Data:   map[string]any{"id": float64(42), "title": "Invoice", "notes": "Repair"},
	}
	row := OperationPreview(op)
	require.NotNil(t, row)
	assert.Equal(t, "documents", row.Table)
	assert.Equal(t, "update", row.Op)
	assert.Equal(t, uint(42), row.RowID)
	assert.Contains(t, row.Columns, "notes")
	assert.Contains(t, row.Columns, "title")
	assert.NotContains(t, row.Columns, "id")
}

func TestOperationPreview_EmptyData(t *testing.T) {
	op := Operation{Action: "create", Table: "vendors", Data: nil}
	assert.Nil(t, OperationPreview(op))
}

func TestOperationPreview_NumericValues(t *testing.T) {
	op := Operation{
		Action: "create",
		Table:  "quotes",
		Data:   map[string]any{"total_cents": float64(150000), "vendor_id": float64(1)},
	}
	row := OperationPreview(op)
	require.NotNil(t, row)
	assert.Contains(t, row.Values, "150000")
	assert.Contains(t, row.Values, "1")
}

// --- formatValue ---

func TestFormatValue(t *testing.T) {
	assert.Equal(t, "hello", formatValue("hello"))
	assert.Equal(t, "42", formatValue(float64(42)))
	assert.Equal(t, "3.14", formatValue(float64(3.14)))
	assert.Equal(t, "true", formatValue(true))
	assert.Equal(t, "false", formatValue(false))
	assert.Equal(t, "null", formatValue(nil))
}

// --- parseUintFromAny ---

func TestParseUintFromAny(t *testing.T) {
	assert.Equal(t, uint(42), parseUintFromAny(float64(42)))
	assert.Equal(t, uint(42), parseUintFromAny("42"))
	assert.Equal(t, uint(0), parseUintFromAny(float64(-1)))
	assert.Equal(t, uint(0), parseUintFromAny("abc"))
	assert.Equal(t, uint(0), parseUintFromAny(nil))
}
