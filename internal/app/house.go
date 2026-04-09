// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/micasa-dev/micasa/internal/data"
)

func (m *Model) houseView() string {
	if !m.hasHouse {
		content := lipgloss.JoinVertical(
			lipgloss.Left,
			joinInline(
				m.houseTitle(),
				m.styles.HeaderBadge().Render("setup"),
				m.keycap("H"),
			),
			m.styles.HeaderHint().Render("Complete the form to add a house profile."),
		)
		return m.zones.Mark(zoneHouse, m.headerBox(content))
	}
	return m.zones.Mark(zoneHouse, m.headerBox(m.houseCollapsed()))
}

// housePill renders the nickname (or "House" when no profile) in pill style.
// Uses AccentOutline when an overlay is active, HeaderTitle (accent pill) otherwise.
func (m *Model) housePill() string {
	label := "House"
	if m.hasHouse && m.house.Nickname != "" {
		label = m.house.Nickname
	}
	if m.hasActiveOverlay() {
		return m.styles.AccentOutline().Render(label)
	}
	return m.styles.HeaderTitle().Render(label)
}

// houseTitle is a backward-compatible alias used by houseView for no-house state.
func (m *Model) houseTitle() string {
	if m.hasActiveOverlay() {
		return m.styles.AccentOutline().Render("House")
	}
	return m.styles.HeaderTitle().Render("House")
}

func (m *Model) houseCollapsed() string {
	pill := m.housePill()
	badge := m.styles.HeaderBadge().Render("▸")
	sep := m.styles.HeaderHint().Render(" · ")
	hint := m.styles.HeaderHint()
	val := m.styles.HeaderValue()

	vitals := joinStyledParts(sep,
		styledPart(hint, formatCityState(m.house)),
		m.collapsedBedBath(),
		m.collapsedSqft(),
		styledPart(val, formatInt(m.house.YearBuilt)),
	)

	line := joinInline(pill, badge) + "  " + vitals

	empty := houseEmptyFieldCount(m.house, m.cur, m.unitSystem)
	if empty > 0 {
		warn := m.styles.Warning().Render(fmt.Sprintf("○ %d", empty))
		line += hint.Render(" · ") + warn
	}
	return line
}

// collapsedBedBath renders bed/bath with bright values and dim suffixes.
func (m *Model) collapsedBedBath() string {
	val := m.styles.HeaderValue()
	hint := m.styles.HeaderHint()
	var parts []string
	if m.house.Bedrooms > 0 {
		parts = append(parts, val.Render(strconv.Itoa(m.house.Bedrooms))+hint.Render("bd"))
	}
	if m.house.Bathrooms > 0 {
		parts = append(parts, val.Render(formatFloat(m.house.Bathrooms))+hint.Render("ba"))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, hint.Render(" / "))
}

// collapsedSqft renders k-formatted sqft with bright value and dim "sf" suffix.
func (m *Model) collapsedSqft() string {
	displaySqft := data.SqFtToDisplayInt(m.house.SquareFeet, m.unitSystem)
	formatted := formatKSqft(displaySqft)
	if formatted == "" {
		return ""
	}
	suffix := "sf"
	if m.unitSystem == data.UnitsMetric {
		suffix = "m\u00B2"
	}
	return m.styles.HeaderValue().Render(formatted) + m.styles.HeaderHint().Render(suffix)
}

func (m *Model) headerBox(content string) string {
	return m.styles.HeaderBox().Render(content)
}

// styledPart returns a styled value, or "" if the underlying value is blank.
func styledPart(style lipgloss.Style, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return style.Render(value)
}

// joinStyledParts joins pre-styled parts with a separator, skipping empty ones.
func joinStyledParts(sep string, parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, sep)
}

func formatInt(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func formatFloat(value float64) string {
	if value == 0 {
		return ""
	}
	if value == math.Trunc(value) {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.1f", value)
}

func formatCityState(profile data.HouseProfile) string {
	parts := []string{
		strings.TrimSpace(profile.City),
		strings.TrimSpace(profile.State),
	}
	return joinWithSeparator(", ", parts...)
}

func formatAddress(profile data.HouseProfile) string {
	parts := []string{
		strings.TrimSpace(profile.AddressLine1),
		strings.TrimSpace(profile.AddressLine2),
		strings.TrimSpace(profile.City),
		strings.TrimSpace(profile.State),
		strings.TrimSpace(profile.PostalCode),
	}
	return joinWithSeparator(", ", parts...)
}
