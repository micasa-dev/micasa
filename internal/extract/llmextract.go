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
	Filename  string
	MIME      string
	SizeBytes int64
	Entities  EntityContext
	Sources   []TextSource
}

// BuildExtractionPrompt creates the system and user messages for document
// extraction. The system prompt defines the JSON schema and rules; the user
// message contains the document metadata and extracted text from all sources.
func BuildExtractionPrompt(in ExtractionPromptInput) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: extractionSystemPrompt(in.Entities)},
		{Role: "user", Content: extractionUserMessage(in)},
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

func extractionUserMessage(in ExtractionPromptInput) string {
	var b strings.Builder
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

const extractionPreamble = `You are a document extraction assistant for a home management application. Given a document's metadata and extracted text, return a JSON object with structured fields. Fill only the fields you can confidently extract. Omit or null fields you cannot determine.

You may receive text from multiple extraction sources. Each source is labeled with its tool and a description. When multiple sources are present, prefer digital text extraction for clean output, and use OCR output for scanned content. Reconcile any conflicts by trusting the more plausible reading.`

const extractionSchema = `## Output schema

Return ONLY a JSON object with these fields (all optional):

{
  "document_type": "quote|invoice|receipt|manual|warranty|permit|inspection|contract|other",
  "title_suggestion": "short descriptive title for the document",
  "summary": "one-line summary for table display",
  "vendor_hint": "vendor or company name, matched against existing vendors if possible",
  "currency_unit": "cents|dollars",
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
3. Money values MUST be in CENTS (integer). $1,500.00 = 150000. Never use floats.
4. Set currency_unit to "cents" when money values are in cents, "dollars" when in dollars.
5. Dates are ISO 8601: YYYY-MM-DD.
6. For vendor_hint and entity_name_hint, prefer exact matches from the existing entities list.
7. document_type must be one of: quote, invoice, receipt, manual, warranty, permit, inspection, contract, other.
8. entity_kind_hint must be one of: project, appliance, vendor, maintenance, quote, service_log.
9. maintenance_items: extract maintenance schedules from manuals (e.g. "replace filter every 3 months").
10. Keep title_suggestion concise (under 60 characters).
11. Keep summary to one sentence.`

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
// around JSON output.
func StripCodeFences(s string) string {
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
