// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package fake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDeterministicSeed(t *testing.T) {
	t.Parallel()
	h1 := New(42)
	h2 := New(42)

	v1 := h1.Vendor()
	v2 := h2.Vendor()
	assert.Equal(t, v1.Name, v2.Name, "same seed should produce identical vendors")
}

func TestHouseProfile(t *testing.T) {
	t.Parallel()
	h := New(1)
	house := h.HouseProfile()

	assert.NotEmpty(t, house.Nickname)
	assert.NotEmpty(t, house.City)
	assert.GreaterOrEqual(t, house.YearBuilt, 1920)
	assert.LessOrEqual(t, house.YearBuilt, 2024)
	assert.GreaterOrEqual(t, house.SquareFeet, 800)
	assert.LessOrEqual(t, house.SquareFeet, 4500)
	assert.GreaterOrEqual(t, house.Bedrooms, 1)
	assert.LessOrEqual(t, house.Bedrooms, 6)
	assert.NotNil(t, house.InsuranceRenewal)
}

func TestVendor(t *testing.T) {
	t.Parallel()
	h := New(2)
	v := h.Vendor()

	assert.NotEmpty(t, v.Name)
	assert.NotEmpty(t, v.ContactName)
	assert.NotEmpty(t, v.Phone)
	assert.NotEmpty(t, v.Email)
}

func TestVendorForTrade(t *testing.T) {
	t.Parallel()
	h := New(3)
	v := h.VendorForTrade("Plumbing")
	assert.NotEmpty(t, v.Name)
}

func TestProject(t *testing.T) {
	t.Parallel()
	h := New(4)

	for _, typeName := range ProjectTypes() {
		p := h.Project(typeName)
		assert.NotEmpty(t, p.Title, "type %q", typeName)
		assert.Equal(t, typeName, p.TypeName)
		assert.NotEmpty(t, p.Description, "type %q", typeName)
	}
}

func TestProjectCompletedHasEndDateAndActual(t *testing.T) {
	t.Parallel()
	for seed := range uint64(100) {
		h := New(seed)
		p := h.Project("Plumbing")
		if p.Status == StatusCompleted {
			require.NotNil(t, p.EndDate, "completed project should have end date")
			require.NotNil(t, p.ActualCents, "completed project should have actual cost")
			return
		}
	}
	t.Skip("never hit completed status in 100 seeds")
}

func TestProjectUnknownType(t *testing.T) {
	t.Parallel()
	h := New(5)
	p := h.Project("Unknown")
	assert.NotEmpty(t, p.Title, "expected fallback title for unknown type")
}

func TestAppliance(t *testing.T) {
	t.Parallel()
	h := New(6)
	a := h.Appliance()

	assert.NotEmpty(t, a.Name)
	assert.NotEmpty(t, a.Brand)
	assert.NotEmpty(t, a.ModelNumber)
	assert.NotEmpty(t, a.SerialNumber)
	assert.NotEmpty(t, a.Location)
	assert.NotNil(t, a.PurchaseDate)
	assert.NotNil(t, a.CostCents)
}

func TestBrandPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		brand string
		want  string
	}{
		{"Frostline", "FR"},
		{"東芝", "東芝"},
		{"Electrolux®", "EL"},
		{"AquaMax", "AQ"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, brandPrefix(tt.brand), "brand %q", tt.brand)
	}
}

func TestMaintenanceItem(t *testing.T) {
	t.Parallel()
	h := New(7)

	for _, cat := range MaintenanceCategories() {
		m := h.MaintenanceItem(cat)
		assert.NotEmpty(t, m.Name, "category %q", cat)
		assert.Positive(t, m.IntervalMonths, "category %q", cat)
	}
}

func TestMaintenanceItemUnknownCategory(t *testing.T) {
	t.Parallel()
	h := New(8)
	m := h.MaintenanceItem("Unknown")
	assert.NotEmpty(t, m.Name)
	assert.Equal(t, 12, m.IntervalMonths)
}

func TestServiceLogEntry(t *testing.T) {
	t.Parallel()
	h := New(9)
	e := h.ServiceLogEntry()

	assert.NotZero(t, e.ServicedAt)
	assert.NotNil(t, e.CostCents)
	assert.NotEmpty(t, e.Notes)
}

func TestQuote(t *testing.T) {
	t.Parallel()
	h := New(10)
	q := h.Quote()

	assert.Positive(t, q.TotalCents)
	require.NotNil(t, q.LaborCents)
	require.NotNil(t, q.MaterialsCents)
	assert.Equal(t, q.TotalCents, *q.LaborCents+*q.MaterialsCents)
	assert.NotNil(t, q.ReceivedDate)
}

func TestVarietyAcrossSeeds(t *testing.T) {
	t.Parallel()
	names := map[string]bool{}
	for seed := range uint64(20) {
		h := New(seed)
		v := h.Vendor()
		names[v.Name] = true
	}
	assert.GreaterOrEqual(t, len(names), 10, "expected variety from 20 seeds")
}

func TestVendorTrades(t *testing.T) {
	t.Parallel()
	trades := VendorTrades()
	assert.NotEmpty(t, trades)
}

func TestIntN(t *testing.T) {
	t.Parallel()
	h := New(42)
	for range 100 {
		v := h.IntN(5)
		assert.GreaterOrEqual(t, v, 0)
		assert.Less(t, v, 5)
	}
}
