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
	schema := SchemaContext{
		DDL: map[string]string{
			"vendors":   "CREATE TABLE `vendors` (`id` integer PRIMARY KEY AUTOINCREMENT, `name` text)",
			"documents": "CREATE TABLE `documents` (`id` integer PRIMARY KEY AUTOINCREMENT, `title` text)",
		},
		Vendors:    []EntityRow{{ID: 1, Name: "Garcia Plumbing"}, {ID: 2, Name: "Acme Electric"}},
		Projects:   []EntityRow{{ID: 1, Name: "Kitchen Remodel"}},
		Appliances: []EntityRow{{ID: 1, Name: "HVAC Unit"}},
	}
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:     42,
		Filename:  "invoice.pdf",
		MIME:      "application/pdf",
		SizeBytes: 12345,
		Schema:    schema,
		Sources: []TextSource{
			{Tool: "pdftotext", Desc: "Digital text.", Text: "Invoice text here"},
		},
	})

	require.Len(t, msgs, 2)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Equal(t, "user", msgs[1].Role)

	// System prompt should include DDL and entity rows.
	sys := msgs[0].Content
	assert.Contains(t, sys, "CREATE TABLE")
	assert.Contains(t, sys, "Garcia Plumbing")
	assert.Contains(t, sys, "Kitchen Remodel")
	assert.Contains(t, sys, "HVAC Unit")
	assert.Contains(t, sys, "create")
	assert.Contains(t, sys, "update")

	// User message should include document ID, metadata, and text.
	user := msgs[1].Content
	assert.Contains(t, user, "Document ID: 42")
	assert.Contains(t, user, "invoice.pdf")
	assert.Contains(t, user, "application/pdf")
	assert.Contains(t, user, "Invoice text here")
}

func TestBuildExtractionPrompt_DualSources(t *testing.T) {
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:    1,
		Filename: "mixed.pdf",
		MIME:     "application/pdf",
		Sources: []TextSource{
			{Tool: "pdftotext", Desc: "Digital text.", Text: "Digital text from pages 1-2"},
			{Tool: "tesseract", Desc: "OCR text.", Text: "OCR text from page 3"},
		},
	})

	require.Len(t, msgs, 2)
	user := msgs[1].Content
	assert.Contains(t, user, "Source: pdftotext")
	assert.Contains(t, user, "Source: tesseract")
	assert.Contains(t, user, "Digital text from pages 1-2")
	assert.Contains(t, user, "OCR text from page 3")
}

func TestBuildExtractionPrompt_OCROnly(t *testing.T) {
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:    1,
		Filename: "scan.pdf",
		MIME:     "application/pdf",
		Sources: []TextSource{
			{Tool: "tesseract", Desc: "OCR text.", Text: "OCR text from all pages"},
		},
	})

	require.Len(t, msgs, 2)
	user := msgs[1].Content
	assert.Contains(t, user, "Source: tesseract")
	assert.NotContains(t, user, "Source: pdftotext")
}

func TestBuildExtractionPrompt_NoEntities(t *testing.T) {
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:    1,
		Filename: "doc.txt",
		MIME:     "text/plain",
		Sources: []TextSource{
			{Tool: "plaintext", Text: "Some text"},
		},
	})
	require.Len(t, msgs, 2)
	assert.NotContains(t, msgs[0].Content, "Existing rows")
}

func TestBuildExtractionPrompt_ZeroDocID(t *testing.T) {
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:    0,
		Filename: "new.pdf",
		MIME:     "application/pdf",
		Sources: []TextSource{
			{Tool: "pdftotext", Text: "Some text"},
		},
	})
	require.Len(t, msgs, 2)
	user := msgs[1].Content
	assert.NotContains(t, user, "Document ID:", "zero DocID should omit Document ID line")
	assert.Contains(t, user, "new.pdf")
}

func TestBuildExtractionPrompt_NonZeroDocID(t *testing.T) {
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:    42,
		Filename: "existing.pdf",
		MIME:     "application/pdf",
		Sources: []TextSource{
			{Tool: "pdftotext", Text: "Some text"},
		},
	})
	require.Len(t, msgs, 2)
	assert.Contains(t, msgs[1].Content, "Document ID: 42")
}

// --- Tests for JSON parsing (kept until ExtractionHints removal) ---

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

func TestParseExtractionResponse_CurrencyUnitDollars(t *testing.T) {
	raw := `{
		"document_type": "invoice",
		"currency_unit": "dollars",
		"total_cents": 1500,
		"labor_cents": 800,
		"materials_cents": 700
	}`
	hints, err := ParseExtractionResponse(raw)
	require.NoError(t, err)
	require.NotNil(t, hints.TotalCents)
	assert.Equal(t, int64(150000), *hints.TotalCents)
	require.NotNil(t, hints.LaborCents)
	assert.Equal(t, int64(80000), *hints.LaborCents)
	require.NotNil(t, hints.MaterialsCents)
	assert.Equal(t, int64(70000), *hints.MaterialsCents)
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
		name      string
		input     any
		isDollars bool
		expect    *int64
	}{
		{"nil", nil, false, nil},
		{"float64 cents", float64(150000), false, ptr(int64(150000))},
		{"float64 zero", float64(0), false, nil},
		{"float64 dollars", float64(1500), true, ptr(int64(150000))},
		{"float64 dollars with fractional", float64(1500.50), true, ptr(int64(150050))},
		{"float64 dollars zero", float64(0), true, nil},
		{"string dollar", "$1,500.00", false, ptr(int64(150000))},
		{"string no dollar sign", "1,500.00", false, ptr(int64(150000))},
		{"string no commas", "1500.00", false, ptr(int64(150000))},
		{"string bare cents", "150000", false, ptr(int64(150000))},
		{"string empty", "", false, nil},
		{"bool", true, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCents(tt.input, tt.isDollars)
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
		{
			"sql fence",
			"```sql\nINSERT INTO vendors (name) VALUES ('Test');\n```",
			"INSERT INTO vendors (name) VALUES ('Test');",
		},
		{
			"commentary before fence",
			"Here are the operations:\n```json\n{\"key\": \"val\"}\n```",
			`{"key": "val"}`,
		},
		{"commentary before and after", "Sure!\n```json\n[1,2,3]\n```\nDone.", "[1,2,3]"},
		{"no closing fence", "```json\n{\"key\": \"val\"}", `{"key": "val"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, StripCodeFences(tt.input))
		})
	}
}

func ptr[T any](v T) *T { return &v }

// --- Schema context formatting tests ---

func TestFormatDDLBlock(t *testing.T) {
	ddl := map[string]string{
		"vendors":   "CREATE TABLE `vendors` (`id` integer, `name` text)",
		"documents": "CREATE TABLE `documents` (`id` integer, `title` text)",
	}
	result := FormatDDLBlock(ddl, []string{"vendors", "documents"})
	assert.Contains(t, result, "CREATE TABLE `vendors`")
	assert.Contains(t, result, "CREATE TABLE `documents`")
}

func TestFormatDDLBlock_MissingTable(t *testing.T) {
	ddl := map[string]string{
		"vendors": "CREATE TABLE `vendors` (`id` integer)",
	}
	result := FormatDDLBlock(ddl, []string{"vendors", "nonexistent"})
	assert.Contains(t, result, "vendors")
	assert.NotContains(t, result, "nonexistent")
}

func TestFormatEntityRows(t *testing.T) {
	rows := []EntityRow{{ID: 1, Name: "Garcia Plumbing"}, {ID: 2, Name: "Acme Electric"}}
	result := FormatEntityRows("vendors", rows)
	assert.Contains(t, result, "-- vendors (id, name)")
	assert.Contains(t, result, "-- 1, Garcia Plumbing")
	assert.Contains(t, result, "-- 2, Acme Electric")
}

func TestFormatEntityRows_Empty(t *testing.T) {
	result := FormatEntityRows("vendors", nil)
	assert.Empty(t, result)
}
