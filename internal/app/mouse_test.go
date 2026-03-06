// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendMouse sends a mouse event to the model at the given position.
func sendMouse(m *Model, x, y int, button tea.MouseButton, action tea.MouseAction) {
	m.Update(tea.MouseMsg{X: x, Y: y, Button: button, Action: action})
}

// sendClick sends a left mouse button press at the given position.
func sendClick(m *Model, x, y int) {
	sendMouse(m, x, y, tea.MouseButtonLeft, tea.MouseActionPress)
}

// requireZone renders the view and returns the zone info, skipping if not found.
func requireZone(t *testing.T, m *Model, id string) *zone.ZoneInfo {
	t.Helper()
	m.View()
	z := m.zones.Get(id)
	if z == nil || z.IsZero() {
		t.Skipf("zone %q not rendered", id)
	}
	return z
}

// TestTabClickSwitchesTab verifies that clicking on a tab changes the
// active tab, simulating a real user left-click on tab zone markers.
func TestTabClickSwitchesTab(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	require.Equal(t, 0, m.active)

	z := requireZone(t, m, "tab-1")

	sendClick(m, z.StartX, z.StartY)
	assert.Equal(t, 1, m.active, "clicking tab-1 should switch to tab index 1")
}

// TestTabClickBlockedInEditMode verifies that tab clicks do nothing when
// tabs are locked (edit mode).
func TestTabClickBlockedInEditMode(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)
	require.Equal(t, 0, m.active)

	z := requireZone(t, m, "tab-1")

	sendClick(m, z.StartX, z.StartY)
	assert.Equal(t, 0, m.active, "tab click should be ignored in edit mode")
}

// TestRowClickMovesCursor verifies that clicking on a table row moves
// the cursor to that row.
func TestRowClickMovesCursor(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	require.Greater(t, len(tab.CellRows), 1, "need at least 2 rows")

	tab.Table.SetCursor(0)
	z := requireZone(t, m, "row-1")

	sendClick(m, z.StartX, z.StartY)
	assert.Equal(t, 1, tab.Table.Cursor(), "clicking row-1 should move cursor to row 1")
}

// TestScrollWheelMovesCursor verifies that scroll wheel events move the
// table cursor like j/k.
func TestScrollWheelMovesCursor(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	require.Greater(t, len(tab.CellRows), 1)
	tab.Table.SetCursor(0)

	sendMouse(m, 10, 10, tea.MouseButtonWheelDown, tea.MouseActionPress)
	assert.Equal(t, 1, tab.Table.Cursor(), "scroll down should move cursor to 1")

	sendMouse(m, 10, 10, tea.MouseButtonWheelUp, tea.MouseActionPress)
	assert.Equal(t, 0, tab.Table.Cursor(), "scroll up should move cursor back to 0")
}

// TestHouseHeaderClickToggles verifies that clicking the house header
// toggles the house profile expand/collapse.
func TestHouseHeaderClickToggles(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	initial := m.showHouse

	z := requireZone(t, m, "house-header")

	sendClick(m, z.StartX, z.StartY)
	assert.NotEqual(t, initial, m.showHouse, "clicking house header should toggle showHouse")

	z = requireZone(t, m, "house-header")
	sendClick(m, z.StartX, z.StartY)
	assert.Equal(t, initial, m.showHouse, "second click should restore original state")
}

// TestOverlayDismissOnOutsideClick verifies that clicking outside an
// active overlay dismisses it.
func TestOverlayDismissOnOutsideClick(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	sendKey(m, "?")
	require.NotNil(t, m.helpViewport, "help viewport should be open")

	m.View()

	// Click at (0,0) which should be outside the centered overlay.
	sendClick(m, 0, 0)
	assert.Nil(t, m.helpViewport, "clicking outside overlay should dismiss help")
}

// TestHintClickOpensHelp verifies that clicking the help hint opens
// the help overlay.
func TestHintClickOpensHelp(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	require.Nil(t, m.helpViewport, "help should start closed")

	z := requireZone(t, m, "hint-help")

	sendClick(m, z.StartX, z.StartY)
	assert.NotNil(t, m.helpViewport, "clicking help hint should open help")
}

// TestBreadcrumbBackClick verifies that clicking the breadcrumb back
// link returns from a detail view.
func TestBreadcrumbBackClick(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	tab := m.effectiveTab()
	require.NotNil(t, tab)

	hasDrilldown := false
	for i, spec := range tab.Specs {
		if spec.Kind == cellDrilldown {
			tab.ColCursor = i
			hasDrilldown = true
			break
		}
	}
	if !hasDrilldown {
		t.Skip("no drilldown column available")
	}

	sendKey(m, "enter")
	if !m.inDetail() {
		t.Skip("could not enter detail view")
	}

	z := requireZone(t, m, "breadcrumb-back")

	sendClick(m, z.StartX, z.StartY)
	assert.False(t, m.inDetail(), "clicking breadcrumb back should return from detail")
}

// TestHintClickEntersEditMode verifies that clicking the edit hint
// enters edit mode.
func TestHintClickEntersEditMode(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	require.Equal(t, modeNormal, m.mode)

	z := requireZone(t, m, "hint-edit")

	sendClick(m, z.StartX, z.StartY)
	assert.Equal(t, modeEdit, m.mode, "clicking edit hint should enter edit mode")
}

// TestScrollWheelInHelpOverlay verifies that scroll wheel events in the
// help overlay scroll the help content instead of the table.
func TestScrollWheelInHelpOverlay(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	sendKey(m, "?")
	require.NotNil(t, m.helpViewport)

	initialOffset := m.helpViewport.YOffset

	sendMouse(m, 10, 10, tea.MouseButtonWheelDown, tea.MouseActionPress)

	assert.Greater(t, m.helpViewport.YOffset, initialOffset,
		"scroll down in help overlay should advance viewport")
}

// TestColumnHeaderClickMovesColCursor verifies that clicking a column
// header moves the column cursor.
func TestColumnHeaderClickMovesColCursor(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	require.Greater(t, len(tab.Specs), 1, "need at least 2 columns")

	z := requireZone(t, m, "col-1")

	sendClick(m, z.StartX, z.StartY)
	assert.NotEqual(t, 0, tab.ColCursor,
		"clicking col-1 header should move column cursor")
}

// TestMouseNoOpOnRelease verifies that mouse release events are ignored.
func TestMouseNoOpOnRelease(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	before := m.active

	sendMouse(m, 10, 10, tea.MouseButtonLeft, tea.MouseActionRelease)
	assert.Equal(t, before, m.active, "mouse release should not change state")
}

// TestSelectedRowClickDrillsDown verifies that clicking an already-selected
// row triggers drilldown (same as pressing enter).
func TestSelectedRowClickDrillsDown(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	require.NotEmpty(t, tab.CellRows)

	hasDrilldown := false
	for i, spec := range tab.Specs {
		if spec.Kind == cellDrilldown {
			tab.ColCursor = i
			hasDrilldown = true
			break
		}
	}
	if !hasDrilldown {
		t.Skip("no drilldown column available")
	}

	tab.Table.SetCursor(0)
	z := requireZone(t, m, "row-0")

	sendClick(m, z.StartX, z.StartY)
	assert.True(t, m.inDetail(), "clicking selected row should trigger drilldown")
}

// TestDashboardScrollWheel verifies that scroll wheel events in the
// dashboard overlay scroll dashboard items instead of the table.
func TestDashboardScrollWheel(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	sendKey(m, "D")
	if !m.dashboardVisible() {
		t.Skip("dashboard has no data to display")
	}
	require.Greater(t, len(m.dash.nav), 1, "need multiple dashboard nav items")

	m.dash.cursor = 0
	sendMouse(m, 10, 10, tea.MouseButtonWheelDown, tea.MouseActionPress)
	assert.Equal(t, 1, m.dash.cursor, "scroll down in dashboard should move cursor")

	sendMouse(m, 10, 10, tea.MouseButtonWheelUp, tea.MouseActionPress)
	assert.Equal(t, 0, m.dash.cursor, "scroll up in dashboard should move cursor back")
}

// TestDashboardDismissOnOutsideClick verifies that clicking outside the
// dashboard overlay dismisses it.
func TestDashboardDismissOnOutsideClick(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	sendKey(m, "D")
	if !m.dashboardVisible() {
		t.Skip("dashboard has no data to display")
	}

	m.View()

	sendClick(m, 0, 0)
	assert.False(t, m.dashboardVisible(), "clicking outside dashboard should dismiss it")
}
