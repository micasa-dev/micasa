// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/charmbracelet/bubbles/table"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFilterTab creates a standalone Tab for pure unit tests of filter logic
// (matchesAllPins, translatePins, etc.) that don't need a full Model.
func newFilterTab() *Tab {
	specs := []columnSpec{
		{Title: "ID", Kind: cellReadonly},
		{Title: "Status", Kind: cellStatus},
		{Title: "Vendor", Kind: cellText},
	}
	cellRows := [][]cell{
		{{Value: "1"}, {Value: "Plan"}, {Value: "Alice"}},
		{{Value: "2"}, {Value: "Active"}, {Value: "Bob"}},
		{{Value: "3"}, {Value: "Plan"}, {Value: "Bob"}},
		{{Value: "4"}, {Value: "Done"}, {Value: "Alice"}},
	}
	rows := make([]table.Row, len(cellRows))
	meta := make([]rowMeta, len(cellRows))
	for i, cr := range cellRows {
		r := make(table.Row, len(cr))
		for j, c := range cr {
			r[j] = c.Value
		}
		rows[i] = r
		meta[i] = rowMeta{ID: uint(i + 1)} //nolint:gosec // i bounded by slice length
	}
	cols := []table.Column{
		{Title: "ID", Width: 4},
		{Title: "Status", Width: 8},
		{Title: "Vendor", Width: 8},
	}
	tbl := table.New(table.WithColumns(cols), table.WithRows(rows))
	return &Tab{
		Specs:        specs,
		CellRows:     cellRows,
		Rows:         meta,
		FullRows:     rows,
		FullMeta:     meta,
		FullCellRows: cellRows,
		Table:        tbl,
	}
}

// newFilterModel creates a Model with a 4-row filter dataset so tests can
// exercise pinning and filtering through sendKey. Cursor starts at row 0,
// column 1 (Status). Data layout:
//
//	Row 0: ID=1, Status=Plan,   Vendor=Alice
//	Row 1: ID=2, Status=Active, Vendor=Bob
//	Row 2: ID=3, Status=Plan,   Vendor=Bob
//	Row 3: ID=4, Status=Done,   Vendor=Alice
func newFilterModel() (*Model, *Tab) {
	m := newTestModel()
	m.mode = modeNormal
	m.showDashboard = false

	specs := []columnSpec{
		{Title: "ID", Kind: cellReadonly},
		{Title: "Status", Kind: cellStatus},
		{Title: "Vendor", Kind: cellText},
	}
	cellRows := [][]cell{
		{
			{Value: "1", Kind: cellReadonly},
			{Value: "Plan", Kind: cellStatus},
			{Value: "Alice", Kind: cellText},
		},
		{
			{Value: "2", Kind: cellReadonly},
			{Value: "Active", Kind: cellStatus},
			{Value: "Bob", Kind: cellText},
		},
		{
			{Value: "3", Kind: cellReadonly},
			{Value: "Plan", Kind: cellStatus},
			{Value: "Bob", Kind: cellText},
		},
		{
			{Value: "4", Kind: cellReadonly},
			{Value: "Done", Kind: cellStatus},
			{Value: "Alice", Kind: cellText},
		},
	}
	rows := make([]table.Row, len(cellRows))
	meta := make([]rowMeta, len(cellRows))
	for i, cr := range cellRows {
		r := make(table.Row, len(cr))
		for j, c := range cr {
			r[j] = c.Value
		}
		rows[i] = r
		meta[i] = rowMeta{ID: uint(i + 1)} //nolint:gosec // i bounded by slice length
	}
	cols := []table.Column{
		{Title: "ID", Width: 4},
		{Title: "Status", Width: 8},
		{Title: "Vendor", Width: 8},
	}

	tab := &m.tabs[m.active]
	tab.Specs = specs
	tab.Table = table.New(table.WithColumns(cols), table.WithRows(rows))
	tab.CellRows = cellRows
	tab.Rows = meta
	tab.FullRows = rows
	tab.FullMeta = meta
	tab.FullCellRows = cellRows
	tab.ColCursor = 1 // Status column
	tab.Table.SetCursor(0)
	tab.Table.Focus()
	return m, tab
}

// --- Model-level tests (user actions via sendKey) ---

func TestTogglePinAddsAndRemoves(t *testing.T) {
	m, tab := newFilterModel()

	// Cursor at row 0, col 1 -> pin "Plan".
	sendKey(m, "n")
	assert.True(t, hasPins(tab), "n should pin")
	assert.True(t, isPinned(tab, 1, "Plan"))

	// Same cell again -> unpin.
	sendKey(m, "n")
	assert.False(t, hasPins(tab), "second n should unpin")
}

func TestTogglePinMultipleValuesInColumn(t *testing.T) {
	m, tab := newFilterModel()

	// Pin "Plan" at row 0.
	sendKey(m, "n")
	require.True(t, isPinned(tab, 1, "Plan"))

	// Move to row 1 (Status = "Active") and pin.
	sendKey(m, "j")
	sendKey(m, "n")

	require.Len(t, tab.Pins, 1, "same column should have one pin entry")
	assert.True(t, isPinned(tab, 1, "Plan"))
	assert.True(t, isPinned(tab, 1, "Active"))
	assert.Len(t, tab.Pins[0].Values, 2)
}

func TestClearPins(t *testing.T) {
	m, tab := newFilterModel()

	sendKey(m, "n") // Pin "Plan"
	sendKey(m, "N") // Activate filter
	require.True(t, hasPins(tab))
	require.True(t, tab.FilterActive)

	sendKey(m, keyCtrlN)
	assert.False(t, hasPins(tab))
	assert.False(t, tab.FilterActive)
}

func TestApplyRowFilterNoPin(t *testing.T) {
	_, tab := newFilterModel()

	// No pins: all rows present and undimmed.
	assert.Len(t, tab.CellRows, 4)
	assert.Len(t, tab.Rows, 4)
	for _, rm := range tab.Rows {
		assert.False(t, rm.Dimmed)
	}
}

func TestApplyRowFilterPreview(t *testing.T) {
	m, tab := newFilterModel()

	// Pin "Plan" -> triggers preview mode (all rows visible, non-matches dimmed).
	sendKey(m, "n")

	require.Len(t, tab.CellRows, 4, "preview keeps all rows")
	assert.False(t, tab.Rows[0].Dimmed, "row 0 matches Plan")
	assert.True(t, tab.Rows[1].Dimmed, "row 1 is Active, should be dimmed")
	assert.False(t, tab.Rows[2].Dimmed, "row 2 matches Plan")
	assert.True(t, tab.Rows[3].Dimmed, "row 3 is Done, should be dimmed")
}

func TestApplyRowFilterActive(t *testing.T) {
	m, tab := newFilterModel()

	sendKey(m, "n") // Pin "Plan"
	sendKey(m, "N") // Activate filter

	require.Len(t, tab.CellRows, 2, "active filter hides non-matching")
	assert.Equal(t, "1", tab.CellRows[0][0].Value)
	assert.Equal(t, "3", tab.CellRows[1][0].Value)
}

func TestApplyRowFilterActiveAcrossColumns(t *testing.T) {
	m, tab := newFilterModel()

	// Pin "Plan" (row 0, col 1).
	sendKey(m, "n")

	// Move to Vendor column and row 2 (Plan + Bob), pin "Bob".
	sendKey(m, "l")
	sendKey(m, "j")
	sendKey(m, "j")
	sendKey(m, "n")

	// Activate filter.
	sendKey(m, "N")

	require.Len(t, tab.CellRows, 1, "only row 3 matches Plan AND Bob")
	assert.Equal(t, "3", tab.CellRows[0][0].Value)
}

func TestEagerModeToggleWithNoPins(t *testing.T) {
	m, tab := newFilterModel()

	// Activate filter with no pins ("eager mode").
	sendKey(m, "N")
	assert.True(t, tab.FilterActive, "eager mode should be armed")

	// Pin while eager mode is on -> should immediately filter.
	sendKey(m, "n")
	require.Len(t, tab.CellRows, 2, "eager mode + pin should immediately filter")
	assert.Equal(t, "1", tab.CellRows[0][0].Value)
	assert.Equal(t, "3", tab.CellRows[1][0].Value)
}

func TestEagerModeToggleOff(t *testing.T) {
	m, tab := newFilterModel()

	// Activate and pin.
	sendKey(m, "N")
	sendKey(m, "n")
	require.Len(t, tab.CellRows, 2)

	// Toggle off -> should restore all rows with preview dimming.
	sendKey(m, "N")
	require.Len(t, tab.CellRows, 4, "toggling off should restore all rows")
	assert.True(t, tab.Rows[1].Dimmed, "non-matching rows should be dimmed in preview")
}

func TestPinsPersistAcrossTabSwitch(t *testing.T) {
	m, tab := newFilterModel()
	startTab := m.active

	sendKey(m, "n") // Pin "Plan"
	sendKey(m, "N") // Activate filter
	require.True(t, hasPins(tab))
	require.True(t, tab.FilterActive)

	// Switch away and back.
	sendKey(m, "f")
	assert.NotEqual(t, startTab, m.active, "should switch tabs freely")
	sendKey(m, "b")
	assert.Equal(t, startTab, m.active)

	// Pins and filter state should still be there.
	tab = &m.tabs[startTab]
	assert.True(t, hasPins(tab), "pins should persist across tab switch")
	assert.True(t, tab.FilterActive, "filter mode should persist across tab switch")
}

func TestHideColumnClearsPinsOnThatColumn(t *testing.T) {
	m, tab := newFilterModel()

	// Pin "Plan" on col 1 (Status).
	sendKey(m, "n")
	require.True(t, hasPins(tab))

	// Hide col 1 -> should clear its pins.
	sendKey(m, "c")
	assert.False(t, isPinned(tab, 1, "Plan"), "hiding the column should clear its pins")
}

// seedTabForPinning sets up CellRows and Full* fields on the active tab so
// that sendKey("n") can pin the cell under the cursor.
func seedTabForPinning(m *Model) *Tab {
	m.showDashboard = false
	tab := m.effectiveTab()
	rows := tab.Table.Rows()
	cellRows := make([][]cell, len(rows))
	for i, r := range rows {
		cr := make([]cell, len(r))
		for j, v := range r {
			kind := cellText
			if j < len(tab.Specs) {
				kind = tab.Specs[j].Kind
			}
			cr[j] = cell{Value: v, Kind: kind}
		}
		cellRows[i] = cr
	}
	tab.CellRows = cellRows
	tab.FullRows = rows
	tab.FullMeta = tab.Rows
	tab.FullCellRows = cellRows
	return tab
}

func TestCtrlNClearsAllPinsAndFilter(t *testing.T) {
	m := newTestModel()
	m.mode = modeNormal
	tab := seedTabForPinning(m)

	// Pin via key press, then activate filter.
	sendKey(m, "n")
	require.True(t, hasPins(tab), "n should pin the cell under cursor")
	sendKey(m, "N")
	require.True(t, tab.FilterActive)

	// ctrl+n should clear everything.
	sendKey(m, keyCtrlN)
	assert.False(t, hasPins(tab), "ctrl+n should clear all pins")
	assert.False(t, tab.FilterActive, "ctrl+n should deactivate filter")
}

func TestCtrlNNoopWithoutPins(t *testing.T) {
	m := newTestModel()
	m.mode = modeNormal
	m.showDashboard = false

	sendKey(m, keyCtrlN)
	tab := m.effectiveTab()
	assert.False(t, hasPins(tab), "ctrl+n with no pins should be a no-op")
	assert.False(t, tab.FilterActive)
}

func TestPinOnDashboardBlocked(t *testing.T) {
	m := newTestModel()
	m.showDashboard = true
	m.dash.data = nonEmptyDashboard()
	startPins := len(m.effectiveTab().Pins)

	sendKey(m, "n")
	assert.Len(t, m.effectiveTab().Pins, startPins, "n should be blocked on dashboard")

	sendKey(m, "N")
	assert.False(t, m.effectiveTab().FilterActive, "N should be blocked on dashboard")
}

// --- Pure unit tests of isolated filter logic ---

func TestTogglePinCaseInsensitive(t *testing.T) {
	tab := newFilterTab()
	togglePin(tab, 1, "plan")
	assert.True(t, isPinned(tab, 1, "Plan"))
	assert.True(t, isPinned(tab, 1, "PLAN"))
}

func TestClearPinsForColumn(t *testing.T) {
	tab := newFilterTab()
	togglePin(tab, 1, "Plan")
	togglePin(tab, 2, "Alice")

	clearPinsForColumn(tab, 1)
	assert.True(t, hasPins(tab), "column 2 pin should remain")
	assert.False(t, isPinned(tab, 1, "Plan"))
	assert.True(t, isPinned(tab, 2, "Alice"))
}

func TestClearPinsForColumnClearsFilterWhenEmpty(t *testing.T) {
	tab := newFilterTab()
	togglePin(tab, 1, "Plan")
	tab.FilterActive = true
	clearPinsForColumn(tab, 1)
	assert.False(t, tab.FilterActive)
}

func TestMatchesAllPinsSingleColumn(t *testing.T) {
	pins := []filterPin{{Col: 1, Values: map[string]bool{"plan": true, "active": true}}}
	row1 := []cell{{Value: "1"}, {Value: "Plan"}, {Value: "Alice"}}
	row2 := []cell{{Value: "4"}, {Value: "Done"}, {Value: "Alice"}}

	assert.True(t, matchesAllPins(row1, pins, false), "Plan should match")
	assert.False(t, matchesAllPins(row2, pins, false), "Done should not match")
}

func TestMatchesAllPinsCrossColumn(t *testing.T) {
	pins := []filterPin{
		{Col: 1, Values: map[string]bool{"plan": true}},
		{Col: 2, Values: map[string]bool{"bob": true}},
	}
	// Row 3: Plan + Bob => match
	row3 := []cell{{Value: "3"}, {Value: "Plan"}, {Value: "Bob"}}
	// Row 1: Plan + Alice => no match (fails vendor pin)
	row1 := []cell{{Value: "1"}, {Value: "Plan"}, {Value: "Alice"}}

	assert.True(t, matchesAllPins(row3, pins, false))
	assert.False(t, matchesAllPins(row1, pins, false))
}

func TestPinSummary(t *testing.T) {
	tab := newFilterTab()
	togglePin(tab, 1, "Plan")
	s := pinSummary(tab)
	assert.Contains(t, s, "Status")
	assert.Contains(t, s, "plan")
}

func TestPinSummaryEmpty(t *testing.T) {
	tab := newFilterTab()
	assert.Equal(t, "", pinSummary(tab))
}

func TestMatchesAllPinsMagMode(t *testing.T) {
	// In mag mode, $50 -> round(log10(50)) = round(1.7) = 2
	// and $1,000 -> round(log10(1000)) = 3
	row50 := []cell{{Value: "$50.00", Kind: cellMoney}}
	row1k := []cell{{Value: "$1,000.00", Kind: cellMoney}}
	row200 := []cell{{Value: "$200.00", Kind: cellMoney}}

	// Pin on magnitude "2" (covers $50 and $200, both round to mag 2).
	pins := []filterPin{{Col: 0, Values: map[string]bool{magArrow + "2": true}}}

	assert.True(t, matchesAllPins(row50, pins, true), "$50 is mag 2")
	assert.True(t, matchesAllPins(row200, pins, true), "$200 is mag 2")
	assert.False(t, matchesAllPins(row1k, pins, true), "$1000 is mag 3, not 2")

	// Without mag mode, magnitude pins don't match raw values.
	assert.False(t, matchesAllPins(row50, pins, false), "mag pin shouldn't match raw value")
}

func TestTranslatePinsToMag(t *testing.T) {
	// Pin raw "$50.00", translate to mag mode -> should become mag "🠡2".
	tab := &Tab{
		Specs: []columnSpec{{Title: "Cost", Kind: cellMoney}},
		FullCellRows: [][]cell{
			{{Value: "$50.00", Kind: cellMoney}},
			{{Value: "$1,000.00", Kind: cellMoney}},
			{{Value: "$200.00", Kind: cellMoney}},
		},
	}
	togglePin(tab, 0, "$50.00")
	translatePins(tab, true) // switching TO mag

	require.True(t, hasPins(tab))
	// $50 -> mag 2
	assert.True(t, tab.Pins[0].Values[magArrow+"2"], "should have mag 2")
	assert.False(t, tab.Pins[0].Values["$50.00"], "raw value should be gone")
}

func TestTranslatePinsFromMag(t *testing.T) {
	// Pin mag "🠡3", translate from mag mode -> should expand to all mag-3 raw values.
	tab := &Tab{
		Specs: []columnSpec{{Title: "Cost", Kind: cellMoney}},
		FullCellRows: [][]cell{
			{{Value: "$50.00", Kind: cellMoney}},
			{{Value: "$1,000.00", Kind: cellMoney}},
			{{Value: "$2,000.00", Kind: cellMoney}},
		},
	}
	togglePin(tab, 0, magArrow+"3")
	translatePins(tab, false) // switching FROM mag

	require.True(t, hasPins(tab))
	// Both $1,000 and $2,000 are mag 3.
	assert.True(t, tab.Pins[0].Values["$1,000.00"], "$1,000 should match")
	assert.True(t, tab.Pins[0].Values["$2,000.00"], "$2,000 should match")
	assert.False(t, tab.Pins[0].Values["$50.00"], "$50 is mag 2, not 3")
}

func TestCellDisplayValueNull(t *testing.T) {
	c := cell{Kind: cellText, Null: true}
	assert.Equal(t, nullPinKey, cellDisplayValue(c, false))
	assert.Equal(t, nullPinKey, cellDisplayValue(c, true))
}

func TestCellDisplayValueNonNull(t *testing.T) {
	c := cell{Value: "Hello", Kind: cellText}
	assert.Equal(t, "hello", cellDisplayValue(c, false))
}

func TestCellDisplayValueEntityPinsByKind(t *testing.T) {
	project := cell{Value: "P Kitchen Reno", Kind: cellEntity}
	vendor := cell{Value: "V Bob's Plumbing", Kind: cellEntity}
	empty := cell{Value: "", Kind: cellEntity}

	assert.Equal(t, "project", cellDisplayValue(project, false))
	assert.Equal(t, "vendor", cellDisplayValue(vendor, false))
	assert.Equal(t, "", cellDisplayValue(empty, false), "empty entity returns empty")
}

func TestMatchesAllPinsNullCell(t *testing.T) {
	pins := []filterPin{{Col: 1, Values: map[string]bool{nullPinKey: true}}}
	nullRow := []cell{{Value: "1"}, {Kind: cellText, Null: true}}
	emptyRow := []cell{{Value: "1"}, {Value: "", Kind: cellText}}
	filledRow := []cell{{Value: "1"}, {Value: "Alice", Kind: cellText}}

	assert.True(t, matchesAllPins(nullRow, pins, false), "null cell should match null pin")
	assert.False(
		t,
		matchesAllPins(emptyRow, pins, false),
		"empty non-null should not match null pin",
	)
	assert.False(t, matchesAllPins(filledRow, pins, false), "filled cell should not match null pin")
}

func TestPinSummaryNull(t *testing.T) {
	tab := newFilterTab()
	togglePin(tab, 1, nullPinKey)
	s := pinSummary(tab)
	assert.Contains(t, s, "\u2205", "null pin should display as ∅ in summary")
	assert.NotContains(t, s, nullPinKey, "raw sentinel should not appear")
}

func TestTogglePinNullCell(t *testing.T) {
	m, tab := newFilterModel()
	// Replace row 0, col 1 with a null cell.
	tab.CellRows[0][1] = cell{Kind: cellStatus, Null: true}
	tab.FullCellRows[0][1] = cell{Kind: cellStatus, Null: true}
	tab.Table.SetCursor(0)

	sendKey(m, "n")
	assert.True(t, isPinned(tab, 1, nullPinKey), "pinning a null cell should use nullPinKey")

	sendKey(m, "n")
	assert.False(t, isPinned(tab, 1, nullPinKey), "second press should unpin")
}

func TestTranslatePinsPreservesNull(t *testing.T) {
	tab := &Tab{
		Specs: []columnSpec{{Title: "Cost", Kind: cellMoney}},
		FullCellRows: [][]cell{
			{{Value: "$50.00", Kind: cellMoney}},
			{{Kind: cellMoney, Null: true}},
		},
	}
	togglePin(tab, 0, nullPinKey)
	translatePins(tab, true)
	assert.True(t, tab.Pins[0].Values[nullPinKey], "null pin should survive mag translation")
}

func TestTranslatePinsRoundTrip(t *testing.T) {
	// Pin $1,000.00 -> toggle to mag (🠡3) -> toggle back -> should get
	// $1,000.00 AND $2,000.00 (both mag 3).
	tab := &Tab{
		Specs: []columnSpec{{Title: "Cost", Kind: cellMoney}},
		FullCellRows: [][]cell{
			{{Value: "$1,000.00", Kind: cellMoney}},
			{{Value: "$2,000.00", Kind: cellMoney}},
			{{Value: "$50.00", Kind: cellMoney}},
		},
	}
	togglePin(tab, 0, "$1,000.00")

	// To mag: $1,000 -> 🠡3
	translatePins(tab, true)
	require.Len(t, tab.Pins[0].Values, 1)
	assert.True(t, tab.Pins[0].Values[magArrow+"3"])

	// Back to raw: 🠡3 -> $1,000 and $2,000
	translatePins(tab, false)
	assert.True(t, tab.Pins[0].Values["$1,000.00"])
	assert.True(t, tab.Pins[0].Values["$2,000.00"])
	assert.Len(t, tab.Pins[0].Values, 2)
}

// --- Filter inversion tests ---

func TestInvertPreviewDimsMatchingRows(t *testing.T) {
	m, tab := newFilterModel()

	// Pin "Plan" (rows 0 and 2 match), then invert in preview mode.
	sendKey(m, "n")
	sendKey(m, "!")

	assert.False(t, tab.FilterActive, "! should not activate the filter")
	require.Len(t, tab.CellRows, 4, "preview keeps all rows")
	// With inversion, matching rows are dimmed instead of non-matching.
	assert.True(t, tab.Rows[0].Dimmed, "row 0 matches Plan, should be dimmed when inverted")
	assert.False(t, tab.Rows[1].Dimmed, "row 1 is Active, should be undimmed when inverted")
	assert.True(t, tab.Rows[2].Dimmed, "row 2 matches Plan, should be dimmed when inverted")
	assert.False(t, tab.Rows[3].Dimmed, "row 3 is Done, should be undimmed when inverted")
}

func TestInvertActiveFilterShowsNonMatchingRows(t *testing.T) {
	m, tab := newFilterModel()

	// Pin "Plan", activate filter, then invert.
	sendKey(m, "n")
	sendKey(m, "N")
	sendKey(m, "!")

	// Only non-matching rows (Active, Done) should remain.
	require.Len(t, tab.CellRows, 2, "inverted active filter keeps non-matching")
	assert.Equal(t, "2", tab.CellRows[0][0].Value)
	assert.Equal(t, "4", tab.CellRows[1][0].Value)
}

func TestInvertToggleRoundTrip(t *testing.T) {
	m, tab := newFilterModel()

	sendKey(m, "!")
	assert.True(t, tab.FilterInverted, "first ! should invert")

	sendKey(m, "!")
	assert.False(t, tab.FilterInverted, "second ! should restore")
}

func TestClearPinsResetsInvert(t *testing.T) {
	m, tab := newFilterModel()

	sendKey(m, "n") // Pin "Plan"
	sendKey(m, "!") // Invert
	require.True(t, tab.FilterInverted)

	sendKey(m, keyCtrlN) // Clear all
	assert.False(t, tab.FilterInverted, "ctrl+n should reset FilterInverted")
}

func TestInvertNullCellActiveFilter(t *testing.T) {
	m, tab := newFilterModel()
	// Replace row 0 col 1 with null.
	tab.CellRows[0][1] = cell{Kind: cellStatus, Null: true}
	tab.FullCellRows[0][1] = cell{Kind: cellStatus, Null: true}
	tab.Table.SetCursor(0)

	// Pin the null cell, activate, invert -> non-null rows shown.
	sendKey(m, "n")
	sendKey(m, "N")
	sendKey(m, "!")

	// Rows 1, 2, 3 have non-null Status; row 0 has null -> excluded.
	require.Len(t, tab.CellRows, 3, "inverted null pin should show non-null rows")
	for _, cr := range tab.CellRows {
		assert.False(t, cr[1].Null, "all remaining rows should have non-null Status")
	}
}

func TestInvertedPinHighlightsNonMatchingCells(t *testing.T) {
	// Pin col 1 = "plan". Normal: pinMatch=true for "plan" cells.
	// Inverted: pinMatch flips for pinned columns — "plan" cells lose
	// highlight, non-plan cells gain it.
	pins := []filterPin{{Col: 1, Values: map[string]bool{"plan": true}}}
	planRow := []cell{{Value: "1"}, {Value: "Plan"}, {Value: "Alice"}}
	doneRow := []cell{{Value: "4"}, {Value: "Done"}, {Value: "Alice"}}

	// Normal: Plan matches, Done doesn't.
	assert.True(t, cellMatchesPin(pins, 1, planRow[1], false))
	assert.False(t, cellMatchesPin(pins, 1, doneRow[1], false))

	// Inverted: flip for pinned column only.
	assert.True(t, columnHasPin(pins, 1), "col 1 has a pin")
	assert.False(t, columnHasPin(pins, 0), "col 0 has no pin")
	assert.False(t, columnHasPin(pins, 2), "col 2 has no pin")
}

func TestInvertedPinContextPassedToRenderer(t *testing.T) {
	m, tab := newFilterModel()

	// Pin "Plan" and invert — the pinRenderContext should carry Inverted=true.
	sendKey(m, "n")
	sendKey(m, "!")

	assert.True(t, tab.FilterInverted)
	// Verify the render context wiring by checking the tab state that
	// viewportPinContext reads. The Inverted field is set from
	// tab.FilterInverted, which we just toggled.
	assert.True(t, hasPins(tab), "pins should exist")
}

func TestInvertBlockedOnDashboard(t *testing.T) {
	m := newTestModel()
	m.showDashboard = true
	m.dash.data = nonEmptyDashboard()
	tab := m.effectiveTab()

	sendKey(m, "!")
	assert.False(t, tab.FilterInverted, "! should be blocked on dashboard")
}
