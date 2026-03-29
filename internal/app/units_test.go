// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToggleUnitSystemViaKeypress(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.hasHouse = true
	require.Equal(t, data.UnitsImperial, m.unitSystem, "should default to imperial")

	sendKey(m, "U")
	assert.Equal(t, data.UnitsMetric, m.unitSystem)
	assert.Contains(t, m.status.Text, "units: metric")

	sendKey(m, "U")
	assert.Equal(t, data.UnitsImperial, m.unitSystem)
	assert.Contains(t, m.status.Text, "units: imperial")
}

func TestToggleUnitSystemPersistsToStore(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	sendKey(m, "U")
	assert.Equal(t, data.UnitsMetric, m.unitSystem)

	// Verify persisted in the store.
	got, err := m.store.GetUnitSystem()
	require.NoError(t, err)
	assert.Equal(t, data.UnitsMetric, got)

	// Toggle back.
	sendKey(m, "U")
	assert.Equal(t, data.UnitsImperial, m.unitSystem)
	got, err = m.store.GetUnitSystem()
	require.NoError(t, err)
	assert.Equal(t, data.UnitsImperial, got)
}

func TestHouseDisplayUsesMetricUnits(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Set up a house with known area.
	m.house.SquareFeet = 1820
	m.house.LotSquareFeet = 7000
	m.hasHouse = true
	m.showHouse = true

	// In imperial mode, should show ft.
	view := m.houseExpanded()
	assert.Contains(t, view, "ft")

	// Switch to metric.
	sendKey(m, "U")
	require.Equal(t, data.UnitsMetric, m.unitSystem)

	view = m.houseExpanded()
	assert.Contains(t, view, "m\u00B2")
	assert.NotContains(t, view, "ft\u00B2")
}

func TestHouseFormMetricSavesAsSqFt(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Switch to metric first.
	sendKey(m, "U")
	require.Equal(t, data.UnitsMetric, m.unitSystem)

	// Open house form.
	openHouseForm(m)

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)

	// User enters 100 m^2.
	values.SquareFeet = "100"
	values.LotSquareFeet = "500"
	m.checkFormDirty()

	sendKey(m, "ctrl+s")

	// Verify stored values are in sq ft (not m^2).
	require.NoError(t, m.loadHouse())
	// 100 m^2 = ~1076 sq ft
	assert.True(t, m.house.SquareFeet >= 1075 && m.house.SquareFeet <= 1077,
		"expected ~1076 sq ft stored, got %d", m.house.SquareFeet)
	// 500 m^2 = ~5382 sq ft
	assert.True(t, m.house.LotSquareFeet >= 5381 && m.house.LotSquareFeet <= 5383,
		"expected ~5382 sq ft stored, got %d", m.house.LotSquareFeet)
}

func TestHouseFormShowsConvertedValues(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Set house with known sq ft values.
	m.house.SquareFeet = 1076 // ~100 m^2
	m.hasHouse = true
	require.NoError(t, m.store.UpdateHouseProfile(m.house))

	// Switch to metric.
	sendKey(m, "U")
	require.Equal(t, data.UnitsMetric, m.unitSystem)

	// Open house form.
	openHouseForm(m)

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)

	// The form should show the value converted to m^2.
	assert.Equal(t, "100", values.SquareFeet,
		"form should show ~100 m^2 for 1076 sq ft")
}

func TestHelpContentIncludesToggleUnits(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	help := m.helpContent()
	assert.Contains(t, help, "toggle units")
}
