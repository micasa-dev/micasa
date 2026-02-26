// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
	"golang.org/x/text/language"
)

// UnitSystem distinguishes imperial (sq ft) from metric (m^2) display.
type UnitSystem int

const (
	UnitsImperial UnitSystem = iota
	UnitsMetric
)

// sqFtPerSqM is the NIST conversion factor: 1 m^2 = 10.7639104 ft^2.
const sqFtPerSqM = 10.7639104

// symSuperTwo is the Unicode superscript 2 used in area labels.
const symSuperTwo = "\u00B2"

// String returns the canonical string representation stored in settings.
func (u UnitSystem) String() string {
	switch u {
	case UnitsImperial:
		return "imperial"
	case UnitsMetric:
		return "metric"
	default:
		return "imperial"
	}
}

// ParseUnitSystem parses a setting value into a UnitSystem.
// Returns UnitsImperial for unrecognized or empty input.
func ParseUnitSystem(s string) UnitSystem {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "metric":
		return UnitsMetric
	default:
		return UnitsImperial
	}
}

// SqFtToSqM converts square feet to square meters.
func SqFtToSqM(sqft float64) float64 {
	return sqft / sqFtPerSqM
}

// SqMToSqFt converts square meters to square feet.
func SqMToSqFt(sqm float64) float64 {
	return sqm * sqFtPerSqM
}

// SqFtToDisplayInt converts a stored sq ft value to the user's display unit.
func SqFtToDisplayInt(sqft int, u UnitSystem) int {
	if u == UnitsMetric {
		return int(SqFtToSqM(float64(sqft)) + 0.5)
	}
	return sqft
}

// DisplayIntToSqFt converts a user-entered display value back to sq ft for storage.
func DisplayIntToSqFt(display int, u UnitSystem) int {
	if u == UnitsMetric {
		return int(SqMToSqFt(float64(display)) + 0.5)
	}
	return display
}

// FormatArea formats a stored sq ft value for display in the user's unit system.
// Returns "" for zero values.
func FormatArea(sqft int, u UnitSystem) string {
	if sqft == 0 {
		return ""
	}
	switch u {
	case UnitsMetric:
		sqm := SqFtToDisplayInt(sqft, UnitsMetric)
		return fmt.Sprintf("%s m%s", humanize.Comma(int64(sqm)), symSuperTwo)
	default:
		return fmt.Sprintf("%s ft%s", humanize.Comma(int64(sqft)), symSuperTwo)
	}
}

// FormatLotArea formats a stored lot sq ft value with a "lot" suffix.
// Returns "" for zero values.
func FormatLotArea(sqft int, u UnitSystem) string {
	if sqft == 0 {
		return ""
	}
	switch u {
	case UnitsMetric:
		sqm := SqFtToDisplayInt(sqft, UnitsMetric)
		return fmt.Sprintf("%s m%s lot", humanize.Comma(int64(sqm)), symSuperTwo)
	default:
		return fmt.Sprintf("%s ft%s lot", humanize.Comma(int64(sqft)), symSuperTwo)
	}
}

// AreaFormTitle returns the form field title for the building area.
func AreaFormTitle(u UnitSystem) string {
	switch u {
	case UnitsMetric:
		return "Square meters"
	default:
		return "Square feet"
	}
}

// LotAreaFormTitle returns the form field title for the lot area.
func LotAreaFormTitle(u UnitSystem) string {
	switch u {
	case UnitsMetric:
		return "Lot square meters"
	default:
		return "Lot square feet"
	}
}

// AreaPlaceholder returns a placeholder value for the building area field.
func AreaPlaceholder(u UnitSystem) string {
	switch u {
	case UnitsMetric:
		return "169"
	default:
		return "1820"
	}
}

// LotAreaPlaceholder returns a placeholder value for the lot area field.
func LotAreaPlaceholder(u UnitSystem) string {
	switch u {
	case UnitsMetric:
		return "650"
	default:
		return "7000"
	}
}

// imperialRegions are ISO 3166-1 alpha-2 codes for regions that use
// imperial/US customary units for area measurement.
var imperialRegions = map[string]bool{
	"US": true,
	"LR": true,
	"MM": true,
}

// DefaultUnitSystem detects the user's preferred unit system from locale
// environment variables. Returns metric unless the region is US, LR, or MM.
func DefaultUnitSystem() UnitSystem {
	locale := os.Getenv("LC_ALL")
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	if locale == "" {
		return UnitsImperial
	}

	// Strip encoding suffix (e.g. ".UTF-8").
	if idx := strings.IndexByte(locale, '.'); idx >= 0 {
		locale = locale[:idx]
	}

	tag := language.Make(locale)
	region, _ := tag.Region()
	if region.String() == "" || region.String() == "ZZ" {
		return UnitsImperial
	}
	if imperialRegions[region.String()] {
		return UnitsImperial
	}
	return UnitsMetric
}
