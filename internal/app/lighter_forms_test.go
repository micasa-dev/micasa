// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// formFieldLabels initializes the form and returns the rendered view text.
// Callers check for presence/absence of field labels.
func formFieldLabels(m *Model) string {
	if m.form == nil {
		return ""
	}
	m.form.Init()
	return m.form.View()
}

func TestSaveFormFocusesNewItem(t *testing.T) {
	m := newTestModelWithStore(t)

	// Create first project and check cursor lands on it.
	m.startProjectForm()
	m.form.Init()
	v1, ok := m.formData.(*projectFormData)
	require.True(t, ok)
	v1.Title = "First"
	m.saveForm()

	meta, ok := m.selectedRowMeta()
	require.True(t, ok, "should have a selected row after creating first project")
	firstID := meta.ID

	// Create second project; cursor should move to the new item.
	m.startProjectForm()
	m.form.Init()
	v2, ok := m.formData.(*projectFormData)
	require.True(t, ok)
	v2.Title = "Second"
	m.saveForm()

	meta, ok = m.selectedRowMeta()
	require.True(t, ok, "should have a selected row after creating second project")
	assert.NotEqual(t, firstID, meta.ID,
		"cursor should move to the newly created item, not stay on the first")
}

// TestSaveFormInPlaceThenEscFocusesNewItem verifies the Ctrl+S → Esc flow:
// creating an item via save-in-place, then aborting the form, should leave
// the cursor on the newly created item.
func TestSaveFormInPlaceThenEscFocusesNewItem(t *testing.T) {
	m := newTestModelWithStore(t)

	// Seed an existing project so the cursor starts on something else.
	m.startProjectForm()
	m.form.Init()
	v1, ok := m.formData.(*projectFormData)
	require.True(t, ok)
	v1.Title = "Existing"
	m.saveForm()

	existingMeta, ok := m.selectedRowMeta()
	require.True(t, ok)
	existingID := existingMeta.ID

	// Start a new add form, save in place (Ctrl+S), then exit (Esc).
	m.startProjectForm()
	m.form.Init()
	v2, ok := m.formData.(*projectFormData)
	require.True(t, ok)
	v2.Title = "Via CtrlS"
	m.saveFormInPlace()
	require.NotNil(t, m.editID, "editID should be set after save-in-place create")
	newID := *m.editID

	// Simulate Esc — form is clean after snapshotForm, so exitForm fires.
	m.exitForm()

	meta, ok := m.selectedRowMeta()
	require.True(t, ok, "should have a selected row after Ctrl+S then Esc")
	assert.Equal(t, newID, meta.ID, "cursor should be on the newly created item")
	assert.NotEqual(t, existingID, meta.ID, "cursor should not stay on the old item")
}

// TestSaveFormInPlaceThenDiscardFocusesNewItem verifies the Ctrl+S → edit
// more → Esc → confirm discard "y" flow.
func TestSaveFormInPlaceThenDiscardFocusesNewItem(t *testing.T) {
	m := newTestModelWithStore(t)

	// Seed an existing project.
	m.startProjectForm()
	m.form.Init()
	v1, ok := m.formData.(*projectFormData)
	require.True(t, ok)
	v1.Title = "Existing"
	m.saveForm()

	// Start add form, save in place, then make the form dirty.
	m.startProjectForm()
	m.form.Init()
	v2, ok := m.formData.(*projectFormData)
	require.True(t, ok)
	v2.Title = "Saved InPlace"
	m.saveFormInPlace()
	require.NotNil(t, m.editID)
	newID := *m.editID

	// Mutate form data after snapshot to make it dirty.
	v2.Title = "Unsaved Change"
	m.checkFormDirty()
	require.True(t, m.formDirty, "form should be dirty after mutation")

	// Simulate confirm-discard "y".
	m.confirmDiscard = true
	m.handleConfirmDiscard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	meta, ok := m.selectedRowMeta()
	require.True(t, ok, "should have a selected row after discard")
	assert.Equal(t, newID, meta.ID, "cursor should be on the saved item, not the old one")
}

// TestSaveFormInPlaceTwiceThenEscFocusesItem verifies that Ctrl+S twice
// (create then update) followed by Esc still lands on the item.
func TestSaveFormInPlaceTwiceThenEscFocusesItem(t *testing.T) {
	m := newTestModelWithStore(t)

	m.startProjectForm()
	m.form.Init()
	v, ok := m.formData.(*projectFormData)
	require.True(t, ok)
	v.Title = "Initial"
	m.saveFormInPlace()
	require.NotNil(t, m.editID)
	createdID := *m.editID

	// Second save — now an update since editID is set.
	v.Title = "Updated"
	m.saveFormInPlace()
	assert.Equal(t, createdID, *m.editID, "editID should not change on update")

	m.exitForm()

	meta, ok := m.selectedRowMeta()
	require.True(t, ok)
	assert.Equal(t, createdID, meta.ID, "cursor should be on the item after two saves then Esc")
}

// TestEditExistingThenEscKeepsCursor verifies that editing an existing item
// and pressing Esc (without saving) keeps the cursor on that item, not
// some arbitrary row. This guards against regressions from the exitForm
// cursor-move logic.
func TestEditExistingThenEscKeepsCursor(t *testing.T) {
	m := newTestModelWithStore(t)

	// Create two projects.
	for _, title := range []string{"Alpha", "Beta"} {
		m.startProjectForm()
		m.form.Init()
		v, ok := m.formData.(*projectFormData)
		require.True(t, ok)
		v.Title = title
		m.saveForm()
	}

	// Cursor is on the last created item ("Beta").
	meta, ok := m.selectedRowMeta()
	require.True(t, ok)
	betaID := meta.ID

	// Open edit form for Beta, then abort without saving.
	require.NoError(t, m.startEditProjectForm(betaID))
	m.form.Init()
	m.exitForm()

	meta, ok = m.selectedRowMeta()
	require.True(t, ok, "should still have a selected row after edit abort")
	assert.Equal(t, betaID, meta.ID, "cursor should stay on the item that was being edited")
}

// TestExitFormWithNoSaveNoCursorMove verifies that aborting a brand-new form
// (no save at all) does not move the cursor — editID is nil so exitForm
// should be a no-op for cursor positioning.
func TestExitFormWithNoSaveNoCursorMove(t *testing.T) {
	m := newTestModelWithStore(t)

	// Create a project so we have a row to be on.
	m.startProjectForm()
	m.form.Init()
	v, ok := m.formData.(*projectFormData)
	require.True(t, ok)
	v.Title = "Only"
	m.saveForm()

	meta, ok := m.selectedRowMeta()
	require.True(t, ok)
	onlyID := meta.ID

	// Open add form and immediately abort — no save.
	m.startProjectForm()
	m.form.Init()
	require.Nil(t, m.editID, "editID should be nil for a new add form")
	m.exitForm()

	meta, ok = m.selectedRowMeta()
	require.True(t, ok, "should still have a selected row after aborting empty form")
	assert.Equal(t, onlyID, meta.ID, "cursor should not move when no save occurred")
}

func TestAddProjectFormHasOnlyEssentialFields(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startProjectForm()

	view := formFieldLabels(m)
	// Essential fields should be present.
	for _, want := range []string{"Title", "Project type", "Status"} {
		assert.Containsf(t, view, want, "add project form should contain %q", want)
	}
	// Optional fields should be absent.
	for _, absent := range []string{"Budget", "Actual cost", "Start date", "End date", "Description"} {
		assert.NotContainsf(t, view, absent, "add project form should NOT contain %q", absent)
	}
}

func TestEditProjectFormHasMoreFieldsThanAdd(t *testing.T) {
	m := newTestModelWithStore(t)
	// Create a project so we can edit it.
	m.startProjectForm()
	m.form.Init()
	values, ok := m.formData.(*projectFormData)
	require.True(t, ok, "unexpected form data type")
	values.Title = testProjectTitle
	require.NoError(t, m.submitProjectForm())
	m.exitForm()
	m.reloadAll()

	require.NoError(t, m.startEditProjectForm(1))
	// The edit form's first group includes Budget and Actual cost,
	// which are absent from the add form.
	view := formFieldLabels(m)
	for _, want := range []string{"Title", "Status", "Budget", "Actual cost"} {
		assert.Containsf(t, view, want, "edit project form should contain %q", want)
	}
}

func TestAddVendorFormHasOnlyName(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startVendorForm()

	view := formFieldLabels(m)
	assert.Contains(t, view, "Name")
	for _, absent := range []string{"Contact name", "Email", "Phone", "Website"} {
		assert.NotContainsf(t, view, absent, "add vendor form should NOT contain %q", absent)
	}
}

func TestEditVendorFormHasAllFields(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startVendorForm()
	m.form.Init()
	values, ok := m.formData.(*vendorFormData)
	require.True(t, ok, "unexpected form data type")
	values.Name = "Test Vendor"
	require.NoError(t, m.submitVendorForm())
	m.exitForm()
	m.reloadAll()

	require.NoError(t, m.startEditVendorForm(1))
	view := formFieldLabels(m)
	for _, want := range []string{"Name", "Contact name", "Email", "Phone", "Website"} {
		assert.Containsf(t, view, want, "edit vendor form should contain %q", want)
	}
}

func TestAddApplianceFormHasOnlyName(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startApplianceForm()

	view := formFieldLabels(m)
	assert.Contains(t, view, "Name")
	for _, absent := range []string{"Brand", "Model number", "Serial number", "Location", "Purchase date", "Warranty expiry", "Cost"} {
		assert.NotContainsf(t, view, absent, "add appliance form should NOT contain %q", absent)
	}
}

func TestAddMaintenanceFormHasOnlyEssentialFields(t *testing.T) {
	m := newTestModelWithStore(t)
	require.NoError(t, m.startMaintenanceForm())

	view := formFieldLabels(m)
	for _, want := range []string{"Item", "Category", "Schedule"} {
		assert.Containsf(t, view, want, "add maintenance form should contain %q", want)
	}
	for _, absent := range []string{"Manual URL", "Manual notes", "Cost", "Last serviced"} {
		assert.NotContainsf(t, view, absent, "add maintenance form should NOT contain %q", absent)
	}
}

func TestAddQuoteFormHasOnlyEssentialFields(t *testing.T) {
	m := newTestModelWithStore(t)
	// Need a project first.
	m.startProjectForm()
	m.form.Init()
	values, ok := m.formData.(*projectFormData)
	require.True(t, ok, "unexpected form data type")
	values.Title = testProjectTitle
	require.NoError(t, m.submitProjectForm())
	m.exitForm()
	m.reloadAll()

	require.NoError(t, m.startQuoteForm())
	view := formFieldLabels(m)
	for _, want := range []string{"Project", "Vendor name", "Total"} {
		assert.Containsf(t, view, want, "add quote form should contain %q", want)
	}
	for _, absent := range []string{"Contact name", "Email", "Phone", "Labor", "Materials", "Other", "Received date"} {
		assert.NotContainsf(t, view, absent, "add quote form should NOT contain %q", absent)
	}
}

func TestAddServiceLogFormHasOnlyEssentialFields(t *testing.T) {
	m := newTestModelWithStore(t)
	require.NoError(t, m.startServiceLogForm(0))
	view := formFieldLabels(m)
	for _, want := range []string{"Date serviced", "Performed by"} {
		assert.Containsf(t, view, want, "add service log form should contain %q", want)
	}
	for _, absent := range []string{"Cost", "Notes"} {
		assert.NotContainsf(t, view, absent, "add service log form should NOT contain %q", absent)
	}
}
