// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListMaintenanceWithSchedule(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cat := MaintenanceCategory{Name: "TestCat"}
	require.NoError(t, store.db.Create(&cat).Error)

	ptrTime := func(y, m, d int) *time.Time {
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		return &t
	}
	// Item with interval > 0 should appear.
	require.NoError(t, store.db.Create(&MaintenanceItem{
		Name: "With Interval", CategoryID: cat.ID,
		IntervalMonths: 3, LastServicedAt: ptrTime(2025, 6, 1),
	}).Error)
	// Item with interval = 0 should NOT appear.
	require.NoError(t, store.db.Create(&MaintenanceItem{
		Name: "No Interval", CategoryID: cat.ID, IntervalMonths: 0,
	}).Error)

	items, err := store.ListMaintenanceWithSchedule()
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "With Interval", items[0].Name)
}

func TestListMaintenanceWithScheduleDueDate(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cat := MaintenanceCategory{Name: "DueDateCat"}
	require.NoError(t, store.db.Create(&cat).Error)

	ptrTime := func(y, m, d int) *time.Time {
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		return &t
	}
	// Item with due date (no interval) should appear.
	require.NoError(t, store.db.Create(&MaintenanceItem{
		Name: "With DueDate", CategoryID: cat.ID,
		DueDate: ptrTime(2025, 11, 1),
	}).Error)
	// Item with neither should NOT appear.
	require.NoError(t, store.db.Create(&MaintenanceItem{
		Name: "Unscheduled", CategoryID: cat.ID,
	}).Error)

	items, err := store.ListMaintenanceWithSchedule()
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "With DueDate", items[0].Name)
}

func TestListMaintenanceBySeason(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cat := MaintenanceCategory{Name: "SeasonCat"}
	require.NoError(t, store.db.Create(&cat).Error)

	require.NoError(t, store.db.Create(&MaintenanceItem{
		Name: "Spring Item", CategoryID: cat.ID, Season: SeasonSpring,
	}).Error)
	require.NoError(t, store.db.Create(&MaintenanceItem{
		Name: "Fall Item", CategoryID: cat.ID, Season: SeasonFall,
	}).Error)
	require.NoError(t, store.db.Create(&MaintenanceItem{
		Name: "No Season", CategoryID: cat.ID,
	}).Error)

	items, err := store.ListMaintenanceBySeason(SeasonSpring)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Spring Item", items[0].Name)
}

func TestListMaintenanceBySeasonExcludesDeleted(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cat := MaintenanceCategory{Name: "SeasonDelCat"}
	require.NoError(t, store.db.Create(&cat).Error)

	require.NoError(t, store.db.Create(&MaintenanceItem{
		Name: "Active Spring", CategoryID: cat.ID, Season: SeasonSpring,
	}).Error)
	item := MaintenanceItem{
		Name: "Deleted Spring", CategoryID: cat.ID, Season: SeasonSpring,
	}
	require.NoError(t, store.db.Create(&item).Error)
	require.NoError(t, store.DeleteMaintenance(item.ID))

	items, err := store.ListMaintenanceBySeason(SeasonSpring)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Active Spring", items[0].Name)
}

func TestSeasonForMonth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		month    time.Month
		expected string
	}{
		{time.January, SeasonWinter},
		{time.February, SeasonWinter},
		{time.March, SeasonSpring},
		{time.April, SeasonSpring},
		{time.May, SeasonSpring},
		{time.June, SeasonSummer},
		{time.July, SeasonSummer},
		{time.August, SeasonSummer},
		{time.September, SeasonFall},
		{time.October, SeasonFall},
		{time.November, SeasonFall},
		{time.December, SeasonWinter},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, SeasonForMonth(tt.month),
			"month %s should be %s", tt.month, tt.expected)
	}
}

func TestListActiveProjects(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	var pt ProjectType
	require.NoError(t, store.db.First(&pt).Error)
	require.NoError(
		t,
		store.db.Create(
			&Project{Title: "A", ProjectTypeID: pt.ID, Status: ProjectStatusInProgress},
		).Error,
	)
	require.NoError(
		t,
		store.db.Create(
			&Project{Title: "B", ProjectTypeID: pt.ID, Status: ProjectStatusDelayed},
		).Error,
	)
	require.NoError(
		t,
		store.db.Create(
			&Project{Title: "C", ProjectTypeID: pt.ID, Status: ProjectStatusCompleted},
		).Error,
	)
	require.NoError(
		t,
		store.db.Create(
			&Project{Title: "D", ProjectTypeID: pt.ID, Status: ProjectStatusIdeating},
		).Error,
	)

	projects, err := store.ListActiveProjects()
	require.NoError(t, err)
	require.Len(t, projects, 2)
	names := map[string]bool{}
	for _, p := range projects {
		names[p.Title] = true
	}
	assert.True(t, names["A"] && names["B"], "expected projects A and B, got %v", names)
}

func TestListOpenIncidents(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Urgent leak", Status: IncidentStatusOpen, Severity: IncidentSeverityUrgent,
	}))
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Cracked tile", Status: IncidentStatusInProgress, Severity: IncidentSeverityWhenever,
	}))
	// Resolved (soft-deleted) -- should NOT appear.
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Fixed fence", Status: IncidentStatusOpen, Severity: IncidentSeveritySoon,
	}))
	items, _ := store.ListIncidents(false)
	for _, inc := range items {
		if inc.Title == "Fixed fence" {
			require.NoError(t, store.DeleteIncident(inc.ID))
		}
	}

	incidents, err := store.ListOpenIncidents()
	require.NoError(t, err)
	require.Len(t, incidents, 2)
	// Urgent should come first (severity ordering).
	assert.Equal(t, "Urgent leak", incidents[0].Title)
	assert.Equal(t, "Cracked tile", incidents[1].Title)
}

func TestListExpiringWarranties(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)
	ptrTime := func(y, m, d int) *time.Time {
		t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
		return &t
	}
	// Expiring in 30 days -- should appear.
	require.NoError(
		t,
		store.db.Create(&Appliance{Name: "Soon", WarrantyExpiry: ptrTime(2026, 3, 10)}).Error,
	)
	// Expired 10 days ago -- should appear (within lookBack).
	require.NoError(
		t,
		store.db.Create(&Appliance{Name: "Recent", WarrantyExpiry: ptrTime(2026, 1, 29)}).Error,
	)
	// Expired 60 days ago -- should NOT appear.
	require.NoError(
		t,
		store.db.Create(&Appliance{Name: "Old", WarrantyExpiry: ptrTime(2025, 12, 1)}).Error,
	)
	// Expiring in 120 days -- should NOT appear.
	require.NoError(
		t,
		store.db.Create(&Appliance{Name: "Far", WarrantyExpiry: ptrTime(2026, 6, 8)}).Error,
	)
	// No warranty -- should NOT appear.
	require.NoError(t, store.db.Create(&Appliance{Name: "None"}).Error)

	apps, err := store.ListExpiringWarranties(now, 30*24*time.Hour, 90*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, apps, 2)
}

func TestListRecentServiceLogs(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cat := MaintenanceCategory{Name: "SLCat"}
	require.NoError(t, store.db.Create(&cat).Error)
	item := MaintenanceItem{Name: "SL Item", CategoryID: cat.ID, IntervalMonths: 6}
	require.NoError(t, store.db.Create(&item).Error)

	for i := range 10 {
		require.NoError(t, store.db.Create(&ServiceLogEntry{
			MaintenanceItemID: item.ID,
			ServicedAt:        time.Date(2025, 1+time.Month(i), 1, 0, 0, 0, 0, time.UTC),
		}).Error)
	}

	entries, err := store.ListRecentServiceLogs(5)
	require.NoError(t, err)
	require.Len(t, entries, 5)
	// Most recent should be first.
	assert.Equal(t, time.October, entries[0].ServicedAt.Month())
}

func TestYTDSpending(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ptr := func(v int64) *int64 { return &v }

	cat := MaintenanceCategory{Name: "SpendCat"}
	require.NoError(t, store.db.Create(&cat).Error)
	item := MaintenanceItem{Name: "Spend Item", CategoryID: cat.ID, IntervalMonths: 6}
	require.NoError(t, store.db.Create(&item).Error)

	// This year.
	require.NoError(t, store.db.Create(&ServiceLogEntry{
		MaintenanceItemID: item.ID,
		ServicedAt:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		CostCents:         ptr(5000),
	}).Error)
	// Last year -- should not count.
	require.NoError(t, store.db.Create(&ServiceLogEntry{
		MaintenanceItemID: item.ID,
		ServicedAt:        time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
		CostCents:         ptr(9999),
	}).Error)

	yearStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	spend, err := store.YTDServiceSpendCents(yearStart)
	require.NoError(t, err)
	assert.Equal(t, int64(5000), spend)

	// Projects — TotalProjectSpendCents sums ALL non-deleted projects
	// regardless of updated_at (the old YTD filter was incorrect).
	var pt ProjectType
	require.NoError(t, store.db.First(&pt).Error)
	require.NoError(t, store.db.Create(&Project{
		Title: "P1", ProjectTypeID: pt.ID, Status: ProjectStatusCompleted,
		ActualCents: ptr(20000),
	}).Error)
	require.NoError(t, store.db.Create(&Project{
		Title: "P2", ProjectTypeID: pt.ID, Status: ProjectStatusInProgress,
		ActualCents: ptr(10000),
	}).Error)
	// Project updated last year — still included (no date filter).
	oldProj := Project{
		Title: "P3", ProjectTypeID: pt.ID, Status: ProjectStatusCompleted,
		ActualCents: ptr(7777),
	}
	require.NoError(t, store.db.Create(&oldProj).Error)
	require.NoError(t, store.db.Exec(
		"UPDATE projects SET updated_at = ? WHERE title = ?",
		time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), "P3",
	).Error)

	projSpend, err := store.TotalProjectSpendCents()
	require.NoError(t, err)
	assert.Equal(t, int64(37777), projSpend)
}

func TestTotalProjectSpendUnaffectedByEdits(t *testing.T) {
	t.Parallel()
	// User scenario: editing a project's description (or any field) should
	// not change the spending total. The old updated_at filter caused edits
	// to inflate/deflate the YTD figure.
	store := newTestStore(t)
	ptr := func(v int64) *int64 { return &v }

	var pt ProjectType
	require.NoError(t, store.db.First(&pt).Error)
	p := Project{
		Title: "Kitchen Remodel", ProjectTypeID: pt.ID,
		Status: ProjectStatusCompleted, ActualCents: ptr(50000),
	}
	require.NoError(t, store.db.Create(&p).Error)

	// Push updated_at into the past to simulate an old project.
	require.NoError(t, store.db.Exec(
		"UPDATE projects SET updated_at = ? WHERE id = ?",
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), p.ID,
	).Error)

	spend1, err := store.TotalProjectSpendCents()
	require.NoError(t, err)
	assert.Equal(t, int64(50000), spend1)

	// Simulate user editing the description — touches updated_at.
	require.NoError(t, store.db.Model(&p).Update("description", "added new countertops").Error)

	spend2, err := store.TotalProjectSpendCents()
	require.NoError(t, err)
	assert.Equal(t, spend1, spend2, "editing a project must not change the spending total")
}
