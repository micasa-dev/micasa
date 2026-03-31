// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// StartAddForm tests
// ---------------------------------------------------------------------------

func TestProjectHandlerStartAddForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newProjectHandler()
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formProject, m.fs.formKind())
	assert.IsType(t, &projectFormData{}, m.fs.formData)
	assert.NotNil(t, m.fs.form)
}

func TestQuoteHandlerStartAddFormRequiresProject(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newQuoteHandler()
	err := h.StartAddForm(m)
	require.Error(t, err, "should fail without projects")
	assert.Contains(t, err.Error(), "project")
}

func TestQuoteHandlerStartAddFormWithProject(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newQuoteHandler()

	// Create a project first.
	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Quote Test Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))

	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formQuote, m.fs.formKind())
	assert.IsType(t, &quoteFormData{}, m.fs.formData)
}

func TestMaintenanceHandlerStartAddForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newMaintenanceHandler()
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formMaintenance, m.fs.formKind())
	assert.IsType(t, &maintenanceFormData{}, m.fs.formData)
}

func TestApplianceHandlerStartAddForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newApplianceHandler()
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formAppliance, m.fs.formKind())
	assert.IsType(t, &applianceFormData{}, m.fs.formData)
}

func TestVendorHandlerStartAddForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newVendorHandler()
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formVendor, m.fs.formKind())
	assert.IsType(t, &vendorFormData{}, m.fs.formData)
}

func TestIncidentHandlerStartAddForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newIncidentHandler()
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formIncident, m.fs.formKind())
	assert.IsType(t, &incidentFormData{}, m.fs.formData)
}

func TestServiceLogHandlerStartAddForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Test Item",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	h := newServiceLogHandler(maintID)
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formServiceLog, m.fs.formKind())
	assert.IsType(t, &serviceLogFormData{}, m.fs.formData)
	fd, ok := m.fs.formData.(*serviceLogFormData)
	require.True(t, ok, "formData should be *serviceLogFormData")
	assert.Equal(t, maintID, fd.MaintenanceItemID)
}

func TestDocumentHandlerStartAddForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newDocumentHandler()
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formDocument, m.fs.formKind())
	assert.IsType(t, &documentFormData{}, m.fs.formData)
}

// ---------------------------------------------------------------------------
// StartEditForm tests
// ---------------------------------------------------------------------------

func TestProjectHandlerStartEditForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newProjectHandler()

	m.fs.formData = &projectFormData{
		Title:         "Edit Me",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formProject, m.fs.formKind())
	require.NotNil(t, m.fs.editID)
	assert.Equal(t, id, *m.fs.editID)
	fd, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Me", fd.Title)
}

func TestProjectHandlerStartEditFormNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newProjectHandler()
	err := h.StartEditForm(m, "01JNOTEXIST000000000099999")
	require.Error(t, err)
}

func TestQuoteHandlerStartEditForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newQuoteHandler()

	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Quote Edit Proj",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusQuoted,
	}))
	projects, _ := m.store.ListProjects(false)

	m.fs.formData = &quoteFormData{
		ProjectID:  projects[0].ID,
		VendorName: "EditQuoteCo",
		Total:      "800.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formQuote, m.fs.formKind())
	require.NotNil(t, m.fs.editID)
	assert.Equal(t, id, *m.fs.editID)
	fd, ok := m.fs.formData.(*quoteFormData)
	require.True(t, ok)
	assert.Equal(t, "EditQuoteCo", fd.VendorName)
}

func TestQuoteHandlerStartEditFormNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newQuoteHandler()
	err := h.StartEditForm(m, "01JNOTEXIST000000000099999")
	require.Error(t, err)
}

func TestMaintenanceHandlerStartEditForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newMaintenanceHandler()
	cats, _ := m.store.MaintenanceCategories()

	m.fs.formData = &maintenanceFormData{
		Name:         "Edit Maint",
		CategoryID:   cats[0].ID,
		ScheduleType: schedNone,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formMaintenance, m.fs.formKind())
	require.NotNil(t, m.fs.editID)
	assert.Equal(t, id, *m.fs.editID)
	fd, ok := m.fs.formData.(*maintenanceFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Maint", fd.Name)
}

func TestMaintenanceHandlerStartEditFormNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newMaintenanceHandler()
	err := h.StartEditForm(m, "01JNOTEXIST000000000099999")
	require.Error(t, err)
}

func TestApplianceHandlerStartEditForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newApplianceHandler()

	m.fs.formData = &applianceFormData{Name: "Edit Fridge"}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formAppliance, m.fs.formKind())
	require.NotNil(t, m.fs.editID)
	assert.Equal(t, id, *m.fs.editID)
	fd, ok := m.fs.formData.(*applianceFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Fridge", fd.Name)
}

func TestApplianceHandlerStartEditFormNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newApplianceHandler()
	err := h.StartEditForm(m, "01JNOTEXIST000000000099999")
	require.Error(t, err)
}

func TestVendorHandlerStartEditForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newVendorHandler()

	m.fs.formData = &vendorFormData{Name: "Edit Vendor Co"}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formVendor, m.fs.formKind())
	require.NotNil(t, m.fs.editID)
	assert.Equal(t, id, *m.fs.editID)
	fd, ok := m.fs.formData.(*vendorFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Vendor Co", fd.Name)
}

func TestVendorHandlerStartEditFormNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newVendorHandler()
	err := h.StartEditForm(m, "01JNOTEXIST000000000099999")
	require.Error(t, err)
}

func TestIncidentHandlerStartEditForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newIncidentHandler()

	m.fs.formData = &incidentFormData{
		Title:       "Edit Incident",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: "2026-02-01",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formIncident, m.fs.formKind())
	require.NotNil(t, m.fs.editID)
	assert.Equal(t, id, *m.fs.editID)
	fd, ok := m.fs.formData.(*incidentFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Incident", fd.Title)
}

func TestIncidentHandlerStartEditFormNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newIncidentHandler()
	err := h.StartEditForm(m, "01JNOTEXIST000000000099999")
	require.Error(t, err)
}

func TestServiceLogHandlerStartEditForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Log Edit Item",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	h := newServiceLogHandler(maintID)
	m.fs.formData = &serviceLogFormData{
		MaintenanceItemID: maintID,
		ServicedAt:        "2026-01-20",
		Notes:             "test log",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formServiceLog, m.fs.formKind())
	require.NotNil(t, m.fs.editID)
	assert.Equal(t, id, *m.fs.editID)
	fd, ok := m.fs.formData.(*serviceLogFormData)
	require.True(t, ok)
	assert.Equal(t, "test log", fd.Notes)
}

func TestServiceLogHandlerStartEditFormNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newServiceLogHandler("01JTEST00000000000000001")
	err := h.StartEditForm(m, "01JNOTEXIST000000000099999")
	require.Error(t, err)
}

func TestDocumentHandlerStartEditForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newDocumentHandler()

	// Create a document directly via store.
	require.NoError(t, m.store.CreateDocument(&data.Document{
		Title: "Edit Doc",
		Notes: "doc notes",
	}))
	docs, _ := m.store.ListDocuments(false)
	id := docs[0].ID

	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formDocument, m.fs.formKind())
	require.NotNil(t, m.fs.editID)
	assert.Equal(t, id, *m.fs.editID)
	fd, ok := m.fs.formData.(*documentFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Doc", fd.Title)
}

func TestDocumentHandlerStartEditFormNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newDocumentHandler()
	err := h.StartEditForm(m, "01JNOTEXIST000000000099999")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// InlineEdit tests
// ---------------------------------------------------------------------------

func TestProjectHandlerInlineEditTextColumn(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newProjectHandler()

	m.fs.formData = &projectFormData{
		Title:         "Inline Proj",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()

	// Title column opens inline input.
	require.NoError(t, h.InlineEdit(m, id, int(projectColTitle)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Title", m.inlineInput.Title)
}

func TestProjectHandlerInlineEditSelectColumn(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newProjectHandler()

	m.fs.formData = &projectFormData{
		Title:         "Select Proj",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()

	// Status column opens form overlay.
	require.NoError(t, h.InlineEdit(m, id, int(projectColStatus)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)
}

func TestProjectHandlerInlineEditDateColumn(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newProjectHandler()

	m.fs.formData = &projectFormData{
		Title:         "Date Proj",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()

	// Start date column opens calendar picker.
	require.NoError(t, h.InlineEdit(m, id, int(projectColStart)))
	assert.NotNil(t, m.calendar)
}

func TestProjectHandlerInlineEditIDFallsBackToEditForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newProjectHandler()

	m.fs.formData = &projectFormData{
		Title:         "Fallback Proj",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()

	// ID column falls back to full edit form.
	require.NoError(t, h.InlineEdit(m, id, int(projectColID)))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formProject, m.fs.formKind())
	require.NotNil(t, m.fs.editID)
}

func TestProjectHandlerInlineEditMoneyColumn(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newProjectHandler()

	m.fs.formData = &projectFormData{
		Title:         "Money Proj",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
		Budget:        "1000.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	m.exitForm()

	// Budget column opens inline input.
	require.NoError(t, h.InlineEdit(m, id, int(projectColBudget)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Budget", m.inlineInput.Title)
}

func TestProjectHandlerInlineEditNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newProjectHandler()
	err := h.InlineEdit(m, "01JNOTEXIST000000000099999", int(projectColTitle))
	require.Error(t, err)
}

func TestQuoteHandlerInlineEditColumns(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newQuoteHandler()

	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "InlineQ Proj",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusQuoted,
	}))
	projects, _ := m.store.ListProjects(false)

	m.fs.formData = &quoteFormData{
		ProjectID:  projects[0].ID,
		VendorName: "InlineQuoteCo",
		Total:      "500.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	cases := []struct {
		col       quoteCol
		wantInput bool
		title     string
	}{
		{quoteColProject, false, "Project"},   // select overlay
		{quoteColVendor, true, "Vendor name"}, // inline input
		{quoteColTotal, true, "Total"},
		{quoteColLabor, true, "Labor"},
		{quoteColMat, true, "Materials"},
		{quoteColOther, true, "Other"},
	}
	for _, tc := range cases {
		m.exitForm()
		m.closeInlineInput()
		m.calendar = nil

		require.NoErrorf(t, h.InlineEdit(m, id, int(tc.col)),
			"InlineEdit col %d", tc.col)
		if tc.wantInput {
			require.NotNilf(
				t,
				m.inlineInput,
				"col %d (%s) should open inline input",
				tc.col,
				tc.title,
			)
			assert.Equalf(t, tc.title, m.inlineInput.Title, "col %d title", tc.col)
		} else {
			assert.Nilf(t, m.inlineInput, "col %d (%s) should not open inline input", tc.col, tc.title)
			assert.Equal(t, modeForm, m.mode)
		}
	}

	// Date column opens calendar.
	m.exitForm()
	m.closeInlineInput()
	m.calendar = nil
	require.NoError(t, h.InlineEdit(m, id, int(quoteColRecv)))
	assert.NotNil(t, m.calendar, "Recv column should open calendar")

	// ID column falls back to edit form.
	m.exitForm()
	m.closeInlineInput()
	m.calendar = nil
	require.NoError(t, h.InlineEdit(m, id, int(quoteColID)))
	assert.Equal(t, modeForm, m.mode)
	require.NotNil(t, m.fs.editID)
}

func TestQuoteHandlerInlineEditNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newQuoteHandler()
	err := h.InlineEdit(m, "01JNOTEXIST000000000099999", int(quoteColVendor))
	require.Error(t, err)
}

func TestMaintenanceHandlerInlineEditColumns(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newMaintenanceHandler()
	cats, _ := m.store.MaintenanceCategories()

	m.fs.formData = &maintenanceFormData{
		Name:           "InlineM Item",
		CategoryID:     cats[0].ID,
		ScheduleType:   schedInterval,
		IntervalMonths: "6",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	// Item column opens inline input.
	m.exitForm()
	require.NoError(t, h.InlineEdit(m, id, int(maintenanceColItem)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Item", m.inlineInput.Title)

	// Category column opens form overlay (select).
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(maintenanceColCategory)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)

	// Appliance column opens form overlay (select).
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(maintenanceColAppliance)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)

	// Last serviced column opens calendar.
	m.exitForm()
	m.closeInlineInput()
	m.calendar = nil
	require.NoError(t, h.InlineEdit(m, id, int(maintenanceColLast)))
	assert.NotNil(t, m.calendar)

	// Interval column opens inline input.
	m.exitForm()
	m.closeInlineInput()
	m.calendar = nil
	require.NoError(t, h.InlineEdit(m, id, int(maintenanceColEvery)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Interval", m.inlineInput.Title)

	// ID column falls back to full edit form.
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(maintenanceColID)))
	assert.Equal(t, modeForm, m.mode)
	require.NotNil(t, m.fs.editID)
}

func TestMaintenanceHandlerInlineEditSeason(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newMaintenanceHandler()
	cats, _ := m.store.MaintenanceCategories()

	m.fs.formData = &maintenanceFormData{
		Name:       "InlineSeason Item",
		CategoryID: cats[0].ID,
		Season:     data.SeasonSpring,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	// Season column opens form overlay (select).
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(maintenanceColSeason)))
	assert.Nil(t, m.inlineInput, "season should use form overlay, not inline input")
	assert.Equal(t, modeForm, m.mode)
}

// Step 7: Inline edit "Next" column sets due date via calendar, clears interval,
// and persists the change to the database.
func TestMaintenanceInlineEditNextSetsDueDateAndSaves(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create a maintenance item with an interval via the form.
	m.active = tabIndex(tabMaintenance)
	openAddForm(m)
	values, ok := m.fs.formData.(*maintenanceFormData)
	require.True(t, ok)
	values.Name = "Gutter Check"
	values.ScheduleType = schedInterval
	values.IntervalMonths = "6"
	sendKey(m, "ctrl+s")
	sendKey(m, "esc")

	// Reload and position on the maintenance tab.
	m.reloadAll()
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	require.NotNil(t, tab)
	require.NotEmpty(t, tab.Rows)
	tab.Table.SetCursor(0)
	id := tab.Rows[0].ID

	// Enter edit mode, position on the "Next" column, press 'e'.
	sendKey(m, "i")
	tab.ColCursor = int(maintenanceColNext)
	sendKey(m, "e")

	require.NotNil(t, m.calendar, "Next column should open a date picker")

	// The form data should have switched to due date mode and cleared interval.
	fd, ok := m.fs.formData.(*maintenanceFormData)
	require.True(t, ok)
	assert.Equal(t, schedDueDate, fd.ScheduleType)
	assert.Empty(t, fd.IntervalMonths, "interval should be cleared when editing due date")

	// Pick a date and confirm.
	m.calendar.Cursor = time.Date(2026, 9, 15, 0, 0, 0, 0, time.Local)
	sendKey(m, "enter")
	assert.Nil(t, m.calendar, "calendar should be dismissed after confirm")

	// Verify the DB was updated.
	item, err := m.store.GetMaintenance(id)
	require.NoError(t, err)
	require.NotNil(t, item.DueDate, "due date should be saved")
	assert.Equal(t, "2026-09-15", item.DueDate.Format(data.DateLayout))
	assert.Zero(t, item.IntervalMonths, "interval should be cleared in DB")
}

// Step 8: Inline edit "Every" column sets interval, clears due date,
// and persists the change to the database.
func TestMaintenanceInlineEditEverySetIntervalAndSaves(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create a maintenance item with a due date via the form.
	m.active = tabIndex(tabMaintenance)
	openAddForm(m)
	values, ok := m.fs.formData.(*maintenanceFormData)
	require.True(t, ok)
	values.Name = "Roof Inspect"
	values.ScheduleType = schedDueDate
	values.DueDate = "2025-11-01"
	sendKey(m, "ctrl+s")
	sendKey(m, "esc")

	// Reload and position on the maintenance tab.
	m.reloadAll()
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	require.NotNil(t, tab)
	require.NotEmpty(t, tab.Rows)
	tab.Table.SetCursor(0)
	id := tab.Rows[0].ID

	// Enter edit mode, position on "Every" column, press 'e'.
	sendKey(m, "i")
	tab.ColCursor = int(maintenanceColEvery)
	sendKey(m, "e")

	require.NotNil(t, m.inlineInput, "Every column should open inline input")
	assert.Equal(t, "Interval", m.inlineInput.Title)

	// The form data should have switched to interval mode and cleared due date.
	fd, ok := m.fs.formData.(*maintenanceFormData)
	require.True(t, ok)
	assert.Equal(t, schedInterval, fd.ScheduleType)
	assert.Empty(t, fd.DueDate, "due date should be cleared when editing interval")

	// Type "12" and press enter to save.
	for _, ch := range "12" {
		m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	sendKey(m, "enter")

	// Verify the DB was updated.
	item, err := m.store.GetMaintenance(id)
	require.NoError(t, err)
	assert.Equal(t, 12, item.IntervalMonths, "interval should be saved as 12 months")
	assert.Nil(t, item.DueDate, "due date should be cleared in DB")

	// Verify table display after reload.
	m.reloadAll()
	require.NoError(t, m.reloadActiveTab())
	tab = m.activeTab()
	require.NotEmpty(t, tab.CellRows)
	cells := tab.CellRows[0]
	assert.Equal(t, "1y", cells[int(maintenanceColEvery)].Value)
}

func TestMaintenanceHandlerInlineEditNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newMaintenanceHandler()
	err := h.InlineEdit(m, "01JNOTEXIST000000000099999", int(maintenanceColItem))
	require.Error(t, err)
}

func TestApplianceHandlerInlineEditColumns(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newApplianceHandler()

	m.fs.formData = &applianceFormData{
		Name:  "InlineA Fridge",
		Brand: "LG",
		Cost:  "500.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	textCols := []struct {
		col   applianceCol
		title string
	}{
		{applianceColName, "Name"},
		{applianceColBrand, "Brand"},
		{applianceColModel, "Model number"},
		{applianceColSerial, "Serial number"},
		{applianceColLocation, "Location"},
		{applianceColCost, "Cost"},
	}
	for _, tc := range textCols {
		m.exitForm()
		m.closeInlineInput()
		m.calendar = nil

		require.NoErrorf(t, h.InlineEdit(m, id, int(tc.col)),
			"InlineEdit col %d", tc.col)
		require.NotNilf(t, m.inlineInput, "col %d (%s) should open inline input", tc.col, tc.title)
		assert.Equalf(t, tc.title, m.inlineInput.Title, "col %d title", tc.col)
	}

	// Date columns open calendar.
	for _, col := range []applianceCol{applianceColPurchased, applianceColWarranty} {
		m.exitForm()
		m.closeInlineInput()
		m.calendar = nil
		require.NoErrorf(t, h.InlineEdit(m, id, int(col)),
			"InlineEdit col %d", col)
		assert.NotNilf(t, m.calendar, "col %d should open calendar", col)
	}

	// ID column falls back to full edit form.
	m.exitForm()
	m.closeInlineInput()
	m.calendar = nil
	require.NoError(t, h.InlineEdit(m, id, int(applianceColID)))
	assert.Equal(t, modeForm, m.mode)
	require.NotNil(t, m.fs.editID)
}

func TestApplianceHandlerInlineEditNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newApplianceHandler()
	err := h.InlineEdit(m, "01JNOTEXIST000000000099999", int(applianceColName))
	require.Error(t, err)
}

func TestIncidentHandlerInlineEditColumns(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newIncidentHandler()

	m.fs.formData = &incidentFormData{
		Title:       "Inline Incident",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: "2026-02-10",
		Location:    "Kitchen",
		Cost:        "100.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	// Title opens inline input.
	m.exitForm()
	require.NoError(t, h.InlineEdit(m, id, int(incidentColTitle)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Title", m.inlineInput.Title)

	// Status opens form overlay (select).
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(incidentColStatus)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)

	// Severity opens form overlay (select).
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(incidentColSeverity)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)

	// Location opens inline input.
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(incidentColLocation)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Location", m.inlineInput.Title)

	// Appliance opens form overlay (select).
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(incidentColAppliance)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)

	// Vendor opens form overlay (select).
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(incidentColVendor)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)

	// Date columns open calendar.
	for _, col := range []incidentCol{incidentColNoticed, incidentColResolved} {
		m.exitForm()
		m.closeInlineInput()
		m.calendar = nil
		require.NoErrorf(t, h.InlineEdit(m, id, int(col)),
			"InlineEdit col %d", col)
		assert.NotNilf(t, m.calendar, "col %d should open calendar", col)
	}

	// Cost opens inline input.
	m.exitForm()
	m.closeInlineInput()
	m.calendar = nil
	require.NoError(t, h.InlineEdit(m, id, int(incidentColCost)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Cost", m.inlineInput.Title)

	// ID column falls back to edit form.
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(incidentColID)))
	assert.Equal(t, modeForm, m.mode)
	require.NotNil(t, m.fs.editID)
}

func TestIncidentHandlerInlineEditNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newIncidentHandler()
	err := h.InlineEdit(m, "01JNOTEXIST000000000099999", int(incidentColTitle))
	require.Error(t, err)
}

func TestServiceLogHandlerInlineEditColumns(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "InlineL Item",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	h := newServiceLogHandler(maintID)
	m.fs.formData = &serviceLogFormData{
		MaintenanceItemID: maintID,
		ServicedAt:        "2026-01-15",
		Cost:              "50.00",
		Notes:             "inline test",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	// Date opens calendar.
	m.exitForm()
	m.calendar = nil
	require.NoError(t, h.InlineEdit(m, id, int(serviceLogColDate)))
	assert.NotNil(t, m.calendar)

	// Performed by opens form overlay (select).
	m.exitForm()
	m.closeInlineInput()
	m.calendar = nil
	require.NoError(t, h.InlineEdit(m, id, int(serviceLogColPerformedBy)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)

	// Cost opens inline input.
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(serviceLogColCost)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Cost", m.inlineInput.Title)

	// Notes opens notes edit (textarea).
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(serviceLogColNotes)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)
	assert.True(t, m.fs.notesEditMode)

	// ID column falls back to edit form.
	m.exitForm()
	m.closeInlineInput()
	m.fs.notesEditMode = false
	require.NoError(t, h.InlineEdit(m, id, int(serviceLogColID)))
	assert.Equal(t, modeForm, m.mode)
	require.NotNil(t, m.fs.editID)
}

func TestServiceLogHandlerInlineEditNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newServiceLogHandler("01JTEST00000000000000001")
	err := h.InlineEdit(m, "01JNOTEXIST000000000099999", int(serviceLogColCost))
	require.Error(t, err)
}

func TestVendorHandlerInlineEditColumns(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newVendorHandler()

	m.fs.formData = &vendorFormData{
		Name:    "InlineV Vendor",
		Email:   "test@test.com",
		Phone:   "555-1234",
		Website: "https://test.com",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false) //nolint:dogsled // Load returns 4 values by design
	id := meta[0].ID

	textCols := []struct {
		col   vendorCol
		title string
	}{
		{vendorColName, "Name"},
		{vendorColContact, "Contact name"},
		{vendorColEmail, "Email"},
		{vendorColPhone, "Phone"},
		{vendorColWebsite, "Website"},
	}
	for _, tc := range textCols {
		m.exitForm()
		m.closeInlineInput()

		require.NoErrorf(t, h.InlineEdit(m, id, int(tc.col)),
			"InlineEdit col %d", tc.col)
		require.NotNilf(t, m.inlineInput, "col %d (%s) should open inline input", tc.col, tc.title)
		assert.Equalf(t, tc.title, m.inlineInput.Title, "col %d title", tc.col)
	}

	// ID column falls back to full edit form.
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(vendorColID)))
	assert.Equal(t, modeForm, m.mode)
	require.NotNil(t, m.fs.editID)
}

func TestVendorHandlerInlineEditNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newVendorHandler()
	err := h.InlineEdit(m, "01JNOTEXIST000000000099999", int(vendorColName))
	require.Error(t, err)
}

func TestDocumentHandlerInlineEditColumns(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newDocumentHandler()

	require.NoError(t, m.store.CreateDocument(&data.Document{
		Title: "InlineD Doc",
		Notes: "some notes",
	}))
	docs, _ := m.store.ListDocuments(false)
	id := docs[0].ID

	// Title opens inline input.
	require.NoError(t, h.InlineEdit(m, id, int(documentColTitle)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Title", m.inlineInput.Title)

	// Notes opens notes edit (textarea).
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(documentColNotes)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)
	assert.True(t, m.fs.notesEditMode)

	// Entity opens form overlay (select).
	m.exitForm()
	m.closeInlineInput()
	m.fs.notesEditMode = false
	require.NoError(t, h.InlineEdit(m, id, int(documentColEntity)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)

	// ID column falls back to edit form.
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(documentColID)))
	assert.Equal(t, modeForm, m.mode)
	require.NotNil(t, m.fs.editID)
}

func TestDocumentHandlerInlineEditNonExistent(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newDocumentHandler()
	err := h.InlineEdit(m, "01JNOTEXIST000000000099999", int(documentColTitle))
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// entityDocumentHandler (scoped) tests
// ---------------------------------------------------------------------------

func TestEntityDocumentHandlerStartAddForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Doc Parent Proj",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, _ := m.store.ListProjects(false)
	projID := projects[0].ID

	h := newEntityDocumentHandler(data.DocumentEntityProject, projID)
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formDocument, m.fs.formKind())
}

func TestEntityDocumentHandlerStartEditForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Doc Edit Parent",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, _ := m.store.ListProjects(false)
	projID := projects[0].ID

	require.NoError(t, m.store.CreateDocument(&data.Document{
		Title:      "Scoped Doc",
		EntityKind: data.DocumentEntityProject,
		EntityID:   projID,
	}))
	docs, _ := m.store.ListDocuments(false)
	docID := docs[0].ID

	h := newEntityDocumentHandler(data.DocumentEntityProject, projID)
	require.NoError(t, h.StartEditForm(m, docID))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formDocument, m.fs.formKind())
	require.NotNil(t, m.fs.editID)
	assert.Equal(t, docID, *m.fs.editID)
}

func TestEntityDocumentHandlerInlineEditSkipsEntityColumn(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Skip Entity Proj",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, _ := m.store.ListProjects(false)
	projID := projects[0].ID

	require.NoError(t, m.store.CreateDocument(&data.Document{
		Title:      "Skip Col Doc",
		EntityKind: data.DocumentEntityProject,
		EntityID:   projID,
	}))
	docs, _ := m.store.ListDocuments(false)
	docID := docs[0].ID

	h := newEntityDocumentHandler(data.DocumentEntityProject, projID)

	// In the entity document view, col 1 is Title (Entity column is removed).
	// Col 0 is ID, col 1 is Title, col 2 is Type (skips Entity).
	require.NoError(t, h.InlineEdit(m, docID, 1))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Title", m.inlineInput.Title)
}

// ---------------------------------------------------------------------------
// SyncFixedValues no-op handlers (handlers with empty FixedValues)
// ---------------------------------------------------------------------------

func TestSyncFixedValuesNoOp(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		h    TabHandler
	}{
		{"quote", newQuoteHandler()},
		{"appliance", newApplianceHandler()},
		{"vendor", newVendorHandler()},
		{"serviceLog", newServiceLogHandler("")},
		{"document", newDocumentHandler()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModelWithStore(t)
			specs := []columnSpec{{Title: "Col"}}
			tc.h.SyncFixedValues(m, specs)
			assert.Empty(t, specs[0].FixedValues)
		})
	}
}

// ---------------------------------------------------------------------------
// scopedHandler tests
// ---------------------------------------------------------------------------

func TestVendorJobsHandlerStartAddFormReturnsError(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := newVendorJobsHandler("01JTEST00000000000000001")
	err := h.StartAddForm(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Maintenance")
}

func TestSkipColEditRemapsIndices(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Skip Col Proj",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusQuoted,
	}))
	projects, _ := m.store.ListProjects(false)

	// Create a quote so we can inline edit it.
	m.fs.formData = &quoteFormData{
		ProjectID:  projects[0].ID,
		VendorName: "SkipCo",
		Total:      "300.00",
	}
	require.NoError(t, (newQuoteHandler()).SubmitForm(m))
	_, meta, _, _ := (newQuoteHandler()).Load(
		m.store,
		false,
	) //nolint:dogsled // Load returns 4 values by design
	quoteID := meta[0].ID

	// Project quote handler skips Project column (col 1).
	h := newProjectQuoteHandler(projects[0].ID)

	// Col 0 in the scoped view = ID (maps to full col 0).
	// Col 1 = Vendor (maps to full col 2, skipping Project).
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, quoteID, 1))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Vendor name", m.inlineInput.Title)
}

// ---------------------------------------------------------------------------
// Edit form populates formData correctly
// ---------------------------------------------------------------------------

func TestStartEditIncidentFormPopulatesAllFields(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "Test App"}))
	apps, _ := m.store.ListAppliances(false)
	appID := apps[0].ID

	require.NoError(t, m.store.CreateVendor(&data.Vendor{Name: "Test Vendor"}))
	vendors, _ := m.store.ListVendors(false)
	vendorID := vendors[0].ID
	// Refresh model's vendor cache.
	m.vendors = vendors

	cost := int64(25000)
	noticed := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	resolved := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.store.CreateIncident(&data.Incident{
		Title:        "Full Incident",
		Description:  "test desc",
		Status:       data.IncidentStatusInProgress,
		Severity:     data.IncidentSeverityUrgent,
		DateNoticed:  noticed,
		DateResolved: &resolved,
		Location:     "Basement",
		CostCents:    &cost,
		ApplianceID:  &appID,
		VendorID:     &vendorID,
		Notes:        "test notes",
	}))
	incidents, _ := m.store.ListIncidents(false)
	id := incidents[0].ID

	h := newIncidentHandler()
	require.NoError(t, h.StartEditForm(m, id))
	fd, ok := m.fs.formData.(*incidentFormData)
	require.True(t, ok)
	assert.Equal(t, "Full Incident", fd.Title)
	assert.Equal(t, "test desc", fd.Description)
	assert.Equal(t, data.IncidentStatusInProgress, fd.Status)
	assert.Equal(t, data.IncidentSeverityUrgent, fd.Severity)
	assert.Equal(t, "2026-02-01", fd.DateNoticed)
	assert.Equal(t, "2026-02-15", fd.DateResolved)
	assert.Equal(t, "Basement", fd.Location)
	assert.Equal(t, "$250.00", fd.Cost)
	assert.Equal(t, appID, fd.ApplianceID)
	assert.Equal(t, vendorID, fd.VendorID)
	assert.Equal(t, "test notes", fd.Notes)
}

func TestStartEditServiceLogFormPopulatesAllFields(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateVendor(&data.Vendor{Name: "Log Vendor"}))
	vendors, _ := m.store.ListVendors(false)
	m.vendors = vendors

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Full Log Item",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	cost := int64(7500)
	require.NoError(t, m.store.CreateServiceLog(
		&data.ServiceLogEntry{
			MaintenanceItemID: maintID,
			ServicedAt:        time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
			CostCents:         &cost,
			Notes:             "full notes",
		},
		vendors[0],
	))
	entries, _ := m.store.ListServiceLog(maintID, false)
	id := entries[0].ID

	h := newServiceLogHandler(maintID)
	require.NoError(t, h.StartEditForm(m, id))
	fd, ok := m.fs.formData.(*serviceLogFormData)
	require.True(t, ok)
	assert.Equal(t, maintID, fd.MaintenanceItemID)
	assert.Equal(t, "2026-01-20", fd.ServicedAt)
	assert.Equal(t, "$75.00", fd.Cost)
	assert.Equal(t, "full notes", fd.Notes)
}

func TestStartEditApplianceFormPopulatesAllFields(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	cost := int64(89900)
	purchased := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	warranty := time.Date(2027, 6, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{
		Name:           "Full Fridge",
		Brand:          "Samsung",
		ModelNumber:    "RF28",
		SerialNumber:   "SN123",
		PurchaseDate:   &purchased,
		WarrantyExpiry: &warranty,
		Location:       "Kitchen",
		CostCents:      &cost,
		Notes:          "appliance notes",
	}))
	apps, _ := m.store.ListAppliances(false)
	id := apps[0].ID

	h := newApplianceHandler()
	require.NoError(t, h.StartEditForm(m, id))
	fd, ok := m.fs.formData.(*applianceFormData)
	require.True(t, ok)
	assert.Equal(t, "Full Fridge", fd.Name)
	assert.Equal(t, "Samsung", fd.Brand)
	assert.Equal(t, "RF28", fd.ModelNumber)
	assert.Equal(t, "SN123", fd.SerialNumber)
	assert.Equal(t, "2024-06-15", fd.PurchaseDate)
	assert.Equal(t, "2027-06-15", fd.WarrantyExpiry)
	assert.Equal(t, "Kitchen", fd.Location)
	assert.Equal(t, "$899.00", fd.Cost)
	assert.Equal(t, "appliance notes", fd.Notes)
}

func TestStartEditMaintenanceFormPopulatesAllFields(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "Maint App"}))
	apps, _ := m.store.ListAppliances(false)
	appID := apps[0].ID

	lastSrv := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cost := int64(12500)
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:           "Full Maint",
		CategoryID:     cats[0].ID,
		ApplianceID:    &appID,
		LastServicedAt: &lastSrv,
		IntervalMonths: 6,
		ManualURL:      "https://manual.com",
		ManualText:     "manual text",
		CostCents:      &cost,
		Notes:          "maint notes",
	}))
	items, _ := m.store.ListMaintenance(false)
	id := items[0].ID

	h := newMaintenanceHandler()
	require.NoError(t, h.StartEditForm(m, id))
	fd, ok := m.fs.formData.(*maintenanceFormData)
	require.True(t, ok)
	assert.Equal(t, "Full Maint", fd.Name)
	assert.Equal(t, cats[0].ID, fd.CategoryID)
	assert.Equal(t, appID, fd.ApplianceID)
	assert.Equal(t, schedInterval, fd.ScheduleType)
	assert.Equal(t, "2026-01-01", fd.LastServiced)
	assert.Equal(t, "6m", fd.IntervalMonths)
	assert.Equal(t, "https://manual.com", fd.ManualURL)
	assert.Equal(t, "manual text", fd.ManualText)
	assert.Equal(t, "$125.00", fd.Cost)
	assert.Equal(t, "maint notes", fd.Notes)
}
