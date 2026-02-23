// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/extract"
	"github.com/cpcloud/micasa/internal/llm"
)

// --- Extraction step types ---

type extractionStep int

const (
	stepText extractionStep = iota
	stepExtract
	stepLLM
	numExtractionSteps
)

const tableDocuments = "documents"

type stepStatus int

const (
	stepPending stepStatus = iota
	stepRunning
	stepDone
	stepFailed
)

// extractionStepInfo tracks the state of a single extraction step.
type extractionStepInfo struct {
	Status  stepStatus
	Detail  string // tool/model identifier (e.g. "pdf", "qwen3:0.6b")
	Metric  string // measurement (e.g. "68 chars")
	Logs    []string
	Elapsed time.Duration
	Started time.Time
}

// extractionLogState holds the state of the extraction progress overlay.
type extractionLogState struct {
	DocID    uint
	Filename string
	Steps    [numExtractionSteps]extractionStepInfo
	Spinner  spinner.Model
	Viewport viewport.Model
	Visible  bool
	Done     bool
	HasError bool
	ctx      context.Context
	CancelFn context.CancelFunc

	// Text sources accumulated during extraction, passed to LLM prompt.
	sources       []extract.TextSource
	extractedText string // best available text for DB storage/display

	// Async extraction results pending persistence (nil until produced).
	pendingText string // text from async extraction
	pendingData []byte // structured data from async extraction

	// LLM token accumulator for JSON parsing on completion.
	llmAccum strings.Builder

	// Carried between steps.
	fileData   []byte
	mime       string
	extractors []extract.Extractor

	// Channel references for the waitFor loop pattern.
	extractCh <-chan extract.ExtractProgress
	llmCh     <-chan llm.StreamChunk

	markdownRenderer

	// Which steps are active (skipped steps are simply not shown).
	hasText    bool
	hasExtract bool
	hasLLM     bool

	// Pending results held until user accepts.
	operations []extract.Operation // validated operations (not yet executed)
	accepted   bool                // true once user accepted results
	pendingDoc *data.Document      // deferred creation: unpersisted document (magic-add)

	// Cursor and expand/collapse state for exploring output.
	cursor   int                     // index into activeSteps()
	expanded map[extractionStep]bool // manual expand/collapse overrides

	// Explore mode: read-only table navigation for proposed operations.
	exploring     bool                // true when in table explore mode
	previewGroups []previewTableGroup // cached grouped operations
	previewTab    int                 // active tab in explore mode
	previewRow    int                 // row cursor within active tab
	previewCol    int                 // column cursor within active tab
}

// activeSteps returns the ordered list of steps that are shown.
func (ex *extractionLogState) activeSteps() []extractionStep {
	var steps []extractionStep
	if ex.hasText {
		steps = append(steps, stepText)
	}
	if ex.hasExtract {
		steps = append(steps, stepExtract)
	}
	if ex.hasLLM {
		steps = append(steps, stepLLM)
	}
	return steps
}

// cursorStep returns the step at the current cursor position.
func (ex *extractionLogState) cursorStep() extractionStep {
	active := ex.activeSteps()
	if ex.cursor >= 0 && ex.cursor < len(active) {
		return active[ex.cursor]
	}
	return stepText
}

// cursorExpanded returns true when the focused step is expanded and the
// viewport content overflows. Used to decide whether j/k scroll the viewport
// instead of moving between steps.
func (ex *extractionLogState) cursorExpanded() bool {
	si := ex.cursorStep()
	info := ex.Steps[si]
	expanded := info.Status == stepRunning || info.Status == stepFailed ||
		(si == stepLLM && info.Status == stepDone)
	if toggled, ok := ex.expanded[si]; ok {
		expanded = toggled
	}
	return expanded && ex.Viewport.TotalLineCount() > ex.Viewport.Height
}

// advanceCursor moves the cursor to the latest settled (done/failed) step.
func (ex *extractionLogState) advanceCursor() {
	active := ex.activeSteps()
	for i := len(active) - 1; i >= 0; i-- {
		s := ex.Steps[active[i]].Status
		if s == stepDone || s == stepFailed {
			ex.cursor = i
			return
		}
	}
}

// --- Messages ---

// extractionProgressMsg delivers a single async extraction progress update.
type extractionProgressMsg struct {
	Progress extract.ExtractProgress
}

// extractionLLMStartedMsg delivers the LLM stream channel.
type extractionLLMStartedMsg struct {
	Ch <-chan llm.StreamChunk
}

// extractionLLMChunkMsg delivers a single LLM token.
type extractionLLMChunkMsg struct {
	Content string
	Done    bool
	Err     error
}

// --- Overlay lifecycle ---

// startExtractionOverlay opens the extraction progress overlay and kicks off
// the first applicable step. Returns nil if no async steps are needed.
func (m *Model) startExtractionOverlay(
	docID uint,
	filename string,
	fileData []byte,
	mime string,
	extractedText string,
) tea.Cmd {
	needsExtract := extract.NeedsOCR(m.extractors, mime)
	needsLLM := m.extractionLLMClient() != nil

	if !needsExtract && !needsLLM {
		return nil
	}

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(accent)

	ctx, cancel := context.WithCancel(context.Background())

	// Text extraction only applies to PDFs and text files; skip for images.
	hasText := !extract.IsImageMIME(mime)

	// Build initial text source from already-extracted text.
	var sources []extract.TextSource
	if hasText && strings.TrimSpace(extractedText) != "" {
		var tool, desc string
		switch {
		case mime == extract.MIMEApplicationPDF:
			tool = "pdftotext"
			desc = "Digital text extracted directly from the PDF."
		case strings.HasPrefix(mime, "text/"):
			tool = "plaintext"
			desc = "Plain text content."
		default:
			tool = mime
		}
		sources = append(sources, extract.TextSource{
			Tool: tool,
			Desc: desc,
			Text: extractedText,
		})
	}

	state := &extractionLogState{
		DocID:         docID,
		Filename:      filename,
		Spinner:       sp,
		Visible:       true,
		ctx:           ctx,
		CancelFn:      cancel,
		sources:       sources,
		extractedText: extractedText,
		fileData:      fileData,
		mime:          mime,
		extractors:    m.extractors,
		hasText:       hasText,
		hasExtract:    needsExtract,
		hasLLM:        needsLLM,
		expanded:      make(map[extractionStep]bool),
	}
	if hasText {
		nChars := len(strings.TrimSpace(extractedText))
		var textTool string
		switch {
		case mime == extract.MIMEApplicationPDF:
			textTool = "pdf"
		case strings.HasPrefix(mime, "text/"):
			textTool = "plaintext"
		default:
			textTool = mime
		}
		textStep := extractionStepInfo{
			Status: stepDone,
			Detail: textTool,
			Metric: fmt.Sprintf("%d chars", nChars),
		}
		if nChars > 0 {
			textStep.Logs = strings.Split(extractedText, "\n")
		}
		state.Steps[stepText] = textStep
	}

	// Replace any existing extraction state.
	if m.extraction != nil && m.extraction.CancelFn != nil {
		m.extraction.CancelFn()
	}
	m.extraction = state

	var cmd tea.Cmd
	if needsExtract {
		state.Steps[stepExtract].Status = stepRunning
		state.Steps[stepExtract].Started = time.Now()
		cmd = asyncExtractCmd(ctx, state)
	} else if needsLLM {
		state.Steps[stepLLM].Status = stepRunning
		state.Steps[stepLLM].Started = time.Now()
		state.Steps[stepLLM].Detail = m.extractionModelLabel()
		cmd = m.llmExtractCmd(ctx)
	}

	return tea.Batch(cmd, state.Spinner.Tick)
}

// cancelExtraction cancels any in-flight extraction and clears state.
func (m *Model) cancelExtraction() {
	if m.extraction == nil {
		return
	}
	if m.extraction.CancelFn != nil {
		m.extraction.CancelFn()
	}
	m.extraction = nil
}

// --- Async commands ---

// asyncExtractCmd starts the async extraction pipeline and returns the
// first progress message via waitForExtractProgress.
func asyncExtractCmd(ctx context.Context, state *extractionLogState) tea.Cmd {
	ch := extract.ExtractWithProgress(
		ctx, state.fileData, state.mime, state.extractors,
	)
	state.extractCh = ch
	return waitForExtractProgress(ch)
}

// waitForExtractProgress blocks until the next extraction progress update.
func waitForExtractProgress(ch <-chan extract.ExtractProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return extractionProgressMsg{
				Progress: extract.ExtractProgress{Done: true},
			}
		}
		return extractionProgressMsg{Progress: p}
	}
}

// llmExtractCmd starts LLM document analysis with streaming.
func (m *Model) llmExtractCmd(ctx context.Context) tea.Cmd {
	client := m.extractionLLMClient()
	if client == nil {
		return nil
	}
	schemaCtx := m.buildSchemaContext()
	ex := m.extraction
	return func() tea.Msg {
		messages := extract.BuildExtractionPrompt(extract.ExtractionPromptInput{
			DocID:     ex.DocID,
			Filename:  ex.Filename,
			MIME:      ex.mime,
			SizeBytes: int64(len(ex.fileData)),
			Schema:    schemaCtx,
			Sources:   ex.sources,
		})
		ch, err := client.ChatStream(ctx, messages)
		if err != nil {
			return extractionLLMChunkMsg{Err: err, Done: true}
		}
		return extractionLLMStartedMsg{Ch: ch}
	}
}

// buildSchemaContext gathers DDL and entity rows for the extraction prompt.
func (m *Model) buildSchemaContext() extract.SchemaContext {
	var ctx extract.SchemaContext
	if m.store == nil {
		return ctx
	}
	ddl, err := m.store.TableDDL(extract.ExtractionTables...)
	if err == nil {
		ctx.DDL = ddl
	}
	rows, err := m.store.EntityRows()
	if err == nil {
		ctx.Vendors = toExtractRows(rows.Vendors)
		ctx.Projects = toExtractRows(rows.Projects)
		ctx.Appliances = toExtractRows(rows.Appliances)
		ctx.MaintenanceCategories = toExtractRows(rows.MaintenanceCategories)
		ctx.ProjectTypes = toExtractRows(rows.ProjectTypes)
	}
	return ctx
}

// toExtractRows converts data.EntityRow slices to extract.EntityRow slices.
func toExtractRows(rows []data.EntityRow) []extract.EntityRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]extract.EntityRow, len(rows))
	for i, r := range rows {
		out[i] = extract.EntityRow{ID: r.ID, Name: r.Name}
	}
	return out
}

// waitForLLMChunk blocks until the next LLM token.
func waitForLLMChunk(ch <-chan llm.StreamChunk) tea.Cmd {
	return func() tea.Msg {
		chunk, ok := <-ch
		if !ok {
			return extractionLLMChunkMsg{Done: true}
		}
		return extractionLLMChunkMsg{
			Content: chunk.Content,
			Done:    chunk.Done,
			Err:     chunk.Err,
		}
	}
}

// --- Message handlers ---

// handleExtractionProgress processes an async extraction progress update.
func (m *Model) handleExtractionProgress(msg extractionProgressMsg) tea.Cmd {
	ex := m.extraction
	if ex == nil {
		return nil
	}

	p := msg.Progress
	step := &ex.Steps[stepExtract]

	if p.Err != nil {
		step.Status = stepFailed
		step.Elapsed = time.Since(step.Started)
		step.Logs = append(step.Logs, p.Err.Error())
		ex.HasError = true
		ex.advanceCursor()
		// Extraction failed but LLM can still run on whatever text exists.
		if ex.hasLLM {
			client := m.extractionLLMClient()
			if client != nil {
				ex.Steps[stepLLM].Status = stepRunning
				ex.Steps[stepLLM].Started = time.Now()
				ex.Steps[stepLLM].Detail = m.extractionModelLabel()
				return m.llmExtractCmd(ex.ctx)
			}
		}
		ex.Done = true
		return nil
	}

	if !p.Done {
		switch p.Phase {
		case "rasterize":
			step.Detail = fmt.Sprintf("rasterizing %d/%d", p.Page, p.Total)
		case "images":
			step.Detail = fmt.Sprintf("%d images", p.Total)
		case "extract":
			step.Detail = fmt.Sprintf("page %d/%d", p.Page, p.Total)
		}
		return waitForExtractProgress(ex.extractCh)
	}

	// Extraction done.
	step.Status = stepDone
	step.Elapsed = time.Since(step.Started)
	nChars := len(strings.TrimSpace(p.Text))
	step.Detail = p.Tool
	step.Metric = fmt.Sprintf("%d chars", nChars)
	ex.advanceCursor()

	// Store output as explorable logs.
	if nChars > 0 {
		step.Logs = strings.Split(p.Text, "\n")
	}

	// Add to LLM sources (prompt builder skips empty text).
	ex.sources = append(ex.sources, extract.TextSource{
		Tool: p.Tool,
		Desc: p.Desc,
		Text: p.Text,
		Data: p.Data,
	})

	// Hold for persistence at accept time.
	ex.pendingText = p.Text
	ex.pendingData = p.Data

	// If no text was extracted synchronously, use async result.
	if nChars > 0 && ex.extractedText == "" {
		ex.extractedText = p.Text
	}

	// Advance to LLM step if configured.
	if ex.hasLLM {
		client := m.extractionLLMClient()
		if client != nil {
			ex.Steps[stepLLM].Status = stepRunning
			ex.Steps[stepLLM].Started = time.Now()
			ex.Steps[stepLLM].Detail = m.extractionModelLabel()
			return m.llmExtractCmd(ex.ctx)
		}
	}

	ex.Done = true
	return nil
}

// handleExtractionLLMStarted stores the LLM stream channel and starts reading.
func (m *Model) handleExtractionLLMStarted(msg extractionLLMStartedMsg) tea.Cmd {
	if m.extraction == nil {
		return nil
	}
	m.extraction.llmCh = msg.Ch
	return waitForLLMChunk(msg.Ch)
}

// handleExtractionLLMChunk processes a single LLM token.
func (m *Model) handleExtractionLLMChunk(msg extractionLLMChunkMsg) tea.Cmd {
	ex := m.extraction
	if ex == nil {
		return nil
	}

	step := &ex.Steps[stepLLM]

	if msg.Err != nil {
		step.Status = stepFailed
		step.Elapsed = time.Since(step.Started)
		step.Logs = append(step.Logs, msg.Err.Error())
		ex.HasError = true
		ex.Done = true
		ex.advanceCursor()
		return nil
	}

	if msg.Content != "" {
		ex.llmAccum.WriteString(msg.Content)
		step.Logs = strings.Split(ex.llmAccum.String(), "\n")
	}

	if msg.Done {
		step.Elapsed = time.Since(step.Started)

		// Parse and validate operations; hold for accept.
		response := ex.llmAccum.String()
		ops, err := extract.ParseOperations(response)
		if err != nil {
			step.Status = stepFailed
			step.Logs = append(step.Logs, "parse error: "+err.Error())
			ex.HasError = true
		} else if err := extract.ValidateOperations(ops, extract.ExtractionAllowedOps); err != nil {
			step.Status = stepFailed
			step.Logs = append(step.Logs, "validation error: "+err.Error())
			ex.HasError = true
		} else {
			step.Status = stepDone
			ex.operations = ops
		}
		step.Metric = fmt.Sprintf("%d ops", len(ex.operations))

		ex.Done = true
		ex.advanceCursor()
		return nil
	}

	// More tokens coming.
	return waitForLLMChunk(ex.llmCh)
}

// dispatchOperations executes validated operations through the Store API.
func (m *Model) dispatchOperations(ops []extract.Operation) error {
	if m.store == nil || len(ops) == 0 {
		return nil
	}
	for _, op := range ops {
		if err := m.dispatchOneOperation(op); err != nil {
			return fmt.Errorf("%s %s: %w", op.Action, op.Table, err)
		}
	}
	m.reloadAfterMutation()
	return nil
}

// dispatchOneOperation routes a single operation to the appropriate Store method.
func (m *Model) dispatchOneOperation(op extract.Operation) error {
	switch {
	case op.Action == extract.ActionCreate && op.Table == tableDocuments:
		return m.dispatchCreateDocument(op)
	case op.Action == extract.ActionUpdate && op.Table == tableDocuments:
		return m.dispatchUpdateDocument(op)
	case op.Action == extract.ActionCreate && op.Table == "vendors":
		return m.dispatchCreateVendor(op)
	case op.Action == extract.ActionCreate && op.Table == "quotes":
		return m.dispatchCreateQuote(op)
	case op.Action == extract.ActionCreate && op.Table == "maintenance_items":
		return m.dispatchCreateMaintenance(op)
	case op.Action == extract.ActionCreate && op.Table == "appliances":
		return m.dispatchCreateAppliance(op)
	default:
		return fmt.Errorf("unsupported operation: %s on %s", op.Action, op.Table)
	}
}

func (m *Model) dispatchCreateDocument(op extract.Operation) error {
	doc := data.Document{}
	applyStringField(op.Data, "title", &doc.Title)
	applyStringField(op.Data, "file_name", &doc.FileName)
	applyStringField(op.Data, "notes", &doc.Notes)
	applyStringField(op.Data, "entity_kind", &doc.EntityKind)
	if v, ok := op.Data["entity_id"]; ok {
		if n := parseUintFromData(v); n > 0 {
			doc.EntityID = n
		}
	}
	return m.store.CreateDocument(&doc)
}

func (m *Model) dispatchUpdateDocument(op extract.Operation) error {
	rowID := parseUintFromData(op.Data["id"])
	if rowID == 0 {
		return fmt.Errorf("update documents requires id in data")
	}
	doc, err := m.store.GetDocument(rowID)
	if err != nil {
		return fmt.Errorf("get document %d: %w", rowID, err)
	}
	applyStringField(op.Data, "title", &doc.Title)
	applyStringField(op.Data, "notes", &doc.Notes)
	applyStringField(op.Data, "entity_kind", &doc.EntityKind)
	if v, ok := op.Data["entity_id"]; ok {
		if n := parseUintFromData(v); n > 0 {
			doc.EntityID = n
		}
	}
	return m.store.UpdateDocument(doc)
}

func (m *Model) dispatchCreateVendor(op extract.Operation) error {
	vendor := data.Vendor{}
	applyStringField(op.Data, "name", &vendor.Name)
	if strings.TrimSpace(vendor.Name) == "" {
		return fmt.Errorf("vendor name is required")
	}
	applyStringField(op.Data, "contact_name", &vendor.ContactName)
	applyStringField(op.Data, "email", &vendor.Email)
	applyStringField(op.Data, "phone", &vendor.Phone)
	applyStringField(op.Data, "website", &vendor.Website)
	applyStringField(op.Data, "notes", &vendor.Notes)
	return m.store.CreateVendor(&vendor)
}

func (m *Model) dispatchCreateQuote(op extract.Operation) error {
	quote := data.Quote{}
	if v, ok := op.Data["project_id"]; ok {
		if n := parseUintFromData(v); n > 0 {
			quote.ProjectID = n
		}
	}
	if v, ok := op.Data["total_cents"]; ok {
		quote.TotalCents = parseInt64FromData(v)
	}
	if v, ok := op.Data["labor_cents"]; ok {
		n := parseInt64FromData(v)
		quote.LaborCents = &n
	}
	if v, ok := op.Data["materials_cents"]; ok {
		n := parseInt64FromData(v)
		quote.MaterialsCents = &n
	}
	applyStringField(op.Data, "notes", &quote.Notes)

	// Resolve vendor: by vendor_id or inline vendor name.
	var vendor data.Vendor
	if v, ok := op.Data["vendor_id"]; ok {
		if n := parseUintFromData(v); n > 0 {
			got, err := m.store.GetVendor(n)
			if err != nil {
				return fmt.Errorf("get vendor %d: %w", n, err)
			}
			vendor = got
		}
	}
	if vendor.ID == 0 {
		// No valid vendor_id; try vendor_name for find-or-create.
		var vendorName string
		applyStringField(op.Data, "vendor_name", &vendorName)
		if vendorName != "" {
			vendor.Name = vendorName
		}
	}

	return m.store.CreateQuote(&quote, vendor)
}

func (m *Model) dispatchCreateMaintenance(op extract.Operation) error {
	item := data.MaintenanceItem{}
	applyStringField(op.Data, "name", &item.Name)
	if v, ok := op.Data["category_id"]; ok {
		if n := parseUintFromData(v); n > 0 {
			item.CategoryID = n
		}
	}
	if v, ok := op.Data["appliance_id"]; ok {
		if n := parseUintFromData(v); n > 0 {
			item.ApplianceID = &n
		}
	}
	if v, ok := op.Data["interval_months"]; ok {
		item.IntervalMonths = int(parseInt64FromData(v))
	}
	applyStringField(op.Data, "notes", &item.Notes)
	if v, ok := op.Data["cost_cents"]; ok {
		n := parseInt64FromData(v)
		item.CostCents = &n
	}
	return m.store.CreateMaintenance(&item)
}

func (m *Model) dispatchCreateAppliance(op extract.Operation) error {
	item := data.Appliance{}
	applyStringField(op.Data, "name", &item.Name)
	applyStringField(op.Data, "brand", &item.Brand)
	applyStringField(op.Data, "model_number", &item.ModelNumber)
	applyStringField(op.Data, "serial_number", &item.SerialNumber)
	applyStringField(op.Data, "location", &item.Location)
	applyStringField(op.Data, "notes", &item.Notes)
	if v, ok := op.Data["cost_cents"]; ok {
		n := parseInt64FromData(v)
		item.CostCents = &n
	}
	return m.store.CreateAppliance(&item)
}

// applyStringField sets *dst to the string value at data[key] if present.
func applyStringField(data map[string]any, key string, dst *string) {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			*dst = s
		}
	}
}

// parseUintFromData extracts a uint from a JSON value (float64 or string).
func parseUintFromData(v any) uint {
	switch val := v.(type) {
	case float64:
		if val > 0 {
			return uint(val)
		}
	case string:
		if n, err := strconv.ParseUint(strings.TrimSpace(val), 10, 64); err == nil {
			return uint(n)
		}
	}
	return 0
}

// parseInt64FromData extracts an int64 from a JSON value.
func parseInt64FromData(v any) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// acceptExtraction persists all pending results and closes the overlay.
func (m *Model) acceptExtraction() {
	ex := m.extraction
	if ex == nil || !ex.Done || ex.accepted {
		return
	}

	if ex.pendingDoc != nil {
		// Deferred creation (magic-add): create document now.
		if err := m.acceptDeferredExtraction(); err != nil {
			m.setStatusError(err.Error())
			return
		}
	} else {
		// Existing document: persist extraction results and dispatch ops.
		if err := m.acceptExistingExtraction(); err != nil {
			m.setStatusError(err.Error())
			return
		}
	}

	ex.accepted = true
	m.extraction = nil
}

// acceptDeferredExtraction creates the deferred document, applying any
// LLM-produced document fields, then dispatches remaining operations.
func (m *Model) acceptDeferredExtraction() error {
	ex := m.extraction
	doc := ex.pendingDoc

	// Apply fields from "create documents" operations to the pending doc.
	for _, op := range ex.operations {
		if op.Table == tableDocuments {
			applyStringField(op.Data, "title", &doc.Title)
			applyStringField(op.Data, "notes", &doc.Notes)
			applyStringField(op.Data, "entity_kind", &doc.EntityKind)
			if v, ok := op.Data["entity_id"]; ok {
				if n := parseUintFromData(v); n > 0 {
					doc.EntityID = n
				}
			}
		}
	}

	// Apply async extraction results to the document before creating.
	if ex.pendingText != "" {
		doc.ExtractedText = ex.pendingText
	}
	if len(ex.pendingData) > 0 {
		doc.ExtractData = ex.pendingData
	}

	if err := m.store.CreateDocument(doc); err != nil {
		return fmt.Errorf("create document: %w", err)
	}

	// Dispatch non-document operations (vendors, quotes, etc.).
	var nonDocOps []extract.Operation
	for _, op := range ex.operations {
		if op.Table != tableDocuments {
			nonDocOps = append(nonDocOps, op)
		}
	}
	if len(nonDocOps) > 0 {
		if err := m.dispatchOperations(nonDocOps); err != nil {
			return fmt.Errorf("dispatch operations: %w", err)
		}
	} else {
		m.reloadAfterMutation()
	}
	return nil
}

// acceptExistingExtraction persists extraction text and dispatches operations
// for an already-saved document.
func (m *Model) acceptExistingExtraction() error {
	ex := m.extraction

	// Persist async extraction results.
	if ex.pendingText != "" || len(ex.pendingData) > 0 {
		if m.store != nil {
			if err := m.store.UpdateDocumentExtraction(
				ex.DocID, ex.pendingText, ex.pendingData,
			); err != nil {
				return fmt.Errorf("save extraction: %w", err)
			}
		}
	}

	// Execute validated operations via Store API.
	if len(ex.operations) > 0 {
		if err := m.dispatchOperations(ex.operations); err != nil {
			return fmt.Errorf("dispatch operations: %w", err)
		}
	}
	return nil
}

// rerunLLMExtraction resets the LLM step and re-runs it.
func (m *Model) rerunLLMExtraction() tea.Cmd {
	ex := m.extraction
	if ex == nil || !ex.hasLLM {
		return nil
	}

	// Reset LLM state.
	ex.llmAccum.Reset()
	ex.operations = nil
	ex.previewGroups = nil
	ex.exploring = false
	ex.Steps[stepLLM] = extractionStepInfo{
		Status:  stepRunning,
		Started: time.Now(),
		Detail:  m.extractionModelLabel(),
	}
	ex.Done = false
	ex.HasError = false
	delete(ex.expanded, stepLLM)

	// Re-check other steps for errors (they stay as-is).
	for _, si := range ex.activeSteps() {
		if si != stepLLM && ex.Steps[si].Status == stepFailed {
			ex.HasError = true
		}
	}

	// Move cursor to the nearest earlier settled step so it stays visible
	// while LLM re-runs (running steps hide the triangle indicator).
	ex.cursor = 0
	active := ex.activeSteps()
	for i := len(active) - 1; i >= 0; i-- {
		s := ex.Steps[active[i]].Status
		if s == stepDone || s == stepFailed {
			ex.cursor = i
			break
		}
	}

	return tea.Batch(m.llmExtractCmd(ex.ctx), ex.Spinner.Tick)
}

// --- Keyboard handler ---

// handleExtractionKey processes keys when the extraction overlay is visible.
func (m *Model) handleExtractionKey(msg tea.KeyMsg) tea.Cmd {
	ex := m.extraction
	if ex.exploring {
		return m.handleExtractionExploreKey(msg)
	}
	return m.handleExtractionPipelineKey(msg)
}

// handleExtractionPipelineKey handles keys in pipeline navigation mode.
func (m *Model) handleExtractionPipelineKey(msg tea.KeyMsg) tea.Cmd {
	ex := m.extraction
	switch msg.String() {
	case keyEsc:
		m.cancelExtraction()
	case "j", keyDown:
		if ex.cursorExpanded() && !ex.Viewport.AtBottom() {
			vp, cmd := ex.Viewport.Update(msg)
			ex.Viewport = vp
			return cmd
		}
		active := ex.activeSteps()
		for next := ex.cursor + 1; next < len(active); next++ {
			s := ex.Steps[active[next]].Status
			if ex.Done || s == stepDone || s == stepFailed {
				ex.cursor = next
				break
			}
		}
	case "k", "up":
		if ex.cursorExpanded() && !ex.Viewport.AtTop() {
			vp, cmd := ex.Viewport.Update(msg)
			ex.Viewport = vp
			return cmd
		}
		active := ex.activeSteps()
		for prev := ex.cursor - 1; prev >= 0; prev-- {
			s := ex.Steps[active[prev]].Status
			if ex.Done || s == stepDone || s == stepFailed {
				ex.cursor = prev
				break
			}
		}
	case keyEnter:
		si := ex.cursorStep()
		status := ex.Steps[si].Status
		if status == stepDone || status == stepFailed {
			effective := status == stepFailed ||
				(si == stepLLM && status == stepDone)
			if toggled, ok := ex.expanded[si]; ok {
				effective = toggled
			}
			ex.expanded[si] = !effective
		}
	case "r":
		if ex.Done && ex.hasLLM && ex.cursorStep() == stepLLM {
			return m.rerunLLMExtraction()
		}
	case "a":
		if ex.Done && !ex.HasError {
			m.acceptExtraction()
		}
	case "x":
		if ex.Done && len(ex.operations) > 0 {
			ex.enterExploreMode()
		}
	default:
		vp, cmd := ex.Viewport.Update(msg)
		ex.Viewport = vp
		return cmd
	}
	return nil
}

// handleExtractionExploreKey handles keys in table explore mode.
func (m *Model) handleExtractionExploreKey(msg tea.KeyMsg) tea.Cmd {
	ex := m.extraction
	switch msg.String() {
	case keyEsc:
		ex.exploring = false
	case "j", keyDown:
		g := ex.activePreviewGroup()
		if g != nil && ex.previewRow < len(g.cells)-1 {
			ex.previewRow++
		}
	case "k", "up":
		if ex.previewRow > 0 {
			ex.previewRow--
		}
	case "h", "left":
		g := ex.activePreviewGroup()
		if g != nil && ex.previewCol > 0 {
			ex.previewCol--
		}
	case "l", "right":
		g := ex.activePreviewGroup()
		if g != nil && ex.previewCol < len(g.specs)-1 {
			ex.previewCol++
		}
	case "b":
		if ex.previewTab > 0 {
			ex.previewTab--
			ex.previewRow = 0
			ex.previewCol = 0
		}
	case "f":
		if ex.previewTab < len(ex.previewGroups)-1 {
			ex.previewTab++
			ex.previewRow = 0
			ex.previewCol = 0
		}
	case "g":
		ex.previewRow = 0
	case "G":
		g := ex.activePreviewGroup()
		if g != nil && len(g.cells) > 0 {
			ex.previewRow = len(g.cells) - 1
		}
	case "^":
		ex.previewCol = 0
	case "$":
		g := ex.activePreviewGroup()
		if g != nil && len(g.specs) > 0 {
			ex.previewCol = len(g.specs) - 1
		}
	case "a":
		if ex.Done && !ex.HasError {
			m.acceptExtraction()
		}
	case "x":
		ex.exploring = false
	}
	return nil
}

// enterExploreMode switches to table explore mode, caching operation groups.
func (ex *extractionLogState) enterExploreMode() {
	if len(ex.previewGroups) == 0 {
		ex.previewGroups = groupOperationsByTable(ex.operations)
	}
	if len(ex.previewGroups) == 0 {
		return
	}
	ex.exploring = true
	// Clamp cursors to valid bounds.
	if ex.previewTab >= len(ex.previewGroups) {
		ex.previewTab = 0
	}
	g := ex.previewGroups[ex.previewTab]
	if ex.previewRow >= len(g.cells) {
		ex.previewRow = 0
	}
	if ex.previewCol >= len(g.specs) {
		ex.previewCol = 0
	}
}

// activePreviewGroup returns the currently focused preview table group.
func (ex *extractionLogState) activePreviewGroup() *previewTableGroup {
	if ex.previewTab < len(ex.previewGroups) {
		return &ex.previewGroups[ex.previewTab]
	}
	return nil
}

// --- Rendering ---

// buildExtractionOverlay renders the extraction progress overlay.
func (m *Model) buildExtractionOverlay() string {
	ex := m.extraction
	if ex == nil {
		return ""
	}

	contentW := m.extractionOverlayWidth()
	innerW := contentW - 4 // padding

	// Title line.
	title := m.styles.HeaderSection.Render(" Extracting ")
	filename := m.styles.HeaderHint.Render(" " + truncateRight(ex.Filename, innerW-16))

	return m.buildExtractionPipelineOverlay(contentW, innerW, title+filename)
}

// previewNaturalWidth returns the minimum inner width needed to display
// all preview tables without wrapping. Returns 0 if there are no groups.
func previewNaturalWidth(groups []previewTableGroup, sepW int) int {
	var maxW int
	for _, g := range groups {
		nw := naturalWidths(g.specs, g.cells)
		w := 0
		for _, cw := range nw {
			w += cw
		}
		if n := len(nw); n > 1 {
			w += (n - 1) * sepW
		}
		if w > maxW {
			maxW = w
		}
	}
	return maxW
}

// buildExtractionPipelineOverlay renders the pipeline step view with an
// optional operation preview section below. The preview is dimmed when
// not in explore mode and fully interactive when exploring.
func (m *Model) buildExtractionPipelineOverlay(
	contentW, innerW int, titleLine string,
) string {
	ex := m.extraction
	ruleStyle := lipgloss.NewStyle().Foreground(border)

	// Compute column widths across all active steps for alignment.
	active := ex.activeSteps()
	var maxDetailW, maxMetricW, maxElapsedW int
	for _, si := range active {
		info := ex.Steps[si]
		if w := len(info.Detail); w > maxDetailW {
			maxDetailW = w
		}
		if w := len(info.Metric); w > maxMetricW {
			maxMetricW = w
		}
		var e string
		switch {
		case info.Elapsed > 0:
			e = fmt.Sprintf("%.2fs", info.Elapsed.Seconds())
		case info.Status == stepRunning && !info.Started.IsZero():
			e = fmt.Sprintf("%.1fs", time.Since(info.Started).Seconds())
		}
		if w := len(e); w > maxElapsedW {
			maxElapsedW = w
		}
	}
	colWidths := extractionColWidths{
		Detail:  maxDetailW,
		Metric:  maxMetricW,
		Elapsed: maxElapsedW,
	}

	// Render step content for the viewport, tracking the line offset of
	// each step header so we can scroll the cursor into view.
	var stepParts []string
	cursorLine := 0
	lineCount := 0
	for i, si := range active {
		info := ex.Steps[si]
		focused := !ex.exploring && i == ex.cursor
		part := m.renderExtractionStep(si, info, innerW, focused, colWidths)
		if i > 0 && strings.Contains(stepParts[i-1], "\n") {
			lineCount++
		}
		if i == ex.cursor {
			cursorLine = lineCount
		}
		lineCount += strings.Count(part, "\n") + 1
		stepParts = append(stepParts, part)
	}
	var stepBuf strings.Builder
	for i, part := range stepParts {
		if i > 0 {
			stepBuf.WriteByte('\n')
			if strings.Contains(stepParts[i-1], "\n") {
				stepBuf.WriteByte('\n')
			}
		}
		stepBuf.WriteString(part)
	}
	stepContent := stepBuf.String()

	// Determine available height for the viewport, reserving space for the
	// operation preview section when operations are available.
	hasOps := ex.Done && len(ex.operations) > 0
	previewSection := ""
	previewLines := 0
	if hasOps {
		previewSection = m.renderOperationPreviewSection(innerW, ex.exploring)
		previewLines = strings.Count(previewSection, "\n") + 2 // +2 for separator + blank
	}

	maxH := m.effectiveHeight()*2/3 - 6 - previewLines
	if maxH < 4 {
		maxH = 4
	}
	contentLines := strings.Count(stepContent, "\n") + 1
	vpH := contentLines
	if vpH > maxH {
		vpH = maxH
	}

	ex.Viewport.Width = innerW
	ex.Viewport.Height = vpH
	ex.Viewport.SetContent(stepContent)

	if vpH < contentLines && !ex.exploring {
		si := ex.cursorStep()
		streaming := ex.Steps[si].Status == stepRunning

		switch {
		case streaming:
			// Follow the growing output so the user sees new tokens.
			ex.Viewport.GotoBottom()
		case ex.cursorExpanded():
			// Step is expanded and overflows: user may be scrolling with
			// j/k, so don't reposition to the header.
		default:
			// Keep the cursor step header in view.
			yOff := ex.Viewport.YOffset
			if cursorLine < yOff {
				ex.Viewport.SetYOffset(cursorLine)
			} else if cursorLine >= yOff+vpH {
				ex.Viewport.SetYOffset(cursorLine - vpH + 1)
			}
		}
	}

	vpView := ex.Viewport.View()
	if ex.exploring {
		vpView = lipgloss.NewStyle().Foreground(textDim).Render(vpView)
	}

	// Scroll indicator in rule.
	var rule string
	if ex.Viewport.TotalLineCount() > ex.Viewport.Height {
		var label string
		switch {
		case ex.Viewport.AtTop():
			label = "Top"
		case ex.Viewport.AtBottom():
			label = "Bot"
		default:
			label = fmt.Sprintf("%d%%", int(ex.Viewport.ScrollPercent()*100))
		}
		indicator := lipgloss.NewStyle().Foreground(textDim).Render(" " + label + " ")
		indicatorW := lipgloss.Width(indicator)
		rightW := max(0, innerW-indicatorW)
		rule = ruleStyle.Render(strings.Repeat("\u2500", rightW)) + indicator
	} else {
		rule = ruleStyle.Render(strings.Repeat("\u2500", innerW))
	}

	// Hint line varies by mode.
	var hints []string
	if ex.exploring {
		hints = append(hints, m.helpItem("j/k", "rows"), m.helpItem("h/l", "cols"))
		if len(ex.previewGroups) > 1 {
			hints = append(hints, m.helpItem("b/f", "tabs"))
		}
		if !ex.HasError {
			hints = append(hints, m.helpItem("a", "accept"))
		}
		hints = append(hints, m.helpItem("x", "back"), m.helpItem("esc", "discard"))
	} else {
		hints = append(hints, m.helpItem("j/k", "navigate"))
		cursorStatus := ex.Steps[ex.cursorStep()].Status
		if ex.Done || cursorStatus == stepDone || cursorStatus == stepFailed {
			hints = append(hints, m.helpItem("\u21b5", "expand"))
		}
		if hasOps {
			hints = append(hints, m.helpItem("x", "explore"))
		}
		if ex.Done {
			if !ex.HasError {
				hints = append(hints, m.helpItem("a", "accept"))
			}
			hints = append(hints, m.helpItem("esc", "discard"))
		} else {
			hints = append(hints, m.helpItem("esc", "cancel"))
		}
	}
	hintStr := joinWithSeparator(m.helpSeparator(), hints...)

	parts := []string{titleLine, "", vpView, rule}
	if previewSection != "" {
		parts = append(parts, "", previewSection)
	}
	parts = append(parts, ruleStyle.Render(strings.Repeat("\u2500", innerW)), hintStr)
	boxContent := lipgloss.JoinVertical(lipgloss.Left, parts...)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2).
		Width(contentW).
		Render(boxContent)
}

// renderOperationPreviewSection renders the operation preview table section.
// When interactive is true, the row/col cursors are shown and the section
// renders at full brightness. When false, the entire section is dimmed.
func (m *Model) renderOperationPreviewSection(innerW int, interactive bool) string {
	ex := m.extraction
	if len(ex.previewGroups) == 0 {
		ex.previewGroups = groupOperationsByTable(ex.operations)
	}
	groups := ex.previewGroups
	if len(groups) == 0 {
		return lipgloss.NewStyle().Foreground(textDim).Render("no operations")
	}

	sep := m.styles.TableSeparator.Render(" \u2502 ")
	divSep := m.styles.TableSeparator.Render("\u2500\u253c\u2500")
	sepW := lipgloss.Width(sep)

	// Tab bar: active tab highlighted in explore mode, all dimmed otherwise.
	tabParts := make([]string, 0, len(groups)*2)
	for i, g := range groups {
		if interactive && i == ex.previewTab {
			tabParts = append(tabParts, m.styles.TabActive.Render(g.name))
		} else {
			tabParts = append(tabParts, m.styles.TabInactive.Render(g.name))
		}
		if i < len(groups)-1 {
			tabParts = append(tabParts, "   ")
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Left, tabParts...)
	underline := m.styles.TabUnderline.Render(strings.Repeat("\u2501", innerW))

	// Always render a single tab: the active one in explore mode,
	// the first one in pipeline mode.
	tabIdx := 0
	if interactive {
		tabIdx = ex.previewTab
	}
	if tabIdx >= len(groups) {
		tabIdx = 0
	}
	g := groups[tabIdx]
	tableSection := m.renderPreviewTable(g, innerW, sepW, sep, divSep, interactive)

	var b strings.Builder
	b.WriteString(tabBar)
	b.WriteByte('\n')
	b.WriteString(underline)
	b.WriteByte('\n')
	b.WriteString(tableSection)

	result := b.String()
	if !interactive {
		result = lipgloss.NewStyle().Foreground(textDim).Render(result)
	}
	return result
}

// renderPreviewTable renders a single table group with header, divider, and rows.
func (m *Model) renderPreviewTable(
	g previewTableGroup, innerW, sepW int, sep, divSep string, interactive bool,
) string {
	ex := m.extraction
	seps := make([]string, max(len(g.specs)-1, 0))
	divSeps := make([]string, len(seps))
	for i := range seps {
		seps[i] = sep
		divSeps[i] = divSep
	}
	widths := columnWidths(g.specs, g.cells, innerW, sepW, nil)

	colCursor := -1
	if interactive {
		colCursor = ex.previewCol
		if colCursor >= len(g.specs) {
			colCursor = len(g.specs) - 1
		}
	}

	header := renderHeaderRow(
		g.specs, widths, seps, colCursor, nil, false, false, g.cells, m.styles,
	)
	divider := renderDivider(widths, seps, divSep, m.styles.TableSeparator)

	rowCursor := -1
	if interactive {
		rowCursor = ex.previewRow
		if rowCursor >= len(g.cells) {
			rowCursor = len(g.cells) - 1
		}
	}
	rows := renderRows(
		g.specs, g.cells, g.meta, widths,
		seps, seps, rowCursor, colCursor, 0, m.styles, pinRenderContext{},
	)

	parts := []string{header, divider}
	if len(rows) > 0 {
		parts = append(parts, strings.Join(rows, "\n"))
	}
	return strings.Join(parts, "\n")
}

// extractionColWidths holds the max width of each column across all steps.
type extractionColWidths struct {
	Detail  int
	Metric  int
	Elapsed int
}

// renderExtractionStep renders a single step line with status icon and detail.
func (m *Model) renderExtractionStep(
	si extractionStep,
	info extractionStepInfo,
	innerW int,
	focused bool,
	cols extractionColWidths,
) string {
	name := stepName(si)
	ex := m.extraction
	hint := m.styles.HeaderHint

	var icon string
	var nameStyle lipgloss.Style
	switch info.Status {
	case stepPending:
		icon = "  "
		nameStyle = lipgloss.NewStyle().Foreground(textDim)
	case stepRunning:
		icon = ex.Spinner.View() + " "
		nameStyle = lipgloss.NewStyle().Foreground(accent)
	case stepDone:
		icon = lipgloss.NewStyle().Foreground(success).Render("ok") + " "
		nameStyle = lipgloss.NewStyle().Foreground(textBright)
	case stepFailed:
		icon = lipgloss.NewStyle().Foreground(danger).Render("xx") + " "
		nameStyle = lipgloss.NewStyle().Foreground(danger)
	}

	// Determine if expanded: auto-expand running/failed, and keep LLM expanded
	// after streaming completes so the result doesn't flash and collapse.
	expanded := info.Status == stepRunning || info.Status == stepFailed ||
		(si == stepLLM && info.Status == stepDone)
	if toggled, ok := ex.expanded[si]; ok {
		expanded = toggled
	}

	// Cursor indicator: show as soon as the step itself is done/failed,
	// so users can inspect completed steps while later steps still run.
	cursor := "  "
	stepSettled := info.Status == stepDone || info.Status == stepFailed
	if focused && (ex.Done || stepSettled) {
		if expanded {
			cursor = lipgloss.NewStyle().Foreground(accent).Render("\u25be ")
		} else {
			cursor = lipgloss.NewStyle().Foreground(accent).Render("\u25b8 ")
		}
	}

	// Columnar header: icon | name | detail | metric | elapsed [| rerun hint].
	header := cursor + icon + nameStyle.Render(fmt.Sprintf("%-4s", name))
	if cols.Detail > 0 {
		header += "  " + hint.Render(fmt.Sprintf("%-*s", cols.Detail, info.Detail))
	}
	if cols.Metric > 0 {
		header += "  " + hint.Render(fmt.Sprintf("%*s", cols.Metric, info.Metric))
	}
	if cols.Elapsed > 0 {
		var e string
		switch {
		case info.Elapsed > 0:
			e = fmt.Sprintf("%.2fs", info.Elapsed.Seconds())
		case info.Status == stepRunning && !info.Started.IsZero():
			e = fmt.Sprintf("%.1fs", time.Since(info.Started).Seconds())
		}
		header += "  " + hint.Render(fmt.Sprintf("%*s", cols.Elapsed, e))
	}
	if si == stepLLM && info.Status == stepDone && ex.Done && focused {
		header += "  " + lipgloss.NewStyle().Foreground(textDim).Render("r to rerun")
	}

	if !expanded || len(info.Logs) == 0 {
		return header
	}

	// Expanded: header + rendered log content with left border pipe.
	pipeIndent := "     " // align pipe under step name
	pipe := lipgloss.NewStyle().Foreground(border).Render("\u2502") + " "
	logW := innerW - len(pipeIndent) - 2 // pipe + space
	raw := strings.Join(info.Logs, "\n")

	var rendered string
	if si == stepLLM {
		// Pretty-print JSON, then render as a fenced code block via glamour.
		formatted := raw
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(extract.StripCodeFences(raw)), "", "  "); err == nil {
			formatted = buf.String()
		}
		md := "```json\n" + formatted + "\n```"
		rendered = strings.TrimSpace(ex.renderMarkdown(md, logW))
	} else {
		rendered = m.styles.HeaderHint.Render(wordWrap(raw, logW))
	}

	var b strings.Builder
	b.WriteString(header)
	for _, line := range strings.Split(rendered, "\n") {
		b.WriteByte('\n')
		b.WriteString(pipeIndent)
		b.WriteString(pipe)
		b.WriteString(line)
	}
	return b.String()
}

func stepName(si extractionStep) string {
	switch si {
	case stepText:
		return "text"
	case stepExtract:
		return "ext"
	case stepLLM:
		return "llm"
	case numExtractionSteps:
		return "?"
	}
	return "?"
}

// extractionModelLabel returns the model name used for extraction.
func (m *Model) extractionModelLabel() string {
	if m.extractionModel != "" {
		return m.extractionModel
	}
	return m.llmModelLabel()
}

func truncateRight(s string, maxW int) string {
	if lipgloss.Width(s) <= maxW {
		return s
	}
	if maxW < 4 {
		return s[:maxW]
	}
	return s[:maxW-2] + ".."
}

// --- Operation preview rendering ---

// previewColDef maps an Operation.Data key to a column spec and formatter.
type previewColDef struct {
	dataKey string
	spec    columnSpec
	format  func(any) string
}

// previewColumns returns the column definitions for rendering an operation
// preview for the given table. Specs are pulled from the same functions that
// define the main tab columns, so the preview matches the real UI.
func previewColumns(tableName string) []previewColDef {
	switch tableName {
	case "vendors":
		s := vendorColumnSpecs()
		return []previewColDef{
			{"name", s[1], fmtAnyText},
			{"contact_name", s[2], fmtAnyText},
			{"email", s[3], fmtAnyText},
			{"phone", s[4], fmtAnyText},
			{"website", s[5], fmtAnyText},
		}
	case tableDocuments:
		s := documentColumnSpecs()
		return []previewColDef{
			{"title", s[1], fmtAnyText},
			{"mime_type", s[3], fmtAnyText},
			{"notes", s[5], fmtAnyText},
		}
	case "quotes":
		s := quoteColumnSpecs()
		return []previewColDef{
			{"project_id", s[1], fmtAnyFK},
			{"vendor_id", s[2], fmtAnyFK},
			{"total_cents", s[3], fmtAnyCents},
			{"labor_cents", s[4], fmtAnyCents},
			{"materials_cents", s[5], fmtAnyCents},
			{"other_cents", s[6], fmtAnyCents},
			{"received_date", s[7], fmtAnyText},
		}
	case "maintenance_items":
		s := maintenanceColumnSpecs()
		return []previewColDef{
			{"name", s[1], fmtAnyText},
			{"category_id", s[2], fmtAnyFK},
			{"appliance_id", s[3], fmtAnyFK},
			{"interval_months", s[6], fmtAnyInterval},
		}
	case "appliances":
		s := applianceColumnSpecs()
		return []previewColDef{
			{"name", s[1], fmtAnyText},
			{"brand", s[2], fmtAnyText},
			{"model_number", s[3], fmtAnyText},
			{"serial_number", s[4], fmtAnyText},
			{"location", s[5], fmtAnyText},
			{"purchase_date", s[6], fmtAnyText},
			{"warranty_expiry", s[8], fmtAnyText},
			{"cost_cents", s[9], fmtAnyCents},
		}
	default:
		return nil
	}
}

// previewTabName maps a DB table name to the display name used in the tab bar.
var previewTabName = map[string]string{
	tableDocuments:      "Docs",
	"vendors":           "Vendors",
	"quotes":            "Quotes",
	"maintenance_items": "Maintenance",
	"appliances":        "Appliances",
}

// previewTableGroup holds the column specs and cell rows for one table section
// in the operation preview.
type previewTableGroup struct {
	name  string // display name for the tab bar
	table string // DB table name
	specs []columnSpec
	cells [][]cell
	meta  []rowMeta
}

// groupOperationsByTable groups operations into per-table sections, collecting
// all data keys across operations within a table and building cell rows.
func groupOperationsByTable(ops []extract.Operation) []previewTableGroup {
	// Preserve first-seen order.
	var order []string
	groups := make(map[string]*previewTableGroup)

	for _, op := range ops {
		allDefs := previewColumns(op.Table)
		if allDefs == nil || len(op.Data) == 0 {
			continue
		}

		g, ok := groups[op.Table]
		if !ok {
			name := previewTabName[op.Table]
			if name == "" {
				name = op.Table
			}
			g = &previewTableGroup{name: name, table: op.Table}
			groups[op.Table] = g
			order = append(order, op.Table)
		}

		// On first op for this table, or when new keys appear, rebuild
		// the spec list as the union of all populated keys.
		for _, d := range allDefs {
			if _, present := op.Data[d.dataKey]; !present {
				continue
			}
			// Check if this column is already in the group's specs.
			found := false
			for _, existing := range g.specs {
				if existing.Title == d.spec.Title {
					found = true
					break
				}
			}
			if !found {
				g.specs = append(g.specs, d.spec)
			}
		}
	}

	// Second pass: build cell rows using the finalized spec list.
	for _, op := range ops {
		g := groups[op.Table]
		if g == nil {
			continue
		}
		allDefs := previewColumns(op.Table)
		if allDefs == nil {
			continue
		}

		// Build a lookup from spec title to the def's formatter.
		fmtByTitle := make(map[string]func(any) string, len(allDefs))
		keyByTitle := make(map[string]string, len(allDefs))
		for _, d := range allDefs {
			fmtByTitle[d.spec.Title] = d.format
			keyByTitle[d.spec.Title] = d.dataKey
		}

		row := make([]cell, len(g.specs))
		for i, spec := range g.specs {
			key := keyByTitle[spec.Title]
			v, ok := op.Data[key]
			if ok {
				fn := fmtByTitle[spec.Title]
				row[i] = cell{Value: fn(v), Kind: spec.Kind}
			} else {
				row[i] = cell{Kind: spec.Kind, Null: true}
			}
		}
		g.cells = append(g.cells, row)
		g.meta = append(g.meta, rowMeta{})
	}

	result := make([]previewTableGroup, 0, len(order))
	for _, tbl := range order {
		result = append(result, *groups[tbl])
	}
	return result
}

// --- Preview value formatters ---

func fmtAnyText(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', 2, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func fmtAnyCents(v any) string {
	if val, ok := v.(float64); ok {
		return data.FormatCents(int64(val))
	}
	return fmtAnyText(v)
}

func fmtAnyFK(v any) string {
	s := fmtAnyText(v)
	if s != "" && s != "0" {
		return "#" + s
	}
	return s
}

func fmtAnyInterval(v any) string {
	if val, ok := v.(float64); ok {
		return formatInterval(int(val))
	}
	return fmtAnyText(v)
}

// --- Layout helpers ---

func (m *Model) extractionOverlayWidth() int {
	screenW := m.effectiveWidth() - 8

	// Base width for pipeline steps.
	w := 80

	// Widen to fit the widest preview table if operations are available.
	ex := m.extraction
	if ex != nil && len(ex.operations) > 0 {
		if len(ex.previewGroups) == 0 {
			ex.previewGroups = groupOperationsByTable(ex.operations)
		}
		sep := m.styles.TableSeparator.Render(" \u2502 ")
		sepW := lipgloss.Width(sep)
		needed := previewNaturalWidth(ex.previewGroups, sepW) + 4 // +4 for padding
		if needed > w {
			w = needed
		}
	}

	if w > screenW {
		w = screenW
	}
	if w < 50 {
		w = 50
	}
	return w
}
