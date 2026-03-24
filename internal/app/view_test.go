// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildViewShowsFullHouseBox(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.hasHouse = true
	m.house = data.HouseProfile{Nickname: "Test House"}

	output := m.buildView()
	lines := strings.Split(output, "\n")

	// The rounded border top-left corner must be on the first line.
	require.NotEmpty(t, lines, "buildView returned empty output")
	assert.Contains(t, lines[0], "╭", "first line should contain the top border")
}

func TestExpandedHouseViewNoEllipsis(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.height = 40

	// Toggle house expanded.
	sendKey(m, "tab")
	require.True(t, m.showHouse)

	// At screenshot dimensions (2400px/32pt font ~ 120-125 columns) the
	// house profile must render without ellipsis truncation.
	for _, width := range []int{120, 160, 200} {
		m.width = width
		house := m.houseView()
		clamped := clampLines(house, width)
		for i, line := range strings.Split(clamped, "\n") {
			assert.NotContains(t, line, symEllipsis,
				"width %d: line %d truncated", width, i)
		}
	}
}

func TestBuildViewShowsTerminalTooSmallMessage(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = minUsableWidth - 1
	m.height = minUsableHeight - 1
	m.showDashboard = true
	m.notePreview = &notePreviewState{text: "test"}

	output := m.buildView()
	assert.Contains(t, output, "Terminal too small")
	assert.Contains(t, output, "need at least 80x24")
}

func TestBuildViewDoesNotShowTerminalTooSmallMessageAtMinimumSize(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = minUsableWidth
	m.height = minUsableHeight

	output := m.buildView()
	assert.NotContains(t, output, "Terminal too small")
}

func TestNaturalWidthsIgnoreMax(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "ID", Min: 4, Max: 6},
		{Title: "Name", Min: 8, Max: 12},
	}
	rows := [][]cell{
		{{Value: "1"}, {Value: "A very long name indeed"}},
	}
	natural := naturalWidths(specs, rows, "$")
	// "A very long name indeed" is 23 chars, well past Max of 12.
	assert.Greater(t, natural[1], 12)
}

func TestColumnWidthsNoTruncationWhenRoomAvailable(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "ID", Min: 4, Max: 6},
		{Title: "Name", Min: 8, Max: 12},
	}
	rows := [][]cell{
		{{Value: "1"}, {Value: "A long name here"}},
	}
	// "A long name here" = 16 chars, exceeds Max=12.
	// With 200 width and 3 separator, natural widths should fit.
	widths := columnWidths(specs, rows, 200, 3, nil)
	assert.GreaterOrEqual(t, widths[1], 16)
}

func TestColumnWidthsTruncatesWhenTerminalNarrow(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "ID", Min: 4, Max: 6},
		{Title: "Name", Min: 8, Max: 12, Flex: true},
	}
	rows := [][]cell{
		{{Value: "1"}, {Value: "A very long name indeed"}},
	}
	// Very narrow terminal: 20 total - 3 separator = 17 available.
	widths := columnWidths(specs, rows, 20, 3, nil)
	total := widths[0] + widths[1]
	assert.LessOrEqual(t, total, 17)
}

func TestColumnWidthsTruncatedColumnsGetExtraFirst(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "ID", Min: 4, Max: 6},
		{Title: "Name", Min: 8, Max: 10},
		{Title: "Desc", Min: 8, Max: 10, Flex: true},
	}
	rows := [][]cell{
		{{Value: "1"}, {Value: "Fifteen chars!!"}, {Value: "short"}},
	}
	widths := columnWidths(specs, rows, 60, 3, nil)
	assert.GreaterOrEqual(t, widths[1], 15)
}

func TestWidenTruncated(t *testing.T) {
	t.Parallel()
	t.Run("distributes all extra space", func(t *testing.T) {
		widths := []int{4, 10, 8}
		remaining := widenTruncated(widths, []int{4, 15, 8}, 3)
		assert.Equal(t, 13, widths[1])
		assert.Equal(t, 0, remaining)
	})
	t.Run("caps at natural width", func(t *testing.T) {
		widths := []int{4, 10, 8}
		remaining := widenTruncated(widths, []int{4, 12, 8}, 5)
		assert.Equal(t, 12, widths[1])
		assert.Equal(t, 3, remaining)
	})
}

// --- Column visibility tests ---

func TestNextVisibleCol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		specs   []columnSpec
		from    int
		forward bool
		want    int
	}{
		{
			"forward skips hidden",
			[]columnSpec{{Title: "A"}, {Title: "B", HideOrder: 1}, {Title: "C"}, {Title: "D"}},
			0,
			true,
			2,
		},
		{
			"backward skips hidden",
			[]columnSpec{{Title: "A"}, {Title: "B", HideOrder: 1}, {Title: "C"}, {Title: "D"}},
			2,
			false,
			0,
		},
		{
			"clamps forward at edge",
			[]columnSpec{{Title: "A"}, {Title: "B", HideOrder: 1}, {Title: "C", HideOrder: 2}},
			0,
			true,
			0,
		},
		{
			"clamps backward at edge",
			[]columnSpec{{Title: "A"}, {Title: "B", HideOrder: 1}, {Title: "C", HideOrder: 2}},
			0,
			false,
			0,
		},
		{"all visible forward", []columnSpec{{Title: "A"}, {Title: "B"}, {Title: "C"}}, 1, true, 2},
		{
			"clamps at right edge",
			[]columnSpec{{Title: "A"}, {Title: "B"}, {Title: "C"}},
			2,
			true,
			2,
		},
		{
			"clamps at left edge",
			[]columnSpec{{Title: "A"}, {Title: "B"}, {Title: "C"}},
			0,
			false,
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, nextVisibleCol(tt.specs, tt.from, tt.forward))
		})
	}
}

func TestFirstVisibleCol(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "A", HideOrder: 1}, {Title: "B"}, {Title: "C"}, {Title: "D"},
	}
	assert.Equal(t, 1, firstVisibleCol(specs))
}

func TestLastVisibleCol(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "A"}, {Title: "B"}, {Title: "C"}, {Title: "D", HideOrder: 1},
	}
	assert.Equal(t, 2, lastVisibleCol(specs))
}

func TestVisibleCount(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "A"}, {Title: "B", HideOrder: 1}, {Title: "C"},
	}
	assert.Equal(t, 2, visibleCount(specs))
}

func TestVisibleProjectionSkipsHidden(t *testing.T) {
	t.Parallel()
	tab := &Tab{
		Specs: []columnSpec{
			{Title: "ID"}, {Title: "Name", HideOrder: 1}, {Title: "Status"},
		},
		CellRows: [][]cell{
			{{Value: "1"}, {Value: "Test"}, {Value: "active"}},
		},
		ColCursor: 2,
		Sorts:     []sortEntry{{Col: 2, Dir: sortAsc}},
	}
	specs, cells, cursor, sorts, visToFull := visibleProjection(tab)
	require.Len(t, specs, 2)
	assert.Equal(t, "ID", specs[0].Title)
	assert.Equal(t, "Status", specs[1].Title)
	require.Len(t, cells[0], 2)
	assert.Equal(t, "1", cells[0][0].Value)
	assert.Equal(t, "active", cells[0][1].Value)
	assert.Equal(t, 1, cursor)
	require.Len(t, sorts, 1)
	assert.Equal(t, 1, sorts[0].Col)
	assert.Equal(t, []int{0, 2}, visToFull)
}

func TestVisibleProjectionHiddenCursor(t *testing.T) {
	t.Parallel()
	tab := &Tab{
		Specs:     []columnSpec{{Title: "A"}, {Title: "B", HideOrder: 1}},
		CellRows:  [][]cell{{{Value: "1"}, {Value: "2"}}},
		ColCursor: 1,
	}
	_, _, cursor, _, _ := visibleProjection(tab)
	assert.Equal(t, -1, cursor)
}

func TestVisibleProjectionHiddenSortOmitted(t *testing.T) {
	t.Parallel()
	tab := &Tab{
		Specs:    []columnSpec{{Title: "A"}, {Title: "B", HideOrder: 1}},
		CellRows: [][]cell{{{Value: "1"}, {Value: "2"}}},
		Sorts:    []sortEntry{{Col: 1, Dir: sortAsc}},
	}
	_, _, _, sorts, _ := visibleProjection(tab)
	assert.Empty(t, sorts)
}

func TestHideCurrentColumnPreventsLastVisible(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.mode = modeNormal
	m.showDashboard = false
	tab := m.effectiveTab()
	// Hide all but one.
	for i := 1; i < len(tab.Specs); i++ {
		tab.Specs[i].HideOrder = i
	}
	tab.ColCursor = 0
	sendKey(m, "c")
	assert.Equal(t, 0, tab.Specs[0].HideOrder, "should not hide the last visible column")
	assert.Equal(t, statusError, m.status.Kind)
}

func TestHideCurrentColumnMovesToNext(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.mode = modeNormal
	m.showDashboard = false
	tab := m.effectiveTab()
	tab.ColCursor = 0
	sendKey(m, "c")
	assert.NotZero(t, tab.Specs[0].HideOrder, "expected column 0 to be hidden")
	assert.Equal(
		t,
		0,
		tab.Specs[tab.ColCursor].HideOrder,
		"cursor should be on a visible column after hiding",
	)
}

func TestShowAllColumns(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.mode = modeNormal
	m.showDashboard = false
	tab := m.effectiveTab()
	tab.Specs[1].HideOrder = 1
	tab.Specs[2].HideOrder = 2
	sendKey(m, "C")
	for i, s := range tab.Specs {
		assert.Equalf(t, 0, s.HideOrder, "expected column %d to be visible", i)
	}
}

func TestJoinCells(t *testing.T) {
	t.Parallel()
	t.Run("per-gap separators", func(t *testing.T) {
		assert.Equal(
			t,
			"A | B \u22ef C",
			joinCells([]string{"A", "B", "C"}, []string{" | ", " \u22ef "}),
		)
	})
	t.Run("fallback separator", func(t *testing.T) {
		assert.Equal(t, "A | B | C", joinCells([]string{"A", "B", "C"}, []string{" | "}))
	})
}

func TestGapSeparators(t *testing.T) {
	t.Parallel()
	t.Run("detects collapsed gaps", func(t *testing.T) {
		normal := "\u2502"
		plainSeps, collapsedSeps := gapSeparators([]int{0, 3, 4}, 5, normal)
		require.Len(t, collapsedSeps, 2)
		assert.NotEqual(t, normal, collapsedSeps[0], "first gap should be collapsed separator")
		assert.Equal(t, normal, collapsedSeps[1], "second gap should be normal separator")
		assert.Equal(t, normal, plainSeps[0])
		assert.Equal(t, normal, plainSeps[1])
	})
	t.Run("single column returns empty", func(t *testing.T) {
		plainSeps, collapsedSeps := gapSeparators([]int{2}, 5, "\u2502")
		assert.Empty(t, plainSeps)
		assert.Empty(t, collapsedSeps)
	})
}

func TestHiddenColumnNames(t *testing.T) {
	t.Parallel()
	t.Run("returns hidden names in order", func(t *testing.T) {
		specs := []columnSpec{
			{Title: "ID"},
			{Title: "Name", HideOrder: 1},
			{Title: "Status"},
			{Title: "Cost", HideOrder: 2},
		}
		assert.Equal(t, []string{"Name", "Cost"}, hiddenColumnNames(specs))
	})
	t.Run("empty when none hidden", func(t *testing.T) {
		assert.Empty(t, hiddenColumnNames([]columnSpec{{Title: "A"}, {Title: "B"}}))
	})
}

func TestNextHideOrder(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "A", HideOrder: 3},
		{Title: "B"},
		{Title: "C", HideOrder: 1},
	}
	assert.Equal(t, 4, nextHideOrder(specs))
}

func TestRenderHiddenBadges(t *testing.T) {
	t.Parallel()
	t.Run("empty when none hidden", func(t *testing.T) {
		specs := []columnSpec{{Title: "A"}, {Title: "B"}}
		assert.Empty(t, renderHiddenBadges(specs, 0))
	})
	t.Run("left only", func(t *testing.T) {
		specs := []columnSpec{{Title: "ID", HideOrder: 1}, {Title: "Name"}, {Title: "Status"}}
		assert.Contains(t, renderHiddenBadges(specs, 2), "ID")
	})
	t.Run("right only", func(t *testing.T) {
		specs := []columnSpec{{Title: "ID"}, {Title: "Name"}, {Title: "Cost", HideOrder: 1}}
		assert.Contains(t, renderHiddenBadges(specs, 0), "Cost")
	})
	t.Run("both sides", func(t *testing.T) {
		specs := []columnSpec{
			{Title: "ID", HideOrder: 1},
			{Title: "Name"},
			{Title: "Cost", HideOrder: 2},
		}
		out := renderHiddenBadges(specs, 1)
		assert.Contains(t, out, "ID")
		assert.Contains(t, out, "Cost")
	})
}

func TestRenderHiddenBadgesStableWidthAcrossCursorMoves(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "ID", HideOrder: 1},
		{Title: "Name"},
		{Title: "Cost", HideOrder: 2},
		{Title: "Status"},
	}

	start := renderHiddenBadges(specs, 0)
	middle := renderHiddenBadges(specs, 1)
	end := renderHiddenBadges(specs, 3)

	startW := lipgloss.Width(start)
	middleW := lipgloss.Width(middle)
	endW := lipgloss.Width(end)
	assert.Equal(t, startW, middleW, "start vs middle badge width")
	assert.Equal(t, middleW, endW, "middle vs end badge width")
}

func TestColumnWidthsFixedValuesStillStabilize(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "Status", Min: 8, Max: 12, FixedValues: []string{
			"ideating", "planned", "underway", "completed", "abandoned",
		}},
	}
	rows := [][]cell{
		{{Value: "planned"}},
	}
	widths := columnWidths(specs, rows, 80, 3, nil)
	assert.GreaterOrEqual(t, widths[0], 9)
}

// --- Line clamping tests ---

func TestClampLines(t *testing.T) {
	t.Parallel()
	t.Run("truncates long line", func(t *testing.T) {
		assert.Equal(t, "hell…", clampLines("hello world", 5))
	})
	t.Run("multiline truncates only long lines", func(t *testing.T) {
		got := clampLines("short\na very long line here\nok", 8)
		lines := strings.Split(got, "\n")
		require.Len(t, lines, 3)
		assert.Equal(t, "short", lines[0])
		assert.Equal(t, "ok", lines[2])
		assert.Less(t, len(lines[1]), len("a very long line here"))
	})
	t.Run("noop when fits", func(t *testing.T) {
		assert.Equal(t, "fits", clampLines("fits", 100))
	})
}

func TestTruncateLeft(t *testing.T) {
	t.Parallel()
	t.Run("truncates long path", func(t *testing.T) {
		got := truncateLeft("/home/user/long/path/to/data.db", 15)
		assert.True(t, strings.HasPrefix(got, "\u2026"))
		assert.True(t, strings.HasSuffix(got, "data.db"))
		assert.LessOrEqual(t, lipgloss.Width(got), 15)
	})
	t.Run("noop when fits", func(t *testing.T) {
		assert.Equal(t, "short.db", truncateLeft("short.db", 20))
	})
	t.Run("grapheme clusters", func(t *testing.T) {
		got := truncateLeft("\U0001F1EF\U0001F1F5/path/to/file.db", 15)
		assert.LessOrEqual(t, lipgloss.Width(got), 15)
		assert.True(t, strings.HasPrefix(got, "\u2026"))
	})
	t.Run("zero and negative width", func(t *testing.T) {
		assert.Empty(t, truncateLeft("anything", 0))
		assert.Empty(t, truncateLeft("anything", -1))
	})
}

func TestShortenHome(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	t.Run("replaces home prefix with tilde", func(t *testing.T) {
		p := filepath.Join(home, ".local", "share", "micasa", "micasa.db")
		got := shortenHome(p)
		assert.Equal(t, filepath.Join("~", ".local", "share", "micasa", "micasa.db"), got)
	})
	t.Run("exact home dir becomes tilde", func(t *testing.T) {
		assert.Equal(t, "~", shortenHome(home))
	})
	t.Run("non-home path unchanged", func(t *testing.T) {
		assert.Equal(t, "/tmp/other.db", shortenHome("/tmp/other.db"))
	})
	t.Run("home as substring does not match", func(t *testing.T) {
		// e.g. /home/user2 should NOT be shortened when home is /home/user
		p := home + "2/data.db"
		assert.Equal(t, p, shortenHome(p))
	})
}

// --- Viewport tests ---

func TestViewportAllColumnsFit(t *testing.T) {
	t.Parallel()
	widths := []int{10, 15, 10}
	start, end, hasL, hasR := viewportRange(widths, 3, 50, 0, 0)
	assert.Equal(t, 0, start)
	assert.Equal(t, 3, end)
	assert.False(t, hasL)
	assert.False(t, hasR)
}

func TestViewportScrollsRight(t *testing.T) {
	t.Parallel()
	widths := []int{10, 10, 10, 10, 10}
	start, end, hasL, _ := viewportRange(widths, 3, 30, 0, 3)
	assert.LessOrEqual(t, start, 3, "start should be <= cursor")
	assert.Greater(t, end, 3, "end should be > cursor")
	assert.True(t, hasL, "expected left indicator when scrolled right")
}

func TestViewportScrollsLeftOnCursorMove(t *testing.T) {
	t.Parallel()
	tab := &Tab{ViewOffset: 3}
	ensureCursorVisible(tab, 1, 5)
	widths := []int{10, 10, 10, 10, 10}
	start, end, _, _ := viewportRange(widths, 3, 30, tab.ViewOffset, 1)
	assert.LessOrEqual(t, start, 1)
	assert.Greater(t, end, 1)
}

func TestEnsureCursorVisibleClamps(t *testing.T) {
	t.Parallel()
	tab := &Tab{ViewOffset: 5}
	ensureCursorVisible(tab, 2, 4)
	assert.LessOrEqual(t, tab.ViewOffset, 2)
}

func TestEnsureCursorVisibleNoopWhenVisible(t *testing.T) {
	t.Parallel()
	tab := &Tab{ViewOffset: 0}
	ensureCursorVisible(tab, 2, 5)
	assert.Equal(t, 0, tab.ViewOffset)
}

func TestViewportSorts(t *testing.T) {
	t.Parallel()
	t.Run("adjusts column indices by offset", func(t *testing.T) {
		adjusted := viewportSorts([]sortEntry{{Col: 3, Dir: sortAsc}, {Col: 5, Dir: sortDesc}}, 2)
		assert.Equal(t, 1, adjusted[0].Col)
		assert.Equal(t, 3, adjusted[1].Col)
	})
	t.Run("no offset passthrough", func(t *testing.T) {
		adjusted := viewportSorts([]sortEntry{{Col: 1, Dir: sortAsc}}, 0)
		assert.Equal(t, 1, adjusted[0].Col)
		assert.Equal(t, sortAsc, adjusted[0].Dir)
	})
}

func TestApplianceAge(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		purchase *time.Time
		want     string
	}{
		{"nil purchase", nil, ""},
		{"less than a month", ptr(time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)), "<1m"},
		{"a few months", ptr(time.Date(2025, 10, 5, 0, 0, 0, 0, time.UTC)), "4m"},
		{"one year exact", ptr(time.Date(2025, 2, 10, 0, 0, 0, 0, time.UTC)), "1y"},
		{"years and months", ptr(time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)), "2y 7m"},
		{"future date", ptr(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, applianceAge(tt.purchase, now))
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestNavBadgeLabel(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	status := m.statusView()
	assert.Contains(t, status, "NAV")
	assert.NotContains(t, status, "NORMAL")
}

func TestStatusBarStableWidthWithFilters(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 200
	m.height = 40

	// Measure the hint line width before any filtering.
	before := m.statusView()
	beforeW := lipgloss.Width(before)

	// Add pins and activate filter — hint bar width should not change.
	tab := m.activeTab()
	require.NotNil(t, tab)
	tab.Pins = []filterPin{{Col: 0, Values: map[string]bool{"test": true}}}
	tab.FilterActive = true
	after := m.statusView()
	afterW := lipgloss.Width(after)

	assert.Equal(t, beforeW, afterW, "status bar width should not change with filtering")
}

func TestStatusViewUsesMoreLabelWhenHintsCollapse(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.height = 40
	// At very narrow width, the help hint compacts from "help" to "more".
	// Add an enter hint to increase the hint count enough to trigger collapse.
	m.width = 20
	tab := m.activeTab()
	require.NotNil(t, tab)
	// Put cursor on a drilldown column to generate an enter hint.
	for i, spec := range tab.Specs {
		if spec.Kind == cellDrilldown {
			tab.ColCursor = i
			break
		}
	}
	status := m.statusView()
	assert.Contains(t, status, "more", "expected collapsed hint label to include more")
}

func TestHelpContentIncludesProjectStatusFilterShortcut(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	help := m.helpContent()
	assert.Contains(t, help, "Toggle settled projects")
}

func TestHelpContentHasGlobalSection(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	help := m.helpContent()
	assert.Contains(t, help, "Global")
	assert.Contains(t, help, "Quit")
	assert.Contains(t, help, "Cancel LLM")
}

func TestHelpContentEditModeHalfPage(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	help := m.helpContent()
	assert.Contains(t, help, "CTRL+D")
	assert.Contains(t, help, "Half page down")
}

func TestHelpContentNavModeEsc(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	help := m.helpContent()
	assert.Contains(t, help, "Close detail")
}

func TestHelpContentFormsShowsFieldNavigation(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	help := m.helpContent()
	assert.Contains(t, help, "Next field")
	assert.Contains(t, help, "Previous field")
}

func TestHelpContentExcludesDatePicker(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	help := m.helpContent()
	assert.NotContains(t, help, "Date Picker",
		"date picker is a transient widget and should not appear in global help")
}

func TestHelpContentShowsArrowKeyAlternatives(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	help := m.helpContent()
	// Nav mode row/column bindings include arrow symbols (renderKeys splits
	// on "/" so they appear as individual badges, not as "↑/↓").
	assert.Contains(t, help, "\u2191")
	assert.Contains(t, help, "\u2193")
	assert.Contains(t, help, "\u2190")
	assert.Contains(t, help, "\u2192")
}

func TestHeaderTitleWidth(t *testing.T) {
	t.Parallel()
	// Single-column: sort indicator is " ▲" (2 chars).
	siw := sortIndicatorWidth(1)
	assert.Equal(t, 2, siw)

	tests := []struct {
		name string
		spec columnSpec
		cols int
		want int
	}{
		{
			"link",
			columnSpec{Title: "Project", Link: &columnLink{TargetTab: tabProjects}},
			1,
			lipgloss.Width("Project") + 1 + lipgloss.Width(linkArrow) + siw,
		},
		{
			"drilldown",
			columnSpec{Title: "Log", Kind: cellDrilldown},
			1,
			lipgloss.Width("Log") + 1 + lipgloss.Width(drilldownArrow) + siw,
		},
		{"plain", columnSpec{Title: "Name"}, 1, lipgloss.Width("Name") + siw},
		{
			"money",
			columnSpec{Title: "Budget", Kind: cellMoney},
			1,
			lipgloss.Width("Budget") + 1 + 1 + siw,
		},
		{
			"entity",
			columnSpec{Title: "Entity", Kind: cellEntity},
			1,
			lipgloss.Width("Entity") + 1 + lipgloss.Width(linkArrow) + siw,
		},
		{
			"multi-col plain",
			columnSpec{Title: "ID"},
			8,
			lipgloss.Width("ID") + sortIndicatorWidth(8),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, headerTitleWidth(tt.spec, tt.cols, "$"))
		})
	}
}

// renderTestHeader is a helper that mirrors the real rendering pipeline:
// naturalWidths → columnWidths → annotateMoneyHeaders → renderHeaderRow.
// Returns the rendered header string for the given specs, sorts, and width.
func renderTestHeader(
	specs []columnSpec,
	sorts []sortEntry,
	termWidth int,
) string {
	cur := locale.DefaultCurrency()
	sepW := lipgloss.Width(" │ ")
	widths := columnWidths(specs, nil, termWidth, sepW, nil)
	seps := make([]string, len(specs)-1)
	for i := range seps {
		seps[i] = " │ "
	}
	headerSpecs := annotateMoneyHeaders(specs, cur)
	vpSorts := make([]sortEntry, len(sorts))
	copy(vpSorts, sorts)
	return renderHeaderRow(
		headerSpecs,
		widths,
		seps,
		0,
		vpSorts,
		false,
		false,
		nil,
		zone.New(),
		zoneCol,
	)
}

// Regression: user reported "ID" truncated to "I" on the quotes table
// when multi-column sort is active.
func TestMultiSortDoesNotTruncateShortTitle(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "ID"},
		{Title: "Project"},
		{Title: "Vendor"},
		{Title: "Total", Kind: cellMoney},
	}
	sorts := []sortEntry{
		{Col: 0, Dir: sortAsc},
		{Col: 1, Dir: sortDesc},
	}
	rendered := renderTestHeader(specs, sorts, 120)
	assert.Contains(t, rendered, "ID",
		"short column title must not be truncated by multi-sort indicator")
}

// Regression: user reported money "$" eaten by multi-column sort indicator.
func TestMultiSortDoesNotEatDollarSign(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "ID"},
		{Title: "Total", Kind: cellMoney},
		{Title: "Labor", Kind: cellMoney},
	}
	sorts := []sortEntry{
		{Col: 1, Dir: sortAsc},
		{Col: 2, Dir: sortDesc},
	}
	rendered := renderTestHeader(specs, sorts, 120)
	assert.Contains(t, rendered, "$",
		"money $ must survive multi-column sort")
}

// Regression: user reported drilldown "↘" arrow eaten by sort indicator.
func TestMultiSortDoesNotEatDrilldownArrow(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "ID"},
		{Title: "Log", Kind: cellDrilldown},
		{Title: "Status"},
	}
	sorts := []sortEntry{
		{Col: 1, Dir: sortAsc},
		{Col: 2, Dir: sortDesc},
	}
	rendered := renderTestHeader(specs, sorts, 120)
	assert.Contains(t, rendered, drilldownArrow,
		"drilldown arrow must survive multi-column sort")
}

// Sort indicator must always include a leading space.
func TestSortIndicatorIncludesLeadingSpace(t *testing.T) {
	t.Parallel()
	indicator := sortIndicator([]sortEntry{{Col: 0, Dir: sortAsc}}, 0)
	assert.True(t, strings.HasPrefix(indicator, " "),
		"sort indicator must start with a space")
}

func TestColumnHasLinks(t *testing.T) {
	t.Parallel()
	rows := [][]cell{
		{{Value: "Self", LinkID: ""}, {Value: "42"}},
		{{Value: "Vendor A", LinkID: "01JNOTEXIST000000000000005"}, {Value: "43"}},
	}
	assert.True(t, columnHasLinks(rows, 0), "column 0 has a linked row")
	assert.False(t, columnHasLinks(rows, 1), "column 1 has no linked rows")
}

func TestColumnHasLinks_AllZero(t *testing.T) {
	t.Parallel()
	rows := [][]cell{
		{{Value: "Self", LinkID: ""}},
		{{Value: "Self", LinkID: ""}},
	}
	assert.False(t, columnHasLinks(rows, 0))
}

func TestColumnHasLinks_Empty(t *testing.T) {
	t.Parallel()
	assert.False(t, columnHasLinks(nil, 0))
	assert.False(t, columnHasLinks([][]cell{}, 0))
}

func TestDimBackgroundNeutralizesCancelFaint(t *testing.T) {
	t.Parallel()
	// Simulate a composited overlay: cancelFaint injects \033[22m (normal
	// intensity) so the overlay content stays bright. A subsequent
	// dimBackground pass must neutralize those markers so the entire
	// background dims uniformly (nested overlay scenario).
	inner := cancelFaint("dashboard content")
	assert.Contains(t, inner, "\033[22m", "cancelFaint should inject normal-intensity")

	dimmed := dimBackground(inner)
	assert.NotContains(t, dimmed, "\033[22m",
		"dimBackground should neutralize cancel-faint markers from nested overlays")
	assert.Contains(t, dimmed, "\033[2m", "dimBackground should apply faint")
}

func TestNormalModeOmitsDiscoveryHints(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 200 // very wide so nothing gets dropped by priority
	m.height = 40
	status := m.statusView()

	// These keybinding hints should be discoverable only via the help
	// overlay, not cluttering the status bar.
	for _, removed := range []string{"find col", "hide col", "sort", "pin"} {
		assert.NotContains(t, status, removed,
			"did not expect %q hint in redesigned normal-mode status bar", removed)
	}

	// Primary actions should still be visible.
	assert.Contains(t, status, "NAV")
	assert.Contains(t, status, "edit")
	assert.Contains(t, status, "help")

	// ctrl+q quit is discoverable via help, not shown in the status bar.
	assert.NotContains(t, status, "quit")
}

func TestEditModeOmitsProfile(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 200
	m.height = 40
	m.mode = modeEdit
	status := m.statusView()

	// Profile is discoverable via help, not shown in edit mode bar.
	assert.NotContains(t, status, "profile")

	// Primary edit actions should be present.
	assert.Contains(t, status, "EDIT")
	assert.Contains(t, status, "add")
	assert.Contains(t, status, "del")
	assert.Contains(t, status, "nav")
}

func TestAskHintHiddenWithoutLLM(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 200
	m.height = 40
	m.llmClient = nil
	status := m.statusView()
	assert.NotContains(t, status, "ask",
		"ask hint should be hidden when LLM client is nil")
}

func TestPinSummaryNotInStatusHints(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 200
	m.height = 40
	tab := m.activeTab()
	require.NotNil(t, tab)

	// Pin summary should not appear in the hint bar (the tab-row triangle
	// handles the visual indicator). This keeps the hint bar width stable.
	tab.Pins = []filterPin{{Col: 0, Values: map[string]bool{"test": true}}}
	status := m.statusView()
	assert.NotContains(t, status, "ID: test",
		"pin summary should not appear in status hints")
}

func TestFilterIndicatorOnTabRow(t *testing.T) {
	t.Parallel()
	m, tab := newFilterModel(t)
	m.width = 120
	m.height = 40

	// No pins — no indicator.
	tabs := m.tabsView()
	assert.NotContains(t, tabs, filterMarkActive)
	assert.NotContains(t, tabs, filterMarkPreview)

	// User pins a value: preview indicator ▽.
	sendKey(m, "n")
	require.NotEmpty(t, tab.Pins)
	tabs = m.tabsView()
	assert.Contains(t, tabs, filterMarkPreview)

	// User activates filter (N): active indicator ▼.
	sendKey(m, "N")
	require.True(t, tab.FilterActive)
	tabs = m.tabsView()
	assert.Contains(t, tabs, filterMarkActive)

	// User inverts (!): active+inverted indicator ▲.
	sendKey(m, "!")
	require.True(t, tab.FilterInverted)
	tabs = m.tabsView()
	assert.Contains(t, tabs, filterMarkActiveInverted)

	// User deactivates filter (N): preview+inverted indicator △.
	sendKey(m, "N")
	require.False(t, tab.FilterActive)
	tabs = m.tabsView()
	assert.Contains(t, tabs, filterMarkPreviewInverted)

	// User clears pins (ctrl+n): no indicator.
	sendKey(m, keyCtrlN)
	tabs = m.tabsView()
	assert.NotContains(t, tabs, filterMarkActive)
	assert.NotContains(t, tabs, filterMarkActiveInverted)
	assert.NotContains(t, tabs, filterMarkPreview)
	assert.NotContains(t, tabs, filterMarkPreviewInverted)
}

// TestTabsLockedInFormMode verifies that tabsLocked returns true in form
// mode (inactive tabs struck through), matching edit-mode behavior.
func TestTabsLockedInFormMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)

	m.mode = modeNormal
	assert.False(t, m.tabsLocked(), "normal mode should not lock tabs")

	m.mode = modeEdit
	assert.True(t, m.tabsLocked(), "edit mode should lock tabs")

	m.mode = modeForm
	assert.True(t, m.tabsLocked(), "form mode should lock tabs")
}

func TestHelpContentIncludesInvertFilter(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	help := m.helpContent()
	assert.Contains(t, help, "Invert filter")
}

func TestRowCountShowsDeletedCount(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 200
	m.height = 40
	tab := m.effectiveTab()
	require.NotNil(t, tab)

	// Mark the single row as deleted and enable ShowDeleted.
	tab.Rows = []rowMeta{{ID: "01JTEST00000000000000001", Deleted: true}}
	tab.ShowDeleted = true
	view := m.tableView(tab)
	assert.Contains(t, view, "1 deleted",
		"row count should include deleted count when ShowDeleted is on")
}

func TestEmptyHintPerTab(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind    TabKind
		want    string
		wantSub string // secondary substring to verify
	}{
		{tabProjects, "No projects yet", "edit mode"},
		{tabQuotes, "No quotes yet", "Create a project first"},
		{tabMaintenance, "No maintenance items yet", "edit mode"},
		{tabIncidents, "No incidents yet", "edit mode"},
		{tabAppliances, "No appliances yet", "edit mode"},
		{tabVendors, "No vendors yet", "edit mode"},
		{tabDocuments, "No documents yet", ""},
	}
	for _, tt := range tests {
		hint := topLevelEmptyHint(tt.kind)
		assert.Contains(t, hint, tt.want)
		if tt.wantSub != "" {
			assert.Contains(t, hint, tt.wantSub)
		}
	}
}

func TestEmptyHintDetailDrilldown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		parentKind TabKind
		subName    string
		wantSub    string // expected substring like "No docs for this appliance"
	}{
		{tabAppliances, tabDocuments.String(), "No docs for this appliance"},
		{tabProjects, tabDocuments.String(), "No docs for this project"},
		{tabIncidents, tabDocuments.String(), "No docs for this incident"},
		{tabProjects, tabQuotes.String(), "No quotes for this project"},
		{tabVendors, tabQuotes.String(), "No quotes for this vendor"},
		{tabVendors, "Jobs", "No jobs for this vendor"},
		{tabAppliances, "Maintenance", "No maintenance for this appliance"},
		{tabMaintenance, "Service Log", "No service log for this maintenance item"},
	}
	for _, tt := range tests {
		m := newTestModel(t)
		// Push a detail context so m.inDetail() is true.
		m.detailStack = append(m.detailStack, &detailContext{
			Tab: Tab{Kind: tt.parentKind, Name: tt.subName},
		})
		hint := m.emptyHint(&m.detailStack[0].Tab)
		assert.Contains(t, hint, tt.wantSub,
			"parentKind=%s subName=%s", tt.parentKind, tt.subName)
		assert.Contains(t, hint, "edit mode",
			"detail hint should include edit-mode guidance")
	}
}

func TestRowCountLabel(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, testSeed)
	tab := m.activeTab()
	require.NotNil(t, tab)
	require.Greater(t, len(tab.Rows), 1, "demo data should populate multiple rows")

	t.Run("plural", func(t *testing.T) {
		output := m.tableView(tab)
		expected := fmt.Sprintf("%d rows", len(tab.Rows))
		assert.Contains(t, output, expected)
	})

	t.Run("singular", func(t *testing.T) {
		// Trim to exactly one row to verify singular form.
		tab.Rows = tab.Rows[:1]
		tab.CellRows = tab.CellRows[:1]
		tab.Table.SetRows(tab.Table.Rows()[:1])

		output := m.tableView(tab)
		assert.Contains(t, output, "1 row")
		assert.NotContains(t, output, "1 rows")
	})
}

func TestRowCountHiddenWhenEmpty(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	tab := m.activeTab()
	require.NotNil(t, tab)
	require.Empty(t, tab.Rows, "store-only model should have no project rows")

	output := m.tableView(tab)
	assert.NotContains(t, output, "row")
}

func TestRowCountUpdatesAcrossTabs(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, testSeed)

	// Check that every populated tab shows its own row count.
	for i := range m.tabs {
		tab := &m.tabs[i]
		if len(tab.Rows) == 0 {
			continue
		}
		output := m.tableView(tab)
		expected := fmt.Sprintf("%d rows", len(tab.Rows))
		if len(tab.Rows) == 1 {
			expected = "1 row"
		}
		assert.Contains(t, output, expected,
			"tab %q should show row count", tab.Kind)
	}
}

func TestRequiredLegendShownOnMultiFieldForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.width = 120
	m.height = 40
	m.startProjectForm()
	m.fs.form.Init()

	output := m.buildView()
	assert.Contains(t, output, "required",
		"multi-field form should show required-field legend")
}

func TestRequiredLegendShownOnFullScreenHouseForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.width = 120
	m.height = 40
	m.startHouseForm()
	m.fs.form.Init()

	// House form renders via formFullScreen, not buildBaseView.
	output := m.buildView()
	assert.Contains(t, output, "required",
		"fullscreen house form should show required-field legend")
}

func TestRequiredLegendHiddenOnInlineEdit(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.width = 120
	m.height = 40

	// Create a project so we can inline-edit it.
	m.startProjectForm()
	m.fs.form.Init()
	values, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	values.Title = testProjectTitle
	require.NoError(t, m.submitProjectForm())
	m.exitForm()
	m.reloadAll()

	// Inline-edit the Status column -- a Select via openInlineEdit.
	tab := m.activeTab()
	require.NotEmpty(t, tab.Rows)
	require.NoError(t, m.inlineEditProject(tab.Rows[0].ID, projectColStatus))
	require.Equal(t, modeForm, m.mode, "inline edit should activate form mode")
	m.fs.form.Init()

	output := m.buildView()
	assert.NotContains(t, output, "required",
		"inline edit should not show required-field legend")
}

func TestFirstRunHouseFormShowsHint(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	// Simulate first run: no house profile exists yet.
	m.hasHouse = false
	m.startHouseForm()
	m.fs.form.Init()

	output := m.buildView()
	assert.Contains(t, output, "edit the rest anytime",
		"first-run house form should hint that only nickname is required")
}

func TestEditHouseFormHidesHint(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	// Model already has a house from newTestModelWithStore.
	openHouseForm(m)
	m.fs.form.Init()

	output := m.buildView()
	assert.NotContains(t, output, "edit the rest anytime",
		"editing existing house profile should not show first-run hint")
}

func TestPluralCoversAllTabKinds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind TabKind
		want string
	}{
		{tabProjects, "projects"},
		{tabQuotes, "quotes"},
		{tabMaintenance, "maintenance items"},
		{tabIncidents, "incidents"},
		{tabAppliances, "appliances"},
		{tabVendors, "vendors"},
		{tabDocuments, "documents"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.kind.plural())
	}
}

func TestOverlayMaxHeightClampsSmallTerminal(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.height = 10
	h := m.overlayMaxHeight()
	assert.Equal(t, 10, h, "should clamp to minimum of 10")
}

func TestOverlayMaxHeightNormalTerminal(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.height = 40
	h := m.overlayMaxHeight()
	assert.Equal(t, m.effectiveHeight()-4, h)
}

func TestNaturalWidthsIndirectMatchesDirect(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "ID", Min: 2, Max: 6},
		{Title: "Name", Min: 4, Max: 20},
	}
	fullRows := [][]cell{
		{{Value: "1"}, {Value: "alpha"}, {Value: "extra"}},
		{{Value: "2"}, {Value: "beta"}, {Value: "extra"}},
	}
	visToFull := []int{0, 1}
	indirect := naturalWidthsIndirect(specs, fullRows, visToFull, "$")
	direct := naturalWidths(specs, fullRows, "$")
	assert.Equal(t, direct, indirect)
}

func TestNaturalWidthsIndirectRemappedColumns(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "Name", Min: 4, Max: 20},
	}
	fullRows := [][]cell{
		{{Value: "1"}, {Value: "alpha"}, {Value: "extra"}},
	}
	visToFull := []int{1}
	widths := naturalWidthsIndirect(specs, fullRows, visToFull, "$")
	require.Len(t, widths, 1)
	assert.GreaterOrEqual(t, widths[0], lipgloss.Width("alpha"))
}

func TestComputeNaturalWidthsSkipsShortRows(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "A", Min: 2, Max: 10},
		{Title: "B", Min: 2, Max: 10},
	}
	rows := [][]cell{
		{{Value: "x"}},
	}
	widths := naturalWidths(specs, rows, "$")
	require.Len(t, widths, 2)
	assert.GreaterOrEqual(t, widths[0], 2)
	assert.GreaterOrEqual(t, widths[1], 2)
}
