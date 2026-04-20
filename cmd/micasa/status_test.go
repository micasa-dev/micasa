// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- exitError ---

func TestExitErrorCode(t *testing.T) {
	t.Parallel()
	err := exitError{code: 2}
	assert.Equal(t, 2, err.code)
	assert.Empty(t, err.Error())
}

func TestExitErrorUnwrap(t *testing.T) {
	t.Parallel()
	err := exitError{code: 2}
	var target exitError
	require.ErrorAs(t, err, &target)
	assert.Equal(t, 2, target.code)
}

func TestExtractExitCodeExitError(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 2, extractExitCode(exitError{code: 2}))
}

func TestExtractExitCodeRegularError(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1, extractExitCode(errors.New("boom")))
}

// --- text output ---

func TestStatusTextEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	err := runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)
	assert.Empty(t, ansi.Strip(buf.String()))
}

func TestStatusTextOverdue(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	pastDue := now.AddDate(0, 0, -10)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Replace filter",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	out := ansi.Strip(buf.String())
	assert.Contains(t, out, "OVERDUE")
	assert.Contains(t, out, "Replace filter")
	assert.Contains(t, out, "10d")
}

func TestStatusTextUpcoming(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	futureDue := now.AddDate(0, 0, 15)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Inspect roof",
		CategoryID: cats[0].ID,
		DueDate:    &futureDue,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)

	out := ansi.Strip(buf.String())
	assert.Contains(t, out, "UPCOMING")
	assert.Contains(t, out, "Inspect roof")
	assert.Contains(t, out, "15d")
}

func TestStatusTextUpcomingDoesNotTriggerExit2(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	futureDue := now.AddDate(0, 0, 5)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Clean gutters",
		CategoryID: cats[0].ID,
		DueDate:    &futureDue,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)
}

func TestStatusTextIncidents(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.CreateIncident(&data.Incident{
		Title:    "Leaking faucet",
		Status:   data.IncidentStatusOpen,
		Severity: data.IncidentSeverityUrgent,
	}))

	var buf bytes.Buffer
	err := runStatus(&buf, &statusOpts{days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	out := ansi.Strip(buf.String())
	assert.Contains(t, out, "INCIDENTS")
	assert.Contains(t, out, "Leaking faucet")
	assert.Contains(t, out, "urgent")
}

func TestStatusTextIncidentsWhenever(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.CreateIncident(&data.Incident{
		Title:    "Loose trim",
		Status:   data.IncidentStatusOpen,
		Severity: data.IncidentSeverityWhenever,
	}))

	var buf bytes.Buffer
	err := runStatus(&buf, &statusOpts{days: 30, isDark: true}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	out := buf.String()
	assert.Contains(t, out, "whenever",
		"output should contain the severity string")
	assert.Contains(t, out, "\x1b[",
		"styled output should contain ANSI escape sequences")
}

func TestStatusTextContainsANSI(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	pastDue := now.AddDate(0, 0, -5)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "ANSI test item",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30, isDark: true}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)

	assert.Contains(t, buf.String(), "\x1b[",
		"output should contain ANSI escape sequences")
}

func TestStatusTextActiveProjects(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Kitchen remodel",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusDelayed,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	out := ansi.Strip(buf.String())
	assert.Contains(t, out, "ACTIVE PROJECTS")
	assert.Contains(t, out, "Kitchen remodel")
	assert.Contains(t, out, "delayed")
}

func TestStatusUnderwayProjectDoesNotTriggerExit2(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Fence repair",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusInProgress,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)

	out := ansi.Strip(buf.String())
	assert.Contains(t, out, "ACTIVE PROJECTS")
	assert.Contains(t, out, "Fence repair")
	assert.Contains(t, out, data.ProjectStatusInProgress)
}

func TestStatusDaysFlag(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	futureDue := now.AddDate(0, 0, 20)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Service heater",
		CategoryID: cats[0].ID,
		DueDate:    &futureDue,
	}))

	// With 10-day window: item at 20 days out should NOT appear
	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 10}, store, now)
	require.NoError(t, err)
	assert.NotContains(t, ansi.Strip(buf.String()), "Service heater")

	// With 30-day window: item at 20 days out SHOULD appear
	buf.Reset()
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)
	assert.Contains(t, ansi.Strip(buf.String()), "Service heater")
}

func TestStatusOverdueSortOrder(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	due3 := now.AddDate(0, 0, -3)
	due15 := now.AddDate(0, 0, -15)

	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Minor task",
		CategoryID: cats[0].ID,
		DueDate:    &due3,
	}))
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Major task",
		CategoryID: cats[0].ID,
		DueDate:    &due15,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)

	out := ansi.Strip(buf.String())
	majorIdx := strings.Index(out, "Major task")
	minorIdx := strings.Index(out, "Minor task")
	assert.Greater(t, minorIdx, majorIdx,
		"most-overdue item should appear first")
}

// --- JSON output ---

func TestStatusJSONEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	err := runStatus(&buf, &statusOpts{asJSON: true, days: 30}, store, now)
	require.NoError(t, err)

	var result statusJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Empty(t, result.Overdue)
	assert.Empty(t, result.Upcoming)
	assert.Empty(t, result.Incidents)
	assert.Empty(t, result.ActiveProjects)
	assert.False(t, result.NeedsAttention)
}

func TestStatusJSONOverdue(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	pastDue := now.AddDate(0, 0, -5)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Change filter",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{asJSON: true, days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	var result statusJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result.Overdue, 1)
	assert.Equal(t, "Change filter", result.Overdue[0].Name)
	assert.Equal(t, 5, result.Overdue[0].DaysOverdue)
	assert.True(t, result.NeedsAttention)
}

func TestStatusJSONDueTodayIncludesZero(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	dueToday := now
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Due today item",
		CategoryID: cats[0].ID,
		DueDate:    &dueToday,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{asJSON: true, days: 30}, store, now)
	require.NoError(t, err)

	var result statusJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result.Upcoming, 1)
	assert.Equal(t, 0, result.Upcoming[0].DaysUntilDue)
}

func TestStatusJSONNeedsAttentionFalseOnCleanDB(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	err := runStatus(&buf, &statusOpts{asJSON: true, days: 30}, store, now)
	require.NoError(t, err)

	var result statusJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.False(t, result.NeedsAttention)
}

// --- validation ---

func TestStatusValidateDays(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		days int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too large", 366},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := &statusOpts{days: tt.days}
			err := opts.validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--days")
		})
	}
}

func TestStatusValidateDaysBoundaries(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (&statusOpts{days: 1}).validate())
	assert.NoError(t, (&statusOpts{days: 365}).validate())
}

// --- CLI integration ---

func TestStatusCLITextClean(t *testing.T) {
	t.Parallel()
	src := createTestDB(t)
	out, err := executeCLI("status", src)
	require.NoError(t, err)
	assert.Empty(t, ansi.Strip(out))
	assert.NotContains(t, out, "\x1b[", "non-TTY output must not contain ANSI escapes")
}

func TestStatusCLITextOverdue(t *testing.T) {
	t.Parallel()
	src := createTestDB(t)
	store, err := data.Open(src)
	require.NoError(t, err)
	cats, catErr := store.MaintenanceCategories()
	require.NoError(t, catErr)

	pastDue := time.Now().AddDate(0, 0, -7)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "CLI overdue item",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))
	require.NoError(t, store.Close())

	out, err := executeCLI("status", src)
	require.Error(t, err)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)
	stripped := ansi.Strip(out)
	assert.Contains(t, stripped, "=== OVERDUE ===")
	assert.Contains(t, stripped, "CLI overdue item")
	assert.NotContains(t, out, "\x1b[", "non-TTY output must not contain ANSI escapes")
}

func TestStatusCLINoStyleWhenNotTerminal(t *testing.T) {
	t.Parallel()
	src := createTestDB(t)
	store, err := data.Open(src)
	require.NoError(t, err)
	cats, catErr := store.MaintenanceCategories()
	require.NoError(t, catErr)

	pastDue := time.Now().AddDate(0, 0, -3)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "No-style item",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))
	require.NoError(t, store.Close())

	// executeCLI captures output to a bytes.Buffer (not a TTY), so
	// term.IsTerminal(os.Stdout.Fd()) returns false -> plain output.
	out, err := executeCLI("status", src)
	require.Error(t, err)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)
	assert.Contains(t, out, "=== OVERDUE ===", "non-TTY path must use plain section headers")
	assert.NotContains(t, out, "\x1b[", "non-TTY output must not contain ANSI escapes")
}

func TestStatusCLIJSONClean(t *testing.T) {
	t.Parallel()
	src := createTestDB(t)
	out, err := executeCLI("status", "--json", src)
	require.NoError(t, err)

	var result statusJSON
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.False(t, result.NeedsAttention)
}

func TestStatusCLIDaysValidation(t *testing.T) {
	t.Parallel()
	src := createTestDB(t)

	_, err := executeCLI("status", "--days", "0", src)
	require.ErrorContains(t, err, "--days")

	_, err = executeCLI("status", "--days", "-1", src)
	require.Error(t, err)

	_, err = executeCLI("status", "--days", "366", src)
	require.Error(t, err)
}

func TestStatusTextMultipleSections(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	types, typErr := store.ProjectTypes()
	require.NoError(t, typErr)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	pastDue := now.AddDate(0, 0, -3)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Overdue task",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))
	futureDue := now.AddDate(0, 0, 10)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Upcoming task",
		CategoryID: cats[0].ID,
		DueDate:    &futureDue,
	}))
	require.NoError(t, store.CreateIncident(&data.Incident{
		Title:    "Broken pipe",
		Status:   data.IncidentStatusOpen,
		Severity: data.IncidentSeveritySoon,
	}))
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Deck build",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusInProgress,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	out := ansi.Strip(buf.String())
	assert.Contains(t, out, "OVERDUE")
	assert.Contains(t, out, "UPCOMING")
	assert.Contains(t, out, "INCIDENTS")
	assert.Contains(t, out, "ACTIVE PROJECTS")
	assert.Contains(t, out, "Overdue task")
	assert.Contains(t, out, "Upcoming task")
	assert.Contains(t, out, "Broken pipe")
	assert.Contains(t, out, "Deck build")
}

func TestStatusJSONMultipleSections(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	types, typErr := store.ProjectTypes()
	require.NoError(t, typErr)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	pastDue := now.AddDate(0, 0, -5)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Overdue JSON",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))
	futureDue := now.AddDate(0, 0, 7)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Upcoming JSON",
		CategoryID: cats[0].ID,
		DueDate:    &futureDue,
	}))
	require.NoError(t, store.CreateIncident(&data.Incident{
		Title:    "Leak",
		Status:   data.IncidentStatusInProgress,
		Severity: data.IncidentSeverityUrgent,
	}))
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Garage",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusDelayed,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{asJSON: true, days: 30}, store, now)
	var ee exitError
	require.ErrorAs(t, err, &ee)
	assert.Equal(t, 2, ee.code)

	var result statusJSON
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Len(t, result.Overdue, 1)
	assert.Len(t, result.Upcoming, 1)
	assert.Len(t, result.Incidents, 1)
	assert.Len(t, result.ActiveProjects, 1)
	assert.True(t, result.NeedsAttention)
	assert.Equal(t, "Overdue JSON", result.Overdue[0].Name)
	assert.Equal(t, "Upcoming JSON", result.Upcoming[0].Name)
	assert.Equal(t, "Leak", result.Incidents[0].Title)
	assert.Equal(t, "Garage", result.ActiveProjects[0].Title)
}

func TestStatusTextProjectWithStartDate(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	started := now.AddDate(0, 0, -14)
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Patio work",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusInProgress,
		StartDate:     &started,
	}))

	var buf bytes.Buffer
	err = runStatus(&buf, &statusOpts{days: 30}, store, now)
	require.NoError(t, err)

	out := ansi.Strip(buf.String())
	assert.Contains(t, out, "Patio work")
	assert.Contains(t, out, "14d")
}

// failWriter always returns an error on Write.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestStatusTextWriteError(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	pastDue := now.AddDate(0, 0, -5)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Fail item",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))

	err = runStatus(failWriter{}, &statusOpts{days: 30}, store, now)
	require.Error(t, err)
	assert.NotErrorIs(t, err, exitError{code: 2})
}

func TestStatusJSONWriteError(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	pastDue := now.AddDate(0, 0, -5)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Fail JSON item",
		CategoryID: cats[0].ID,
		DueDate:    &pastDue,
	}))

	err = runStatus(failWriter{}, &statusOpts{asJSON: true, days: 30}, store, now)
	require.Error(t, err)
	assert.NotErrorIs(t, err, exitError{code: 2})
}

func TestStatusCLIMissingDB(t *testing.T) {
	t.Parallel()
	_, err := executeCLI("status", "/nonexistent/path.db")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
}
