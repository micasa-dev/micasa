// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDashboardAtClassifiesOverdueAndUpcoming(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	app := data.Appliance{Name: "Furnace"}
	require.NoError(t, m.store.CreateAppliance(&app))
	apps, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	appID := apps[0].ID

	cats, err := m.store.MaintenanceCategories()
	require.NoError(t, err)

	// Item serviced 4 months ago, interval 3 months -> 1 month overdue.
	fourMonthsAgo := time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)
	overdue := data.MaintenanceItem{
		Name:           "Replace Filter",
		CategoryID:     cats[0].ID,
		ApplianceID:    &appID,
		LastServicedAt: &fourMonthsAgo,
		IntervalMonths: 3,
	}
	require.NoError(t, m.store.CreateMaintenance(&overdue))

	// Item serviced 1 month ago, interval 3 months -> due in ~2 months (upcoming).
	oneMonthAgo := time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC)
	upcoming := data.MaintenanceItem{
		Name:           "Clean Coils",
		CategoryID:     cats[0].ID,
		LastServicedAt: &oneMonthAgo,
		IntervalMonths: 3,
	}
	require.NoError(t, m.store.CreateMaintenance(&upcoming))

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	require.Len(t, m.dash.data.Overdue, 1)
	assert.Equal(t, "Replace Filter", m.dash.data.Overdue[0].Item.Name)
	assert.Equal(t, "Furnace", m.dash.data.Overdue[0].ApplianceName)
	assert.Negative(t, m.dash.data.Overdue[0].DaysFromNow)

	// "Clean Coils" is due in ~2 months — not within 30 days, so not upcoming.
	assert.Empty(t, m.dash.data.Upcoming)
}

func TestLoadDashboardAtUpcomingWithin30Days(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	// Serviced 2.5 months ago with 3-month interval -> due in ~2 weeks.
	lastSrv := time.Date(2025, 11, 15, 0, 0, 0, 0, time.UTC)
	item := data.MaintenanceItem{
		Name:           "Check Sump Pump",
		CategoryID:     cats[0].ID,
		LastServicedAt: &lastSrv,
		IntervalMonths: 3,
	}
	require.NoError(t, m.store.CreateMaintenance(&item))

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	require.Len(t, m.dash.data.Upcoming, 1)
	assert.GreaterOrEqual(t, m.dash.data.Upcoming[0].DaysFromNow, 0)
	assert.LessOrEqual(t, m.dash.data.Upcoming[0].DaysFromNow, 30)
}

func TestLoadDashboardAtActiveProjects(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	types, _ := m.store.ProjectTypes()

	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusInProgress,
	}))
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Done Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusCompleted,
	}))

	now := time.Now()
	require.NoError(t, m.loadDashboardAt(now))

	// Only in-progress projects should appear.
	require.Len(t, m.dash.data.ActiveProjects, 1)
	assert.Equal(t, "Kitchen Remodel", m.dash.data.ActiveProjects[0].Title)
}

func TestLoadDashboardAtExpiringWarranties(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	expiry := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{
		Name:           "Dishwasher",
		WarrantyExpiry: &expiry,
	}))

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	require.Len(t, m.dash.data.ExpiringWarranties, 1)
	assert.Equal(t, "Dishwasher", m.dash.data.ExpiringWarranties[0].Appliance.Name)
}

func TestLoadDashboardAtInsuranceRenewal(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	renewal := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	m.house.InsuranceCarrier = "State Farm"
	m.house.InsuranceRenewal = &renewal
	m.hasHouse = true

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	require.NotNil(t, m.dash.data.InsuranceRenewal)
	assert.Equal(t, "State Farm", m.dash.data.InsuranceRenewal.Carrier)
	assert.Equal(t, 28, m.dash.data.InsuranceRenewal.DaysFromNow)
}

func TestLoadDashboardAtInsuranceRenewalOutOfRange(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Renewal 6 months away — outside the -30..+90 window.
	renewal := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	m.house.InsuranceCarrier = "Allstate"
	m.house.InsuranceRenewal = &renewal
	m.hasHouse = true

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	assert.Nil(t, m.dash.data.InsuranceRenewal)
}

func TestLoadDashboardAtBuildsNav(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	// Create an overdue item so nav has at least one entry.
	fourMonthsAgo := time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:           "Check Gutters",
		CategoryID:     cats[0].ID,
		LastServicedAt: &fourMonthsAgo,
		IntervalMonths: 3,
	}))

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	assert.NotEmpty(t, m.dash.nav)
	// First entry is the section header for Overdue.
	assert.True(t, m.dash.nav[0].IsHeader)
	assert.Equal(t, dashSectionOverdue, m.dash.nav[0].Section)
}

func TestLoadDashboardAtOpenIncidents(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	require.NoError(t, m.store.CreateIncident(&data.Incident{
		Title:    "Burst pipe",
		Status:   data.IncidentStatusOpen,
		Severity: data.IncidentSeverityUrgent,
	}))
	require.NoError(t, m.store.CreateIncident(&data.Incident{
		Title:    "Cracked window",
		Status:   data.IncidentStatusInProgress,
		Severity: data.IncidentSeverityWhenever,
	}))
	// Resolved (soft-deleted) should NOT appear.
	require.NoError(t, m.store.CreateIncident(&data.Incident{
		Title:    "Fixed gutter",
		Status:   data.IncidentStatusOpen,
		Severity: data.IncidentSeveritySoon,
	}))
	items, _ := m.store.ListIncidents(false)
	for _, inc := range items {
		if inc.Title == "Fixed gutter" {
			require.NoError(t, m.store.DeleteIncident(inc.ID))
		}
	}

	now := time.Now()
	require.NoError(t, m.loadDashboardAt(now))

	require.Len(t, m.dash.data.OpenIncidents, 2)
	// Urgent should come first (severity ordering).
	assert.Equal(t, "Burst pipe", m.dash.data.OpenIncidents[0].Title)
	assert.Equal(t, "Cracked window", m.dash.data.OpenIncidents[1].Title)

	// Nav should include incident entries.
	hasIncidentNav := false
	for _, entry := range m.dash.nav {
		if entry.Tab == tabIncidents {
			hasIncidentNav = true
			break
		}
	}
	assert.True(t, hasIncidentNav, "dashboard nav should include incident entries")
}

func TestLoadDashboardAtDueDateOverdue(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	// Due date in the past -> overdue.
	pastDue := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Inspect Roof",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	require.Len(t, m.dash.data.Overdue, 1)
	assert.Equal(t, "Inspect Roof", m.dash.data.Overdue[0].Item.Name)
	assert.Negative(t, m.dash.data.Overdue[0].DaysFromNow)
}

func TestLoadDashboardAtDueDateUpcoming(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	// Due date 10 days in the future -> upcoming.
	soonDue := time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Replace Batteries",
		CategoryID: cats[0].ID,
		DueDate:    &soonDue,
	}))

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	require.Len(t, m.dash.data.Upcoming, 1)
	assert.Equal(t, "Replace Batteries", m.dash.data.Upcoming[0].Item.Name)
	assert.Equal(t, 10, m.dash.data.Upcoming[0].DaysFromNow)
}

func TestLoadDashboardAtDueDateFarFuture(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	// Due date 6 months away -> neither overdue nor upcoming.
	farDue := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Annual Furnace",
		CategoryID: cats[0].ID,
		DueDate:    &farDue,
	}))

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	assert.Empty(t, m.dash.data.Overdue)
	assert.Empty(t, m.dash.data.Upcoming)
}

// Step 11: Unscheduled items (no interval, no due date) never appear on dashboard.
func TestLoadDashboardAtExcludesUnscheduledItems(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Unscheduled Task",
		CategoryID: cats[0].ID,
	}))

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	assert.Empty(t, m.dash.data.Overdue)
	assert.Empty(t, m.dash.data.Upcoming)
}

func TestLoadDashboardExcludesAppliancesWithoutWarranty(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// One appliance with warranty in range, one without any warranty.
	expiry := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{
		Name:           "Fridge",
		WarrantyExpiry: &expiry,
	}))
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{
		Name: "Toaster",
	}))

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.loadDashboardAt(now))

	require.Len(t, m.dash.data.ExpiringWarranties, 1)
	assert.Equal(t, "Fridge", m.dash.data.ExpiringWarranties[0].Appliance.Name)
}

func TestLoadDashboardAtOverdueCapDoesNotHideUpcoming(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	// Create 12 overdue items (past due dates).
	for i := range 12 {
		pastDue := now.AddDate(0, 0, -(i + 1))
		require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
			Name:       fmt.Sprintf("Overdue %d", i),
			CategoryID: cats[0].ID,
			DueDate:    &pastDue,
		}))
	}

	// Create 5 upcoming items (due within 30 days).
	for i := range 5 {
		soonDue := now.AddDate(0, 0, i+1)
		require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
			Name:       fmt.Sprintf("Upcoming %d", i),
			CategoryID: cats[0].ID,
			DueDate:    &soonDue,
		}))
	}

	require.NoError(t, m.loadDashboardAt(now))

	// Overdue should be capped at 10.
	assert.Len(t, m.dash.data.Overdue, 10)
	// Upcoming must NOT be empty — a full overdue list should not hide upcoming.
	assert.Len(t, m.dash.data.Upcoming, 5)
}
