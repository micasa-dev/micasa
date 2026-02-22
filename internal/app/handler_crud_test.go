// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// projectHandler CRUD
// ---------------------------------------------------------------------------

func TestProjectHandlerLoadDeleteRestoreRoundTrip(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}

	// Create a project via form data.
	m.formData = &projectFormData{
		Title:         "Deck Build",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))

	// Load should return the project.
	rows, meta, cells, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, meta, 1)
	require.Len(t, cells, 1)
	id := meta[0].ID

	// Delete.
	require.NoError(t, h.Delete(m.store, id))

	// Load without deleted should be empty.
	rows, _, _, err = h.Load(m.store, false)
	require.NoError(t, err)
	assert.Empty(t, rows)

	// Load with deleted should show it.
	rows, _, _, err = h.Load(m.store, true)
	require.NoError(t, err)
	assert.Len(t, rows, 1)

	// Restore.
	require.NoError(t, h.Restore(m.store, id))
	rows, _, _, _ = h.Load(m.store, false)
	assert.Len(t, rows, 1)
}

func TestProjectHandlerEditRoundTrip(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}

	// Create.
	m.formData = &projectFormData{
		Title:         "Paint Fence",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusIdeating,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	// Edit via form data.
	editID := id
	m.editID = &editID
	m.formData = &projectFormData{
		Title:         "Paint Fence Red",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusInProgress,
		Budget:        "500.00",
	}
	require.NoError(t, h.SubmitForm(m))
	m.editID = nil

	project, err := m.store.GetProject(id)
	require.NoError(t, err)
	assert.Equal(t, "Paint Fence Red", project.Title)
	assert.Equal(t, data.ProjectStatusInProgress, project.Status)
	require.NotNil(t, project.BudgetCents)
	assert.Equal(t, int64(50000), *project.BudgetCents)
}

func TestProjectHandlerSnapshot(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}

	m.formData = &projectFormData{
		Title:         "Roof Repair",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	entry, ok := h.Snapshot(m.store, id)
	require.True(t, ok)
	assert.Equal(t, formProject, entry.FormKind)
	assert.Equal(t, id, entry.EntityID)
}

func TestProjectTabStatusFiltersRows(t *testing.T) {
	m := newTestModelWithStore(t)
	types, _ := m.store.ProjectTypes()

	for _, p := range []data.Project{
		{
			Title:         "Kitchen Plan",
			ProjectTypeID: types[0].ID,
			Status:        data.ProjectStatusPlanned,
		},
		{
			Title:         "Fence Done",
			ProjectTypeID: types[0].ID,
			Status:        data.ProjectStatusCompleted,
		},
		{
			Title:         "Basement Work",
			ProjectTypeID: types[0].ID,
			Status:        data.ProjectStatusInProgress,
		},
		{
			Title:         "Old Patio Idea",
			ProjectTypeID: types[0].ID,
			Status:        data.ProjectStatusAbandoned,
		},
	} {
		require.NoError(t, m.store.CreateProject(&p), "CreateProject(%q)", p.Title)
	}

	m.active = tabIndex(tabProjects)
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	require.NotNil(t, tab, "expected active projects tab")
	require.Len(t, tab.Rows, 4, "rows before filtering")

	col := statusColumnIndex(tab.Specs)
	require.GreaterOrEqual(t, col, 0, "expected Status column")

	// Pin only "planned" → filter shows only planned rows.
	togglePin(tab, col, data.ProjectStatusPlanned)
	tab.FilterActive = true
	applyRowFilter(tab, false, locale.DefaultCurrency().Symbol())
	assert.Len(t, tab.Rows, 1, "rows with only planned pinned")

	// Clear and pin active statuses (what 't' does) → hides settled.
	clearPinsForColumn(tab, col)
	for _, s := range activeProjectStatuses {
		togglePin(tab, col, s)
	}
	tab.FilterActive = true
	applyRowFilter(tab, false, locale.DefaultCurrency().Symbol())
	assert.Len(t, tab.Rows, 2, "rows with settled hidden")
	for i, cells := range tab.CellRows {
		if len(cells) > col {
			status := cells[col].Value
			assert.NotEqual(t, data.ProjectStatusCompleted, status, "row %d", i)
			assert.NotEqual(t, data.ProjectStatusAbandoned, status, "row %d", i)
		}
	}

	// Clear all pins → shows everything.
	clearPins(tab)
	applyRowFilter(tab, false, locale.DefaultCurrency().Symbol())
	assert.Len(t, tab.Rows, 4, "rows after clearing all pins")
}

func TestProjectStatusFilterToggleKeysReloadRows(t *testing.T) {
	m := newTestModelWithStore(t)
	types, _ := m.store.ProjectTypes()

	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Done Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusCompleted,
	}), "CreateProject completed")
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Live Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusInProgress,
	}), "CreateProject in-progress")
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Abandoned Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusAbandoned,
	}), "CreateProject abandoned")

	m.active = tabIndex(tabProjects)
	require.NoError(t, m.reloadActiveTab())
	require.Len(t, m.activeTab().Rows, 3, "rows before toggles")

	sendKey(m, "t")
	assert.Len(t, m.activeTab().Rows, 1, "rows after hiding settled")
	assert.True(t, m.activeTab().FilterActive, "filter should be active after t")

	sendKey(m, "t")
	assert.Len(t, m.activeTab().Rows, 3, "rows after showing settled")
	assert.False(t, m.activeTab().FilterActive, "filter should be inactive after second t")
}

// ---------------------------------------------------------------------------
// applianceHandler CRUD
// ---------------------------------------------------------------------------

func TestApplianceHandlerLoadDeleteRestoreRoundTrip(t *testing.T) {
	m := newTestModelWithStore(t)
	h := applianceHandler{}

	m.formData = &applianceFormData{Name: "Washer"}
	require.NoError(t, h.SubmitForm(m))

	rows, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	id := meta[0].ID

	require.NoError(t, h.Delete(m.store, id))
	rows, _, _, _ = h.Load(m.store, false)
	assert.Empty(t, rows)

	require.NoError(t, h.Restore(m.store, id))
	rows, _, _, _ = h.Load(m.store, false)
	assert.Len(t, rows, 1)
}

func TestApplianceHandlerEditRoundTrip(t *testing.T) {
	m := newTestModelWithStore(t)
	h := applianceHandler{}

	m.formData = &applianceFormData{Name: "Dryer"}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	editID := id
	m.editID = &editID
	m.formData = &applianceFormData{
		Name:  "Dryer",
		Brand: "LG",
		Cost:  "800.00",
	}
	require.NoError(t, h.SubmitForm(m))
	m.editID = nil

	app, _ := m.store.GetAppliance(id)
	assert.Equal(t, "LG", app.Brand)
	require.NotNil(t, app.CostCents)
	assert.Equal(t, int64(80000), *app.CostCents)
}

// ---------------------------------------------------------------------------
// maintenanceHandler CRUD
// ---------------------------------------------------------------------------

func TestMaintenanceHandlerLoadDeleteRestoreRoundTrip(t *testing.T) {
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	cats, _ := m.store.MaintenanceCategories()

	m.formData = &maintenanceFormData{
		Name:           "Change Air Filter",
		CategoryID:     cats[0].ID,
		IntervalMonths: "3",
	}
	require.NoError(t, h.SubmitForm(m))

	rows, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	id := meta[0].ID

	require.NoError(t, h.Delete(m.store, id))
	rows, _, _, _ = h.Load(m.store, false)
	assert.Empty(t, rows)

	require.NoError(t, h.Restore(m.store, id))
	rows, _, _, _ = h.Load(m.store, false)
	assert.Len(t, rows, 1)
}

// ---------------------------------------------------------------------------
// vendorHandler CRUD
// ---------------------------------------------------------------------------

func TestVendorHandlerLoadAndSubmit(t *testing.T) {
	m := newTestModelWithStore(t)
	h := vendorHandler{}

	m.formData = &vendorFormData{
		Name:  "Bob's Plumbing",
		Phone: "555-1234",
	}
	require.NoError(t, h.SubmitForm(m))

	rows, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	// Edit vendor.
	editID := meta[0].ID
	m.editID = &editID
	m.formData = &vendorFormData{
		Name:  "Bob's Plumbing",
		Phone: "555-5678",
		Email: "bob@plumbing.com",
	}
	require.NoError(t, h.SubmitForm(m))
	m.editID = nil

	vendor, _ := m.store.GetVendor(editID)
	assert.Equal(t, "555-5678", vendor.Phone)
	assert.Equal(t, "bob@plumbing.com", vendor.Email)
}

// Vendor delete/restore tests moved to vendor_test.go (TestVendorHandlerDeleteRestore)
// -- they now require a real store.

// ---------------------------------------------------------------------------
// quoteHandler CRUD
// ---------------------------------------------------------------------------

func TestQuoteHandlerRoundTrip(t *testing.T) {
	m := newTestModelWithStore(t)
	h := quoteHandler{}

	// Need a project first.
	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Bathroom Reno",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusQuoted,
	}))
	projects, _ := m.store.ListProjects(false)
	projID := projects[0].ID

	m.formData = &quoteFormData{
		ProjectID:  projID,
		VendorName: "Acme Contractors",
		Total:      "1,500.00",
	}
	require.NoError(t, h.SubmitForm(m))

	rows, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	id := meta[0].ID

	// Delete.
	require.NoError(t, h.Delete(m.store, id))
	rows, _, _, _ = h.Load(m.store, false)
	assert.Empty(t, rows)

	// Restore.
	require.NoError(t, h.Restore(m.store, id))
	rows, _, _, _ = h.Load(m.store, false)
	assert.Len(t, rows, 1)
}

func TestQuoteHandlerSnapshot(t *testing.T) {
	m := newTestModelWithStore(t)
	h := quoteHandler{}

	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Garage Door",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusQuoted,
	}))
	projects, _ := m.store.ListProjects(false)

	m.formData = &quoteFormData{
		ProjectID:  projects[0].ID,
		VendorName: "QuoteCo",
		Total:      "200.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	entry, ok := h.Snapshot(m.store, id)
	require.True(t, ok)
	assert.Equal(t, formQuote, entry.FormKind)
}

// ---------------------------------------------------------------------------
// serviceLogHandler CRUD
// ---------------------------------------------------------------------------

func TestServiceLogHandlerRoundTrip(t *testing.T) {
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	// Create a maintenance item to attach logs to.
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Oil Furnace",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	h := serviceLogHandler{maintenanceItemID: maintID}

	m.formData = &serviceLogFormData{
		MaintenanceItemID: maintID,
		ServicedAt:        "2026-01-15",
		Cost:              "75.00",
		Notes:             "routine service",
	}
	require.NoError(t, h.SubmitForm(m))

	rows, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	id := meta[0].ID

	// Delete.
	require.NoError(t, h.Delete(m.store, id))
	rows, _, _, _ = h.Load(m.store, false)
	assert.Empty(t, rows)

	// Restore.
	require.NoError(t, h.Restore(m.store, id))
	rows, _, _, _ = h.Load(m.store, false)
	assert.Len(t, rows, 1)
}

func TestServiceLogHandlerSnapshot(t *testing.T) {
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Gutter Clean",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	h := serviceLogHandler{maintenanceItemID: maintID}

	m.formData = &serviceLogFormData{
		MaintenanceItemID: maintID,
		ServicedAt:        "2026-01-20",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	entry, ok := h.Snapshot(m.store, id)
	require.True(t, ok)
	assert.Equal(t, formServiceLog, entry.FormKind)
	// Restore function should not error.
	assert.NoError(t, entry.Restore())
}

// ---------------------------------------------------------------------------
// Handler SyncFixedValues
// ---------------------------------------------------------------------------

func TestProjectHandlerSyncFixedValues(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}
	specs := []columnSpec{
		{Title: "Type"},
		{Title: "Status"},
	}
	h.SyncFixedValues(m, specs)

	assert.NotEmpty(t, specs[0].FixedValues, "expected FixedValues for Type column")
}

func TestMaintenanceHandlerSyncFixedValues(t *testing.T) {
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	specs := []columnSpec{
		{Title: "Category"},
		{Title: "Item"},
	}
	h.SyncFixedValues(m, specs)

	assert.NotEmpty(t, specs[0].FixedValues, "expected FixedValues for Category column")
}

// ---------------------------------------------------------------------------
// Handler with non-existent IDs
// ---------------------------------------------------------------------------

func TestHandlerSnapshotNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	handlers := []struct {
		name string
		h    TabHandler
	}{
		{"project", projectHandler{}},
		{"quote", quoteHandler{}},
		{"maintenance", maintenanceHandler{}},
		{"appliance", applianceHandler{}},
		{"vendor", vendorHandler{}},
		{"serviceLog", serviceLogHandler{maintenanceItemID: 1}},
		{"incident", incidentHandler{}},
	}
	for _, tc := range handlers {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := tc.h.Snapshot(m.store, 99999)
			assert.False(t, ok)
		})
	}
}

// ---------------------------------------------------------------------------
// applianceMaintenanceHandler (detail view)
// ---------------------------------------------------------------------------

// vendorJobsHandler inline edit
// ---------------------------------------------------------------------------

func TestVendorJobsInlineEditNotesOpensTextarea(t *testing.T) {
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	// Create a vendor.
	require.NoError(t, m.store.CreateVendor(&data.Vendor{Name: "Fix-It Co"}))
	vendors, _ := m.store.ListVendors(false)
	vendorID := vendors[0].ID

	// Create a maintenance item.
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Replace Filter",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	// Create a service log entry assigned to the vendor.
	require.NoError(t, m.store.CreateServiceLog(
		&data.ServiceLogEntry{
			MaintenanceItemID: maintID,
			ServicedAt:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Notes:             "initial notes",
		},
		data.Vendor{Name: "Fix-It Co"},
	))

	h := newVendorJobsHandler(vendorID)
	rows, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	// Notes column should open textarea overlay (not inline input).
	require.NoError(t, h.InlineEdit(m, meta[0].ID, int(vendorJobsColNotes)))
	assert.Nil(t, m.inlineInput, "Notes should not use inline input")
	assert.Equal(t, modeForm, m.mode, "Notes should open in form mode")
	assert.True(t, m.notesEditMode, "notesEditMode should be set")
}

func TestVendorJobsInlineEditItemShowsStatusMessage(t *testing.T) {
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateVendor(&data.Vendor{Name: "Fix-It Co"}))
	vendors, _ := m.store.ListVendors(false)
	vendorID := vendors[0].ID

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Replace Filter",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	require.NoError(t, m.store.CreateServiceLog(
		&data.ServiceLogEntry{
			MaintenanceItemID: maintID,
			ServicedAt:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		},
		data.Vendor{Name: "Fix-It Co"},
	))

	h := newVendorJobsHandler(vendorID)
	_, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)

	// Item is a FK reference; should set a status message.
	require.NoError(t, h.InlineEdit(m, meta[0].ID, int(vendorJobsColItem)))
	assert.Nil(t, m.inlineInput, "Item column should not open inline input")
	assert.Contains(t, m.status.Text, "Maintenance")
}

// ---------------------------------------------------------------------------
// incidentHandler CRUD
// ---------------------------------------------------------------------------

func TestIncidentHandlerLoadDeleteRestoreRoundTrip(t *testing.T) {
	m := newTestModelWithStore(t)
	h := incidentHandler{}

	m.formData = &incidentFormData{
		Title:       "Broken window",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeverityUrgent,
		DateNoticed: time.Now().Format("2006-01-02"),
	}
	require.NoError(t, h.SubmitForm(m))

	rows, meta, cells, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, meta, 1)
	require.Len(t, cells, 1)
	id := meta[0].ID

	// Delete.
	require.NoError(t, h.Delete(m.store, id))
	rows, _, _, err = h.Load(m.store, false)
	require.NoError(t, err)
	assert.Empty(t, rows)

	// Load with deleted should show it.
	rows, _, _, err = h.Load(m.store, true)
	require.NoError(t, err)
	assert.Len(t, rows, 1)

	// Restore.
	require.NoError(t, h.Restore(m.store, id))
	rows, _, _, _ = h.Load(m.store, false)
	assert.Len(t, rows, 1)
}

func TestIncidentHandlerEditRoundTrip(t *testing.T) {
	m := newTestModelWithStore(t)
	h := incidentHandler{}

	m.formData = &incidentFormData{
		Title:       "Water stain",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: "2026-02-01",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	editID := id
	m.editID = &editID
	m.formData = &incidentFormData{
		Title:       "Water stain on ceiling",
		Status:      data.IncidentStatusInProgress,
		Severity:    data.IncidentSeverityUrgent,
		DateNoticed: "2026-02-01",
		Cost:        "250.00",
	}
	require.NoError(t, h.SubmitForm(m))
	m.editID = nil

	inc, err := m.store.GetIncident(id)
	require.NoError(t, err)
	assert.Equal(t, "Water stain on ceiling", inc.Title)
	assert.Equal(t, data.IncidentStatusInProgress, inc.Status)
	require.NotNil(t, inc.CostCents)
	assert.Equal(t, int64(25000), *inc.CostCents)
}

func TestIncidentHandlerSnapshot(t *testing.T) {
	m := newTestModelWithStore(t)
	h := incidentHandler{}

	m.formData = &incidentFormData{
		Title:       "Cracked tile",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeverityWhenever,
		DateNoticed: "2026-01-10",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	entry, ok := h.Snapshot(m.store, id)
	require.True(t, ok)
	assert.Equal(t, formIncident, entry.FormKind)
	assert.Equal(t, id, entry.EntityID)
}

func TestIncidentHandlerSyncFixedValues(t *testing.T) {
	m := newTestModelWithStore(t)
	h := incidentHandler{}
	specs := []columnSpec{
		{Title: "ID"},
		{Title: "Title"},
		{Title: "Status"},
		{Title: "Severity"},
	}
	h.SyncFixedValues(m, specs)

	assert.NotEmpty(t, specs[2].FixedValues, "expected FixedValues for Status column")
	assert.NotEmpty(t, specs[3].FixedValues, "expected FixedValues for Severity column")
}

// ---------------------------------------------------------------------------
// Incident tab: Model-level user journey
// ---------------------------------------------------------------------------

func TestIncidentTabShowsDeletedByDefault(t *testing.T) {
	m := newTestModelWithStore(t)

	// User creates two incidents.
	for _, inc := range []data.Incident{
		{Title: "Broken pipe", Status: data.IncidentStatusOpen, Severity: data.IncidentSeverityUrgent},
		{Title: "Cracked tile", Status: data.IncidentStatusOpen, Severity: data.IncidentSeverityWhenever},
	} {
		require.NoError(t, m.store.CreateIncident(&inc))
	}

	// User navigates to the Incidents tab and sees both.
	m.active = tabIndex(tabIncidents)
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	require.NotNil(t, tab)
	require.Len(t, tab.Rows, 2, "both incidents visible")
	assert.True(t, tab.ShowDeleted, "incidents tab defaults to ShowDeleted")

	// User resolves (soft-deletes) one incident.
	items, _ := m.store.ListIncidents(false)
	require.NoError(t, m.store.DeleteIncident(items[0].ID))

	// Reload: resolved incident still visible because ShowDeleted is true.
	require.NoError(t, m.reloadActiveTab())
	assert.Len(t, tab.Rows, 2, "resolved incident still visible with ShowDeleted")

	// Verify the deleted row has Deleted flag set (visible as strikethrough).
	found := false
	for _, meta := range tab.Rows {
		if meta.ID == items[0].ID {
			assert.True(t, meta.Deleted, "resolved row should be marked deleted")
			found = true
		}
	}
	assert.True(t, found, "expected to find the resolved incident in tab rows")
}

// ---------------------------------------------------------------------------
// applianceMaintenanceHandler (detail view)
// ---------------------------------------------------------------------------

func TestApplianceMaintenanceHandlerLoad(t *testing.T) {
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	// Create an appliance with maintenance items.
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "HVAC"}))
	apps, _ := m.store.ListAppliances(false)
	appID := apps[0].ID

	lastSrv := time.Now()
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:           "Replace Belt",
		CategoryID:     cats[0].ID,
		ApplianceID:    &appID,
		LastServicedAt: &lastSrv,
		IntervalMonths: 12,
	}))

	h := newApplianceMaintenanceHandler(appID)
	rows, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.NotZero(t, meta[0].ID)
}

// ---------------------------------------------------------------------------
// Regression: ctrl+s during add form must not create duplicates (#354)
// ---------------------------------------------------------------------------

// TestSaveFormInPlaceSetEditID exercises the real ctrl+s code path
// (saveFormInPlace) for every entity type. Two consecutive saves with the
// form data modified between them must produce exactly one row whose
// contents match the second save (proving the update path ran).
func TestSaveFormInPlaceSetEditID(t *testing.T) {
	// Assert that saveFormInPlace didn't surface a status-bar error.
	requireNoStatusError := func(t *testing.T, m *Model, ctx string) {
		t.Helper()
		require.NotEqualf(
			t, statusError, m.status.Kind,
			"%s: unexpected status error: %s", ctx, m.status.Text,
		)
	}

	t.Run("project", func(t *testing.T) {
		m := newTestModelWithStore(t)
		m.formKind = formProject
		m.formData = &projectFormData{
			Title:         "Deck Build",
			ProjectTypeID: m.projectTypes[0].ID,
			Status:        data.ProjectStatusPlanned,
		}

		require.Nil(t, m.editID)
		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.editID, "editID should be set after create")
		firstID := *m.editID

		// Modify form data so the second save proves update, not a
		// blocked duplicate create.
		m.formData = &projectFormData{
			Title:         "Deck Build v2",
			ProjectTypeID: m.projectTypes[0].ID,
			Status:        data.ProjectStatusInProgress,
		}
		m.status = statusMsg{}
		m.saveFormInPlace()
		requireNoStatusError(t, m, "second save")
		assert.Equal(t, firstID, *m.editID, "editID must not change on update")

		projects, err := m.store.ListProjects(false)
		require.NoError(t, err)
		require.Len(t, projects, 1, "expected exactly 1 project")
		assert.Equal(t, "Deck Build v2", projects[0].Title)
	})

	t.Run("vendor", func(t *testing.T) {
		m := newTestModelWithStore(t)
		m.formKind = formVendor
		m.formData = &vendorFormData{Name: "Test Plumber", Phone: "555-0001"}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.editID)

		// Change phone — without the fix the old code would try to
		// create a second vendor with the same name and hit a unique
		// constraint error instead of updating.
		m.formData = &vendorFormData{Name: "Test Plumber", Phone: "555-0002"}
		m.status = statusMsg{}
		m.saveFormInPlace()
		requireNoStatusError(t, m, "second save")

		vendors, err := m.store.ListVendors(false)
		require.NoError(t, err)
		require.Len(t, vendors, 1)
		assert.Equal(t, "555-0002", vendors[0].Phone, "second save should update")
	})

	t.Run("appliance", func(t *testing.T) {
		m := newTestModelWithStore(t)
		m.formKind = formAppliance
		m.formData = &applianceFormData{Name: "Dishwasher"}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.editID)

		m.formData = &applianceFormData{Name: "Dishwasher", Brand: "Bosch"}
		m.status = statusMsg{}
		m.saveFormInPlace()
		requireNoStatusError(t, m, "second save")

		apps, err := m.store.ListAppliances(false)
		require.NoError(t, err)
		require.Len(t, apps, 1)
		assert.Equal(t, "Bosch", apps[0].Brand, "second save should update")
	})

	t.Run("maintenance", func(t *testing.T) {
		m := newTestModelWithStore(t)
		cats, _ := m.store.MaintenanceCategories()
		m.formKind = formMaintenance
		m.formData = &maintenanceFormData{
			Name:         "Change Filter",
			CategoryID:   cats[0].ID,
			ScheduleType: schedNone,
		}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.editID)

		m.formData = &maintenanceFormData{
			Name:           "Change Filter",
			CategoryID:     cats[0].ID,
			ScheduleType:   schedInterval,
			IntervalMonths: "6",
		}
		m.status = statusMsg{}
		m.saveFormInPlace()
		requireNoStatusError(t, m, "second save")

		items, err := m.store.ListMaintenance(false)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, 6, items[0].IntervalMonths, "second save should update")
	})

	t.Run("quote", func(t *testing.T) {
		m := newTestModelWithStore(t)
		types, _ := m.store.ProjectTypes()
		require.NoError(t, m.store.CreateProject(&data.Project{
			Title:         "Test Proj",
			ProjectTypeID: types[0].ID,
			Status:        data.ProjectStatusPlanned,
		}))
		projects, _ := m.store.ListProjects(false)

		m.formKind = formQuote
		m.formData = &quoteFormData{
			ProjectID:  projects[0].ID,
			VendorName: "QuoteCo",
			Total:      "500.00",
		}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.editID)

		m.formData = &quoteFormData{
			ProjectID:  projects[0].ID,
			VendorName: "QuoteCo",
			Total:      "750.00",
		}
		m.status = statusMsg{}
		m.saveFormInPlace()
		requireNoStatusError(t, m, "second save")

		quotes, err := m.store.ListQuotes(false)
		require.NoError(t, err)
		require.Len(t, quotes, 1)
		assert.Equal(t, int64(75000), quotes[0].TotalCents, "second save should update")
	})

	t.Run("serviceLog", func(t *testing.T) {
		m := newTestModelWithStore(t)
		cats, _ := m.store.MaintenanceCategories()
		require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
			Name:       "HVAC Filter",
			CategoryID: cats[0].ID,
		}))
		items, _ := m.store.ListMaintenance(false)
		maintID := items[0].ID

		// Use the real detail-stack setup path instead of manually
		// wiring detailStack.
		require.NoError(t, m.openServiceLogDetail(maintID, "HVAC Filter"))

		m.formKind = formServiceLog
		m.formData = &serviceLogFormData{
			MaintenanceItemID: maintID,
			ServicedAt:        "2026-01-15",
		}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.editID)

		m.formData = &serviceLogFormData{
			MaintenanceItemID: maintID,
			ServicedAt:        "2026-01-15",
			Notes:             "replaced filter",
		}
		m.status = statusMsg{}
		m.saveFormInPlace()
		requireNoStatusError(t, m, "second save")

		entries, err := m.store.ListServiceLog(maintID, false)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "replaced filter", entries[0].Notes, "second save should update")
	})

	t.Run("document", func(t *testing.T) {
		m := newTestModelWithStore(t)
		m.formKind = formDocument
		m.formData = &documentFormData{
			Title:     "Test Doc",
			EntityRef: entityRef{Kind: data.DocumentEntityProject},
		}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.editID)

		m.formData = &documentFormData{
			Title:     "Test Doc (revised)",
			EntityRef: entityRef{Kind: data.DocumentEntityProject},
		}
		m.status = statusMsg{}
		m.saveFormInPlace()
		requireNoStatusError(t, m, "second save")

		docs, err := m.store.ListDocuments(false)
		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, "Test Doc (revised)", docs[0].Title, "second save should update")
	})

	t.Run("scopedDocument", func(t *testing.T) {
		m := newTestModelWithStore(t)
		types, _ := m.store.ProjectTypes()
		require.NoError(t, m.store.CreateProject(&data.Project{
			Title:         "Scoped Doc Proj",
			ProjectTypeID: types[0].ID,
			Status:        data.ProjectStatusPlanned,
		}))
		projects, _ := m.store.ListProjects(false)
		projID := projects[0].ID

		// Use the real project-document detail view.
		require.NoError(t, m.openProjectDocumentDetail(projID, "Scoped Doc Proj"))

		m.formKind = formDocument
		m.formData = &documentFormData{
			Title:     "Permit",
			EntityRef: entityRef{Kind: data.DocumentEntityProject},
		}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.editID)

		m.formData = &documentFormData{
			Title:     "Permit (final)",
			EntityRef: entityRef{Kind: data.DocumentEntityProject},
		}
		m.status = statusMsg{}
		m.saveFormInPlace()
		requireNoStatusError(t, m, "second save")

		docs, err := m.store.ListDocuments(false)
		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, "Permit (final)", docs[0].Title, "second save should update")
		assert.Equal(t, projID, docs[0].EntityID, "entity ID must survive update")
		assert.Equal(
			t,
			data.DocumentEntityProject,
			docs[0].EntityKind,
			"entity kind must survive update",
		)
	})
}
