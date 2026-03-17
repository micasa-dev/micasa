// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// formFieldLabels initializes the form and returns the rendered view text.
// Callers check for presence/absence of field labels.
func formFieldLabels(m *Model) string {
	if m.fs.form == nil {
		return ""
	}
	m.fs.form.Init()
	return m.fs.form.View()
}

func TestSaveFormFocusesNewItem(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create first project via user interaction.
	openAddForm(m)
	v1, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	v1.Title = "First"
	sendKey(m, "ctrl+s")
	sendKey(m, "esc")

	meta, ok := m.selectedRowMeta()
	require.True(t, ok, "should have a selected row after creating first project")
	firstID := meta.ID

	// Create second project; cursor should move to the new item.
	openAddForm(m)
	v2, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	v2.Title = "Second"
	sendKey(m, "ctrl+s")
	sendKey(m, "esc")

	meta, ok = m.selectedRowMeta()
	require.True(t, ok, "should have a selected row after creating second project")
	assert.NotEqual(t, firstID, meta.ID,
		"cursor should move to the newly created item, not stay on the first")
}

// TestSaveFormInPlaceThenEscFocusesNewItem verifies the Ctrl+S -> Esc flow:
// creating an item via save-in-place, then closing the form, should leave
// the cursor on the newly created item.
func TestSaveFormInPlaceThenEscFocusesNewItem(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Seed an existing project so the cursor starts on something else.
	openAddForm(m)
	v1, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	v1.Title = "Existing"
	sendKey(m, "ctrl+s")
	sendKey(m, "esc")

	existingMeta, ok := m.selectedRowMeta()
	require.True(t, ok)
	existingID := existingMeta.ID

	// Start a new add form, save in place (Ctrl+S), then exit (Esc).
	openAddForm(m)
	v2, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	v2.Title = "Via CtrlS"
	sendKey(m, "ctrl+s")
	require.NotNil(t, m.fs.editID, "editID should be set after save-in-place create")
	newID := *m.fs.editID

	// Esc closes the form (clean after Ctrl+S snapshot).
	sendKey(m, "esc")

	meta, ok := m.selectedRowMeta()
	require.True(t, ok, "should have a selected row after Ctrl+S then Esc")
	assert.Equal(t, newID, meta.ID, "cursor should be on the newly created item")
	assert.NotEqual(t, existingID, meta.ID, "cursor should not stay on the old item")
}

// TestSaveFormInPlaceThenDiscardFocusesNewItem verifies the Ctrl+S -> edit
// more -> Esc -> confirm discard "y" flow.
func TestSaveFormInPlaceThenDiscardFocusesNewItem(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Seed an existing project.
	openAddForm(m)
	v1, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	v1.Title = "Existing"
	sendKey(m, "ctrl+s")
	sendKey(m, "esc")

	// Start add form, save in place, then make the form dirty.
	openAddForm(m)
	v2, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	v2.Title = "Saved InPlace"
	sendKey(m, "ctrl+s")
	require.NotNil(t, m.fs.editID)
	newID := *m.fs.editID

	// Mutate form data after snapshot to make it dirty.
	v2.Title = "Unsaved Change"
	m.checkFormDirty()
	require.True(t, m.fs.formDirty, "form should be dirty after mutation")

	// Esc on dirty form triggers confirm dialog, y confirms discard.
	sendKey(m, "esc")
	require.Equal(t, confirmFormDiscard, m.confirm, "dirty form esc should show confirm dialog")
	sendKey(m, "y")

	meta, ok := m.selectedRowMeta()
	require.True(t, ok, "should have a selected row after discard")
	assert.Equal(t, newID, meta.ID, "cursor should be on the saved item, not the old one")
}

// TestSaveFormInPlaceTwiceThenEscFocusesItem verifies that Ctrl+S twice
// (create then update) followed by Esc still lands on the item.
func TestSaveFormInPlaceTwiceThenEscFocusesItem(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	openAddForm(m)
	v, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	v.Title = "Initial"
	sendKey(m, "ctrl+s")
	require.NotNil(t, m.fs.editID)
	createdID := *m.fs.editID

	// Second save -- now an update since editID is set.
	v.Title = "Updated"
	sendKey(m, "ctrl+s")
	assert.Equal(t, createdID, *m.fs.editID, "editID should not change on update")

	sendKey(m, "esc")

	meta, ok := m.selectedRowMeta()
	require.True(t, ok)
	assert.Equal(t, createdID, meta.ID, "cursor should be on the item after two saves then Esc")
}

// TestEditExistingThenEscKeepsCursor verifies that editing an existing item
// and pressing Esc (without saving) keeps the cursor on that item, not
// some arbitrary row. This guards against regressions from the exitForm
// cursor-move logic.
func TestEditExistingThenEscKeepsCursor(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create two projects via user interaction.
	for _, title := range []string{"Alpha", "Beta"} {
		openAddForm(m)
		v, ok := m.fs.formData.(*projectFormData)
		require.True(t, ok)
		v.Title = title
		sendKey(m, "ctrl+s")
		sendKey(m, "esc")
	}

	// Cursor is on the last created item ("Beta").
	meta, ok := m.selectedRowMeta()
	require.True(t, ok)
	betaID := meta.ID

	// Open edit form for Beta via the ID column (fallback to full form).
	tab := m.activeTab()
	require.NotNil(t, tab)
	sendKey(m, "i")
	tab.ColCursor = int(projectColID)
	sendKey(m, "e")
	require.Equal(t, modeForm, m.mode, "should open full edit form")

	// Abort without saving.
	sendKey(m, "esc")

	meta, ok = m.selectedRowMeta()
	require.True(t, ok, "should still have a selected row after edit abort")
	assert.Equal(t, betaID, meta.ID, "cursor should stay on the item that was being edited")
}

// TestExitFormWithNoSaveNoCursorMove verifies that aborting a brand-new form
// (no save at all) does not move the cursor -- editID is nil so exitForm
// should be a no-op for cursor positioning.
func TestExitFormWithNoSaveNoCursorMove(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create a project so we have a row to be on.
	openAddForm(m)
	v, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	v.Title = "Only"
	sendKey(m, "ctrl+s")
	sendKey(m, "esc")

	meta, ok := m.selectedRowMeta()
	require.True(t, ok)
	onlyID := meta.ID

	// Open add form and immediately abort -- no save.
	openAddForm(m)
	require.Nil(t, m.fs.editID, "editID should be nil for a new add form")
	sendKey(m, "esc")

	meta, ok = m.selectedRowMeta()
	require.True(t, ok, "should still have a selected row after aborting empty form")
	assert.Equal(t, onlyID, meta.ID, "cursor should not move when no save occurred")
}

func TestAddProjectFormHasOnlyEssentialFields(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	openAddForm(m)

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
	t.Parallel()
	m := newTestModelWithStore(t)
	// Create a project via user interaction.
	openAddForm(m)
	values, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok, "unexpected form data type")
	values.Title = testProjectTitle
	sendKey(m, "ctrl+s")
	sendKey(m, "esc")

	// Open edit form via ID column.
	tab := m.activeTab()
	require.NotNil(t, tab)
	sendKey(m, "i")
	tab.ColCursor = int(projectColID)
	sendKey(m, "e")
	require.Equal(t, modeForm, m.mode, "should open full edit form")
	// The edit form's first group includes Budget and Actual cost,
	// which are absent from the add form.
	view := formFieldLabels(m)
	for _, want := range []string{"Title", "Status", "Budget", "Actual cost"} {
		assert.Containsf(t, view, want, "edit project form should contain %q", want)
	}
}

func TestAddVendorFormHasOnlyName(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.active = tabIndex(tabVendors)
	openAddForm(m)

	view := formFieldLabels(m)
	assert.Contains(t, view, "Name")
	for _, absent := range []string{"Contact name", "Email", "Phone", "Website"} {
		assert.NotContainsf(t, view, absent, "add vendor form should NOT contain %q", absent)
	}
}

func TestEditVendorFormHasAllFields(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.active = tabIndex(tabVendors)
	// Create a vendor via user interaction.
	openAddForm(m)
	values, ok := m.fs.formData.(*vendorFormData)
	require.True(t, ok, "unexpected form data type")
	values.Name = "Test Vendor"
	sendKey(m, "ctrl+s")
	sendKey(m, "esc")

	// Open edit form via ID column.
	tab := m.activeTab()
	require.NotNil(t, tab)
	sendKey(m, "i")
	tab.ColCursor = int(vendorColID)
	sendKey(m, "e")
	require.Equal(t, modeForm, m.mode, "should open full edit form")
	view := formFieldLabels(m)
	for _, want := range []string{"Name", "Contact name", "Email", "Phone", "Website"} {
		assert.Containsf(t, view, want, "edit vendor form should contain %q", want)
	}
}

func TestAddApplianceFormHasOnlyName(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.active = tabIndex(tabAppliances)
	openAddForm(m)

	view := formFieldLabels(m)
	assert.Contains(t, view, "Name")
	for _, absent := range []string{"Brand", "Model number", "Serial number", "Location", "Purchase date", "Warranty expiry", "Cost"} {
		assert.NotContainsf(t, view, absent, "add appliance form should NOT contain %q", absent)
	}
}

func TestAddMaintenanceFormHasOnlyEssentialFields(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.active = tabIndex(tabMaintenance)
	openAddForm(m)

	view := formFieldLabels(m)
	for _, want := range []string{"Item", "Category", "Schedule"} {
		assert.Containsf(t, view, want, "add maintenance form should contain %q", want)
	}
	for _, absent := range []string{"Manual URL", "Manual notes", "Cost", "Last serviced"} {
		assert.NotContainsf(t, view, absent, "add maintenance form should NOT contain %q", absent)
	}
}

func TestAddQuoteFormHasOnlyEssentialFields(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	// Need a project first.
	openAddForm(m)
	values, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok, "unexpected form data type")
	values.Title = testProjectTitle
	sendKey(m, "ctrl+s")
	sendKey(m, "esc")

	// Navigate to Quotes tab and open add form.
	m.active = tabIndex(tabQuotes)
	openAddForm(m)
	view := formFieldLabels(m)
	for _, want := range []string{"Project", "Vendor name", "Total"} {
		assert.Containsf(t, view, want, "add quote form should contain %q", want)
	}
	for _, absent := range []string{"Contact name", "Email", "Phone", "Labor", "Materials", "Other", "Received date"} {
		assert.NotContainsf(t, view, absent, "add quote form should NOT contain %q", absent)
	}
}

func TestAddServiceLogFormHasOnlyEssentialFields(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	require.NoError(t, m.startServiceLogForm(""))
	view := formFieldLabels(m)
	for _, want := range []string{"Date serviced", "Performed by"} {
		assert.Containsf(t, view, want, "add service log form should contain %q", want)
	}
	for _, absent := range []string{"Cost", "Notes"} {
		assert.NotContainsf(t, view, absent, "add service log form should NOT contain %q", absent)
	}
}
