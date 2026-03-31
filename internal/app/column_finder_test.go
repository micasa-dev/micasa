// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuzzyMatch_ExactPrefix(t *testing.T) {
	t.Parallel()
	score, positions := fuzzyMatch("Pro", "Projects")
	assert.NotZero(t, score)
	assert.Equal(t, []int{0, 1, 2}, positions)
}

func TestFuzzyMatch_CaseInsensitive(t *testing.T) {
	t.Parallel()
	score, _ := fuzzyMatch("pro", "Projects")
	assert.NotZero(t, score)
}

func TestFuzzyMatch_NonContiguous(t *testing.T) {
	t.Parallel()
	score, positions := fuzzyMatch("pj", "Projects")
	assert.NotZero(t, score)
	require.Len(t, positions, 2)
	assert.Equal(t, 0, positions[0])
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	t.Parallel()
	score, _ := fuzzyMatch("xyz", "Projects")
	assert.Zero(t, score)
}

func TestFuzzyMatch_EmptyQuery(t *testing.T) {
	t.Parallel()
	score, _ := fuzzyMatch("", "Projects")
	assert.NotZero(t, score, "empty query should match everything")
}

func TestFuzzyMatch_QueryLongerThanTarget(t *testing.T) {
	t.Parallel()
	score, _ := fuzzyMatch("very long query", "ID")
	assert.Zero(t, score)
}

func TestFuzzyMatch_PrefixScoresHigher(t *testing.T) {
	t.Parallel()
	prefixScore, _ := fuzzyMatch("na", "Name")
	midScore, _ := fuzzyMatch("na", "Maintenance")
	assert.Greater(t, prefixScore, midScore)
}

func TestSortFuzzyMatches_ScoreDescending(t *testing.T) {
	t.Parallel()
	matches := []columnFinderMatch{
		{Entry: columnFinderEntry{FullIndex: 0, Title: "A"}, Score: 10},
		{Entry: columnFinderEntry{FullIndex: 1, Title: "B"}, Score: 30},
		{Entry: columnFinderEntry{FullIndex: 2, Title: "C"}, Score: 20},
	}
	sortFuzzyScored(matches)
	assert.Equal(t, 30, matches[0].Score)
	assert.Equal(t, 20, matches[1].Score)
	assert.Equal(t, 10, matches[2].Score)
}

func TestSortFuzzyMatches_TiebreakByIndex(t *testing.T) {
	t.Parallel()
	matches := []columnFinderMatch{
		{Entry: columnFinderEntry{FullIndex: 5, Title: "E"}, Score: 10},
		{Entry: columnFinderEntry{FullIndex: 2, Title: "B"}, Score: 10},
	}
	sortFuzzyScored(matches)
	assert.Equal(t, 2, matches[0].Entry.FullIndex)
	assert.Equal(t, 5, matches[1].Entry.FullIndex)
}

func TestColumnFinderState_RefilterEmpty(t *testing.T) {
	t.Parallel()
	cf := &columnFinderState{
		All: []columnFinderEntry{
			{FullIndex: 0, Title: "ID"},
			{FullIndex: 1, Title: "Name"},
			{FullIndex: 2, Title: "Status"},
		},
	}
	cf.refilter()
	assert.Len(t, cf.Matches, 3)
}

func TestColumnFinderState_RefilterNarrows(t *testing.T) {
	t.Parallel()
	cf := &columnFinderState{
		All: []columnFinderEntry{
			{FullIndex: 0, Title: "ID"},
			{FullIndex: 1, Title: "Name"},
			{FullIndex: 2, Title: "Status"},
			{FullIndex: 3, Title: "Maintenance"},
		},
	}
	cf.Query = "na"
	cf.refilter()
	require.Len(t, cf.Matches, 2)
	// "Name" should score higher than "Maintenance" because of prefix.
	assert.Equal(t, "Name", cf.Matches[0].Entry.Title)
}

func TestColumnFinderState_CursorClamps(t *testing.T) {
	t.Parallel()
	cf := &columnFinderState{
		All: []columnFinderEntry{
			{FullIndex: 0, Title: "ID"},
			{FullIndex: 1, Title: "Name"},
		},
		Cursor: 5,
	}
	cf.refilter()
	assert.Equal(t, 1, cf.Cursor)
}

func TestOpenColumnFinder(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.openColumnFinder()
	require.NotNil(t, m.columnFinder)
	assert.NotEmpty(t, m.columnFinder.All)
	assert.Contains(t, m.buildView(), "Jump to Column")
}

func TestColumnFinderJump(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	tab := m.effectiveTab()
	require.NotNil(t, tab)
	origCol := tab.ColCursor

	m.openColumnFinder()
	cf := m.columnFinder
	cf.Cursor = len(cf.Matches) - 1
	targetIdx := cf.Matches[cf.Cursor].Entry.FullIndex

	m.columnFinderJump()
	assert.Nil(t, m.columnFinder)
	assert.NotContains(t, m.buildView(), "Jump to Column", "finder should close after jump")
	if origCol != targetIdx {
		assert.NotEqual(t, origCol, tab.ColCursor, "ColCursor should have moved")
	}
	assert.Equal(t, targetIdx, tab.ColCursor)
}

func TestColumnFinderJump_UnhidesHiddenColumn(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	tab := m.effectiveTab()
	if tab == nil || len(tab.Specs) < 3 {
		t.Skip("need at least 3 columns")
	}

	// Hide column 2.
	tab.Specs[2].HideOrder = 1

	m.openColumnFinder()
	cf := m.columnFinder

	// Find the match for the hidden column.
	found := false
	for i, match := range cf.Matches {
		if match.Entry.FullIndex == 2 {
			cf.Cursor = i
			found = true
			break
		}
	}
	require.True(t, found, "hidden column should still appear in finder")

	m.columnFinderJump()
	assert.Equal(t, 0, tab.Specs[2].HideOrder, "jumping to hidden column should unhide it")
	assert.Equal(t, 2, tab.ColCursor)
}

func TestHandleColumnFinderKey_EscCloses(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.openColumnFinder()
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Nil(t, m.columnFinder)
	assert.Contains(t, m.statusView(), "NAV", "finder should close after esc")
}

func TestHandleColumnFinderKey_Typing(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.openColumnFinder()
	cf := m.columnFinder
	initial := len(cf.Matches)

	// Type "st" to filter.
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: 't', Text: "t"})

	assert.Equal(t, "st", cf.Query)
	if initial > 1 {
		assert.Less(t, len(cf.Matches), initial, "typing should narrow matches")
	}
}

func TestHandleColumnFinderKey_Backspace(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.openColumnFinder()
	cf := m.columnFinder

	m.handleColumnFinderKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	require.Equal(t, "ab", cf.Query)

	m.handleColumnFinderKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "a", cf.Query)
}

func TestHandleColumnFinderKey_BackspaceMultibyte(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.openColumnFinder()
	cf := m.columnFinder

	// Type a multi-byte character followed by an ASCII character.
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: 'ü', Text: "ü"})
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.Equal(t, "üx", cf.Query)

	// Backspace should remove 'x', leaving the full 'ü' intact.
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, "ü", cf.Query)

	// Backspace again should remove 'ü' entirely, not just one byte.
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Empty(t, cf.Query)
}

func TestHandleColumnFinderKey_CtrlU(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.openColumnFinder()
	cf := m.columnFinder

	m.handleColumnFinderKey(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	assert.Empty(t, cf.Query)
}

func TestHandleColumnFinderKey_Navigation(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.openColumnFinder()
	cf := m.columnFinder
	if len(cf.Matches) < 2 {
		t.Skip("need at least 2 columns")
	}

	assert.Equal(t, 0, cf.Cursor)

	m.handleColumnFinderKey(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, cf.Cursor)

	m.handleColumnFinderKey(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, cf.Cursor)

	// Should clamp at top.
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, cf.Cursor)
}

func TestBuildColumnFinderOverlay_CursorBeforeGhostText(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 80
	m.height = 40
	m.openColumnFinder()

	rendered := m.buildColumnFinderOverlay()

	// The bar cursor (│) should appear before the ghost text, not after.
	cursorIdx := strings.Index(rendered, "│")
	ghostIdx := strings.Index(rendered, "type to filter")
	require.NotEqual(t, -1, cursorIdx, "cursor should be present")
	require.NotEqual(t, -1, ghostIdx, "ghost text should be present")
	assert.Less(t, cursorIdx, ghostIdx,
		"cursor should appear before ghost text")
}

func TestBuildColumnFinderOverlay_StableHeight(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 80
	m.height = 40
	m.openColumnFinder()
	cf := m.columnFinder
	require.Greater(t, len(cf.All), 2, "need >2 columns to test narrowing")

	// Render with all matches visible.
	allMatchesView := m.buildColumnFinderOverlay()
	allLines := strings.Count(allMatchesView, "\n")

	// Type a character to narrow matches.
	m.handleColumnFinderKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	require.Less(t, len(cf.Matches), len(cf.All),
		"typing 's' should narrow matches")

	narrowView := m.buildColumnFinderOverlay()
	narrowLines := strings.Count(narrowView, "\n")

	assert.Equal(t, allLines, narrowLines,
		"overlay height should remain stable when matches narrow")
}

func TestBuildColumnFinderOverlay_ShowsColumns(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 80
	m.height = 24
	m.openColumnFinder()
	rendered := m.buildColumnFinderOverlay()
	assert.NotEmpty(t, rendered)
	assert.Contains(t, rendered, "Jump to Column")
}

func TestHighlightFuzzyMatch(t *testing.T) {
	t.Parallel()
	match := columnFinderMatch{
		Entry:     columnFinderEntry{Title: "Status"},
		Score:     50,
		Positions: []int{0, 1},
	}
	result := highlightFuzzyMatch(match)
	assert.Contains(t, result, "St")
	assert.Contains(t, result, "atus")
}

func TestSlashBlockedOnDashboard(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	handled := m.handleDashboardKeys(tea.KeyPressMsg{Code: '/', Text: "/"})
	assert.True(t, handled, "/ should be blocked on dashboard")
}

func TestSlashOpensColumnFinder(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "/")
	assert.NotNil(t, m.columnFinder)
	assert.Contains(t, m.buildView(), "Jump to Column",
		"/ in Normal mode should open column finder")
}

func TestSlashBlockedInEditMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.mode = modeEdit
	sendKey(m, "/")
	assert.Nil(t, m.columnFinder)
	assert.NotContains(t, m.buildView(), "Jump to Column",
		"/ should not open column finder in Edit mode")
}

func TestColumnFinderEnterJumps(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.openColumnFinder()
	cf := m.columnFinder
	if len(cf.Matches) < 2 {
		t.Skip("need at least 2 columns")
	}
	// Move to second match.
	cf.Cursor = 1
	target := cf.Matches[1].Entry.FullIndex

	m.handleColumnFinderKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, m.columnFinder)
	assert.NotContains(t, m.buildView(), "Jump to Column", "finder should close after enter")
	tab := m.effectiveTab()
	assert.Equal(t, target, tab.ColCursor)
}
