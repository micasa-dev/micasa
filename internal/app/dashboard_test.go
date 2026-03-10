// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDummyLLMClient returns a non-nil LLM client for tests that need
// insightsWanted() to return true. Not connected to a real server.
func testDummyLLMClient(t *testing.T) *llm.Client {
	t.Helper()
	c, err := llm.NewClient("llamacpp", "http://127.0.0.1:1", "test", "", time.Second, 0)
	require.NoError(t, err)
	return c
}

// nonEmptyDashboard returns a minimal dashboardData that is not empty,
// for tests that just need the dashboard overlay to render.
func nonEmptyDashboard() dashboardData {
	return dashboardData{
		OpenIncidents: []data.Incident{{Title: "stub"}},
	}
}

func TestDaysUntil(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 2, 8, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		target time.Time
		want   int
	}{
		{"same day", time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC), 0},
		{"tomorrow", time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC), 1},
		{"yesterday", time.Date(2026, 2, 7, 0, 0, 0, 0, time.UTC), -1},
		{"30 days ahead", time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), 30},
		{"10 days ago", time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC), -10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, daysUntil(now, tt.target))
		})
	}
}

func TestDaysText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		days int
		want string
	}{
		{0, "today"},
		{-5, "5d"},
		{-1, "1d"},
		{3, "3d"},
		{1, "1d"},
		{-45, "1mo"},
		{400, "1y"},
	}
	for _, tt := range tests {
		assert.Equalf(t, tt.want, daysText(tt.days),
			"daysText(%d)", tt.days)
	}
}

func TestSortByDays(t *testing.T) {
	t.Parallel()
	items := []maintenanceUrgency{
		{DaysFromNow: 10},
		{DaysFromNow: -5},
		{DaysFromNow: 2},
		{DaysFromNow: -20},
	}
	sortByDays(items)
	for i := 1; i < len(items); i++ {
		assert.GreaterOrEqualf(t, items[i].DaysFromNow, items[i-1].DaysFromNow,
			"not sorted: items[%d]=%d < items[%d]=%d",
			i, items[i].DaysFromNow, i-1, items[i-1].DaysFromNow)
	}
}

func TestCapSlice(t *testing.T) {
	t.Parallel()
	assert.Len(t, capSlice([]int{1, 2, 3, 4, 5}, 3), 3)
	assert.Len(t, capSlice([]int{1, 2}, 5), 2)
	assert.Empty(t, capSlice([]int{1, 2, 3}, -1))
}

func TestDashboardToggle(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = false

	sendKey(m, "D")
	assert.True(t, m.showDashboard)
	sendKey(m, "D")
	assert.False(t, m.showDashboard)
}

func TestDashboardDismissedByTabSwitch(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"f", "b"} {
		t.Run(key, func(t *testing.T) {
			m := newTestModel(t)
			m.showDashboard = true
			sendKey(m, key)
			assert.False(t, m.showDashboard)
		})
	}
}

func TestDashboardNavigation(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	m.dash.data = nonEmptyDashboard()
	// Populate nav with 5 entries.
	m.dash.nav = []dashNavEntry{
		{Tab: tabMaintenance, ID: 1},
		{Tab: tabMaintenance, ID: 2},
		{Tab: tabProjects, ID: 3},
		{Tab: tabAppliances, ID: 4},
		{Tab: tabMaintenance, ID: 5},
	}
	m.dash.cursor = 0

	// j moves down.
	sendKey(m, "j")
	assert.Equal(t, 1, m.dash.cursor)
	// k moves up.
	sendKey(m, "k")
	assert.Equal(t, 0, m.dash.cursor)
	// k at 0 stays at 0 (no wrap).
	sendKey(m, "k")
	assert.Equal(t, 0, m.dash.cursor)
	// G jumps to bottom.
	sendKey(m, "G")
	assert.Equal(t, 4, m.dash.cursor)
	// j at bottom stays at bottom.
	sendKey(m, "j")
	assert.Equal(t, 4, m.dash.cursor)
	// g jumps to top.
	sendKey(m, "g")
	assert.Equal(t, 0, m.dash.cursor)
}

func TestDashboardEnterKeyJumps(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	m.dash.data = nonEmptyDashboard()
	m.dash.nav = []dashNavEntry{
		{Tab: tabMaintenance, ID: 1},
		{Tab: tabProjects, ID: 42},
	}
	m.dash.cursor = 1

	sendKey(m, "enter")
	assert.False(t, m.showDashboard)
	assert.Equal(t, tabIndex(tabProjects), m.active)
}

func TestDashboardBlocksTableKeys(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	m.dash.data = nonEmptyDashboard()
	m.dash.nav = []dashNavEntry{{Tab: tabMaintenance, ID: 1}}
	m.dash.cursor = 0
	startTab := m.active

	// h/l should not move column cursor.
	colBefore := m.tabs[m.active].ColCursor
	sendKey(m, "l")
	assert.Equal(t, colBefore, m.tabs[m.active].ColCursor, "l should be blocked on dashboard")
	sendKey(m, "h")
	assert.Equal(t, colBefore, m.tabs[m.active].ColCursor, "h should be blocked on dashboard")

	// s should not add a sort.
	sortsBefore := len(m.tabs[m.active].Sorts)
	sendKey(m, "s")
	assert.Len(t, m.tabs[m.active].Sorts, sortsBefore, "s should be blocked on dashboard")

	// i should not enter edit mode.
	sendKey(m, "i")
	assert.Equal(t, modeNormal, m.mode, "i should be blocked on dashboard")

	// tab should not toggle house profile.
	houseBefore := m.showHouse
	sendKey(m, "tab")
	assert.Equal(t, houseBefore, m.showHouse, "tab should be blocked on dashboard")

	// Dashboard should still be showing.
	assert.True(t, m.showDashboard)
	assert.Equal(t, startTab, m.active)
}

func TestDashboardViewEmptySections(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.dash.data = dashboardData{}
	m.dash.nav = nil
	m.dash.cursor = 0
	m.prepareDashboardView()

	view := m.dashboardView(50, 120)
	// Empty dashboard returns empty string -- silence is success.
	assert.Empty(t, view)
}

func TestDashboardViewWithData(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)
	overdueDue := time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)
	lastSrv := time.Date(2025, 10, 25, 0, 0, 0, 0, time.UTC)

	m.dash.data = dashboardData{
		Overdue: []maintenanceUrgency{{
			Item: data.MaintenanceItem{
				Name:           "HVAC Filter",
				LastServicedAt: &lastSrv,
			},
			NextDue:     overdueDue,
			DaysFromNow: daysUntil(now, overdueDue),
		}},
		ActiveProjects: []data.Project{{
			Title:  "Kitchen Remodel",
			Status: data.ProjectStatusInProgress,
		}},
	}
	// Expand all sections so data rows are visible.
	m.dash.expanded = map[string]bool{
		dashSectionOverdue:  true,
		dashSectionProjects: true,
	}
	m.prepareDashboardView()

	view := m.dashboardView(50, 120)
	assert.Contains(t, view, "HVAC Filter")
	assert.Contains(t, view, "Kitchen Remodel")
	assert.Contains(t, view, "overdue")
}

func TestDashboardViewSeasonalSection(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40

	m.dash.data = dashboardData{
		Seasonal: []data.MaintenanceItem{
			{ID: 1, Name: "Clean Gutters", Season: data.SeasonSpring},
			{ID: 2, Name: "Service AC", Season: data.SeasonSpring},
		},
	}
	m.dash.expanded = map[string]bool{
		dashSectionSeasonal: true,
	}
	m.prepareDashboardView()

	view := m.dashboardView(50, 120)
	assert.Contains(t, view, "Clean Gutters")
	assert.Contains(t, view, "Service AC")
	assert.Contains(t, view, "Seasonal")
}

func TestDashboardSeasonalEmpty(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40

	m.dash.data = dashboardData{
		Seasonal: nil,
	}
	m.prepareDashboardView()

	view := m.dashboardView(50, 120)
	assert.NotContains(t, view, "Seasonal",
		"seasonal section should not appear when there are no seasonal items")
}

func TestDashboardViewIncidentsFirst(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)
	overdueDue := time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)

	m.dash.data = dashboardData{
		OpenIncidents: []data.Incident{{
			Title:    "Burst pipe",
			Status:   data.IncidentStatusOpen,
			Severity: data.IncidentSeverityUrgent,
		}},
		Overdue: []maintenanceUrgency{{
			Item:        data.MaintenanceItem{Name: "HVAC Filter"},
			NextDue:     overdueDue,
			DaysFromNow: daysUntil(now, overdueDue),
		}},
		ActiveProjects: []data.Project{{
			Title:  "Kitchen Remodel",
			Status: data.ProjectStatusInProgress,
		}},
	}
	// Expand all sections so data rows are visible for ordering check.
	m.dash.expanded = map[string]bool{
		dashSectionIncidents: true,
		dashSectionOverdue:   true,
		dashSectionProjects:  true,
	}
	m.prepareDashboardView()

	view := m.dashboardView(50, 120)
	incIdx := strings.Index(view, "Burst pipe")
	overdueIdx := strings.Index(view, "HVAC Filter")
	projIdx := strings.Index(view, "Kitchen Remodel")

	require.NotEqual(t, -1, incIdx, "incidents should appear in view")
	require.NotEqual(t, -1, overdueIdx, "overdue items should appear in view")
	require.NotEqual(t, -1, projIdx, "projects should appear in view")

	assert.Less(t, incIdx, overdueIdx,
		"incidents should appear before overdue items")
	assert.Less(t, overdueIdx, projIdx,
		"overdue items should appear before projects")
}

func TestDashboardViewFitsOverlayWidth(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 80
	m.height = 40

	// Build data with long names that would wrap without truncation.
	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)
	overdueDue := time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)
	lastSrv := time.Date(2025, 10, 25, 0, 0, 0, 0, time.UTC)

	m.dash.data = dashboardData{
		Overdue: []maintenanceUrgency{{
			Item: data.MaintenanceItem{
				Name:           "Refrigerator coil cleaning and deep inspection",
				LastServicedAt: &lastSrv,
			},
			NextDue:       overdueDue,
			DaysFromNow:   daysUntil(now, overdueDue),
			ApplianceName: "Kitchen Refrigerator",
		}},
		ActiveProjects: []data.Project{{
			Title:  "Kitchen countertop upgrade with premium materials",
			Status: data.ProjectStatusInProgress,
		}},
	}
	m.prepareDashboardView()

	innerW := m.overlayContentWidth() - 4

	view := m.dashboardView(50, innerW)
	for i, line := range strings.Split(view, "\n") {
		w := lipgloss.Width(line)
		assert.LessOrEqual(t, w, innerW,
			"line %d width %d exceeds overlay inner width %d: %q",
			i, w, innerW, line)
	}
}

func TestDashboardOverlay(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.showDashboard = true
	m.dash.data = dashboardData{}
	m.dash.nav = nil

	ov := m.buildDashboardOverlay()
	today := time.Now().Format("Monday, Jan 2")
	assert.Contains(t, ov, today)
	assert.Contains(t, ov, "help")
}

func TestDashboardOverlayFitsHeight(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 80
	m.height = 30
	m.showDashboard = true

	// Lots of data to stress the height budget.
	overdue := make([]maintenanceUrgency, 12)
	for i := range overdue {
		overdue[i] = maintenanceUrgency{
			Item: data.MaintenanceItem{
				ID:   uint(i + 1), //nolint:gosec // i bounded by slice length (≤12)
				Name: fmt.Sprintf("Long maintenance task %d", i+1),
			},
			DaysFromNow:   -(i + 1),
			ApplianceName: "Big Appliance",
		}
	}
	projects := make([]data.Project, 8)
	for i := range projects {
		projects[i] = data.Project{
			Title:  fmt.Sprintf("Project with a fairly long name %d", i+1),
			Status: data.ProjectStatusInProgress,
		}
		projects[i].ID = uint(100 + i) //nolint:gosec // i bounded by slice length (≤8)
	}
	m.dash.data = dashboardData{
		Overdue:        overdue,
		ActiveProjects: projects,
	}
	m.buildDashNav()

	overlay := m.buildDashboardOverlay()
	overlayH := lipgloss.Height(overlay)
	maxH := m.effectiveHeight() - 4
	assert.LessOrEqual(t, overlayH, maxH,
		"overlay height %d exceeds max %d", overlayH, maxH)
}

func TestDashboardOverlayDimsSurroundingContent(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.showDashboard = true
	// Populate dashboard with data so the overlay actually renders.
	m.dash.data = nonEmptyDashboard()

	view := m.buildView()
	// Every line of the composited view that contains background content
	// (the tab underline, table headers, etc.) should carry the ANSI faint
	// attribute (\033[2m). Verify no line contains the tab underline
	// character without being wrapped in faint.
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "━") {
			assert.Contains(t, line, "\033[2m",
				"tab underline should be dimmed in overlay")
		}
	}
}

func TestDashboardHiddenWhenEmpty(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.showDashboard = true
	m.dash.data = dashboardData{}

	view := m.buildView()
	// Empty dashboard should not show the overlay — no dimming.
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "━") {
			assert.NotContains(t, line, "\033[2m",
				"empty dashboard should not dim the background")
		}
	}
}

func TestDashboardStatusBarShowsNormal(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.showDashboard = true
	m.dash.data = nonEmptyDashboard()
	status := m.statusView()
	// With overlay active, main tab keybindings should be hidden.
	assert.NotContains(t, status, "NAV")
}

func TestBuildDashNav(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)
	overdueDue := time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)

	m.dash.data = dashboardData{
		OpenIncidents: []data.Incident{
			{Title: "Burst pipe", Severity: data.IncidentSeverityUrgent},
		},
		Overdue: []maintenanceUrgency{{
			Item:        data.MaintenanceItem{ID: 10, Name: "Filter"},
			NextDue:     overdueDue,
			DaysFromNow: daysUntil(now, overdueDue),
		}},
		ActiveProjects: []data.Project{{ID: 20, Title: "Deck"}},
		ExpiringWarranties: []warrantyStatus{{
			Appliance:   data.Appliance{ID: 30, Name: "Fridge"},
			DaysFromNow: 45,
		}},
	}
	m.dash.data.OpenIncidents[0].ID = 5
	// Expand incidents (default) so its data rows appear in nav.
	m.dash.expanded = map[string]bool{
		dashSectionIncidents: true,
	}
	m.buildDashNav()

	// Nav has: incidents header + 1 row, overdue header, projects header,
	// expiring header = 5 entries. Only incidents is expanded.
	require.Len(t, m.dash.nav, 5)

	// Incidents header first, then data row.
	assert.True(t, m.dash.nav[0].IsHeader)
	assert.Equal(t, dashSectionIncidents, m.dash.nav[0].Section)
	assert.Equal(t, tabIncidents, m.dash.nav[1].Tab)
	assert.Equal(t, uint(5), m.dash.nav[1].ID)

	// Collapsed sections: just headers.
	assert.True(t, m.dash.nav[2].IsHeader)
	assert.Equal(t, dashSectionOverdue, m.dash.nav[2].Section)
	assert.True(t, m.dash.nav[3].IsHeader)
	assert.Equal(t, dashSectionProjects, m.dash.nav[3].Section)
	assert.True(t, m.dash.nav[4].IsHeader)
	assert.Equal(t, dashSectionExpiring, m.dash.nav[4].Section)

	// Expand overdue, verify its data rows appear.
	m.dash.expanded[dashSectionOverdue] = true
	m.buildDashNav()
	require.Len(t, m.dash.nav, 6) // +1 data row
	assert.Equal(t, tabMaintenance, m.dash.nav[3].Tab)
	assert.Equal(t, uint(10), m.dash.nav[3].ID)
}

func TestRenderMiniTable(t *testing.T) {
	t.Parallel()
	rows := []dashRow{
		{Cells: []dashCell{
			{Text: "Short", Style: lipgloss.NewStyle()},
			{Text: "123", Style: lipgloss.NewStyle(), Align: alignRight},
		}},
		{Cells: []dashCell{
			{Text: "Longer name", Style: lipgloss.NewStyle()},
			{Text: "7", Style: lipgloss.NewStyle(), Align: alignRight},
		}},
	}
	lines := renderMiniTable(nil, rows, 3, 0, -1, lipgloss.NewStyle(), lipgloss.NewStyle())
	require.Len(t, lines, 2)
	// Both lines should have the same visible width due to column alignment.
	assert.Equal(t, lipgloss.Width(lines[0]), lipgloss.Width(lines[1]))
}

func TestRenderMiniTableUnicode(t *testing.T) {
	t.Parallel()
	plain := lipgloss.NewStyle()

	t.Run("accented Latin", func(t *testing.T) {
		rows := []dashRow{
			{Cells: []dashCell{
				{Text: "Garc\u00eda Plumbing", Style: plain},
				{Text: "3 days overdue", Style: plain, Align: alignRight},
			}},
			{Cells: []dashCell{
				{Text: "HVAC Filter", Style: plain},
				{Text: "in 14 days", Style: plain, Align: alignRight},
			}},
		}
		lines := renderMiniTable(nil, rows, 3, 0, -1, plain, plain)
		require.Len(t, lines, 2)
		assert.Equal(t, lipgloss.Width(lines[0]), lipgloss.Width(lines[1]),
			"rows with accented characters should align")
	})

	t.Run("CJK wide characters", func(t *testing.T) {
		// CJK characters occupy 2 terminal cells each.
		rows := []dashRow{
			{Cells: []dashCell{
				{Text: "\u6771\u829d\u88fd\u54c1", Style: plain}, // 東芝製品 = 8 cells
				{Text: "$500", Style: plain, Align: alignRight},
			}},
			{Cells: []dashCell{
				{Text: "Short", Style: plain}, // 5 cells
				{Text: "$1,000", Style: plain, Align: alignRight},
			}},
		}
		lines := renderMiniTable(nil, rows, 3, 0, -1, plain, plain)
		require.Len(t, lines, 2)
		assert.Equal(t, lipgloss.Width(lines[0]), lipgloss.Width(lines[1]),
			"rows with CJK characters should align")
	})

	t.Run("emoji", func(t *testing.T) {
		rows := []dashRow{
			{Cells: []dashCell{
				{Text: "Check \u2705", Style: plain},
				{Text: "done", Style: plain},
			}},
			{Cells: []dashCell{
				{Text: "Long task name", Style: plain},
				{Text: "pending", Style: plain},
			}},
		}
		lines := renderMiniTable(nil, rows, 3, 0, -1, plain, plain)
		require.Len(t, lines, 2)
		assert.Equal(t, lipgloss.Width(lines[0]), lipgloss.Width(lines[1]),
			"rows with emoji should align")
	})
}

func TestRenderMiniTableTruncatesOnNarrowWidth(t *testing.T) {
	t.Parallel()
	plain := lipgloss.NewStyle()
	rows := []dashRow{
		{Cells: []dashCell{
			{Text: "Very long maintenance item name here", Style: plain},
			{Text: "3 days overdue", Style: plain, Align: alignRight},
		}},
		{Cells: []dashCell{
			{Text: "Another long name for testing", Style: plain},
			{Text: "in 14 days", Style: plain, Align: alignRight},
		}},
	}

	// Without width cap, rows are as wide as content demands.
	uncapped := renderMiniTable(nil, rows, 3, 0, -1, plain, plain)
	require.Len(t, uncapped, 2)
	uncappedWidth := lipgloss.Width(uncapped[0])

	// With a tight width cap, rows should be truncated.
	capped := renderMiniTable(nil, rows, 3, 40, -1, plain, plain)
	require.Len(t, capped, 2)
	for i, line := range capped {
		w := lipgloss.Width(line)
		assert.LessOrEqual(t, w, 40,
			"line %d width %d exceeds maxWidth 40", i, w)
	}

	// Capped should be narrower than uncapped.
	assert.Less(t, lipgloss.Width(capped[0]), uncappedWidth,
		"capped lines should be narrower")

	// Truncated first column should contain an ellipsis.
	assert.Contains(t, capped[0], "\u2026", "truncated line should contain ellipsis")
}

func TestTruncateToWidth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		text  string
		maxW  int
		check func(t *testing.T, result string)
	}{
		{
			name: "fits within width",
			text: "hello",
			maxW: 10,
			check: func(t *testing.T, result string) {
				assert.Equal(t, "hello", result)
			},
		},
		{
			name: "truncated with ellipsis",
			text: "very long text here",
			maxW: 10,
			check: func(t *testing.T, result string) {
				assert.LessOrEqual(t, lipgloss.Width(result), 10)
				assert.Contains(t, result, "\u2026")
			},
		},
		{
			name: "CJK truncation",
			text: "\u6771\u829d\u88fd\u54c1\u682a\u5f0f\u4f1a\u793e", // 東芝製品株式会社
			maxW: 8,
			check: func(t *testing.T, result string) {
				assert.LessOrEqual(t, lipgloss.Width(result), 8)
				assert.Contains(t, result, "\u2026")
			},
		},
		{
			name: "width 1 returns ellipsis",
			text: "hello",
			maxW: 1,
			check: func(t *testing.T, result string) {
				assert.Equal(t, "\u2026", result)
			},
		},
		{
			name: "width 0 returns empty",
			text: "hello",
			maxW: 0,
			check: func(t *testing.T, result string) {
				assert.Empty(t, result)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateToWidth(tt.text, tt.maxW)
			tt.check(t, result)
		})
	}
}

func TestDashboardViewScrollsWithSmallBudget(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40

	overdue := make([]maintenanceUrgency, 8)
	for i := range overdue {
		overdue[i] = maintenanceUrgency{
			Item: data.MaintenanceItem{
				ID:   uint(i + 1), //nolint:gosec // i bounded by slice length (≤8)
				Name: fmt.Sprintf("Task %d", i+1),
			},
			DaysFromNow: -(i + 1),
		}
	}
	projects := make([]data.Project, 5)
	for i := range projects {
		projects[i] = data.Project{
			Title:  fmt.Sprintf("Proj %d", i+1),
			Status: "underway",
		}
		projects[i].ID = uint(100 + i) //nolint:gosec // i bounded by slice length (≤5)
	}
	m.dash.data = dashboardData{
		Overdue:        overdue,
		ActiveProjects: projects,
	}
	m.dash.expanded = map[string]bool{
		dashSectionOverdue:  true,
		dashSectionProjects: true,
	}
	m.prepareDashboardView()

	// With a generous budget, all rows appear.
	bigView := m.dashboardView(100, 120)
	for i := 1; i <= 8; i++ {
		assert.Containsf(
			t,
			bigView,
			fmt.Sprintf("Task %d", i),
			"expected Task %d in big-budget view",
			i,
		)
	}

	// Nav: 2 headers + 8 overdue rows + 5 project rows = 15.
	assert.Len(t, m.dash.nav, 15, "nav should have all entries")

	// With a tiny budget the view is scrolled, not trimmed.
	m.dash.expanded = map[string]bool{
		dashSectionOverdue:  true,
		dashSectionProjects: true,
	}
	m.prepareDashboardView()
	m.dash.cursor = 0
	smallView := m.dashboardView(6, 120)
	lines := strings.Split(smallView, "\n")
	assert.LessOrEqual(t, len(lines), 6, "scrolled view should fit budget")
	assert.Greater(t, m.dash.totalLines, 6, "total lines should exceed budget")
}

func TestDashboardScrollFollowsCursor(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40

	overdue := make([]maintenanceUrgency, 10)
	for i := range overdue {
		overdue[i] = maintenanceUrgency{
			Item: data.MaintenanceItem{
				ID:   uint(i + 1), //nolint:gosec // i bounded by slice length (≤10)
				Name: fmt.Sprintf("Item %d", i+1),
			},
			DaysFromNow: -(i + 1),
		}
	}
	m.dash.data = dashboardData{Overdue: overdue}
	m.dash.expanded = map[string]bool{dashSectionOverdue: true}
	m.prepareDashboardView()

	// Nav: 1 header + 10 rows = 11. Budget must fit pill + col header + tail.
	m.dash.cursor = 10
	view := m.dashboardView(8, 120)
	assert.Contains(t, view, "Item 10", "cursor item should be visible")
	assert.Positive(t, m.dash.scrollOffset, "should have scrolled down")

	// All nav entries are preserved (header + 10 rows).
	assert.Len(t, m.dash.nav, 11, "nav should have all entries")
}

// TestDashboardExpandCollapseWithEKey verifies a user can toggle section
// expand/collapse with e and bulk-toggle with E, as they would in a real
// session.
func TestDashboardExpandCollapseWithEKey(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	m.width = 120
	m.height = 40

	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)
	overdueDue := time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)
	m.dash.data = dashboardData{
		OpenIncidents: []data.Incident{{
			Title:    "Burst pipe",
			Severity: data.IncidentSeverityUrgent,
		}},
		Overdue: []maintenanceUrgency{{
			Item:        data.MaintenanceItem{ID: 10, Name: "Filter"},
			NextDue:     overdueDue,
			DaysFromNow: daysUntil(now, overdueDue),
		}},
	}
	m.dash.data.OpenIncidents[0].ID = 5
	m.dash.expanded = map[string]bool{dashSectionIncidents: true}
	m.prepareDashboardView()

	// Incidents is expanded by default — data row should be in nav.
	view := m.dashboardView(50, 120)
	assert.Contains(t, view, "Burst pipe")

	// Cursor on Incidents header. Press e to collapse it.
	m.dash.cursor = 0
	sendKey(m, "e")
	assert.False(t, m.dash.expanded[dashSectionIncidents], "e should collapse current section")

	// Press E to expand all.
	sendKey(m, "E")
	assert.True(t, m.dash.expanded[dashSectionIncidents])
	assert.True(t, m.dash.expanded[dashSectionOverdue])

	// Press E again to collapse all.
	sendKey(m, "E")
	assert.False(t, m.dash.expanded[dashSectionIncidents])
	assert.False(t, m.dash.expanded[dashSectionOverdue])
}

// TestDashboardSectionNavWithShiftJK verifies J/K jump between section
// headers, simulating how a user skips through sections.
func TestDashboardSectionNavWithShiftJK(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	m.width = 120
	m.height = 40

	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)
	overdueDue := time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC)
	m.dash.data = dashboardData{
		OpenIncidents: []data.Incident{{
			Title:    "Burst pipe",
			Severity: data.IncidentSeverityUrgent,
		}},
		Overdue: []maintenanceUrgency{{
			Item:        data.MaintenanceItem{ID: 10, Name: "Filter"},
			DaysFromNow: daysUntil(now, overdueDue),
		}},
		ActiveProjects: []data.Project{{ID: 20, Title: "Deck"}},
	}
	m.dash.data.OpenIncidents[0].ID = 5
	m.dash.expanded = map[string]bool{dashSectionIncidents: true}
	m.buildDashNav()

	// Start on Incidents header.
	m.dash.cursor = 0
	assert.True(t, m.dash.nav[0].IsHeader)
	assert.Equal(t, dashSectionIncidents, m.dash.nav[0].Section)

	// J jumps to the next section header (Overdue).
	sendKey(m, "J")
	assert.True(t, m.dash.nav[m.dash.cursor].IsHeader)
	assert.Equal(t, dashSectionOverdue, m.dash.nav[m.dash.cursor].Section)

	// Another J jumps to Projects header.
	sendKey(m, "J")
	assert.True(t, m.dash.nav[m.dash.cursor].IsHeader)
	assert.Equal(t, dashSectionProjects, m.dash.nav[m.dash.cursor].Section)

	// K jumps back to Overdue.
	sendKey(m, "K")
	assert.Equal(t, dashSectionOverdue, m.dash.nav[m.dash.cursor].Section)
}

// TestDashboardEnterKeyJumpsToIncidents verifies a user can navigate to an
// incident row in the dashboard and press Enter to land on the Incidents tab
// with that incident selected.
func TestDashboardEnterKeyJumpsToIncidents(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	m.width = 120
	m.height = 40

	m.dash.data = dashboardData{
		OpenIncidents: []data.Incident{{
			Title:    "Burst pipe",
			Severity: data.IncidentSeverityUrgent,
		}},
	}
	m.dash.data.OpenIncidents[0].ID = 42
	m.dash.expanded = map[string]bool{dashSectionIncidents: true}
	m.buildDashNav()

	// Cursor starts on Incidents header. Move down to the data row.
	m.dash.cursor = 0
	sendKey(m, "j")
	require.False(t, m.dash.nav[m.dash.cursor].IsHeader, "should be on a data row")
	assert.Equal(t, uint(42), m.dash.nav[m.dash.cursor].ID)

	// Press Enter to jump.
	sendKey(m, "enter")
	assert.False(t, m.showDashboard, "dashboard should close")
	assert.Equal(t, tabIndex(tabIncidents), m.active, "should land on Incidents tab")
}

// TestDashboardEnterKeyJumpsToExpiring verifies a user can navigate to the
// Expiring Soon section, expand it, select a warranty row, and press Enter
// to land on the Appliances tab with that appliance selected.
func TestDashboardEnterKeyJumpsToExpiring(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	m.width = 120
	m.height = 40

	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)
	expiry := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	m.dash.data = dashboardData{
		OpenIncidents: []data.Incident{{
			Title:    "Burst pipe",
			Severity: data.IncidentSeverityUrgent,
		}},
		ExpiringWarranties: []warrantyStatus{{
			Appliance:   data.Appliance{Name: "Dishwasher", WarrantyExpiry: &expiry},
			DaysFromNow: daysUntil(now, expiry),
		}},
	}
	m.dash.data.OpenIncidents[0].ID = 5
	m.dash.data.ExpiringWarranties[0].Appliance.ID = 99
	m.dash.expanded = map[string]bool{dashSectionIncidents: true}
	m.buildDashNav()

	// Use J to jump to the Expiring section header.
	m.dash.cursor = 0
	sendKey(m, "J") // → Expiring header
	require.True(t, m.dash.nav[m.dash.cursor].IsHeader)
	assert.Equal(t, dashSectionExpiring, m.dash.nav[m.dash.cursor].Section,
		"J should land on Expiring Soon header")

	// Expand the section and rebuild nav (simulates the View cycle).
	sendKey(m, "e")
	m.buildDashNav()

	// Move down into the data row.
	sendKey(m, "j")
	require.False(t, m.dash.nav[m.dash.cursor].IsHeader, "should be on a data row")
	assert.Equal(t, uint(99), m.dash.nav[m.dash.cursor].ID)

	// Press Enter to jump to the Appliances tab.
	sendKey(m, "enter")
	assert.False(t, m.showDashboard, "dashboard should close")
	assert.Equal(t, tabIndex(tabAppliances), m.active, "should land on Appliances tab")
}

// TestDashboardExpiringNavWithInsuranceOnly verifies the Expiring Soon section
// is navigable when the only expiring item is an insurance renewal (no
// warranties). Before the fix, buildDashNav only included the section when
// ExpiringWarranties was non-empty, making the visible section unreachable.
func TestDashboardExpiringNavWithInsuranceOnly(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	m.width = 120
	m.height = 40

	renewal := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	m.dash.data = dashboardData{
		OpenIncidents: []data.Incident{{
			Title:    "Burst pipe",
			Severity: data.IncidentSeverityUrgent,
		}},
		InsuranceRenewal: &insuranceStatus{
			Carrier:     "Acme Insurance",
			RenewalDate: renewal,
			DaysFromNow: 60,
		},
	}
	m.dash.data.OpenIncidents[0].ID = 5
	m.dash.expanded = map[string]bool{dashSectionIncidents: true}
	m.buildDashNav()

	// The Expiring section should be in the nav even with only insurance.
	var found bool
	for _, entry := range m.dash.nav {
		if entry.IsHeader && entry.Section == dashSectionExpiring {
			found = true
			break
		}
	}
	require.True(t, found,
		"Expiring Soon header should be in nav with insurance-only data")

	// User can J-jump to the Expiring header.
	m.dash.cursor = 0
	sendKey(m, "J") // → Expiring header
	assert.Equal(t, dashSectionExpiring, m.dash.nav[m.dash.cursor].Section,
		"J should reach Expiring Soon header")

	// Expand and rebuild nav to verify the section expands without crashing.
	sendKey(m, "e")
	m.buildDashNav()
	m.prepareDashboardView()
	view := m.dashboardView(50, 120)
	assert.Contains(t, view, "Acme Insurance",
		"expanded section should show insurance renewal")
}

// TestDashboardEnterOnHeaderDoesNotJump verifies enter on a section header
// does nothing (user should press e to expand instead).
func TestDashboardEnterOnHeaderDoesNotJump(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	m.dash.data = nonEmptyDashboard()
	m.dash.expanded = map[string]bool{dashSectionIncidents: true}
	m.buildDashNav()
	m.dash.cursor = 0

	require.True(t, m.dash.nav[0].IsHeader)
	sendKey(m, "enter")
	assert.True(t, m.showDashboard, "enter on header should not dismiss dashboard")
}

// TestDashboardGoTopResetsScroll verifies that g (go to top) after scrolling
// down makes the first section header fully visible.
func TestDashboardGoTopResetsScroll(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.showDashboard = true

	overdue := make([]maintenanceUrgency, 10)
	for i := range overdue {
		overdue[i] = maintenanceUrgency{
			Item: data.MaintenanceItem{
				ID:   uint(i + 1), //nolint:gosec // i bounded by slice length (≤10)
				Name: fmt.Sprintf("Task %d", i+1),
			},
			DaysFromNow: -(i + 1),
		}
	}
	m.dash.data = dashboardData{Overdue: overdue}
	m.dash.expanded = map[string]bool{dashSectionOverdue: true}
	m.prepareDashboardView()

	// Go to bottom, then back to top.
	sendKey(m, "G")
	require.Positive(t, m.dash.cursor)
	m.prepareDashboardView()
	m.dashboardView(6, 120) // render to set scroll offset
	require.Positive(t, m.dash.scrollOffset, "should have scrolled down")

	sendKey(m, "g")
	assert.Equal(t, 0, m.dash.cursor)
	assert.Equal(t, 0, m.dash.scrollOffset, "g should reset scroll to top")
}

// TestDashboardDemoDataExpiringReachable loads real demo data (seed 42) and
// verifies that the Expiring Soon data rows are visible and navigable via j
// on a terminal with height 40.
func TestDashboardDemoDataExpiringReachable(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.width = 80
	m.height = 40
	m.showDashboard = true

	now := time.Now()
	require.NoError(t, m.loadDashboardAt(now))
	m.dash.expanded = map[string]bool{
		dashSectionIncidents: true,
		dashSectionOverdue:   true,
		dashSectionUpcoming:  true,
		dashSectionProjects:  true,
		dashSectionExpiring:  true,
	}
	m.prepareDashboardView()

	t.Logf(
		"dashboard sections: incidents=%d overdue=%d upcoming=%d projects=%d expiring=%d insurance=%v",
		len(m.dash.data.OpenIncidents),
		len(m.dash.data.Overdue),
		len(m.dash.data.Upcoming),
		len(m.dash.data.ActiveProjects),
		len(m.dash.data.ExpiringWarranties),
		m.dash.data.InsuranceRenewal != nil,
	)
	t.Logf("dashNav has %d entries", len(m.dash.nav))

	if len(m.dash.data.ExpiringWarranties) == 0 && m.dash.data.InsuranceRenewal == nil {
		t.Skip("no Expiring data in demo seed at current date")
	}

	// Navigate to bottom with G, then render.
	sendKey(m, "G")
	overlay := m.buildDashboardOverlay()

	// The Expiring section's data rows must be visible — including the
	// insurance renewal which has no nav Target. Before the fix, the
	// cursor couldn't reach the Expiring section when the only row was
	// the non-navigable insurance renewal, leaving it clipped below the
	// scroll window.
	require.Less(t, m.dash.cursor, len(m.dash.nav))
	assert.Equal(t, dashSectionExpiring, m.dash.nav[m.dash.cursor].Section,
		"cursor should reach Expiring section")
	assert.Contains(t, overlay, dashSectionExpiring,
		"Expiring section header should be visible")
	if m.dash.data.InsuranceRenewal != nil {
		assert.Contains(t, overlay, "Insurance renewal",
			"insurance renewal row should be visible when cursor is at bottom")
	}
	for _, w := range m.dash.data.ExpiringWarranties {
		assert.Contains(t, overlay, w.Appliance.Name,
			"warranty row should be visible when cursor is at bottom")
	}
}

// TestDashboardScrollReachesAllSections verifies that navigating with j/k
// through a fully-expanded dashboard on a short terminal reaches every
// section. The Incidents header must be visible when at the top, and
// Expiring Soon rows must be visible and navigable when at the bottom.
func TestDashboardScrollReachesAllSections(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 25 // short terminal — forces scrolling
	m.showDashboard = true

	m.dash.data = dashboardData{
		OpenIncidents: []data.Incident{
			{Title: "Burst pipe", Severity: data.IncidentSeverityUrgent},
		},
		Overdue: []maintenanceUrgency{
			{Item: data.MaintenanceItem{ID: 10, Name: "Filter"}, DaysFromNow: -5},
			{Item: data.MaintenanceItem{ID: 11, Name: "Gutters"}, DaysFromNow: -3},
		},
		Upcoming: []maintenanceUrgency{
			{Item: data.MaintenanceItem{ID: 20, Name: "HVAC"}, DaysFromNow: 7},
		},
		ActiveProjects: []data.Project{
			{Title: "Kitchen reno", Status: "underway"},
		},
		ExpiringWarranties: []warrantyStatus{
			{Appliance: data.Appliance{Name: "Dishwasher"}, DaysFromNow: 14},
			{Appliance: data.Appliance{Name: "Fridge"}, DaysFromNow: 30},
		},
	}
	m.dash.data.OpenIncidents[0].ID = 1
	m.dash.data.ActiveProjects[0].ID = 100
	m.dash.data.ExpiringWarranties[0].Appliance.ID = 200
	m.dash.data.ExpiringWarranties[1].Appliance.ID = 201

	// Expand all sections.
	m.dash.expanded = map[string]bool{
		dashSectionIncidents: true,
		dashSectionOverdue:   true,
		dashSectionUpcoming:  true,
		dashSectionProjects:  true,
		dashSectionExpiring:  true,
	}
	m.prepareDashboardView()

	// Navigate all the way down with j.
	for range len(m.dash.nav) {
		sendKey(m, "j")
	}
	overlay := m.buildDashboardOverlay()
	assert.Contains(t, overlay, "Dishwasher",
		"last section rows should be visible when scrolled to bottom")
	assert.Contains(t, overlay, "Fridge",
		"all Expiring rows should be visible when cursor is at bottom")

	// Navigate all the way back up with k.
	for range len(m.dash.nav) {
		sendKey(m, "k")
	}
	overlay = m.buildDashboardOverlay()
	assert.Contains(t, overlay, dashSectionIncidents,
		"Incidents header should be visible when scrolled to top")
}

// TestDashboardScrollIndicators verifies that scroll indicators appear when
// content is clipped, showing how many lines are hidden above/below. Uses
// sendKey to drive navigation through the real user code path.
func TestDashboardScrollIndicators(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 20 // short terminal forces scrolling
	m.showDashboard = true

	overdue := make([]maintenanceUrgency, 15)
	for i := range overdue {
		overdue[i] = maintenanceUrgency{
			Item: data.MaintenanceItem{
				ID:   uint(i + 1), //nolint:gosec // i bounded by slice length (<=15)
				Name: fmt.Sprintf("Task %d", i+1),
			},
			DaysFromNow: -(i + 1),
		}
	}
	m.dash.data = dashboardData{Overdue: overdue}
	m.dash.expanded = map[string]bool{dashSectionOverdue: true}
	m.prepareDashboardView()

	// Cursor starts at top: bottom indicator only.
	overlay := m.buildDashboardOverlay()
	assert.NotContains(t, overlay, symTriUp,
		"no top indicator when cursor at top")
	assert.Contains(t, overlay, symTriDown+" ",
		"bottom indicator when content below")

	// Navigate to bottom with G: top indicator only.
	sendKey(m, "G")
	overlay = m.buildDashboardOverlay()
	assert.Contains(t, overlay, symTriUp+" ",
		"top indicator when scrolled past top")
	assert.NotContains(t, overlay, symTriDown+" ",
		"no bottom indicator when at bottom")

	// Navigate to middle with g then j*5: both indicators.
	sendKey(m, "g")
	for range 5 {
		sendKey(m, "j")
	}
	overlay = m.buildDashboardOverlay()
	assert.Contains(t, overlay, symTriUp, "top indicator when scrolled")
	assert.Contains(t, overlay, symTriDown, "bottom indicator when more below")

	// Collapse section (no scrolling needed): no indicators.
	sendKey(m, "g") // back to header
	sendKey(m, "e") // collapse
	overlay = m.buildDashboardOverlay()
	assert.NotContains(t, overlay, "more",
		"no indicators when everything fits")
}

func TestOverlayContentWidth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		termWidth int
		want      int
	}{
		{"wide terminal caps at 72", 200, 72},
		{"normal terminal", 100, 72},
		{"narrow terminal", 60, 48},
		{"very narrow caps at 30", 30, 30},
		{"minimum clamp", 20, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.width = tt.termWidth
			assert.Equal(t, tt.want, m.overlayContentWidth())
		})
	}
}

// ---------------------------------------------------------------------------
// Proactive Insights tests
// ---------------------------------------------------------------------------

func TestInsightsWanted_DisabledByDefault(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	assert.False(t, m.insightsWanted(), "insights should be off by default")
}

func TestInsightsWanted_EnabledWithLLM(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.insightsEnabled = true
	// Still false because llmClient is nil.
	assert.False(t, m.insightsWanted(), "insights needs llmClient")
}

func TestInsightsState_InitiallyEmpty(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.insightsEnabled = true
	assert.Empty(t, m.dash.insights.items)
	assert.False(t, m.dash.insights.loading)
	assert.False(t, m.dash.insights.stale)
	assert.NoError(t, m.dash.insights.err)
}

func TestInsightsRows_Empty(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	rows := m.dashInsightsRows()
	assert.Empty(t, rows)
}

func TestInsightsRows_WithItems(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.insightsEnabled = true
	m.dash.insights.items = []insightItem{
		{
			Text:     "Water heater is 12y old",
			Tab:      "appliances",
			EntityID: 5,
			Category: insightAttention,
		},
		{
			Text:     "4 HVAC calls this year",
			Tab:      "maintenance",
			EntityID: 12,
			Category: insightPattern,
		},
	}
	rows := m.dashInsightsRows()
	// 2 category headers + 2 data rows = 4
	require.Len(t, rows, 4)
	// First: attention header.
	assert.Equal(t, "Needs Attention", rows[0].Cells[0].Text)
	assert.True(t, rows[0].Target.InfoOnly)
	// Second: data row.
	assert.Equal(t, "Water heater is 12y old", rows[1].Cells[0].Text)
	assert.Equal(t, "Appl.", rows[1].Cells[1].Text)
	assert.Equal(t, tabAppliances, rows[1].Target.Tab)
	assert.Equal(t, uint(5), rows[1].Target.ID)
	assert.False(t, rows[1].Target.InfoOnly)
	// Third: pattern header.
	assert.Equal(t, "Patterns", rows[2].Cells[0].Text)
	// Fourth: data row.
	assert.Equal(t, "4 HVAC calls this year", rows[3].Cells[0].Text)
	assert.Equal(t, tabMaintenance, rows[3].Target.Tab)
	assert.Equal(t, uint(12), rows[3].Target.ID)
	assert.False(t, rows[3].Target.InfoOnly)
}

func TestInsightsSection_AppearsAfterResultMsg(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.insightsEnabled = true
	m.width = 120
	m.height = 40

	// Open dashboard with D.
	sendKey(m, "D")
	require.True(t, m.showDashboard)

	// Simulate streamed insights arriving.
	m.dash.insights.items = []insightItem{
		{
			Text:     "Water heater is 12y old",
			Tab:      "appliances",
			EntityID: 1,
			Category: insightAttention,
		},
	}
	m.dash.insights.loading = false
	m.buildDashNav()
	m.prepareDashboardView()

	// Expand insights section (last section).
	sendKey(m, "G") // jump to bottom
	// Find and expand the insights header.
	for m.dash.cursor > 0 {
		entry := m.dash.nav[m.dash.cursor]
		if entry.IsHeader && entry.Section == dashSectionInsights {
			break
		}
		sendKey(m, "k")
	}
	sendKey(m, "e") // expand
	m.prepareDashboardView()

	overlay := m.buildDashboardOverlay()
	assert.Contains(t, overlay, dashSectionInsights)
	assert.Contains(t, overlay, "Water heater is 12y old")
}

func TestInsightsSection_LoadingShowsSpinner(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.insightsEnabled = true
	m.width = 120
	m.height = 40

	// Open dashboard.
	sendKey(m, "D")
	require.True(t, m.showDashboard)

	// Simulate loading state (insights fetch in progress).
	m.dash.insights.loading = true
	m.prepareDashboardView()

	overlay := m.buildDashboardOverlay()
	assert.Contains(t, overlay, "Insights")
}

func TestInsightsSection_ErrorShowsMutedMessage(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.insightsEnabled = true
	m.width = 120
	m.height = 40

	// Open dashboard.
	sendKey(m, "D")
	require.True(t, m.showDashboard)

	// Simulate error result arriving.
	m.Update(insightsResultMsg{Err: fmt.Errorf("network timeout")})
	m.prepareDashboardView()

	overlay := m.buildDashboardOverlay()
	assert.Contains(t, overlay, "unavailable")
	assert.Contains(t, overlay, "network timeout")
	assert.NotContains(t, overlay, "error:")
}

func TestInsightsNav_JumpsToEntityViaKeyboard(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.insightsEnabled = true
	m.width = 120
	m.height = 40

	// Open dashboard.
	sendKey(m, "D")
	require.True(t, m.showDashboard)

	// Deliver insights.
	m.dash.insights.items = []insightItem{
		{Text: "Test insight", Tab: "appliances", EntityID: 42, Category: insightAttention},
	}
	m.dash.insights.loading = false
	m.buildDashNav()
	m.prepareDashboardView()

	// Navigate to the Insights section header with J (jump between sections).
	for range 20 {
		entry := m.dash.nav[m.dash.cursor]
		if entry.IsHeader && entry.Section == dashSectionInsights {
			break
		}
		sendKey(m, "J")
	}
	require.Equal(t, dashSectionInsights, m.dash.nav[m.dash.cursor].Section)

	// Expand it.
	sendKey(m, "e")
	m.prepareDashboardView()

	// Move past category sub-header to the data row.
	sendKey(m, "j") // category sub-header (InfoOnly)
	sendKey(m, "j") // actual data row
	require.False(t, m.dash.nav[m.dash.cursor].IsHeader, "should be on data row")
	require.False(t, m.dash.nav[m.dash.cursor].InfoOnly, "should not be info-only")

	// Press enter to jump to the entity.
	sendKey(m, "enter")
	assert.False(t, m.showDashboard, "dashboard should close")
	assert.Equal(t, tabIndex(tabAppliances), m.active)
}

func TestInsightsRefreshKey_MarksStale(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.insightsEnabled = true
	m.llmClient = testDummyLLMClient(t)
	m.width = 120
	m.height = 40

	// Open dashboard.
	sendKey(m, "D")
	require.True(t, m.showDashboard)

	// Deliver insights so they're cached.
	m.dash.insights.items = []insightItem{
		{Text: "Old insight", Tab: "appliances", EntityID: 1, Category: insightAttention},
	}
	m.dash.insights.loading = false
	m.buildDashNav()
	m.prepareDashboardView()
	require.False(t, m.dash.insights.stale)

	// Press r to refresh.
	sendKey(m, "r")
	assert.True(t, m.dash.insights.stale)
}

func TestInsightsStale_AfterMutation(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.insightsEnabled = true
	m.width = 120
	m.height = 40

	// Open dashboard.
	sendKey(m, "D")
	require.True(t, m.showDashboard)

	// Deliver insights.
	m.dash.insights.items = []insightItem{
		{Text: "Old insight", Tab: "appliances", EntityID: 1, Category: insightAttention},
	}
	m.dash.insights.loading = false
	m.buildDashNav()
	require.False(t, m.dash.insights.stale)

	// Simulate a data mutation (e.g. user saved an edit).
	m.reloadAfterMutation()
	assert.True(t, m.dash.insights.stale, "mutation should mark insights stale")
}

func TestTabKindFromString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  TabKind
		ok    bool
	}{
		{"projects", tabProjects, true},
		{"quotes", tabQuotes, true},
		{"maintenance", tabMaintenance, true},
		{"incidents", tabIncidents, true},
		{"appliances", tabAppliances, true},
		{"vendors", tabVendors, true},
		{"documents", tabDocuments, true},
		{"unknown", 0, false},
	}
	for _, tt := range tests {
		got, ok := tabKindFromString(tt.input)
		assert.Equal(t, tt.ok, ok, "tabKindFromString(%q) ok", tt.input)
		if ok {
			assert.Equal(t, tt.want, got, "tabKindFromString(%q)", tt.input)
		}
	}
}

func TestTabAbbrev(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Proj.", tabAbbrev("projects"))
	assert.Equal(t, "Appl.", tabAbbrev("appliances"))
	assert.Equal(t, "Mnt.", tabAbbrev("maintenance"))
}

func TestDashboardVisible_WithInsightsOnly(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.showDashboard = true
	m.insightsEnabled = true
	// No deterministic dashboard data.
	m.dash.data = dashboardData{}
	assert.False(t, m.dashboardVisible(), "should be hidden when empty")

	// But if insights are loading, it should be visible.
	m.dash.insights.loading = true
	assert.True(t, m.dashboardVisible(), "should show when insights loading")

	// Or if insights have items.
	m.dash.insights.loading = false
	m.dash.insights.items = []insightItem{{Text: "test", Category: insightAttention}}
	assert.True(t, m.dashboardVisible(), "should show when insights have items")

	// Or if insights have an error.
	m.dash.insights.items = nil
	m.dash.insights.err = fmt.Errorf("err")
	assert.True(t, m.dashboardVisible(), "should show when insights errored")
}

func TestInsightsStaleness(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)

	// No generated time means empty string.
	assert.Empty(t, m.insightsStaleness())

	// Recent generation shows short duration.
	m.dash.insights.generatedAt = time.Now().Add(-2 * time.Minute)
	s := m.insightsStaleness()
	assert.Contains(t, s, "ago")
}

func TestParsePartialInsights(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"no array", `{"insights":`, 0},
		{"array start only", `{"insights": [`, 0},
		{
			"one complete item",
			`{"insights": [{"text": "old heater", "tab": "appliances", "entity_id": 5, "category": "attention"}`,
			1,
		},
		{
			"one complete, one partial",
			`{"insights": [{"text": "old heater", "tab": "appliances", "entity_id": 5, "category": "attention"}, {"text": "lots of`,
			1,
		},
		{
			"two complete",
			`{"insights": [{"text": "a", "tab": "appliances", "entity_id": 1, "category": "attention"}, {"text": "b", "tab": "maintenance", "entity_id": 2, "category": "pattern"}]}`,
			2,
		},
		{
			"brace in string",
			`{"insights": [{"text": "has } brace", "tab": "appliances", "entity_id": 3, "category": "stale"}]}`,
			1,
		},
		{
			"entity_id zero filtered",
			`{"insights": [{"text": "a", "tab": "appliances", "entity_id": 0, "category": "attention"}]}`,
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			items := parsePartialInsights(tt.input)
			assert.Len(t, items, tt.want)
		})
	}
}

func TestInsightsChunkMsg_ItemsAppearIncrementally(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.insightsEnabled = true
	m.llmClient = testDummyLLMClient(t)
	m.width = 120
	m.height = 40

	// Open dashboard.
	sendKey(m, "D")
	require.True(t, m.showDashboard)

	// Simulate stream started.
	m.dash.insights.loading = true
	m.dash.insights.generation = 1

	// First chunk: partial JSON with one complete item.
	m.Update(insightsChunkMsg{
		Content:    `{"insights": [{"text": "old heater", "tab": "appliances", "entity_id": 5, "category": "attention"}`,
		Generation: 1,
	})
	assert.Len(t, m.dash.insights.items, 1)
	assert.True(t, m.dash.insights.loading)

	// Second chunk: completes another item.
	m.Update(insightsChunkMsg{
		Content:    `, {"text": "roof needs check", "tab": "maintenance", "entity_id": 8, "category": "pattern"}`,
		Generation: 1,
	})
	assert.Len(t, m.dash.insights.items, 2)
	assert.True(t, m.dash.insights.loading)

	// Final chunk: stream done.
	m.Update(insightsChunkMsg{
		Content:    `]}`,
		Done:       true,
		Generation: 1,
	})
	assert.Len(t, m.dash.insights.items, 2)
	assert.False(t, m.dash.insights.loading)
	assert.False(t, m.dash.insights.generatedAt.IsZero())
}

func TestInsightsChunkMsg_StaleGenerationIgnored(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.insightsEnabled = true
	m.width = 120
	m.height = 40

	m.dash.insights.generation = 2

	// Chunk from generation 1 should be ignored.
	m.Update(insightsChunkMsg{
		Content:    `{"insights": [{"text": "stale", "tab": "appliances", "entity_id": 1}]}`,
		Done:       true,
		Generation: 1,
	})
	assert.Empty(t, m.dash.insights.items)
}
