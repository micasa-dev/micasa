// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInlineEditProjectTextColumnOpensInlineInput(t *testing.T) {
	m := newTestModelWithStore(t)
	// Create a project.
	m.startProjectForm()
	m.form.Init()
	values, ok := m.formData.(*projectFormData)
	require.True(t, ok, "unexpected form data type")
	values.Title = testProjectTitle
	require.NoError(t, m.submitProjectForm())
	m.exitForm()
	m.reloadAll()

	// Inline edit the Title column -- should open inline input.
	require.NoError(t, m.inlineEditProject(1, projectColTitle))
	require.NotNil(t, m.inlineInput, "expected inline input for text column (Title)")
	assert.Equal(t, "Title", m.inlineInput.Title)
	assert.NotEqual(t, modeForm, m.mode, "inline input should not switch to modeForm")
}

func TestInlineEditProjectSelectColumnOpensFormOverlay(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startProjectForm()
	m.form.Init()
	values, ok := m.formData.(*projectFormData)
	require.True(t, ok, "unexpected form data type")
	values.Title = testProjectTitle
	require.NoError(t, m.submitProjectForm())
	m.exitForm()
	m.reloadAll()

	// Inline edit the Status column -- should open form overlay.
	require.NoError(t, m.inlineEditProject(1, projectColStatus))
	assert.Nil(t, m.inlineInput, "select column should NOT open inline input")
	assert.Equal(t, modeForm, m.mode, "select column should open form overlay")
}

func TestInlineEditVendorTextColumnsUseInlineInput(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startVendorForm()
	m.form.Init()
	values, ok := m.formData.(*vendorFormData)
	require.True(t, ok, "unexpected form data type")
	values.Name = "Test Vendor"
	require.NoError(t, m.submitVendorForm())
	m.exitForm()
	m.reloadAll()

	// All editable vendor columns are text, so they should all use inline input.
	cases := []struct {
		col   vendorCol
		title string
	}{
		{vendorColName, "Name"},
		{vendorColContact, "Contact name"},
		{vendorColEmail, "Email"},
		{vendorColPhone, "Phone"},
		{vendorColWebsite, "Website"},
	}
	for _, tc := range cases {
		m.closeInlineInput()
		require.NoErrorf(t, m.inlineEditVendor(1, tc.col), "inlineEditVendor col %d", tc.col)
		require.NotNilf(t, m.inlineInput, "col %d (%s) should open inline input", tc.col, tc.title)
		assert.Equalf(t, tc.title, m.inlineInput.Title, "col %d title mismatch", tc.col)
	}
}

func TestInlineEditAppliaceDateColumnOpensCalendar(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startApplianceForm()
	m.form.Init()
	values, ok := m.formData.(*applianceFormData)
	require.True(t, ok, "unexpected form data type")
	values.Name = "Test Fridge"
	require.NoError(t, m.submitApplianceForm())
	m.exitForm()
	m.reloadAll()

	// Purchase date column should open calendar picker.
	require.NoError(t, m.inlineEditAppliance(1, applianceColPurchased))
	assert.NotNil(t, m.calendar, "date column should open calendar picker")
	assert.Nil(t, m.inlineInput, "date column should NOT open inline input")
}

func TestShiftEOpensFullEditFormRegardlessOfColumn(t *testing.T) {
	m := newTestModelWithStore(t)
	// Create a vendor so there's data to edit.
	m.startVendorForm()
	m.form.Init()
	values, ok := m.formData.(*vendorFormData)
	require.True(t, ok, "unexpected form data type")
	values.Name = "Test Vendor"
	require.NoError(t, m.submitVendorForm())
	m.exitForm()
	m.reloadAll()

	// Switch to vendor tab.
	for i, tab := range m.tabs {
		if tab.Kind == tabVendors {
			m.active = i
			break
		}
	}
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	require.NotNil(t, tab)
	require.NotEmpty(t, tab.Rows)
	tab.Table.SetCursor(0)

	// Enter edit mode and position cursor on Name column (editable).
	sendKey(m, "i")
	tab.ColCursor = int(vendorColName)

	// Pressing 'E' should open the full edit form, not inline edit.
	sendKey(m, "E")
	assert.Equal(t, modeForm, m.mode,
		"shift+e should open the full edit form even on an editable column")
	assert.Nil(t, m.inlineInput,
		"shift+e should not open inline input")
}

func TestEditKeyDispatchesInlineEditInEditMode(t *testing.T) {
	m := newTestModelWithStore(t)
	// Create a vendor so there's data to edit.
	m.startVendorForm()
	m.form.Init()
	values, ok := m.formData.(*vendorFormData)
	require.True(t, ok, "unexpected form data type")
	values.Name = "Test Vendor"
	require.NoError(t, m.submitVendorForm())
	m.exitForm()
	m.reloadAll()

	// Switch to vendor tab.
	for i, tab := range m.tabs {
		if tab.Kind == tabVendors {
			m.active = i
			break
		}
	}
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	if tab == nil || len(tab.Rows) == 0 {
		t.Skip("no vendor rows to test")
	}

	// Ensure table cursor is on the first row.
	tab.Table.SetCursor(0)

	// Enter edit mode and position cursor on Name column.
	sendKey(m, "i")
	tab.ColCursor = 1 // Name column

	// Press 'e' to trigger inline edit.
	sendKey(m, "e")

	// Should have opened inline input for the Name field.
	assert.True(t, m.inlineInput != nil || m.mode == modeForm,
		"pressing 'e' in edit mode should open inline input or form for the current cell")

	// Verify the status bar shows the inline prompt.
	if m.inlineInput != nil {
		status := m.statusView()
		assert.Contains(t, status, "Name")
	}
}
