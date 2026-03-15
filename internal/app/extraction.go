// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/extract"
	"github.com/cpcloud/micasa/internal/llm"
	"github.com/cpcloud/micasa/internal/locale"
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
	ID          uint64
	DocID       uint
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

	markdownRenderer

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
	docID uint,
	filename string,
	fileData []byte,
	mime string,
	extractedText string,
	extractData []byte,
) tea.Cmd {
	needsExtract := extract.NeedsOCR(m.ex.extractors, mime)
	needsLLM := m.extractionLLMClient() != nil

	// Surface cached client creation errors so misconfigured providers
	// don't silently skip LLM extraction.
	if !needsLLM && m.ex.extractionClientErr != nil {
		m.setStatusError("extraction LLM: " + m.ex.extractionClientErr.Error())
	}

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
		context.Background(),
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
	for _, bg := range m.ex.bgExtractions {
		if bg == ex {
			return true
		}
	}
	return false
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
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), quickOpTimeout)
		defer cancel()
		err := client.Ping(ctx)
		return extractionLLMPingMsg{ID: id, Err: err}
	}
}

// llmExtractCmd starts LLM document analysis with streaming.
func (m *Model) llmExtractCmd(ctx context.Context, ex *extractionLogState) tea.Cmd {
	client := m.extractionLLMClient()
	if client == nil {
		return nil
	}
	schemaCtx := m.buildSchemaContext()
	id := ex.ID
	timeout := m.ex.extractionTimeout
	return func() tea.Msg {
		llmCtx := ctx
		if timeout > 0 {
			var cancel context.CancelFunc
			llmCtx, cancel = context.WithTimeout(ctx, timeout)
			ex.llmCancelFn = cancel
		}
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
		ch, err := client.ChatStream(
			llmCtx,
			messages,
			llm.WithJSONSchema("extraction_operations", extract.OperationsSchema()),
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
			m.setStatusError(fmt.Sprintf("Extraction failed: %s", ex.Filename))
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
		m.setStatusInfo(fmt.Sprintf("Extracted: %s", ex.Filename))
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
func (m *Model) handleExtractionLLMPing(msg extractionLLMPingMsg) tea.Cmd {
	ex := m.findExtraction(msg.ID)
	if ex == nil {
		return nil
	}
	ex.llmPingDone = true

	if msg.Err != nil {
		// Providers that don't support model listing (e.g. Anthropic)
		// return ErrPingNotSupported. Treat this as "proceed
		// optimistically" -- the real error will surface when
		// ChatStream is attempted.
		if errors.Is(msg.Err, llm.ErrPingNotSupported) {
			ex.llmPingErr = nil
			return nil
		}

		ex.llmPingErr = msg.Err

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
	return nil
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
			m.setStatusError(fmt.Sprintf("Extraction failed: %s", ex.Filename))
		}
		return nil
	}

	if msg.Content != "" {
		ex.llmAccum.WriteString(msg.Content)
		step.Logs = strings.Split(ex.llmAccum.String(), "\n")
	}

	if msg.Done {
		ex.cancelLLMTimeout()
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
				m.setStatusError(fmt.Sprintf("Extraction failed: %s", ex.Filename))
			} else {
				m.setStatusInfo(fmt.Sprintf("Extracted: %s", ex.Filename))
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
				if n := extract.ParseUint(v); n > 0 {
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
	if m.store != nil && ex.DocID > 0 {
		if err := m.store.EnsureDocumentAlive(ex.DocID); err != nil {
			return fmt.Errorf("document was deleted and could not be restored: %w", err)
		}
	}

	// Persist async extraction results and the model that produced them.
	if ex.pendingText != "" || len(ex.pendingData) > 0 || ex.hasLLM {
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
		return fmt.Errorf("no shadow DB: operations were not staged")
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
			context.Background(),
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
func (m *Model) handleExtractionKey(msg tea.KeyMsg) tea.Cmd {
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
func (m *Model) handleExtractionPipelineKey(msg tea.KeyMsg) tea.Cmd {
	ex := m.ex.extraction
	switch msg.String() {
	case keyEsc:
		m.cancelExtraction()
	case keyCtrlC:
		m.interruptExtraction()
	case keyJ, keyDown:
		ex.cursorManual = true
		overflow := ex.Viewport.TotalLineCount() > ex.Viewport.Height
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
	case keyK, keyUp:
		ex.cursorManual = true
		overflow := ex.Viewport.TotalLineCount() > ex.Viewport.Height
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
	case keyEnter:
		si := ex.cursorStep()
		// Only toggle from the parent header, not from a child tool line.
		if si != stepExtract || len(ex.acquireTools) == 0 || ex.toolCursor == -1 {
			ex.expanded[si] = !ex.stepExpanded(si)
		}
	case keyR:
		if ex.Done && ex.hasLLM && ex.cursorStep() == stepLLM {
			return m.activateExtractionModelPicker()
		}
	case keyT:
		if ex.Done && ex.hasLLM {
			return m.toggleExtractionTSV()
		}
	case keyA:
		if ex.Done {
			m.acceptExtraction()
		}
	case keyX:
		if ex.Done && len(ex.operations) > 0 {
			ex.enterExploreMode(m.cur)
		}
	case keyCtrlB:
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
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		models, err := client.ListModels(ctx)
		return modelsListMsg{Models: models, Err: err}
	}
}

// handleExtractionModelPickerKey handles keys when the extraction model
// picker is showing.
func (m *Model) handleExtractionModelPickerKey(msg tea.KeyMsg) tea.Cmd {
	ex := m.ex.extraction
	mc := ex.modelPicker
	switch msg.String() {
	case keyEsc:
		ex.modelPicker = nil
		ex.modelFilter = ""
	case keyUp, keyCtrlP:
		if mc.Cursor > 0 {
			mc.Cursor--
		}
	case keyDown, keyCtrlN:
		if mc.Cursor < len(mc.Matches)-1 {
			mc.Cursor++
		}
	case keyEnter:
		if len(mc.Matches) > 0 {
			selected := mc.Matches[mc.Cursor].Name
			isLocal := mc.Matches[mc.Cursor].Local
			ex.modelPicker = nil
			ex.modelFilter = ""
			return m.switchExtractionModel(selected, isLocal)
		}
		ex.modelPicker = nil
		ex.modelFilter = ""
	case keyBackspace:
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
	m.ex.extractionClientErr = nil

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
	return func() tea.Msg {
		// Cloud providers without model listing: trust the name.
		if !canList {
			return pullProgressMsg{
				Status: "Switched to " + name,
				Done:   true,
				Model:  name,
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		models, listErr := client.ListModels(ctx)
		if listErr != nil {
			return pullProgressMsg{
				Err:  fmt.Errorf("cannot switch extraction model: %w", listErr),
				Done: true,
			}
		}
		for _, model := range models {
			if model == name || strings.HasPrefix(model, name+":") {
				return pullProgressMsg{
					Status: "Switched to " + model,
					Done:   true,
					Model:  model,
				}
			}
		}
		return startPull(client.BaseURL(), name)
	}
}

// handleExtractionExploreKey handles keys in table explore mode.
func (m *Model) handleExtractionExploreKey(msg tea.KeyMsg) tea.Cmd {
	ex := m.ex.extraction
	switch msg.String() {
	case keyEsc:
		ex.exploring = false
	case keyJ, keyDown:
		g := ex.activePreviewGroup()
		if g != nil && ex.previewRow < len(g.cells)-1 {
			ex.previewRow++
		}
	case keyK, keyUp:
		if ex.previewRow > 0 {
			ex.previewRow--
		}
	case keyH, keyLeft:
		g := ex.activePreviewGroup()
		if g != nil && ex.previewCol > 0 {
			ex.previewCol--
		}
	case keyL, keyRight:
		g := ex.activePreviewGroup()
		if g != nil && ex.previewCol < len(g.specs)-1 {
			ex.previewCol++
		}
	case keyB:
		if ex.previewTab > 0 {
			ex.previewTab--
			ex.previewRow = 0
			ex.previewCol = 0
		}
	case keyF:
		if ex.previewTab < len(ex.previewGroups)-1 {
			ex.previewTab++
			ex.previewRow = 0
			ex.previewCol = 0
		}
	case keyG:
		ex.previewRow = 0
	case keyShiftG:
		g := ex.activePreviewGroup()
		if g != nil && len(g.cells) > 0 {
			ex.previewRow = len(g.cells) - 1
		}
	case keyCaret:
		ex.previewCol = 0
	case keyDollar:
		g := ex.activePreviewGroup()
		if g != nil && len(g.specs) > 0 {
			ex.previewCol = len(g.specs) - 1
		}
	case keyA:
		if ex.Done {
			m.acceptExtraction()
		}
	case keyX:
		ex.exploring = false
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

// --- Rendering ---

// buildExtractionOverlay renders the extraction progress overlay.
func (m *Model) buildExtractionOverlay() string {
	ex := m.ex.extraction
	if ex == nil {
		return ""
	}

	contentW := m.extractionOverlayWidth()
	innerW := contentW - 4 // padding

	// Title line.
	title := m.styles.HeaderSection().Render(" Extracting ")
	filename := m.styles.HeaderHint().Render(" " + truncateRight(ex.Filename, innerW-16))

	return m.buildExtractionPipelineOverlay(contentW, innerW, title+filename)
}

// previewNaturalWidth returns the minimum inner width needed to display
// all preview tables without wrapping. Returns 0 if there are no groups.
func previewNaturalWidth(groups []previewTableGroup, sepW int, currencySymbol string) int {
	var maxW int
	for _, g := range groups {
		nw := naturalWidths(g.specs, g.cells, currencySymbol)
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
	ex := m.ex.extraction
	ruleStyle := appStyles.Rule()

	// Compute column widths across all active steps for alignment.
	active := ex.activeSteps()
	var maxDetailW, maxMetricW, maxElapsedW int
	for _, si := range active {
		info := ex.Steps[si]
		// When tools are present, the parent shows "ocr" as detail;
		// child tool names use their own sub-column width.
		if si == stepExtract && len(ex.acquireTools) > 0 {
			if w := len("ocr"); w > maxDetailW {
				maxDetailW = w
			}
		} else if w := len(info.Detail); w > maxDetailW {
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
		if i == ex.cursor {
			cursorLine = lineCount
			// Offset within ext: parent is line 0, children start at 1.
			if si == stepExtract && len(ex.acquireTools) > 0 && ex.toolCursor >= 0 {
				cursorLine += 1 + ex.toolCursor
			}
		}
		lineCount += strings.Count(part, "\n") + 1
		stepParts = append(stepParts, part)
	}
	var stepBuf strings.Builder
	for i, part := range stepParts {
		if i > 0 {
			stepBuf.WriteByte('\n')
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

	// When content fits entirely, reset any stale scroll offset so the
	// top of the pipeline stays visible (e.g. after collapsing a step).
	if contentLines <= vpH {
		ex.Viewport.SetYOffset(0)
	}

	if vpH < contentLines && !ex.exploring {
		si := ex.cursorStep()
		streaming := ex.Steps[si].Status == stepRunning

		switch {
		case streaming:
			// Follow the growing output so the user sees new tokens.
			ex.Viewport.GotoBottom()
		case ex.stepExpanded(si):
			// Cursor step expanded: user may be scrolling, don't reposition.
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
		vpView = appStyles.TextDim().Render(vpView)
	}

	rule := m.scrollRule(innerW, ex.Viewport.TotalLineCount(), ex.Viewport.Height,
		ex.Viewport.AtTop(), ex.Viewport.AtBottom(), ex.Viewport.ScrollPercent(), symHLine)

	// Model picker section (shown between viewport and hints when active).
	pickerSection := ""
	if ex.modelPicker != nil {
		filterLine := m.styles.HeaderHint().Render("model ") +
			m.styles.Base().Render(ex.modelFilter) +
			m.styles.BlinkCursor().Render("\u2588")
		list := m.renderModelCompleterFor(ex.modelPicker, ex.modelFilter, innerW)
		pickerSection = filterLine + "\n" + list
	}

	// Hint line varies by mode.
	var hints []string
	if ex.modelPicker != nil {
		hints = append(hints,
			m.helpItem(symUp+"/"+symDown, "navigate"),
			m.helpItem(symReturn, "select"),
			m.helpItem(keyEsc, "cancel"),
		)
	} else if ex.exploring {
		hints = append(hints, m.helpItem(keyJ+"/"+keyK, "rows"), m.helpItem(keyH+"/"+keyL, "cols"))
		if len(ex.previewGroups) > 1 {
			hints = append(hints, m.helpItem(keyB+"/"+keyF, "tabs"))
		}
		hints = append(hints, m.helpItem(keyA, "accept"), m.helpItem(keyX, "back"), m.helpItem(keyEsc, "discard"))
	} else {
		hints = append(hints, m.helpItem(keyJ+"/"+keyK, "navigate"))
		cursorStatus := ex.Steps[ex.cursorStep()].Status
		if cursorStatus != stepPending {
			hints = append(hints, m.helpItem(symReturn, "expand"))
		}
		if hasOps {
			hints = append(hints, m.helpItem(keyX, "explore"))
		}
		if ex.Done {
			if ex.hasLLM {
				label := "layout on"
				if m.ex.ocrTSV {
					label = "layout off"
				}
				hints = append(hints, m.helpItem(keyT, label))
			}
			hints = append(hints, m.helpItem(keyA, "accept"), m.helpItem(keyEsc, "discard"))
		} else {
			hints = append(hints,
				m.helpItem(symCtrlC, "int"),
				m.helpItem(symCtrlB, "bg"),
				m.helpItem(keyEsc, "cancel"),
			)
		}
	}
	hintStr := joinWithSeparator(m.helpSeparator(), hints...)

	parts := []string{titleLine, "", vpView, rule}
	if pickerSection != "" {
		parts = append(parts, "", pickerSection)
	} else if previewSection != "" {
		parts = append(parts, "", previewSection)
	}
	parts = append(parts, ruleStyle.Render(strings.Repeat(symHLine, innerW)), hintStr)
	boxContent := lipgloss.JoinVertical(lipgloss.Left, parts...)

	return m.styles.OverlayBox().
		Width(contentW).
		Render(boxContent)
}

// renderOperationPreviewSection renders the operation preview table section.
// When interactive is true, the row/col cursors are shown and the section
// renders at full brightness. When false, the entire section is dimmed.
func (m *Model) renderOperationPreviewSection(innerW int, interactive bool) string {
	ex := m.ex.extraction
	if len(ex.previewGroups) == 0 {
		ex.previewGroups = groupOperationsByTable(ex.operations, m.cur)
	}
	groups := ex.previewGroups
	if len(groups) == 0 {
		return appStyles.TextDim().Render("no operations")
	}

	sep := m.styles.TableSeparator().Render(" " + symVLine + " ")
	divSep := m.styles.TableSeparator().Render(symHLine + symCross + symHLine)
	sepW := lipgloss.Width(sep)

	// Tab bar: active tab highlighted in explore mode, all dimmed otherwise.
	tabParts := make([]string, 0, len(groups)*2)
	for i, g := range groups {
		var rendered string
		if interactive && i == ex.previewTab {
			rendered = m.styles.TabActive().Render(g.name)
		} else {
			rendered = m.styles.TabInactive().Render(g.name)
		}
		tabParts = append(tabParts, m.zones.Mark(fmt.Sprintf("%s%d", zoneExtTab, i), rendered))
		if i < len(groups)-1 {
			tabParts = append(tabParts, "   ")
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Left, tabParts...)
	underline := m.styles.TabUnderline().Render(strings.Repeat(symHLineHeavy, innerW))

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
		result = appStyles.TextDim().Render(result)
	}
	return result
}

// renderPreviewTable renders a single table group with header, divider, and rows.
func (m *Model) renderPreviewTable(
	g previewTableGroup, innerW, sepW int, sep, divSep string, interactive bool,
) string {
	ex := m.ex.extraction
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
		g.specs, widths, seps, colCursor, nil, false, false, g.cells, m.zones, zoneExtCol,
	)
	divider := renderDivider(widths, seps, divSep, m.styles.TableSeparator())

	rowCursor := -1
	if interactive {
		rowCursor = ex.previewRow
		if rowCursor >= len(g.cells) {
			rowCursor = len(g.cells) - 1
		}
	}
	rows := renderRows(
		g.specs, g.cells, g.meta, widths,
		seps, seps, rowCursor, colCursor, 0, pinRenderContext{}, m.zones, zoneExtRow,
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
	ex := m.ex.extraction
	hint := m.styles.HeaderHint()

	var icon string
	var nameStyle lipgloss.Style
	switch info.Status {
	case stepPending:
		icon = "  "
		nameStyle = m.styles.ExtPending()
	case stepRunning:
		icon = ex.Spinner.View() + " "
		nameStyle = m.styles.ExtRunning()
	case stepDone:
		icon = m.styles.ExtOk().Render("ok") + " "
		nameStyle = m.styles.ExtDone()
	case stepFailed:
		icon = m.styles.ExtFail().Render("xx") + " "
		nameStyle = m.styles.ExtFailed()
	case stepSkipped:
		icon = m.styles.ExtPending().Render("na") + " "
		nameStyle = m.styles.ExtPending()
	}

	hasTools := si == stepExtract && len(ex.acquireTools) > 0
	expanded := ex.stepExpanded(si)

	// Cursor indicator: show on any non-pending step so the user can
	// track focus during streaming and inspect completed steps.
	// Auto-follow mode uses dim triangles; manual mode uses bright ones.
	// For ext with tools, cursor only shows on parent when toolCursor == -1.
	cursor := "  "
	showParentCursor := focused && info.Status != stepPending &&
		(!hasTools || ex.toolCursor == -1)
	if showParentCursor {
		cursorStyle := m.styles.ExtPending()
		if ex.cursorManual {
			cursorStyle = m.styles.ExtCursor()
		}
		if expanded {
			cursor = cursorStyle.Render(symTriDownSm + " ")
		} else {
			cursor = cursorStyle.Render(symTriRightSm + " ")
		}
	}

	// Columnar header: icon | name | detail | metric | elapsed [| rerun hint].
	var hdr strings.Builder
	hdr.WriteString(cursor)
	hdr.WriteString(icon)
	hdr.WriteString(nameStyle.Render(fmt.Sprintf("%-4s", name)))
	if cols.Detail > 0 {
		detail := info.Detail
		if hasTools {
			detail = "ocr"
		}
		hdr.WriteString("  ")
		hdr.WriteString(hint.Render(fmt.Sprintf("%-*s", cols.Detail, detail)))
	}
	if hasTools {
		// Parent metric: combined pipeline percentage.
		// Each tool is an equal-weight stage, so
		// pct = sum(tool.Count) / (total * numTools) * 100.
		var pct int
		if denom := ex.extractedPages * len(ex.acquireTools); denom > 0 {
			var sumCount int
			for _, ts := range ex.acquireTools {
				sumCount += ts.Count
			}
			pct = sumCount * 100 / denom
		}
		hdr.WriteString("  ")
		hdr.WriteString(m.styles.ExtDone().Render(fmt.Sprintf("%d%%", pct)))
	} else if cols.Metric > 0 {
		hdr.WriteString("  ")
		hdr.WriteString(hint.Render(fmt.Sprintf("%*s", cols.Metric, info.Metric)))
	}
	if cols.Elapsed > 0 {
		var e string
		switch {
		case info.Elapsed > 0:
			e = fmt.Sprintf("%.2fs", info.Elapsed.Seconds())
		case info.Status == stepRunning && !info.Started.IsZero():
			e = fmt.Sprintf("%.1fs", time.Since(info.Started).Seconds())
		}
		hdr.WriteString("  ")
		hdr.WriteString(hint.Render(fmt.Sprintf("%*s", cols.Elapsed, e)))
	}
	llmTerminal := info.Status == stepDone || info.Status == stepFailed ||
		info.Status == stepSkipped
	if si == stepLLM && llmTerminal && ex.Done && focused && ex.modelPicker == nil {
		hdr.WriteString("  ")
		hdr.WriteString(m.styles.ExtRerun().Render("r model"))
	}
	header := hdr.String()

	// Render parent + children for the ext step.
	// Children only show when expanded; logs beneath children.
	if hasTools {
		var b strings.Builder
		b.WriteString(header)

		if expanded {
			// Compute max tool name width for child column alignment.
			maxToolW := 0
			for _, ts := range ex.acquireTools {
				if w := len(ts.Tool); w > maxToolW {
					maxToolW = w
				}
			}

			dim := m.styles.ExtPending()
			childIndent := "   "
			for ti, ts := range ex.acquireTools {
				b.WriteByte('\n')
				b.WriteString(childIndent)

				// Child cursor triangle (always right-pointing; children
				// don't individually expand).
				childCursor := "   "
				if focused && ti == ex.toolCursor {
					cursorStyle := m.styles.ExtPending()
					if ex.cursorManual {
						cursorStyle = m.styles.ExtCursor()
					}
					childCursor = cursorStyle.Render(symTriRightSm) + "  "
				}
				b.WriteString(childCursor)

				// Per-tool status icon and style.
				isTerminal := ti == len(ex.acquireTools)-1
				var toolIcon string
				var toolNameStyle lipgloss.Style
				switch {
				case ts.Running:
					toolIcon = ex.Spinner.View() + " "
					if isTerminal {
						toolNameStyle = m.styles.ExtRunning()
					} else {
						toolIcon = dim.Render(ex.Spinner.View()) + " "
						toolNameStyle = dim
					}
				case ts.Err != nil:
					if isTerminal {
						toolIcon = m.styles.ExtFail().Render("xx") + " "
						toolNameStyle = m.styles.ExtFailed()
					} else {
						toolIcon = dim.Render("xx") + " "
						toolNameStyle = dim
					}
				default:
					if isTerminal {
						toolIcon = m.styles.ExtOk().Render("ok") + " "
						toolNameStyle = m.styles.ExtDone()
					} else {
						toolIcon = dim.Render("ok") + " "
						toolNameStyle = dim
					}
				}

				b.WriteString(toolIcon)
				b.WriteString(toolNameStyle.Render(fmt.Sprintf("%-*s", maxToolW, ts.Tool)))

				if ts.Count > 0 || !ts.Running {
					b.WriteString("  ")
					if isTerminal {
						b.WriteString(m.renderPageRatio(ts.Count, ex.extractedPages, ex.docPages))
					} else {
						b.WriteString(dim.Render(fmt.Sprintf("%d/%d pp", ts.Count, ex.extractedPages)))
					}
				}
			}

			// Log content beneath children.
			if len(info.Logs) > 0 {
				pipeIndent := "      "
				pipe := m.styles.TableSeparator().Render(symVLine) + " "
				logW := innerW - len(pipeIndent) - 2
				raw := strings.Join(info.Logs, "\n")
				rendered := m.styles.HeaderHint().Render(wordWrap(raw, logW))
				for _, line := range strings.Split(rendered, "\n") {
					b.WriteByte('\n')
					b.WriteString(pipeIndent)
					b.WriteString(pipe)
					b.WriteString(line)
				}
			}
		}

		return b.String()
	}

	if !expanded || len(info.Logs) == 0 {
		return header
	}

	// Expanded: header + rendered log content with left border pipe.
	pipeIndent := "     " // align pipe under step name
	pipe := m.styles.TableSeparator().Render(symVLine) + " "
	logW := innerW - len(pipeIndent) - 2 // pipe + space
	raw := strings.Join(info.Logs, "\n")

	var rendered string
	if si == stepLLM && info.Status != stepSkipped && info.Status != stepFailed {
		// Pretty-print JSON, then render as a fenced code block via glamour.
		formatted := raw
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(extract.StripCodeFences(raw)), "", "  "); err == nil {
			formatted = buf.String()
		}
		md := fmt.Sprintf("```json\n%s\n```", formatted)
		rendered = strings.TrimSpace(ex.renderMarkdown(md, logW))
	} else if info.Status == stepFailed {
		rendered = m.styles.ExtFail().Render(wordWrap(raw, logW))
	} else if info.Status == stepSkipped {
		rendered = m.styles.ExtSkipLog().Render(wordWrap(raw, logW))
	} else {
		rendered = m.styles.HeaderHint().Render(wordWrap(raw, logW))
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

// renderPageRatio formats a page progress indicator with differentiated
// colors: count (bright), limit and total (dim). When docPages is 0 (no
// cap), shows "count/total pg". When capped, shows "count/limit/total pg".
func (m *Model) renderPageRatio(count, limit, docPages int) string {
	sep := m.styles.ExtPending().Render("/")
	hint := m.styles.HeaderHint()
	bright := m.styles.ExtDone()
	dim := m.styles.ExtPending()
	countStr := bright.Render(fmt.Sprintf("%d", count))
	if docPages > 0 {
		return countStr + sep +
			hint.Render(fmt.Sprintf("%d", limit)) + sep +
			dim.Render(fmt.Sprintf("%d", docPages)) +
			dim.Render(" pp")
	}
	total := limit
	if total == 0 {
		total = count
	}
	return countStr + sep +
		hint.Render(fmt.Sprintf("%d", total)) +
		dim.Render(" pp")
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

// marshalOps serialises extraction operations to JSON for persistence.
// A nil slice (LLM didn't run / failed) returns nil so callers skip
// the update. A non-nil but empty slice (LLM ran, zero ops) returns
// "[]" so stale data is cleared.
func marshalOps(ops []extract.Operation) ([]byte, error) {
	if ops == nil {
		return nil, nil
	}
	b, err := json.Marshal(ops)
	if err != nil {
		return nil, fmt.Errorf("marshal ops: %w", err)
	}
	return b, nil
}

// extractionModelUsed returns the model name if the LLM step completed
// successfully, or empty string if LLM was skipped or failed.
func (m *Model) extractionModelUsed(ex *extractionLogState) string {
	if ex.hasLLM && ex.Steps[stepLLM].Status == stepDone {
		return m.extractionModelLabel()
	}
	return ""
}

// extractionModelLabel returns the model name used for extraction.
func (m *Model) extractionModelLabel() string {
	if m.ex.extractionModel != "" {
		return m.ex.extractionModel
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
func previewColumns(tableName string, cur locale.Currency) []previewColDef {
	fmtAnyCents := func(v any) string {
		if val, ok := v.(float64); ok {
			return cur.FormatCents(int64(val))
		}
		return fmtAnyText(v)
	}
	switch tableName {
	case data.TableVendors:
		s := vendorColumnSpecs()
		return []previewColDef{
			{data.ColName, s[1], fmtAnyText},
			{data.ColContactName, s[2], fmtAnyText},
			{data.ColEmail, s[3], fmtAnyText},
			{data.ColPhone, s[4], fmtAnyText},
			{data.ColWebsite, s[5], fmtAnyText},
		}
	case tableDocuments:
		s := documentColumnSpecs()
		return []previewColDef{
			{data.ColTitle, s[documentColTitle], fmtAnyText},
			{data.ColMIMEType, s[documentColType], fmtAnyText},
			{data.ColNotes, s[documentColNotes], fmtAnyText},
		}
	case data.TableQuotes:
		s := quoteColumnSpecs()
		return []previewColDef{
			{data.ColProjectID, s[1], fmtAnyFK},
			{data.ColVendorID, s[2], fmtAnyFK},
			{data.ColTotalCents, s[3], fmtAnyCents},
			{data.ColLaborCents, s[4], fmtAnyCents},
			{data.ColMaterialsCents, s[5], fmtAnyCents},
			{data.ColOtherCents, s[6], fmtAnyCents},
			{data.ColReceivedDate, s[7], fmtAnyText},
		}
	case data.TableMaintenanceItems:
		s := maintenanceColumnSpecs()
		return []previewColDef{
			{data.ColName, s[1], fmtAnyText},
			{data.ColCategoryID, s[2], fmtAnyFK},
			{data.ColApplianceID, s[3], fmtAnyFK},
			{data.ColIntervalMonths, s[6], fmtAnyInterval},
		}
	case data.TableAppliances:
		s := applianceColumnSpecs()
		return []previewColDef{
			{data.ColName, s[1], fmtAnyText},
			{data.ColBrand, s[2], fmtAnyText},
			{data.ColModelNumber, s[3], fmtAnyText},
			{data.ColSerialNumber, s[4], fmtAnyText},
			{data.ColLocation, s[5], fmtAnyText},
			{data.ColPurchaseDate, s[6], fmtAnyText},
			{data.ColWarrantyExpiry, s[8], fmtAnyText},
			{data.ColCostCents, s[9], fmtAnyCents},
		}
	default:
		return nil
	}
}

// previewTabName maps a DB table name to the display name used in the tab bar.
var previewTabName = map[string]string{
	tableDocuments:             "Docs",
	data.TableVendors:          "Vendors",
	data.TableQuotes:           "Quotes",
	data.TableMaintenanceItems: "Maintenance",
	data.TableAppliances:       "Appliances",
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
func groupOperationsByTable(ops []extract.Operation, cur locale.Currency) []previewTableGroup {
	// Preserve first-seen order.
	var order []string
	groups := make(map[string]*previewTableGroup)

	for _, op := range ops {
		allDefs := previewColumns(op.Table, cur)
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
		allDefs := previewColumns(op.Table, cur)
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
	ex := m.ex.extraction
	if ex != nil && len(ex.operations) > 0 {
		if len(ex.previewGroups) == 0 {
			ex.previewGroups = groupOperationsByTable(ex.operations, m.cur)
		}
		sep := m.styles.TableSeparator().Render(" " + symVLine + " ")
		sepW := lipgloss.Width(sep)
		needed := previewNaturalWidth(
			ex.previewGroups,
			sepW,
			m.cur.Symbol(),
		) + 4 // +4 for padding
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
