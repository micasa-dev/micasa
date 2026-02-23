// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

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

You may receive text from multiple extraction sources. Each source is labeled with its tool and a description. When multiple sources are present, prefer digital text extraction for clean output, and use OCR output for scanned content. Reconcile any conflicts by trusting the more plausible reading.`

const operationExtractionRules = `## Output format

Output ONLY a raw JSON array. No code fences, no markdown, no commentary.

Each operation has:
- "action": "create" or "update"
- "table": one of the allowed tables below
- "data": object mapping column names to values

Example (output exactly like this, with NO wrapping):

[
  {"action": "create", "table": "vendors", "data": {"name": "Garcia Plumbing"}},
  {"action": "update", "table": "documents", "data": {"id": 42, "title": "Invoice", "notes": "Repair"}},
  {"action": "create", "table": "quotes", "data": {"total_cents": 150000, "vendor_id": 1}}
]

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
10. For quotes/invoices, create quotes with the correct project_id and vendor_id.
11. Only use "create" and "update". No other actions.

## Allowed operations per table (STRICT -- any violation is rejected)

- documents: create or update. Include "id" in data when updating an existing document.
- vendors: create only.
- quotes: create only.
- maintenance_items: create only.
- appliances: create only.

No other tables may be written to.`

// rawExtractionResponse mirrors the JSON schema but uses flexible types
// for parsing (strings for money/dates that need conversion).
type rawExtractionResponse struct {
	DocumentType   string `json:"document_type"`
	TitleSugg      string `json:"title_suggestion"`
	Summary        string `json:"summary"`
	VendorHint     string `json:"vendor_hint"`
	CurrencyUnit   string `json:"currency_unit"`
	TotalCents     any    `json:"total_cents"`
	LaborCents     any    `json:"labor_cents"`
	MaterialsCents any    `json:"materials_cents"`
	Date           string `json:"date"`
	WarrantyExpiry string `json:"warranty_expiry"`
	EntityKindHint string `json:"entity_kind_hint"`
	EntityNameHint string `json:"entity_name_hint"`
	Maintenance    []struct {
		Name           string `json:"name"`
		IntervalMonths any    `json:"interval_months"`
	} `json:"maintenance_items"`
	Notes string `json:"notes"`
}

// ParseExtractionResponse parses the LLM's JSON response into
// ExtractionHints. Tolerant of markdown fences, partial responses,
// and minor format variations in money/date fields.
func ParseExtractionResponse(raw string) (ExtractionHints, error) {
	cleaned := StripCodeFences(raw)

	var resp rawExtractionResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return ExtractionHints{}, fmt.Errorf("parse extraction json: %w", err)
	}

	hints := ExtractionHints{
		TitleSugg:      resp.TitleSugg,
		Summary:        resp.Summary,
		VendorHint:     resp.VendorHint,
		EntityNameHint: resp.EntityNameHint,
		Notes:          resp.Notes,
	}

	// Validate enums.
	if validDocumentTypes[resp.DocumentType] {
		hints.DocumentType = resp.DocumentType
	}
	if validEntityKindHints[resp.EntityKindHint] {
		hints.EntityKindHint = resp.EntityKindHint
	}

	// Parse money fields. If the model reported currency_unit, use it
	// to resolve the ambiguity between cents and dollars.
	isDollars := strings.EqualFold(resp.CurrencyUnit, "dollars")
	hints.TotalCents = parseCents(resp.TotalCents, isDollars)
	hints.LaborCents = parseCents(resp.LaborCents, isDollars)
	hints.MaterialsCents = parseCents(resp.MaterialsCents, isDollars)

	// Parse date fields.
	hints.Date = parseDate(resp.Date)
	hints.WarrantyExpiry = parseDate(resp.WarrantyExpiry)

	// Parse maintenance items.
	for _, item := range resp.Maintenance {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		months := parsePositiveInt(item.IntervalMonths)
		if months <= 0 {
			continue
		}
		hints.Maintenance = append(hints.Maintenance, MaintenanceHint{
			Name:           name,
			IntervalMonths: months,
		})
	}

	return hints, nil
}

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

// parseCents converts a money value from the LLM response to cents.
// When isDollars is true (model reported currency_unit=dollars), numeric
// values are multiplied by 100. Otherwise numbers are treated as cents.
// Strings with dollar formatting ("$1,500.00") are always converted
// regardless of isDollars.
func parseCents(v any, isDollars bool) *int64 {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case float64:
		if isDollars {
			cents := int64(math.Round(val * 100))
			if cents == 0 {
				return nil
			}
			return &cents
		}
		cents := int64(math.Round(val))
		if cents == 0 {
			return nil
		}
		return &cents
	case string:
		return parseCentsFromString(val)
	default:
		return nil
	}
}

// dollarPattern matches dollar amounts like "$1,234.56" or "1234.56".
var dollarPattern = regexp.MustCompile(`^\$?([\d,]+)\.(\d{2})$`)

// parseCentsFromString parses a money string into cents.
func parseCentsFromString(s string) *int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// Try dollar format: "$1,234.56" or "1,234.56" or "1234.56"
	if m := dollarPattern.FindStringSubmatch(s); m != nil {
		whole := strings.ReplaceAll(m[1], ",", "")
		w, err := strconv.ParseInt(whole, 10, 64)
		if err != nil {
			return nil
		}
		f, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return nil
		}
		cents := w*100 + f
		return &cents
	}

	// Try bare integer (already cents).
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		return &n
	}

	return nil
}

// dateFormats are the date layouts to try when parsing LLM date output.
var dateFormats = []string{
	"2006-01-02",       // ISO 8601
	"01/02/2006",       // US format
	"1/2/2006",         // US format short
	"January 2, 2006",  // long form
	"Jan 2, 2006",      // abbreviated
	"2006-01-02T15:04", // datetime without seconds
}

// parseDate tries multiple date formats and returns the first successful
// parse as a pointer to time.Time, or nil if no format matches.
func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range dateFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// parsePositiveInt extracts a positive integer from a JSON value that
// could be float64 (from JSON number) or string.
func parsePositiveInt(v any) int {
	switch val := v.(type) {
	case float64:
		n := int(math.Round(val))
		if n > 0 {
			return n
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
