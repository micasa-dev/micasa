// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"fmt"
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
	stepOCR
	stepLLM
	numExtractionSteps
)

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

	// Text sources -- kept separate so the LLM receives both with provenance.
	pdfText       string // pdftotext output (PDFs only)
	extractedText string // best available text for DB storage

	// LLM token accumulator for JSON parsing on completion.
	llmAccum strings.Builder

	// Carried between steps.
	fileData    []byte
	mime        string
	maxOCRPages int

	// Channel references for the waitFor loop pattern.
	ocrCh <-chan extract.OCRProgress
	llmCh <-chan llm.StreamChunk

	markdownRenderer

	// Which steps are active (skipped steps are simply not shown).
	hasText bool
	hasOCR  bool
	hasLLM  bool

	// Pending results held until user accepts.
	ocrText  string                   // OCR extracted text (not yet persisted)
	ocrTSV   []byte                   // OCR TSV data (not yet persisted)
	hints    *extract.ExtractionHints // parsed LLM hints (not yet persisted)
	accepted bool                     // true once user accepted results

	// Cursor and expand/collapse state for exploring output.
	cursor   int                     // index into activeSteps()
	expanded map[extractionStep]bool // manual expand/collapse overrides
}

// activeSteps returns the ordered list of steps that are shown.
func (ex *extractionLogState) activeSteps() []extractionStep {
	var steps []extractionStep
	if ex.hasText {
		steps = append(steps, stepText)
	}
	if ex.hasOCR {
		steps = append(steps, stepOCR)
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

// --- Messages ---

// extractionOCRProgressMsg delivers a single OCR progress update.
type extractionOCRProgressMsg struct {
	Progress extract.OCRProgress
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
	needsOCR := (isPDF(mime) && extract.OCRAvailable()) ||
		(extract.IsImageMIME(mime) && extract.ImageOCRAvailable())
	needsLLM := m.extractionLLMClient() != nil

	if !needsOCR && !needsLLM {
		return nil
	}

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(accent)

	ctx, cancel := context.WithCancel(context.Background())

	// Text extraction only applies to PDFs and text files; skip for images.
	hasText := !extract.IsImageMIME(mime)

	var pdfText string
	if isPDF(mime) {
		pdfText = extractedText
	}

	state := &extractionLogState{
		DocID:         docID,
		Filename:      filename,
		Spinner:       sp,
		Visible:       true,
		ctx:           ctx,
		CancelFn:      cancel,
		pdfText:       pdfText,
		extractedText: extractedText,
		fileData:      fileData,
		mime:          mime,
		maxOCRPages:   m.maxOCRPages,
		hasText:       hasText,
		hasOCR:        needsOCR,
		hasLLM:        needsLLM,
		expanded:      make(map[extractionStep]bool),
	}
	if hasText {
		nChars := len(strings.TrimSpace(extractedText))
		var textTool string
		switch {
		case isPDF(mime):
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
	if needsOCR {
		state.Steps[stepOCR].Status = stepRunning
		state.Steps[stepOCR].Started = time.Now()
		cmd = ocrExtractCmd(ctx, state)
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

// ocrExtractCmd starts the OCR pipeline in a goroutine and returns the
// first progress message via waitForOCRProgress.
func ocrExtractCmd(ctx context.Context, state *extractionLogState) tea.Cmd {
	ch := extract.OCRWithProgress(ctx, state.fileData, state.mime, state.maxOCRPages)
	state.ocrCh = ch
	return waitForOCRProgress(ch)
}

// waitForOCRProgress blocks until the next OCR progress update.
func waitForOCRProgress(ch <-chan extract.OCRProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return extractionOCRProgressMsg{
				Progress: extract.OCRProgress{Done: true},
			}
		}
		return extractionOCRProgressMsg{Progress: p}
	}
}

// llmExtractCmd starts LLM document analysis with streaming.
func (m *Model) llmExtractCmd(ctx context.Context) tea.Cmd {
	client := m.extractionLLMClient()
	if client == nil {
		return nil
	}
	store := m.store
	var ec extract.EntityContext
	if store != nil {
		vendors, projects, appliances, err := store.EntityNames()
		if err == nil {
			ec = extract.EntityContext{
				Vendors:    vendors,
				Projects:   projects,
				Appliances: appliances,
			}
		}
	}
	ex := m.extraction
	return func() tea.Msg {
		messages := extract.BuildExtractionPrompt(extract.ExtractionPromptInput{
			Filename:  ex.Filename,
			MIME:      ex.mime,
			SizeBytes: int64(len(ex.fileData)),
			Entities:  ec,
			PdfText:   ex.pdfText,
			OCRText:   ex.ocrText,
			Text:      ex.extractedText,
		})
		ch, err := client.ChatStream(ctx, messages)
		if err != nil {
			return extractionLLMChunkMsg{Err: err, Done: true}
		}
		return extractionLLMStartedMsg{Ch: ch}
	}
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

// handleExtractionOCRProgress processes an OCR progress update.
func (m *Model) handleExtractionOCRProgress(msg extractionOCRProgressMsg) tea.Cmd {
	ex := m.extraction
	if ex == nil {
		return nil
	}

	p := msg.Progress
	step := &ex.Steps[stepOCR]

	if p.Err != nil {
		step.Status = stepFailed
		step.Elapsed = time.Since(step.Started)
		step.Logs = append(step.Logs, p.Err.Error())
		ex.HasError = true
		// OCR failed but LLM can still run on whatever text was extracted.
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
		case "ocr":
			step.Detail = fmt.Sprintf("page %d/%d", p.Page, p.Total)
		}
		return waitForOCRProgress(ex.ocrCh)
	}

	// OCR done.
	step.Status = stepDone
	step.Elapsed = time.Since(step.Started)
	nChars := len(strings.TrimSpace(p.Text))
	step.Detail = "tesseract"
	step.Metric = fmt.Sprintf("%d chars", nChars)

	// Store OCR output as explorable logs.
	if nChars > 0 {
		step.Logs = strings.Split(p.Text, "\n")
	}

	// Hold OCR results for accept.
	ex.ocrText = p.Text
	ex.ocrTSV = p.TSV

	// For images (no pdftotext), OCR is the primary text source.
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
		return nil
	}

	if msg.Content != "" {
		ex.llmAccum.WriteString(msg.Content)
		step.Logs = strings.Split(ex.llmAccum.String(), "\n")
	}

	if msg.Done {
		step.Status = stepDone
		step.Elapsed = time.Since(step.Started)

		// Parse hints; hold for accept.
		response := ex.llmAccum.String()
		hints, err := extract.ParseExtractionResponse(response)
		if err != nil {
			step.Logs = append(step.Logs, "parse error: "+err.Error())
			ex.HasError = true
		} else {
			ex.hints = &hints
		}
		step.Metric = fmt.Sprintf("%d chars", len(response))

		ex.Done = true
		return nil
	}

	// More tokens coming.
	return waitForLLMChunk(ex.llmCh)
}

// applyExtractionHints saves LLM hints to the document and refreshes the table.
func (m *Model) applyExtractionHints(docID uint, hints *extract.ExtractionHints) error {
	if m.store == nil || hints == nil {
		return nil
	}
	doc, err := m.store.GetDocument(docID)
	if err != nil {
		return fmt.Errorf("load document: %w", err)
	}
	if hints.TitleSugg != "" {
		autoTitle := data.TitleFromFilename(doc.FileName)
		if doc.Title == autoTitle {
			doc.Title = hints.TitleSugg
		}
	}
	if hints.Summary != "" && doc.Notes == "" {
		doc.Notes = hints.Summary
	}
	if err := m.store.UpdateDocument(doc); err != nil {
		return fmt.Errorf("save hints: %w", err)
	}
	m.reloadAfterMutation()
	return nil
}

// acceptExtraction persists all pending results and closes the overlay.
func (m *Model) acceptExtraction() {
	ex := m.extraction
	if ex == nil || !ex.Done || ex.accepted {
		return
	}

	// Persist OCR results.
	if ex.ocrText != "" || len(ex.ocrTSV) > 0 {
		if m.store != nil {
			if err := m.store.UpdateDocumentOCR(ex.DocID, ex.ocrText, ex.ocrTSV); err != nil {
				m.setStatusError(fmt.Sprintf("save ocr: %s", err))
				return
			}
		}
	}

	// Persist LLM hints.
	if ex.hints != nil {
		if err := m.applyExtractionHints(ex.DocID, ex.hints); err != nil {
			m.setStatusError(fmt.Sprintf("save hints: %s", err))
			return
		}
	}

	ex.accepted = true
	m.extraction = nil
}

// rerunLLMExtraction resets the LLM step and re-runs it.
func (m *Model) rerunLLMExtraction() tea.Cmd {
	ex := m.extraction
	if ex == nil || !ex.hasLLM {
		return nil
	}

	// Reset LLM state.
	ex.llmAccum.Reset()
	ex.hints = nil
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

	return tea.Batch(m.llmExtractCmd(ex.ctx), ex.Spinner.Tick)
}

// --- Keyboard handler ---

// handleExtractionKey processes keys when the extraction overlay is visible.
func (m *Model) handleExtractionKey(msg tea.KeyMsg) tea.Cmd {
	ex := m.extraction
	switch msg.String() {
	case keyEsc:
		m.cancelExtraction()
	case "j", keyDown:
		active := ex.activeSteps()
		if ex.cursor < len(active)-1 {
			ex.cursor++
		}
	case "k", "up":
		if ex.cursor > 0 {
			ex.cursor--
		}
	case keyEnter:
		si := ex.cursorStep()
		ex.expanded[si] = !ex.expanded[si]
	case "r":
		if ex.Done && ex.hasLLM && ex.cursorStep() == stepLLM {
			return m.rerunLLMExtraction()
		}
	case "a":
		if ex.Done && !ex.HasError {
			m.acceptExtraction()
		}
	default:
		// Delegate scroll keys to the viewport.
		vp, cmd := ex.Viewport.Update(msg)
		ex.Viewport = vp
		return cmd
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
	ruleStyle := lipgloss.NewStyle().Foreground(border)

	// Title line.
	title := m.styles.HeaderSection.Render(" Extracting ")
	filename := m.styles.HeaderHint.Render(" " + truncateRight(ex.Filename, innerW-16))

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
		focused := i == ex.cursor
		part := m.renderExtractionStep(si, info, innerW, focused, colWidths)
		// Blank line between steps when the previous step had expanded content.
		if i > 0 && strings.Contains(stepParts[i-1], "\n") {
			lineCount++ // account for the blank separator
		}
		if i == ex.cursor {
			cursorLine = lineCount
		}
		lineCount += strings.Count(part, "\n") + 1
		stepParts = append(stepParts, part)
	}
	// Join: insert a blank line after multi-line (expanded) parts.
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

	// Size the viewport.
	maxH := m.effectiveHeight()*2/3 - 6 // border + padding + title + rule + hints
	if maxH < 6 {
		maxH = 6
	}
	contentLines := strings.Count(stepContent, "\n") + 1
	vpH := contentLines
	if vpH > maxH {
		vpH = maxH
	}

	ex.Viewport.Width = innerW
	ex.Viewport.Height = vpH
	ex.Viewport.SetContent(stepContent)

	// Auto-scroll to keep the cursor step header visible.
	if vpH < contentLines {
		yOff := ex.Viewport.YOffset
		if cursorLine < yOff {
			ex.Viewport.SetYOffset(cursorLine)
		} else if cursorLine >= yOff+vpH {
			ex.Viewport.SetYOffset(cursorLine - vpH + 1)
		}
	}

	vpView := ex.Viewport.View()

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

	// Hint line.
	var hints []string
	if ex.Done {
		hints = append(hints, m.helpItem("j/k", "navigate"), m.helpItem("\u21b5", "expand"))
		if !ex.HasError {
			hints = append(hints, m.helpItem("a", "accept"))
		}
		hints = append(hints, m.helpItem("esc", "discard"))
	} else {
		hints = append(hints, m.helpItem("esc", "hide"))
	}
	hintStr := joinWithSeparator(m.helpSeparator(), hints...)

	boxContent := lipgloss.JoinVertical(lipgloss.Left,
		title+filename,
		"",
		vpView,
		rule,
		hintStr,
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2).
		Width(contentW).
		Render(boxContent)
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

	// Cursor indicator: right triangle when collapsed, down when expanded.
	cursor := "  "
	if focused && ex.Done {
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

	// Render log content: JSON gets syntax highlighting via glamour,
	// everything else is plain dim text.
	var rendered string
	switch {
	case si == stepLLM && (info.Status == stepDone || info.Status == stepRunning):
		json := strings.TrimSpace(extract.StripCodeFences(raw))
		rendered = strings.TrimSpace(ex.renderMarkdown("```json\n"+json+"\n```", logW))
	default:
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
	case stepOCR:
		return "ocr"
	case stepLLM:
		return "llm"
	case numExtractionSteps:
		return "?"
	}
	return "?"
}

func isPDF(mime string) bool {
	return mime == "application/pdf"
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

// --- Layout helpers ---

func (m *Model) extractionOverlayWidth() int {
	w := m.effectiveWidth() - 8
	if w > 80 {
		w = 80
	}
	if w < 50 {
		w = 50
	}
	return w
}
