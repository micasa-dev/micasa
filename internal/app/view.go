// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	overlay "github.com/rmhubbert/bubbletea-overlay"
)

func (m *Model) buildView() string {
	if m.terminalTooSmall() {
		return m.buildTerminalTooSmallView()
	}

	if m.mode == modeForm && m.form != nil && m.formKind == formHouse {
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
		{m.showNotePreview, m.buildNotePreviewOverlay},
		{m.columnFinder != nil, m.buildColumnFinderOverlay},
		{m.extraction != nil && m.extraction.Visible, m.buildExtractionOverlay},
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
			fg := cancelFaint(o.render())
			base = overlay.Composite(fg, dimBackground(base), overlay.Center, overlay.Center, 0, 0)
		}
	}

	return base
}

func (m *Model) buildTerminalTooSmallView() string {
	width := m.effectiveWidth()
	height := m.effectiveHeight()

	panel := lipgloss.JoinVertical(
		lipgloss.Center,
		m.styles.Error.Render("Terminal too small"),
		"",
		m.styles.HeaderHint.Render(
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
	if m.mode == modeForm && m.form != nil {
		if legend := m.requiredLegend(); legend != "" {
			content = legend + "\n\n" + m.form.View()
		} else {
			content = m.form.View()
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
			label := m.styles.HeaderHint.Render(path)
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
	hints := joinWithSeparator(m.helpSeparator(),
		m.helpItem("D", "close"),
		m.helpItem("?", "help"),
	)

	// Budget for dashboardView content: outer box height minus chrome.
	// Chrome: border (2) + padding (2) + header (1) + rule (1) + blank (1)
	// + hints (1) = 8 lines.
	maxH := m.effectiveHeight() - 4
	if maxH < 10 {
		maxH = 10
	}
	contentBudget := maxH - 8
	if contentBudget < 3 {
		contentBudget = 3
	}
	m.prepareDashboardView()
	content := m.dashboardView(contentBudget, innerW)

	rule := m.styles.DashRule.Render(strings.Repeat("─", innerW))
	boxContent := lipgloss.JoinVertical(
		lipgloss.Left, header, rule, content, "", hints,
	)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2).
		Width(contentW).
		MaxHeight(maxH)

	return box.Render(boxContent)
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
		if i == m.active {
			parts = append(parts, m.styles.TabActive.Render(tab.Name))
		} else if pinned {
			parts = append(parts, m.styles.TabLocked.Render(tab.Name))
		} else {
			parts = append(parts, m.styles.TabInactive.Render(tab.Name))
		}
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
			parts = append(parts, " "+m.styles.FilterMark.Render(mark)+" ")
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

	arrow := m.styles.BreadcrumbArrow.Render(breadcrumbSep)

	// Collect all breadcrumb segments from the stack.
	var parts []string
	for _, dc := range m.detailStack {
		parts = append(parts, strings.Split(dc.Breadcrumb, breadcrumbSep)...)
	}

	rendered := make([]string, len(parts))
	for i, p := range parts {
		if i < len(parts)-1 {
			rendered[i] = m.styles.HeaderHint.Render(p)
		} else {
			rendered[i] = m.styles.Breadcrumb.Render(p)
		}
	}
	crumb := strings.Join(rendered, arrow)
	back := m.styles.HeaderHint.Render(" (")
	back += m.keycap("esc")
	back += m.styles.HeaderHint.Render(" back)")
	return crumb + back
}

func (m *Model) tabUnderline() string {
	return m.styles.TabUnderline.Render(strings.Repeat("━", m.effectiveWidth()))
}

func (m *Model) statusView() string {
	if m.inlineInput != nil {
		return m.withPullProgress(m.inlineInputStatusView())
	}
	if m.mode == modeForm {
		if m.confirmDiscard {
			prompt := m.styles.FormDirty.Render("Discard unsaved changes?")
			hints := joinWithSeparator(
				m.helpSeparator(),
				m.helpItem("y", "discard"),
				m.helpItem("n", "keep editing"),
			)
			return m.withPullProgress(prompt + "  " + hints)
		}
		dirtyIndicator := m.styles.FormClean.Render("○ saved")
		if m.formDirty {
			dirtyIndicator = m.styles.FormDirty.Render("● unsaved")
		}
		parts := []string{
			dirtyIndicator,
			m.helpItem("ctrl+s", "save"),
		}
		if m.notesEditMode {
			parts = append(parts, m.helpItem("ctrl+e", "editor"))
		}
		parts = append(parts,
			m.helpItem("esc", "cancel"),
			m.helpItem("ctrl+q", "quit"),
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
	navW := lipgloss.Width(m.styles.ModeNormal.Render("NAV"))
	editW := lipgloss.Width(m.styles.ModeEdit.Render("EDIT"))
	badgeWidth := navW
	if editW > badgeWidth {
		badgeWidth = editW
	}
	modeBadge := m.styles.ModeNormal.
		Width(badgeWidth).
		Align(lipgloss.Center).
		Render("NAV")
	if m.mode == modeEdit {
		modeBadge = m.styles.ModeEdit.
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

	return m.withPullProgress(m.withStatusMessage(help))
}

func (m *Model) inlineInputStatusView() string {
	ii := m.inlineInput
	title := m.styles.HeaderLabel.Render(ii.Title + ":")
	input := ii.Input.View()
	hints := joinWithSeparator(
		m.helpSeparator(),
		m.helpItem("\u21b5", "save"),
		m.helpItem("esc", "cancel"),
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
			full:     m.helpItem("\u21b5", hint),
			compact:  m.helpItem("\u21b5", "open"),
			priority: 2,
		})
	}

	// Primary actions.
	hints = append(hints,
		statusHint{id: "edit", full: m.helpItem("i", "edit"), priority: 2},
	)
	if m.effectiveTab().isDocumentTab() {
		hints = append(hints, statusHint{
			id: "open", full: m.helpItem("o", "open"), priority: 2,
		})
	}
	if m.llmClient != nil {
		hints = append(hints, statusHint{
			id:       "ask",
			full:     m.helpItem("@", "ask"),
			priority: 3,
		})
	}

	// Anchors: help is always visible; back only in detail view.
	hints = append(hints, statusHint{
		id:       "help",
		full:     m.helpItem("?", "help"),
		compact:  m.helpItem("?", "more"),
		priority: 0,
		required: true,
	})
	if m.inDetail() {
		hints = append(hints, statusHint{
			id:       "exit",
			full:     m.helpItem("esc", "back"),
			compact:  m.renderKeys("esc"),
			priority: 0,
			required: true,
		})
	}
	return hints
}

// editModeStatusHelp renders the edit mode status bar with only primary
// actions. Undo/redo and profile are discoverable via the help overlay.
func (m *Model) editModeStatusHelp(modeBadge string) string {
	hints := []statusHint{
		{id: "mode", full: modeBadge, priority: 0, required: true},
	}
	if m.effectiveTab().isDocumentTab() {
		hints = append(hints, statusHint{
			id: "magicadd", full: m.helpItem("A", "\U0001FA84 add"), priority: 3,
		})
	}
	hints = append(hints,
		statusHint{id: "add", full: m.helpItem("a", "add"), priority: 1},
		statusHint{id: "edit", full: m.helpItem("e", m.editHint()), priority: 1},
		statusHint{
			id:       "del",
			full:     m.helpItem("d", "del/restore"),
			compact:  m.helpItem("d", "del"),
			priority: 2,
		},
	)
	if m.effectiveTab().isDocumentTab() {
		hints = append(hints, statusHint{
			id: "open", full: m.helpItem("o", "open"), priority: 2,
		})
	}
	hints = append(hints, statusHint{
		id:       "exit",
		full:     m.helpItem("esc", "nav"),
		compact:  m.renderKeys("esc"),
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
	style := m.styles.Info
	if m.status.Kind == statusError {
		style = m.styles.Error
	}
	return lipgloss.JoinVertical(lipgloss.Left, style.Render(m.status.Text), helpLine)
}

// withPullProgress appends the model download progress line below the status
// output when a pull is active.
func (m *Model) withPullProgress(statusOutput string) string {
	if m.pullDisplay == "" {
		return statusOutput
	}
	progressLine := lipgloss.NewStyle().Foreground(textDim).Render(m.pullDisplay)
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
		if c, ok := m.selectedCell(col); ok && c.LinkID > 0 {
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
	if spec.Kind == cellDrilldown {
		return m.drilldownHint(tab, spec)
	}
	if spec.Link != nil || spec.Kind == cellEntity {
		if c, ok := m.selectedCell(col); ok && c.LinkID > 0 {
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
	formContent := m.form.View()
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
	grid := calendarGrid(*m.calendar, m.styles)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2).
		Render(grid)
}

func (m *Model) buildNotePreviewOverlay() string {
	contentW := m.overlayContentWidth()

	var b strings.Builder
	title := m.notePreviewTitle
	if title == "" {
		title = "Notes"
	}
	b.WriteString(m.styles.HeaderSection.Render(" " + title + " "))
	b.WriteString("\n\n")

	// Word-wrap the note text to fit within the box.
	innerW := contentW - 4 // padding
	text := m.notePreviewText
	b.WriteString(wordWrap(text, innerW))
	b.WriteString("\n\n")

	b.WriteString(m.styles.HeaderHint.Render("Press any key to close"))

	maxH := m.effectiveHeight() - 4
	if maxH < 10 {
		maxH = 10
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2).
		Width(contentW).
		MaxHeight(maxH).
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
	for i := 0; i < padTop; i++ {
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
				{"ctrl+c", "Cancel LLM operation"},
				{"ctrl+q", "Quit"},
			},
		},
		{
			title: "Nav Mode",
			bindings: []binding{
				{"j/k/\u2191/\u2193", "Rows"},
				{"h/l/\u2190/\u2192", "Columns"},
				{"^/$", "First/last column"},
				{"g/G", "First/last row"},
				{"d/u", "Half page down/up"},
				{"b/f", "Switch tabs"},
				{"B/F", "First/last tab"},
				{"s/S", "Sort / clear sorts"},
				{"t", "Toggle settled projects"},
				{"/", "Find column"},
				{"c/C", "Toggle column visibility"},
				{"N", "Toggle filter"},
				{"n", "Pin/unpin"},
				{"!", "Invert filter"},
				{keyCtrlN, "Clear pins and filter"},
				{"\u21b5", drilldownArrow + " drill / " + linkArrow + " follow / preview"},
				{"o", "Open document"},
				{"tab", "House profile"},
				{"D", "Summary"},
				{"@", "Ask LLM"},
				{"i", "Edit mode"},
				{"?", "Help"},
				{"esc", "Close detail / clear status"},
			},
		},
		{
			title: "Edit Mode",
			bindings: []binding{
				{"a", "Add entry"},
				{"e", "Edit cell or row"},
				{"d", "Delete / restore"},
				{"u/r", "Undo / redo"},
				{"ctrl+d/ctrl+u", "Half page down/up"},
				{"x", "Show deleted"},
				{"p", "House profile"},
				{"esc", "Nav mode"},
			},
		},
		{
			title: "Forms",
			bindings: []binding{
				{"tab", "Next field"},
				{"shift+tab", "Previous field"},
				{"1-9", "Jump to Nth option"},
				{"ctrl+e", "Open notes in $EDITOR"},
				{"ctrl+s", "Save"},
				{"esc", "Cancel"},
			},
		},
		{
			title: "Chat (@)",
			bindings: []binding{
				{"\u21b5", "Send message"},
				{"ctrl+s", "Toggle SQL display"},
				{"\u2191/\u2193", "Prompt history"},
				{"esc", "Hide chat"},
			},
		},
	}

	var b strings.Builder
	b.WriteString(m.styles.HeaderTitle.Render(" Keyboard Shortcuts "))
	b.WriteString("\n\n")
	for i, section := range sections {
		b.WriteString(m.styles.HeaderSection.Render(" " + section.title + " "))
		b.WriteString("\n")
		for _, bind := range section.bindings {
			keys := m.renderKeys(bind.key)
			desc := m.styles.HeaderHint.Render(bind.desc)
			b.WriteString(fmt.Sprintf("  %s  %s\n", keys, desc))
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
	contentW := vp.Width
	ruleStyle := lipgloss.NewStyle().Foreground(border)

	// Embed a Vim-style scroll indicator in the rule when content overflows.
	var rule string
	if vp.TotalLineCount() > vp.Height {
		var label string
		switch {
		case vp.AtTop():
			label = "Top"
		case vp.AtBottom():
			label = "Bot"
		default:
			label = fmt.Sprintf("%d%%", int(vp.ScrollPercent()*100))
		}
		indicator := lipgloss.NewStyle().Foreground(textDim).Render(" " + label + " ")
		indicatorW := lipgloss.Width(indicator)
		rightW := max(0, contentW-indicatorW)
		rule = ruleStyle.Render(strings.Repeat("─", rightW)) + indicator
	} else {
		rule = ruleStyle.Render(strings.Repeat("─", contentW))
	}

	hints := []string{m.helpItem("esc", "close")}
	if vp.TotalLineCount() > vp.Height {
		hints = append([]string{m.helpItem("j/k", "scroll")}, hints...)
	}
	closeHintStr := joinWithSeparator(m.helpSeparator(), hints...)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2).
		Render(content + "\n\n" + rule + "\n" + closeHintStr)
}

// tableView orchestrates the full table rendering: visible projection,
// column sizing, horizontal scroll viewport, header/divider/rows, and
// hidden-column badge line.
func (m *Model) tableView(tab *Tab) string {
	if tab == nil || len(tab.Specs) == 0 {
		return ""
	}

	width := m.effectiveWidth()
	normalSep := m.styles.TableSeparator.Render(" │ ")
	normalDiv := m.styles.TableSeparator.Render("─┼─")
	sepW := lipgloss.Width(normalSep)

	vp := computeTableViewport(tab, width, normalSep, m.styles)
	if len(vp.Specs) == 0 {
		return ""
	}
	headerSpecs := annotateMoneyHeaders(vp.Specs, m.styles)
	header := renderHeaderRow(
		headerSpecs,
		vp.Widths,
		vp.CollapsedSeps,
		vp.Cursor,
		vp.Sorts,
		vp.HasLeft,
		vp.HasRight,
		vp.LinkCells,
		m.styles,
	)
	divider := renderDivider(vp.Widths, vp.PlainSeps, normalDiv, m.styles.TableSeparator)

	// Badge line accounts for 1 row of vertical space when visible.
	badges := renderHiddenBadges(tab.Specs, tab.ColCursor, m.styles)
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
		displayCells = magTransformCells(vp.Cells)
	} else {
		displayCells = compactMoneyCells(vp.Cells)
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
		m.styles,
		pinCtx,
	)

	// Assemble body (header + divider + data rows).
	bodyParts := []string{header, divider}
	if len(rows) == 0 {
		if tab.FilterActive && hasPins(tab) {
			bodyParts = append(bodyParts, m.styles.Empty.Render("No matches."))
		} else {
			bodyParts = append(bodyParts, m.styles.Empty.Render(m.emptyHint(tab)))
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
				label += " · " + m.styles.DeletedLabel.Render(suffix)
			}
		}
		bodyParts = append(bodyParts, m.styles.Empty.Render(label))
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
		Pins:     translated,
		RawCells: vp.Cells,
		MagMode:  m.magMode,
		Inverted: tab.FilterInverted,
	}
}

// dimBackground applies ANSI faint (dim) to an already-styled string. It
// replaces full resets with reset+faint so the dim survives through existing
// color codes. Faint is applied per-line so that overlay compositing (which
// splices foreground lines into background lines) cannot permanently disrupt
// the dim state.
func dimBackground(s string) string {
	dimmed := strings.ReplaceAll(s, "\033[0m", "\033[0;2m")
	// Neutralize cancel-faint markers left by a previous cancelFaint pass
	// (nested overlays). Without this, \033[22m codes embedded in an
	// earlier overlay's content would override the dim we're applying.
	dimmed = strings.ReplaceAll(dimmed, "\033[22m", "\033[2m")
	lines := strings.Split(dimmed, "\n")
	for i, line := range lines {
		lines[i] = "\033[2m" + line
	}
	return strings.Join(lines, "\n") + "\033[0m"
}

// cancelFaint wraps each line with ANSI "normal intensity" at the start and
// "faint" at the end. The leading \033[22m prevents dim from bleeding into
// overlay content; the trailing \033[2m re-establishes dim for the right-side
// background portion that follows the overlay in the composited output.
func cancelFaint(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = "\033[22m" + line + "\033[2m"
	}
	return strings.Join(lines, "\n")
}

// --- Keycap rendering ---

func (m *Model) helpItem(keys, label string) string {
	keycaps := m.renderKeys(keys)
	desc := m.styles.HeaderHint.Render(label)
	return strings.TrimSpace(fmt.Sprintf("%s %s", keycaps, desc))
}

func (m *Model) helpSeparator() string {
	return m.styles.HeaderHint.Render(" · ")
}

func (m *Model) renderKeys(keys string) string {
	// A bare "/" is a single key, not a separator between two keys.
	if strings.TrimSpace(keys) == "/" {
		return m.keycap("/")
	}
	parts := strings.Split(keys, "/")
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rendered = append(rendered, m.keycap(part))
	}
	return joinWithSeparator(m.styles.HeaderHint.Render(" · "), rendered...)
}

func (m *Model) keycap(value string) string {
	// Single letters: preserve case to distinguish A from a.
	if len(value) == 1 &&
		((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) {
		return m.styles.Keycap.Render(value)
	}
	return m.styles.Keycap.Render(strings.ToUpper(value))
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
func topLevelEmptyHint(kind TabKind) string {
	switch kind {
	case tabProjects:
		return "No projects yet. Press i for edit mode, then a to add one. ? for help."
	case tabQuotes:
		return "No quotes yet. Create a project first, then drill in and add a quote."
	case tabMaintenance:
		return "No maintenance items yet. Press i for edit mode, then a to add one. ? for help."
	case tabIncidents:
		return "No incidents yet. Press i for edit mode, then a to add one. ? for help."
	case tabAppliances:
		return "No appliances yet. Press i for edit mode, then a to add one. ? for help."
	case tabVendors:
		return "No vendors yet. Press i for edit mode, then a to add one. ? for help."
	case tabDocuments:
		return "No documents yet."
	}
	panic(fmt.Sprintf("unhandled TabKind: %d", kind))
}

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
			glamour.WithAutoStyle(),
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
