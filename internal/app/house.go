// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/locale"
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
	if m.showHouse {
		return m.zones.Mark(zoneHouse, m.headerBox(m.houseExpanded()))
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

func (m *Model) houseExpanded() string {
	title := m.housePill()
	badge := m.styles.HeaderBadge().Render("▾")
	hint := m.styles.HeaderHint()
	val := m.styles.HeaderValue()
	sep := hint.Render(" · ")

	addr := formatAddress(m.house)
	if addr != "" {
		mapsURL := "https://maps.google.com/maps?q=" + url.QueryEscape(addr)
		addr = osc8Link(mapsURL, addr)
	}
	titleLine := joinInline(title, badge)
	if addr != "" {
		titleLine += "  " + styledPart(hint, addr)
	}

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
		parts = append(parts, strconv.Itoa(profile.Bedrooms)+"bd")
	}
	if profile.Bathrooms > 0 {
		parts = append(parts, formatFloat(profile.Bathrooms)+"ba")
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
