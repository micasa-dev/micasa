// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"os"
	"strings"

	"charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

func (m *Model) buildView() string {
	if m.terminalTooSmall() {
		return m.buildTerminalTooSmallView()
	}

	if m.mode == modeForm && m.fs.form != nil && m.fs.formKind() == formHouse {
		return m.formFullScreen()
	}

	base := m.buildBaseView()

	// Overlays applied in priority order (later overlays render on top).
	overlays := []struct {
		active bool
		render func() string
	}{
		{m.dashboardVisible(), m.buildDashboardOverlay},
		{m.calendar != nil, m.buildCalendarOverlay},
		{m.notePreview != nil, m.buildNotePreviewOverlay},
		{m.opsTree != nil, m.buildOpsTreeOverlay},
		{m.columnFinder != nil, m.buildColumnFinderOverlay},
		{m.docSearch != nil, m.buildDocSearchOverlay},
		{m.ex.extraction != nil && m.ex.extraction.Visible, m.buildExtractionOverlay},
		{m.chat != nil && m.chat.Visible, m.buildChatOverlay},
		{m.helpViewport != nil, m.buildHelpOverlay},
	}

	hasOverlay := false
	for _, o := range overlays {
		if o.active {
			hasOverlay = true
			break
		}
	}

	// When the base view overflows the terminal (e.g. a tall form), clamp
	// it to the terminal height so overlay.Composite centers correctly.
	if hasOverlay {
		if h := m.effectiveHeight(); h > 0 {
			if lines := strings.Split(base, "\n"); len(lines) > h {
				base = strings.Join(lines[len(lines)-h:], "\n")
			}
		}
	}

	for _, o := range overlays {
		if o.active {
			fg := m.zones.Mark(zoneOverlay, cancelFaint(o.render()))
			base = compositeOverlay(fg, dimBackground(base))
		}
	}

	return base
}

func (m *Model) buildTerminalTooSmallView() string {
	width := m.effectiveWidth()
	height := m.effectiveHeight()

	panel := lipgloss.JoinVertical(
		lipgloss.Center,
		m.styles.Error().Render("Terminal too small"),
		"",
		m.styles.HeaderHint().Render(
			fmt.Sprintf(
				"%dx%d — need at least %dx%d",
				width,
				height,
				minUsableWidth,
				minUsableHeight,
			),
		),
	)

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		clampLines(panel, width),
	)
}

// buildBaseView renders the normal table/detail/form view with house, tabs,
// content area, and status bar. Used as the background when the dashboard
// overlay is active.
func (m *Model) buildBaseView() string {
	house := m.houseView()

	tabs := m.tabsView()
	if m.inDetail() {
		tabs = m.breadcrumbView()
	}
	tabLine := m.tabUnderline()

	var content string
	if m.mode == modeForm && m.fs.form != nil {
		if legend := m.requiredLegend(); legend != "" {
			content = legend + "\n\n" + m.fs.form.View()
		} else {
			content = m.fs.form.View()
		}
	} else if tab := m.effectiveTab(); tab != nil {
		content = m.tableView(tab)
	}
	status := m.statusView()

	// Right-align db path on the tab row when set.
	if m.dbPath != "" {
		tabsW := lipgloss.Width(tabs)
		minGap := 2 // breathing room between tabs and path
		available := m.effectiveWidth() - tabsW - minGap
		if available > 5 {
			path := truncateLeft(shortenHome(m.dbPath), available)
			if m.dbPath != ":memory:" {
				path = osc8Link("file://"+m.dbPath, path)
			}
			label := m.styles.HeaderHint().Render(path)
			gap := m.effectiveWidth() - tabsW - lipgloss.Width(label)
			if gap > 0 {
				tabs += strings.Repeat(" ", gap) + label
			}
		}
	}

	// Assemble upper portion with intentional spacing.
	upper := lipgloss.JoinVertical(lipgloss.Left, house, "", tabs, tabLine)
	if content != "" {
		upper = lipgloss.JoinVertical(lipgloss.Left, upper, content)
	}

	// Anchor the status bar to the terminal bottom.
	upperH := lipgloss.Height(upper)
	statusH := lipgloss.Height(status)
	gap := m.height - upperH - statusH + 1
	if gap < 1 {
		gap = 1
	}

	var b strings.Builder
	b.WriteString(upper)
	b.WriteString(strings.Repeat("\n", gap))
	b.WriteString(status)
	return clampLines(b.String(), m.effectiveWidth())
}

// buildDashboardOverlay renders the dashboard content inside a bordered box
// with navigation hints, suitable for compositing over the base view.
func (m *Model) buildDashboardOverlay() string {
	contentW := m.overlayContentWidth()
	innerW := contentW - 4 // exclude box padding (2 each side)
	header := m.dashboardHeader()

	// Minimal hints inside the overlay.
	hintParts := []string{
		m.helpItem(keyShiftD, "close"),
		m.helpItem(keyQuestion, "help"),
	}
	if m.dash.flash != "" {
		hintParts = append(hintParts, m.styles.DashHouseValue().Render(m.dash.flash))
	}
	hints := joinWithSeparator(m.helpSeparator(), hintParts...)

	// Budget for dashboardView content: outer box height minus chrome.
	// Chrome: border (2) + padding (2) + header (1) + rule (1) + blank (1)
	// + hints (1) = 8 lines.
	maxH := m.overlayMaxHeight()
	contentBudget := maxH - 8
	if contentBudget < 3 {
		contentBudget = 3
	}
	m.prepareDashboardView()
	content := m.dashboardView(contentBudget, innerW)

	rule := m.styles.DashRule().Render(strings.Repeat("─", innerW))
	boxContent := lipgloss.JoinVertical(
		lipgloss.Left, header, rule, content, "", hints,
	)

	return m.styles.OverlayBox().
		Width(contentW).
		MaxHeight(maxH).
		Render(boxContent)
}

// tabsLocked returns true when tab switching is disabled and inactive tabs
// should be visually struck through. This applies in edit mode (tabs are
// pinned) and form mode (tabs are inaccessible while the form is open).
func (m *Model) tabsLocked() bool {
	return m.mode == modeEdit || m.mode == modeForm
}

func (m *Model) tabsView() string {
	pinned := m.tabsLocked()
	parts := make([]string, 0, len(m.tabs)*2)
	for i, tab := range m.tabs {
		var rendered string
		if i == m.active {
			rendered = m.styles.TabActive().Render(tab.Name)
		} else if pinned {
			rendered = m.styles.TabLocked().Render(tab.Name)
		} else {
			rendered = m.styles.TabInactive().Render(tab.Name)
		}
		parts = append(parts, m.zones.Mark(fmt.Sprintf("%s%d", zoneTab, i), rendered))
		// Gap between tabs: triangle indicates filter state.
		// Filled/hollow = active/preview, down/up = normal/inverted.
		var mark string
		switch {
		case tab.FilterActive && tab.FilterInverted:
			mark = filterMarkActiveInverted
		case tab.FilterActive:
			mark = filterMarkActive
		case tab.FilterInverted:
			mark = filterMarkPreviewInverted
		case len(tab.Pins) > 0:
			mark = filterMarkPreview
		}
		if mark != "" {
			parts = append(parts, " "+m.styles.FilterMark().Render(mark)+" ")
		} else {
			parts = append(parts, "   ")
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m *Model) breadcrumbView() string {
	if !m.inDetail() {
		return ""
	}

	arrow := m.styles.BreadcrumbArrow().Render(breadcrumbSep)

	// Collect all breadcrumb segments from the stack.
	var parts []string
	for _, dc := range m.detailStack {
		parts = append(parts, strings.Split(dc.Breadcrumb, breadcrumbSep)...)
	}

	rendered := make([]string, len(parts))
	for i, p := range parts {
		if i < len(parts)-1 {
			rendered[i] = m.styles.HeaderHint().Render(p)
		} else {
			rendered[i] = m.styles.Breadcrumb().Render(p)
		}
	}
	crumb := strings.Join(rendered, arrow)
	back := m.styles.HeaderHint().Render(" (")
	back += m.keycap("esc")
	back += m.styles.HeaderHint().Render(" back)")
	return crumb + m.zones.Mark(zoneBreadcrumb, back)
}

func (m *Model) tabUnderline() string {
	return m.styles.TabUnderline().Render(strings.Repeat("━", m.effectiveWidth()))
}

func (m *Model) statusView() string {
	if m.inlineInput != nil {
		return m.withPullProgress(m.inlineInputStatusView())
	}
	if m.confirm == confirmHardDelete {
		prompt := m.styles.FormDirty().Render("Permanently delete this incident?")
		hints := joinWithSeparator(
			m.helpSeparator(),
			m.helpItem(keyY, "delete forever"),
			m.helpItem(keyN, "cancel"),
		)
		return m.withPullProgress(prompt + "  " + hints)
	}
	if m.mode == modeForm {
		if m.confirm.isFormConfirm() {
			prompt := m.styles.FormDirty().Render("Discard unsaved changes?")
			hints := joinWithSeparator(
				m.helpSeparator(),
				m.helpItem(keyY, "discard"),
				m.helpItem(keyN, "keep editing"),
			)
			return m.withPullProgress(prompt + "  " + hints)
		}
		dirtyIndicator := m.styles.FormClean().Render("○ saved")
		if m.fs.formDirty {
			dirtyIndicator = m.styles.FormDirty().Render("● unsaved")
		}
		parts := []string{
			dirtyIndicator,
			m.helpItem(keyCtrlS, "save"),
		}
		if m.fs.notesEditMode {
			parts = append(parts, m.helpItem(keyCtrlE, "editor"))
		}
		parts = append(parts,
			m.helpItem(keyEsc, "cancel"),
			m.helpItem(keyCtrlQ, "quit"),
		)
		help := joinWithSeparator(m.helpSeparator(), parts...)
		return m.withPullProgress(m.withStatusMessage(help))
	}

	// When overlays are active, don't show main tab keybindings since they're
	// not accessible. Overlays show their own relevant hints.
	if m.hasActiveOverlay() {
		return m.withPullProgress(m.withStatusMessage(""))
	}

	// Both badges render at the same width to prevent layout shift.
	// Anchor to the wider label so the narrower one gets padded, not squeezed.
	navW := lipgloss.Width(m.styles.ModeNormal().Render("NAV"))
	editW := lipgloss.Width(m.styles.ModeEdit().Render("EDIT"))
	badgeWidth := navW
	if editW > badgeWidth {
		badgeWidth = editW
	}
	modeBadge := m.styles.ModeNormal().
		Width(badgeWidth).
		Align(lipgloss.Center).
		Render("NAV")
	if m.mode == modeEdit {
		modeBadge = m.styles.ModeEdit().
			Width(badgeWidth).
			Align(lipgloss.Center).
			Render("EDIT")
	}

	var help string
	if m.mode == modeNormal {
		help = m.normalModeStatusHelp(modeBadge)
	} else {
		help = m.editModeStatusHelp(modeBadge)
	}

	return m.withBgExtractionIndicator(m.withPullProgress(m.withStatusMessage(help)))
}

// withBgExtractionIndicator prepends a background extraction indicator when
// extractions are running or awaiting review in the background.
func (m *Model) withBgExtractionIndicator(statusOutput string) string {
	n := len(m.ex.bgExtractions)
	if n == 0 {
		return statusOutput
	}
	var running, ready int
	for _, bg := range m.ex.bgExtractions {
		if bg.Done {
			ready++
		} else {
			running++
		}
	}
	var parts []string
	if running > 0 {
		sp := m.ex.bgExtractions[0].Spinner.View()
		parts = append(parts, appStyles.AccentText().Render(
			fmt.Sprintf("%s %d extracting", sp, running),
		))
	}
	if ready > 0 {
		parts = append(parts, appStyles.AccentText().Render(
			fmt.Sprintf("%d ready", ready),
		))
	}
	indicator := strings.Join(parts, "  ")
	return lipgloss.JoinVertical(lipgloss.Left, indicator, statusOutput)
}

func (m *Model) inlineInputStatusView() string {
	ii := m.inlineInput
	title := m.styles.HeaderLabel().Render(ii.Title + ":")
	input := ii.Input.View()
	hints := joinWithSeparator(
		m.helpSeparator(),
		m.helpItem(symReturn, "save"),
		m.helpItem(keyEsc, "cancel"),
	)
	prompt := title + " " + input + "  " + hints
	return m.withStatusMessage(prompt)
}

type statusHint struct {
	id       string
	full     string
	compact  string
	priority int
	required bool
}

func (m *Model) normalModeStatusHelp(modeBadge string) string {
	hints := m.normalModeStatusHints(modeBadge)
	return m.renderStatusHints(hints)
}

func (m *Model) normalModeStatusHints(modeBadge string) []statusHint {
	hints := []statusHint{
		{
			id:       "mode",
			full:     modeBadge,
			priority: 0,
			required: true,
		},
	}

	// Context-dependent action: what enter does on the current column.
	if hint := m.enterHint(); hint != "" {
		hints = append(hints, statusHint{
			id:       "enter",
			full:     m.helpItem(symReturn, hint),
			compact:  m.helpItem(symReturn, "open"),
			priority: 2,
		})
	}

	// Primary actions.
	hints = append(hints,
		statusHint{id: "edit", full: m.helpItem(keyI, "edit"), priority: 2},
	)
	if m.effectiveTab().isDocumentTab() {
		hints = append(hints, statusHint{
			id: "open", full: m.helpItem(keyO, "open"), priority: 2,
		})
	}
	if m.effectiveTab().isDocumentTab() {
		hints = append(hints, statusHint{
			id:       "search",
			full:     m.helpItem(keyCtrlF, "search"),
			priority: 3,
		})
	}
	if m.llmClient != nil {
		hints = append(hints, statusHint{
			id:       "ask",
			full:     m.helpItem(keyAt, "ask"),
			priority: 3,
		})
	}

	// Anchors: help is always visible; back only in detail view.
	hints = append(hints, statusHint{
		id:       "help",
		full:     m.helpItem(keyQuestion, "help"),
		compact:  m.helpItem(keyQuestion, "more"),
		priority: 0,
		required: true,
	})
	if m.inDetail() {
		hints = append(hints, statusHint{
			id:       "exit",
			full:     m.helpItem(keyEsc, "back"),
			compact:  m.renderKeys(keyEsc),
			priority: 0,
			required: true,
		})
	}
	return hints
}

// editModeStatusHelp renders the edit mode status bar with only primary
// actions. Profile is discoverable via the help overlay.
func (m *Model) editModeStatusHelp(modeBadge string) string {
	hints := []statusHint{
		{id: "mode", full: modeBadge, priority: 0, required: true},
	}
	addKey := keyA
	if m.effectiveTab().isDocumentTab() {
		addKey = keyA + "/" + keyShiftA
	}
	hints = append(hints,
		statusHint{id: "add", full: m.helpItem(addKey, "add"), priority: 1},
		statusHint{id: "edit", full: m.helpItem(keyE+"/"+keyShiftE, m.editHint()), priority: 1},
		statusHint{
			id:       "del",
			full:     m.helpItem(keyD, "del/restore"),
			compact:  m.helpItem(keyD, "del"),
			priority: 2,
		},
	)
	if m.effectiveTab().isDocumentTab() {
		hints = append(hints,
			statusHint{id: "open", full: m.helpItem(keyO, "open"), priority: 2},
			statusHint{id: "extract", full: m.helpItem(keyR, "extract"), priority: 3},
		)
	}
	hints = append(hints, statusHint{
		id:       "exit",
		full:     m.helpItem(keyEsc, "nav"),
		compact:  m.renderKeys(keyEsc),
		priority: 0,
		required: true,
	})
	return m.renderStatusHints(hints)
}

func (m *Model) renderStatusHints(hints []statusHint) string {
	if len(hints) == 0 {
		return ""
	}
	maxW := m.effectiveWidth()
	sep := m.helpSeparator()
	compact := make([]bool, len(hints))
	dropped := make([]bool, len(hints))
	maxPriority := 0
	for _, hint := range hints {
		if hint.priority > maxPriority {
			maxPriority = hint.priority
		}
	}
	build := func() string {
		parts := make([]string, 0, len(hints))
		for i, hint := range hints {
			if dropped[i] {
				continue
			}
			value := hint.full
			if compact[i] && hint.compact != "" {
				value = hint.compact
			}
			if hint.id != "" {
				value = m.zones.Mark(zoneHint+hint.id, value)
			}
			parts = append(parts, value)
		}
		return joinWithSeparator(sep, parts...)
	}

	line := build()
	if lipgloss.Width(line) <= maxW {
		return line
	}

	// Phase 1: compact hints (non-required first, then required).
	for _, skipRequired := range []bool{true, false} {
		for priority := maxPriority; priority >= 0; priority-- {
			for i := len(hints) - 1; i >= 0; i-- {
				hint := hints[i]
				if (skipRequired && hint.required) ||
					hint.priority != priority || hint.compact == "" || compact[i] {
					continue
				}
				compact[i] = true
				line = build()
				if lipgloss.Width(line) <= maxW {
					return line
				}
			}
		}
	}

	// Phase 2: drop non-required hints by descending priority.
	droppedAny := false
	for priority := maxPriority; priority >= 0; priority-- {
		for i := len(hints) - 1; i >= 0; i-- {
			hint := hints[i]
			if hint.required || hint.priority != priority || dropped[i] {
				continue
			}
			dropped[i] = true
			if !droppedAny {
				droppedAny = true
				if helpIdx := statusHintIndex(hints, "help"); helpIdx >= 0 &&
					hints[helpIdx].compact != "" {
					compact[helpIdx] = true
				}
			}
			line = build()
			if lipgloss.Width(line) <= maxW {
				return line
			}
		}
	}

	return line
}

func statusHintIndex(hints []statusHint, id string) int {
	for i, hint := range hints {
		if hint.id == id {
			return i
		}
	}
	return -1
}

// withStatusMessage renders the help line, prepending the status message if set.
func (m *Model) withStatusMessage(helpLine string) string {
	if m.status.Text == "" {
		return helpLine
	}
	style := m.styles.Info()
	if m.status.Kind == statusError {
		style = m.styles.Error()
	}
	return lipgloss.JoinVertical(lipgloss.Left, style.Render(m.status.Text), helpLine)
}

// withPullProgress appends the model download progress line below the status
// output when a pull is active.
func (m *Model) withPullProgress(statusOutput string) string {
	if m.pull.display == "" {
		return statusOutput
	}
	progressLine := m.styles.TextDim().Render(m.pull.display)
	return lipgloss.JoinVertical(lipgloss.Left, statusOutput, progressLine)
}

func (m *Model) editHint() string {
	tab := m.effectiveTab()
	if tab == nil {
		return "edit"
	}
	col := tab.ColCursor
	if col < 0 || col >= len(tab.Specs) {
		return "edit"
	}
	spec := tab.Specs[col]
	// Show "follow link" hint when on a linked cell with a target.
	if spec.Link != nil || spec.Kind == cellEntity {
		if c, ok := m.selectedCell(col); ok && c.LinkID != "" {
			return "follow " + linkArrow
		}
	}
	if spec.Kind == cellReadonly {
		return "edit"
	}
	return "edit: " + spec.Title
}

// enterHint returns a contextual label for the enter key in Normal mode,
// or "" if enter has no action on the current column.
func (m *Model) enterHint() string {
	tab := m.effectiveTab()
	if tab == nil {
		return ""
	}
	col := tab.ColCursor
	if col < 0 || col >= len(tab.Specs) {
		return ""
	}
	spec := tab.Specs[col]
	if spec.Kind == cellNotes {
		return "preview"
	}
	if spec.Kind == cellOps {
		return "ops"
	}
	if spec.Kind == cellDrilldown {
		return m.drilldownHint(tab, spec)
	}
	if spec.Link != nil || spec.Kind == cellEntity {
		if c, ok := m.selectedCell(col); ok && c.LinkID != "" {
			return "follow " + linkArrow
		}
	}
	return ""
}

// drilldownHint returns a short label for the drilldown target based on the
// current tab and column. Used in status bar hints.
func (m *Model) drilldownHint(_ *Tab, _ columnSpec) string {
	return drilldownArrow + " drill"
}

func (m *Model) formFullScreen() string {
	formContent := m.fs.form.View()
	if legend := m.requiredLegend(); legend != "" {
		formContent = legend + "\n\n" + formContent
	}
	status := m.statusView()
	panel := lipgloss.JoinVertical(lipgloss.Left, formContent, "", status)
	return m.centerPanel(panel, 1)
}

func (m *Model) buildCalendarOverlay() string {
	if m.calendar == nil {
		return ""
	}
	grid := calendarGrid(*m.calendar)
	return m.styles.OverlayBox().Render(grid)
}

func (m *Model) buildNotePreviewOverlay() string {
	contentW := m.overlayContentWidth()

	var b strings.Builder
	title := m.notePreview.title
	if title == "" {
		title = "Notes"
	}
	b.WriteString(m.styles.HeaderSection().Render(" " + title + " "))
	b.WriteString("\n\n")

	// Word-wrap the note text to fit within the box.
	innerW := contentW - 4 // padding
	text := m.notePreview.text
	b.WriteString(wordWrap(text, innerW))
	b.WriteString("\n\n")

	b.WriteString(m.styles.HeaderHint().Render("Press any key to close"))

	return m.styles.OverlayBox().
		Width(contentW).
		MaxHeight(m.overlayMaxHeight()).
		Render(b.String())
}

func (m *Model) buildHelpOverlay() string {
	// helpView() already renders a bordered box with padding.
	return m.helpView()
}

// centerPanel centers a rendered panel within the terminal dimensions.
// minPadTop sets the minimum top padding (e.g. 1 to keep a gap above forms).
func (m *Model) centerPanel(panel string, minPadTop int) string {
	width := m.effectiveWidth()
	height := m.effectiveHeight()
	panelH := lipgloss.Height(panel)
	panelW := lipgloss.Width(panel)
	padTop := (height - panelH) / 2
	if padTop < minPadTop {
		padTop = minPadTop
	}
	padLeft := (width - panelW) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	lines := strings.Split(panel, "\n")
	var b strings.Builder
	for range padTop {
		b.WriteString("\n")
	}
	indent := strings.Repeat(" ", padLeft)
	for i, line := range lines {
		b.WriteString(indent)
		b.WriteString(line)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// helpContent generates the static help text (keyboard shortcuts).
// Separated from rendering so it can be set once on the viewport.
func (m *Model) helpContent() string {
	type binding struct {
		key  string
		desc string
	}
	sections := []struct {
		title    string
		bindings []binding
	}{
		{
			title: "Global",
			bindings: []binding{
				{keyCtrlC, "Cancel LLM operation"},
				{keyCtrlQ, "Quit"},
			},
		},
		{
			title: "Nav Mode",
			bindings: []binding{
				{keyJ + "/" + keyK + "/" + symUp + "/" + symDown, "Rows"},
				{keyH + "/" + keyL + "/" + symLeft + "/" + symRight, "Columns"},
				{keyCaret + "/" + keyDollar, "First/last column"},
				{keyG + "/" + keyShiftG, "First/last row"},
				{keyD + "/" + keyU, "Half page down/up"},
				{keyB + "/" + keyF, "Switch tabs"},
				{keyShiftB + "/" + keyShiftF, "First/last tab"},
				{keyS + "/" + keyShiftS, "Sort / clear sorts"},
				{keyT, "Toggle settled projects"},
				{keyCtrlF, "Search documents"},
				{keySlash, "Find column"},
				{keyC + "/" + keyShiftC, "Toggle column visibility"},
				{keyShiftN, "Toggle filter"},
				{keyN, "Pin/unpin"},
				{keyBang, "Invert filter"},
				{keyCtrlN, "Clear pins and filter"},
				{symReturn, drilldownArrow + " drill / " + linkArrow + " follow / preview"},
				{keyO, "Open document"},
				{keyTab, "House profile"},
				{keyShiftU, "Toggle units"},
				{keyShiftD, "Summary"},
				{keyAt, "Ask LLM"},
				{keyI, "Edit mode"},
				{keyQuestion, "Help"},
				{keyEsc, "Close detail / clear status"},
			},
		},
		{
			title: "Edit Mode",
			bindings: []binding{
				{keyA, "Add entry"},
				{keyShiftA, "Add document with extraction"},
				{keyE, "Edit cell or row"},
				{keyShiftE, "Edit row (full form)"},
				{keyD, "Delete / restore"},
				{keyShiftD, "Permanently delete (incidents)"},
				{keyCtrlD, "Half page down"},
				{keyX, "Show deleted"},
				{keyP, "House profile"},
				{keyEsc, "Nav mode"},
			},
		},
		{
			title: "Forms",
			bindings: []binding{
				{keyTab, "Next field"},
				{keyShiftTab, "Previous field"},
				{"1-9", "Jump to Nth option"},
				{keyShiftH, "Toggle hidden files (file picker)"},
				{keyCtrlE, "Open notes in $EDITOR"},
				{keyCtrlS, "Save"},
				{keyEsc, "Cancel"},
			},
		},
		{
			title: "Chat (" + keyAt + ")",
			bindings: []binding{
				{symReturn, "Send message"},
				{keyCtrlS, "Toggle SQL display"},
				{symUp + "/" + symDown, "Prompt history"},
				{keyEsc, "Hide chat"},
			},
		},
	}

	var b strings.Builder
	b.WriteString(m.styles.HeaderTitle().Render(" Keyboard Shortcuts "))
	b.WriteString("\n\n")
	for i, section := range sections {
		b.WriteString(m.styles.HeaderSection().Render(" " + section.title + " "))
		b.WriteString("\n")
		for _, bind := range section.bindings {
			keys := m.renderKeysLight(bind.key)
			desc := m.styles.HeaderHint().Render(bind.desc)
			fmt.Fprintf(&b, "  %s  %s\n", keys, desc)
		}
		if i < len(sections)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// helpView renders the help overlay using a viewport for scrolling.
func (m *Model) helpView() string {
	vp := m.helpViewport
	if vp == nil {
		return ""
	}

	content := vp.View()
	contentW := vp.Width()
	rule := m.scrollRule(contentW, vp.TotalLineCount(), vp.Height(),
		vp.AtTop(), vp.AtBottom(), vp.ScrollPercent(), "─")

	hints := []string{m.helpItem(keyEsc, "close")}
	if vp.TotalLineCount() > vp.Height() {
		hints = append([]string{m.helpItem(keyJ+"/"+keyK, "scroll")}, hints...)
	}
	closeHintStr := joinWithSeparator(m.helpSeparator(), hints...)

	return m.styles.OverlayBox().
		Render(content + "\n\n" + rule + "\n" + closeHintStr)
}

// tableView orchestrates the full table rendering: visible projection,
// column sizing, horizontal scroll viewport, header/divider/rows, and
// hidden-column badge line.
func (m *Model) tableView(tab *Tab) string {
	if tab == nil || len(tab.Specs) == 0 {
		return ""
	}

	normalSep := m.styles.TableSeparator().Render(" │ ")
	normalDiv := m.styles.TableSeparator().Render("─┼─")
	sepW := lipgloss.Width(normalSep)

	vp := m.tabViewport(tab)
	if len(vp.Specs) == 0 {
		return ""
	}
	headerSpecs := annotateMoneyHeaders(vp.Specs, m.cur)
	header := renderHeaderRow(
		headerSpecs,
		vp.Widths,
		vp.CollapsedSeps,
		vp.Cursor,
		vp.Sorts,
		vp.HasLeft,
		vp.HasRight,
		vp.LinkCells,
		m.zones,
		zoneCol,
	)
	divider := renderDivider(vp.Widths, vp.PlainSeps, normalDiv, m.styles.TableSeparator())

	// Badge line accounts for 1 row of vertical space when visible.
	badges := renderHiddenBadges(tab.Specs, tab.ColCursor)
	badgeChrome := 0
	if badges != "" {
		badgeChrome = 1
	}

	// Row count line accounts for 1 row when visible (hidden when empty).
	rowCountChrome := 0
	if len(tab.Rows) > 0 {
		rowCountChrome = 1
	}

	effectiveHeight := tab.Table.Height() - badgeChrome - rowCountChrome
	if effectiveHeight < 2 {
		effectiveHeight = 2
	}
	// Mag and compact transforms are mutually exclusive: mag replaces
	// values with order-of-magnitude notation, compact abbreviates them.
	// Both strip the $ prefix since the header carries the unit.
	var displayCells [][]cell
	if m.magMode {
		displayCells = magTransformCells(vp.Cells, m.cur.Symbol())
	} else {
		displayCells = compactMoneyCells(vp.Cells, m.cur)
	}
	// Translate pin column indices from tab-space to viewport-space.
	pinCtx := m.viewportPinContext(tab, vp)
	rows := renderRows(
		vp.Specs,
		displayCells,
		tab.Rows,
		vp.Widths,
		vp.PlainSeps,
		vp.CollapsedSeps,
		tab.Table.Cursor(),
		vp.Cursor,
		effectiveHeight,
		pinCtx,
		m.zones,
		zoneRow,
	)

	// Assemble body (header + divider + data rows).
	bodyParts := []string{header, divider}
	if len(rows) == 0 {
		if tab.FilterActive && hasPins(tab) {
			bodyParts = append(bodyParts, m.styles.Empty().Render("No matches."))
		} else {
			bodyParts = append(bodyParts, m.styles.Empty().Render(m.emptyHint(tab)))
		}
	} else {
		bodyParts = append(bodyParts, strings.Join(rows, "\n"))
	}
	if badges != "" {
		tableWidth := sumInts(vp.Widths)
		if len(vp.Widths) > 1 {
			tableWidth += (len(vp.Widths) - 1) * sepW
		}
		centered := lipgloss.PlaceHorizontal(tableWidth, lipgloss.Center, badges)
		bodyParts = append(bodyParts, centered)
	}
	// Row count: flush-left, muted, hidden when empty (silence is success).
	if n := len(tab.Rows); n > 0 {
		label := fmt.Sprintf("%d rows", n)
		if n == 1 {
			label = "1 row"
		}
		if tab.ShowDeleted {
			var nd int
			for _, rm := range tab.Rows {
				if rm.Deleted {
					nd++
				}
			}
			if nd > 0 {
				suffix := fmt.Sprintf("%d deleted", nd)
				label += " · " + m.styles.DeletedLabel().Render(suffix)
			}
		}
		bodyParts = append(bodyParts, m.styles.Empty().Render(label))
	}
	return joinVerticalNonEmpty(bodyParts...)
}

// viewportPinContext translates the tab's pin column indices into viewport
// coordinate space so the renderer can identify pinned cells.
func (m *Model) viewportPinContext(tab *Tab, vp tableViewport) pinRenderContext {
	if !hasPins(tab) {
		return pinRenderContext{}
	}
	// Build a full→viewport column index map from VisToFull.
	fullToVP := make(map[int]int, len(vp.VisToFull))
	for vpIdx, fullIdx := range vp.VisToFull {
		fullToVP[fullIdx] = vpIdx
	}
	var translated []filterPin
	for _, pin := range tab.Pins {
		if vpIdx, ok := fullToVP[pin.Col]; ok {
			translated = append(translated, filterPin{
				Col:    vpIdx,
				Values: pin.Values,
			})
		}
	}
	return pinRenderContext{
		Pins:           translated,
		RawCells:       vp.Cells,
		MagMode:        m.magMode,
		Inverted:       tab.FilterInverted,
		CurrencySymbol: m.cur.Symbol(),
	}
}

// compositeOverlay centers fg over bg, splicing foreground lines into the
// background. Inlined from bubbletea-overlay (Apache-2.0) to drop the v1
// dependency.
func compositeOverlay(fg, bg string) string {
	if fg == "" {
		return bg
	}
	if bg == "" {
		return fg
	}

	fgW, fgH := lipgloss.Size(fg)
	bgW, bgH := lipgloss.Size(bg)

	if fgW >= bgW && fgH >= bgH {
		return fg
	}

	x := max((bgW-fgW)/2, 0)
	y := max((bgH-fgH)/2, 0)
	x = min(x, bgW-fgW)
	y = min(y, bgH-fgH)

	fgLines := strings.Split(strings.ReplaceAll(fg, "\r\n", "\n"), "\n")
	bgLines := strings.Split(strings.ReplaceAll(bg, "\r\n", "\n"), "\n")

	var sb strings.Builder
	for i, bgLine := range bgLines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if i < y || i >= y+fgH {
			sb.WriteString(bgLine)
			continue
		}

		pos := 0
		if x > 0 {
			left := ansi.Truncate(bgLine, x, "")
			pos = ansi.StringWidth(left)
			sb.WriteString(left)
			if pos < x {
				sb.WriteString(strings.Repeat(" ", x-pos))
				pos = x
			}
		}

		fgLine := fgLines[i-y]
		sb.WriteString(fgLine)
		pos += ansi.StringWidth(fgLine)

		right := ansi.TruncateLeft(bgLine, pos, "")
		bgLW := ansi.StringWidth(bgLine)
		rightW := ansi.StringWidth(right)
		if rightW <= bgLW-pos {
			sb.WriteString(strings.Repeat(" ", bgLW-rightW-pos))
		}
		sb.WriteString(right)
	}
	return sb.String()
}

// dimBackground applies ANSI faint (dim) to an already-styled string. It
// replaces full resets with reset+faint so the dim survives through existing
// color codes. Faint is applied per-line so that overlay compositing (which
// splices foreground lines into background lines) cannot permanently disrupt
// the dim state.
func dimBackground(s string) string {
	// Keep the vectorized ReplaceAll for escape-sequence substitution (fast
	// SIMD path in the runtime), but avoid Split+Join for the per-line
	// prefix by scanning for newlines with a Builder.
	s = strings.ReplaceAll(s, "\033[0m", "\033[0;2m")
	s = strings.ReplaceAll(s, "\033[22m", "\033[2m")
	return prependLines(s, "\033[2m", "\033[0m")
}

// prependLines prepends prefix to every line in s and appends suffix to
// the result, using a single Builder instead of Split+Join.
func prependLines(s, prefix, suffix string) string {
	n := strings.Count(s, "\n")
	// Each line gets prefix; total overhead = (n+1)*len(prefix) + len(suffix).
	var b strings.Builder
	b.Grow(len(s) + (n+1)*len(prefix) + len(suffix))
	b.WriteString(prefix)
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		b.WriteByte('\n')
		b.WriteString(prefix)
		s = s[i+1:]
	}
	b.WriteString(suffix)
	return b.String()
}

// cancelFaint wraps each line with ANSI "normal intensity" at the start and
// "faint" at the end. The leading \033[22m prevents dim from bleeding into
// overlay content; the trailing \033[2m re-establishes dim for the right-side
// background portion that follows the overlay in the composited output.
func cancelFaint(s string) string {
	n := strings.Count(s, "\n")
	const pre = "\033[22m"
	const suf = "\033[2m"
	var b strings.Builder
	b.Grow(len(s) + (n+1)*(len(pre)+len(suf)))
	b.WriteString(pre)
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		b.WriteString(suf)
		b.WriteByte('\n')
		b.WriteString(pre)
		s = s[i+1:]
	}
	b.WriteString(suf)
	return b.String()
}

// osc8Link wraps text in an OSC 8 hyperlink escape sequence, making it
// clickable in terminals that support the OSC 8 standard.
func osc8Link(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// --- Keycap rendering ---

func (m *Model) helpItem(keys, label string) string {
	keycaps := m.renderKeys(keys)
	desc := m.styles.HeaderHint().Render(label)
	return strings.TrimSpace(fmt.Sprintf("%s %s", keycaps, desc))
}

func (m *Model) helpSeparator() string {
	return m.styles.HeaderHint().Render(" · ")
}

func (m *Model) renderKeys(keys string) string {
	return m.renderKeysWith(keys, m.styles.Keycap())
}

func (m *Model) renderKeysLight(keys string) string {
	return m.renderKeysWith(keys, m.styles.KeycapLight())
}

func (m *Model) renderKeysWith(keys string, style lipgloss.Style) string {
	// A bare "/" is a single key, not a separator between two keys.
	if strings.TrimSpace(keys) == "/" {
		return renderKeycap("/", style)
	}
	parts := strings.Split(keys, "/")
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rendered = append(rendered, renderKeycap(part, style))
	}
	return joinWithSeparator(m.styles.HeaderHint().Render(" · "), rendered...)
}

func (m *Model) keycap(value string) string {
	return renderKeycap(value, m.styles.Keycap())
}

// renderKeycap formats a key string using the given style.
// Single letters preserve case; everything else is uppercased.
func renderKeycap(value string, style lipgloss.Style) string {
	if len(value) == 1 &&
		((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) {
		return style.Render(value)
	}
	return style.Render(strings.ToUpper(value))
}

// --- General view utilities ---

// filterNonBlank returns only the values that have visible content.
func filterNonBlank(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func joinInline(values ...string) string {
	if f := filterNonBlank(values); len(f) > 0 {
		return lipgloss.JoinHorizontal(lipgloss.Center, f...)
	}
	return ""
}

func joinVerticalNonEmpty(values ...string) string {
	if f := filterNonBlank(values); len(f) > 0 {
		return lipgloss.JoinVertical(lipgloss.Left, f...)
	}
	return ""
}

func joinWithSeparator(sep string, values ...string) string {
	return strings.Join(filterNonBlank(values), sep)
}

// clampLines truncates each line in s to maxW visible columns, appending "…"
// when truncation occurs. ANSI escape sequences are preserved.
func clampLines(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > maxW {
			lines[i] = ansi.Truncate(line, maxW, "…")
		}
	}
	return strings.Join(lines, "\n")
}

// shortenHome replaces the user's home directory prefix with "~" for
// display. Falls back to the original path if the home dir cannot be
// determined or doesn't match.
func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	// Check for home prefix followed by a path separator.
	prefix := home + string(os.PathSeparator)
	if rest, ok := strings.CutPrefix(path, prefix); ok {
		return "~" + string(os.PathSeparator) + rest
	}
	return path
}

// truncateLeft trims s from the left so the result fits within maxW visible
// columns, prepending "…" when truncation occurs. Delegates to
// ansi.TruncateLeft for correct grapheme-cluster handling.
func truncateLeft(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	sw := lipgloss.Width(s)
	if sw <= maxW {
		return s
	}
	return ansi.TruncateLeft(s, sw-maxW+1, "…")
}

// emptyHint returns a context-aware empty state message. In a detail
// drilldown the message references the sub-tab name and the parent entity
// kind so the user sees e.g. "No docs for this appliance yet." instead of
// the generic top-level message.
func (m *Model) emptyHint(tab *Tab) string {
	if m.inDetail() {
		return fmt.Sprintf("No %s for this %s yet. Press i for edit mode, then a to add one.",
			strings.ToLower(tab.Name), tab.Kind.singular())
	}
	return topLevelEmptyHint(tab.Kind)
}

// topLevelEmptyHint returns the empty-state message for a top-level tab.
var emptyHintOverrides = map[TabKind]string{
	tabQuotes:    "No quotes yet. Create a project first, then drill in and add a quote.",
	tabDocuments: "No documents yet.",
}

func topLevelEmptyHint(kind TabKind) string {
	if hint, ok := emptyHintOverrides[kind]; ok {
		return hint
	}
	return fmt.Sprintf(
		"No %s yet. Press i for edit mode, then a to add one. ? for help.",
		kind.plural(),
	)
}

// glamourStyle caches the glamour style config at init time so
// renderMarkdown never sends an OSC 11 background-color query.
// In virtual terminals like VHS the async response leaks into stdin
// and appears as literal text in focused text inputs.
var glamourStyle = func() glamouransi.StyleConfig {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return glamourstyles.NoTTYStyleConfig
	}
	if lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
		return glamourstyles.DarkStyleConfig
	}
	return glamourstyles.LightStyleConfig
}()

// markdownRenderer caches a glamour terminal renderer keyed by width.
// Embed or store a pointer in any state struct that needs markdown rendering.
type markdownRenderer struct {
	renderer *glamour.TermRenderer
	width    int
}

// renderMarkdown renders markdown text for terminal display using glamour.
// The renderer is cached and reused across calls at the same width,
// avoiding repeated JSON stylesheet parsing during streaming.
func (mr *markdownRenderer) renderMarkdown(text string, width int) string {
	if width < 10 {
		width = 10
	}
	if mr.renderer == nil || mr.width != width {
		r, err := glamour.NewTermRenderer(
			glamour.WithStyles(glamourStyle),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return wordWrap(text, width)
		}
		mr.renderer = r
		mr.width = width
	}
	out, err := mr.renderer.Render(text)
	if err != nil {
		return wordWrap(text, width)
	}
	return strings.TrimRight(out, "\n")
}

// wordWrap breaks text into lines of at most maxW visible columns, splitting
// on word boundaries when possible.
func wordWrap(text string, maxW int) string {
	if maxW <= 0 || text == "" {
		return text
	}
	var result strings.Builder
	for _, paragraph := range strings.Split(text, "\n") {
		if result.Len() > 0 {
			result.WriteByte('\n')
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			continue
		}
		lineW := 0
		for i, word := range words {
			ww := lipgloss.Width(word)
			if i == 0 {
				result.WriteString(word)
				lineW = ww
				continue
			}
			if lineW+1+ww > maxW {
				result.WriteByte('\n')
				result.WriteString(word)
				lineW = ww
			} else {
				result.WriteByte(' ')
				result.WriteString(word)
				lineW += 1 + ww
			}
		}
	}
	return result.String()
}
