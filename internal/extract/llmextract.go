// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"fmt"
	"strings"

	"github.com/cpcloud/micasa/internal/llm"
)

// ExtractionPromptInput holds the inputs for building an extraction prompt.
type ExtractionPromptInput struct {
	DocID     uint
	Filename  string
	MIME      string
	SizeBytes int64
	Schema    SchemaContext
	Sources   []TextSource
}

// BuildExtractionPrompt creates the system and user messages for document
// extraction. The system prompt includes the database DDL and existing entity
// rows; the LLM outputs a JSON array of operations.
func BuildExtractionPrompt(in ExtractionPromptInput) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: operationExtractionSystemPrompt(in.Schema)},
		{Role: "user", Content: operationExtractionUserMessage(in)},
	}
}

func operationExtractionSystemPrompt(ctx SchemaContext) string {
	var b strings.Builder
	b.WriteString(operationExtractionPreamble)

	b.WriteString("\n\n## Database schema\n\n")
	b.WriteString(FormatDDLBlock(ctx.DDL, ExtractionTables))

	hasRows := len(ctx.Vendors) > 0 || len(ctx.Projects) > 0 ||
		len(ctx.Appliances) > 0 || len(ctx.MaintenanceCategories) > 0 ||
		len(ctx.ProjectTypes) > 0
	if hasRows {
		b.WriteString("\n## Existing rows (use these IDs for foreign keys)\n\n")
		b.WriteString(FormatEntityRows("vendors", ctx.Vendors))
		b.WriteString(FormatEntityRows("projects", ctx.Projects))
		b.WriteString(FormatEntityRows("appliances", ctx.Appliances))
		b.WriteString(FormatEntityRows("maintenance_categories", ctx.MaintenanceCategories))
		b.WriteString(FormatEntityRows("project_types", ctx.ProjectTypes))
	}

	b.WriteString("\n")
	b.WriteString(operationExtractionRules)
	return b.String()
}

func operationExtractionUserMessage(in ExtractionPromptInput) string {
	var b strings.Builder
	if in.DocID > 0 {
		b.WriteString(fmt.Sprintf("Document ID: %d\n", in.DocID))
	}
	b.WriteString(fmt.Sprintf("Filename: %s\n", in.Filename))
	b.WriteString(fmt.Sprintf("MIME: %s\n", in.MIME))
	b.WriteString(fmt.Sprintf("Size: %d bytes\n", in.SizeBytes))

	for _, src := range in.Sources {
		if strings.TrimSpace(src.Text) == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("\n---\n\n## Source: %s\n", src.Tool))
		if src.Desc != "" {
			b.WriteString(src.Desc + "\n\n")
		}
		b.WriteString(src.Text)
	}

	return b.String()
}

const operationExtractionPreamble = `You are a document extraction assistant for a home management application. Given a document's metadata and extracted text, output a JSON array of operations to store the extracted information in the database.

Note: In this application, "quotes" means contractor/vendor cost estimates (bids for home projects), not quoted text or quotation marks. Create a quotes row when a document contains a cost estimate from a contractor or vendor, but not when dollar amounts appear in other contexts (e.g. receipts, manuals, general text).

You may receive text from multiple extraction sources. Each source is labeled with its tool and a description. When multiple sources are present, prefer digital text extraction for clean output, and use OCR output for scanned content. Reconcile any conflicts by trusting the more plausible reading.`

const operationExtractionRules = `## Output format

Output ONLY a JSON object with an "operations" key containing an array. No code fences, no markdown, no commentary.

Each operation has:
- "action": "create" or "update"
- "table": one of the allowed tables below
- "data": object mapping column names to values

Example:

{"operations": [
  {"action": "create", "table": "vendors", "data": {"name": "Garcia Plumbing"}},
  {"action": "update", "table": "documents", "data": {"id": 42, "title": "Invoice", "notes": "Repair"}},
  {"action": "create", "table": "quotes", "data": {"total_cents": 150000, "vendor_id": 1}}
]}

## Rules

1. Output ONLY valid JSON. No code fences, no markdown, no commentary.
2. Only write fields you can confidently extract. Do not guess.
3. Money values MUST be in CENTS (integer). $1,500.00 = 150000.
4. Dates are ISO 8601: YYYY-MM-DD.
5. Use real IDs from the existing rows above for all foreign keys. Do not invent IDs.
6. If a vendor is mentioned but does not exist, create it.
7. When a Document ID is provided, use "update" for that document and include "id" in data. When no document exists yet, use "create".
8. To link a document to an entity, set "entity_kind" and "entity_id" in the document operation.
9. For maintenance schedules (from manuals), create maintenance_items.
10. For contractor/vendor cost estimates (bids, proposals), create quotes with the correct project_id and vendor_id. Incidental dollar amounts (e.g. in receipts or manuals) are not quotes.
11. Only use "create" and "update". No other actions.

## Allowed operations per table (STRICT -- any violation is rejected)

- documents: create or update. Include "id" in data when updating an existing document.
- vendors: create only.
- quotes: create only.
- maintenance_items: create only.
- appliances: create only.

No other tables may be written to.`

// StripCodeFences removes markdown code fences that LLMs sometimes wrap
// around JSON output. Handles fences anywhere in the text (not just at
// the start), since LLMs may produce commentary before the fenced block.
func StripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")

	// Find the opening fence (``` or ```json etc.) anywhere in the text.
	fenceStart := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenceStart = i
			break
		}
	}
	if fenceStart < 0 {
		return s
	}

	// Find the closing fence after the opening one.
	fenceEnd := -1
	for i := len(lines) - 1; i > fenceStart; i-- {
		if strings.TrimSpace(lines[i]) == "```" {
			fenceEnd = i
			break
		}
	}
	if fenceEnd < 0 {
		// Opening fence but no closing fence: strip the opening and return rest.
		return strings.TrimSpace(strings.Join(lines[fenceStart+1:], "\n"))
	}

	return strings.TrimSpace(strings.Join(lines[fenceStart+1:fenceEnd], "\n"))
}
