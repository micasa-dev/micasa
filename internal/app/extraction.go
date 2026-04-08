// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/extract"
	"github.com/micasa-dev/micasa/internal/llm"
	"github.com/micasa-dev/micasa/internal/locale"
)

// --- Extraction step types ---

type extractionStep int

const (
	stepText extractionStep = iota
	stepExtract
	stepLLM
	numExtractionSteps
)

const tableDocuments = data.TableDocuments

var nextExtractionID atomic.Uint64

type stepStatus int

const (
	stepPending stepStatus = iota
	stepRunning
	stepDone
	stepFailed
	stepSkipped
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
	markdownRenderer

	ID          uint64
	DocID       string
	Filename    string
	Steps       [numExtractionSteps]extractionStepInfo
	Spinner     spinner.Model
	Viewport    viewport.Model
	Visible     bool
	Done        bool
	HasError    bool
	ctx         context.Context
	CancelFn    context.CancelFunc
	llmCancelFn context.CancelFunc // cancels the LLM timeout context

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

	// Per-tool image acquisition state (non-nil during acquisition phase).
	acquireTools   []extract.AcquireToolState
	docPages       int // total PDF pages when capped (0 = all pages processed)
	extractedPages int // pages actually processed

	// Channel references for the waitFor loop pattern.
	extractCh <-chan extract.ExtractProgress
	llmCh     <-chan llm.StreamChunk

	// Which steps are active (skipped steps are simply not shown).
	hasText    bool
	hasExtract bool
	hasLLM     bool

	// Pending results held until user accepts.
	operations []extract.Operation // validated operations (not yet executed)
	shadowDB   *extract.ShadowDB   // staged operations for cross-reference resolution
	accepted   bool                // true once user accepted results
	pendingDoc *data.Document      // deferred creation: unpersisted document (magic-add)

	// Cursor and expand/collapse state for exploring output.
	cursor       int                     // index into activeSteps()
	toolCursor   int                     // -1 = parent header, 0..N-1 = child tool line
	cursorManual bool                    // true after j/k; disables auto-follow
	expanded     map[extractionStep]bool // manual expand/collapse overrides

	// Explore mode: read-only table navigation for proposed operations.
	exploring     bool                // true when in table explore mode
	previewGroups []previewTableGroup // cached grouped operations
	previewTab    int                 // active tab in explore mode
	previewRow    int                 // row cursor within active tab
	previewCol    int                 // column cursor within active tab

	// LLM ping state: ping runs concurrently with earlier steps.
	llmPingDone bool  // true once ping completed (success or fail)
	llmPingErr  error // non-nil if LLM was unreachable

	// Model picker: inline model selection before rerunning LLM step.
	modelPicker *modelCompleter // non-nil when picker is showing
	modelFilter string          // current filter text for fuzzy matching
}

// cancelLLMTimeout releases the LLM inference timeout context if set.
func (ex *extractionLogState) cancelLLMTimeout() {
	if ex.llmCancelFn != nil {
		ex.llmCancelFn()
		ex.llmCancelFn = nil
	}
}

// closeShadowDB closes and nils the shadow DB if present.
func (ex *extractionLogState) closeShadowDB() {
	if ex.shadowDB != nil {
		_ = ex.shadowDB.Close()
		ex.shadowDB = nil
	}
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

// stepDefaultExpanded returns the default expanded state for a step before
// any user toggle. Running and failed steps auto-expand so the cursor
// tracks progress. The ext/ocr step stays collapsed by default while
// running since the parent header shows combined progress. Once done,
// only the LLM step stays expanded (streaming output); text and ext
// steps collapse their log content by default.
func (ex *extractionLogState) stepDefaultExpanded(si extractionStep) bool {
	info := ex.Steps[si]
	if info.Status == stepRunning || info.Status == stepFailed || info.Status == stepSkipped {
		// Ext with tools: collapsed by default while running since
		// the parent header shows the combined percentage.
		if si == stepExtract && len(ex.acquireTools) > 0 && info.Status == stepRunning {
			return false
		}
		return true
	}
	return si == stepLLM && info.Status == stepDone
}

// stepExpanded returns whether a step is currently expanded, accounting
// for both the default and any user toggle.
func (ex *extractionLogState) stepExpanded(si extractionStep) bool {
	if toggled, ok := ex.expanded[si]; ok {
		return toggled
	}
	return ex.stepDefaultExpanded(si)
}

// advanceCursor moves the cursor to the latest settled (done/failed/skipped)
// step. In manual mode (after user presses j/k) this is a no-op.
func (ex *extractionLogState) advanceCursor() {
	if ex.cursorManual {
		return
	}
	active := ex.activeSteps()
	for i := len(active) - 1; i >= 0; i-- {
		s := ex.Steps[active[i]].Status
		if s == stepDone || s == stepFailed || s == stepSkipped {
			ex.cursor = i
			ex.toolCursor = -1
			return
		}
	}
}

// --- Messages ---

// extractionProgressMsg delivers a single async extraction progress update.
type extractionProgressMsg struct {
	ID       uint64
	Progress extract.ExtractProgress
}

// extractionLLMStartedMsg delivers the LLM stream channel.
type extractionLLMStartedMsg struct {
	ID uint64
	Ch <-chan llm.StreamChunk
}

// extractionLLMChunkMsg delivers a single LLM token.
type extractionLLMChunkMsg struct {
	ID      uint64
	Content string
	Done    bool
	Err     error
}

// extractionLLMPingMsg delivers the result of a background LLM ping.
type extractionLLMPingMsg struct {
	ID  uint64
	Err error // nil = reachable, non-nil = unreachable
}

// --- Overlay lifecycle ---

// startExtractionOverlay opens the extraction progress overlay and kicks off
// the first applicable step. Returns nil if no async steps are needed.
func (m *Model) startExtractionOverlay(
	docID string,
	filename string,
	fileData []byte,
	mime string,
	extractedText string,
	extractData []byte,
) tea.Cmd {
	needsExtract := extract.NeedsOCR(m.ex.extractors, mime)
	needsLLM := m.extractionLLMClient() != nil

	// Skip OCR when the document already has extracted text from a
	// previous run -- feed existing text directly to the LLM.
	hasExistingText := strings.TrimSpace(extractedText) != ""
	if hasExistingText {
		needsExtract = false
	}

	if !needsExtract && !needsLLM {
		return nil
	}

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = appStyles.AccentText()

	//nolint:gosec // cancel stored in ex.CancelFn, called on extraction close
	ctx, cancel := context.WithCancel(
		m.lifecycleCtx(),
	)

	// Text extraction only applies to PDFs and text files -- unless
	// we already have text from a previous extraction (e.g. prior OCR
	// on an image), in which case we show the cached text.
	hasText := !extract.IsImageMIME(mime) || hasExistingText

	// Build initial text source from already-extracted text.
	var sources []extract.TextSource
	if hasText && hasExistingText {
		var tool, desc string
		switch {
		case mime == extract.MIMEApplicationPDF:
			tool = "pdftotext"
			desc = "Digital text extracted directly from the PDF."
		case strings.HasPrefix(mime, "text/"):
			tool = "plaintext"
			desc = "Plain text content."
		case extract.IsImageMIME(mime):
			tool = "tesseract"
			desc = "Text from previous OCR extraction."
		default:
			tool = mime
		}
		sources = append(sources, extract.TextSource{
			Tool: tool,
			Desc: desc,
			Text: extractedText,
			Data: extractData,
		})
	}

	state := &extractionLogState{
		ID:            nextExtractionID.Add(1),
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
		extractors:    m.ex.extractors,
		hasText:       hasText,
		hasExtract:    needsExtract,
		hasLLM:        needsLLM,
		toolCursor:    -1,
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
		case extract.IsImageMIME(mime):
			textTool = "ocr"
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

	// Background any existing foreground extraction instead of cancelling.
	if m.ex.extraction != nil {
		m.backgroundExtraction()
	}
	m.ex.extraction = state

	var cmd tea.Cmd
	if needsExtract {
		state.Steps[stepExtract].Status = stepRunning
		state.Steps[stepExtract].Started = time.Now()
		cmd = asyncExtractCmd(ctx, state)
		// Ping LLM concurrently so we know before OCR finishes whether
		// the LLM endpoint is reachable.
		if needsLLM {
			return tea.Batch(cmd, m.llmPingCmd(state), state.Spinner.Tick)
		}
	} else if needsLLM {
		state.Steps[stepLLM].Status = stepRunning
		state.Steps[stepLLM].Started = time.Now()
		state.Steps[stepLLM].Detail = m.extractionModelLabel()
		cmd = m.llmExtractCmd(ctx, state)
	}

	return tea.Batch(cmd, state.Spinner.Tick)
}

// findExtraction returns the extraction with the given ID, checking the
// foreground extraction first, then scanning bgExtractions.
func (m *Model) findExtraction(id uint64) *extractionLogState {
	if m.ex.extraction != nil && m.ex.extraction.ID == id {
		return m.ex.extraction
	}
	for _, ex := range m.ex.bgExtractions {
		if ex.ID == id {
			return ex
		}
	}
	return nil
}

// isBgExtraction returns true when the given extraction is in bgExtractions.
func (m *Model) isBgExtraction(ex *extractionLogState) bool {
	return slices.Contains(m.ex.bgExtractions, ex)
}

// cancelExtraction cancels any in-flight extraction and clears state.
func (m *Model) cancelExtraction() {
	if m.ex.extraction == nil {
		return
	}
	m.ex.extraction.cancelLLMTimeout()
	if m.ex.extraction.CancelFn != nil {
		m.ex.extraction.CancelFn()
	}
	m.ex.extraction.closeShadowDB()
	m.ex.extraction = nil
}

// interruptExtraction cancels the running step but keeps the overlay open so
// the user can inspect partial results, rerun, or dismiss with ESC.
func (m *Model) interruptExtraction() {
	ex := m.ex.extraction
	if ex == nil || ex.Done {
		return
	}
	ex.cancelLLMTimeout()
	if ex.CancelFn != nil {
		ex.CancelFn()
	}
	for i := range ex.Steps {
		if ex.Steps[i].Status == stepRunning {
			ex.Steps[i].Status = stepFailed
			ex.Steps[i].Elapsed = time.Since(ex.Steps[i].Started)
			ex.Steps[i].Logs = append(ex.Steps[i].Logs, "interrupted")
		}
	}
	ex.Done = true
	ex.HasError = true
	ex.advanceCursor()
}

// cancelAllExtractions cancels the foreground and all background extractions.
func (m *Model) cancelAllExtractions() {
	m.cancelExtraction()
	for _, ex := range m.ex.bgExtractions {
		ex.cancelLLMTimeout()
		if ex.CancelFn != nil {
			ex.CancelFn()
		}
		ex.closeShadowDB()
	}
	m.ex.bgExtractions = nil
}

// backgroundExtraction moves the foreground extraction to bgExtractions.
func (m *Model) backgroundExtraction() {
	if m.ex.extraction == nil {
		return
	}
	m.ex.extraction.Visible = false
	m.ex.bgExtractions = append(m.ex.bgExtractions, m.ex.extraction)
	m.ex.extraction = nil
}

// foregroundExtraction brings the most recent bg extraction to the foreground.
func (m *Model) foregroundExtraction() {
	n := len(m.ex.bgExtractions)
	if n == 0 {
		return
	}
	// If there's already a foreground extraction, background it first.
	if m.ex.extraction != nil {
		m.backgroundExtraction()
	}
	ex := m.ex.bgExtractions[n-1]
	m.ex.bgExtractions = m.ex.bgExtractions[:n-1]
	ex.Visible = true
	m.ex.extraction = ex
}

// --- Async commands ---

// asyncExtractCmd starts the async extraction pipeline and returns the
// first progress message via waitForExtractProgress.
func asyncExtractCmd(ctx context.Context, state *extractionLogState) tea.Cmd {
	ch := extract.ExtractWithProgress(
		ctx, state.fileData, state.mime, state.extractors,
	)
	state.extractCh = ch
	return waitForExtractProgress(state.ID, ch)
}

// waitForExtractProgress blocks until the next extraction progress update.
func waitForExtractProgress(id uint64, ch <-chan extract.ExtractProgress) tea.Cmd {
	return waitForStream(ch, func(p extract.ExtractProgress) tea.Msg {
		return extractionProgressMsg{ID: id, Progress: p}
	}, extractionProgressMsg{ID: id, Progress: extract.ExtractProgress{Done: true}})
}

// llmPingCmd fires a background ping to the LLM endpoint. The result is
// delivered via extractionLLMPingMsg so the extraction can skip the LLM
// step early if the server is unreachable.
func (m *Model) llmPingCmd(state *extractionLogState) tea.Cmd {
	client := m.extractionLLMClient()
	if client == nil {
		return nil
	}
	id := state.ID
	quickOpTimeout := client.Timeout()
	appCtx := m.lifecycleCtx()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(appCtx, quickOpTimeout)
		defer cancel()
		err := client.Ping(ctx)
		return extractionLLMPingMsg{ID: id, Err: err}
	}
}

// llmExtractCmd starts LLM document analysis with streaming.
//
// The timeout context (and its cancel function) is created on the calling
// goroutine -- not inside the returned closure -- so that ex.llmCancelFn
// is installed synchronously. This prevents a data race between the
// goroutine that runs the cmd and the main loop reading ex.llmCancelFn
// in cancelLLMTimeout (e.g. via Ctrl+C → interruptExtraction).
func (m *Model) llmExtractCmd(ctx context.Context, ex *extractionLogState) tea.Cmd {
	client := m.extractionLLMClient()
	if client == nil {
		return nil
	}
	schemaCtx := m.buildSchemaContext()
	id := ex.ID

	llmCtx := ctx
	if m.ex.extractionTimeout > 0 {
		var cancel context.CancelFunc
		// cancel is stored in ex.llmCancelFn and invoked by cancelLLMTimeout.
		llmCtx, cancel = context.WithTimeout(ctx, m.ex.extractionTimeout)
		ex.llmCancelFn = cancel
	}

	return func() tea.Msg {
		messages := extract.BuildExtractionPrompt(extract.ExtractionPromptInput{
			DocID:         ex.DocID,
			Filename:      ex.Filename,
			MIME:          ex.mime,
			SizeBytes:     int64(len(ex.fileData)),
			Schema:        schemaCtx,
			Sources:       ex.sources,
			SendTSV:       m.ex.ocrTSV,
			ConfThreshold: m.ex.ocrConfThreshold,
		})
		ch, err := client.ExtractStream(
			llmCtx,
			messages,
			extract.OperationsSchema(),
		)
		if err != nil {
			return extractionLLMChunkMsg{ID: id, Err: err, Done: true}
		}
		return extractionLLMStartedMsg{ID: id, Ch: ch}
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
		ctx.MaintenanceItems = toExtractRows(rows.MaintenanceItems)
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
func waitForLLMChunk(id uint64, ch <-chan llm.StreamChunk) tea.Cmd {
	return waitForStream(ch, func(c llm.StreamChunk) tea.Msg {
		return extractionLLMChunkMsg{ID: id, Content: c.Content, Done: c.Done, Err: c.Err}
	}, extractionLLMChunkMsg{ID: id, Done: true})
}

// --- Message handlers ---

// handleExtractionProgress processes an async extraction progress update.
func (m *Model) handleExtractionProgress(msg extractionProgressMsg) tea.Cmd {
	ex := m.findExtraction(msg.ID)
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
		if cmd := m.maybeStartLLMStep(ex); cmd != nil {
			return cmd
		}
		ex.Done = true
		if m.isBgExtraction(ex) {
			m.setStatusError("Extraction failed: " + ex.Filename)
		}
		return nil
	}

	if !p.Done {
		// Per-tool acquisition state update.
		if len(p.AcquireTools) > 0 {
			ex.acquireTools = p.AcquireTools
		}
		// OCR phase: page progress is shown in the tool line via
		// renderPageRatio; detail stays simple for the header.
		switch p.Phase {
		case "extract":
			step.Detail = fmt.Sprintf("page %d/%d", p.Page, p.Total)
			ex.docPages = p.DocPages
			ex.extractedPages = p.Total
		}
		return waitForExtractProgress(ex.ID, ex.extractCh)
	}

	// Extraction done.
	step.Status = stepDone
	step.Elapsed = time.Since(step.Started)
	nChars := len(strings.TrimSpace(p.Text))
	step.Detail = p.Tool
	step.Metric = fmt.Sprintf("%d chars", nChars)
	ex.docPages = p.DocPages
	ex.extractedPages = p.Total
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

	// Advance to LLM step if configured and reachable.
	if cmd := m.maybeStartLLMStep(ex); cmd != nil {
		return cmd
	}

	ex.Done = true
	if m.isBgExtraction(ex) {
		m.setStatusInfo("Extracted: " + ex.Filename)
	}
	return nil
}

// maybeStartLLMStep attempts to advance to the LLM step. If the concurrent
// ping determined the LLM is unreachable, the step is marked skipped and nil
// is returned. Otherwise it starts the LLM streaming command.
func (m *Model) maybeStartLLMStep(ex *extractionLogState) tea.Cmd {
	if !ex.hasLLM {
		return nil
	}
	// Already marked skipped by the ping handler.
	if ex.Steps[stepLLM].Status == stepSkipped {
		return nil
	}
	client := m.extractionLLMClient()
	if client == nil {
		return nil
	}
	ex.Steps[stepLLM].Status = stepRunning
	ex.Steps[stepLLM].Started = time.Now()
	ex.Steps[stepLLM].Detail = m.extractionModelLabel()
	return m.llmExtractCmd(ex.ctx, ex)
}

// handleExtractionLLMPing processes the background LLM ping result.
func (m *Model) handleExtractionLLMPing(msg extractionLLMPingMsg) {
	ex := m.findExtraction(msg.ID)
	if ex == nil {
		return
	}
	ex.llmPingDone = true
	ex.llmPingErr = msg.Err

	if msg.Err != nil {
		// Mark LLM as skipped immediately so the strikethrough renders
		// in real time, even while earlier steps are still running.
		ex.Steps[stepLLM].Status = stepSkipped
		ex.Steps[stepLLM].Detail = m.extractionModelLabel()
		ex.Steps[stepLLM].Logs = append(ex.Steps[stepLLM].Logs, msg.Err.Error())

		// If extraction already finished, the pipeline is done.
		if ex.Steps[stepExtract].Status == stepDone || ex.Steps[stepExtract].Status == stepFailed {
			ex.Done = true
			ex.advanceCursor()
			if m.isBgExtraction(ex) {
				m.setStatusInfo(fmt.Sprintf("Extracted: %s (LLM skipped)", ex.Filename))
			}
		}
	}
}

// handleExtractionLLMStarted stores the LLM stream channel and starts reading.
func (m *Model) handleExtractionLLMStarted(msg extractionLLMStartedMsg) tea.Cmd {
	ex := m.findExtraction(msg.ID)
	if ex == nil {
		return nil
	}
	ex.llmCh = msg.Ch
	return waitForLLMChunk(ex.ID, msg.Ch)
}

// handleExtractionLLMChunk processes a single LLM token.
func (m *Model) handleExtractionLLMChunk(msg extractionLLMChunkMsg) tea.Cmd {
	ex := m.findExtraction(msg.ID)
	if ex == nil {
		return nil
	}

	step := &ex.Steps[stepLLM]

	if msg.Err != nil {
		ex.cancelLLMTimeout()
		step.Status = stepFailed
		step.Elapsed = time.Since(step.Started)
		errMsg := msg.Err.Error()
		if errors.Is(msg.Err, context.DeadlineExceeded) {
			errMsg = fmt.Sprintf(
				"timed out after %s -- increase extraction.llm.timeout in config",
				step.Elapsed.Truncate(time.Second),
			)
		}
		step.Logs = append(step.Logs, errMsg)
		ex.HasError = true
		ex.Done = true
		ex.advanceCursor()
		if m.isBgExtraction(ex) {
			m.setStatusError("Extraction failed: " + ex.Filename)
		}
		return nil
	}

	if msg.Content != "" {
		ex.llmAccum.WriteString(msg.Content)
		step.Logs = strings.Split(ex.llmAccum.String(), "\n")
	}

	if msg.Done && step.Status == stepRunning {
		ex.cancelLLMTimeout()
		step.Elapsed = time.Since(step.Started)

		// Pretty-print the accumulated JSON for the log viewport.
		response := ex.llmAccum.String()
		if pretty, err := prettyJSON(response); err == nil {
			step.Logs = strings.Split(pretty, "\n")
		}
		ops, err := extract.ParseOperations(response)
		if err != nil {
			step.Status = stepFailed
			step.Logs = append(step.Logs, "parse error: "+err.Error())
			ex.HasError = true
		} else if err := extract.ValidateOperations(ops, extract.ExtractionAllowedOps); err != nil {
			step.Status = stepFailed
			step.Logs = append(step.Logs, "validation error: "+err.Error())
			ex.HasError = true
		} else if sdb, err := extract.NewShadowDB(m.store); err != nil {
			step.Status = stepFailed
			step.Logs = append(step.Logs, "shadow db: "+err.Error())
			ex.HasError = true
		} else if err := sdb.Stage(ops); err != nil {
			step.Status = stepFailed
			step.Logs = append(step.Logs, "stage ops: "+err.Error())
			ex.HasError = true
		} else {
			step.Status = stepDone
			ex.operations = ops
			ex.shadowDB = sdb
		}
		step.Metric = fmt.Sprintf("%d ops", len(ex.operations))

		ex.Done = true
		ex.advanceCursor()
		if m.isBgExtraction(ex) {
			if ex.HasError {
				m.setStatusError("Extraction failed: " + ex.Filename)
			} else {
				m.setStatusInfo("Extracted: " + ex.Filename)
			}
		}
		return nil
	}

	// More tokens coming.
	return waitForLLMChunk(ex.ID, ex.llmCh)
}

// applyStringField sets *dst to the string value at data[key] if present.
func applyStringField(data map[string]any, key string, dst *string) {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			*dst = s
		}
	}
}

// acceptExtraction persists all pending results and closes the overlay.
// Works regardless of whether LLM ran, failed, or was skipped.
func (m *Model) acceptExtraction() {
	ex := m.ex.extraction
	if ex == nil || !ex.Done || ex.accepted {
		return
	}

	if ex.pendingDoc != nil {
		if err := m.acceptDeferredExtraction(); err != nil {
			m.setStatusError(err.Error())
			return
		}
	} else {
		if err := m.acceptExistingExtraction(); err != nil {
			m.setStatusError(err.Error())
			return
		}
	}

	ex.accepted = true
	m.ex.extraction = nil
}

// acceptDeferredExtraction creates the deferred document, applying any
// LLM-produced document fields, then dispatches remaining operations.
func (m *Model) acceptDeferredExtraction() error {
	ex := m.ex.extraction
	doc := ex.pendingDoc

	// Apply fields from "create documents" operations to the pending doc.
	for _, op := range ex.operations {
		if op.Table == tableDocuments {
			applyStringField(op.Data, "title", &doc.Title)
			applyStringField(op.Data, "notes", &doc.Notes)
			applyStringField(op.Data, "entity_kind", &doc.EntityKind)
			if v, ok := op.Data["entity_id"]; ok {
				if n := extract.ParseStringID(v); n != "" {
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
	doc.ExtractionModel = m.extractionModelUsed(ex)
	ops, err := marshalOps(ex.operations)
	if err != nil {
		return fmt.Errorf("marshal extraction ops: %w", err)
	}
	doc.ExtractionOps = ops

	if err := m.store.CreateDocument(doc); err != nil {
		return fmt.Errorf("create document: %w", err)
	}

	// Commit non-document operations via shadow DB (vendors, quotes, etc.).
	var nonDocOps []extract.Operation
	for _, op := range ex.operations {
		if op.Table != tableDocuments {
			nonDocOps = append(nonDocOps, op)
		}
	}
	if err := m.commitShadowOperations(ex, nonDocOps); err != nil {
		return fmt.Errorf("dispatch operations: %w", err)
	}
	m.reloadAfterMutation()
	return nil
}

// acceptExistingExtraction persists extraction text and dispatches operations
// for an already-saved document. If the document was soft-deleted between
// extraction start and accept, it is restored first so the update and
// shadow operations succeed.
func (m *Model) acceptExistingExtraction() error {
	ex := m.ex.extraction

	// Restore the document if it was soft-deleted since extraction started.
	if m.store != nil && ex.DocID != "" {
		if err := m.store.EnsureDocumentAlive(ex.DocID); err != nil {
			return fmt.Errorf("document was deleted and could not be restored: %w", err)
		}
	}

	// Persist async extraction results and the model that produced them.
	if ex.DocID != "" && (ex.pendingText != "" || len(ex.pendingData) > 0 || ex.hasLLM) {
		if m.store != nil {
			model := m.extractionModelUsed(ex)
			ops, err := marshalOps(ex.operations)
			if err != nil {
				return fmt.Errorf("marshal extraction ops: %w", err)
			}
			if err := m.store.UpdateDocumentExtraction(
				ex.DocID, ex.pendingText, ex.pendingData, model, ops,
			); err != nil {
				return fmt.Errorf("save extraction: %w", err)
			}
		}
	}

	// Commit validated operations via shadow DB.
	if err := m.commitShadowOperations(ex, ex.operations); err != nil {
		return fmt.Errorf("dispatch operations: %w", err)
	}
	return nil
}

// commitShadowOperations commits staged operations through the shadow DB,
// remapping cross-referenced IDs to real database IDs.
func (m *Model) commitShadowOperations(ex *extractionLogState, ops []extract.Operation) error {
	if m.store == nil || len(ops) == 0 {
		return nil
	}
	if ex.shadowDB == nil {
		return errors.New("no shadow DB: operations were not staged")
	}
	err := ex.shadowDB.Commit(m.store, ops)
	ex.closeShadowDB()
	if err != nil {
		return err
	}
	m.reloadAfterMutation()
	return nil
}

// toggleExtractionTSV flips the ocrTSV setting and reruns the LLM step
// so the user can compare extraction quality with and without spatial layout.
func (m *Model) toggleExtractionTSV() tea.Cmd {
	m.ex.ocrTSV = !m.ex.ocrTSV
	if m.ex.ocrTSV {
		m.setStatusInfo("layout on")
	} else {
		m.setStatusInfo("layout off")
	}
	return m.rerunLLMExtraction()
}

// rerunLLMExtraction resets the LLM step and re-runs it.
func (m *Model) rerunLLMExtraction() tea.Cmd {
	ex := m.ex.extraction
	if ex == nil || !ex.hasLLM {
		return nil
	}

	// Cancel any previous LLM timeout before restarting.
	ex.cancelLLMTimeout()

	// Replace a cancelled context so the rerun has a live one.
	if ex.ctx.Err() != nil {
		ctx, cancel := context.WithCancel( //nolint:gosec // cancel stored in ex.CancelFn, called on extraction close
			m.lifecycleCtx(),
		)
		ex.ctx = ctx
		ex.CancelFn = cancel
	}

	// Reset LLM state (including any prior ping failure).
	ex.llmAccum.Reset()
	ex.llmPingDone = false
	ex.llmPingErr = nil
	ex.operations = nil
	ex.closeShadowDB()
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

	// Position cursor on the LLM step being rerun.
	active := ex.activeSteps()
	for i, s := range active {
		if s == stepLLM {
			ex.cursor = i
			break
		}
	}

	return tea.Batch(m.llmExtractCmd(ex.ctx, ex), ex.Spinner.Tick)
}

// --- Keyboard handler ---

// handleExtractionKey processes keys when the extraction overlay is visible.
func (m *Model) handleExtractionKey(msg tea.KeyPressMsg) tea.Cmd {
	ex := m.ex.extraction
	if ex.modelPicker != nil && !ex.modelPicker.Loading {
		return m.handleExtractionModelPickerKey(msg)
	}
	if ex.exploring {
		return m.handleExtractionExploreKey(msg)
	}
	return m.handleExtractionPipelineKey(msg)
}

// handleExtractionPipelineKey handles keys in pipeline navigation mode.
func (m *Model) handleExtractionPipelineKey(msg tea.KeyPressMsg) tea.Cmd {
	ex := m.ex.extraction
	switch {
	case key.Matches(msg, m.keys.ExtCancel):
		m.cancelExtraction()
	case key.Matches(msg, m.keys.ExtInterrupt):
		m.interruptExtraction()
	case key.Matches(msg, m.keys.ExtDown):
		ex.cursorManual = true
		overflow := ex.Viewport.TotalLineCount() > ex.Viewport.Height()
		scrollable := !ex.Done || ex.stepExpanded(ex.cursorStep())
		if overflow && scrollable && !ex.Viewport.AtBottom() {
			vp, cmd := ex.Viewport.Update(msg)
			ex.Viewport = vp
			return cmd
		}
		// Navigate within ext parent/child lines before moving to next step.
		if ex.cursorStep() == stepExtract && len(ex.acquireTools) > 0 {
			if ex.toolCursor == -1 && ex.stepExpanded(stepExtract) {
				ex.toolCursor = 0
				break
			}
			if ex.toolCursor >= 0 && ex.toolCursor < len(ex.acquireTools)-1 {
				ex.toolCursor++
				break
			}
			// On last child or collapsed parent: fall through to next step.
		}
		active := ex.activeSteps()
		for next := ex.cursor + 1; next < len(active); next++ {
			s := ex.Steps[active[next]].Status
			if s != stepPending {
				ex.cursor = next
				ex.toolCursor = -1
				break
			}
		}
	case key.Matches(msg, m.keys.ExtUp):
		ex.cursorManual = true
		overflow := ex.Viewport.TotalLineCount() > ex.Viewport.Height()
		scrollable := !ex.Done || ex.stepExpanded(ex.cursorStep())
		if overflow && scrollable && !ex.Viewport.AtTop() {
			vp, cmd := ex.Viewport.Update(msg)
			ex.Viewport = vp
			return cmd
		}
		// Navigate within ext parent/child lines before moving to prev step.
		if ex.cursorStep() == stepExtract && len(ex.acquireTools) > 0 {
			if ex.toolCursor > 0 {
				ex.toolCursor--
				break
			}
			if ex.toolCursor == 0 {
				ex.toolCursor = -1
				break
			}
			// toolCursor == -1: fall through to prev step.
		}
		active := ex.activeSteps()
		for prev := ex.cursor - 1; prev >= 0; prev-- {
			s := ex.Steps[active[prev]].Status
			if s != stepPending {
				ex.cursor = prev
				// Landing on ext from below: last child if expanded, else parent.
				if active[prev] == stepExtract && len(ex.acquireTools) > 0 &&
					ex.stepExpanded(stepExtract) {
					ex.toolCursor = len(ex.acquireTools) - 1
				} else {
					ex.toolCursor = -1
				}
				break
			}
		}
	case key.Matches(msg, m.keys.ExtToggle):
		si := ex.cursorStep()
		// Only toggle from the parent header, not from a child tool line.
		if si != stepExtract || len(ex.acquireTools) == 0 || ex.toolCursor == -1 {
			ex.expanded[si] = !ex.stepExpanded(si)
		}
	case key.Matches(msg, m.keys.ExtRemodel):
		if ex.Done && ex.hasLLM && ex.cursorStep() == stepLLM {
			return m.activateExtractionModelPicker()
		}
	case key.Matches(msg, m.keys.MagToggle):
		m.toggleMagMode()
	case key.Matches(msg, m.keys.ExtToggleTSV):
		if ex.Done && ex.hasLLM {
			return m.toggleExtractionTSV()
		}
	case key.Matches(msg, m.keys.ExtAccept):
		if ex.Done {
			m.acceptExtraction()
		}
	case key.Matches(msg, m.keys.ExtExplore):
		if ex.Done && len(ex.operations) > 0 {
			ex.enterExploreMode(m.cur)
		}
	case key.Matches(msg, m.keys.ExtBackground):
		if !ex.Done {
			m.backgroundExtraction()
		}
	default:
		vp, cmd := ex.Viewport.Update(msg)
		ex.Viewport = vp
		return cmd
	}
	return nil
}

// activateExtractionModelPicker opens the inline model picker in the
// extraction overlay, fetching the list of available models.
func (m *Model) activateExtractionModelPicker() tea.Cmd {
	ex := m.ex.extraction
	if ex.modelPicker != nil {
		return nil
	}
	ex.modelPicker = &modelCompleter{Loading: true}
	ex.modelFilter = ""

	client := m.extractionLLMClient()
	if client == nil {
		ex.modelPicker.Loading = false
		ex.modelPicker.All = mergeModelLists(nil)
		refilterModelCompleter(ex.modelPicker, "", m.extractionModelLabel())
		return nil
	}
	timeout := client.Timeout()
	appCtx := m.lifecycleCtx()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(appCtx, timeout)
		defer cancel()
		models, err := client.ListModels(ctx)
		return modelsListMsg{Models: models, Err: err}
	}
}

// handleExtractionModelPickerKey handles keys when the extraction model
// picker is showing.
func (m *Model) handleExtractionModelPickerKey(msg tea.KeyPressMsg) tea.Cmd {
	ex := m.ex.extraction
	mc := ex.modelPicker
	switch {
	case key.Matches(msg, m.keys.ExtModelCancel):
		ex.modelPicker = nil
		ex.modelFilter = ""
	case key.Matches(msg, m.keys.ExtModelUp):
		if mc.Cursor > 0 {
			mc.Cursor--
		}
	case key.Matches(msg, m.keys.ExtModelDown):
		if mc.Cursor < len(mc.Matches)-1 {
			mc.Cursor++
		}
	case key.Matches(msg, m.keys.ExtModelConfirm):
		if len(mc.Matches) > 0 {
			selected := mc.Matches[mc.Cursor].Name
			isLocal := mc.Matches[mc.Cursor].Local
			ex.modelPicker = nil
			ex.modelFilter = ""
			return m.switchExtractionModel(selected, isLocal)
		}
		ex.modelPicker = nil
		ex.modelFilter = ""
	case key.Matches(msg, m.keys.ExtModelBackspace):
		if len(ex.modelFilter) > 0 {
			ex.modelFilter = ex.modelFilter[:len(ex.modelFilter)-1]
			refilterModelCompleter(mc, ex.modelFilter, m.extractionModelLabel())
		}
	default:
		r := []rune(msg.String())
		if len(r) == 1 && unicode.IsPrint(r[0]) {
			ex.modelFilter += string(r[0])
			refilterModelCompleter(mc, ex.modelFilter, m.extractionModelLabel())
		}
	}
	return nil
}

// switchExtractionModel sets the extraction model and either reruns
// immediately (if local) or initiates a pull first.
func (m *Model) switchExtractionModel(name string, isLocal bool) tea.Cmd {
	m.ex.extractionModel = name
	m.ex.extractionClient = nil

	if isLocal {
		m.ex.extractionReady = true
		return m.rerunLLMExtraction()
	}

	// Model needs pulling -- use the same pull infrastructure.
	if m.pull.active {
		m.setStatusError("a model pull is already in progress")
		return nil
	}
	m.pull.display = "checking " + name + symEllipsis
	m.resizeTables()

	client := m.extractionLLMClient()
	if client == nil {
		return nil
	}
	timeout := client.Timeout()
	canList := client.SupportsModelListing()
	appCtx := m.lifecycleCtx()
	return func() tea.Msg {
		// Cloud providers without model listing: trust the name.
		if !canList {
			return pullProgressMsg{
				Status: "Switched to " + name,
				Done:   true,
				Model:  name,
			}
		}
		ctx, cancel := context.WithTimeout(appCtx, timeout)
		defer cancel()
		models, _ := client.ListModels(ctx) // best-effort, like chat path
		for _, model := range models {
			if model == name || strings.HasPrefix(model, name+":") {
				return pullProgressMsg{
					Status: "Switched to " + model,
					Done:   true,
					Model:  model,
				}
			}
		}
		if !client.IsLocalServer() {
			return pullProgressMsg{
				Err: fmt.Errorf(
					"model %q not found -- check the model name",
					name,
				),
				Done: true,
			}
		}
		return startPull(appCtx, client.BaseURL(), name)
	}
}

// handleExtractionExploreKey handles keys in table explore mode.
func (m *Model) handleExtractionExploreKey(msg tea.KeyPressMsg) tea.Cmd {
	ex := m.ex.extraction
	switch {
	case key.Matches(msg, m.keys.ExploreExit):
		ex.exploring = false
	case key.Matches(msg, m.keys.ExploreDown):
		g := ex.activePreviewGroup()
		if g != nil && ex.previewRow < len(g.cells)-1 {
			ex.previewRow++
		}
	case key.Matches(msg, m.keys.ExploreUp):
		if ex.previewRow > 0 {
			ex.previewRow--
		}
	case key.Matches(msg, m.keys.ExploreLeft):
		g := ex.activePreviewGroup()
		if g != nil && ex.previewCol > 0 {
			ex.previewCol--
		}
	case key.Matches(msg, m.keys.ExploreRight):
		g := ex.activePreviewGroup()
		if g != nil && ex.previewCol < len(g.specs)-1 {
			ex.previewCol++
		}
	case key.Matches(msg, m.keys.ExploreTabPrev):
		if ex.previewTab > 0 {
			ex.previewTab--
			ex.previewRow = 0
			ex.previewCol = 0
		}
	case key.Matches(msg, m.keys.ExploreTabNext):
		if ex.previewTab < len(ex.previewGroups)-1 {
			ex.previewTab++
			ex.previewRow = 0
			ex.previewCol = 0
		}
	case key.Matches(msg, m.keys.ExploreTop):
		ex.previewRow = 0
	case key.Matches(msg, m.keys.ExploreBottom):
		g := ex.activePreviewGroup()
		if g != nil && len(g.cells) > 0 {
			ex.previewRow = len(g.cells) - 1
		}
	case key.Matches(msg, m.keys.ExploreColStart):
		ex.previewCol = 0
	case key.Matches(msg, m.keys.ExploreColEnd):
		g := ex.activePreviewGroup()
		if g != nil && len(g.specs) > 0 {
			ex.previewCol = len(g.specs) - 1
		}
	case key.Matches(msg, m.keys.ExploreAccept):
		if ex.Done {
			m.acceptExtraction()
		}
	case key.Matches(msg, m.keys.MagToggle):
		m.toggleMagMode()
	}
	return nil
}

// enterExploreMode switches to table explore mode, caching operation groups.
func (ex *extractionLogState) enterExploreMode(cur locale.Currency) {
	if len(ex.previewGroups) == 0 {
		ex.previewGroups = groupOperationsByTable(ex.operations, cur)
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

// prettyJSON indents a compact JSON string.
func prettyJSON(s string) (string, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), "", "  "); err != nil {
		return "", fmt.Errorf("indent JSON: %w", err)
	}
	return buf.String(), nil
}
