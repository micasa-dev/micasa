// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildExtractionPrompt(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestStripCodeFences(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			assert.Equal(t, tt.expect, StripCodeFences(tt.input))
		})
	}
}

// --- Schema context formatting tests ---

func TestFormatDDLBlock(t *testing.T) {
	t.Parallel()
	ddl := map[string]string{
		"vendors":   "CREATE TABLE `vendors` (`id` integer, `name` text)",
		"documents": "CREATE TABLE `documents` (`id` integer, `title` text)",
	}
	result := FormatDDLBlock(ddl, []string{"vendors", "documents"})
	assert.Contains(t, result, "CREATE TABLE `vendors`")
	assert.Contains(t, result, "CREATE TABLE `documents`")
}

func TestFormatDDLBlock_MissingTable(t *testing.T) {
	t.Parallel()
	ddl := map[string]string{
		"vendors": "CREATE TABLE `vendors` (`id` integer)",
	}
	result := FormatDDLBlock(ddl, []string{"vendors", "nonexistent"})
	assert.Contains(t, result, "vendors")
	assert.NotContains(t, result, "nonexistent")
}

func TestFormatEntityRows(t *testing.T) {
	t.Parallel()
	rows := []EntityRow{{ID: 1, Name: "Garcia Plumbing"}, {ID: 2, Name: "Acme Electric"}}
	result := FormatEntityRows("vendors", rows)
	assert.Contains(t, result, "-- vendors (id, name)")
	assert.Contains(t, result, "-- 1, Garcia Plumbing")
	assert.Contains(t, result, "-- 2, Acme Electric")
}

func TestFormatEntityRows_Empty(t *testing.T) {
	t.Parallel()
	result := FormatEntityRows("vendors", nil)
	assert.Empty(t, result)
}
