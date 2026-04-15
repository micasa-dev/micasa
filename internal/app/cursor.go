// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
)

// OSC 22 escape sequences for mouse pointer shape control.
//
// Format: ESC ] 22 ; <shape> ST
// Where ST (String Terminator) is ESC \.
//
// Terminals that do not support OSC 22 silently ignore these sequences.
// When running inside tmux, sequences are wrapped in DCS passthrough
// so they reach the outer terminal.
// See plans/631-mouse-cursor-hints.md for compatibility matrix.
const (
	pointerShapeDefault = ""
	pointerShapePointer = "pointer"

	osc22Prefix = "\x1b]22;"
	osc22Suffix = "\x1b\\"
)

// buildOSC22 builds an OSC 22 escape sequence for the given shape.
// When tmux is true, the sequence is wrapped in DCS passthrough
// (ESC P tmux; <escaped> ST) so tmux forwards it to the outer terminal.
func buildOSC22(shape string, tmux bool) string {
	if !tmux {
		return osc22Prefix + shape + osc22Suffix
	}
	// DCS passthrough: double each ESC in the inner sequence.
	// Inner: \x1b]22;<shape>\x1b\\
	// Wrapped: \x1bPtmux; \x1b\x1b]22;<shape>\x1b\x1b\\ \x1b\\
	return "\x1bPtmux;\x1b\x1b]22;" + shape + "\x1b\x1b\\\x1b\\"
}

// setPointerShape writes an OSC 22 escape sequence to change the mouse
// pointer shape. It only writes when the shape differs from the last
// written shape, avoiding redundant writes on every motion event.
//
// Returns the new shape value to store as lastPointerShape.
func setPointerShape(w io.Writer, shape, last string, tmux bool) string {
	if shape == last {
		return last
	}
	if w == nil {
		return shape
	}
	// Ignore write errors -- the terminal may not support OSC 22,
	// and there is no recovery action. The sequence is purely cosmetic.
	_, _ = io.WriteString(w, buildOSC22(shape, tmux))
	return shape
}

// resetPointerShape unconditionally resets the pointer to the terminal
// default. Used during shutdown where we always want to ensure cleanup
// regardless of tracked state.
func resetPointerShape(w io.Writer, tmux bool) {
	if w == nil {
		return
	}
	_, _ = io.WriteString(w, buildOSC22(pointerShapeDefault, tmux))
}

// isOverClickableZone returns true if the mouse position is within any
// clickable zone. It mirrors the zone checks in handleLeftClick and
// handleOverlayClick but only tests containment, executing no actions.
func (m *Model) isOverClickableZone(msg tea.MouseMotionMsg) bool {
	if m.hasActiveOverlay() {
		return m.isOverOverlayZone(msg)
	}
	return m.isOverBaseZone(msg)
}

// isOverBaseZone checks non-overlay clickable zones: tabs, rows, columns,
// hints, house header, and breadcrumb back.
func (m *Model) isOverBaseZone(msg tea.MouseMotionMsg) bool {
	// Tab bar.
	if !m.tabsLocked() && !m.inDetail() {
		for i := range m.tabs {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneTab, i)).InBounds(msg) {
				return true
			}
		}
	}

	// Breadcrumb back.
	if m.inDetail() {
		if m.zones.Get(zoneBreadcrumb).InBounds(msg) {
			return true
		}
	}

	// House header.
	if m.zones.Get(zoneHouse).InBounds(msg) {
		return true
	}

	// Column headers and table rows.
	if tab := m.effectiveTab(); tab != nil {
		vp := m.tabViewport(tab)
		for i := range vp.Specs {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneCol, i)).InBounds(msg) {
				return true
			}
		}

		total := len(tab.CellRows)
		if total > 0 {
			cursor := tab.Table.Cursor()
			height := tab.Table.Height()
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
					return true
				}
			}
		}
	}

	// Status bar hints.
	hintIDs := []string{"edit", "help", "add", "exit", "enter", "del", "open", "search", "ask"}
	for _, id := range hintIDs {
		if m.zones.Get(zoneHint + id).InBounds(msg) {
			return true
		}
	}

	return false
}

// isOverOverlayZone checks clickable zones within active overlays:
// dashboard rows, house fields, search results, ops tree nodes/tabs,
// and extraction preview elements.
func (m *Model) isOverOverlayZone(msg tea.MouseMotionMsg) bool {
	// House overlay fields.
	if m.houseOverlay != nil {
		for _, d := range houseFieldDefs() {
			if d.section == houseSectionIdentity {
				continue
			}
			if m.zones.Get(zoneHouseField + d.key).InBounds(msg) {
				return true
			}
		}
	}

	// Dashboard rows.
	if m.dashboardVisible() {
		for i := range m.dash.nav {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneDashRow, i)).InBounds(msg) {
				return true
			}
		}
	}

	// Search results.
	if ds := m.docSearch; ds != nil {
		for i := range ds.Results {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneSearchRow, i)).InBounds(msg) {
				return true
			}
		}
	}

	// Ops tree nodes.
	if tree := m.opsTree; tree != nil {
		nodes := tree.visibleNodes()
		for i := range nodes {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneOpsNode, i)).InBounds(msg) {
				return true
			}
		}
		for i := range tree.previewGroups {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneOpsTab, i)).InBounds(msg) {
				return true
			}
		}
	}

	// Extraction preview.
	if ex := m.ex.extraction; ex != nil && ex.Visible && ex.exploring {
		for i := range ex.previewGroups {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneExtTab, i)).InBounds(msg) {
				return true
			}
		}
		if g := ex.activePreviewGroup(); g != nil {
			for i := range g.cells {
				if m.zones.Get(fmt.Sprintf("%s%d", zoneExtRow, i)).InBounds(msg) {
					return true
				}
			}
			for i := range g.specs {
				if m.zones.Get(fmt.Sprintf("%s%d", zoneExtCol, i)).InBounds(msg) {
					return true
				}
			}
		}
	}

	return false
}
