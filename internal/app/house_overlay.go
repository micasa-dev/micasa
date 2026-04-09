// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"net/url"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// houseOverlayState holds cursor and edit state for the house profile overlay.
type houseOverlayState struct {
	section int // 0=identity, 1=structure, 2=utilities, 3=financial
	row     int // cursor row within current section
}

// houseProfileOverlay adapts houseOverlayState to the overlay interface.
type houseProfileOverlay struct{ m *Model }

func (o houseProfileOverlay) isVisible() bool { return o.m.houseOverlay != nil }

func (o houseProfileOverlay) hidesMainKeys() bool { return true }

func (o houseProfileOverlay) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, o.m.keys.HouseToggle):
		o.m.houseOverlay = nil
		o.m.resizeTables()
	case key.Matches(msg, o.m.keys.Escape):
		o.m.houseOverlay = nil
	}
	return nil
}

// buildHouseOverlay renders the three-column house profile overlay.
func (m *Model) buildHouseOverlay() string {
	contentW := m.houseOverlayWidth()
	innerW := contentW - m.styles.OverlayBox().GetHorizontalFrameSize()

	// Identity line: nickname pill + address + completion fraction.
	identity := m.houseOverlayIdentity(innerW)

	// Group fields by section (skip identity — shown in header).
	columns := m.houseOverlayColumns(innerW)

	// Hint bar.
	hints := joinWithSeparator(m.helpSeparator(),
		m.helpItem(keyTab, "close"),
		m.helpItem(keyEsc, "close"),
	)

	rule := m.styles.DashRule().Render(strings.Repeat("─", innerW))
	boxContent := lipgloss.JoinVertical(lipgloss.Left,
		identity, rule, columns, "", hints,
	)

	return m.styles.OverlayBox().
		Width(contentW).
		MaxHeight(m.overlayMaxHeight()).
		Render(boxContent)
}

// houseOverlayWidth returns a wider content width for the house overlay
// since it has three columns.
func (m *Model) houseOverlayWidth() int {
	w := min(m.effectiveWidth()-8, 90)
	w = max(w, 40)
	return w
}

// houseOverlayIdentity renders the top identity line with pill, address, and
// completion count.
func (m *Model) houseOverlayIdentity(innerW int) string {
	pill := m.housePill()
	hint := m.styles.HeaderHint()
	val := m.styles.HeaderValue()

	addr := formatAddress(m.house)
	if addr != "" {
		mapsURL := "https://maps.google.com/maps?q=" + url.QueryEscape(addr)
		addr = osc8Link(mapsURL, addr)
	}

	left := pill
	if addr != "" {
		left += "  " + hint.Render(addr)
	}

	// Completion fraction.
	total := len(houseFieldDefs())
	empty := houseEmptyFieldCount(m.house, m.cur, m.unitSystem)
	filled := total - empty
	frac := val.Render(fmt.Sprintf("%d/%d", filled, total))
	if empty > 0 {
		frac = m.styles.Warning().Render(fmt.Sprintf("%d/%d", filled, total))
	}

	leftW := lipgloss.Width(left)
	fracW := lipgloss.Width(frac)
	gap := innerW - leftW - fracW
	if gap < 1 {
		return left + " " + frac
	}
	return left + strings.Repeat(" ", gap) + frac
}

// houseOverlayColumns renders the three-column grid (Structure, Utilities,
// Financial) with label/value rows.
func (m *Model) houseOverlayColumns(innerW int) string {
	defs := houseFieldDefs()
	hint := m.styles.HeaderHint()
	val := m.styles.HeaderValue()
	warn := m.styles.Warning()
	section := m.styles.HeaderSection()

	// Group fields by section, skip identity (shown in header line).
	type sectionData struct {
		title  string
		fields []houseFieldDef
	}
	sections := []sectionData{
		{houseSectionStructure.title(), nil},
		{houseSectionUtilities.title(), nil},
		{houseSectionFinancial.title(), nil},
	}
	for _, d := range defs {
		switch d.section {
		case houseSectionIdentity:
			// Identity fields shown in header line, not in columns.
		case houseSectionStructure:
			sections[0].fields = append(sections[0].fields, d)
		case houseSectionUtilities:
			sections[1].fields = append(sections[1].fields, d)
		case houseSectionFinancial:
			sections[2].fields = append(sections[2].fields, d)
		}
	}

	// Render each section as a column.
	colGap := 3
	colW := (innerW - colGap*(len(sections)-1)) / len(sections)
	colW = max(colW, 12)

	rendered := make([]string, len(sections))
	maxLines := 0
	for i, sec := range sections {
		lines := []string{
			section.Render(sec.title),
			hint.Render(strings.Repeat("─", colW)),
		}

		for _, f := range sec.fields {
			v := strings.TrimSpace(f.get(m.house, m.cur, m.unitSystem))
			label := hint.Render(f.label)
			if v == "" {
				lines = append(lines, label+"  "+warn.Render("—"))
			} else {
				lines = append(lines, label+"  "+val.Render(v))
			}
		}
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
		rendered[i] = strings.Join(lines, "\n")
	}

	// Pad shorter columns to align heights.
	for i := range rendered {
		h := strings.Count(rendered[i], "\n") + 1
		if h < maxLines {
			rendered[i] += strings.Repeat("\n", maxLines-h)
		}
	}

	gap := strings.Repeat(" ", colGap)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		rendered[0], gap, rendered[1], gap, rendered[2],
	)
}
