// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashMaintSplitRows(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.styles = appStyles

	lastSrv := time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)
	m.dash.data = dashboardData{
		Overdue: []maintenanceUrgency{{
			Item: data.MaintenanceItem{
				ID:             1,
				Name:           "Replace Filter",
				LastServicedAt: &lastSrv,
			},
			ApplianceName: "Furnace",
			DaysFromNow:   -14,
		}},
		Upcoming: []maintenanceUrgency{{
			Item:        data.MaintenanceItem{ID: 2, Name: "Check Pump"},
			DaysFromNow: 10,
		}},
	}

	overdueRows, upcomingRows := m.dashMaintSplitRows()
	require.Len(t, overdueRows, 1)
	require.Len(t, upcomingRows, 1)

	assert.Equal(t, "Replace Filter", overdueRows[0].Cells[0].Text)
	assert.Equal(t, "14d", overdueRows[0].Cells[1].Text)
	assert.Equal(
		t,
		m.styles.DashOverdue(),
		overdueRows[0].Cells[1].Style,
		"overdue duration uses DashOverdue style",
	)
	require.NotNil(t, overdueRows[0].Target)
	assert.Equal(t, tabMaintenance, overdueRows[0].Target.Tab)

	assert.Equal(t, "Check Pump", upcomingRows[0].Cells[0].Text)
	assert.Equal(t, "10d", upcomingRows[0].Cells[1].Text)
	assert.Equal(
		t,
		m.styles.DashUpcoming(),
		upcomingRows[0].Cells[1].Style,
		"upcoming duration uses DashUpcoming style",
	)
}

func TestDashMaintSplitRowsEmpty(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.dash.data = dashboardData{}
	overdue, upcoming := m.dashMaintSplitRows()
	assert.Nil(t, overdue)
	assert.Nil(t, upcoming)
}

func TestDashProjectRowsColumns(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.styles = appStyles

	m.dash.data = dashboardData{
		ActiveProjects: []data.Project{{
			Title:  "Deck Build",
			Status: data.ProjectStatusInProgress,
		}},
	}

	rows := m.dashProjectRows()
	require.Len(t, rows, 1)
	assert.Equal(t, "Deck Build", rows[0].Cells[0].Text)
	assert.NotEmpty(t, rows[0].Cells[1].Text, "expected status text")
	assert.NotEmpty(t, rows[0].Cells[2].Text, "expected started duration")
}

func TestDashExpiringRowsOverdueAndUpcoming(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.styles = appStyles

	expiredDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	upcomingDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	m.dash.data = dashboardData{
		ExpiringWarranties: []warrantyStatus{
			{
				Appliance:   data.Appliance{ID: 1, Name: "Fridge", WarrantyExpiry: &expiredDate},
				DaysFromNow: -20,
			},
			{
				Appliance:   data.Appliance{ID: 2, Name: "Oven", WarrantyExpiry: &upcomingDate},
				DaysFromNow: 55,
			},
		},
	}

	rows := m.dashExpiringRows()
	require.Len(t, rows, 2)
	assert.Equal(t, "Fridge warranty", rows[0].Cells[0].Text)
	assert.Equal(t, "Oven warranty", rows[1].Cells[0].Text)
	// Both should have nav targets.
	require.NotNil(t, rows[0].Target)
	assert.Equal(t, tabAppliances, rows[0].Target.Tab)
}

func TestDashExpiringRowsEmpty(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.dash.data = dashboardData{}
	rows := m.dashExpiringRows()
	assert.Nil(t, rows)
}
