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
	raw := `{"operations": [
		{"action": "create", "table": "vendors", "data": {"name": "Garcia Plumbing"}},
		{"action": "update", "table": "documents", "data": {"title": "Invoice", "notes": "Repair"}}
	]}`
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
	raw := "```json\n" + `{"operations": [{"action": "create", "table": "vendors", "data": {"name": "Test"}}]}` + "\n```"
	_, err := ParseOperations(raw)
	assert.Error(t, err)
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
	_, err := ParseOperations(`{"operations": []}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no operations found")
}

func TestParseOperations_InvalidWrapper(t *testing.T) {
	_, err := ParseOperations(`{"operations": "not an array"}`)
	assert.Error(t, err)
}

func TestParseOperations_RawArrayRejected(t *testing.T) {
	raw := `[{"action": "create", "table": "vendors", "data": {"name": "Test"}}]`
	_, err := ParseOperations(raw)
	assert.Error(t, err, "raw arrays should be rejected; schema requires object wrapper")
}

// --- OperationsSchema ---

func TestOperationsSchema(t *testing.T) {
	schema := OperationsSchema()
	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)

	opsProp, ok := props["operations"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "array", opsProp["type"])

	items, ok := opsProp["items"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", items["type"])

	itemProps, ok := items["properties"].(map[string]any)
	require.True(t, ok)

	actionProp, ok := itemProps["action"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", actionProp["type"])

	tableProp, ok := itemProps["table"].(map[string]any)
	require.True(t, ok)
	tableEnum, ok := tableProp["enum"].([]any)
	require.True(t, ok)
	assert.Contains(t, tableEnum, "documents")
	assert.Contains(t, tableEnum, "vendors")
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

// --- ParseUint ---

func TestParseUint(t *testing.T) {
	assert.Equal(t, uint(42), ParseUint(float64(42)))
	assert.Equal(t, uint(42), ParseUint("42"))
	assert.Equal(t, uint(42), ParseUint(" 42 "))
	assert.Equal(t, uint(0), ParseUint(float64(-1)))
	assert.Equal(t, uint(0), ParseUint("abc"))
	assert.Equal(t, uint(0), ParseUint(nil))
}
