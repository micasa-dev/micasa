// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHardDeleteWorksInMaintenanceDrilldown verifies that Shift+D
// hard-deletes a soft-deleted maintenance item from the
// Appliances > Maintenance drill-down the same way it does from the
// top-level Maintenance tab. Without the fix, both the promptHardDelete
// gate and the confirmHardDelete dispatch keyed on tab.Kind, which in
// the drill-down is tabAppliances -- silently routing or blocking.
func TestHardDeleteWorksInMaintenanceDrilldown(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Set up appliance + one maintenance item scoped to it.
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "Furnace"}))
	appls, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	require.NotEmpty(t, appls)
	applID := appls[0].ID

	cats, err := m.store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:        "Replace filter",
		CategoryID:  cats[0].ID,
		ApplianceID: &applID,
	}))
	items, err := m.store.ListMaintenanceByAppliance(applID, false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	itemID := items[0].ID

	// Open Appliance > Maintenance drill-down.
	require.NoError(t, m.openApplianceMaintenanceDetail(applID, "Furnace"))
	require.True(t, m.inDetail())
	tab := m.effectiveTab()
	require.NotNil(t, tab)
	assert.Equal(t, formMaintenance, tab.Handler.FormKind(),
		"drill-down handler must identify as maintenance via FormKind")

	// Select the row, enter edit mode.
	require.NotEmpty(t, tab.Rows)
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)

	// Shift+D on a live row must surface the maintenance-specific message.
	sendKey(m, "D")
	assert.NotEqual(t, confirmHardDelete, m.confirm,
		"Shift+D on live row should not prompt hard-delete")
	assert.Contains(t, m.statusView(), "Delete the item first",
		"message must use 'item' (maintenance), not 'incident'")

	// Soft-delete, then hard-delete.
	sendKey(m, "d")
	require.NoError(t, m.reloadEffectiveTab())

	sendKey(m, "D")
	assert.Equal(t, confirmHardDelete, m.confirm,
		"Shift+D on soft-deleted row should prompt hard-delete in drill-down")
	assert.Contains(t, m.statusView(), "Permanently delete this item?",
		"prompt label must say 'item' in a maintenance drill-down")

	sendKey(m, "y")
	assert.Equal(t, confirmNone, m.confirm)
	assert.Contains(t, m.statusView(), "Permanently deleted")

	// Row must be gone from the store.
	_, err = m.store.GetMaintenance(itemID)
	assert.Error(t, err, "maintenance item must be hard-deleted from the store")
}

// TestToggleSettledFilterIdentifiesProjectTabByFormKind ensures the
// settled-filter no longer key-cases on tab.Kind, so future project
// drill-downs (none exists today) would inherit the feature and
// non-project drill-downs continue to be no-ops.
func TestToggleSettledFilterIdentifiesProjectTabByFormKind(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Top-level Projects tab: toggle should succeed.
	m.active = tabIndex(tabProjects)
	require.NoError(t, m.reloadActiveTab())
	assert.True(t, m.toggleSettledFilter(),
		"top-level Projects must still respond to settled filter")

	// Non-project drill-down: toggle must be a no-op.
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "Toggle Test"}))
	appls, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	require.NoError(t, m.openApplianceMaintenanceDetail(appls[0].ID, "Toggle Test"))
	assert.False(t, m.toggleSettledFilter(),
		"settled filter must not fire in a non-project drill-down")
}

// TestApplianceMaintenanceDrilldownAddPrescopesAppliance ensures that
// pressing "a" in an Appliance > Maintenance drill-down pre-populates
// the parent appliance instead of defaulting to the first appliance in
// the store. Without the fix, the drill-down context is lost as soon
// as the add form opens.
func TestApplianceMaintenanceDrilldownAddPrescopesAppliance(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "Decoy First"}))
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "Parent Appliance"}))
	require.NoError(t, m.loadLookups())

	appls, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	require.Len(t, appls, 2)
	var parentID string
	for _, a := range appls {
		if a.Name == "Parent Appliance" {
			parentID = a.ID
			break
		}
	}
	require.NotEmpty(t, parentID)

	require.NoError(t, m.openApplianceMaintenanceDetail(parentID, "Parent Appliance"))
	m.enterEditMode()
	sendKey(m, "a")
	require.Equal(t, modeForm, m.mode)

	fd, ok := m.fs.formData.(*maintenanceFormData)
	require.True(t, ok, "expected maintenance form data")
	assert.Equal(t, parentID, fd.ApplianceID,
		"add form in Appliance > Maintenance must pre-scope to the drilled-into appliance")
}

// TestProjectQuoteDrilldownAddPrescopesProject ensures Project > Quotes
// drill-down pre-selects the parent project on the quote add form.
// Two projects are created so the parent is NOT the default (most
// recently updated) project returned by ListProjects.
func TestProjectQuoteDrilldownAddPrescopesProject(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	types, err := m.store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title: "Parent Project", ProjectTypeID: types[0].ID, Status: data.ProjectStatusPlanned,
	}))
	// Create the decoy AFTER the parent so it sorts ahead in
	// ListProjects (order: updated_at desc), ensuring the default
	// options[0] is the decoy and the test must rely on pre-scoping.
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title: "Decoy Project", ProjectTypeID: types[0].ID, Status: data.ProjectStatusPlanned,
	}))
	projects, err := m.store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 2)
	var parentID string
	for _, p := range projects {
		if p.Title == "Parent Project" {
			parentID = p.ID
			break
		}
	}
	require.NotEmpty(t, parentID)

	require.NoError(t, m.openProjectQuoteDetail(parentID, "Parent Project"))
	m.enterEditMode()
	sendKey(m, "a")
	require.Equal(t, modeForm, m.mode)

	fd, ok := m.fs.formData.(*quoteFormData)
	require.True(t, ok, "expected quote form data")
	assert.Equal(t, parentID, fd.ProjectID,
		"add form in Project > Quotes must pre-scope to the drilled-into project")
}

// TestVendorQuoteDrilldownAddPrescopesVendor ensures Vendor > Quotes
// drill-down pre-fills the vendor name on the quote add form.
func TestVendorQuoteDrilldownAddPrescopesVendor(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	types, err := m.store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title: "Any Project", ProjectTypeID: types[0].ID, Status: data.ProjectStatusPlanned,
	}))
	require.NoError(t, m.store.CreateVendor(&data.Vendor{Name: "Parent Vendor"}))
	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	require.NotEmpty(t, vendors)
	parentID := vendors[0].ID

	require.NoError(t, m.openVendorQuoteDetail(parentID, "Parent Vendor"))
	m.enterEditMode()
	sendKey(m, "a")
	require.Equal(t, modeForm, m.mode)

	fd, ok := m.fs.formData.(*quoteFormData)
	require.True(t, ok, "expected quote form data")
	assert.Equal(t, "Parent Vendor", fd.VendorName,
		"add form in Vendor > Quotes must pre-fill the drilled-into vendor name")
}
