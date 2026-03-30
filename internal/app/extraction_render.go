// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/micasa-dev/micasa/internal/config"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/extract"
	"github.com/micasa-dev/micasa/internal/locale"
)

// buildExtractionOverlay renders the extraction progress overlay.
func (m *Model) buildExtractionOverlay() string {
	ex := m.ex.extraction
	if ex == nil {
		return ""
	}

	contentW := m.extractionOverlayWidth()
	innerW := contentW - m.styles.OverlayBox().GetHorizontalFrameSize()

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

	ex.Viewport.SetWidth(innerW)
	ex.Viewport.SetHeight(vpH)
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
			yOff := ex.Viewport.YOffset()
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

	rule := m.scrollRule(innerW, ex.Viewport.TotalLineCount(), ex.Viewport.Height(),
		ex.Viewport.AtTop(), ex.Viewport.AtBottom(), ex.Viewport.ScrollPercent(), symHLine)

	// Model picker section (shown between viewport and hints when active).
	pickerSection := ""
	if ex.modelPicker != nil {
		filterLine := m.styles.HeaderHint().Render("model ") +
			m.styles.Base().Render(ex.modelFilter) +
			m.styles.BlinkCursor().Render("\u2502")
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
			{data.ColPhone, s[4], fmtPhone},
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

func fmtPhone(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmtAnyText(v)
	}
	return locale.FormatPhoneNumber(s, strings.ToUpper(config.DetectCountry()))
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
