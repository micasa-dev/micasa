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
	t.Parallel()
	m := newTestModelWithStore(t)
	h := projectHandler{}

	// Create a project via form data.
	m.fs.formData = &projectFormData{
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
	t.Parallel()
	m := newTestModelWithStore(t)
	h := projectHandler{}

	// Create.
	m.fs.formData = &projectFormData{
		Title:         "Paint Fence",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusIdeating,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	// Edit via form data.
	editID := id
	m.fs.editID = &editID
	m.fs.formData = &projectFormData{
		Title:         "Paint Fence Red",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusInProgress,
		Budget:        "500.00",
	}
	require.NoError(t, h.SubmitForm(m))
	m.fs.editID = nil

	project, err := m.store.GetProject(id)
	require.NoError(t, err)
	assert.Equal(t, "Paint Fence Red", project.Title)
	assert.Equal(t, data.ProjectStatusInProgress, project.Status)
	require.NotNil(t, project.BudgetCents)
	assert.Equal(t, int64(50000), *project.BudgetCents)
}

func TestProjectTabStatusFiltersRows(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	m := newTestModelWithStore(t)
	h := applianceHandler{}

	m.fs.formData = &applianceFormData{Name: "Washer"}
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
	t.Parallel()
	m := newTestModelWithStore(t)
	h := applianceHandler{}

	m.fs.formData = &applianceFormData{Name: "Dryer"}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	editID := id
	m.fs.editID = &editID
	m.fs.formData = &applianceFormData{
		Name:  "Dryer",
		Brand: "LG",
		Cost:  "800.00",
	}
	require.NoError(t, h.SubmitForm(m))
	m.fs.editID = nil

	app, _ := m.store.GetAppliance(id)
	assert.Equal(t, "LG", app.Brand)
	require.NotNil(t, app.CostCents)
	assert.Equal(t, int64(80000), *app.CostCents)
}

// ---------------------------------------------------------------------------
// maintenanceHandler CRUD
// ---------------------------------------------------------------------------

func TestMaintenanceHandlerLoadDeleteRestoreRoundTrip(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	cats, _ := m.store.MaintenanceCategories()

	m.fs.formData = &maintenanceFormData{
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
	t.Parallel()
	m := newTestModelWithStore(t)
	h := vendorHandler{}

	m.fs.formData = &vendorFormData{
		Name:  "Bob's Plumbing",
		Phone: "555-1234",
	}
	require.NoError(t, h.SubmitForm(m))

	rows, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	// Edit vendor.
	editID := meta[0].ID
	m.fs.editID = &editID
	m.fs.formData = &vendorFormData{
		Name:  "Bob's Plumbing",
		Phone: "555-5678",
		Email: "bob@plumbing.com",
	}
	require.NoError(t, h.SubmitForm(m))
	m.fs.editID = nil

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
	t.Parallel()
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

	m.fs.formData = &quoteFormData{
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

// ---------------------------------------------------------------------------
// serviceLogHandler CRUD
// ---------------------------------------------------------------------------

func TestServiceLogHandlerRoundTrip(t *testing.T) {
	t.Parallel()
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

	m.fs.formData = &serviceLogFormData{
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

func TestServiceLogFormSyncsLastServiced(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, err := m.store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Filter Change",
		CategoryID: cats[0].ID,
	}))
	items, err := m.store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	maintID := items[0].ID

	// Navigate to the Maintenance tab and load it.
	maintIdx := tabIndex(tabMaintenance)
	m.active = maintIdx
	require.NoError(t, m.reloadActiveTab())
	maintTab := m.activeTab()
	require.NotNil(t, maintTab)
	require.Len(t, maintTab.CellRows, 1)

	// Last column should be empty before any service log entries.
	lastCol := int(maintenanceColLast)
	assert.True(t, maintTab.CellRows[0][lastCol].Null,
		"Last should be empty before any service log entries")

	// Open service log detail for the maintenance item.
	require.NoError(t, m.openServiceLogDetail(maintID, "Filter Change"))
	require.True(t, m.inDetail())

	// Submit a service log entry via the form path (ctrl+s equivalent).
	m.fs.formData = &serviceLogFormData{
		MaintenanceItemID: maintID,
		ServicedAt:        "2026-02-15",
		Notes:             "changed filter",
	}
	m.saveFormInPlace()
	require.NotEqual(t, statusError, m.status.Kind,
		"service log save: unexpected error: %s", m.status.Text)

	// Close the detail view (Esc back to Maintenance tab).
	m.closeDetail()
	assert.False(t, m.inDetail())
	assert.Equal(t, maintIdx, m.active)

	// The maintenance tab should now show the updated Last date.
	maintTab = m.activeTab()
	require.Len(t, maintTab.CellRows, 1)
	assert.False(t, maintTab.CellRows[0][lastCol].Null,
		"Last should be populated after service log creation")
	assert.Equal(t, "2026-02-15", maintTab.CellRows[0][lastCol].Value)
}

// ---------------------------------------------------------------------------
// Handler SyncFixedValues
// ---------------------------------------------------------------------------

func TestProjectHandlerSyncFixedValues(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	specs := []columnSpec{
		{Title: "Category"},
		{Title: "Season"},
		{Title: "Item"},
	}
	h.SyncFixedValues(m, specs)

	assert.NotEmpty(t, specs[0].FixedValues, "expected FixedValues for Category column")
	assert.NotEmpty(t, specs[1].FixedValues, "expected FixedValues for Season column")
	assert.Equal(t, []string{
		data.SeasonSpring,
		data.SeasonSummer,
		data.SeasonFall,
		data.SeasonWinter,
	}, specs[1].FixedValues)
}

func TestMaintenanceHandlerCreateWithSeasonRoundTrip(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	cats, _ := m.store.MaintenanceCategories()

	m.fs.formData = &maintenanceFormData{
		Name:       "Winterize Sprinklers",
		CategoryID: cats[0].ID,
		Season:     data.SeasonFall,
	}
	require.NoError(t, h.SubmitForm(m))

	_, _, cells, err := h.Load(m.store, false)
	require.NoError(t, err)
	require.Len(t, cells, 1)
	seasonCell := cells[0][int(maintenanceColSeason)]
	assert.Equal(t, data.SeasonFall, seasonCell.Value)
	assert.Equal(t, cellStatus, seasonCell.Kind)
}

func TestMaintenanceHandlerEditSeasonRoundTrip(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	cats, _ := m.store.MaintenanceCategories()

	m.fs.formData = &maintenanceFormData{
		Name:       "Clean Gutters",
		CategoryID: cats[0].ID,
		Season:     data.SeasonSpring,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	// Edit to change season.
	editID := id
	m.fs.editID = &editID
	m.fs.formData = &maintenanceFormData{
		Name:       "Clean Gutters",
		CategoryID: cats[0].ID,
		Season:     data.SeasonWinter,
	}
	require.NoError(t, h.SubmitForm(m))
	m.fs.editID = nil

	item, err := m.store.GetMaintenance(id)
	require.NoError(t, err)
	assert.Equal(t, data.SeasonWinter, item.Season)
}

// ---------------------------------------------------------------------------
// Handler with non-existent IDs
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// applianceMaintenanceHandler (detail view)
// ---------------------------------------------------------------------------

// vendorJobsHandler inline edit
// ---------------------------------------------------------------------------

func TestVendorJobsInlineEditNotesOpensTextarea(t *testing.T) {
	t.Parallel()
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
	assert.True(t, m.fs.notesEditMode, "notesEditMode should be set")
}

func TestVendorJobsInlineEditItemShowsStatusMessage(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	m := newTestModelWithStore(t)
	h := incidentHandler{}

	m.fs.formData = &incidentFormData{
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
	t.Parallel()
	m := newTestModelWithStore(t)
	h := incidentHandler{}

	m.fs.formData = &incidentFormData{
		Title:       "Water stain",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: "2026-02-01",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	editID := id
	m.fs.editID = &editID
	m.fs.formData = &incidentFormData{
		Title:       "Water stain on ceiling",
		Status:      data.IncidentStatusInProgress,
		Severity:    data.IncidentSeverityUrgent,
		DateNoticed: "2026-02-01",
		Cost:        "250.00",
	}
	require.NoError(t, h.SubmitForm(m))
	m.fs.editID = nil

	inc, err := m.store.GetIncident(id)
	require.NoError(t, err)
	assert.Equal(t, "Water stain on ceiling", inc.Title)
	assert.Equal(t, data.IncidentStatusInProgress, inc.Status)
	require.NotNil(t, inc.CostCents)
	assert.Equal(t, int64(25000), *inc.CostCents)
}

func TestIncidentHandlerSyncFixedValues(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestIncidentDeleteSetsResolvedStatus(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := incidentHandler{}

	m.fs.formData = &incidentFormData{
		Title:       "Flickering light",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: "2026-03-01",
	}
	require.NoError(t, h.SubmitForm(m))

	_, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	id := meta[0].ID

	// Delete sets status to resolved.
	require.NoError(t, h.Delete(m.store, id))
	items, err := m.store.ListIncidents(true)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, data.IncidentStatusResolved, items[0].Status)

	// Restore resets status to open.
	require.NoError(t, h.Restore(m.store, id))
	inc, err := m.store.GetIncident(id)
	require.NoError(t, err)
	assert.Equal(t, data.IncidentStatusOpen, inc.Status)
}

func TestIncidentRestorePreservesPreviousStatus(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := incidentHandler{}

	m.fs.formData = &incidentFormData{
		Title:       "Active repair",
		Status:      data.IncidentStatusInProgress,
		Severity:    data.IncidentSeverityUrgent,
		DateNoticed: "2026-03-01",
	}
	require.NoError(t, h.SubmitForm(m))

	_, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	id := meta[0].ID

	require.NoError(t, h.Delete(m.store, id))
	require.NoError(t, h.Restore(m.store, id))

	inc, err := m.store.GetIncident(id)
	require.NoError(t, err)
	assert.Equal(
		t,
		data.IncidentStatusInProgress,
		inc.Status,
		"should restore to in_progress, not open",
	)
}

func TestIncidentResolveRestoreUserFlow(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create an incident via the store.
	require.NoError(t, m.store.CreateIncident(&data.Incident{
		Title:    "Burst pipe",
		Status:   data.IncidentStatusOpen,
		Severity: data.IncidentSeverityUrgent,
	}))

	// Navigate to incidents tab.
	m.active = tabIndex(tabIncidents)
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	require.NotNil(t, tab)
	require.Len(t, tab.Rows, 1)

	// Enter edit mode, press d to resolve.
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)
	sendKey(m, "d")
	assert.Contains(t, m.statusView(), "Resolved")

	// Verify status changed to resolved (must list with deleted to see it).
	id := tab.Rows[0].ID
	items, err := m.store.ListIncidents(true)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, data.IncidentStatusResolved, items[0].Status)

	// Press d again to reopen.
	sendKey(m, "d")
	assert.Contains(t, m.statusView(), "Reopened")

	inc, err := m.store.GetIncident(id)
	require.NoError(t, err)
	assert.Equal(t, data.IncidentStatusOpen, inc.Status)
}

func TestIncidentHardDeleteUserFlow(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	require.NoError(t, m.store.CreateIncident(&data.Incident{
		Title:    "Temporary issue",
		Status:   data.IncidentStatusOpen,
		Severity: data.IncidentSeverityWhenever,
	}))

	m.active = tabIndex(tabIncidents)
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	require.Len(t, tab.Rows, 1)
	id := tab.Rows[0].ID

	// Enter edit mode, D on a live row should be rejected.
	sendKey(m, "i")
	sendKey(m, "D")
	assert.NotEqual(t, confirmHardDelete, m.confirm, "should not prompt on non-deleted row")
	assert.Contains(t, m.statusView(), "Resolve the incident first")

	// Soft-delete first, then hard delete.
	sendKey(m, "d")
	require.NoError(t, m.reloadActiveTab())

	sendKey(m, "D")
	assert.Equal(t, confirmHardDelete, m.confirm, "should be in confirm state")
	assert.Contains(t, m.statusView(), "Permanently delete")

	// Press n to cancel.
	sendKey(m, "n")
	assert.Equal(t, confirmNone, m.confirm, "confirm should be dismissed")

	// Row should still exist.
	require.NoError(t, m.reloadActiveTab())
	assert.Len(t, tab.Rows, 1)

	// Press D then y to confirm.
	sendKey(m, "D")
	sendKey(m, "y")
	assert.Equal(t, confirmNone, m.confirm)
	assert.Contains(t, m.statusView(), "Permanently deleted")

	// Row is gone even with showDeleted.
	tab.ShowDeleted = true
	require.NoError(t, m.reloadActiveTab())
	assert.Empty(t, tab.Rows)

	// Verify at the DB level.
	_, err := m.store.GetIncident(id)
	assert.Error(t, err)
}

func TestIncidentStatusResolvedViaFormSoftDeletes(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := incidentHandler{}

	// Create an incident.
	m.fs.formData = &incidentFormData{
		Title:       "Noisy pipe",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: "2026-03-01",
	}
	require.NoError(t, h.SubmitForm(m))

	_, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	id := meta[0].ID

	// Edit the incident, setting status to resolved via form.
	editID := id
	m.fs.editID = &editID
	m.fs.formData = &incidentFormData{
		Title:       "Noisy pipe",
		Status:      data.IncidentStatusResolved,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: "2026-03-01",
	}
	require.NoError(t, h.SubmitForm(m))
	m.fs.editID = nil

	// Should be soft-deleted.
	rows, _, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	assert.Empty(t, rows, "resolved incident should be soft-deleted")

	// Should still be visible with showDeleted.
	rows, rowMeta, _, err := h.Load(m.store, true)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.True(t, rowMeta[0].Deleted)
}

func TestIncidentHardDeleteOnlyWorksOnIncidents(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create a project via form submission so FK constraints are satisfied.
	m.fs.formData = &projectFormData{
		Title:         "Paint fence",
		ProjectTypeID: m.projectTypes[0].ID,
	}
	require.NoError(t, projectHandler{}.SubmitForm(m))

	m.active = tabIndex(tabProjects)
	require.NoError(t, m.reloadActiveTab())

	sendKey(m, "i")
	sendKey(m, "D")
	assert.NotEqual(
		t,
		confirmHardDelete,
		m.confirm,
		"hard delete should not activate on projects tab",
	)
}

func TestIncidentHardDeleteOnEmptyTable(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Incidents tab with no rows.
	m.active = tabIndex(tabIncidents)
	require.NoError(t, m.reloadActiveTab())
	require.Empty(t, m.activeTab().Rows)

	sendKey(m, "i")
	sendKey(m, "D")
	assert.Equal(t, confirmNone, m.confirm, "should not prompt when nothing is selected")
	assert.Contains(t, m.statusView(), "Nothing selected")
}

func TestIncidentHardDeleteErrorPath(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Set up a hard-delete confirmation with an ID that doesn't exist.
	m.active = tabIndex(tabIncidents)
	m.mode = modeEdit
	m.confirm = confirmHardDelete
	m.hardDeleteID = "01JNOTEXIST000000000999999"

	sendKey(m, "y")
	assert.Equal(t, confirmNone, m.confirm, "confirm should be cleared even on error")
	assert.Contains(t, m.statusView(), "not found")
}

func TestMaintenanceHardDeleteUserFlow(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, err := m.store.MaintenanceCategories()
	require.NoError(t, err)

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name: "Replace filter", CategoryID: cats[0].ID,
	}))

	m.active = tabIndex(tabMaintenance)
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	require.Len(t, tab.Rows, 1)

	// Enter edit mode, D on a non-deleted row should be rejected.
	sendKey(m, "i")
	sendKey(m, "D")
	assert.NotEqual(t, confirmHardDelete, m.confirm, "should not prompt on non-deleted row")
	assert.Contains(t, m.statusView(), "Delete the item first")

	// Soft-delete first, then hard delete.
	sendKey(m, "d")
	require.NoError(t, m.reloadActiveTab())

	sendKey(m, "D")
	assert.Equal(t, confirmHardDelete, m.confirm, "should be in confirm state")
	assert.Contains(t, m.statusView(), "Permanently delete this item")

	// Press y to confirm.
	sendKey(m, "y")
	assert.Equal(t, confirmNone, m.confirm)
	assert.Contains(t, m.statusView(), "Permanently deleted")

	// Row is gone even with showDeleted.
	tab.ShowDeleted = true
	require.NoError(t, m.reloadActiveTab())
	assert.Empty(t, tab.Rows)
}

// ---------------------------------------------------------------------------
// applianceMaintenanceHandler (detail view)
// ---------------------------------------------------------------------------

func TestApplianceMaintenanceHandlerLoad(t *testing.T) {
	t.Parallel()
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
	assert.NotEmpty(t, meta[0].ID)
}

func TestApplianceMaintenanceInlineEditSeasonDispatchesCorrectly(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "Furnace"}))
	apps, _ := m.store.ListAppliances(false)
	appID := apps[0].ID

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:        "Replace Filter",
		CategoryID:  cats[0].ID,
		ApplianceID: &appID,
		Season:      data.SeasonFall,
	}))

	h := newApplianceMaintenanceHandler(appID)
	_, meta, _, err := h.Load(m.store, false)
	require.NoError(t, err)
	id := meta[0].ID

	// In the sub-table (Appliance column removed), Season is at sub-table
	// index 3. Since that's below the skipAt (maintenanceColAppliance),
	// skipColEdit passes it through to maintenanceColSeason in the full table.
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, 3))
	assert.Equal(t, modeForm, m.mode)

	// Verify the form opened for season, not appliance:
	// the form data should reflect the current season value.
	fd, ok := m.fs.formData.(*maintenanceFormData)
	require.True(t, ok)
	assert.Equal(t, data.SeasonFall, fd.Season,
		"inline edit on sub-table col 3 should dispatch to Season, not Appliance")
}

// ---------------------------------------------------------------------------
// Regression: ctrl+s during add form must not create duplicates (#354)
// ---------------------------------------------------------------------------

// TestSaveFormInPlaceSetEditID exercises the real ctrl+s code path
// (saveFormInPlace) for every entity type. Two consecutive saves with the
// form data modified between them must produce exactly one row whose
// contents match the second save (proving the update path ran).
func TestSaveFormInPlaceSetEditID(t *testing.T) {
	t.Parallel()
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
		m.fs.formData = &projectFormData{
			Title:         "Deck Build",
			ProjectTypeID: m.projectTypes[0].ID,
			Status:        data.ProjectStatusPlanned,
		}

		require.Nil(t, m.fs.editID)
		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.fs.editID, "editID should be set after create")
		firstID := *m.fs.editID

		// Modify form data so the second save proves update, not a
		// blocked duplicate create.
		m.fs.formData = &projectFormData{
			Title:         "Deck Build v2",
			ProjectTypeID: m.projectTypes[0].ID,
			Status:        data.ProjectStatusInProgress,
		}
		m.status = statusMsg{}
		m.saveFormInPlace()
		requireNoStatusError(t, m, "second save")
		assert.Equal(t, firstID, *m.fs.editID, "editID must not change on update")

		projects, err := m.store.ListProjects(false)
		require.NoError(t, err)
		require.Len(t, projects, 1, "expected exactly 1 project")
		assert.Equal(t, "Deck Build v2", projects[0].Title)
	})

	t.Run("vendor", func(t *testing.T) {
		m := newTestModelWithStore(t)
		m.fs.formData = &vendorFormData{Name: "Test Plumber", Phone: "555-0001"}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.fs.editID)

		// Change phone — without the fix the old code would try to
		// create a second vendor with the same name and hit a unique
		// constraint error instead of updating.
		m.fs.formData = &vendorFormData{Name: "Test Plumber", Phone: "555-0002"}
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
		m.fs.formData = &applianceFormData{Name: "Dishwasher"}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.fs.editID)

		m.fs.formData = &applianceFormData{Name: "Dishwasher", Brand: "Bosch"}
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
		m.fs.formData = &maintenanceFormData{
			Name:         "Change Filter",
			CategoryID:   cats[0].ID,
			ScheduleType: schedNone,
		}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.fs.editID)

		m.fs.formData = &maintenanceFormData{
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

		m.fs.formData = &quoteFormData{
			ProjectID:  projects[0].ID,
			VendorName: "QuoteCo",
			Total:      "500.00",
		}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.fs.editID)

		m.fs.formData = &quoteFormData{
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

		m.fs.formData = &serviceLogFormData{
			MaintenanceItemID: maintID,
			ServicedAt:        "2026-01-15",
		}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.fs.editID)

		m.fs.formData = &serviceLogFormData{
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
		m.fs.formData = &documentFormData{
			Title:     "Test Doc",
			EntityRef: entityRef{Kind: data.DocumentEntityProject},
		}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.fs.editID)

		m.fs.formData = &documentFormData{
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

		m.fs.formData = &documentFormData{
			Title:     "Permit",
			EntityRef: entityRef{Kind: data.DocumentEntityProject},
		}

		m.saveFormInPlace()
		requireNoStatusError(t, m, "first save")
		require.NotNil(t, m.fs.editID)

		m.fs.formData = &documentFormData{
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
