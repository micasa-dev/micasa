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
// requiredDate
// ---------------------------------------------------------------------------

func TestRequiredDateValid(t *testing.T) {
	validate := requiredDate("test date")
	assert.NoError(t, validate("2026-01-15"))
	assert.NoError(t, validate("2024-12-31"))
}

func TestRequiredDateEmpty(t *testing.T) {
	validate := requiredDate("test date")
	err := validate("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestRequiredDateWhitespace(t *testing.T) {
	validate := requiredDate("test date")
	err := validate("   ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestRequiredDateInvalid(t *testing.T) {
	validate := requiredDate("test date")
	err := validate("not-a-date")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "YYYY-MM-DD")
}

func TestRequiredDatePartialFormat(t *testing.T) {
	validate := requiredDate("test date")
	err := validate("2026-1-5")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// documentFormValues
// ---------------------------------------------------------------------------

func TestDocumentFormValuesRoundTrip(t *testing.T) {
	doc := data.Document{
		Title:      "My Doc",
		EntityKind: data.DocumentEntityProject,
		EntityID:   42,
		Notes:      "doc notes",
	}
	fd := documentFormValues(doc)
	assert.Equal(t, "My Doc", fd.Title)
	assert.Equal(t, data.DocumentEntityProject, fd.EntityRef.Kind)
	assert.Equal(t, uint(42), fd.EntityRef.ID)
	assert.Equal(t, "doc notes", fd.Notes)
}

func TestDocumentFormValuesNoEntity(t *testing.T) {
	doc := data.Document{
		Title: "Standalone Doc",
	}
	fd := documentFormValues(doc)
	assert.Equal(t, "Standalone Doc", fd.Title)
	assert.Equal(t, "", fd.EntityRef.Kind)
	assert.Equal(t, uint(0), fd.EntityRef.ID)
}

// ---------------------------------------------------------------------------
// documentEntityOptions
// ---------------------------------------------------------------------------

func TestDocumentEntityOptionsEmpty(t *testing.T) {
	m := newTestModelWithStore(t)
	opts, err := m.documentEntityOptions()
	require.NoError(t, err)
	// Should have at least the "(none)" option.
	require.NotEmpty(t, opts)
	assert.Equal(t, entityRef{}, opts[0].Value, "first option should be (none)")
}

func TestDocumentEntityOptionsIncludesEntities(t *testing.T) {
	m := newTestModelWithStore(t)

	// Create one of each entity type so they appear in options.
	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Opt Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "Opt App"}))
	require.NoError(t, m.store.CreateVendor(&data.Vendor{Name: "Opt Vendor"}))
	m.vendors, _ = m.store.ListVendors(false)

	cats, _ := m.store.MaintenanceCategories()
	require.NoError(t, m.store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "Opt Maint",
		CategoryID: cats[0].ID,
	}))
	require.NoError(t, m.store.CreateIncident(&data.Incident{
		Title:       "Opt Incident",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}))

	opts, err := m.documentEntityOptions()
	require.NoError(t, err)
	// (none) + 1 project + 1 appliance + 1 vendor + 1 maintenance + 1 incident = 6
	require.GreaterOrEqual(t, len(opts), 6,
		"should have (none) + all entity types")

	// Verify at least one option per entity kind.
	kindsSeen := map[string]bool{}
	for _, opt := range opts {
		if opt.Value.Kind != "" {
			kindsSeen[opt.Value.Kind] = true
		}
	}
	for _, k := range []string{
		data.DocumentEntityProject,
		data.DocumentEntityAppliance,
		data.DocumentEntityVendor,
		data.DocumentEntityMaintenance,
		data.DocumentEntityIncident,
	} {
		assert.Truef(t, kindsSeen[k], "expected entity kind %q in options", k)
	}
}

// ---------------------------------------------------------------------------
// incidentFormValues round-trip
// ---------------------------------------------------------------------------

func TestIncidentFormValuesRoundTrip(t *testing.T) {
	appID := uint(5)
	vendorID := uint(3)
	cost := int64(15000)
	resolved := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	item := data.Incident{
		Title:        "Test",
		Description:  "desc",
		Status:       data.IncidentStatusInProgress,
		Severity:     data.IncidentSeverityUrgent,
		DateNoticed:  time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		DateResolved: &resolved,
		Location:     "Garage",
		CostCents:    &cost,
		ApplianceID:  &appID,
		VendorID:     &vendorID,
		Notes:        "notes",
	}
	fd := incidentFormValues(item)
	assert.Equal(t, "Test", fd.Title)
	assert.Equal(t, "desc", fd.Description)
	assert.Equal(t, data.IncidentStatusInProgress, fd.Status)
	assert.Equal(t, data.IncidentSeverityUrgent, fd.Severity)
	assert.Equal(t, "2026-02-01", fd.DateNoticed)
	assert.Equal(t, "2026-02-20", fd.DateResolved)
	assert.Equal(t, "Garage", fd.Location)
	assert.Equal(t, "$150.00", fd.Cost)
	assert.Equal(t, appID, fd.ApplianceID)
	assert.Equal(t, vendorID, fd.VendorID)
	assert.Equal(t, "notes", fd.Notes)
}

func TestIncidentFormValuesNilOptionals(t *testing.T) {
	item := data.Incident{
		Title:       "Minimal",
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeverityWhenever,
		DateNoticed: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	fd := incidentFormValues(item)
	assert.Equal(t, uint(0), fd.ApplianceID)
	assert.Equal(t, uint(0), fd.VendorID)
	assert.Equal(t, "", fd.Cost)
	assert.Equal(t, "", fd.DateResolved)
}

// ---------------------------------------------------------------------------
// serviceLogFormValues round-trip
// ---------------------------------------------------------------------------

func TestServiceLogFormValuesRoundTrip(t *testing.T) {
	vendorID := uint(7)
	cost := int64(5000)
	entry := data.ServiceLogEntry{
		MaintenanceItemID: 10,
		ServicedAt:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		VendorID:          &vendorID,
		CostCents:         &cost,
		Notes:             "service notes",
	}
	fd := serviceLogFormValues(entry)
	assert.Equal(t, uint(10), fd.MaintenanceItemID)
	assert.Equal(t, "2026-01-15", fd.ServicedAt)
	assert.Equal(t, vendorID, fd.VendorID)
	assert.Equal(t, "$50.00", fd.Cost)
	assert.Equal(t, "service notes", fd.Notes)
}

func TestServiceLogFormValuesNilOptionals(t *testing.T) {
	entry := data.ServiceLogEntry{
		MaintenanceItemID: 1,
		ServicedAt:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	fd := serviceLogFormValues(entry)
	assert.Equal(t, uint(0), fd.VendorID)
	assert.Equal(t, "", fd.Cost)
}

// ---------------------------------------------------------------------------
// vendorFormValues round-trip
// ---------------------------------------------------------------------------

func TestVendorFormValuesRoundTrip(t *testing.T) {
	vendor := data.Vendor{
		Name:        "Acme",
		ContactName: "Bob",
		Email:       "bob@acme.com",
		Phone:       "555-1234",
		Website:     "https://acme.com",
		Notes:       "vendor notes",
	}
	fd := vendorFormValues(vendor)
	assert.Equal(t, "Acme", fd.Name)
	assert.Equal(t, "Bob", fd.ContactName)
	assert.Equal(t, "bob@acme.com", fd.Email)
	assert.Equal(t, "555-1234", fd.Phone)
	assert.Equal(t, "https://acme.com", fd.Website)
	assert.Equal(t, "vendor notes", fd.Notes)
}
