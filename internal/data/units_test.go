// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnitSystemString(t *testing.T) {
	assert.Equal(t, "imperial", UnitsImperial.String())
	assert.Equal(t, "metric", UnitsMetric.String())
}

func TestParseUnitSystem(t *testing.T) {
	assert.Equal(t, UnitsImperial, ParseUnitSystem("imperial"))
	assert.Equal(t, UnitsMetric, ParseUnitSystem("metric"))
	assert.Equal(t, UnitsMetric, ParseUnitSystem("Metric"))
	assert.Equal(t, UnitsMetric, ParseUnitSystem(" metric "))
	assert.Equal(t, UnitsImperial, ParseUnitSystem(""))
	assert.Equal(t, UnitsImperial, ParseUnitSystem("garbage"))
}

func TestSqFtToSqMRoundTrip(t *testing.T) {
	original := 1820.0
	sqm := SqFtToSqM(original)
	roundTripped := SqMToSqFt(sqm)
	assert.InDelta(t, original, roundTripped, 0.01, "round-trip should be near-lossless")
}

func TestSqFtToSqMKnownValue(t *testing.T) {
	// 1000 sq ft = ~92.9 m^2
	sqm := SqFtToSqM(1000)
	assert.InDelta(t, 92.9, sqm, 0.1)
}

func TestSqFtToDisplayIntImperial(t *testing.T) {
	assert.Equal(t, 1820, SqFtToDisplayInt(1820, UnitsImperial))
}

func TestSqFtToDisplayIntMetric(t *testing.T) {
	got := SqFtToDisplayInt(1820, UnitsMetric)
	// 1820 sq ft = ~169 m^2
	assert.True(t, got >= 168 && got <= 170,
		"expected ~169 m^2, got %d", got)
}

func TestDisplayIntToSqFtImperial(t *testing.T) {
	assert.Equal(t, 1820, DisplayIntToSqFt(1820, UnitsImperial))
}

func TestDisplayIntToSqFtMetric(t *testing.T) {
	got := DisplayIntToSqFt(169, UnitsMetric)
	// 169 m^2 = ~1819 sq ft
	assert.True(t, got >= 1818 && got <= 1820,
		"expected ~1819 sq ft, got %d", got)
}

func TestDisplayRoundTrip(t *testing.T) {
	original := 2000
	display := SqFtToDisplayInt(original, UnitsMetric)
	require.NotZero(t, display)
	roundTripped := DisplayIntToSqFt(display, UnitsMetric)
	// Two integer rounding steps (sqft->m2->sqft) can each add +/-0.5,
	// yielding up to +/-1 per step, so allow +/-2 total.
	assert.True(t, math.Abs(float64(original-roundTripped)) <= 2,
		"round-trip %d -> %d -> %d drift too large", original, display, roundTripped)
}

func TestFormatAreaImperial(t *testing.T) {
	assert.Equal(t, "1,820 ft\u00B2", FormatArea(1820, UnitsImperial))
	assert.Equal(t, "", FormatArea(0, UnitsImperial))
}

func TestFormatAreaMetric(t *testing.T) {
	result := FormatArea(1820, UnitsMetric)
	assert.Contains(t, result, "m\u00B2")
	assert.NotContains(t, result, "ft")
	assert.Equal(t, "", FormatArea(0, UnitsMetric))
}

func TestFormatLotAreaImperial(t *testing.T) {
	assert.Equal(t, "7,000 ft\u00B2 lot", FormatLotArea(7000, UnitsImperial))
	assert.Equal(t, "", FormatLotArea(0, UnitsImperial))
}

func TestFormatLotAreaMetric(t *testing.T) {
	result := FormatLotArea(7000, UnitsMetric)
	assert.Contains(t, result, "m\u00B2 lot")
	assert.NotContains(t, result, "ft")
	assert.Equal(t, "", FormatLotArea(0, UnitsMetric))
}

func TestFormTitlesAndPlaceholders(t *testing.T) {
	assert.Equal(t, "Square feet", AreaFormTitle(UnitsImperial))
	assert.Equal(t, "Square meters", AreaFormTitle(UnitsMetric))
	assert.Equal(t, "Lot square feet", LotAreaFormTitle(UnitsImperial))
	assert.Equal(t, "Lot square meters", LotAreaFormTitle(UnitsMetric))
	assert.Equal(t, "1820", AreaPlaceholder(UnitsImperial))
	assert.Equal(t, "169", AreaPlaceholder(UnitsMetric))
	assert.Equal(t, "7000", LotAreaPlaceholder(UnitsImperial))
	assert.Equal(t, "650", LotAreaPlaceholder(UnitsMetric))
}

func TestDefaultUnitSystemUSLocale(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "en_US.UTF-8")
	assert.Equal(t, UnitsImperial, DefaultUnitSystem())
}

func TestDefaultUnitSystemGermanLocale(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "de_DE.UTF-8")
	assert.Equal(t, UnitsMetric, DefaultUnitSystem())
}

func TestDefaultUnitSystemFrenchLocale(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "fr_FR.UTF-8")
	assert.Equal(t, UnitsMetric, DefaultUnitSystem())
}

func TestDefaultUnitSystemJapaneseLocale(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "ja_JP.UTF-8")
	assert.Equal(t, UnitsMetric, DefaultUnitSystem())
}

func TestDefaultUnitSystemEmptyLocale(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "")
	assert.Equal(t, UnitsImperial, DefaultUnitSystem())
}

func TestDefaultUnitSystemLCAllOverrides(t *testing.T) {
	t.Setenv("LC_ALL", "de_DE.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	assert.Equal(t, UnitsMetric, DefaultUnitSystem(),
		"LC_ALL should take precedence over LANG")
}

func TestDefaultUnitSystemLiberiaIsImperial(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "en_LR.UTF-8")
	assert.Equal(t, UnitsImperial, DefaultUnitSystem())
}
