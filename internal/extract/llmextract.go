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

// BuildExtractionPrompt creates the system and user messages for document
// extraction. The system prompt defines the JSON schema and rules; the user
// message contains the document metadata and extracted text.
func BuildExtractionPrompt(
	filename string,
	mime string,
	sizeBytes int64,
	entities EntityContext,
	text string,
) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: extractionSystemPrompt(entities)},
		{Role: "user", Content: extractionUserMessage(filename, mime, sizeBytes, text)},
	}
}

func extractionSystemPrompt(entities EntityContext) string {
	var b strings.Builder
	b.WriteString(extractionPreamble)
	b.WriteString("\n\n")
	b.WriteString(extractionSchema)
	b.WriteString("\n\n")
	b.WriteString(extractionRules)

	if len(entities.Vendors) > 0 || len(entities.Projects) > 0 || len(entities.Appliances) > 0 {
		b.WriteString("\n\n## Existing entities in the database\n\n")
		b.WriteString("Match extracted names against these when possible.\n\n")
		if len(entities.Vendors) > 0 {
			b.WriteString("Vendors: ")
			b.WriteString(strings.Join(entities.Vendors, ", "))
			b.WriteString("\n")
		}
		if len(entities.Projects) > 0 {
			b.WriteString("Projects: ")
			b.WriteString(strings.Join(entities.Projects, ", "))
			b.WriteString("\n")
		}
		if len(entities.Appliances) > 0 {
			b.WriteString("Appliances: ")
			b.WriteString(strings.Join(entities.Appliances, ", "))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func extractionUserMessage(filename, mime string, sizeBytes int64, text string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Filename: %s\n", filename))
	b.WriteString(fmt.Sprintf("MIME: %s\n", mime))
	b.WriteString(fmt.Sprintf("Size: %d bytes\n", sizeBytes))
	b.WriteString("\n---\n\n")
	b.WriteString(text)
	return b.String()
}

const extractionPreamble = `You are a document extraction assistant for a home management application. Given a document's metadata and extracted text, return a JSON object with structured fields. Fill only the fields you can confidently extract. Omit or null fields you cannot determine.`

const extractionSchema = `## Output schema

Return ONLY a JSON object with these fields (all optional):

{
  "document_type": "quote|invoice|receipt|manual|warranty|permit|inspection|contract|other",
  "title_suggestion": "short descriptive title for the document",
  "summary": "one-line summary for table display",
  "vendor_hint": "vendor or company name, matched against existing vendors if possible",
  "total_cents": 150000,
  "labor_cents": 80000,
  "materials_cents": 70000,
  "date": "2025-01-15",
  "warranty_expiry": "2027-01-15",
  "entity_kind_hint": "project|appliance|vendor|maintenance|quote|service_log",
  "entity_name_hint": "name of the related entity, matched against existing names if possible",
  "maintenance_items": [
    {"name": "Replace filter", "interval_months": 3}
  ],
  "notes": "anything else worth capturing"
}`

const extractionRules = `## Rules

1. Return ONLY valid JSON. No markdown fences, no commentary, no explanation.
2. All fields are optional. Omit fields you cannot determine. Do not guess.
3. Money values are in CENTS (integer). $1,500.00 = 150000. Never use floats.
4. Dates are ISO 8601: YYYY-MM-DD.
5. For vendor_hint and entity_name_hint, prefer exact matches from the existing entities list.
6. document_type must be one of: quote, invoice, receipt, manual, warranty, permit, inspection, contract, other.
7. entity_kind_hint must be one of: project, appliance, vendor, maintenance, quote, service_log.
8. maintenance_items: extract maintenance schedules from manuals (e.g. "replace filter every 3 months").
9. Keep title_suggestion concise (under 60 characters).
10. Keep summary to one sentence.`

// rawExtractionResponse mirrors the JSON schema but uses flexible types
// for parsing (strings for money/dates that need conversion).
type rawExtractionResponse struct {
	DocumentType   string `json:"document_type"`
	TitleSugg      string `json:"title_suggestion"`
	Summary        string `json:"summary"`
	VendorHint     string `json:"vendor_hint"`
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
	cleaned := stripCodeFences(raw)

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
	if ValidDocumentTypes[resp.DocumentType] {
		hints.DocumentType = resp.DocumentType
	}
	if ValidEntityKindHints[resp.EntityKindHint] {
		hints.EntityKindHint = resp.EntityKindHint
	}

	// Parse money fields.
	hints.TotalCents = parseCents(resp.TotalCents)
	hints.LaborCents = parseCents(resp.LaborCents)
	hints.MaterialsCents = parseCents(resp.MaterialsCents)

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

// stripCodeFences removes markdown code fences that LLMs sometimes wrap
// around JSON output.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		// Remove opening fence.
		if len(lines) > 0 {
			lines = lines[1:]
		}
		// Remove closing fence.
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) == "```" {
				lines = lines[:i]
				break
			}
		}
		s = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return s
}

// parseCents converts a money value from the LLM response to cents.
// Handles: integer (150000), float (1500.00 treated as dollars), and
// string ("$1,500.00", "1500.00", "150000").
func parseCents(v any) *int64 {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case float64:
		// JSON numbers unmarshal as float64. If the value looks like it's
		// already in cents (integer, >= 100), use it directly. If it has
		// a fractional part suggesting dollars, convert.
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
