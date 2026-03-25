// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"bytes"
	"encoding/json"
	"testing"

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

	for _, entity := range []string{"projects", "vendors", "appliances", "incidents"} {
		var buf bytes.Buffer
		require.NoError(t, runShow(&buf, store, entity, false, false))
		assert.Empty(t, buf.String(), "expected no output for empty %s", entity)
	}
}

func TestShowEmptyCollectionJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	for _, entity := range []string{"projects", "vendors", "appliances", "incidents"} {
		var buf bytes.Buffer
		require.NoError(t, runShow(&buf, store, entity, true, false))

		var result []map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
		assert.Empty(t, result, "expected empty array for %s", entity)
	}
}
