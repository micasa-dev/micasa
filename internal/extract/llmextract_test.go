// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildExtractionPrompt(t *testing.T) {
	t.Parallel()
	schema := SchemaContext{
		DDL: map[string]string{
			data.TableVendors:   "CREATE TABLE `vendors` (`id` integer PRIMARY KEY AUTOINCREMENT, `name` text)",
			data.TableDocuments: "CREATE TABLE `documents` (`id` integer PRIMARY KEY AUTOINCREMENT, `title` text)",
		},
		Vendors: []EntityRow{
			{ID: "1", Name: "Garcia Plumbing"},
			{ID: "2", Name: "Acme Electric"},
		},
		Projects:   []EntityRow{{ID: "1", Name: "Kitchen Remodel"}},
		Appliances: []EntityRow{{ID: "1", Name: "HVAC Unit"}},
	}
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:     "42",
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
		DocID:    "1",
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
		DocID:    "1",
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
		DocID:    "1",
		Filename: "doc.txt",
		MIME:     "text/plain",
		Sources: []TextSource{
			{Tool: "plaintext", Text: "Some text"},
		},
	})
	require.Len(t, msgs, 2)
	assert.NotContains(t, msgs[0].Content, "Existing rows")
}

func TestBuildExtractionPrompt_EmptyDocID(t *testing.T) {
	t.Parallel()
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:    "",
		Filename: "new.pdf",
		MIME:     "application/pdf",
		Sources: []TextSource{
			{Tool: "pdftotext", Text: "Some text"},
		},
	})
	require.Len(t, msgs, 2)
	user := msgs[1].Content
	assert.NotContains(t, user, "Document ID:", "empty DocID should omit Document ID line")
	assert.Contains(t, user, "new.pdf")
}

func TestBuildExtractionPrompt_NonZeroDocID(t *testing.T) {
	t.Parallel()
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:    "42",
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

func TestOperationExtractionRules_CoversSemanticConventions(t *testing.T) {
	t.Parallel()
	// The rules section covers what the JSON schema cannot enforce:
	// semantic conventions (cents, ISO dates), FK resolution, and the
	// document linking pattern. Allowed tables and actions are enforced
	// by the schema, not the prompt.
	rules := operationExtractionRules
	assert.Contains(t, rules, "cents")
	assert.Contains(t, rules, "ISO 8601")
	assert.Contains(t, rules, "foreign key")
	assert.Contains(t, rules, "entity_kind")
	assert.Contains(t, rules, "entity_id")
}

func TestBuildExtractionPrompt_ContainsDomainHints(t *testing.T) {
	t.Parallel()
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:    "1",
		Filename: "test.pdf",
		MIME:     "application/pdf",
		Schema: SchemaContext{
			DDL: map[string]string{
				data.TableVendors: "CREATE TABLE `vendors` (`id` integer)",
			},
		},
		Sources: []TextSource{{Tool: "pdftotext", Text: "text"}},
	})

	sys := msgs[0].Content
	assert.Contains(t, sys, "Document type hints")
	assert.Contains(t, sys, "Contractor invoice")
	assert.Contains(t, sys, "Appliance manual")
	assert.Contains(t, sys, "Inspection report")
}

// TestBuildExtractionPrompt_OmitsSchemaRedundantSections asserts that the
// prompt no longer duplicates content the JSON schema already enforces:
// JSON shape examples, "output ONLY JSON" instructions, the allowed-ops
// table, or full input/output worked examples.
func TestBuildExtractionPrompt_OmitsSchemaRedundantSections(t *testing.T) {
	t.Parallel()
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:    "1",
		Filename: "test.pdf",
		MIME:     "application/pdf",
		Schema: SchemaContext{
			DDL: map[string]string{
				data.TableVendors: "CREATE TABLE `vendors` (`id` integer)",
			},
		},
		Sources: []TextSource{{Tool: "pdftotext", Text: "text"}},
	})

	sys := msgs[0].Content
	assert.NotContains(t, sys, "Output format")
	assert.NotContains(t, sys, "Output ONLY")
	assert.NotContains(t, sys, "Allowed operations")
	assert.NotContains(t, sys, "Worked examples")
	assert.NotContains(t, sys, "code fences")
}

// --- OCR TSV prompt tests ---

// sampleTSV is a minimal tesseract TSV fragment for testing.
const sampleTSV = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
	"1\t1\t1\t1\t1\t1\t100\t200\t50\t20\t95\tInvoice\n" +
	"1\t1\t1\t1\t1\t2\t160\t200\t40\t20\t92\t#1042\n"

func TestBuildExtractionPrompt_SpatialSentWhenEnabled(t *testing.T) {
	t.Parallel()
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:         "1",
		Filename:      "scan.pdf",
		MIME:          "application/pdf",
		SendTSV:       true,
		ConfThreshold: DefaultOCRConfThreshold,
		Sources: []TextSource{
			{Tool: "tesseract", Desc: "OCR text.", Text: "Invoice #1042", Data: []byte(sampleTSV)},
		},
	})

	require.Len(t, msgs, 2)
	user := msgs[1].Content
	// Compact spatial format should appear with bounding boxes.
	assert.Contains(t, user, "[100,200,")
	assert.Contains(t, user, "Invoice #1042")
	// Raw TSV header should NOT appear.
	assert.NotContains(t, user, "level\tpage_num")
	// Confidence 92 is above threshold 70, so no confidence annotation on the data line.
	assert.NotContains(t, user, ";92]")
}

func TestBuildExtractionPrompt_SpatialNotSentByDefault(t *testing.T) {
	t.Parallel()
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:    "1",
		Filename: "scan.pdf",
		MIME:     "application/pdf",
		// SendTSV defaults to false.
		Sources: []TextSource{
			{Tool: "tesseract", Desc: "OCR text.", Text: "Invoice #1042", Data: []byte(sampleTSV)},
		},
	})

	require.Len(t, msgs, 2)
	user := msgs[1].Content
	// Plain text should appear.
	assert.Contains(t, user, "Invoice #1042")
	// No bounding box annotations.
	assert.NotContains(t, user, "[100,200,")
}

func TestBuildExtractionPrompt_SpatialMixedSources(t *testing.T) {
	t.Parallel()
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:         "1",
		Filename:      "mixed.pdf",
		MIME:          "application/pdf",
		SendTSV:       true,
		ConfThreshold: DefaultOCRConfThreshold,
		Sources: []TextSource{
			{Tool: "pdftotext", Desc: "Digital text.", Text: "Digital text from pages 1-2"},
			{Tool: "tesseract", Desc: "OCR text.", Text: "OCR words", Data: []byte(sampleTSV)},
		},
	})

	require.Len(t, msgs, 2)
	user := msgs[1].Content
	// pdftotext source should still use plain text (no TSV data).
	assert.Contains(t, user, "Digital text from pages 1-2")
	// tesseract source should use compact spatial format.
	assert.Contains(t, user, "[100,200,")
	assert.Contains(t, user, "Invoice #1042")
	// The reconstructed plain text for the OCR source should NOT appear.
	assert.NotContains(t, user, "OCR words")
}

func TestBuildExtractionPrompt_TSVColumnHintIncluded(t *testing.T) {
	t.Parallel()
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:         "1",
		Filename:      "scan.pdf",
		MIME:          "application/pdf",
		SendTSV:       true,
		ConfThreshold: DefaultOCRConfThreshold,
		Sources: []TextSource{
			{Tool: "tesseract", Desc: "OCR.", Text: "word", Data: []byte(sampleTSV)},
		},
	})

	require.Len(t, msgs, 2)
	user := msgs[1].Content
	// Should include a hint about how to interpret spatial layout annotations.
	assert.Contains(t, user, "left,top,width")
	assert.Contains(t, user, "spatial layout")
}

func TestBuildExtractionPrompt_TSVSourceWithoutData(t *testing.T) {
	t.Parallel()
	// A tesseract source with no Data field should fall back to plain text
	// even when SendTSV is true.
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:         "1",
		Filename:      "scan.pdf",
		MIME:          "application/pdf",
		SendTSV:       true,
		ConfThreshold: DefaultOCRConfThreshold,
		Sources: []TextSource{
			{Tool: "tesseract", Desc: "OCR.", Text: "Fallback text"},
		},
	})

	require.Len(t, msgs, 2)
	user := msgs[1].Content
	assert.Contains(t, user, "Fallback text")
}

func TestBuildExtractionPrompt_SpatialFallbackOnEmptyTSV(t *testing.T) {
	t.Parallel()
	// TSV with only a header (no data rows) should fall back to plain text.
	headerOnlyTSV := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:         "1",
		Filename:      "scan.pdf",
		MIME:          "application/pdf",
		SendTSV:       true,
		ConfThreshold: DefaultOCRConfThreshold,
		Sources: []TextSource{
			{
				Tool: "tesseract",
				Desc: "OCR.",
				Text: "Fallback plain text",
				Data: []byte(headerOnlyTSV),
			},
		},
	})

	require.Len(t, msgs, 2)
	user := msgs[1].Content
	assert.Contains(t, user, "Fallback plain text",
		"should fall back to plain text when TSV conversion yields empty")
	assert.NotContains(t, user, "[",
		"no bounding boxes when falling back to plain text")
}

func TestBuildExtractionPrompt_TSVPreambleMentionsSpatial(t *testing.T) {
	t.Parallel()
	msgs := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:         "1",
		Filename:      "scan.pdf",
		MIME:          "application/pdf",
		SendTSV:       true,
		ConfThreshold: DefaultOCRConfThreshold,
		Sources: []TextSource{
			{Tool: "tesseract", Desc: "OCR.", Text: "word", Data: []byte(sampleTSV)},
		},
	})

	require.Len(t, msgs, 2)
	sys := msgs[0].Content
	// System prompt should mention spatial layout when TSV is enabled.
	assert.Contains(t, sys, "spatial")
}

func TestBuildExtractionPrompt_ConfThresholdThreaded(t *testing.T) {
	t.Parallel()
	// sampleTSV has confidence 95 and 92. With threshold 96, the line
	// should show confidence (min 92 < 96). With threshold 70, it should not.
	msgsHigh := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:         "1",
		Filename:      "scan.pdf",
		MIME:          "application/pdf",
		SendTSV:       true,
		ConfThreshold: 96,
		Sources: []TextSource{
			{Tool: "tesseract", Desc: "OCR.", Text: "Invoice #1042", Data: []byte(sampleTSV)},
		},
	})
	require.Len(t, msgsHigh, 2)
	assert.Contains(t, msgsHigh[1].Content, ";92]",
		"confidence should appear when min conf (92) < threshold (96)")

	msgsLow := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:         "1",
		Filename:      "scan.pdf",
		MIME:          "application/pdf",
		SendTSV:       true,
		ConfThreshold: 70,
		Sources: []TextSource{
			{Tool: "tesseract", Desc: "OCR.", Text: "Invoice #1042", Data: []byte(sampleTSV)},
		},
	})
	require.Len(t, msgsLow, 2)
	assert.NotContains(t, msgsLow[1].Content, ";92]",
		"confidence should not appear when min conf (92) >= threshold (70)")
}

// --- Schema context formatting tests ---

func TestFormatDDLBlock(t *testing.T) {
	t.Parallel()
	ddl := map[string]string{
		data.TableVendors:   "CREATE TABLE `vendors` (`id` integer, `name` text)",
		data.TableDocuments: "CREATE TABLE `documents` (`id` integer, `title` text)",
	}
	result := FormatDDLBlock(ddl, []string{data.TableVendors, data.TableDocuments})
	assert.Contains(t, result, "CREATE TABLE `vendors`")
	assert.Contains(t, result, "CREATE TABLE `documents`")
}

func TestFormatDDLBlock_MissingTable(t *testing.T) {
	t.Parallel()
	ddl := map[string]string{
		data.TableVendors: "CREATE TABLE `vendors` (`id` integer)",
	}
	result := FormatDDLBlock(ddl, []string{data.TableVendors, "nonexistent"})
	assert.Contains(t, result, data.TableVendors)
	assert.NotContains(t, result, "nonexistent")
}

func TestFormatEntityRows(t *testing.T) {
	t.Parallel()
	rows := []EntityRow{{ID: "1", Name: "Garcia Plumbing"}, {ID: "2", Name: "Acme Electric"}}
	result := FormatEntityRows(data.TableVendors, rows)
	assert.Contains(t, result, "-- vendors (id, name)")
	assert.Contains(t, result, "-- 1, Garcia Plumbing")
	assert.Contains(t, result, "-- 2, Acme Electric")
}

func TestFormatEntityRows_Empty(t *testing.T) {
	t.Parallel()
	result := FormatEntityRows(data.TableVendors, nil)
	assert.Empty(t, result)
}
