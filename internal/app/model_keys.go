// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/rivo/uniseg"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// handleDashboardKeys intercepts keys that belong to the dashboard (j/k
// navigation, enter to jump) and blocks keys that affect backgrounded
// widgets. Keys like D, b/f, ?, q fall through to the normal handlers.
func (m *Model) handleDashboardKeys(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if !key.Matches(msg, m.keys.DashJump) {
		m.dash.flash = ""
	}
	switch {
	case key.Matches(msg, m.keys.DashDown):
		m.dashDown()
		return nil, true
	case key.Matches(msg, m.keys.DashUp):
		m.dashUp()
		return nil, true
	case key.Matches(msg, m.keys.DashNextSection):
		m.dashNextSection()
		return nil, true
	case key.Matches(msg, m.keys.DashPrevSection):
		m.dashPrevSection()
		return nil, true
	case key.Matches(msg, m.keys.DashTop):
		m.dashTop()
		return nil, true
	case key.Matches(msg, m.keys.DashBottom):
		m.dashBottom()
		return nil, true
	case key.Matches(msg, m.keys.DashToggle):
		m.dashToggleCurrent()
		return nil, true
	case key.Matches(msg, m.keys.DashToggleAll):
		m.dashToggleAll()
		return nil, true
	case key.Matches(msg, m.keys.DashJump):
		m.dashJump()
		return nil, true
	case key.Matches(msg, m.keys.HouseToggle):
		// Block house profile toggle on dashboard.
		return nil, true
	case key.Matches(msg, m.keys.ColLeft, m.keys.ColRight):
		// Block column movement on dashboard.
		return nil, true
	case key.Matches(msg, m.keys.Sort, m.keys.SortClear, m.keys.ColHide, m.keys.ColShowAll, m.keys.EnterEditMode, m.keys.ColFinder, m.keys.FilterPin, m.keys.FilterToggle, m.keys.FilterNegate, m.keys.YankCell):
		// Block table-specific keys on dashboard.
		return nil, true
	}
	return nil, false
}

// handleCommonKeys processes keys available in both Normal and Edit modes.
func (m *Model) handleCommonKeys(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Help):
		m.openHelp()
		return nil, true
	case key.Matches(msg, m.keys.HouseToggle):
		m.showHouse = !m.showHouse
		m.resizeTables()
		return nil, true
	case key.Matches(msg, m.keys.MagToggle):
		m.magMode = !m.magMode
		if m.chat != nil && m.chat.Visible {
			m.refreshChatViewport()
		}
		// Translate pin values on ALL tabs (not just the active one)
		// so non-visible tabs don't retain stale pin formats.
		for i := range m.tabs {
			tab := &m.tabs[i]
			if !hasPins(tab) {
				continue
			}
			translatePins(tab, m.magMode, m.cur.Symbol())
			applyRowFilter(tab, m.magMode, m.cur.Symbol())
			applySorts(tab)
		}
		for _, dc := range m.detailStack {
			if hasPins(&dc.Tab) {
				translatePins(&dc.Tab, m.magMode, m.cur.Symbol())
				applyRowFilter(&dc.Tab, m.magMode, m.cur.Symbol())
				applySorts(&dc.Tab)
			}
		}
		if tab := m.effectiveTab(); tab != nil {
			m.updateTabViewport(tab)
		}
		return nil, true
	case key.Matches(msg, m.keys.ColLeft):
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = nextVisibleCol(tab.Specs, tab.ColCursor, false)
			m.updateTabViewport(tab)
		}
		return nil, true
	case key.Matches(msg, m.keys.ColRight):
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = nextVisibleCol(tab.Specs, tab.ColCursor, true)
			m.updateTabViewport(tab)
		}
		return nil, true
	case key.Matches(msg, m.keys.ColStart):
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = firstVisibleCol(tab.Specs)
			m.updateTabViewport(tab)
		}
		return nil, true
	case key.Matches(msg, m.keys.ColEnd):
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = lastVisibleCol(tab.Specs)
			m.updateTabViewport(tab)
		}
		return nil, true
	case key.Matches(msg, m.keys.FgExtract):
		if len(m.ex.bgExtractions) > 0 {
			m.foregroundExtraction()
			return nil, true
		}
	}
	return nil, false
}

// handleNormalKeys processes keys unique to Normal mode.
func (m *Model) handleNormalKeys(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Dashboard):
		m.toggleDashboard()
		return nil, true
	case key.Matches(msg, m.keys.TabNext):
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.nextTab()
		}
		return nil, true
	case key.Matches(msg, m.keys.TabPrev):
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.prevTab()
		}
		return nil, true
	case key.Matches(msg, m.keys.TabLast):
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.switchToTab(len(m.tabs) - 1)
		}
		return nil, true
	case key.Matches(msg, m.keys.TabFirst):
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.switchToTab(0)
		}
		return nil, true
	case key.Matches(msg, m.keys.FilterPin):
		m.togglePinAtCursor()
		return nil, true
	case key.Matches(msg, m.keys.FilterToggle):
		m.toggleFilterActivation()
		return nil, true
	case key.Matches(msg, m.keys.FilterClear):
		m.clearAllPins()
		return nil, true
	case key.Matches(msg, m.keys.FilterNegate):
		m.toggleFilterInvert()
		return nil, true
	case key.Matches(msg, m.keys.Sort):
		if tab := m.effectiveTab(); tab != nil {
			toggleSort(tab, tab.ColCursor)
			applySorts(tab)
			tab.cachedVP = nil
		}
		return nil, true
	case key.Matches(msg, m.keys.SortClear):
		if tab := m.effectiveTab(); tab != nil {
			clearSorts(tab)
			applySorts(tab)
			tab.cachedVP = nil
		}
		return nil, true
	case key.Matches(msg, m.keys.ToggleUnits):
		m.toggleUnitSystem()
		return nil, true
	case key.Matches(msg, m.keys.ToggleSettled):
		if m.toggleSettledFilter() {
			return nil, true
		}
	case key.Matches(msg, m.keys.ColHide):
		m.hideCurrentColumn()
		return nil, true
	case key.Matches(msg, m.keys.ColShowAll):
		m.showAllColumns()
		return nil, true
	case key.Matches(msg, m.keys.ColFinder):
		m.openColumnFinder()
		return nil, true
	case key.Matches(msg, m.keys.DocSearch):
		if m.effectiveTab().isDocumentTab() {
			return m.openDocSearch(), true
		}
	case key.Matches(msg, m.keys.DocOpen):
		if cmd := m.openSelectedDocument(); cmd != nil {
			return cmd, true
		}
	case key.Matches(msg, m.keys.EnterEditMode):
		m.enterEditMode()
		return nil, true
	case key.Matches(msg, m.keys.Enter):
		if err := m.handleNormalEnter(); err != nil {
			m.setStatusError(err.Error())
			return nil, true
		}
		if m.mode == modeForm {
			return m.formInitCmd(), true
		}
		return nil, true
	case key.Matches(msg, m.keys.Chat):
		return m.openChat(), true
	case key.Matches(msg, m.keys.YankCell):
		// Guard nil tab before accessing ColCursor. selectedCell also checks
		// internally, but we need the tab reference for the column index.
		tab := m.effectiveTab()
		if tab == nil {
			return nil, true
		}
		c, ok := m.selectedCell(tab.ColCursor)
		if !ok || c.Null || c.Value == "" {
			m.setStatusInfo("Nothing to copy.")
			return nil, true
		}
		clipValue := c.Value
		if c.Kind == cellMoney {
			clipValue = m.cur.StripSymbol(clipValue)
		}
		var opsLabel string
		if c.Kind == cellOps {
			if result := m.yankOpsJSON(); result != nil {
				clipValue = result.data
				if result.pretty {
					opsLabel = fmt.Sprintf("JSON (%s ops)", c.Value)
				} else {
					opsLabel = fmt.Sprintf("raw data (%s ops)", c.Value)
				}
			}
		}
		prefix := appStyles.AccentText().Render("Copied: ")
		prefixW := lipgloss.Width(prefix)
		budget := m.width - prefixW
		if budget < 10 {
			budget = 10
		}
		displayValue := clipValue
		if opsLabel != "" {
			displayValue = opsLabel
		}
		displayValue = truncateForStatus(sanitizeForStatus(displayValue), budget)
		m.status = statusMsg{
			Text: prefix + yankStyle(c.Kind).Render(displayValue),
			Kind: statusStyled,
		}
		return tea.SetClipboard(clipValue), true
	case key.Matches(msg, m.keys.Escape):
		if m.inDetail() {
			m.closeDetail()
			return nil, true
		}
		m.status = statusMsg{}
		return nil, true
	}
	return nil, false
}

// opsJSON holds the result of fetching extraction-ops JSON for clipboard copy.
type opsJSON struct {
	data   string // pretty-printed or raw JSON
	pretty bool   // true if json.Indent succeeded
}

// yankOpsJSON fetches the extraction-ops JSON for the selected row's document.
// Returns nil if the data is unavailable (no row meta, fetch error, empty ops).
func (m *Model) yankOpsJSON() *opsJSON {
	meta, ok := m.selectedRowMeta()
	if !ok {
		return nil
	}
	doc, err := m.store.GetDocumentMetadata(meta.ID)
	if err != nil || len(doc.ExtractionOps) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, doc.ExtractionOps, "", "  "); err != nil {
		return &opsJSON{data: string(doc.ExtractionOps), pretty: false}
	}
	return &opsJSON{data: buf.String(), pretty: true}
}

// yankStyle returns the display style for a copied cell value based on its kind.
func yankStyle(kind cellKind) lipgloss.Style {
	switch kind {
	case cellMoney:
		return appStyles.Money()
	case cellReadonly:
		return appStyles.Readonly()
	case cellDate, cellWarranty, cellUrgency, cellDrilldown, cellOps:
		return appStyles.AccentText()
	case cellStatus:
		return appStyles.SecondaryText()
	case cellEntity:
		return appStyles.SecondaryText()
	case cellText, cellNotes, cellTelephoneNumber:
		return appStyles.DashValue()
	}
	return appStyles.DashValue()
}

// sanitizeForStatus replaces control characters and collapses whitespace
// so that free-text cell values don't break the single-line status bar.
// Uses unicode.IsControl and unicode.IsSpace to catch all Unicode
// separators (U+0085, U+2028, U+2029, etc.), not just ASCII.
func sanitizeForStatus(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

// truncateForStatus trims a string so its rendered terminal width fits within
// maxWidth columns (including room for "…" if truncated). Iterates by grapheme
// cluster via uniseg to correctly handle ZWJ emoji sequences and other
// multi-rune glyphs.
func truncateForStatus(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	const ellipsis = "…"
	ellipsisW := lipgloss.Width(ellipsis)
	budget := maxWidth - ellipsisW

	var b strings.Builder
	w := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		gw := g.Width()
		if w+gw > budget {
			break
		}
		b.WriteString(g.Str())
		w += gw
	}
	return b.String() + ellipsis
}

// handleNormalEnter handles enter in Normal mode: drill into detail views
// on drilldown columns, or follow FK links.
func (m *Model) handleNormalEnter() error {
	tab := m.effectiveTab()
	if tab == nil {
		return nil
	}
	meta, ok := m.selectedRowMeta()
	if !ok {
		return nil
	}

	col := tab.ColCursor
	if col < 0 || col >= len(tab.Specs) {
		return nil
	}
	spec := tab.Specs[col]

	// On a notes column, show the note preview overlay.
	if spec.Kind == cellNotes {
		if c, ok := m.selectedCell(col); ok && c.Value != "" {
			m.notePreview = &notePreviewState{text: c.Value, title: spec.Title}
		}
		return nil
	}

	// On an ops column, open the extraction ops tree overlay.
	if spec.Kind == cellOps {
		m.openOpsTree()
		return nil
	}

	// On a drilldown column, open the detail view for that row.
	if spec.Kind == cellDrilldown {
		return m.openDetailForRow(tab, meta.ID, spec.Title)
	}

	// On a linked column with a target, follow the FK.
	if spec.Link != nil {
		if c, ok := m.selectedCell(col); ok {
			if c.LinkID != "" {
				return m.navigateToLink(spec.Link, c.LinkID)
			}
			m.setStatusInfo("Nothing to follow.")
		}
		return nil
	}

	// On a polymorphic entity cell, resolve the target tab from the kind letter.
	if spec.Kind == cellEntity {
		if c, ok := m.selectedCell(col); ok {
			if c.LinkID != "" && len(c.Value) > 0 {
				if target, ok := entityLetterTab[c.Value[0]]; ok {
					return m.navigateToLink(&columnLink{TargetTab: target}, c.LinkID)
				}
			}
			m.setStatusInfo("Nothing to follow.")
		}
		return nil
	}

	// On the Documents tab, hint at the open-file shortcut.
	if tab.Kind == tabDocuments {
		m.setStatusInfo("Press o to open.")
		return nil
	}

	m.setStatusInfo("Press i to edit.")
	return nil
}

// handleEditKeys processes keys unique to Edit mode.
func (m *Model) handleEditKeys(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Add):
		m.startAddForm()
		return m.formInitCmd(), true
	case key.Matches(msg, m.keys.QuickAdd):
		if tab := m.effectiveTab(); tab != nil && tab.Kind == tabDocuments {
			if err := m.startQuickDocumentForm(); err != nil {
				m.setStatusError(err.Error())
			}
			return m.formInitCmd(), true
		}
		return nil, false
	case key.Matches(msg, m.keys.EditCell):
		if err := m.startCellOrFormEdit(); err != nil {
			m.setStatusError(err.Error())
			return nil, true
		}
		return m.formInitCmd(), true
	case key.Matches(msg, m.keys.EditFull):
		if err := m.startEditForm(); err != nil {
			m.setStatusError(err.Error())
			return nil, true
		}
		return m.formInitCmd(), true
	case key.Matches(msg, m.keys.Delete):
		m.toggleDeleteSelected()
		return nil, true
	case key.Matches(msg, m.keys.HardDelete):
		m.promptHardDelete()
		return nil, true
	case key.Matches(msg, m.keys.DocOpen):
		if cmd := m.openSelectedDocument(); cmd != nil {
			return cmd, true
		}
	case key.Matches(msg, m.keys.ReExtract):
		if cmd := m.extractSelectedDocument(); cmd != nil {
			return cmd, true
		}
	case key.Matches(msg, m.keys.ShowDeleted):
		m.toggleShowDeleted()
		return nil, true
	case key.Matches(msg, m.keys.HouseEdit):
		m.startHouseForm()
		return m.formInitCmd(), true
	case key.Matches(msg, m.keys.ExitEdit):
		m.enterNormalMode()
		return nil, true
	}
	return nil, false
}

func (m *Model) handleCalendarKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.CalLeft):
		calendarMove(m.calendar, -1)
	case key.Matches(msg, m.keys.CalRight):
		calendarMove(m.calendar, 1)
	case key.Matches(msg, m.keys.CalDown):
		calendarMove(m.calendar, 7)
	case key.Matches(msg, m.keys.CalUp):
		calendarMove(m.calendar, -7)
	case key.Matches(msg, m.keys.CalPrevMonth):
		calendarMoveMonth(m.calendar, -1)
	case key.Matches(msg, m.keys.CalNextMonth):
		calendarMoveMonth(m.calendar, 1)
	case key.Matches(msg, m.keys.CalPrevYear):
		calendarMoveYear(m.calendar, -1)
	case key.Matches(msg, m.keys.CalNextYear):
		calendarMoveYear(m.calendar, 1)
	case key.Matches(msg, m.keys.CalToday):
		calendarToday(m.calendar)
	case key.Matches(msg, m.keys.CalConfirm):
		m.confirmCalendar()
	case key.Matches(msg, m.keys.CalCancel):
		m.calendar = nil
		m.resetFormState()
	}
	return nil
}

func (m *Model) confirmCalendar() {
	if m.calendar == nil {
		return
	}
	dateStr := m.calendar.Cursor.Format("2006-01-02")
	if m.calendar.FieldPtr != nil {
		*m.calendar.FieldPtr = dateStr
	}
	if m.calendar.OnConfirm != nil {
		m.calendar.OnConfirm()
	}
	m.calendar = nil
}

// openCalendar opens the date picker for a form field value pointer.
func (m *Model) openCalendar(fieldPtr *string, onConfirm func()) {
	cursor := time.Now()
	var selected time.Time
	hasValue := false
	if fieldPtr != nil && *fieldPtr != "" {
		if t, err := time.ParseInLocation("2006-01-02", *fieldPtr, time.Local); err == nil {
			cursor = t
			selected = t
			hasValue = true
		}
	}
	m.calendar = &calendarState{
		Cursor:    cursor,
		Selected:  selected,
		HasValue:  hasValue,
		FieldPtr:  fieldPtr,
		OnConfirm: onConfirm,
	}
}
