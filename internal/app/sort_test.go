// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSortTab() *Tab {
	return &Tab{
		Specs: []columnSpec{
			{Title: "ID", Kind: cellReadonly},
			{Title: "Name", Kind: cellText},
			{Title: "Cost", Kind: cellMoney},
			{Title: "Date", Kind: cellDate},
		},
		CellRows: [][]cell{
			{
				{Value: "3", Kind: cellReadonly},
				{Value: "Charlie", Kind: cellText},
				{Value: "$200.00", Kind: cellMoney},
				{Value: "2025-03-01", Kind: cellDate},
			},
			{
				{Value: "1", Kind: cellReadonly},
				{Value: "Alice", Kind: cellText},
				{Value: "$50.00", Kind: cellMoney},
				{Value: "2025-01-15", Kind: cellDate},
			},
			{
				{Value: "2", Kind: cellReadonly},
				{Value: "Bob", Kind: cellText},
				{Value: "$1,000.00", Kind: cellMoney},
				{Value: "2025-02-10", Kind: cellDate},
			},
		},
		Rows: []rowMeta{
			{ID: 3},
			{ID: 1},
			{ID: 2},
		},
	}
}

func TestToggleSortCycle(t *testing.T) {
	t.Parallel()
	tab := &Tab{}

	// none -> asc
	toggleSort(tab, 1)
	require.Len(t, tab.Sorts, 1)
	assert.Equal(t, sortAsc, tab.Sorts[0].Dir)
	assert.Equal(t, 1, tab.Sorts[0].Col)

	// asc -> desc
	toggleSort(tab, 1)
	require.Len(t, tab.Sorts, 1)
	assert.Equal(t, sortDesc, tab.Sorts[0].Dir)

	// desc -> none (removed)
	toggleSort(tab, 1)
	assert.Empty(t, tab.Sorts)
}

func TestToggleSortMultiColumn(t *testing.T) {
	t.Parallel()
	tab := &Tab{}
	toggleSort(tab, 0) // col 0 asc
	toggleSort(tab, 2) // col 2 asc

	require.Len(t, tab.Sorts, 2)
	assert.Equal(t, 0, tab.Sorts[0].Col)
	assert.Equal(t, 2, tab.Sorts[1].Col)

	// Toggle col 0 to desc; col 2 stays asc.
	toggleSort(tab, 0)
	assert.Equal(t, sortDesc, tab.Sorts[0].Dir)
	assert.Equal(t, sortAsc, tab.Sorts[1].Dir)
}

func TestClearSorts(t *testing.T) {
	t.Parallel()
	tab := &Tab{}
	toggleSort(tab, 0)
	toggleSort(tab, 1)
	clearSorts(tab)
	assert.Empty(t, tab.Sorts)
}

func TestApplySortsDefaultPK(t *testing.T) {
	t.Parallel()
	tab := newSortTab()
	// No explicit sorts => default PK (col 0) asc.
	applySorts(tab)

	ids := collectIDs(tab)
	assert.Equal(t, []uint{1, 2, 3}, ids)
}

func TestApplySortsByNameAsc(t *testing.T) {
	t.Parallel()
	tab := newSortTab()
	toggleSort(tab, 1) // Name asc
	applySorts(tab)

	names := collectCol(tab, 1)
	assert.Equal(t, []string{"Alice", "Bob", "Charlie"}, names)
}

func TestApplySortsByNameDesc(t *testing.T) {
	t.Parallel()
	tab := newSortTab()
	toggleSort(tab, 1) // Name asc
	toggleSort(tab, 1) // Name desc
	applySorts(tab)

	names := collectCol(tab, 1)
	assert.Equal(t, []string{"Charlie", "Bob", "Alice"}, names)
}

func TestApplySortsByMoneyAsc(t *testing.T) {
	t.Parallel()
	tab := newSortTab()
	toggleSort(tab, 2) // Cost asc
	applySorts(tab)

	costs := collectCol(tab, 2)
	assert.Equal(t, []string{"$50.00", "$200.00", "$1,000.00"}, costs)
}

func TestApplySortsByDateDesc(t *testing.T) {
	t.Parallel()
	tab := newSortTab()
	toggleSort(tab, 3) // Date asc
	toggleSort(tab, 3) // Date desc
	applySorts(tab)

	dates := collectCol(tab, 3)
	assert.Equal(t, []string{"2025-03-01", "2025-02-10", "2025-01-15"}, dates)
}

func TestApplySortsNullLastRegardlessOfDirection(t *testing.T) {
	t.Parallel()
	tab := &Tab{
		Specs: []columnSpec{
			{Title: "Name", Kind: cellText},
		},
		CellRows: [][]cell{
			{{Kind: cellText, Null: true}},
			{{Value: "Bravo", Kind: cellText}},
			{{Value: "Alpha", Kind: cellText}},
		},
		Rows: []rowMeta{{ID: 1}, {ID: 2}, {ID: 3}},
	}
	toggleSort(tab, 0) // asc
	applySorts(tab)

	assert.Equal(t, []string{"Alpha", "Bravo", ""}, collectCol(tab, 0),
		"null should sort last in asc")
	assert.True(t, tab.CellRows[2][0].Null, "last row should be null")

	// Now desc: null should still be last.
	toggleSort(tab, 0) // desc
	applySorts(tab)

	assert.Equal(t, []string{"Bravo", "Alpha", ""}, collectCol(tab, 0),
		"null should sort last in desc")
	assert.True(t, tab.CellRows[2][0].Null, "last row should be null")
}

func TestApplySortsEmptySortsNormally(t *testing.T) {
	t.Parallel()
	tab := &Tab{
		Specs: []columnSpec{
			{Title: "Name", Kind: cellText},
		},
		CellRows: [][]cell{
			{{Value: "Bravo", Kind: cellText}},
			{{Value: "", Kind: cellText}},
			{{Value: "Alpha", Kind: cellText}},
		},
		Rows: []rowMeta{{ID: 1}, {ID: 2}, {ID: 3}},
	}
	toggleSort(tab, 0) // asc
	applySorts(tab)

	assert.Equal(t, []string{"", "Alpha", "Bravo"}, collectCol(tab, 0),
		"empty non-null should sort first (before alphabetic values)")
}

func TestApplySortsNullAfterEmpty(t *testing.T) {
	t.Parallel()
	tab := &Tab{
		Specs: []columnSpec{
			{Title: "Name", Kind: cellText},
		},
		CellRows: [][]cell{
			{{Kind: cellText, Null: true}},
			{{Value: "", Kind: cellText}},
			{{Value: "Alpha", Kind: cellText}},
		},
		Rows: []rowMeta{{ID: 1}, {ID: 2}, {ID: 3}},
	}
	toggleSort(tab, 0) // asc
	applySorts(tab)

	assert.Equal(t, []string{"", "Alpha", ""}, collectCol(tab, 0))
	assert.False(t, tab.CellRows[0][0].Null, "first row should be empty, not null")
	assert.True(t, tab.CellRows[2][0].Null, "last row should be null")
}

func TestApplySortsMultiKey(t *testing.T) {
	t.Parallel()
	tab := &Tab{
		Specs: []columnSpec{
			{Title: "Group", Kind: cellText},
			{Title: "Name", Kind: cellText},
		},
		CellRows: [][]cell{
			{{Value: "B", Kind: cellText}, {Value: "Zara", Kind: cellText}},
			{{Value: "A", Kind: cellText}, {Value: "Yuri", Kind: cellText}},
			{{Value: "A", Kind: cellText}, {Value: "Alex", Kind: cellText}},
			{{Value: "B", Kind: cellText}, {Value: "Mia", Kind: cellText}},
		},
		Rows: []rowMeta{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}},
	}
	toggleSort(tab, 0) // Group asc (primary)
	toggleSort(tab, 1) // Name asc (secondary)
	applySorts(tab)

	names := collectCol(tab, 1)
	assert.Equal(t, []string{"Alex", "Yuri", "Mia", "Zara"}, names)
}

func TestSortIndicatorSingle(t *testing.T) {
	t.Parallel()
	sorts := []sortEntry{{Col: 2, Dir: sortAsc}}
	assert.Equal(t, " \u25b2", sortIndicator(sorts, 2))
}

func TestSortIndicatorMulti(t *testing.T) {
	t.Parallel()
	sorts := []sortEntry{
		{Col: 2, Dir: sortAsc},
		{Col: 5, Dir: sortDesc},
	}
	assert.Equal(t, " \u25b21", sortIndicator(sorts, 2))
	assert.Equal(t, " \u25bc2", sortIndicator(sorts, 5))
	assert.Empty(t, sortIndicator(sorts, 0))
}

func TestPKTiebreaker(t *testing.T) {
	t.Parallel()
	// Col 0 not in stack: gets appended.
	sorts := []sortEntry{{Col: 2, Dir: sortAsc}}
	result := withPKTiebreaker(sorts)
	require.Len(t, result, 2)
	assert.Equal(t, 0, result[1].Col)
	assert.Equal(t, sortAsc, result[1].Dir)

	// Col 0 already in stack: unchanged.
	sorts = []sortEntry{{Col: 0, Dir: sortDesc}, {Col: 3, Dir: sortAsc}}
	result = withPKTiebreaker(sorts)
	assert.Len(t, result, 2)
}

func TestSortKeyOnlyInNormalMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = false
	tab := m.effectiveTab()
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)

	// Press 's' in edit mode -- should not add a sort entry.
	sendKey(m, "s")
	assert.Empty(t, tab.Sorts, "s should not trigger sort in edit mode")
}

// helpers

func collectIDs(tab *Tab) []uint {
	ids := make([]uint, len(tab.Rows))
	for i, m := range tab.Rows {
		ids[i] = m.ID
	}
	return ids
}

func collectCol(tab *Tab, col int) []string {
	vals := make([]string, len(tab.CellRows))
	for i, row := range tab.CellRows {
		if col < len(row) {
			vals[i] = row[col].Value
		}
	}
	return vals
}
