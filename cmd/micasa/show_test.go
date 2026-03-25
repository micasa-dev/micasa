// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStoreWithMigration(t *testing.T) *data.Store {
	t.Helper()
	store, err := data.Open(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestShowHouseText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{
		Nickname:     "Test House",
		AddressLine1: "123 Main St",
		City:         "Springfield",
		State:        "IL",
		PostalCode:   "62701",
		YearBuilt:    1985,
		SquareFeet:   2400,
		Bedrooms:     3,
		Bathrooms:    2.5,
	}))

	var buf bytes.Buffer
	err := runShow(&buf, store, "house", false, false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "=== HOUSE ===")
	assert.Contains(t, out, "Nickname:")
	assert.Contains(t, out, "Test House")
	assert.Contains(t, out, "123 Main St")
	assert.Contains(t, out, "Springfield, IL 62701")
}

func TestShowHouseJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{
		Nickname: "Test House",
	}))

	var buf bytes.Buffer
	err := runShow(&buf, store, "house", true, false)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "Test House", result["nickname"])
}

func TestShowHouseEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	err := runShow(&buf, store, "house", false, false)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestFormatAddress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		h    data.HouseProfile
		want string
	}{
		{"full", data.HouseProfile{
			AddressLine1: "123 Main St", City: "Springfield", State: "IL", PostalCode: "62701",
		}, "123 Main St, Springfield, IL 62701"},
		{"with line2", data.HouseProfile{
			AddressLine1: "123 Main", AddressLine2: "Apt 4", City: "NYC", State: "NY", PostalCode: "10001",
		}, "123 Main, Apt 4, NYC, NY 10001"},
		{"city only", data.HouseProfile{City: "Denver"}, "Denver"},
		{"state only", data.HouseProfile{State: "CO"}, "CO"},
		{"postal only", data.HouseProfile{PostalCode: "80202"}, "80202"},
		{"empty", data.HouseProfile{}, ""},
		{"line1 only", data.HouseProfile{AddressLine1: "PO Box 42"}, "PO Box 42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, formatAddress(tt.h))
		})
	}
}

func TestShowUnknownEntity(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	err := runShow(&buf, store, "bogus", false, false)
	require.Error(t, err)
	require.ErrorContains(t, err, "unknown entity")
	require.ErrorContains(t, err, "bogus")
}

func TestShowProjectsText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	ptypes, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, ptypes)

	budget := int64(500000)
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: ptypes[0].ID,
		Status:        data.ProjectStatusPlanned,
		BudgetCents:   &budget,
		Description:   "Redo the kitchen",
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "projects", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== PROJECTS ===")
	assert.Contains(t, out, "Kitchen Remodel")
	assert.Contains(t, out, "planned")
	assert.Contains(t, out, "$5000.00")
}

func TestShowProjectsJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	ptypes, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, ptypes)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Deck Build",
		ProjectTypeID: ptypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "projects", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Deck Build", result[0]["title"])
	assert.Equal(t, "planned", result[0]["status"])
	assert.NotEmpty(t, result[0]["id"])
	assert.Equal(t, ptypes[0].Name, result[0]["project_type"])
}

func TestShowVendorsText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	require.NoError(t, store.CreateVendor(&data.Vendor{
		Name:        "Acme Plumbing",
		ContactName: "John Doe",
		Email:       "john@acme.com",
		Phone:       "555-1234",
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "vendors", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== VENDORS ===")
	assert.Contains(t, out, "Acme Plumbing")
	assert.Contains(t, out, "John Doe")
	assert.Contains(t, out, "john@acme.com")
	assert.Contains(t, out, "555-1234")
}

func TestShowVendorsJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	require.NoError(t, store.CreateVendor(&data.Vendor{
		Name:    "Acme Plumbing",
		Website: "https://acme.example.com",
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "vendors", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Acme Plumbing", result[0]["name"])
	assert.Equal(t, "https://acme.example.com", result[0]["website"])
	assert.NotEmpty(t, result[0]["id"])
}

func TestShowAppliancesText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	cost := int64(120000)
	require.NoError(t, store.CreateAppliance(&data.Appliance{
		Name:        "Dishwasher",
		Brand:       "Bosch",
		ModelNumber: "SHX88",
		Location:    "Kitchen",
		CostCents:   &cost,
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "appliances", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== APPLIANCES ===")
	assert.Contains(t, out, "Dishwasher")
	assert.Contains(t, out, "Bosch")
	assert.Contains(t, out, "SHX88")
	assert.Contains(t, out, "Kitchen")
	assert.Contains(t, out, "$1200.00")
}

func TestShowAppliancesJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	require.NoError(t, store.CreateAppliance(&data.Appliance{
		Name:         "Furnace",
		Brand:        "Carrier",
		SerialNumber: "ABC123",
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "appliances", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Furnace", result[0]["name"])
	assert.Equal(t, "Carrier", result[0]["brand"])
	assert.Equal(t, "ABC123", result[0]["serial_number"])
	assert.NotEmpty(t, result[0]["id"])
}

func TestShowIncidentsText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	cost := int64(25000)
	require.NoError(t, store.CreateIncident(&data.Incident{
		Title:     "Pipe burst",
		Status:    data.IncidentStatusOpen,
		Severity:  data.IncidentSeveritySoon,
		Location:  "Basement",
		CostCents: &cost,
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "incidents", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== INCIDENTS ===")
	assert.Contains(t, out, "Pipe burst")
	assert.Contains(t, out, "open")
	assert.Contains(t, out, "soon")
	assert.Contains(t, out, "Basement")
	assert.Contains(t, out, "$250.00")
}

func TestShowIncidentsJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	require.NoError(t, store.CreateIncident(&data.Incident{
		Title:    "Roof leak",
		Status:   data.IncidentStatusOpen,
		Severity: data.IncidentSeverityUrgent,
		Location: "Attic",
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "incidents", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Roof leak", result[0]["title"])
	assert.Equal(t, "open", result[0]["status"])
	assert.Equal(t, "urgent", result[0]["severity"])
	assert.Equal(t, "Attic", result[0]["location"])
	assert.NotEmpty(t, result[0]["id"])
}

func TestShowEmptyCollection(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	for _, entity := range []string{
		"projects", "vendors", "appliances", "incidents",
		"quotes", "maintenance", "service-log", "documents",
	} {
		var buf bytes.Buffer
		require.NoError(t, runShow(&buf, store, entity, false, false))
		assert.Empty(t, buf.String(), "expected no output for empty %s", entity)
	}
}

func TestShowEmptyCollectionJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	for _, entity := range []string{
		"projects", "vendors", "appliances", "incidents",
		"quotes", "maintenance", "service-log", "documents",
	} {
		var buf bytes.Buffer
		require.NoError(t, runShow(&buf, store, entity, true, false))

		var result []map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
		assert.Empty(t, result, "expected empty array for %s", entity)
	}
}

// --- quotes ---

func TestShowQuotesText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	ptypes, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, ptypes)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Roof Replacement",
		ProjectTypeID: ptypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)

	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "TopRoof Inc"}))
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)

	require.NoError(t, store.CreateQuote(&data.Quote{
		ProjectID:  projects[0].ID,
		TotalCents: 750000,
		Notes:      "includes permit fees",
	}, data.Vendor{Name: "TopRoof Inc"}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "quotes", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== QUOTES ===")
	assert.Contains(t, out, "Roof Replacement")
	assert.Contains(t, out, "TopRoof Inc")
	assert.Contains(t, out, "$7500.00")
	assert.Contains(t, out, "includes permit fees")
}

func TestShowQuotesJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	ptypes, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, ptypes)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Fence Install",
		ProjectTypeID: ptypes[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)

	require.NoError(t, store.CreateQuote(&data.Quote{
		ProjectID:  projects[0].ID,
		TotalCents: 320000,
	}, data.Vendor{Name: "FencePro"}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "quotes", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Fence Install", result[0]["project"])
	assert.Equal(t, "FencePro", result[0]["vendor"])
	assert.InDelta(t, float64(320000), result[0]["total_cents"], 0.1)
	assert.NotEmpty(t, result[0]["id"])
}

// --- maintenance ---

func TestShowMaintenanceText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	cost := int64(5000)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:           "Replace HVAC filter",
		CategoryID:     cats[0].ID,
		Season:         "spring",
		IntervalMonths: 3,
		CostCents:      &cost,
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "maintenance", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== MAINTENANCE ===")
	assert.Contains(t, out, "Replace HVAC filter")
	assert.Contains(t, out, cats[0].Name)
	assert.Contains(t, out, "spring")
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "$50.00")
}

func TestShowMaintenanceJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:           "Clean gutters",
		CategoryID:     cats[0].ID,
		Season:         "fall",
		IntervalMonths: 12,
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "maintenance", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Clean gutters", result[0]["name"])
	assert.Equal(t, cats[0].Name, result[0]["category"])
	assert.Equal(t, "fall", result[0]["season"])
	assert.InDelta(t, float64(12), result[0]["interval_months"], 0.1)
	assert.NotEmpty(t, result[0]["id"])
}

// --- service-log ---

func TestShowServiceLogText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:           "Oil furnace",
		CategoryID:     cats[0].ID,
		IntervalMonths: 12,
	}))
	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)

	cost := int64(15000)
	require.NoError(t, store.CreateServiceLog(&data.ServiceLogEntry{
		MaintenanceItemID: items[0].ID,
		ServicedAt:        time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		CostCents:         &cost,
		Notes:             "annual service",
	}, data.Vendor{Name: "HeatPros"}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "service-log", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== SERVICE LOG ===")
	assert.Contains(t, out, "Oil furnace")
	assert.Contains(t, out, "HeatPros")
	assert.Contains(t, out, "2025-06-15")
	assert.Contains(t, out, "$150.00")
	assert.Contains(t, out, "annual service")
}

func TestShowServiceLogJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:           "Check sump pump",
		CategoryID:     cats[0].ID,
		IntervalMonths: 6,
	}))
	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)

	require.NoError(t, store.CreateServiceLog(&data.ServiceLogEntry{
		MaintenanceItemID: items[0].ID,
		ServicedAt:        time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
	}, data.Vendor{}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "service-log", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Check sump pump", result[0]["maintenance_item"])
	assert.NotEmpty(t, result[0]["id"])
}

// --- documents ---

func TestShowDocumentsText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.SetMaxDocumentSize(1<<30))

	require.NoError(t, store.CreateDocument(&data.Document{
		Title:      "Inspection Report",
		FileName:   "inspection.pdf",
		EntityKind: "project",
		EntityID:   "proj-001",
		MIMEType:   "application/pdf",
		SizeBytes:  204800,
		Notes:      "2025 inspection",
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "documents", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== DOCUMENTS ===")
	assert.Contains(t, out, "Inspection Report")
	assert.Contains(t, out, "inspection.pdf")
	assert.Contains(t, out, "project")
	assert.Contains(t, out, "application/pdf")
	assert.Contains(t, out, "204800")
}

func TestShowDocumentsJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.SetMaxDocumentSize(1<<30))

	require.NoError(t, store.CreateDocument(&data.Document{
		Title:      "Warranty Card",
		FileName:   "warranty.png",
		EntityKind: "appliance",
		EntityID:   "app-001",
		MIMEType:   "image/png",
		SizeBytes:  51200,
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "documents", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Warranty Card", result[0]["title"])
	assert.Equal(t, "warranty.png", result[0]["file_name"])
	assert.Equal(t, "appliance", result[0]["entity_kind"])
	assert.Equal(t, "app-001", result[0]["entity_id"])
	assert.Equal(t, "image/png", result[0]["mime_type"])
	assert.InDelta(t, float64(51200), result[0]["size_bytes"], 0.1)
	assert.NotEmpty(t, result[0]["id"])
}

// --- project-types ---

func TestShowProjectTypesText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "project-types", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== PROJECT TYPES ===")
	assert.Contains(t, out, "NAME")
}

func TestShowProjectTypesJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "project-types", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.NotEmpty(t, result)
	assert.NotEmpty(t, result[0]["name"])
	assert.NotEmpty(t, result[0]["id"])
}

// --- maintenance-categories ---

func TestShowMaintenanceCategoriesText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "maintenance-categories", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== MAINTENANCE CATEGORIES ===")
	assert.Contains(t, out, "NAME")
}

// --- deleted flag ---

func TestShowDeletedProjects(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Active",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))

	p2 := &data.Project{
		Title:         "Deleted",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusAbandoned,
	}
	require.NoError(t, store.CreateProject(p2))
	require.NoError(t, store.DeleteProject(p2.ID))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "projects", false, false))
	assert.Contains(t, buf.String(), "Active")
	assert.NotContains(t, buf.String(), "Deleted")

	buf.Reset()
	require.NoError(t, runShow(&buf, store, "projects", false, true))
	out := buf.String()
	assert.Contains(t, out, "Active")
	assert.Contains(t, out, "Deleted")
	assert.Contains(t, out, "DELETED")
}

func TestShowDeletedJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	p := &data.Project{
		Title:         "Gone",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))
	require.NoError(t, store.DeleteProject(p.ID))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "projects", true, true))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.NotNil(t, result[0]["deleted_at"])
}

func TestShowDeletedActiveHasNoDash(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "StillActive",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "projects", true, true))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Nil(t, result[0]["deleted_at"])
}

func TestShowMaintenanceCategoriesJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "maintenance-categories", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.NotEmpty(t, result)
	assert.NotEmpty(t, result[0]["name"])
	assert.NotEmpty(t, result[0]["id"])
}

// --- all ---

func TestShowAllText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Test Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Test Vendor"}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "all", false, false))

	out := buf.String()
	assert.Contains(t, out, "PROJECTS")
	assert.Contains(t, out, "Test Project")
	assert.Contains(t, out, "VENDORS")
	assert.Contains(t, out, "Test Vendor")
}

func TestShowAllJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Test Vendor"}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "all", true, false))

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Contains(t, result, "vendors")
	assert.Contains(t, result, "projects")
	assert.Contains(t, result, "project_types")
	assert.Contains(t, result, "quotes")
	assert.Contains(t, result, "maintenance")
	assert.Contains(t, result, "maintenance_categories")
	assert.Contains(t, result, "service_log")
	assert.Contains(t, result, "appliances")
	assert.Contains(t, result, "incidents")
	assert.Contains(t, result, "documents")

	vendors, ok := result["vendors"].([]any)
	require.True(t, ok)
	require.Len(t, vendors, 1)
	vendor, ok2 := vendors[0].(map[string]any)
	require.True(t, ok2)
	assert.Equal(t, "Test Vendor", vendor["name"])
}

func TestShowAllTextEmptyStore(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "all", false, false))

	// project-types and maintenance-categories are seeded, so output is non-empty,
	// but no user data sections should appear
	out := buf.String()
	assert.NotContains(t, out, "PROJECTS")
	assert.NotContains(t, out, "VENDORS")
}

func TestShowAllJSONNoHouse(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "all", true, false))

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.NotContains(t, result, "house")
}
