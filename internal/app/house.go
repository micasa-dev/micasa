// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/locale"
)

func (m *Model) houseView() string {
	if !m.hasHouse {
		content := lipgloss.JoinVertical(
			lipgloss.Left,
			joinInline(
				m.styles.HeaderTitle().Render("House"),
				m.styles.HeaderBadge().Render("setup"),
				m.keycap("H"),
			),
			m.styles.HeaderHint().Render("Complete the form to add a house profile."),
		)
		return m.headerBox(content)
	}
	if m.showHouse {
		return m.headerBox(m.houseExpanded())
	}
	return m.headerBox(m.houseCollapsed())
}

func (m *Model) houseCollapsed() string {
	title := m.styles.HeaderTitle().Render("House")
	badge := m.styles.HeaderBadge().Render("▸")
	sep := m.styles.HeaderHint().Render(" · ")
	hint := m.styles.HeaderHint()
	val := m.styles.HeaderValue()
	stats := joinStyledParts(sep,
		styledPart(val, m.house.Nickname),
		styledPart(hint, formatCityState(m.house)),
		styledPart(hint, bedBathLabel(m.house)),
		styledPart(hint, data.FormatArea(m.house.SquareFeet, m.unitSystem)),
		styledPart(hint, formatInt(m.house.YearBuilt)),
	)
	return joinInline(title, badge) + "  " + stats
}

func (m *Model) houseExpanded() string {
	title := m.styles.HeaderTitle().Render("House")
	badge := m.styles.HeaderBadge().Render("▾")
	hint := m.styles.HeaderHint()
	val := m.styles.HeaderValue()
	sep := hint.Render(" · ")

	identity := joinStyledParts(sep,
		styledPart(val, m.house.Nickname),
		styledPart(hint, formatAddress(m.house)),
	)
	titleLine := joinInline(title, badge) + "  " + identity

	structNums := joinStyledParts(sep,
		styledPart(val, formatInt(m.house.YearBuilt)),
		styledPart(val, data.FormatArea(m.house.SquareFeet, m.unitSystem)),
		styledPart(val, data.FormatLotArea(m.house.LotSquareFeet, m.unitSystem)),
		styledPart(val, bedBathLabel(m.house)),
	)
	structMaterials := joinStyledParts(sep,
		m.hlv("fnd", m.house.FoundationType),
		m.hlv("wir", m.house.WiringType),
		m.hlv("roof", m.house.RoofType),
		m.hlv("ext", m.house.ExteriorType),
		m.hlv("bsmt", m.house.BasementType),
	)
	structure := m.houseSection("Structure", structNums, structMaterials)

	utilLine := joinStyledParts(sep,
		m.hlv("heat", m.house.HeatingType),
		m.hlv("cool", m.house.CoolingType),
		m.hlv("water", m.house.WaterSource),
		m.hlv("sewer", m.house.SewerType),
		m.hlv("park", m.house.ParkingType),
	)
	utilities := m.houseSection("Utilities", utilLine)

	finLine1 := joinStyledParts(sep,
		m.hlv("ins", m.house.InsuranceCarrier),
		m.hlv("policy", m.house.InsurancePolicy),
		m.hlv("renew", data.FormatDate(m.house.InsuranceRenewal)),
	)
	finLine2 := joinStyledParts(sep,
		m.hlv("tax", m.cur.FormatOptionalCents(m.house.PropertyTaxCents)),
		m.hlv("hoa", hoaSummary(m.house, m.cur)),
	)
	financial := m.houseSection("Financial", finLine1, finLine2)

	content := joinVerticalNonEmpty(titleLine, "", structure, utilities, financial)
	art := m.houseArt()
	if art == "" {
		return content
	}
	artGap := 3
	boxOverhead := m.styles.HeaderBox().GetHorizontalFrameSize()
	needed := lipgloss.Width(content) + artGap + lipgloss.Width(art) + boxOverhead
	if needed > m.effectiveWidth() {
		return content
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, content, "   ", art)
}

func (m *Model) headerBox(content string) string {
	return m.styles.HeaderBox().Render(content)
}

// houseArt renders a retro pixel-art house for the expanded profile.
// Uses shade characters (░▒▓█) for a classic DOS/BBS aesthetic.
// Returns "" if the terminal is too narrow.
func (m *Model) houseArt() string {
	if m.effectiveWidth() < 80 {
		return ""
	}
	rf := m.styles.HouseRoof()   // roof
	wl := m.styles.HouseWall()   // walls
	wn := m.styles.HouseWindow() // windows (lit)
	dr := m.styles.HouseDoor()   // door
	lines := []string{
		rf.Render("      ▄▓▄"),
		rf.Render("    ▄▓▓▓▓▓▄"),
		rf.Render("  ▄▓▓▓▓▓▓▓▓▓▄"),
		wl.Render("  ██ ") + wn.Render("░░") + wl.Render(" ") + wn.Render("░░") + wl.Render(" ██"),
		wl.Render("  ██  ") + dr.Render("████") + wl.Render(" ██"),
		wl.Render("  ██  ") + dr.Render("█  █") + wl.Render(" ██"),
		wl.Render("  ▀▀▀▀▀▀▀▀▀▀▀"),
	}
	return strings.Join(lines, "\n")
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

// hlv renders a dim label followed by a bright value, or "" if the value is blank.
func (m *Model) hlv(label, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return m.styles.HeaderLabel().Render(label) + " " + m.styles.HeaderValue().Render(value)
}

// houseSection renders a section header with values, indenting continuation lines.
func (m *Model) houseSection(header string, lines ...string) string {
	label := m.styles.HeaderSection().Render(header)
	labelWidth := lipgloss.Width(label)
	indent := strings.Repeat(" ", labelWidth+1)
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	result := make([]string, len(filtered))
	for i, line := range filtered {
		if i == 0 {
			result[i] = label + " " + line
		} else {
			result[i] = indent + line
		}
	}
	return strings.Join(result, "\n")
}

func bedBathLabel(profile data.HouseProfile) string {
	var parts []string
	if profile.Bedrooms > 0 {
		parts = append(parts, fmt.Sprintf("%dbd", profile.Bedrooms))
	}
	if profile.Bathrooms > 0 {
		parts = append(parts, fmt.Sprintf("%sba", formatFloat(profile.Bathrooms)))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " / ")
}

func formatInt(value int) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
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

func hoaSummary(profile data.HouseProfile, cur locale.Currency) string {
	if profile.HOAName == "" && profile.HOAFeeCents == nil {
		return ""
	}
	fee := cur.FormatOptionalCents(profile.HOAFeeCents)
	if profile.HOAName == "" {
		return fee
	}
	if fee == "" {
		return profile.HOAName
	}
	return fmt.Sprintf("%s (%s)", profile.HOAName, fee)
}
