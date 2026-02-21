// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildExtractionPrompt(t *testing.T) {
	entities := EntityContext{
		Vendors:    []string{"Garcia Plumbing", "Acme Electric"},
		Projects:   []string{"Kitchen Remodel"},
		Appliances: []string{"HVAC Unit"},
	}
	msgs := BuildExtractionPrompt(
		"invoice.pdf",
		"application/pdf",
		12345,
		entities,
		"Invoice text here",
	)

	require.Len(t, msgs, 2)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Equal(t, "user", msgs[1].Role)

	// System prompt should include entity context.
	assert.Contains(t, msgs[0].Content, "Garcia Plumbing")
	assert.Contains(t, msgs[0].Content, "Kitchen Remodel")
	assert.Contains(t, msgs[0].Content, "HVAC Unit")

	// User message should include metadata and text.
	assert.Contains(t, msgs[1].Content, "invoice.pdf")
	assert.Contains(t, msgs[1].Content, "application/pdf")
	assert.Contains(t, msgs[1].Content, "Invoice text here")
}

func TestBuildExtractionPrompt_NoEntities(t *testing.T) {
	msgs := BuildExtractionPrompt("doc.txt", "text/plain", 100, EntityContext{}, "Some text")
	require.Len(t, msgs, 2)
	assert.NotContains(t, msgs[0].Content, "Existing entities")
}

func TestParseExtractionResponse_FullResponse(t *testing.T) {
	raw := `{
		"document_type": "invoice",
		"title_suggestion": "Garcia Plumbing Invoice Jan 2025",
		"summary": "Plumbing repair invoice for $1,500",
		"vendor_hint": "Garcia Plumbing",
		"total_cents": 150000,
		"labor_cents": 80000,
		"materials_cents": 70000,
		"date": "2025-01-15",
		"warranty_expiry": "2027-01-15",
		"entity_kind_hint": "quote",
		"entity_name_hint": "Kitchen Remodel",
		"maintenance_items": [
			{"name": "Replace filter", "interval_months": 3}
		],
		"notes": "Paid in full"
	}`

	hints, err := ParseExtractionResponse(raw)
	require.NoError(t, err)

	assert.Equal(t, "invoice", hints.DocumentType)
	assert.Equal(t, "Garcia Plumbing Invoice Jan 2025", hints.TitleSugg)
	assert.Equal(t, "Plumbing repair invoice for $1,500", hints.Summary)
	assert.Equal(t, "Garcia Plumbing", hints.VendorHint)
	require.NotNil(t, hints.TotalCents)
	assert.Equal(t, int64(150000), *hints.TotalCents)
	require.NotNil(t, hints.LaborCents)
	assert.Equal(t, int64(80000), *hints.LaborCents)
	require.NotNil(t, hints.MaterialsCents)
	assert.Equal(t, int64(70000), *hints.MaterialsCents)
	require.NotNil(t, hints.Date)
	assert.Equal(t, 2025, hints.Date.Year())
	assert.Equal(t, time.January, hints.Date.Month())
	assert.Equal(t, 15, hints.Date.Day())
	require.NotNil(t, hints.WarrantyExpiry)
	assert.Equal(t, 2027, hints.WarrantyExpiry.Year())
	assert.Equal(t, "quote", hints.EntityKindHint)
	assert.Equal(t, "Kitchen Remodel", hints.EntityNameHint)
	require.Len(t, hints.Maintenance, 1)
	assert.Equal(t, "Replace filter", hints.Maintenance[0].Name)
	assert.Equal(t, 3, hints.Maintenance[0].IntervalMonths)
	assert.Equal(t, "Paid in full", hints.Notes)
}

func TestParseExtractionResponse_Partial(t *testing.T) {
	raw := `{"document_type": "receipt", "vendor_hint": "Home Depot"}`
	hints, err := ParseExtractionResponse(raw)
	require.NoError(t, err)

	assert.Equal(t, "receipt", hints.DocumentType)
	assert.Equal(t, "Home Depot", hints.VendorHint)
	assert.Nil(t, hints.TotalCents)
	assert.Nil(t, hints.Date)
	assert.Empty(t, hints.Maintenance)
}

func TestParseExtractionResponse_WithCodeFences(t *testing.T) {
	raw := "```json\n{\"document_type\": \"manual\"}\n```"
	hints, err := ParseExtractionResponse(raw)
	require.NoError(t, err)
	assert.Equal(t, "manual", hints.DocumentType)
}

func TestParseExtractionResponse_InvalidJSON(t *testing.T) {
	_, err := ParseExtractionResponse("not json at all")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse extraction json")
}

func TestParseExtractionResponse_InvalidEnum(t *testing.T) {
	raw := `{"document_type": "banana", "entity_kind_hint": "spaceship"}`
	hints, err := ParseExtractionResponse(raw)
	require.NoError(t, err)
	assert.Empty(t, hints.DocumentType, "invalid document_type should be dropped")
	assert.Empty(t, hints.EntityKindHint, "invalid entity_kind_hint should be dropped")
}

func TestParseExtractionResponse_EmptyMaintenanceItems(t *testing.T) {
	raw := `{"maintenance_items": [{"name": "", "interval_months": 3}, {"name": "Filter", "interval_months": 0}]}`
	hints, err := ParseExtractionResponse(raw)
	require.NoError(t, err)
	assert.Empty(t, hints.Maintenance, "items with empty name or zero interval should be dropped")
}

func TestParseCents(t *testing.T) {
	tests := []struct {
		name   string
		input  any
		expect *int64
	}{
		{"nil", nil, nil},
		{"float64 cents", float64(150000), ptr(int64(150000))},
		{"float64 zero", float64(0), nil},
		{"string dollar", "$1,500.00", ptr(int64(150000))},
		{"string no dollar sign", "1,500.00", ptr(int64(150000))},
		{"string no commas", "1500.00", ptr(int64(150000))},
		{"string bare cents", "150000", ptr(int64(150000))},
		{"string empty", "", nil},
		{"bool", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCents(tt.input)
			if tt.expect == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expect, *result)
			}
		})
	}
}

func TestParseCentsFromString(t *testing.T) {
	tests := []struct {
		input  string
		expect *int64
	}{
		{"$1,234.56", ptr(int64(123456))},
		{"1,234.56", ptr(int64(123456))},
		{"1234.56", ptr(int64(123456))},
		{"$0.99", ptr(int64(99))},
		{"150000", ptr(int64(150000))},
		{"", nil},
		{"abc", nil},
		{"$abc.00", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseCentsFromString(tt.input)
			if tt.expect == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expect, *result)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		input string
		year  int
		month time.Month
		day   int
		isNil bool
	}{
		{"2025-01-15", 2025, time.January, 15, false},
		{"01/15/2025", 2025, time.January, 15, false},
		{"1/5/2025", 2025, time.January, 5, false},
		{"January 15, 2025", 2025, time.January, 15, false},
		{"Jan 15, 2025", 2025, time.January, 15, false},
		{"", 0, 0, 0, true},
		{"not a date", 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseDate(tt.input)
			if tt.isNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.year, result.Year())
				assert.Equal(t, tt.month, result.Month())
				assert.Equal(t, tt.day, result.Day())
			}
		})
	}
}

func TestParsePositiveInt(t *testing.T) {
	assert.Equal(t, 3, parsePositiveInt(float64(3)))
	assert.Equal(t, 12, parsePositiveInt(float64(12.4)))
	assert.Equal(t, 6, parsePositiveInt("6"))
	assert.Equal(t, 0, parsePositiveInt(float64(0)))
	assert.Equal(t, 0, parsePositiveInt(float64(-1)))
	assert.Equal(t, 0, parsePositiveInt("abc"))
	assert.Equal(t, 0, parsePositiveInt(nil))
}

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"no fences", `{"key": "val"}`, `{"key": "val"}`},
		{"json fence", "```json\n{\"key\": \"val\"}\n```", `{"key": "val"}`},
		{"bare fence", "```\n{\"key\": \"val\"}\n```", `{"key": "val"}`},
		{"whitespace around", "  ```json\n{\"key\": \"val\"}\n```  ", `{"key": "val"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, stripCodeFences(tt.input))
		})
	}
}

func ptr[T any](v T) *T { return &v }
