// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/cpcloud/micasa/internal/data"
)

// defaultStyle is reused for cells that need no special styling, avoiding
// a lipgloss.NewStyle() allocation per cell per render.
var defaultStyle = appStyles.Base()

const (
	linkArrow                 = "→"   // FK link to another tab
	drilldownArrow            = "↘"   // opens a detail sub-table
	breadcrumbSep             = " › " // delimiter used in breadcrumb strings
	filterMarkActive          = "▼"   // filled down: active filter, normal
	filterMarkActiveInverted  = "▲"   // filled up: active filter, inverted
	filterMarkPreview         = "▽"   // hollow down: preview (pinned), normal
	filterMarkPreviewInverted = "△"   // hollow up: preview (pinned), inverted
)

// visibleProjection computes the visible-only view of a tab's columns and data.
// It returns projected specs, cell rows, the translated column cursor (-1 if
// the real cursor is hidden), remapped sort entries, and the vis-to-full index map.
func visibleProjection(tab *Tab) (
	specs []columnSpec,
	cellRows [][]cell,
	colCursor int,
	sorts []sortEntry,
	visToFull []int,
) {
	fullToVis := make(map[int]int, len(tab.Specs))
	for i, spec := range tab.Specs {
		if spec.HideOrder > 0 {
			continue
		}
		fullToVis[i] = len(visToFull)
		visToFull = append(visToFull, i)
		specs = append(specs, spec)
	}

	colCursor = -1
	if vis, ok := fullToVis[tab.ColCursor]; ok {
		colCursor = vis
	}

	cellRows = make([][]cell, len(tab.CellRows))
	for r, row := range tab.CellRows {
		projected := make([]cell, 0, len(visToFull))
		for _, fi := range visToFull {
			if fi < len(row) {
				projected = append(projected, row[fi])
			}
		}
		cellRows[r] = projected
	}

	for _, se := range tab.Sorts {
		if vis, ok := fullToVis[se.Col]; ok {
			sorts = append(sorts, sortEntry{Col: vis, Dir: se.Dir})
		}
	}
	return
}

func renderHeaderRow(
	specs []columnSpec,
	widths []int,
	separators []string,
	colCursor int,
	sorts []sortEntry,
	hasLeft, hasRight bool,
	rows [][]cell,
) string {
	cells := make([]string, 0, len(specs))
	last := len(specs) - 1
	arrow := appStyles.SortArrow()
	for i, spec := range specs {
		width := safeWidth(widths, i)
		title := spec.Title
		if (spec.Link != nil || spec.Kind == cellEntity) && columnHasLinks(rows, i) {
			title = title + " " + appStyles.LinkIndicator().Render(linkArrow)
		} else if spec.Kind == cellDrilldown {
			title = title + " " + appStyles.LinkIndicator().Render(drilldownArrow)
		}
		// Scroll arrows embedded in edge column headers.
		if i == 0 && hasLeft {
			title = arrow.Render("◀") + " " + title
		}
		if i == last && hasRight {
			title = title + " " + arrow.Render("▶")
		}
		indicator := sortIndicator(sorts, i)
		text := formatHeaderCell(title, indicator, width)
		if i == colCursor {
			cells = append(cells, appStyles.ColActiveHeader().Render(text))
		} else {
			cells = append(cells, appStyles.TableHeader().Render(text))
		}
	}
	return joinCells(cells, separators)
}

// columnHasLinks reports whether any row in the given column has a non-zero
// LinkID. Used to decide whether to show the → arrow in the header.
func columnHasLinks(rows [][]cell, col int) bool {
	for _, row := range rows {
		if col < len(row) && row[col].LinkID > 0 {
			return true
		}
	}
	return false
}

type tableViewport struct {
	HasLeft       bool
	HasRight      bool
	Specs         []columnSpec
	Cells         [][]cell
	LinkCells     [][]cell // unfiltered rows for link-arrow presence check
	Widths        []int
	PlainSeps     []string
	CollapsedSeps []string
	Cursor        int
	Sorts         []sortEntry
	VisToFull     []int // viewport column index → full tab.Specs index
}

func computeTableViewport(
	tab *Tab,
	termWidth int,
	normalSep string,
	currencySymbol string,
) tableViewport {
	var vp tableViewport
	if tab == nil {
		return vp
	}
	visSpecs, visCells, visColCursor, visSorts, visToFull := visibleProjection(tab)
	if len(visSpecs) == 0 {
		return vp
	}

	// When pins are active, use unfiltered data for natural widths so that
	// activating/deactivating a filter doesn't shift column widths.
	// Compute once and reuse for both viewport-range and final sizing.
	hasPins := len(tab.Pins) > 0 && len(tab.FullCellRows) > 0
	var visNatural []int
	if hasPins {
		visNatural = naturalWidthsIndirect(visSpecs, tab.FullCellRows, visToFull, currencySymbol)
	} else {
		visNatural = naturalWidths(visSpecs, visCells, currencySymbol)
	}

	sepW := lipgloss.Width(normalSep)
	fullWidths := columnWidths(visSpecs, visCells, termWidth, sepW, visNatural)

	start, end, hasLeft, hasRight := viewportRange(
		fullWidths, sepW, termWidth, tab.ViewOffset, visColCursor,
	)
	vp.HasLeft = hasLeft
	vp.HasRight = hasRight

	vp.Specs = sliceViewport(visSpecs, start, end)
	vp.Cells = sliceViewportRows(visCells, start, end)
	vp.Sorts = viewportSorts(visSorts, start)
	vpVisToFull := sliceViewport(visToFull, start, end)

	vp.Cursor = visColCursor - start
	if visColCursor < start || visColCursor >= end {
		vp.Cursor = -1
	}
	vp.VisToFull = vpVisToFull

	fullCells := vp.Cells
	if hasPins {
		fullCells = projectCellRows(tab.FullCellRows, visToFull, start, end)
	}
	vp.LinkCells = fullCells
	vp.Widths = columnWidths(vp.Specs, fullCells, termWidth, sepW, visNatural[start:end])

	// Per-gap separators need to match the viewport's projected columns.
	vp.PlainSeps, vp.CollapsedSeps = gapSeparators(vpVisToFull, len(tab.Specs), normalSep)

	return vp
}

// formatHeaderCell renders a header cell with the title left-aligned and
// the sort indicator right-aligned within the given width. If there's no
// indicator, it falls back to plain left-aligned formatting.
func formatHeaderCell(title, indicator string, width int) string {
	if indicator == "" {
		return formatCell(title, width, alignLeft)
	}
	titleW := lipgloss.Width(title)
	indW := lipgloss.Width(indicator)
	gap := width - titleW - indW
	if gap < 0 {
		// Not enough room; truncate title to make space.
		available := width - indW
		if available < 1 {
			return formatCell(title, width, alignLeft)
		}
		title = ansi.Truncate(title, available, "")
		titleW = lipgloss.Width(title)
		gap = width - titleW - indW
	}
	return title + strings.Repeat(" ", gap) + indicator
}

// projectCellRows projects fullCellRows through the visible column mapping
// and viewport slice [start, end). Used to compute stable column widths from
// the unfiltered data set.
func projectCellRows(
	fullCellRows [][]cell,
	visToFull []int,
	start, end int,
) [][]cell {
	vpMap := visToFull[start:end]
	projected := make([][]cell, len(fullCellRows))
	for r, row := range fullCellRows {
		p := make([]cell, 0, len(vpMap))
		for _, fi := range vpMap {
			if fi < len(row) {
				p = append(p, row[fi])
			}
		}
		projected[r] = p
	}
	return projected
}

// viewportSorts adjusts sort entries so column indices are relative to the
// viewport start offset.
func viewportSorts(sorts []sortEntry, vpStart int) []sortEntry {
	if vpStart == 0 {
		return sorts
	}
	adjusted := make([]sortEntry, 0, len(sorts))
	for _, s := range sorts {
		adjusted = append(adjusted, sortEntry{Col: s.Col - vpStart, Dir: s.Dir})
	}
	return adjusted
}

// sortIndicatorWidth returns the maximum rendered width of a sort indicator
// given the total number of columns. Single-column sorts show " ▲" (2),
// decimalDigits returns the number of decimal digits in a positive integer.
func decimalDigits(n int) int {
	if n <= 0 {
		return 1
	}
	return int(math.Log10(float64(n))) + 1
}

// multi-column sorts show " ▲N" where N can be up to log10(columns)+1 digits.
func sortIndicatorWidth(columnCount int) int {
	if columnCount <= 1 {
		return 2 // " ▲"
	}
	return 2 + decimalDigits(columnCount) // " ▲" + index digits
}

// headerTitleWidth returns the rendered width of a column header including
// any suffix added at render time (link arrow, drilldown arrow, money symbol)
// plus room for the worst-case sort indicator given the column count.
// currencySymbol is the narrow currency glyph (e.g. "$", "EUR") used in
// money header annotations.
func headerTitleWidth(spec columnSpec, columnCount int, currencySymbol string) int {
	w := lipgloss.Width(spec.Title)
	if spec.Link != nil || spec.Kind == cellEntity {
		w += 1 + lipgloss.Width(linkArrow) // " →"
	} else if spec.Kind == cellDrilldown {
		w += 1 + lipgloss.Width(drilldownArrow) // " ↘"
	} else if spec.Kind == cellMoney {
		w += 1 + lipgloss.Width(currencySymbol) // " $" / " €" / " £"
	}
	w += sortIndicatorWidth(columnCount)
	return w
}

func sortIndicator(sorts []sortEntry, col int) string {
	for i, entry := range sorts {
		if entry.Col == col {
			arrow := " " + symTriUp // ▲ with leading space
			if entry.Dir == sortDesc {
				arrow = " " + symTriDown // ▼ with leading space
			}
			if len(sorts) == 1 {
				return arrow
			}
			return fmt.Sprintf("%s%d", arrow, i+1)
		}
	}
	return ""
}

func renderDivider(
	widths []int,
	separators []string,
	divSep string,
	style lipgloss.Style,
) string {
	parts := make([]string, 0, len(widths))
	for _, width := range widths {
		if width < 1 {
			width = 1
		}
		parts = append(parts, style.Render(strings.Repeat("─", width)))
	}
	// Uniform divider separator for all gaps (no ⋯ on the divider line).
	if len(separators) > 0 {
		uniform := make([]string, len(separators))
		for i := range uniform {
			uniform[i] = divSep
		}
		separators = uniform
	}
	return joinCells(parts, separators)
}

// pinRenderContext carries pin state into the rendering pipeline so cells and
// rows can be styled for pin preview / filter mode.
type pinRenderContext struct {
	Pins           []filterPin // nil when no pins are active
	RawCells       [][]cell    // pre-transform cells for pin matching (viewport coords)
	MagMode        bool        // true when magnitude display is active
	Inverted       bool        // true when filter is inverted (highlight non-matching)
	CurrencySymbol string      // currency symbol for mag-mode formatting
}

func renderRows(
	specs []columnSpec,
	rows [][]cell,
	meta []rowMeta,
	widths []int,
	plainSeps []string,
	collapsedSeps []string,
	cursor int,
	colCursor int,
	height int,
	pinCtx pinRenderContext,
) []string {
	total := len(rows)
	if total == 0 {
		return nil
	}
	if height <= 0 {
		height = total
	}
	start, end := visibleRange(total, height, cursor)
	count := end - start
	mid := start + count/2
	rendered := make([]string, 0, count)
	for i := start; i < end; i++ {
		selected := i == cursor
		deleted := i < len(meta) && meta[i].Deleted
		dimmed := i < len(meta) && meta[i].Dimmed
		// Show ⋯ on first, middle, and last visible rows only.
		seps := plainSeps
		if i == start || i == mid || i == end-1 {
			seps = collapsedSeps
		}
		row := renderRow(
			specs,
			rows[i],
			widths,
			seps,
			selected,
			deleted,
			dimmed,
			colCursor,
			pinCtx,
			i,
		)
		rendered = append(rendered, row)
	}
	return rendered
}

// cellHighlight describes how a cell should be visually marked.
type cellHighlight int

const (
	highlightNone   cellHighlight = iota
	highlightRow                  // selected row, not the active column
	highlightCursor               // selected row AND active column
)

func renderRow(
	specs []columnSpec,
	row []cell,
	widths []int,
	separators []string,
	selected bool,
	deleted bool,
	dimmed bool,
	colCursor int,
	pinCtx pinRenderContext,
	rowIdx int,
) string {
	cells := make([]string, 0, len(specs))
	for i, spec := range specs {
		width := safeWidth(widths, i)
		var cellValue cell
		if i < len(row) {
			cellValue = row[i]
		}
		hl := highlightNone
		if selected && i == colCursor {
			hl = highlightCursor
		} else if selected {
			hl = highlightRow
		}
		// Use raw (pre-transform) cell for pin matching so the comparison
		// stays consistent with how pins were stored, regardless of
		// display transforms (compact money, mag mode).
		pinMatch := false
		if len(pinCtx.Pins) > 0 {
			rawCell := cellValue
			if rowIdx < len(pinCtx.RawCells) && i < len(pinCtx.RawCells[rowIdx]) {
				rawCell = pinCtx.RawCells[rowIdx][i]
			}
			pinMatch = cellMatchesPin(
				pinCtx.Pins,
				i,
				rawCell,
				pinCtx.MagMode,
				pinCtx.CurrencySymbol,
			)
			// When inverted, highlight non-matching cells in pinned columns.
			if pinCtx.Inverted && columnHasPin(pinCtx.Pins, i) {
				pinMatch = !pinMatch
			}
		}
		rendered := renderCell(cellValue, spec, width, hl, deleted, dimmed, pinMatch)
		cells = append(cells, rendered)
	}
	return joinCells(cells, separators)
}

// columnHasPin reports whether any pin targets the given column index.
func columnHasPin(pins []filterPin, col int) bool {
	for _, pin := range pins {
		if pin.Col == col {
			return true
		}
	}
	return false
}

// cellMatchesPin checks if a cell matches any pinned value for its column
// (in the viewport's coordinate space). Uses the raw cell value (pre-display
// transform) and applies mag formatting when magMode is true to stay
// consistent with how pins were stored.
func cellMatchesPin(pins []filterPin, col int, c cell, magMode bool, currencySymbol string) bool {
	key := cellDisplayValue(c, magMode, currencySymbol)
	for _, pin := range pins {
		if pin.Col == col {
			return pin.Values[key]
		}
	}
	return false
}

func renderCell(
	cellValue cell,
	spec columnSpec,
	width int,
	hl cellHighlight,
	deleted bool,
	dimmed bool,
	pinMatch bool,
) string {
	if width < 1 {
		width = 1
	}
	value := firstLine(cellValue.Value)
	style := cellStyle(cellValue.Kind)
	if cellValue.Null {
		value = symEmptySet
		style = appStyles.Null()
	} else if value == "" {
		value = symEmDash
		style = appStyles.Empty()
	} else if cellValue.Kind == cellDrilldown && value != "0" {
		return renderPillCell(value, width, hl, deleted, dimmed)
	} else if cellValue.Kind == cellDrilldown {
		// Zero count: dim instead of pill to keep the grid quiet.
		style = appStyles.Empty()
	} else if cellValue.Kind == cellStatus {
		if s, ok := appStyles.StatusStyle(value); ok {
			style = s
		}
		value = statusLabel(value)
	} else if cellValue.Kind == cellEntity {
		// Strip the hidden kind-letter prefix ("P Kitchen Reno" → "Kitchen Reno")
		// and color the name by entity kind.
		if len(value) >= 2 && value[1] == ' ' {
			style = entityCellStyle(value)
			value = value[2:]
		} else {
			style = appStyles.CellDim()
		}
	} else if cellValue.Kind == cellUrgency {
		style = urgencyStyle(value)
	} else if cellValue.Kind == cellWarranty {
		style = warrantyStyle(value)
	}

	// Pin match overrides semantic color with the muted/pin color.
	if pinMatch {
		style = appStyles.Pinned()
	}

	if deleted {
		style = style.Foreground(textDim).Strikethrough(true).Italic(true)
	}

	// Dimmed rows in pin preview mode.
	if dimmed && !deleted {
		style = style.Foreground(textDim)
	}

	// Right-aligned grayed-out line count for multi-line notes.
	var noteSuffix string
	var noteSuffixW int
	if cellValue.Kind == cellNotes && !cellValue.Null && value != "" && value != symEmDash {
		if n := extraLineCount(cellValue.Value); n > 0 {
			value += symEllipsis
			noteSuffix = fmt.Sprintf("+%d", n)
			noteSuffixW = lipgloss.Width(noteSuffix)
		}
	}

	// For cursor underline and deleted strikethrough, style just the
	// text and pad separately so the decoration matches text length.
	if hl == highlightCursor || deleted {
		cursorStyle := style
		if hl == highlightCursor {
			cursorStyle = cursorStyle.Underline(true).Bold(true)
		}
		if hl == highlightRow {
			cursorStyle = cursorStyle.Background(surface).Bold(true)
		}
		textMaxW := width
		if noteSuffixW > 0 {
			textMaxW = width - noteSuffixW - 1
			if textMaxW < 1 {
				textMaxW = 1
			}
		}
		truncated := ansi.Truncate(value, textMaxW, symEllipsis)
		styled := cursorStyle.Render(truncated)
		textW := lipgloss.Width(truncated)
		if noteSuffixW > 0 {
			gap := width - textW - noteSuffixW
			if gap < 1 {
				gap = 1
			}
			dimSuffix := appStyles.Empty().Render(noteSuffix)
			return styled + strings.Repeat(" ", gap) + dimSuffix
		}
		if pad := width - textW; pad > 0 {
			if spec.Align == alignRight {
				return strings.Repeat(" ", pad) + styled
			}
			return styled + strings.Repeat(" ", pad)
		}
		return styled
	}

	if hl == highlightRow {
		style = style.Background(surface).Bold(true)
	}

	if noteSuffixW > 0 {
		textMaxW := width - noteSuffixW - 1
		if textMaxW < 1 {
			textMaxW = 1
		}
		truncated := ansi.Truncate(value, textMaxW, symEllipsis)
		styledText := style.Render(truncated)
		textW := lipgloss.Width(truncated)
		gap := width - textW - noteSuffixW
		if gap < 1 {
			gap = 1
		}
		dimSuffix := appStyles.Empty().Render(noteSuffix)
		return styledText + strings.Repeat(" ", gap) + dimSuffix
	}

	aligned := formatCell(value, width, spec.Align)
	return style.Render(aligned)
}

// renderPillCell renders a drilldown value as a compact pill badge,
// right-aligned within the column width.
func renderPillCell(
	value string,
	width int,
	hl cellHighlight,
	deleted bool,
	dimmed bool,
) string {
	style := appStyles.Drilldown()
	if dimmed && !deleted {
		style = appStyles.CellDim()
	}
	if deleted {
		style = appStyles.DeletedCell()
		pill := style.Render(value)
		pillW := lipgloss.Width(pill)
		if pad := width - pillW; pad > 0 {
			return strings.Repeat(" ", pad) + pill
		}
		return pill
	}

	if hl == highlightCursor {
		style = style.Underline(true)
	}

	pill := style.Render(value)
	pillW := lipgloss.Width(pill)

	// Pad to fill the column; pill is always right-aligned.
	if pad := width - pillW; pad > 0 {
		padStyle := appStyles.Base()
		if hl == highlightRow {
			padStyle = padStyle.Background(surface)
		}
		return padStyle.Render(strings.Repeat(" ", pad)) + pill
	}
	return pill
}

// joinCells joins rendered cell strings using per-gap separators.
// If separators is shorter than needed, falls back to the last separator.
func joinCells(cells []string, separators []string) string {
	if len(cells) == 0 {
		return ""
	}
	var b strings.Builder
	for i, c := range cells {
		if i > 0 {
			idx := i - 1
			if idx < len(separators) {
				b.WriteString(separators[idx])
			} else if len(separators) > 0 {
				b.WriteString(separators[len(separators)-1])
			}
		}
		b.WriteString(c)
	}
	return b.String()
}

func cellStyle(kind cellKind) lipgloss.Style {
	switch kind {
	case cellMoney:
		return appStyles.Money()
	case cellReadonly:
		return appStyles.Readonly()
	case cellDrilldown:
		return appStyles.Drilldown()
	default:
		return defaultStyle
	}
}

// Urgency/warranty/entity-kind styles live in appStyles to avoid
// duplicate definitions. Package-level aliases for readability in
// cell-rendering code.
var (
	urgencyOverdue  = appStyles.UrgencyOverdue()
	urgencySoon     = appStyles.UrgencySoon()
	urgencyUpcoming = appStyles.UrgencyUpcoming()
	urgencyFar      = appStyles.UrgencyFar()
	warrantyExpired = appStyles.WarrantyExpired()
	warrantyActive  = appStyles.WarrantyActive()
)

// entityCellStyle returns a style for the entity cell based on the kind-letter
// prefix. The letter (A/I/M/P/Q/V) maps to a kind-specific color.
func entityCellStyle(value string) lipgloss.Style {
	if len(value) > 0 {
		if s, ok := appStyles.EntityKindStyle(value[0]); ok {
			return s
		}
	}
	return appStyles.TextDim()
}

// urgencyStyle returns a style colored from green (far out) through yellow
// (upcoming) to red (overdue) based on the number of days until a date.
// Thresholds: >60 days = green, 30-60 = yellow, 0-30 = orange, <0 = red.
func urgencyStyle(dateStr string) lipgloss.Style {
	return urgencyStyleAt(dateStr, time.Now())
}

func urgencyStyleAt(dateStr string, now time.Time) lipgloss.Style {
	if dateStr == "" {
		return defaultStyle
	}
	t, err := time.Parse(data.DateLayout, strings.TrimSpace(dateStr))
	if err != nil {
		return defaultStyle
	}
	days := dateDiffDays(now, t)
	switch {
	case days < 0:
		return urgencyOverdue
	case days <= 30:
		return urgencySoon
	case days <= 60:
		return urgencyUpcoming
	default:
		return urgencyFar
	}
}

// warrantyStyle returns green if the warranty is still active, red if expired.
func warrantyStyle(dateStr string) lipgloss.Style {
	return warrantyStyleAt(dateStr, time.Now())
}

func warrantyStyleAt(dateStr string, now time.Time) lipgloss.Style {
	if dateStr == "" {
		return defaultStyle
	}
	t, err := time.Parse(data.DateLayout, strings.TrimSpace(dateStr))
	if err != nil {
		return defaultStyle
	}
	if dateDiffDays(now, t) < 0 {
		return warrantyExpired
	}
	return warrantyActive
}

// dateDiffDays returns the number of calendar days from now to target,
// using each time's local Y/M/D. Positive means target is in the future.
func dateDiffDays(now, target time.Time) int {
	nowDate := time.Date(
		now.Year(), now.Month(), now.Day(),
		0, 0, 0, 0, time.UTC,
	)
	tgtDate := time.Date(
		target.Year(), target.Month(), target.Day(),
		0, 0, 0, 0, time.UTC,
	)
	return int(math.Round(tgtDate.Sub(nowDate).Hours() / 24))
}

// firstLine returns the first line of s, trimmed of surrounding whitespace.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimRight(s[:i], "\r \t")
	}
	return s
}

// extraLineCount returns the number of additional lines beyond the first.
// Returns 0 for single-line or empty strings.
func extraLineCount(s string) int {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, '\n')
	if i < 0 {
		return 0
	}
	return strings.Count(s[i:], "\n")
}

// noteSuffixWidth returns the display width of the "+N" indicator for a
// multi-line note with n extra lines.
func noteSuffixWidth(n int) int {
	if n <= 0 {
		return 0
	}
	return 1 + decimalDigits(n) // "+" + digits
}

func formatCell(value string, width int, align alignKind) string {
	if width < 1 {
		return ""
	}
	truncated := ansi.Truncate(value, width, "…")
	current := lipgloss.Width(truncated)
	if current >= width {
		return truncated
	}
	padding := width - current
	switch align {
	case alignRight:
		return strings.Repeat(" ", padding) + truncated
	default:
		return truncated + strings.Repeat(" ", padding)
	}
}

func visibleRange(total, height, cursor int) (int, int) {
	if total <= height {
		return 0, total
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= total {
		cursor = total - 1
	}
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > total {
		end = total
		start = end - height
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func columnWidths(
	specs []columnSpec,
	rows [][]cell,
	width int,
	separatorWidth int,
	precompNatural []int, // nil = compute on the fly
) []int {
	columnCount := len(specs)
	if columnCount == 0 {
		return nil
	}
	available := width - separatorWidth*(columnCount-1)
	if available < columnCount {
		available = columnCount
	}

	natural := precompNatural
	if natural == nil {
		natural = naturalWidths(specs, rows, "$")
	}

	// If content-driven widths fit, use them — no truncation.
	if sumInts(natural) <= available {
		widths := make([]int, columnCount)
		copy(widths, natural)
		extra := available - sumInts(widths)
		if extra > 0 {
			flex := flexColumns(specs)
			if len(flex) == 0 {
				flex = allColumns(specs)
			}
			distribute(widths, specs, flex, extra, true)
		}
		return widths
	}

	// Content exceeds terminal width — apply Max constraints.
	widths := make([]int, columnCount)
	for i, w := range natural {
		if specs[i].Max > 0 && w > specs[i].Max {
			w = specs[i].Max
		}
		widths[i] = w
	}

	total := sumInts(widths)
	if total <= available {
		// Max-capped fits. Give extra space to truncated columns first
		// so they can show their full content before flex columns grow.
		extra := available - total
		extra = widenTruncated(widths, natural, extra)
		if extra > 0 {
			flex := flexColumns(specs)
			if len(flex) == 0 {
				flex = allColumns(specs)
			}
			distribute(widths, specs, flex, extra, true)
		}
		return widths
	}

	// Still too wide — shrink flex columns.
	deficit := total - available
	flex := flexColumns(specs)
	if len(flex) == 0 {
		flex = allColumns(specs)
	}
	distribute(widths, specs, flex, deficit, false)
	return widths
}

// naturalWidths returns the content-driven width for each column (header,
// fixed values, and actual cell values) floored by Min but NOT capped by Max.
func naturalWidths(specs []columnSpec, rows [][]cell, currencySymbol string) []int {
	widths := make([]int, len(specs))
	colCount := len(specs)
	for i, spec := range specs {
		w := headerTitleWidth(spec, colCount, currencySymbol)
		for _, fv := range spec.FixedValues {
			if fw := lipgloss.Width(fv); fw > w {
				w = fw
			}
		}
		for _, row := range rows {
			if i >= len(row) {
				continue
			}
			value := firstLine(row[i].Value)
			if value == "" {
				continue
			}
			cw := lipgloss.Width(value)
			if spec.Kind == cellNotes {
				if n := extraLineCount(row[i].Value); n > 0 {
					// Account for "…" + gap + right-aligned "+N" indicator.
					cw += 1 + 1 + noteSuffixWidth(n)
				}
			}
			if cw > w {
				w = cw
			}
		}
		if w < spec.Min {
			w = spec.Min
		}
		widths[i] = w
	}
	return widths
}

// naturalWidthsIndirect computes natural widths using fullRows indexed
// through visToFull, avoiding a temporary projected [][]cell allocation.
func naturalWidthsIndirect(
	specs []columnSpec,
	fullRows [][]cell,
	visToFull []int,
	currencySymbol string,
) []int {
	widths := make([]int, len(specs))
	colCount := len(specs)
	for vi, spec := range specs {
		fi := visToFull[vi]
		w := headerTitleWidth(spec, colCount, currencySymbol)
		for _, fv := range spec.FixedValues {
			if fw := lipgloss.Width(fv); fw > w {
				w = fw
			}
		}
		for _, row := range fullRows {
			if fi >= len(row) {
				continue
			}
			value := firstLine(row[fi].Value)
			if value == "" {
				continue
			}
			cw := lipgloss.Width(value)
			if spec.Kind == cellNotes {
				if n := extraLineCount(row[fi].Value); n > 0 {
					cw += 1 + 1 + noteSuffixWidth(n)
				}
			}
			if cw > w {
				w = cw
			}
		}
		if w < spec.Min {
			w = spec.Min
		}
		widths[vi] = w
	}
	return widths
}

// widenTruncated gives extra space to columns whose current width is less than
// their natural (content-driven) width, eliminating truncation where possible.
// Returns the remaining unused extra.
func widenTruncated(widths, natural []int, extra int) int {
	for extra > 0 {
		changed := false
		for i := range widths {
			if extra == 0 {
				break
			}
			if widths[i] < natural[i] {
				widths[i]++
				extra--
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return extra
}

func distribute(
	widths []int,
	specs []columnSpec,
	indices []int,
	amount int,
	grow bool,
) {
	if amount <= 0 || len(indices) == 0 {
		return
	}
	for amount > 0 {
		changed := false
		for _, idx := range indices {
			if idx >= len(widths) {
				continue
			}
			if grow {
				if specs[idx].Max > 0 && widths[idx] >= specs[idx].Max {
					continue
				}
				widths[idx]++
			} else {
				if widths[idx] <= specs[idx].Min {
					continue
				}
				widths[idx]--
			}
			amount--
			changed = true
			if amount == 0 {
				break
			}
		}
		if !changed {
			return
		}
	}
}

func flexColumns(specs []columnSpec) []int {
	indices := make([]int, 0, len(specs))
	for i, spec := range specs {
		if spec.Flex {
			indices = append(indices, i)
		}
	}
	return indices
}

func allColumns(specs []columnSpec) []int {
	indices := make([]int, len(specs))
	for i := range specs {
		indices[i] = i
	}
	return indices
}

func sumInts(values []int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}

func safeWidth(widths []int, idx int) int {
	if idx >= len(widths) {
		return 1
	}
	if widths[idx] < 1 {
		return 1
	}
	return widths[idx]
}

// ---------------------------------------------------------------------------
// Horizontal scroll viewport
// ---------------------------------------------------------------------------

// scrollIndicatorWidth is the visual width reserved for the "< " / " >" gutter
// arrows that signal off-screen columns.
const scrollIndicatorWidth = 2

// ensureCursorVisible adjusts tab.ViewOffset so that the column cursor is
// within the viewport. Call after any ColCursor change. visCursor is the
// cursor index in the visible projection (post-hidden-column filter).
func ensureCursorVisible(tab *Tab, visCursor int, visCount int) {
	if visCount == 0 {
		tab.ViewOffset = 0
		return
	}
	if tab.ViewOffset > visCursor {
		tab.ViewOffset = visCursor
	}
	// The upper bound check requires knowing which columns fit. We use a
	// conservative rule: offset can never exceed cursor. The viewport
	// computation in tableView does the precise fitting.
	if tab.ViewOffset > visCount-1 {
		tab.ViewOffset = visCount - 1
	}
	if tab.ViewOffset < 0 {
		tab.ViewOffset = 0
	}
}

// viewportRange determines the range of visible-projection column indices
// [start, end) that fit within the terminal width, starting from viewOffset.
// It also returns whether there are columns off-screen to the left or right.
func viewportRange(
	widths []int,
	sepWidth int,
	termWidth int,
	viewOffset int,
	visCursor int,
) (start, end int, hasLeft, hasRight bool) {
	n := len(widths)
	if n == 0 {
		return 0, 0, false, false
	}
	if viewOffset < 0 {
		viewOffset = 0
	}
	if viewOffset >= n {
		viewOffset = n - 1
	}

	// Check if everything already fits without scrolling.
	totalWidth := sumInts(widths)
	if n > 1 {
		totalWidth += (n - 1) * sepWidth
	}
	if totalWidth <= termWidth {
		return 0, n, false, false
	}

	// Start from viewOffset and greedily include columns left-to-right.
	start = viewOffset
	hasLeft = start > 0
	budget := termWidth
	if hasLeft {
		budget -= scrollIndicatorWidth
	}

	end = start
	for end < n {
		colW := widths[end]
		if end > start {
			colW += sepWidth
		}
		if budget-colW < 0 && end > start {
			break
		}
		budget -= colW
		end++
	}

	hasRight = end < n
	// If right indicator is needed but took the last column's space, drop it.
	if hasRight && budget < scrollIndicatorWidth && end > start+1 {
		end--
	}

	// Ensure cursor is visible: if cursor is past the right edge, shift right.
	for visCursor >= end && end < n {
		// Drop leftmost column and try to add more on the right.
		start++
		hasLeft = true
		budget = termWidth - scrollIndicatorWidth // left indicator
		for e := start; e < n; e++ {
			colW := widths[e]
			if e > start {
				colW += sepWidth
			}
			if budget-colW < 0 && e > start {
				end = e
				break
			}
			budget -= colW
			end = e + 1
		}
		hasRight = end < n
		if hasRight && budget < scrollIndicatorWidth && end > start+1 {
			end--
		}
		if visCursor < end {
			break
		}
	}

	return start, end, hasLeft, hasRight
}

// sliceViewport extracts the viewport window from visible-projection data.
func sliceViewport[T any](items []T, start, end int) []T {
	if start >= len(items) {
		return nil
	}
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

// sliceViewportRows extracts the viewport window from each row of cells.
func sliceViewportRows(rows [][]cell, start, end int) [][]cell {
	result := make([][]cell, len(rows))
	for i, row := range rows {
		result[i] = sliceViewport(row, start, end)
	}
	return result
}
