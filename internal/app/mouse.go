// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
)

// handleMouse dispatches mouse events to the appropriate handler.
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		return m.handleLeftClick(msg)
	case msg.Button == tea.MouseButtonWheelUp:
		return m.handleScroll(-1)
	case msg.Button == tea.MouseButtonWheelDown:
		return m.handleScroll(1)
	}
	return m, nil
}

// handleLeftClick routes a left click to the appropriate zone handler.
func (m *Model) handleLeftClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
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
		vSpecs, _, visColCursor, _, _ := visibleProjection(tab)
		_ = visColCursor
		for i := range vSpecs {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneCol, i)).InBounds(msg) {
				// Map viewport column back to full tab column.
				width := m.effectiveWidth()
				normalSep := m.styles.TableSeparator().Render(" │ ")
				vp := computeTableViewport(tab, width, normalSep, m.cur.Symbol())
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
func (m *Model) handleHintClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
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
func (m *Model) handleOverlayClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
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
	ex *extractionLogState, g *previewTableGroup, msg tea.MouseMsg,
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
func (m *Model) selectClickedColumn(tab *Tab, msg tea.MouseMsg) {
	vSpecs, _, _, _, _ := visibleProjection(tab)
	width := m.effectiveWidth()
	normalSep := m.styles.TableSeparator().Render(" │ ")
	vp := computeTableViewport(tab, width, normalSep, m.cur.Symbol())
	for i := range vSpecs {
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
	case m.columnFinder != nil:
		m.columnFinder = nil
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
