// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"net/url"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// houseOverlayState holds cursor and edit state for the house profile overlay.
type houseOverlayState struct {
	section  int // 0=identity, 1=structure, 2=utilities, 3=financial
	row      int // cursor row within current section
	editing  bool
	form     *huh.Form
	formData *houseFormData
}

// houseProfileOverlay adapts houseOverlayState to the overlay interface.
type houseProfileOverlay struct{ m *Model }

func (o houseProfileOverlay) isVisible() bool { return o.m.houseOverlay != nil }

func (o houseProfileOverlay) hidesMainKeys() bool { return true }

func (o houseProfileOverlay) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	if o.m.houseOverlay.editing {
		return o.m.handleHouseEditKey(msg)
	}
	switch {
	case key.Matches(msg, o.m.keys.HouseDown):
		o.m.houseOverlayDown()
	case key.Matches(msg, o.m.keys.HouseUp):
		o.m.houseOverlayUp()
	case key.Matches(msg, o.m.keys.HouseRight):
		o.m.houseOverlayRight()
	case key.Matches(msg, o.m.keys.HouseLeft):
		o.m.houseOverlayLeft()
	case key.Matches(msg, o.m.keys.HouseClose):
		o.m.houseOverlay = nil
	case key.Matches(msg, o.m.keys.HouseToggle):
		o.m.houseOverlay = nil
		o.m.resizeTables()
	case msg.String() == keyEnter:
		o.m.houseOverlayStartEdit()
	}
	return nil
}

// handleHouseEditKey processes keys while an inline edit form is active.
func (m *Model) handleHouseEditKey(msg tea.KeyPressMsg) tea.Cmd {
	s := m.houseOverlay
	switch msg.String() {
	case keyEsc:
		m.houseOverlayCancelEdit()
		return nil
	case keyEnter:
		m.houseOverlaySubmitEdit()
		return nil
	}
	// Forward to huh form for text input handling.
	if s.form != nil {
		_, cmd := s.form.Update(msg)
		return cmd
	}
	return nil
}

// houseSectionLen returns the number of fields in the given section.
func houseSectionLen(sec houseSection) int {
	n := 0
	for _, d := range houseFieldDefs() {
		if d.section == sec {
			n++
		}
	}
	return n
}

// houseOverlayDown moves the cursor down within the current section.
// From identity, moves to the first structure field.
func (m *Model) houseOverlayDown() {
	s := m.houseOverlay
	if s.section == int(houseSectionIdentity) {
		s.section = int(houseSectionStructure)
		s.row = 0
		return
	}
	sec := houseSection(s.section)
	maxRow := houseSectionLen(sec) - 1
	if s.row < maxRow {
		s.row++
	}
}

// houseOverlayUp moves the cursor up within the current section.
// From row 0 in a grid section, moves to identity.
func (m *Model) houseOverlayUp() {
	s := m.houseOverlay
	if s.section == int(houseSectionIdentity) {
		// Already at top; clamp.
		return
	}
	if s.row > 0 {
		s.row--
		return
	}
	// Row 0 in grid section -> identity.
	s.section = int(houseSectionIdentity)
	s.row = 0
}

// houseOverlayRight moves the cursor right.
// In identity: cycles through identity fields.
// In grid: moves to next section, clamping row to target length.
func (m *Model) houseOverlayRight() {
	s := m.houseOverlay
	if s.section == int(houseSectionIdentity) {
		maxRow := houseSectionLen(houseSectionIdentity) - 1
		if s.row < maxRow {
			s.row++
		}
		return
	}
	if s.section >= int(houseSectionFinancial) {
		return // rightmost grid column
	}
	s.section++
	maxRow := houseSectionLen(houseSection(s.section)) - 1
	if s.row > maxRow {
		s.row = maxRow
	}
}

// houseOverlayLeft moves the cursor left.
// In identity: cycles backward through identity fields.
// In grid: moves to previous section (or identity from structure).
func (m *Model) houseOverlayLeft() {
	s := m.houseOverlay
	if s.section == int(houseSectionIdentity) {
		if s.row > 0 {
			s.row--
		}
		return
	}
	if s.section <= int(houseSectionStructure) {
		// Structure -> identity.
		s.section = int(houseSectionIdentity)
		s.row = 0
		return
	}
	s.section--
	maxRow := houseSectionLen(houseSection(s.section)) - 1
	if s.row > maxRow {
		s.row = maxRow
	}
}

// houseOverlayCurrentDef returns the field definition at the current cursor
// position, or nil if no field matches.
func (m *Model) houseOverlayCurrentDef() *houseFieldDef {
	s := m.houseOverlay
	sec := houseSection(s.section)
	defs := houseFieldDefs()
	row := 0
	for i := range defs {
		if defs[i].section != sec {
			continue
		}
		if row == s.row {
			return &defs[i]
		}
		row++
	}
	return nil
}

// houseOverlayStartEdit opens a single-field huh form for the focused field.
func (m *Model) houseOverlayStartEdit() {
	s := m.houseOverlay
	def := m.houseOverlayCurrentDef()
	if def == nil {
		return
	}
	fd := m.houseFormValues(m.house)
	field := def.build(m, def.ptr(fd))
	form := huh.NewForm(huh.NewGroup(field))
	form.WithWidth(m.houseOverlayFieldWidth())
	form.Init()
	s.form = form
	s.formData = fd
	s.editing = true
}

// houseOverlayCancelEdit discards the edit and returns to browse mode.
func (m *Model) houseOverlayCancelEdit() {
	s := m.houseOverlay
	s.editing = false
	s.form = nil
	s.formData = nil
}

// houseOverlaySubmitEdit validates, parses, and saves the edited field value.
func (m *Model) houseOverlaySubmitEdit() {
	s := m.houseOverlay
	if s.form == nil || s.formData == nil {
		return
	}
	// Run field validation before saving.
	def := m.houseOverlayCurrentDef()
	if def == nil {
		m.houseOverlayCancelEdit()
		return
	}
	val := strings.TrimSpace(*def.ptr(s.formData))
	*def.ptr(s.formData) = val

	if err := m.saveHouseFormData(s.formData); err != nil {
		m.setStatusError(err.Error())
		return
	}
	s.editing = false
	s.form = nil
	s.formData = nil
}

// houseOverlayFieldWidth returns the column width for inline edit fields.
func (m *Model) houseOverlayFieldWidth() int {
	contentW := m.houseOverlayWidth()
	innerW := contentW - m.styles.OverlayBox().GetHorizontalFrameSize()
	if m.houseOverlayIsNarrow() {
		return max(innerW, 12)
	}
	colGap := 3
	colW := (innerW - colGap*2) / 3
	return max(colW, 12)
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
	var hints string
	if m.houseOverlay.editing {
		hints = joinWithSeparator(m.helpSeparator(),
			m.helpItem(symReturn, "confirm"),
			m.helpItem(keyEsc, "cancel"),
		)
	} else {
		hints = joinWithSeparator(m.helpSeparator(),
			m.helpItem(symUp+symDown, "navigate"),
			m.helpItem(symLeft+symRight, "section"),
			m.helpItem(symReturn, "edit"),
			m.helpItem(keyEsc, "close"),
		)
	}

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

// houseOverlayNarrowThreshold is the overlay content width below which
// sections stack vertically instead of rendering side-by-side.
const houseOverlayNarrowThreshold = 80

// houseOverlayIsNarrow returns true when the overlay should use stacked layout.
func (m *Model) houseOverlayIsNarrow() bool {
	return m.houseOverlayWidth() < houseOverlayNarrowThreshold
}

// houseOverlayColumns renders the three-column grid (Structure, Utilities,
// Financial) with label/value rows. When the overlay is narrow, sections
// stack vertically instead of side-by-side.
func (m *Model) houseOverlayColumns(innerW int) string {
	sections, gridSections := m.houseOverlaySectionData()

	narrow := m.houseOverlayIsNarrow()
	colW := innerW
	if !narrow {
		colGap := 3
		colW = (innerW - colGap*(len(sections)-1)) / len(sections)
		colW = max(colW, 12)
	}

	rendered := m.houseOverlayRenderSections(sections, gridSections, colW)

	if narrow {
		return strings.Join(rendered, "\n")
	}

	// Side-by-side: pad shorter columns to align heights.
	maxLines := 0
	for _, r := range rendered {
		h := strings.Count(r, "\n") + 1
		if h > maxLines {
			maxLines = h
		}
	}
	for i := range rendered {
		h := strings.Count(rendered[i], "\n") + 1
		if h < maxLines {
			rendered[i] += strings.Repeat("\n", maxLines-h)
		}
	}

	gap := strings.Repeat(" ", 3)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		rendered[0], gap, rendered[1], gap, rendered[2],
	)
}

// houseOverlaySectionData groups field definitions by section, returning
// display data and the corresponding houseSection enum values.
func (m *Model) houseOverlaySectionData() ([]houseOverlaySec, []houseSection) {
	defs := houseFieldDefs()
	sections := []houseOverlaySec{
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
	gridSections := []houseSection{
		houseSectionStructure, houseSectionUtilities, houseSectionFinancial,
	}
	return sections, gridSections
}

// houseOverlaySec holds a section title and its field definitions.
type houseOverlaySec struct {
	title  string
	fields []houseFieldDef
}

// houseOverlayRenderSections renders each section as a block of lines.
func (m *Model) houseOverlayRenderSections(
	sections []houseOverlaySec,
	gridSections []houseSection,
	colW int,
) []string {
	hint := m.styles.HeaderHint()
	val := m.styles.HeaderValue()
	warn := m.styles.Warning()
	section := m.styles.HeaderSection()
	s := m.houseOverlay

	rendered := make([]string, len(sections))
	for i, sec := range sections {
		lines := []string{
			section.Render(sec.title),
			hint.Render(strings.Repeat("─", colW)),
		}

		for rowIdx, f := range sec.fields {
			isFocused := s.section == int(gridSections[i]) && s.row == rowIdx
			if isFocused && s.editing && s.form != nil {
				lines = append(lines, s.form.View())
			} else {
				v := strings.TrimSpace(f.get(m.house, m.cur, m.unitSystem))
				label := hint.Render(f.label)
				var line string
				if v == "" {
					line = label + "  " + warn.Render("—")
				} else {
					line = label + "  " + val.Render(v)
				}
				lines = append(lines, m.zones.Mark(zoneHouseField+f.key, line))
			}
		}
		rendered[i] = strings.Join(lines, "\n")
	}
	return rendered
}
