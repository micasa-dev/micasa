// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/fake"
	"github.com/stretchr/testify/require"
)

// benchModel returns a Model populated with demo data, sized for a
// realistic terminal. Reusable across benchmarks.
func benchModel(b *testing.B) *Model {
	b.Helper()
	path := filepath.Join(b.TempDir(), "bench.db")
	require.NoError(b, os.WriteFile(path, templateBytes, 0o600))
	store, err := data.Open(path)
	require.NoError(b, err)
	b.Cleanup(func() { _ = store.Close() })
	require.NoError(b, store.SeedDemoDataFrom(fake.New(42)))
	require.NoError(b, store.ResolveCurrency(""))
	m, err := NewModel(store, Options{DBPath: path})
	require.NoError(b, err)
	m.width = 120
	m.height = 40
	m.showDashboard = false
	require.NoError(b, m.reloadAllTabs())
	return m
}

func BenchmarkView(b *testing.B) {
	m := benchModel(b)
	b.ResetTimer()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkViewDashboard(b *testing.B) {
	m := benchModel(b)
	m.showDashboard = true
	require.NoError(b, m.loadDashboardAt(time.Now()))
	b.ResetTimer()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkReloadAll(b *testing.B) {
	m := benchModel(b)
	b.ResetTimer()
	for b.Loop() {
		m.reloadAll()
	}
}

func BenchmarkReloadActiveTab(b *testing.B) {
	m := benchModel(b)
	b.ResetTimer()
	for b.Loop() {
		_ = m.reloadActiveTab()
	}
}

func BenchmarkReloadAfterMutation(b *testing.B) {
	m := benchModel(b)
	b.ResetTimer()
	for b.Loop() {
		m.reloadAfterMutation()
	}
}

func BenchmarkReloadAfterMutationWithDashboard(b *testing.B) {
	m := benchModel(b)
	m.showDashboard = true
	require.NoError(b, m.loadDashboardAt(time.Now()))
	b.ResetTimer()
	for b.Loop() {
		m.reloadAfterMutation()
	}
}

func BenchmarkLoadDashboard(b *testing.B) {
	m := benchModel(b)
	now := time.Now()
	b.ResetTimer()
	for b.Loop() {
		_ = m.loadDashboardAt(now)
	}
}

func BenchmarkColumnWidths(b *testing.B) {
	m := benchModel(b)
	tab := m.activeTab()
	visSpecs, visCells, _, _, _ := visibleProjection(
		tab,
	)
	sepW := 3
	b.ResetTimer()
	for b.Loop() {
		_ = columnWidths(visSpecs, visCells, 120, sepW, nil)
	}
}

func BenchmarkNaturalWidths(b *testing.B) {
	m := benchModel(b)
	tab := m.activeTab()
	visSpecs, visCells, _, _, _ := visibleProjection(
		tab,
	)
	b.ResetTimer()
	for b.Loop() {
		_ = naturalWidths(visSpecs, visCells, "$")
	}
}

func BenchmarkVisibleProjection(b *testing.B) {
	m := benchModel(b)
	tab := m.activeTab()
	b.ResetTimer()
	for b.Loop() {
		_, _, _, _, _ = visibleProjection(tab)
	}
}

func BenchmarkComputeTableViewport(b *testing.B) {
	m := benchModel(b)
	tab := m.activeTab()
	sep := m.styles.TableSeparator().Render(" │ ")
	b.ResetTimer()
	for b.Loop() {
		_ = computeTableViewport(tab, 120, sep, "$")
	}
}

func BenchmarkComputeTableViewportPins(b *testing.B) {
	m := benchModel(b)
	tab := m.activeTab()
	require.NotEmpty(b, tab.CellRows, "need data rows")
	// Pin the first cell value in the first column.
	pinVal := tab.CellRows[0][0].Value
	tab.Pins = []filterPin{{Col: 0, Values: map[string]bool{pinVal: true}}}
	sep := m.styles.TableSeparator().Render(" │ ")
	b.ResetTimer()
	for b.Loop() {
		_ = computeTableViewport(tab, 120, sep, "$")
	}
}

func BenchmarkTableView(b *testing.B) {
	m := benchModel(b)
	tab := m.activeTab()
	b.ResetTimer()
	for b.Loop() {
		_ = m.tableView(tab)
	}
}

func BenchmarkTableViewPins(b *testing.B) {
	m := benchModel(b)
	tab := m.activeTab()
	require.NotEmpty(b, tab.CellRows, "need data rows")
	pinVal := tab.CellRows[0][0].Value
	tab.Pins = []filterPin{{Col: 0, Values: map[string]bool{pinVal: true}}}
	b.ResetTimer()
	for b.Loop() {
		_ = m.tableView(tab)
	}
}

func BenchmarkSelectClickedColumn(b *testing.B) {
	m := benchModel(b)
	tab := m.activeTab()
	require.NotEmpty(b, tab.CellRows, "need data rows")
	// Pre-populate the cache as View() would.
	_ = m.View()
	require.NotNil(b, tab.cachedVP)
	cached := tab.cachedVP
	msg := tea.MouseClickMsg{X: 20, Y: 5, Button: tea.MouseLeft}
	origCursor := tab.ColCursor
	origOffset := tab.ViewOffset
	b.ResetTimer()
	for b.Loop() {
		// Restore state so updateTabViewport's invalidation doesn't cascade.
		tab.ColCursor = origCursor
		tab.ViewOffset = origOffset
		tab.cachedVP = cached
		m.selectClickedColumn(tab, msg)
	}
}

func BenchmarkDashboardView(b *testing.B) {
	m := benchModel(b)
	m.showDashboard = true
	require.NoError(b, m.loadDashboardAt(time.Now()))
	b.ResetTimer()
	for b.Loop() {
		m.prepareDashboardView()
		_ = m.dashboardView(30, 68)
	}
}

func BenchmarkBuildBaseView(b *testing.B) {
	m := benchModel(b)
	b.ResetTimer()
	for b.Loop() {
		_ = m.buildBaseView()
	}
}

func BenchmarkApplySorts(b *testing.B) {
	m := benchModel(b)
	tab := m.activeTab()
	// Sort by a date column to exercise date parsing.
	dateCol := -1
	for i, spec := range tab.Specs {
		if spec.Kind == cellDate || spec.Kind == cellUrgency {
			dateCol = i
			break
		}
	}
	if dateCol < 0 {
		b.Skip("no date column in active tab")
	}
	tab.Sorts = []sortEntry{{Col: dateCol, Dir: sortAsc}}
	b.ResetTimer()
	for b.Loop() {
		applySorts(tab)
	}
}

func BenchmarkMouseMotion(b *testing.B) {
	m := benchModel(b)
	_ = m.View()
	_ = m.View()
	var buf bytes.Buffer
	m.pointerWriter = &buf
	msg := tea.MouseMotionMsg{X: 20, Y: 5}
	b.ResetTimer()
	for b.Loop() {
		buf.Reset()
		m.handleMouseMotion(msg)
	}
}

func BenchmarkDimBackground(b *testing.B) {
	m := benchModel(b)
	base := m.buildBaseView()
	b.ResetTimer()
	for b.Loop() {
		_ = dimBackground(base)
	}
}
