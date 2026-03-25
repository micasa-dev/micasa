// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// handleDashboardKeys intercepts keys that belong to the dashboard (j/k
// navigation, enter to jump) and blocks keys that affect backgrounded
// widgets. Keys like D, b/f, ?, q fall through to the normal handlers.
func (m *Model) handleDashboardKeys(key tea.KeyPressMsg) (tea.Cmd, bool) {
	if key.String() != keyEnter {
		m.dash.flash = ""
	}
	switch key.String() {
	case keyJ, keyDown:
		m.dashDown()
		return nil, true
	case keyK, keyUp:
		m.dashUp()
		return nil, true
	case keyShiftJ, keyShiftDown:
		m.dashNextSection()
		return nil, true
	case keyShiftK, keyShiftUp:
		m.dashPrevSection()
		return nil, true
	case keyG:
		m.dashTop()
		return nil, true
	case keyShiftG:
		m.dashBottom()
		return nil, true
	case keyE:
		m.dashToggleCurrent()
		return nil, true
	case keyShiftE:
		m.dashToggleAll()
		return nil, true
	case keyEnter:
		m.dashJump()
		return nil, true
	case keyTab:
		// Block house profile toggle on dashboard.
		return nil, true
	case keyH, keyL, keyLeft, keyRight:
		// Block column movement on dashboard.
		return nil, true
	case keyS, keyShiftS, keyC, keyShiftC, keyI, keySlash, keyN, keyShiftN, keyBang:
		// Block table-specific keys on dashboard.
		return nil, true
	}
	return nil, false
}

// handleCommonKeys processes keys available in both Normal and Edit modes.
func (m *Model) handleCommonKeys(key tea.KeyPressMsg) (tea.Cmd, bool) {
	switch key.String() {
	case keyQuestion:
		m.openHelp()
		return nil, true
	case keyTab:
		m.showHouse = !m.showHouse
		m.resizeTables()
		return nil, true
	case keyCtrlO:
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
	case keyH, keyLeft:
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = nextVisibleCol(tab.Specs, tab.ColCursor, false)
			m.updateTabViewport(tab)
		}
		return nil, true
	case keyL, keyRight:
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = nextVisibleCol(tab.Specs, tab.ColCursor, true)
			m.updateTabViewport(tab)
		}
		return nil, true
	case keyCaret:
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = firstVisibleCol(tab.Specs)
			m.updateTabViewport(tab)
		}
		return nil, true
	case keyDollar:
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = lastVisibleCol(tab.Specs)
			m.updateTabViewport(tab)
		}
		return nil, true
	case keyCtrlB:
		if len(m.ex.bgExtractions) > 0 {
			m.foregroundExtraction()
			return nil, true
		}
	}
	return nil, false
}

// handleNormalKeys processes keys unique to Normal mode.
func (m *Model) handleNormalKeys(key tea.KeyPressMsg) (tea.Cmd, bool) {
	switch key.String() {
	case keyShiftD:
		m.toggleDashboard()
		return nil, true
	case keyF:
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.nextTab()
		}
		return nil, true
	case keyB:
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.prevTab()
		}
		return nil, true
	case keyShiftF:
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.switchToTab(len(m.tabs) - 1)
		}
		return nil, true
	case keyShiftB:
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.switchToTab(0)
		}
		return nil, true
	case keyN:
		m.togglePinAtCursor()
		return nil, true
	case keyShiftN:
		m.toggleFilterActivation()
		return nil, true
	case keyCtrlN:
		m.clearAllPins()
		return nil, true
	case keyBang:
		m.toggleFilterInvert()
		return nil, true
	case keyS:
		if tab := m.effectiveTab(); tab != nil {
			toggleSort(tab, tab.ColCursor)
			applySorts(tab)
			tab.cachedVP = nil
		}
		return nil, true
	case keyShiftS:
		if tab := m.effectiveTab(); tab != nil {
			clearSorts(tab)
			applySorts(tab)
			tab.cachedVP = nil
		}
		return nil, true
	case keyShiftU:
		m.toggleUnitSystem()
		return nil, true
	case keyT:
		if m.toggleSettledFilter() {
			return nil, true
		}
	case keyC:
		m.hideCurrentColumn()
		return nil, true
	case keyShiftC:
		m.showAllColumns()
		return nil, true
	case keySlash:
		m.openColumnFinder()
		return nil, true
	case keyCtrlF:
		if m.effectiveTab().isDocumentTab() {
			return m.openDocSearch(), true
		}
	case keyO:
		if cmd := m.openSelectedDocument(); cmd != nil {
			return cmd, true
		}
	case keyI:
		m.enterEditMode()
		return nil, true
	case keyEnter:
		if err := m.handleNormalEnter(); err != nil {
			m.setStatusError(err.Error())
			return nil, true
		}
		if m.mode == modeForm {
			return m.formInitCmd(), true
		}
		return nil, true
	case keyAt:
		return m.openChat(), true
	case keyEsc:
		if m.inDetail() {
			m.closeDetail()
			return nil, true
		}
		m.status = statusMsg{}
		return nil, true
	}
	return nil, false
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
func (m *Model) handleEditKeys(key tea.KeyPressMsg) (tea.Cmd, bool) {
	switch key.String() {
	case keyA:
		m.startAddForm()
		return m.formInitCmd(), true
	case keyShiftA:
		if tab := m.effectiveTab(); tab != nil && tab.Kind == tabDocuments {
			var entity entityRef
			if dc := m.detail(); dc != nil && dc.EntityKind != "" {
				entity = entityRef{Kind: dc.EntityKind, ID: dc.ParentRowID}
			}
			return m.startBatchDocOverlay(entity), true
		}
		return nil, false
	case keyE:
		if err := m.startCellOrFormEdit(); err != nil {
			m.setStatusError(err.Error())
			return nil, true
		}
		return m.formInitCmd(), true
	case keyShiftE:
		if err := m.startEditForm(); err != nil {
			m.setStatusError(err.Error())
			return nil, true
		}
		return m.formInitCmd(), true
	case keyD:
		m.toggleDeleteSelected()
		return nil, true
	case keyShiftD:
		m.promptHardDelete()
		return nil, true
	case keyO:
		if cmd := m.openSelectedDocument(); cmd != nil {
			return cmd, true
		}
	case keyR:
		if cmd := m.extractSelectedDocument(); cmd != nil {
			return cmd, true
		}
	case keyX:
		m.toggleShowDeleted()
		return nil, true
	case keyP:
		m.startHouseForm()
		return m.formInitCmd(), true
	case keyEsc:
		m.enterNormalMode()
		return nil, true
	}
	return nil, false
}

func (m *Model) handleCalendarKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case keyH, keyLeft:
		calendarMove(m.calendar, -1)
	case keyL, keyRight:
		calendarMove(m.calendar, 1)
	case keyJ, keyDown:
		calendarMove(m.calendar, 7)
	case keyK, keyUp:
		calendarMove(m.calendar, -7)
	case keyShiftH:
		calendarMoveMonth(m.calendar, -1)
	case keyShiftL:
		calendarMoveMonth(m.calendar, 1)
	case keyLBracket:
		calendarMoveYear(m.calendar, -1)
	case keyRBracket:
		calendarMoveYear(m.calendar, 1)
	case "t":
		calendarToday(m.calendar)
	case keyEnter:
		m.confirmCalendar()
	case keyEsc:
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
