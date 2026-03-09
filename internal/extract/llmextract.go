// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"fmt"
	"strings"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/llm"
)

// ExtractionPromptInput holds the inputs for building an extraction prompt.
type ExtractionPromptInput struct {
	DocID         uint
	Filename      string
	MIME          string
	SizeBytes     int64
	Schema        SchemaContext
	Sources       []TextSource
	SendTSV       bool // send spatial layout annotations from tesseract OCR
	ConfThreshold int  // confidence threshold for spatial annotations
}

// BuildExtractionPrompt creates the system and user messages for document
// extraction. The system prompt includes the database DDL and existing entity
// rows; the LLM outputs a JSON array of operations.
func BuildExtractionPrompt(in ExtractionPromptInput) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: operationExtractionSystemPrompt(in.Schema, in.SendTSV)},
		{Role: "user", Content: operationExtractionUserMessage(in)},
	}
}

func operationExtractionSystemPrompt(ctx SchemaContext, sendTSV bool) string {
	var b strings.Builder
	b.WriteString(operationExtractionPreamble)
	if sendTSV {
		b.WriteString(operationExtractionTSVPreamble)
	}

	b.WriteString("\n\n## Database schema\n\n")
	b.WriteString(FormatDDLBlock(ctx.DDL, ExtractionTables))

	hasRows := len(ctx.Vendors) > 0 || len(ctx.Projects) > 0 ||
		len(ctx.Appliances) > 0 || len(ctx.MaintenanceItems) > 0 ||
		len(ctx.MaintenanceCategories) > 0 || len(ctx.ProjectTypes) > 0
	if hasRows {
		b.WriteString("\n## Existing rows (use these IDs for foreign keys)\n\n")
		b.WriteString(FormatEntityRows(data.TableVendors, ctx.Vendors))
		b.WriteString(FormatEntityRows(data.TableProjects, ctx.Projects))
		b.WriteString(FormatEntityRows(data.TableAppliances, ctx.Appliances))
		b.WriteString(FormatEntityRows(data.TableMaintenanceItems, ctx.MaintenanceItems))
		b.WriteString(FormatEntityRows(data.TableMaintenanceCategories, ctx.MaintenanceCategories))
		b.WriteString(FormatEntityRows(data.TableProjectTypes, ctx.ProjectTypes))
	}

	b.WriteString("\n")
	b.WriteString(operationExtractionRules())
	b.WriteString("\n\n")
	b.WriteString(operationExtractionExamples)
	return b.String()
}

func operationExtractionUserMessage(in ExtractionPromptInput) string {
	var b strings.Builder
	if in.DocID > 0 {
		fmt.Fprintf(&b, "Document ID: %d\n", in.DocID)
	}
	fmt.Fprintf(&b, "Filename: %s\n", in.Filename)
	fmt.Fprintf(&b, "MIME: %s\n", in.MIME)
	fmt.Fprintf(&b, "Size: %d bytes\n", in.SizeBytes)

	for _, src := range in.Sources {
		// When SendTSV is enabled and the source has TSV data, send
		// a compact spatial format (line-level bounding boxes) instead
		// of reconstructed plain text.
		content := strings.TrimSpace(src.Text)
		hasSpatial := in.SendTSV && len(src.Data) > 0
		if hasSpatial {
			content = strings.TrimSpace(SpatialTextFromTSV(src.Data, in.ConfThreshold))
		}
		if content == "" {
			continue
		}
		fmt.Fprintf(&b, "\n---\n\n## Source: %s\n", src.Tool)
		if src.Desc != "" {
			b.WriteString(src.Desc + "\n\n")
		}
		if hasSpatial {
			b.WriteString(spatialFormatHint)
		}
		b.WriteString(content)
	}

	return b.String()
}

const operationExtractionPreamble = `You are a document extraction assistant for a home management application. Given a document's metadata and extracted text, output a JSON array of operations to store the extracted information in the database.

Note: In this application, "quotes" means contractor/vendor cost estimates (bids for home projects), not quoted text or quotation marks. Create a quotes row when a document contains a cost estimate from a contractor or vendor, but not when dollar amounts appear in other contexts (e.g. receipts, manuals, general text).

You may receive text from multiple extraction sources. Each source is labeled with its tool and a description. Multiple OCR sources may contain overlapping or duplicate text because different extraction methods (digital text extraction, OCR) process the same pages independently. Deduplicate the information: extract each fact once regardless of how many sources mention it. When multiple sources are present, prefer digital text extraction for clean output, and use OCR output for scanned content. Reconcile any conflicts by trusting the more plausible reading.`

const operationExtractionTSVPreamble = `

OCR sources include spatial layout annotations. Each line is prefixed with a bounding box: [left,top,width]. Use the coordinates to understand document layout -- especially for invoices, forms, and tables where spatial relationships between labels and values matter. When OCR confidence is low, a confidence score is appended: [left,top,width;conf]. Prefer high-confidence readings when reconciling conflicts.`

const spatialFormatHint = "Each line is prefixed with [left,top,width]. Low-confidence lines include [left,top,width;conf]. Use coordinates to infer spatial layout (column alignment, vertical proximity).\n\n"

// operationExtractionRules builds the rules and allowed-operations section
// of the extraction prompt dynamically from ExtractionTableDefs so the prompt
// stays in sync with the model definitions.
func operationExtractionRules() string {
	var b strings.Builder
	b.WriteString(operationExtractionRulesStatic)
	b.WriteString("\n\n## Allowed operations\n\n")
	b.WriteString(formatAllowedOps())
	return b.String()
}

// formatAllowedOps generates the "Allowed operations" section from
// ExtractionTableDefs, excluding the documents table (handled separately
// in the static rules).
func formatAllowedOps() string {
	var b strings.Builder
	b.WriteString("Operations array (non-document tables):\n")
	for _, td := range ExtractionTableDefs {
		if td.Table == data.TableDocuments {
			continue
		}
		actions := make([]string, 0, len(td.Actions))
		for _, ad := range td.Actions {
			actions = append(actions, string(ad.Action))
		}
		fmt.Fprintf(&b, "- %s: %s.", td.Table, strings.Join(actions, " or "))
		for _, ad := range td.Actions {
			if ad.Action == ActionUpdate {
				b.WriteString(` Include "id" in data when updating.`)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(`
Document field (separate from operations array):
- create or update. Include "id" in data when updating an existing document.

No other tables may be written to.`)
	return b.String()
}

const operationExtractionRulesStatic = `## Output format

Output ONLY a JSON object. No code fences, no markdown, no commentary.

The object has two top-level fields:
- "operations" (required): array of entity operations. Each element has "action", "table", and "data".
- "document" (optional): a single object for document create/update. Has "action" and "data" only (no "table" -- it is always "documents").

Example:

{"operations": [
  {"action": "create", "table": "vendors", "data": {"name": "Garcia Plumbing"}},
  {"action": "create", "table": "projects", "data": {"title": "Kitchen Renovation"}},
  {"action": "create", "table": "quotes", "data": {"total_cents": 150000, "project_id": 3, "vendor_id": 5}}
], "document": {"action": "update", "data": {"id": 42, "title": "Invoice", "notes": "Repair"}}}

## Rules

1. Output ONLY valid JSON. No code fences, no markdown, no commentary.
2. Only write fields you can confidently extract. Do not guess.
3. Money values MUST be in CENTS (integer). $1,500.00 = 150000.
4. Dates are ISO 8601: YYYY-MM-DD.
5. Use real IDs from the existing rows above for foreign keys to existing entities. When you need to reference an entity you are creating in the same batch, use the ID it will receive: IDs are assigned sequentially starting from max(existing IDs) + 1 per table.
6. If a vendor is mentioned but does not exist, create it in the operations array before referencing it.
7. If a project is mentioned but does not exist, create it. Same for appliances, maintenance items, and incidents.
8. When a Document ID is provided, use "update" in the "document" field and include "id" in data. When no document exists yet, use "create".
9. To link a document to an entity, set "entity_kind" and "entity_id" in the "document" field.
10. For maintenance schedules (from manuals), create maintenance_items.
11. For contractor/vendor cost estimates (bids, proposals), create quotes with the correct project_id and vendor_id. Incidental dollar amounts (e.g. in receipts or manuals) are not quotes.
12. Only use "create" and "update". No other actions.`

const operationExtractionExamples = `## Worked examples

Below are complete input/output examples for representative document types.

### Example 1: Contractor invoice

Input:

Filename: garcia-plumbing-invoice-2024-11.pdf
MIME: application/pdf

---

Source: pdftotext

GARCIA PLUMBING LLC
123 Main St, Springfield IL 62701
Phone: (217) 555-0147

INVOICE #1042
Date: 2024-11-15

Bill To: Jane Homeowner
Project: Master bathroom remodel

Description                     Qty    Rate      Amount
-------------------------------------------------------
Rough-in plumbing labor          16h   $95.00   $1,520.00
PEX tubing 1/2" (100 ft)         1    $89.00      $89.00
SharkBite fittings (assorted)    12     $8.50     $102.00
Drain assembly kit                1    $45.00      $45.00
-------------------------------------------------------
                          Labor:              $1,520.00
                       Materials:                $236.00
                           Total:             $1,756.00

Payment due within 30 days.

Output:

{"operations": [
  {"action": "create", "table": "vendors", "data": {"name": "Garcia Plumbing LLC", "phone": "(217) 555-0147"}},
  {"action": "create", "table": "projects", "data": {"title": "Master bathroom remodel", "status": "underway"}},
  {"action": "create", "table": "quotes", "data": {"project_id": 1, "vendor_id": 1, "total_cents": 175600, "labor_cents": 152000, "materials_cents": 23600, "notes": "Invoice #1042, 2024-11-15. 16h rough-in plumbing, PEX tubing, SharkBite fittings, drain assembly."}}
], "document": {"action": "update", "data": {"id": 42, "title": "Garcia Plumbing invoice #1042", "entity_kind": "quote", "entity_id": 1}}}

### Example 2: Appliance manual

Input:

Filename: bosch-500-dishwasher-manual.pdf
MIME: application/pdf

---

Source: pdftotext

Bosch 500 Series Dishwasher
Model: SHPM65Z55N
Use & Care Manual

MAINTENANCE SCHEDULE

To keep your dishwasher running efficiently, perform the following at the
recommended intervals:

- Clean the filter assembly: every 1 month. Remove the filter at the bottom
  of the tub, rinse under running water, and replace.
- Inspect and clean spray arms: every 6 months. Remove both spray arms and
  clear any debris from the nozzles with a toothpick.
- Run a cleaning cycle: every 3 months. Place a dishwasher-safe cup of white
  vinegar on the top rack and run a hot cycle empty.
- Check the door gasket: every 6 months. Wipe the rubber seal around the door
  with a damp cloth; replace if cracked or worn.

Output:

{"operations": [
  {"action": "create", "table": "appliances", "data": {"name": "Dishwasher", "brand": "Bosch", "model_number": "SHPM65Z55N", "notes": "500 Series"}},
  {"action": "create", "table": "maintenance_items", "data": {"name": "Clean dishwasher filter assembly", "appliance_id": 1, "interval_months": 1, "notes": "Remove filter at bottom of tub, rinse under running water, replace."}},
  {"action": "create", "table": "maintenance_items", "data": {"name": "Inspect and clean dishwasher spray arms", "appliance_id": 1, "interval_months": 6, "notes": "Remove both spray arms, clear debris from nozzles with a toothpick."}},
  {"action": "create", "table": "maintenance_items", "data": {"name": "Run dishwasher cleaning cycle", "appliance_id": 1, "interval_months": 3, "notes": "Place a dishwasher-safe cup of white vinegar on top rack, run hot cycle empty."}},
  {"action": "create", "table": "maintenance_items", "data": {"name": "Check dishwasher door gasket", "appliance_id": 1, "interval_months": 6, "notes": "Wipe rubber seal around door with damp cloth; replace if cracked or worn."}}
], "document": {"action": "update", "data": {"id": 7, "title": "Bosch 500 Series dishwasher manual", "entity_kind": "appliance", "entity_id": 1}}}

### Example 3: Home inspection report

Input:

Filename: annual-inspection-2024.pdf
MIME: application/pdf

---

Source: tesseract

HOME INSPECTION REPORT
Date: 2024-09-20
Inspector: Midwest Home Inspectors

FINDINGS

1. HVAC System (Carrier 24ACC636A003)
   - Condenser coil has visible corrosion on lower fins.
   - Recommend professional cleaning and evaluation within 60 days.

2. Roof
   - Three cracked shingles on south-facing slope near chimney flashing.
   - Minor issue; repair before winter to prevent water intrusion.

3. Water Heater (Rheem PROG50-38N RH67)
   - Anode rod not inspected in over 3 years; likely depleted.
   - Replace anode rod to extend tank life. Estimated cost: $150-200.

Output:

{"operations": [
  {"action": "create", "table": "vendors", "data": {"name": "Midwest Home Inspectors"}},
  {"action": "create", "table": "incidents", "data": {"title": "HVAC condenser coil corrosion", "description": "Condenser coil has visible corrosion on lower fins. Recommend professional cleaning and evaluation within 60 days.", "status": "open", "severity": "soon", "date_noticed": "2024-09-20", "appliance_id": 5}},
  {"action": "create", "table": "incidents", "data": {"title": "Cracked roof shingles near chimney", "description": "Three cracked shingles on south-facing slope near chimney flashing. Repair before winter to prevent water intrusion.", "status": "open", "severity": "soon", "date_noticed": "2024-09-20", "location": "Roof, south-facing slope"}},
  {"action": "create", "table": "incidents", "data": {"title": "Water heater anode rod depleted", "description": "Anode rod not inspected in over 3 years; likely depleted. Replace anode rod to extend tank life.", "status": "open", "severity": "soon", "date_noticed": "2024-09-20", "cost_cents": 17500, "appliance_id": 8}}
], "document": {"action": "update", "data": {"id": 15, "title": "Annual home inspection 2024", "notes": "Inspection by Midwest Home Inspectors, 2024-09-20. Findings: HVAC corrosion, cracked roof shingles, water heater anode rod.", "entity_kind": "vendor", "entity_id": 1}}}`

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
