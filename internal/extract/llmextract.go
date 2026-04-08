// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"fmt"
	"strings"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/llm"
)

// ExtractionPromptInput holds the inputs for building an extraction prompt.
type ExtractionPromptInput struct {
	DocID         string
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
	b.WriteString(operationExtractionRules)
	return b.String()
}

func operationExtractionUserMessage(in ExtractionPromptInput) string {
	var b strings.Builder
	if in.DocID != "" {
		fmt.Fprintf(&b, "Document ID: %s\n", in.DocID)
	}
	fmt.Fprintf(&b, "Filename: %s\n", in.Filename)
	fmt.Fprintf(&b, "MIME: %s\n", in.MIME)
	fmt.Fprintf(&b, "Size: %d bytes\n", in.SizeBytes)

	for _, src := range in.Sources {
		// When SendTSV is enabled and the source has TSV data, prefer
		// a compact spatial format (line-level bounding boxes). If TSV
		// conversion yields no content (e.g., header-only/invalid TSV),
		// fall back to the reconstructed plain text.
		content := strings.TrimSpace(src.Text)
		hasSpatial := false
		if in.SendTSV && len(src.Data) > 0 {
			spatialContent := strings.TrimSpace(SpatialTextFromTSV(src.Data, in.ConfThreshold))
			if spatialContent != "" {
				content = spatialContent
				hasSpatial = true
			}
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

const operationExtractionPreamble = `You are a document extraction assistant for a home management application. Given a document's metadata and extracted text, output operations to record what the document describes. The output is constrained by a JSON schema -- focus on choosing the right rows and field values, not on the JSON shape.

In this app, "quotes" means contractor or vendor cost estimates (bids for home projects). Create a quotes row only when a document contains such an estimate -- not for incidental dollar amounts in receipts, manuals, or other text.

You may receive text from multiple extraction sources, each labeled with its tool. Sources may overlap; deduplicate facts so each is recorded once. Prefer digital text extraction for clean output and use OCR for scanned content. Reconcile conflicts by trusting the more plausible reading.`

const operationExtractionTSVPreamble = `

OCR sources include spatial layout annotations. Each line is prefixed with a bounding box: [left,top,width]. Use the coordinates to understand document layout -- especially for invoices, forms, and tables where spatial relationships between labels and values matter. When OCR confidence is low, a confidence score is appended: [left,top,width;conf]. Prefer high-confidence readings when reconciling conflicts.`

const spatialFormatHint = "Each line is prefixed with [left,top,width]. Low-confidence lines include [left,top,width;conf]. Use coordinates to infer spatial layout (column alignment, vertical proximity).\n\n"

// operationExtractionRules is the static rules and domain hints section of
// the extraction prompt. The JSON schema enforces output shape, allowed
// tables, allowed actions, column names, and column types; the rules below
// cover only what the schema cannot express (semantic conventions, FK
// resolution against existing rows, and domain mapping hints).
const operationExtractionRules = `## Rules

1. Only set fields you can confidently extract. Do not guess.
2. Money values are integer cents: $1,500.00 -> 150000.
3. Dates are ISO 8601 (YYYY-MM-DD).
4. For foreign keys to existing entities, use real IDs from the existing rows above. To reference an entity you create in the same batch, use the ID it will receive: IDs are assigned sequentially starting at max(existing IDs) + 1 per table.
5. If a vendor, project, appliance, maintenance item, or incident is mentioned but does not exist, create it before referencing it.
6. When a Document ID is provided, update that document; otherwise create one. To link a document to its primary entity, set entity_kind and entity_id.
7. For maintenance schedules from appliance manuals, create maintenance_items linked to the appliance.
8. For contractor or vendor cost estimates (bids, proposals), create quotes with the correct project_id and vendor_id.

## Document type hints

- Contractor invoice or proposal: create the vendor (if new) and project (if new), then a quote with labor_cents, materials_cents, and total_cents.
- Appliance manual: create the appliance with brand and model_number, then one maintenance_items row per scheduled task with interval_months.
- Inspection report: create one incidents row per finding with severity and date_noticed; if the inspector is identifiable, create them as a vendor.`

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
