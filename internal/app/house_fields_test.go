// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// houseFormFields returns the reflect.StructField slice for houseFormData,
// used by reflection-based completeness tests.
func houseFormFields() []reflect.StructField {
	rt := reflect.TypeFor[houseFormData]()
	fields := make([]reflect.StructField, 0, rt.NumField())
	for f := range rt.Fields() {
		fields = append(fields, f)
	}
	return fields
}

// toSnakeCase converts a CamelCase Go field name to snake_case.
// Handles consecutive uppercase runs (e.g. "HOAName" → "hoa_name")
// and embedded digits (e.g. "AddressLine1" → "address_line1").
func toSnakeCase(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i == 0 {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		if unicode.IsUpper(r) {
			// Insert underscore before uppercase that follows lowercase,
			// or before the last letter of a consecutive uppercase run
			// when followed by a lowercase letter.
			prev := runes[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				b.WriteByte('_')
			} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestToSnakeCase(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"Nickname", "nickname"},
		{"PostalCode", "postal_code"},
		{"AddressLine1", "address_line1"},
		{"AddressLine2", "address_line2"},
		{"YearBuilt", "year_built"},
		{"SquareFeet", "square_feet"},
		{"LotSquareFeet", "lot_square_feet"},
		{"HOAName", "hoa_name"},
		{"HOAFee", "hoa_fee"},
		{"InsuranceCarrier", "insurance_carrier"},
		{"PropertyTax", "property_tax"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, toSnakeCase(tc.input), "toSnakeCase(%q)", tc.input)
	}
}

func TestHouseFieldDefsComplete(t *testing.T) {
	t.Parallel()
	defs := houseFieldDefs()
	keys := make(map[string]bool, len(defs))
	for _, d := range defs {
		require.False(t, keys[d.key], "duplicate key: %s", d.key)
		keys[d.key] = true
	}
	// Derive expected keys from houseFormData struct fields.
	expected := make(map[string]bool)
	for _, f := range houseFormFields() {
		key := toSnakeCase(f.Name)
		expected[key] = true
		assert.True(t, keys[key], "houseFormData.%s (key %q) has no field def", f.Name, key)
	}
	// Check no stale/extra defs exist without matching form field.
	for _, d := range defs {
		assert.True(t, expected[d.key], "field def %q has no matching houseFormData field", d.key)
	}
}

func TestHouseFieldDefSections(t *testing.T) {
	t.Parallel()
	defs := houseFieldDefs()
	for _, d := range defs {
		assert.NotEmpty(t, d.label, "field %s has empty label", d.key)
		assert.True(t, d.section >= houseSectionIdentity && d.section <= houseSectionFinancial,
			"field %s has invalid section", d.key)
	}
}

func TestHouseFieldDefGetSet(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	defs := houseFieldDefs()
	fd := m.houseFormValues(m.house)
	for _, d := range defs {
		val := d.get(m.house, m.cur, m.unitSystem)
		// Write through ptr, verify it sticks.
		*d.ptr(fd) = val
		assert.Equal(t, val, *d.ptr(fd), "ptr round-trip failed for %s", d.key)
	}
}
