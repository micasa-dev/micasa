// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceLogCloseMovesToLastColumn(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, err := m.store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	// Create a maintenance item.
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Cursor Test Item",
		CategoryID: cats[0].ID,
	}))

	// Navigate to Maintenance tab and reload.
	m.active = tabIndex(tabMaintenance)
	require.NoError(t, m.reloadActiveTab())

	items, err := m.store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	maintID := items[0].ID

	// Open service log detail via enter on the drilldown column.
	require.NoError(t, m.openServiceLogDetail(maintID, "Cursor Test Item"))
	require.True(t, m.inDetail())

	// Submit a service log entry to mark the detail as mutated.
	m.fs.formData = &serviceLogFormData{
		MaintenanceItemID: maintID,
		ServicedAt:        "2026-03-01",
		Notes:             "test service",
	}
	m.saveFormInPlace()
	require.NotEqual(t, statusError, m.status.Kind,
		"unexpected error: %s", m.status.Text)

	// Close detail via Esc.
	sendKey(m, keyEsc)
	assert.False(t, m.inDetail())

	// Column cursor should have moved to the "Last" column.
	tab := m.effectiveTab()
	require.NotNil(t, tab)
	assert.Equal(t, int(maintenanceColLast), tab.ColCursor,
		"column cursor should point to the Last column after closing mutated service log detail")

	// Status bar should show the sync message.
	assert.Equal(t, statusInfo, m.status.Kind)
	assert.Contains(t, m.status.Text, "synced")
}

func TestServiceLogCloseNoMoveWhenUnmutated(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, err := m.store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "No Move Item",
		CategoryID: cats[0].ID,
	}))

	m.active = tabIndex(tabMaintenance)
	require.NoError(t, m.reloadActiveTab())

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	originalCol := tab.ColCursor

	items, err := m.store.ListMaintenance(false)
	require.NoError(t, err)
	maintID := items[0].ID

	// Open and close detail without any mutations.
	require.NoError(t, m.openServiceLogDetail(maintID, "No Move Item"))
	require.True(t, m.inDetail())

	sendKey(m, keyEsc)
	assert.False(t, m.inDetail())

	// Column cursor should not have changed.
	tab = m.effectiveTab()
	require.NotNil(t, tab)
	assert.Equal(t, originalCol, tab.ColCursor,
		"column cursor should not move when detail was not mutated")

	// No status message should be set.
	assert.Empty(t, m.status.Text, "no status message when detail was not mutated")
}

func TestServiceLogCloseStatusClearedOnNextAction(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, err := m.store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Status Clear Item",
		CategoryID: cats[0].ID,
	}))

	m.active = tabIndex(tabMaintenance)
	require.NoError(t, m.reloadActiveTab())

	items, err := m.store.ListMaintenance(false)
	require.NoError(t, err)
	maintID := items[0].ID

	// Open, mutate, close.
	require.NoError(t, m.openServiceLogDetail(maintID, "Status Clear Item"))
	m.fs.formData = &serviceLogFormData{
		MaintenanceItemID: maintID,
		ServicedAt:        "2026-03-01",
		Notes:             "service entry",
	}
	m.saveFormInPlace()
	sendKey(m, keyEsc)
	require.Contains(t, m.status.Text, "synced")

	// Pressing Esc again clears the status message.
	sendKey(m, keyEsc)
	assert.Empty(t, m.status.Text, "esc should clear the info status")
}
