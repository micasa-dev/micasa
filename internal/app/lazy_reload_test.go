// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReloadAfterMutationMarksOtherTabsStale(t *testing.T) {
	m := newTestModelWithDemoData(t, 42)
	m.width = 120
	m.height = 40

	// Start on the Projects tab (index 0).
	m.active = 0
	m.reloadAfterMutation()

	// Active tab (0) should NOT be stale.
	assert.False(t, m.tabs[0].Stale, "active tab should not be stale after reloadAfterMutation")

	// All other tabs should be stale.
	for i := 1; i < len(m.tabs); i++ {
		assert.Truef(
			t,
			m.tabs[i].Stale,
			"tab %d (%s) should be stale after mutation on tab 0",
			i,
			m.tabs[i].Name,
		)
	}
}

func TestNavigatingToStaleTabClearsStaleFlag(t *testing.T) {
	m := newTestModelWithDemoData(t, 42)
	m.width = 120
	m.height = 40

	// Simulate a mutation on tab 0 to mark others stale.
	m.active = 0
	m.reloadAfterMutation()

	// Navigate to the next tab.
	m.nextTab()
	require.Equal(t, 1, m.active)

	// After navigation, the new active tab should not be stale.
	assert.False(t, m.tabs[1].Stale, "tab 1 should not be stale after navigating to it")

	// But tab 2 should still be stale (we haven't visited it).
	assert.True(t, m.tabs[2].Stale, "tab 2 should still be stale")
}

func TestPrevTabClearsStaleFlag(t *testing.T) {
	m := newTestModelWithDemoData(t, 42)
	m.width = 120
	m.height = 40

	// Start on tab 2, mutate to mark others stale.
	m.active = 2
	m.reloadAfterMutation()

	// Navigate backward.
	m.prevTab()
	require.Equal(t, 1, m.active)
	assert.False(t, m.tabs[1].Stale, "tab 1 should not be stale after navigating to it via prevTab")
}

func TestReloadAllClearsAllStaleFlags(t *testing.T) {
	m := newTestModelWithDemoData(t, 42)
	m.width = 120
	m.height = 40

	// Mark tabs stale.
	for i := range m.tabs {
		m.tabs[i].Stale = true
	}

	// reloadAllTabs resets all data, and reloadIfStale clears per-tab.
	m.reloadAll()

	// After reloadAll, no tabs should be stale (they were all freshly loaded).
	for i := range m.tabs {
		assert.Falsef(
			t,
			m.tabs[i].Stale,
			"tab %d (%s) should not be stale after reloadAll",
			i,
			m.tabs[i].Name,
		)
	}
}

func TestDashJumpClearsStaleFlag(t *testing.T) {
	m := newTestModelWithDemoData(t, 42)
	m.width = 120
	m.height = 40

	// Open the dashboard and load data so we have nav entries.
	m.showDashboard = true
	require.NoError(t, m.loadDashboardAt(time.Now()))
	if m.dashNavCount() == 0 {
		t.Skip("no dashboard nav entries in demo data")
	}

	// Mark all tabs stale.
	for i := range m.tabs {
		m.tabs[i].Stale = true
	}

	// Find the first non-header nav entry to jump to.
	jumpIdx := -1
	for i, entry := range m.dash.nav {
		if !entry.IsHeader {
			jumpIdx = i
			break
		}
	}
	if jumpIdx < 0 {
		t.Skip("no data nav entries in demo data")
	}
	m.dash.cursor = jumpIdx
	targetTab := m.dash.nav[jumpIdx].Tab
	sendKey(m, "enter")

	// The target tab should be fresh after the jump.
	idx := tabIndex(targetTab)
	assert.Falsef(
		t,
		m.tabs[idx].Stale,
		"tab %d (%s) should not be stale after dashJump",
		idx,
		m.tabs[idx].Name,
	)

	// A different tab should still be stale.
	otherIdx := (idx + 1) % len(m.tabs)
	assert.Truef(
		t,
		m.tabs[otherIdx].Stale,
		"tab %d (%s) should still be stale after jumping to tab %d",
		otherIdx,
		m.tabs[otherIdx].Name,
		idx,
	)
}

func TestNavigateToLinkClearsStaleFlag(t *testing.T) {
	m := newTestModelWithDemoData(t, 42)
	m.width = 120
	m.height = 40

	// Mark all tabs stale.
	for i := range m.tabs {
		m.tabs[i].Stale = true
	}

	// Navigate to the Vendors tab via a link. The target ID doesn't need
	// to match an actual row — we just verify the tab reload happens.
	link := &columnLink{TargetTab: tabVendors}
	_ = m.navigateToLink(link, 1)

	vendorIdx := tabIndex(tabVendors)
	assert.False(t, m.tabs[vendorIdx].Stale, "vendors tab should not be stale after navigateToLink")

	// A different tab should still be stale.
	projIdx := tabIndex(tabProjects)
	assert.True(
		t,
		m.tabs[projIdx].Stale,
		"projects tab should still be stale after navigating to vendors",
	)
}

func TestCloseDetailClearsStaleParentTab(t *testing.T) {
	m := newTestModelWithDemoData(t, 42)
	m.width = 120
	m.height = 40

	// Switch to Maintenance tab and open a service log detail view.
	m.active = tabIndex(tabMaintenance)
	_ = m.reloadActiveTab()

	// We need a maintenance item ID to open the detail.
	tab := m.activeTab()
	if tab == nil || len(tab.Rows) == 0 {
		t.Skip("no maintenance rows in demo data")
	}
	itemID := tab.Rows[0].ID

	require.NoError(t, m.openServiceLogDetail(itemID, "Test Item"))
	require.NotNil(t, m.detail(), "expected detail view to be open")

	// Mark the parent (Maintenance) tab stale while in the detail view.
	maintIdx := tabIndex(tabMaintenance)
	m.tabs[maintIdx].Stale = true

	// Close the detail — should reload the stale parent tab.
	m.closeDetail()

	assert.False(t, m.tabs[maintIdx].Stale, "maintenance tab should not be stale after closeDetail")
}
