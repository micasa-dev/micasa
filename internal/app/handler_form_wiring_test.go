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

// ---------------------------------------------------------------------------
// StartAddForm tests
// ---------------------------------------------------------------------------

func TestProjectHandlerStartAddForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formProject, m.formKind)
	assert.IsType(t, &projectFormData{}, m.formData)
	assert.NotNil(t, m.form)
}

func TestQuoteHandlerStartAddFormRequiresProject(t *testing.T) {
	m := newTestModelWithStore(t)
	h := quoteHandler{}
	err := h.StartAddForm(m)
	require.Error(t, err, "should fail without projects")
	assert.Contains(t, err.Error(), "project")
}

func TestQuoteHandlerStartAddFormWithProject(t *testing.T) {
	m := newTestModelWithStore(t)
	h := quoteHandler{}

	// Create a project first.
	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Quote Test Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))

	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formQuote, m.formKind)
	assert.IsType(t, &quoteFormData{}, m.formData)
}

func TestMaintenanceHandlerStartAddForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formMaintenance, m.formKind)
	assert.IsType(t, &maintenanceFormData{}, m.formData)
}

func TestApplianceHandlerStartAddForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := applianceHandler{}
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formAppliance, m.formKind)
	assert.IsType(t, &applianceFormData{}, m.formData)
}

func TestVendorHandlerStartAddForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := vendorHandler{}
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formVendor, m.formKind)
	assert.IsType(t, &vendorFormData{}, m.formData)
}

func TestIncidentHandlerStartAddForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := incidentHandler{}
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formIncident, m.formKind)
	assert.IsType(t, &incidentFormData{}, m.formData)
}

func TestServiceLogHandlerStartAddForm(t *testing.T) {
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Test Item",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	h := serviceLogHandler{maintenanceItemID: maintID}
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formServiceLog, m.formKind)
	assert.IsType(t, &serviceLogFormData{}, m.formData)
	fd, ok := m.formData.(*serviceLogFormData)
	require.True(t, ok, "formData should be *serviceLogFormData")
	assert.Equal(t, maintID, fd.MaintenanceItemID)
}

func TestDocumentHandlerStartAddForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := documentHandler{}
	require.NoError(t, h.StartAddForm(m))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formDocument, m.formKind)
	assert.IsType(t, &documentFormData{}, m.formData)
}

// ---------------------------------------------------------------------------
// StartEditForm tests
// ---------------------------------------------------------------------------

func TestProjectHandlerStartEditForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}

	m.formData = &projectFormData{
		Title:         "Edit Me",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formProject, m.formKind)
	require.NotNil(t, m.editID)
	assert.Equal(t, id, *m.editID)
	fd, ok := m.formData.(*projectFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Me", fd.Title)
}

func TestProjectHandlerStartEditFormNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}
	err := h.StartEditForm(m, 99999)
	require.Error(t, err)
}

func TestQuoteHandlerStartEditForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := quoteHandler{}

	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Quote Edit Proj",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusQuoted,
	}))
	projects, _ := m.store.ListProjects(false)

	m.formData = &quoteFormData{
		ProjectID:  projects[0].ID,
		VendorName: "EditQuoteCo",
		Total:      "800.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formQuote, m.formKind)
	require.NotNil(t, m.editID)
	assert.Equal(t, id, *m.editID)
	fd, ok := m.formData.(*quoteFormData)
	require.True(t, ok)
	assert.Equal(t, "EditQuoteCo", fd.VendorName)
}

func TestQuoteHandlerStartEditFormNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := quoteHandler{}
	err := h.StartEditForm(m, 99999)
	require.Error(t, err)
}

func TestMaintenanceHandlerStartEditForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	cats, _ := m.store.MaintenanceCategories()

	m.formData = &maintenanceFormData{
		Name:       "Edit Maint",
		CategoryID: cats[0].ID,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formMaintenance, m.formKind)
	require.NotNil(t, m.editID)
	assert.Equal(t, id, *m.editID)
	fd, ok := m.formData.(*maintenanceFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Maint", fd.Name)
}

func TestMaintenanceHandlerStartEditFormNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	err := h.StartEditForm(m, 99999)
	require.Error(t, err)
}

func TestApplianceHandlerStartEditForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := applianceHandler{}

	m.formData = &applianceFormData{Name: "Edit Fridge"}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formAppliance, m.formKind)
	require.NotNil(t, m.editID)
	assert.Equal(t, id, *m.editID)
	fd, ok := m.formData.(*applianceFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Fridge", fd.Name)
}

func TestApplianceHandlerStartEditFormNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := applianceHandler{}
	err := h.StartEditForm(m, 99999)
	require.Error(t, err)
}

func TestVendorHandlerStartEditForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := vendorHandler{}

	m.formData = &vendorFormData{Name: "Edit Vendor Co"}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formVendor, m.formKind)
	require.NotNil(t, m.editID)
	assert.Equal(t, id, *m.editID)
	fd, ok := m.formData.(*vendorFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Vendor Co", fd.Name)
}

func TestVendorHandlerStartEditFormNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := vendorHandler{}
	err := h.StartEditForm(m, 99999)
	require.Error(t, err)
}

func TestIncidentHandlerStartEditForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := incidentHandler{}

	m.formData = &incidentFormData{
		Title:       "Edit Incident",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: "2026-02-01",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formIncident, m.formKind)
	require.NotNil(t, m.editID)
	assert.Equal(t, id, *m.editID)
	fd, ok := m.formData.(*incidentFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Incident", fd.Title)
}

func TestIncidentHandlerStartEditFormNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := incidentHandler{}
	err := h.StartEditForm(m, 99999)
	require.Error(t, err)
}

func TestServiceLogHandlerStartEditForm(t *testing.T) {
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Log Edit Item",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	h := serviceLogHandler{maintenanceItemID: maintID}
	m.formData = &serviceLogFormData{
		MaintenanceItemID: maintID,
		ServicedAt:        "2026-01-20",
		Notes:             "test log",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()
	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formServiceLog, m.formKind)
	require.NotNil(t, m.editID)
	assert.Equal(t, id, *m.editID)
	fd, ok := m.formData.(*serviceLogFormData)
	require.True(t, ok)
	assert.Equal(t, "test log", fd.Notes)
}

func TestServiceLogHandlerStartEditFormNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := serviceLogHandler{maintenanceItemID: 1}
	err := h.StartEditForm(m, 99999)
	require.Error(t, err)
}

func TestDocumentHandlerStartEditForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := documentHandler{}

	// Create a document directly via store.
	require.NoError(t, m.store.CreateDocument(&data.Document{
		Title: "Edit Doc",
		Notes: "doc notes",
	}))
	docs, _ := m.store.ListDocuments(false)
	id := docs[0].ID

	require.NoError(t, h.StartEditForm(m, id))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formDocument, m.formKind)
	require.NotNil(t, m.editID)
	assert.Equal(t, id, *m.editID)
	fd, ok := m.formData.(*documentFormData)
	require.True(t, ok)
	assert.Equal(t, "Edit Doc", fd.Title)
}

func TestDocumentHandlerStartEditFormNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := documentHandler{}
	err := h.StartEditForm(m, 99999)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// InlineEdit tests
// ---------------------------------------------------------------------------

func TestProjectHandlerInlineEditTextColumn(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}

	m.formData = &projectFormData{
		Title:         "Inline Proj",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()

	// Title column opens inline input.
	require.NoError(t, h.InlineEdit(m, id, int(projectColTitle)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Title", m.inlineInput.Title)
}

func TestProjectHandlerInlineEditSelectColumn(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}

	m.formData = &projectFormData{
		Title:         "Select Proj",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()

	// Status column opens form overlay.
	require.NoError(t, h.InlineEdit(m, id, int(projectColStatus)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)
}

func TestProjectHandlerInlineEditDateColumn(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}

	m.formData = &projectFormData{
		Title:         "Date Proj",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()

	// Start date column opens calendar picker.
	require.NoError(t, h.InlineEdit(m, id, int(projectColStart)))
	assert.NotNil(t, m.calendar)
}

func TestProjectHandlerInlineEditIDFallsBackToEditForm(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}

	m.formData = &projectFormData{
		Title:         "Fallback Proj",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()

	// ID column falls back to full edit form.
	require.NoError(t, h.InlineEdit(m, id, int(projectColID)))
	assert.Equal(t, modeForm, m.mode)
	assert.Equal(t, formProject, m.formKind)
	require.NotNil(t, m.editID)
}

func TestProjectHandlerInlineEditMoneyColumn(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}

	m.formData = &projectFormData{
		Title:         "Money Proj",
		ProjectTypeID: m.projectTypes[0].ID,
		Status:        data.ProjectStatusPlanned,
		Budget:        "1000.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
	id := meta[0].ID

	m.exitForm()

	// Budget column opens inline input.
	require.NoError(t, h.InlineEdit(m, id, int(projectColBudget)))
	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Budget", m.inlineInput.Title)
}

func TestProjectHandlerInlineEditNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := projectHandler{}
	err := h.InlineEdit(m, 99999, int(projectColTitle))
	require.Error(t, err)
}

func TestQuoteHandlerInlineEditColumns(t *testing.T) {
	m := newTestModelWithStore(t)
	h := quoteHandler{}

	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "InlineQ Proj",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusQuoted,
	}))
	projects, _ := m.store.ListProjects(false)

	m.formData = &quoteFormData{
		ProjectID:  projects[0].ID,
		VendorName: "InlineQuoteCo",
		Total:      "500.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
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
	require.NotNil(t, m.editID)
}

func TestQuoteHandlerInlineEditNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := quoteHandler{}
	err := h.InlineEdit(m, 99999, int(quoteColVendor))
	require.Error(t, err)
}

func TestMaintenanceHandlerInlineEditColumns(t *testing.T) {
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	cats, _ := m.store.MaintenanceCategories()

	m.formData = &maintenanceFormData{
		Name:           "InlineM Item",
		CategoryID:     cats[0].ID,
		IntervalMonths: "6",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
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
	require.NotNil(t, m.editID)
}

func TestMaintenanceHandlerInlineEditNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := maintenanceHandler{}
	err := h.InlineEdit(m, 99999, int(maintenanceColItem))
	require.Error(t, err)
}

func TestApplianceHandlerInlineEditColumns(t *testing.T) {
	m := newTestModelWithStore(t)
	h := applianceHandler{}

	m.formData = &applianceFormData{
		Name:  "InlineA Fridge",
		Brand: "LG",
		Cost:  "500.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
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
	require.NotNil(t, m.editID)
}

func TestApplianceHandlerInlineEditNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := applianceHandler{}
	err := h.InlineEdit(m, 99999, int(applianceColName))
	require.Error(t, err)
}

func TestIncidentHandlerInlineEditColumns(t *testing.T) {
	m := newTestModelWithStore(t)
	h := incidentHandler{}

	m.formData = &incidentFormData{
		Title:       "Inline Incident",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: "2026-02-10",
		Location:    "Kitchen",
		Cost:        "100.00",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
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
	require.NotNil(t, m.editID)
}

func TestIncidentHandlerInlineEditNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := incidentHandler{}
	err := h.InlineEdit(m, 99999, int(incidentColTitle))
	require.Error(t, err)
}

func TestServiceLogHandlerInlineEditColumns(t *testing.T) {
	m := newTestModelWithStore(t)
	cats, _ := m.store.MaintenanceCategories()

	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "InlineL Item",
		CategoryID: cats[0].ID,
	}))
	items, _ := m.store.ListMaintenance(false)
	maintID := items[0].ID

	h := serviceLogHandler{maintenanceItemID: maintID}
	m.formData = &serviceLogFormData{
		MaintenanceItemID: maintID,
		ServicedAt:        "2026-01-15",
		Cost:              "50.00",
		Notes:             "inline test",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
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
	assert.True(t, m.notesEditMode)

	// ID column falls back to edit form.
	m.exitForm()
	m.closeInlineInput()
	m.notesEditMode = false
	require.NoError(t, h.InlineEdit(m, id, int(serviceLogColID)))
	assert.Equal(t, modeForm, m.mode)
	require.NotNil(t, m.editID)
}

func TestServiceLogHandlerInlineEditNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := serviceLogHandler{maintenanceItemID: 1}
	err := h.InlineEdit(m, 99999, int(serviceLogColCost))
	require.Error(t, err)
}

func TestVendorHandlerInlineEditColumns(t *testing.T) {
	m := newTestModelWithStore(t)
	h := vendorHandler{}

	m.formData = &vendorFormData{
		Name:    "InlineV Vendor",
		Email:   "test@test.com",
		Phone:   "555-1234",
		Website: "https://test.com",
	}
	require.NoError(t, h.SubmitForm(m))
	_, meta, _, _ := h.Load(m.store, false)
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
	require.NotNil(t, m.editID)
}

func TestVendorHandlerInlineEditNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := vendorHandler{}
	err := h.InlineEdit(m, 99999, int(vendorColName))
	require.Error(t, err)
}

func TestDocumentHandlerInlineEditColumns(t *testing.T) {
	m := newTestModelWithStore(t)
	h := documentHandler{}

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
	assert.True(t, m.notesEditMode)

	// Entity opens form overlay (select).
	m.exitForm()
	m.closeInlineInput()
	m.notesEditMode = false
	require.NoError(t, h.InlineEdit(m, id, int(documentColEntity)))
	assert.Nil(t, m.inlineInput)
	assert.Equal(t, modeForm, m.mode)

	// ID column falls back to edit form.
	m.exitForm()
	m.closeInlineInput()
	require.NoError(t, h.InlineEdit(m, id, int(documentColID)))
	assert.Equal(t, modeForm, m.mode)
	require.NotNil(t, m.editID)
}

func TestDocumentHandlerInlineEditNonExistent(t *testing.T) {
	m := newTestModelWithStore(t)
	h := documentHandler{}
	err := h.InlineEdit(m, 99999, int(documentColTitle))
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// entityDocumentHandler (scoped) tests
// ---------------------------------------------------------------------------

func TestEntityDocumentHandlerStartAddForm(t *testing.T) {
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
	assert.Equal(t, formDocument, m.formKind)
}

func TestEntityDocumentHandlerStartEditForm(t *testing.T) {
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
	assert.Equal(t, formDocument, m.formKind)
	require.NotNil(t, m.editID)
	assert.Equal(t, docID, *m.editID)
}

func TestEntityDocumentHandlerInlineEditSkipsEntityColumn(t *testing.T) {
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
// SyncFixedValues tests for handlers that have non-trivial implementations
// ---------------------------------------------------------------------------

func TestIncidentHandlerSyncFixedValuesSetsStatusAndSeverity(t *testing.T) {
	m := newTestModelWithStore(t)
	h := incidentHandler{}
	specs := []columnSpec{
		{Title: "ID"},
		{Title: "Title"},
		{Title: "Status"},
		{Title: "Severity"},
	}
	h.SyncFixedValues(m, specs)

	require.NotEmpty(t, specs[2].FixedValues)
	assert.Contains(t, specs[2].FixedValues, data.IncidentStatusOpen)
	assert.Contains(t, specs[2].FixedValues, data.IncidentStatusInProgress)

	require.NotEmpty(t, specs[3].FixedValues)
	assert.Contains(t, specs[3].FixedValues, data.IncidentSeverityUrgent)
	assert.Contains(t, specs[3].FixedValues, data.IncidentSeveritySoon)
	assert.Contains(t, specs[3].FixedValues, data.IncidentSeverityWhenever)
}

func TestQuoteHandlerSyncFixedValuesNoOp(t *testing.T) {
	m := newTestModelWithStore(t)
	h := quoteHandler{}
	specs := []columnSpec{{Title: "Total"}}
	h.SyncFixedValues(m, specs)
	assert.Empty(t, specs[0].FixedValues)
}

func TestApplianceHandlerSyncFixedValuesNoOp(t *testing.T) {
	m := newTestModelWithStore(t)
	h := applianceHandler{}
	specs := []columnSpec{{Title: "Name"}}
	h.SyncFixedValues(m, specs)
	assert.Empty(t, specs[0].FixedValues)
}

func TestVendorHandlerSyncFixedValuesNoOp(t *testing.T) {
	m := newTestModelWithStore(t)
	h := vendorHandler{}
	specs := []columnSpec{{Title: "Name"}}
	h.SyncFixedValues(m, specs)
	assert.Empty(t, specs[0].FixedValues)
}

func TestServiceLogHandlerSyncFixedValuesNoOp(t *testing.T) {
	m := newTestModelWithStore(t)
	h := serviceLogHandler{}
	specs := []columnSpec{{Title: "Date"}}
	h.SyncFixedValues(m, specs)
	assert.Empty(t, specs[0].FixedValues)
}

func TestDocumentHandlerSyncFixedValuesNoOp(t *testing.T) {
	m := newTestModelWithStore(t)
	h := documentHandler{}
	specs := []columnSpec{{Title: "Title"}}
	h.SyncFixedValues(m, specs)
	assert.Empty(t, specs[0].FixedValues)
}

// ---------------------------------------------------------------------------
// scopedHandler tests
// ---------------------------------------------------------------------------

func TestVendorJobsHandlerStartAddFormReturnsError(t *testing.T) {
	m := newTestModelWithStore(t)
	h := newVendorJobsHandler(1)
	err := h.StartAddForm(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Maintenance")
}

func TestSkipColEditRemapsIndices(t *testing.T) {
	m := newTestModelWithStore(t)
	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Skip Col Proj",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusQuoted,
	}))
	projects, _ := m.store.ListProjects(false)

	// Create a quote so we can inline edit it.
	m.formData = &quoteFormData{
		ProjectID:  projects[0].ID,
		VendorName: "SkipCo",
		Total:      "300.00",
	}
	require.NoError(t, (quoteHandler{}).SubmitForm(m))
	_, meta, _, _ := (quoteHandler{}).Load(m.store, false)
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

	h := incidentHandler{}
	require.NoError(t, h.StartEditForm(m, id))
	fd, ok := m.formData.(*incidentFormData)
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

	h := serviceLogHandler{maintenanceItemID: maintID}
	require.NoError(t, h.StartEditForm(m, id))
	fd, ok := m.formData.(*serviceLogFormData)
	require.True(t, ok)
	assert.Equal(t, maintID, fd.MaintenanceItemID)
	assert.Equal(t, "2026-01-20", fd.ServicedAt)
	assert.Equal(t, "$75.00", fd.Cost)
	assert.Equal(t, "full notes", fd.Notes)
}

func TestStartEditApplianceFormPopulatesAllFields(t *testing.T) {
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

	h := applianceHandler{}
	require.NoError(t, h.StartEditForm(m, id))
	fd, ok := m.formData.(*applianceFormData)
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

	h := maintenanceHandler{}
	require.NoError(t, h.StartEditForm(m, id))
	fd, ok := m.formData.(*maintenanceFormData)
	require.True(t, ok)
	assert.Equal(t, "Full Maint", fd.Name)
	assert.Equal(t, cats[0].ID, fd.CategoryID)
	assert.Equal(t, appID, fd.ApplianceID)
	assert.Equal(t, "2026-01-01", fd.LastServiced)
	assert.Equal(t, "6m", fd.IntervalMonths)
	assert.Equal(t, "https://manual.com", fd.ManualURL)
	assert.Equal(t, "manual text", fd.ManualText)
	assert.Equal(t, "$125.00", fd.Cost)
	assert.Equal(t, "maint notes", fd.Notes)
}
