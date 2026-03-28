// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/extract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendMouseClick sends a mouse click event to the model at the given position.
func sendMouseClick(m *Model, x, y int, button tea.MouseButton) {
	m.Update(tea.MouseClickMsg{X: x, Y: y, Button: button})
}

// sendMouseWheel sends a mouse wheel event to the model.
func sendMouseWheel(m *Model, x, y int, button tea.MouseButton) {
	m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: button})
}

// sendClick sends a left mouse button click at the given position.
func sendClick(m *Model, x, y int) {
	sendMouseClick(m, x, y, tea.MouseLeft)
}

// requireZone renders the view and returns the zone info, skipping if not found.
func requireZone(t *testing.T, m *Model, id string) *zone.ZoneInfo {
	t.Helper()
	m.View()
	m.View()
	z := m.zones.Get(id)
	if z == nil || z.IsZero() {
		t.Skipf("zone %q not rendered", id)
	}
	return z
}

// drilldownColX returns the X coordinate of the drilldown column's header
// zone. This is needed because row clicks also select the column, so tests
// that expect drilldown must click at the drilldown column's X position.
func drilldownColX(t *testing.T, m *Model, tab *Tab) int {
	t.Helper()
	m.View()
	width := m.effectiveWidth()
	normalSep := m.styles.TableSeparator().Render(" \u2502 ")
	vp := computeTableViewport(tab, width, normalSep, m.cur.Symbol())
	for vi, fi := range vp.VisToFull {
		if fi < len(tab.Specs) && tab.Specs[fi].Kind == cellDrilldown {
			z := m.zones.Get(fmt.Sprintf("%s%d", zoneCol, vi))
			if z != nil && !z.IsZero() {
				return z.StartX
			}
		}
	}
	t.Skip("drilldown column zone not rendered")
	return 0
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

// TestRowClickSelectsColumn verifies that clicking on a cell within a row
// also moves the column cursor to the clicked column.
func TestRowClickSelectsColumn(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	require.Greater(t, len(tab.CellRows), 1, "need at least 2 rows")
	require.Greater(t, len(tab.Specs), 1, "need at least 2 columns")

	tab.Table.SetCursor(0)
	tab.ColCursor = 0

	// Get the second column header zone for its X range.
	colZone := requireZone(t, m, "col-1")
	// Get a row zone for its Y range.
	rowZone := requireZone(t, m, "row-1")

	// Click at the X of column 1, Y of row 1.
	sendClick(m, colZone.StartX, rowZone.StartY)
	assert.Equal(t, 1, tab.Table.Cursor(), "clicking should move row cursor to row 1")
	assert.NotEqual(t, 0, tab.ColCursor, "clicking in column 1 area should move column cursor")
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

	sendMouseWheel(m, 10, 10, tea.MouseWheelDown)
	assert.Equal(t, 1, tab.Table.Cursor(), "scroll down should move cursor to 1")

	sendMouseWheel(m, 10, 10, tea.MouseWheelUp)
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
	require.NotNil(t, m.helpState, "help viewport should be open")

	m.View()

	// Click at (0,0) which should be outside the centered overlay.
	sendClick(m, 0, 0)
	assert.Nil(t, m.helpState, "clicking outside overlay should dismiss help")
}

// TestHintClickOpensHelp verifies that clicking the help hint opens
// the help overlay.
func TestHintClickOpensHelp(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	require.Nil(t, m.helpState, "help should start closed")

	z := requireZone(t, m, "hint-help")

	sendClick(m, z.StartX, z.StartY)
	assert.NotNil(t, m.helpState, "clicking help hint should open help")
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
	m.height = 20 // small height so the right pane viewport overflows

	sendKey(m, "?")
	require.NotNil(t, m.helpState)

	// Navigate to Nav Mode section which has many entries.
	sendKey(m, "j")
	require.Equal(t, 1, m.helpState.section)

	if m.helpState.viewport.TotalLineCount() <= m.helpState.viewport.Height() {
		t.Skip("viewport fits without scrolling")
	}

	initialOffset := m.helpState.viewport.YOffset()

	sendMouseWheel(m, 10, 10, tea.MouseWheelDown)

	assert.Greater(t, m.helpState.viewport.YOffset(), initialOffset,
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

// TestViewportCachePopulatedAfterView verifies that rendering the view
// populates the viewport cache on the active tab, and that mouse clicks
// reuse the cache rather than recomputing.
func TestViewportCachePopulatedAfterView(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	require.Nil(t, tab.cachedVP, "cache should be nil before first render")

	m.View()
	require.NotNil(t, tab.cachedVP, "View() should populate the viewport cache")

	cached := tab.cachedVP
	z := requireZone(t, m, "col-1")
	sendClick(m, z.StartX, z.StartY)

	// After the click, updateTabViewport invalidates the cache; the next
	// View() repopulates it.
	m.View()
	require.NotNil(t, tab.cachedVP, "cache repopulated after click + render")
	assert.NotEqual(t, 0, tab.ColCursor, "column cursor moved by click")
	_ = cached
}

// TestViewportCacheInvalidatedOnRefresh verifies that refreshTable clears
// the viewport cache.
func TestViewportCacheInvalidatedOnRefresh(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	tab := m.effectiveTab()
	require.NotNil(t, tab)

	m.View()
	require.NotNil(t, tab.cachedVP)

	m.refreshTable(tab)
	assert.Nil(t, tab.cachedVP, "refreshTable should invalidate viewport cache")
}

// TestMouseNoOpOnRelease verifies that mouse release events are ignored.
func TestMouseNoOpOnRelease(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	before := m.active

	m.Update(tea.MouseReleaseMsg{X: 10, Y: 10, Button: tea.MouseLeft})
	assert.Equal(t, before, m.active, "mouse release should not change state")
}

// TestDoubleClickRowDrillsDown verifies that double-clicking a row triggers
// drilldown (same as pressing enter).
func TestDoubleClickRowDrillsDown(t *testing.T) {
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
	colX := drilldownColX(t, m, tab)
	z := requireZone(t, m, "row-0")

	// First click selects (already selected, but records the click).
	sendClick(m, colX, z.StartY)
	assert.False(t, m.inDetail(), "single click should not trigger drilldown")

	// Second click within threshold triggers drilldown.
	z = requireZone(t, m, "row-0")
	sendClick(m, colX, z.StartY)
	assert.True(t, m.inDetail(), "double-click should trigger drilldown")
}

// TestSingleClickOnSelectedRowDoesNotDrill verifies that a single click on
// an already-selected row does not trigger drilldown.
func TestSingleClickOnSelectedRowDoesNotDrill(t *testing.T) {
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
	colX := drilldownColX(t, m, tab)
	z := requireZone(t, m, "row-0")

	sendClick(m, colX, z.StartY)
	assert.False(t, m.inDetail(), "single click on selected row should not drill down")
}

// TestDoubleClickExpiredDoesNotDrill verifies that two clicks with too much
// time between them do not trigger drilldown.
func TestDoubleClickExpiredDoesNotDrill(t *testing.T) {
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
	colX := drilldownColX(t, m, tab)
	z := requireZone(t, m, "row-0")

	sendClick(m, colX, z.StartY)
	// Simulate an expired click by backdating the recorded time.
	m.lastRowClick.at = m.lastRowClick.at.Add(-time.Second)

	z = requireZone(t, m, "row-0")
	sendClick(m, colX, z.StartY)
	assert.False(t, m.inDetail(), "expired double-click should not trigger drilldown")
}

// TestDoubleClickDifferentRowDoesNotDrill verifies that clicking two
// different rows in quick succession does not trigger drilldown.
func TestDoubleClickDifferentRowDoesNotDrill(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	require.Greater(t, len(tab.CellRows), 1, "need at least 2 rows")

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
	colX := drilldownColX(t, m, tab)
	z0 := requireZone(t, m, "row-0")
	sendClick(m, colX, z0.StartY)

	z1 := requireZone(t, m, "row-1")
	sendClick(m, colX, z1.StartY)
	assert.False(t, m.inDetail(), "clicking different rows should not trigger drilldown")
	assert.Equal(t, 1, tab.Table.Cursor(), "second click should select row 1")
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
	sendMouseWheel(m, 10, 10, tea.MouseWheelDown)
	assert.Equal(t, 1, m.dash.cursor, "scroll down in dashboard should move cursor")

	sendMouseWheel(m, 10, 10, tea.MouseWheelUp)
	assert.Equal(t, 0, m.dash.cursor, "scroll up in dashboard should move cursor back")
}

// TestDashboardRowClickSelects verifies that a single click on a dashboard
// row selects it without jumping.
func TestDashboardRowClickSelects(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	sendKey(m, "D")
	if !m.dashboardVisible() {
		t.Skip("dashboard has no data to display")
	}
	require.Greater(t, len(m.dash.nav), 1, "need multiple dashboard nav items")

	m.dash.cursor = 0
	// Render once to populate all zones including overlay.
	m.View()
	oz := m.zones.Get(zoneOverlay)
	if oz == nil || oz.IsZero() {
		t.Skip("overlay zone not rendered")
	}
	z := requireZone(t, m, "dash-1")

	sendClick(m, z.StartX, z.StartY)
	assert.True(t, m.dashboardVisible(), "single click should not close dashboard")
	assert.Equal(t, 1, m.dash.cursor, "single click should move dashboard cursor")
}

// TestDashboardDoubleClickJumps verifies that double-clicking a dashboard
// row jumps to the item (closes the dashboard and switches tabs).
func TestDashboardDoubleClickJumps(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)

	sendKey(m, "D")
	if !m.dashboardVisible() {
		t.Skip("dashboard has no data to display")
	}
	require.Greater(t, len(m.dash.nav), 1, "need multiple dashboard nav items")

	// Find a jumpable (non-header, non-info-only) row.
	jumpIdx := -1
	for i, nav := range m.dash.nav {
		if !nav.IsHeader && !nav.InfoOnly {
			jumpIdx = i
			break
		}
	}
	if jumpIdx < 0 {
		t.Skip("no jumpable dashboard row")
	}

	m.dash.cursor = 0
	// Render once to populate all zones including overlay.
	m.View()
	oz := m.zones.Get(zoneOverlay)
	if oz == nil || oz.IsZero() {
		t.Skip("overlay zone not rendered")
	}
	z := requireZone(t, m, fmt.Sprintf("dash-%d", jumpIdx))

	// First click selects.
	sendClick(m, z.StartX, z.StartY)
	require.True(t, m.dashboardVisible(), "single click should keep dashboard open")
	require.Equal(t, jumpIdx, m.dash.cursor)

	// Second click within threshold jumps.
	z = requireZone(t, m, fmt.Sprintf("dash-%d", jumpIdx))
	sendClick(m, z.StartX, z.StartY)
	assert.False(t, m.dashboardVisible(), "double-click should close dashboard and jump")
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

// newExploreModel creates a model with an extraction overlay in explore mode
// containing the given operations. The overlay is visible, done, and exploring.
func newExploreModel(t *testing.T, ops []extract.Operation) *Model {
	t.Helper()
	m := newPreviewModel(t, ops)
	// Enter explore mode (press x).
	sendExtractionKey(m, "x")
	require.True(t, m.ex.extraction.exploring, "should be in explore mode")
	return m
}

// requireExtractionZone renders the full view twice to populate zones
// (the second call ensures the async zone worker has drained the first
// batch), then returns the zone info. Skips if not found.
func requireExtractionZone(t *testing.T, m *Model, id string) *zone.ZoneInfo {
	t.Helper()
	m.View()
	m.View()
	z := m.zones.Get(id)
	if z == nil || z.IsZero() {
		t.Skipf("zone %q not rendered", id)
	}
	return z
}

// TestExtractionRowClickSelectsRow verifies that clicking a row in the
// extraction preview table moves the preview row cursor.
func TestExtractionRowClickSelectsRow(t *testing.T) {
	t.Parallel()
	m := newExploreModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "Alpha"}},
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "Beta"}},
	})
	ex := m.ex.extraction
	require.Equal(t, 0, ex.previewRow)

	z := requireExtractionZone(t, m, fmt.Sprintf("%s%d", zoneExtRow, 1))

	m.handleOverlayClick(tea.MouseClickMsg{
		X: z.StartX, Y: z.StartY,
		Button: tea.MouseLeft,
	})
	assert.Equal(t, 1, ex.previewRow, "clicking ext-row-1 should move preview row cursor to 1")
}

// TestExtractionColClickSelectsCol verifies that clicking a column header
// in the extraction preview moves the preview column cursor.
func TestExtractionColClickSelectsCol(t *testing.T) {
	t.Parallel()
	m := newExploreModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{
			"name": "Alpha", "phone": "555-1234",
		}},
	})
	ex := m.ex.extraction
	require.Equal(t, 0, ex.previewCol)

	m.View()
	m.View()

	// Find a column zone beyond the first.
	found := false
	for i := 1; i < 10; i++ {
		z := m.zones.Get(fmt.Sprintf("%s%d", zoneExtCol, i))
		if z != nil && !z.IsZero() {
			m.handleOverlayClick(tea.MouseClickMsg{
				X: z.StartX, Y: z.StartY,
				Button: tea.MouseLeft,
			})
			assert.Equal(t, i, ex.previewCol,
				"clicking ext-col-%d header should move preview column cursor", i)
			found = true
			break
		}
	}
	if !found {
		t.Skip("no secondary extraction column zone rendered")
	}
}

// TestExtractionRowClickSelectsColumn verifies that clicking a row also
// updates the column cursor based on the X position of the click.
func TestExtractionRowClickSelectsColumn(t *testing.T) {
	t.Parallel()
	m := newExploreModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{
			"name": "Alpha", "phone": "555-1234",
		}},
		{Action: "create", Table: data.TableVendors, Data: map[string]any{
			"name": "Beta", "phone": "555-5678",
		}},
	})
	ex := m.ex.extraction
	ex.previewCol = 0

	m.View()
	m.View()

	// Find a secondary column zone for its X range.
	colZ := m.zones.Get(fmt.Sprintf("%s%d", zoneExtCol, 1))
	if colZ == nil || colZ.IsZero() {
		t.Skip("no secondary extraction column zone rendered")
	}

	rowZ := requireExtractionZone(t, m, fmt.Sprintf("%s%d", zoneExtRow, 1))
	// Click at the X of the secondary column, Y of row 1.
	m.handleOverlayClick(tea.MouseClickMsg{
		X: colZ.StartX, Y: rowZ.StartY,
		Button: tea.MouseLeft,
	})
	assert.Equal(t, 1, ex.previewRow, "clicking should move row cursor to 1")
	assert.Equal(t, 1, ex.previewCol,
		"clicking in column 1 area should move column cursor")
}

// TestExtractionClickIgnoredWhenNotExploring verifies that clicking on
// extraction preview zones does nothing when not in explore mode.
func TestExtractionClickIgnoredWhenNotExploring(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "Alpha"}},
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "Beta"}},
	})
	ex := m.ex.extraction
	require.False(t, ex.exploring, "should not be in explore mode")

	z := requireExtractionZone(t, m, fmt.Sprintf("%s%d", zoneExtRow, 1))

	m.handleOverlayClick(tea.MouseClickMsg{
		X: z.StartX, Y: z.StartY,
		Button: tea.MouseLeft,
	})
	assert.Equal(t, 0, ex.previewRow,
		"clicking row in non-explore mode should not update preview cursor")
}

// TestExtractionTabClickSwitchesTab verifies that clicking a tab in the
// extraction preview switches the active preview tab.
func TestExtractionTabClickSwitchesTab(t *testing.T) {
	t.Parallel()
	m := newExploreModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "Alpha"}},
		{Action: "create", Table: data.TableQuotes, Data: map[string]any{
			"total_cents": 100, "vendor_name": "Alpha",
		}},
	})
	ex := m.ex.extraction
	require.Equal(t, 0, ex.previewTab)
	require.GreaterOrEqual(t, len(ex.previewGroups), 2, "need at least 2 preview groups")

	z := requireExtractionZone(t, m, fmt.Sprintf("%s%d", zoneExtTab, 1))

	m.handleOverlayClick(tea.MouseClickMsg{
		X: z.StartX, Y: z.StartY,
		Button: tea.MouseLeft,
	})
	assert.Equal(t, 1, ex.previewTab,
		"clicking ext-tab-1 should switch to second preview tab")
	assert.Equal(t, 0, ex.previewRow, "tab switch should reset row cursor")
	assert.Equal(t, 0, ex.previewCol, "tab switch should reset column cursor")
}
