// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ParseOperations ---

func TestParseOperations_Valid(t *testing.T) {
	t.Parallel()
	raw := `{"operations": [
		{"action": "create", "table": "vendors", "data": {"name": "Garcia Plumbing"}}
	], "document": {"action": "update", "data": {"id": 42, "title": "Invoice", "notes": "Repair"}}}`
	ops, err := ParseOperations(raw)
	require.NoError(t, err)
	require.Len(t, ops, 2)

	assert.Equal(t, ActionCreate, ops[0].Action)
	assert.Equal(t, data.TableVendors, ops[0].Table)
	assert.Equal(t, "Garcia Plumbing", ops[0].Data["name"])

	assert.Equal(t, ActionUpdate, ops[1].Action)
	assert.Equal(t, documentsTable, ops[1].Table)
	assert.Equal(t, "Invoice", ops[1].Data["title"])
}

func TestParseOperations_DocumentOnly(t *testing.T) {
	t.Parallel()
	raw := `{"operations": [], "document": {"action": "update", "data": {"id": 1, "title": "Receipt"}}}`
	ops, err := ParseOperations(raw)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, ActionUpdate, ops[0].Action)
	assert.Equal(t, documentsTable, ops[0].Table)
	assert.Equal(t, "Receipt", ops[0].Data["title"])
}

func TestParseOperations_NoDocument(t *testing.T) {
	t.Parallel()
	raw := `{"operations": [
		{"action": "create", "table": "vendors", "data": {"name": "Test Vendor"}}
	]}`
	ops, err := ParseOperations(raw)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, ActionCreate, ops[0].Action)
	assert.Equal(t, data.TableVendors, ops[0].Table)
}

func TestParseOperations_RejectsCodeFences(t *testing.T) {
	t.Parallel()
	raw := "```json\n" + `{"operations": [{"action": "create", "table": "vendors", "data": {"name": "Test"}}]}` + "\n```"
	_, err := ParseOperations(raw)
	assert.Error(t, err)
}

func TestParseOperations_Empty(t *testing.T) {
	t.Parallel()
	_, err := ParseOperations("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty LLM output")
}

func TestParseOperations_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := ParseOperations("I don't understand the question")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse operations json")
}

func TestParseOperations_EmptyArray(t *testing.T) {
	t.Parallel()
	_, err := ParseOperations(`{"operations": []}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no operations found")
}

func TestParseOperations_InvalidWrapper(t *testing.T) {
	t.Parallel()
	_, err := ParseOperations(`{"operations": "not an array"}`)
	assert.Error(t, err)
}

func TestParseOperations_RawArrayRejected(t *testing.T) {
	t.Parallel()
	raw := `[{"action": "create", "table": "vendors", "data": {"name": "Test"}}]`
	_, err := ParseOperations(raw)
	assert.Error(t, err, "raw arrays should be rejected; schema requires object wrapper")
}

// --- OperationsSchema ---

func TestOperationsSchema_TopLevel(t *testing.T) {
	t.Parallel()
	schema := OperationsSchema()
	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, false, schema["additionalProperties"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)

	opsProp, ok := props["operations"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "array", opsProp["type"])

	items, ok := opsProp["items"].(map[string]any)
	require.True(t, ok)

	variants, ok := items["anyOf"].([]any)
	require.True(t, ok)
	assert.Len(t, variants, 11, "expected 11 non-document variants")

	docProp, ok := props["document"].(map[string]any)
	require.True(t, ok)

	docVariants, ok := docProp["anyOf"].([]any)
	require.True(t, ok)
	assert.Len(t, docVariants, 2, "expected 2 document variants (create + update)")
}

func TestOperationsSchema_VariantStructure(t *testing.T) {
	t.Parallel()
	variants := operationVariants()

	for i, v := range variants {
		variant, ok := v.(map[string]any)
		require.True(t, ok, "variant %d is not a map", i)
		assert.Equal(t, "object", variant["type"])
		assert.Equal(t, false, variant["additionalProperties"])

		required, ok := variant["required"].([]any)
		require.True(t, ok, "variant %d missing required", i)
		assert.Contains(t, required, "action")
		assert.Contains(t, required, "table")
		assert.Contains(t, required, "data")

		props, ok := variant["properties"].(map[string]any)
		require.True(t, ok)

		actionProp, ok := props["action"].(map[string]any)
		require.True(t, ok)
		actionEnum, ok := actionProp["enum"].([]any)
		require.True(t, ok)
		assert.Len(t, actionEnum, 1, "each variant constrains action to one value")

		tableProp, ok := props["table"].(map[string]any)
		require.True(t, ok)
		tableEnum, ok := tableProp["enum"].([]any)
		require.True(t, ok)
		assert.Len(t, tableEnum, 1, "each variant constrains table to one value")

		dataProp, ok := props["data"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "object", dataProp["type"])
		assert.Equal(t, false, dataProp["additionalProperties"],
			"variant %d data must disallow additional properties", i)
	}
}

func TestOperationsSchema_DocumentVariantStructure(t *testing.T) {
	t.Parallel()
	variants := documentVariants()
	require.Len(t, variants, 2)

	for i, v := range variants {
		variant, ok := v.(map[string]any)
		require.True(t, ok, "document variant %d is not a map", i)
		assert.Equal(t, "object", variant["type"])
		assert.Equal(t, false, variant["additionalProperties"])

		required, ok := variant["required"].([]any)
		require.True(t, ok, "document variant %d missing required", i)
		assert.Contains(t, required, "action")
		assert.Contains(t, required, "data")
		assert.NotContains(t, required, "table")

		props, ok := variant["properties"].(map[string]any)
		require.True(t, ok)

		_, hasTable := props["table"]
		assert.False(t, hasTable, "document variant %d should not have table property", i)

		actionProp, ok := props["action"].(map[string]any)
		require.True(t, ok)
		actionEnum, ok := actionProp["enum"].([]any)
		require.True(t, ok)
		assert.Len(t, actionEnum, 1)

		dataProp, ok := props["data"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "object", dataProp["type"])
		assert.Equal(t, false, dataProp["additionalProperties"])
	}
}

func TestOperationsSchema_CoversTables(t *testing.T) {
	t.Parallel()
	type tableAction struct {
		table  string
		action Action
	}

	// Non-document operation variants.
	opVariants := operationVariants()
	expectedOps := []tableAction{
		{data.TableVendors, ActionCreate},
		{data.TableVendors, ActionUpdate},
		{data.TableAppliances, ActionCreate},
		{data.TableAppliances, ActionUpdate},
		{data.TableProjects, ActionCreate},
		{data.TableQuotes, ActionCreate},
		{data.TableQuotes, ActionUpdate},
		{data.TableMaintenanceItems, ActionCreate},
		{data.TableMaintenanceItems, ActionUpdate},
		{data.TableIncidents, ActionCreate},
		{data.TableServiceLogEntries, ActionCreate},
	}
	require.Len(t, opVariants, len(expectedOps))

	seenOps := make(map[tableAction]bool)
	for _, op := range ExtractionOps {
		if op.Table == documentsTable {
			continue
		}
		seenOps[tableAction{op.Table, op.Action}] = true
	}
	for _, ta := range expectedOps {
		assert.True(t, seenOps[ta], "missing op variant for %s/%s", ta.action, ta.table)
	}

	// Document variants.
	docVars := documentVariants()
	expectedDocs := []tableAction{
		{documentsTable, ActionCreate},
		{documentsTable, ActionUpdate},
	}
	require.Len(t, docVars, len(expectedDocs))

	seenDocs := make(map[tableAction]bool)
	for _, op := range ExtractionOps {
		if op.Table == documentsTable {
			seenDocs[tableAction{op.Table, op.Action}] = true
		}
	}
	for _, ta := range expectedDocs {
		assert.True(t, seenDocs[ta], "missing doc variant for %s/%s", ta.action, ta.table)
	}
}

func TestOperationsSchema_NoDocumentInOperations(t *testing.T) {
	t.Parallel()
	for i, v := range operationVariants() {
		variant, ok := v.(map[string]any)
		require.True(t, ok)
		props, ok := variant["properties"].(map[string]any)
		require.True(t, ok, "variant %d missing properties", i)
		tableProp, ok := props["table"].(map[string]any)
		require.True(t, ok, "variant %d missing table", i)
		tableEnum, ok := tableProp["enum"].([]any)
		require.True(t, ok, "variant %d missing table enum", i)
		assert.NotEqual(t, documentsTable, tableEnum[0],
			"operation variant %d should not be for documents", i)
	}
}

func TestOperationsSchema_VendorsCreateColumns(t *testing.T) {
	t.Parallel()
	variant := findVariant(t, ActionCreate, data.TableVendors)
	dataProps := variantDataProps(t, variant)

	expected := []string{"name", "contact_name", "email", "phone", "website", "notes"}
	assert.Len(t, dataProps, len(expected))
	for _, col := range expected {
		_, ok := dataProps[col]
		assert.True(t, ok, "missing column %q", col)
	}

	dataRequired := variantDataRequired(t, variant)
	assert.Contains(t, dataRequired, "name")
}

func TestOperationsSchema_DocumentsUpdateRequiresID(t *testing.T) {
	t.Parallel()
	variant := findDocumentVariant(t, ActionUpdate)
	dataRequired := variantDataRequired(t, variant)
	assert.Contains(t, dataRequired, "id")
}

func TestOperationsSchema_MaintenanceUpdateRequiresID(t *testing.T) {
	t.Parallel()
	variant := findVariant(t, ActionUpdate, data.TableMaintenanceItems)
	dataRequired := variantDataRequired(t, variant)
	assert.Contains(t, dataRequired, "id")
}

func TestOperationsSchema_EntityKindEnum(t *testing.T) {
	t.Parallel()
	variant := findDocumentVariant(t, ActionUpdate)
	dataProps := variantDataProps(t, variant)

	ekProp, ok := dataProps["entity_kind"].(map[string]any)
	require.True(t, ok)
	ekEnum, ok := ekProp["enum"].([]any)
	require.True(t, ok)
	assert.Contains(t, ekEnum, "project")
	assert.Contains(t, ekEnum, "vendor")
	assert.Contains(t, ekEnum, "maintenance")
}

func TestOperationsSchema_QuotesCreateColumns(t *testing.T) {
	t.Parallel()
	variant := findVariant(t, ActionCreate, data.TableQuotes)
	dataProps := variantDataProps(t, variant)
	dataRequired := variantDataRequired(t, variant)

	assert.Contains(t, dataRequired, "total_cents")

	expected := []string{
		"project_id", "vendor_id", "vendor_name",
		"total_cents", "labor_cents", "materials_cents", "notes",
	}
	assert.Len(t, dataProps, len(expected))
	for _, col := range expected {
		_, ok := dataProps[col]
		assert.True(t, ok, "missing column %q", col)
	}
}

// --- schema test helpers ---

// findVariant builds the schema variant for the given {action, table} pair
// by looking up ExtractionOps and calling buildVariant directly.
func findVariant(t *testing.T, action Action, table string) map[string]any {
	t.Helper()
	for _, op := range ExtractionOps {
		if op.Action == action && op.Table == table {
			return buildVariant(op)
		}
	}
	t.Fatalf("no variant for %s/%s", action, table)
	return nil
}

// findDocumentVariant builds the document schema variant for the given action
// by looking up ExtractionOps and calling buildDocumentVariant directly.
func findDocumentVariant(t *testing.T, action Action) map[string]any {
	t.Helper()
	for _, op := range ExtractionOps {
		if op.Action == action && op.Table == documentsTable {
			return buildDocumentVariant(op)
		}
	}
	t.Fatalf("no document variant for %s", action)
	return nil
}

func variantDataProps(t *testing.T, variant map[string]any) map[string]any {
	t.Helper()
	props, ok := variant["properties"].(map[string]any)
	require.True(t, ok, "variant missing properties")
	dataProp, ok := props["data"].(map[string]any)
	require.True(t, ok, "variant missing data")
	dataProps, ok := dataProp["properties"].(map[string]any)
	require.True(t, ok, "data missing properties")
	return dataProps
}

func variantDataRequired(t *testing.T, variant map[string]any) []any {
	t.Helper()
	props, ok := variant["properties"].(map[string]any)
	require.True(t, ok, "variant missing properties")
	dataProp, ok := props["data"].(map[string]any)
	require.True(t, ok, "variant missing data")
	req, _ := dataProp["required"].([]any)
	return req
}

// --- ValidateOperations ---

var testAllowedOps = map[string]AllowedOps{
	documentsTable:             {Update: true},
	data.TableVendors:          {Insert: true},
	data.TableQuotes:           {Insert: true},
	data.TableMaintenanceItems: {Insert: true},
	data.TableAppliances:       {Insert: true},
}

func TestValidateOperations_Valid(t *testing.T) {
	t.Parallel()
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{"name": "Test"}},
		{Action: ActionUpdate, Table: documentsTable, Data: map[string]any{"title": "Doc"}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	assert.NoError(t, err)
}

func TestValidateOperations_InvalidAction(t *testing.T) {
	t.Parallel()
	ops := []Operation{
		{
			Action: "delete",
			Table:  data.TableVendors,
			Data:   map[string]any{"id": 1},
		}, //nolint:exhaustive // intentionally invalid
	}
	err := ValidateOperations(ops, testAllowedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "action must be")
}

func TestValidateOperations_UnknownTable(t *testing.T) {
	t.Parallel()
	ops := []Operation{
		{Action: ActionCreate, Table: "users", Data: map[string]any{"name": "Test"}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the allowed set")
}

func TestValidateOperations_CreateOnUpdateOnlyTable(t *testing.T) {
	t.Parallel()
	ops := []Operation{
		{Action: ActionCreate, Table: documentsTable, Data: map[string]any{"title": "X"}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create not allowed")
}

func TestValidateOperations_UpdateOnInsertOnlyTable(t *testing.T) {
	t.Parallel()
	ops := []Operation{
		{Action: ActionUpdate, Table: data.TableVendors, Data: map[string]any{"name": "X"}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update not allowed")
}

func TestValidateOperations_EmptyData(t *testing.T) {
	t.Parallel()
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{}},
	}
	err := ValidateOperations(ops, testAllowedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data must not be empty")
}

// --- ParseStringID ---

func TestParseStringID(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "42", ParseStringID(float64(42)))
	assert.Equal(t, "42", ParseStringID("42"))
	assert.Equal(t, "42", ParseStringID(" 42 "))
	assert.Equal(t, "-1", ParseStringID(float64(-1)))
	assert.Equal(t, "abc", ParseStringID("abc"))
	assert.Empty(t, ParseStringID(nil))
	// Concrete integer types from GORM/SQLite map queries.
	assert.Equal(t, "7", ParseStringID(uint(7)))
	assert.Equal(t, "9", ParseStringID(uint64(9)))
	assert.Equal(t, "5", ParseStringID(int64(5)))
	assert.Equal(t, "-3", ParseStringID(int64(-3)))
	assert.Equal(t, "11", ParseStringID(int(11)))
	assert.Equal(t, "-1", ParseStringID(int(-1)))
	// json.Number from UseNumber decoder (implements String()).
	assert.Equal(t, "99", ParseStringID(json.Number("99")))
	assert.Equal(t, "not-a-number", ParseStringID(json.Number("not-a-number")))
	// []byte from GORM raw queries.
	assert.Equal(t, "01JTEST", ParseStringID([]byte("01JTEST")))
	// Unsupported type.
	assert.Empty(t, ParseStringID([]int{1}))
	// ULID string passes through.
	assert.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV", ParseStringID("01ARZ3NDEKTSV4RRFFQ69G5FAV"))
}

// --- ParseInt64 ---

func TestParseInt64(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int64(0), ParseInt64(nil))
	assert.Equal(t, int64(42), ParseInt64(int64(42)))
	assert.Equal(t, int64(-3), ParseInt64(int64(-3)))
	assert.Equal(t, int64(7), ParseInt64(float64(7)))
	assert.Equal(t, int64(5), ParseInt64(int(5)))
	assert.Equal(t, int64(10), ParseInt64(uint(10)))
	assert.Equal(t, int64(0), ParseInt64(uint(math.MaxInt64+1)))
	assert.Equal(t, int64(99), ParseInt64("99"))
	assert.Equal(t, int64(0), ParseInt64("abc"))
	assert.Equal(t, int64(77), ParseInt64(json.Number("77")))
	assert.Equal(t, int64(0), ParseInt64(json.Number("nope")))
	// Unsupported type with no String() method.
	assert.Equal(t, int64(0), ParseInt64([]int{1}))
}
