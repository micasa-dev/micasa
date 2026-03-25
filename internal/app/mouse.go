// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// doubleClickThreshold is the maximum duration between two clicks on the
// same row for them to count as a double-click.
const doubleClickThreshold = 300 * time.Millisecond

// rowClickState tracks the last row click for double-click detection.
type rowClickState struct {
	at  time.Time
	row int
}

// Zone ID prefixes for clickable UI regions.
const (
	zoneTab        = "tab-"
	zoneRow        = "row-"
	zoneCol        = "col-"
	zoneHint       = "hint-"
	zoneDashRow    = "dash-"
	zoneHouse      = "house-header"
	zoneBreadcrumb = "breadcrumb-back"
	zoneOverlay    = "overlay"

	// Extraction preview uses distinct prefixes to avoid colliding with
	// main table row-N/col-N zones during overlay compositing. Without
	// separate IDs the scanner mis-pairs interleaved markers.
	zoneExtTab = "ext-tab-"
	zoneExtRow = "ext-row-"
	zoneExtCol = "ext-col-"

	// Batch document staging overlay: each staged file row.
	zoneBatchFile = "batch-file-"
)

// handleMouseClick dispatches click events to the appropriate handler.
func (m *Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button == tea.MouseLeft {
		return m.handleLeftClick(msg)
	}
	return m, nil
}

// handleMouseWheel dispatches wheel events to scroll handlers.
func (m *Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseWheelUp:
		return m.handleScroll(-1)
	case tea.MouseWheelDown:
		return m.handleScroll(1)
	}
	return m, nil
}

// handleLeftClick routes a left click to the appropriate zone handler.
func (m *Model) handleLeftClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	// Overlay dismiss: if an overlay is active and the click is outside it,
	// dismiss the overlay (same as pressing esc).
	if m.hasActiveOverlay() {
		if m.zones.Get(zoneOverlay).InBounds(msg) {
			return m.handleOverlayClick(msg)
		}
		m.dismissActiveOverlay()
		return m, nil
	}

	// Tab click.
	if !m.tabsLocked() && !m.inDetail() {
		for i := range m.tabs {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneTab, i)).InBounds(msg) {
				if i != m.active {
					m.switchToTab(i)
				}
				return m, nil
			}
		}
	}

	// Breadcrumb back click.
	if m.inDetail() {
		if m.zones.Get(zoneBreadcrumb).InBounds(msg) {
			m.closeDetail()
			return m, nil
		}
	}

	// House header click.
	if m.zones.Get(zoneHouse).InBounds(msg) {
		m.showHouse = !m.showHouse
		m.resizeTables()
		return m, nil
	}

	// Column header click.
	if tab := m.effectiveTab(); tab != nil {
		vp := m.tabViewport(tab)
		for i := range vp.Specs {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneCol, i)).InBounds(msg) {
				if i < len(vp.VisToFull) {
					tab.ColCursor = vp.VisToFull[i]
					m.updateTabViewport(tab)
				}
				return m, nil
			}
		}
	}

	// Row click: single click selects row+column, double-click drills down.
	if tab := m.effectiveTab(); tab != nil {
		total := len(tab.CellRows)
		if total > 0 {
			cursor := tab.Table.Cursor()
			height := tab.Table.Height()
			// Account for chrome lines (badge, row count).
			badges := renderHiddenBadges(tab.Specs, tab.ColCursor)
			if badges != "" {
				height--
			}
			if len(tab.Rows) > 0 {
				height--
			}
			if height < 2 {
				height = 2
			}
			start, end := visibleRange(total, height, cursor)
			for i := start; i < end; i++ {
				if m.zones.Get(fmt.Sprintf("%s%d", zoneRow, i)).InBounds(msg) {
					now := time.Now()
					isDouble := m.lastRowClick.row == i &&
						!m.lastRowClick.at.IsZero() &&
						now.Sub(m.lastRowClick.at) <= doubleClickThreshold
					if isDouble && m.mode == modeNormal {
						m.lastRowClick = rowClickState{}
						if err := m.handleNormalEnter(); err != nil {
							m.setStatusError(err.Error())
						}
					} else {
						tab.Table.SetCursor(i)
						m.selectClickedColumn(tab, msg)
						m.lastRowClick = rowClickState{at: now, row: i}
					}
					return m, nil
				}
			}
		}
	}

	// Status hint clicks.
	return m.handleHintClick(msg)
}

// handleHintClick checks if a click landed on a status bar hint and triggers
// the corresponding action.
func (m *Model) handleHintClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	type hintAction struct {
		id     string
		action func() (tea.Model, tea.Cmd)
	}
	actions := []hintAction{
		{"edit", func() (tea.Model, tea.Cmd) {
			if m.mode == modeNormal {
				m.enterEditMode()
			}
			return m, nil
		}},
		{"help", func() (tea.Model, tea.Cmd) {
			m.openHelp()
			return m, nil
		}},
		{"add", func() (tea.Model, tea.Cmd) {
			if m.mode == modeEdit {
				m.startAddForm()
				return m, m.formInitCmd()
			}
			return m, nil
		}},
		{"exit", func() (tea.Model, tea.Cmd) {
			if m.mode == modeEdit {
				m.enterNormalMode()
			} else if m.inDetail() {
				m.closeDetail()
			}
			return m, nil
		}},
		{"enter", func() (tea.Model, tea.Cmd) {
			if m.mode == modeNormal {
				if err := m.handleNormalEnter(); err != nil {
					m.setStatusError(err.Error())
				}
				if m.mode == modeForm {
					return m, m.formInitCmd()
				}
			}
			return m, nil
		}},
		{"del", func() (tea.Model, tea.Cmd) {
			if m.mode == modeEdit {
				m.toggleDeleteSelected()
			}
			return m, nil
		}},
		{"open", func() (tea.Model, tea.Cmd) {
			if cmd := m.openSelectedDocument(); cmd != nil {
				return m, nil
			}
			return m, nil
		}},
		{"search", func() (tea.Model, tea.Cmd) {
			if m.mode == modeNormal && m.effectiveTab().isDocumentTab() {
				return m, m.openDocSearch()
			}
			return m, nil
		}},
		{"ask", func() (tea.Model, tea.Cmd) {
			if m.mode == modeNormal {
				return m, m.openChat()
			}
			return m, nil
		}},
	}
	for _, ha := range actions {
		if m.zones.Get(zoneHint + ha.id).InBounds(msg) {
			return ha.action()
		}
	}
	return m, nil
}

// handleOverlayClick handles clicks within an active overlay.
func (m *Model) handleOverlayClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	// Dashboard row clicks: single click selects, double-click jumps.
	if m.dashboardVisible() {
		for i := range m.dash.nav {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneDashRow, i)).InBounds(msg) {
				now := time.Now()
				isDouble := m.lastDashClick.row == i &&
					!m.lastDashClick.at.IsZero() &&
					now.Sub(m.lastDashClick.at) <= doubleClickThreshold
				if isDouble {
					m.lastDashClick = rowClickState{}
					m.dashJump()
				} else {
					m.dash.cursor = i
					m.lastDashClick = rowClickState{at: now, row: i}
				}
				return m, nil
			}
		}
	}

	// Search result clicks: single click selects, double-click navigates.
	if ds := m.docSearch; ds != nil {
		for i := range ds.Results {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneSearchRow, i)).InBounds(msg) {
				ds.Cursor = i
				m.docSearchNavigate()
				return m, nil
			}
		}
	}

	// Ops tree node clicks: toggle expand/collapse.
	if tree := m.opsTree; tree != nil {
		nodes := tree.visibleNodes()
		for i := range nodes {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneOpsNode, i)).InBounds(msg) {
				tree.cursor = i
				if nodes[i].isExpandable() {
					tree.expanded[nodes[i].path] = !tree.expanded[nodes[i].path]
					tree.clampCursor()
				}
				return m, nil
			}
		}

		// Ops tree tab clicks: switch preview tab.
		for i := range tree.previewGroups {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneOpsTab, i)).InBounds(msg) {
				tree.previewTab = i
				return m, nil
			}
		}
	}

	// Batch document staging overlay: click on a staged file row selects it.
	if bd := m.batchDoc; bd != nil && bd.phase == batchPhaseStaging {
		for i := range bd.files {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneBatchFile, i)).InBounds(msg) {
				bd.cursor = i
				bd.focus = batchFocusList
				return m, nil
			}
		}
	}

	// Extraction preview clicks: tab switch, row select, column select.
	if ex := m.ex.extraction; ex != nil && ex.Visible && ex.exploring {
		for i := range ex.previewGroups {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneExtTab, i)).InBounds(msg) {
				ex.previewTab = i
				ex.previewRow = 0
				ex.previewCol = 0
				return m, nil
			}
		}
		if g := ex.activePreviewGroup(); g != nil {
			for i := range g.cells {
				if m.zones.Get(fmt.Sprintf("%s%d", zoneExtRow, i)).InBounds(msg) {
					ex.previewRow = i
					m.selectExtractionPreviewColumn(ex, g, msg)
					return m, nil
				}
			}
			for i := range g.specs {
				if m.zones.Get(fmt.Sprintf("%s%d", zoneExtCol, i)).InBounds(msg) {
					ex.previewCol = i
					return m, nil
				}
			}
		}
	}

	return m, nil
}

// selectExtractionPreviewColumn updates the extraction preview column cursor
// to match the column zone the click's X coordinate falls within.
func (m *Model) selectExtractionPreviewColumn(
	ex *extractionLogState, g *previewTableGroup, msg tea.MouseClickMsg,
) {
	for i := range g.specs {
		z := m.zones.Get(fmt.Sprintf("%s%d", zoneExtCol, i))
		if z == nil || z.IsZero() {
			continue
		}
		if msg.X >= z.StartX && msg.X <= z.EndX {
			ex.previewCol = i
			return
		}
	}
}

// selectClickedColumn updates the tab's column cursor to match the column
// zone the click's X coordinate falls within. Column header zones (col-N)
// share the same X ranges as body cells, so we reuse them.
func (m *Model) selectClickedColumn(tab *Tab, msg tea.MouseClickMsg) {
	vp := m.tabViewport(tab)
	for i := range vp.Specs {
		z := m.zones.Get(fmt.Sprintf("%s%d", zoneCol, i))
		if z == nil || z.IsZero() {
			continue
		}
		if msg.X >= z.StartX && msg.X <= z.EndX {
			if i < len(vp.VisToFull) {
				tab.ColCursor = vp.VisToFull[i]
				m.updateTabViewport(tab)
			}
			return
		}
	}
}

// handleScroll scrolls the active surface by delta lines.
func (m *Model) handleScroll(delta int) (tea.Model, tea.Cmd) {
	// Overlay scroll.
	if m.dashboardVisible() {
		if delta > 0 {
			m.dashDown()
		} else {
			m.dashUp()
		}
		return m, nil
	}
	if m.helpViewport != nil {
		if delta > 0 {
			m.helpViewport.ScrollDown(1)
		} else {
			m.helpViewport.ScrollUp(1)
		}
		return m, nil
	}

	// Table scroll: move the cursor like j/k.
	tab := m.effectiveTab()
	if tab == nil {
		return m, nil
	}
	cursor := tab.Table.Cursor()
	total := len(tab.CellRows)
	if total == 0 {
		return m, nil
	}
	next := cursor + delta
	if next < 0 {
		next = 0
	}
	if next >= total {
		next = total - 1
	}
	tab.Table.SetCursor(next)
	return m, nil
}

// dismissActiveOverlay closes the topmost active overlay.
func (m *Model) dismissActiveOverlay() {
	switch {
	case m.helpViewport != nil:
		m.helpViewport = nil
	case m.notePreview != nil:
		m.notePreview = nil
	case m.opsTree != nil:
		m.opsTree = nil
	case m.columnFinder != nil:
		m.columnFinder = nil
	case m.docSearch != nil:
		m.docSearch = nil
	case m.ex.extraction != nil && m.ex.extraction.Visible:
		m.ex.extraction.Visible = false
	case m.chat != nil && m.chat.Visible:
		m.chat.Visible = false
	case m.calendar != nil:
		m.calendar = nil
		m.resetFormState()
	case m.dashboardVisible():
		m.showDashboard = false
	}
}
