// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// helpSections returns the structured section data for the help overlay.
// Each section groups related key bindings under a titled header.
func (m *Model) helpSections() []helpSection {
	fromBinding := func(b key.Binding) helpEntry {
		h := b.Help()
		return helpEntry{keys: h.Key, desc: h.Desc}
	}
	return []helpSection{
		{
			title: "Global",
			entries: []helpEntry{
				fromBinding(m.keys.Cancel),
				fromBinding(m.keys.Quit),
			},
		},
		{
			title: "Nav Mode",
			entries: []helpEntry{
				{keyJ + "/" + keyK + "/" + symUp + "/" + symDown, "rows"},
				fromBinding(m.keys.ColLeft),
				fromBinding(m.keys.ColStart),
				{keyG + "/" + keyShiftG, "first/last row"},
				{keyD + "/" + keyU, "half page down/up"},
				fromBinding(m.keys.TabNext),
				fromBinding(m.keys.TabFirst),
				fromBinding(m.keys.Sort),
				fromBinding(m.keys.ToggleSettled),
				fromBinding(m.keys.DocSearch),
				fromBinding(m.keys.ColFinder),
				fromBinding(m.keys.ColHide),
				fromBinding(m.keys.FilterToggle),
				fromBinding(m.keys.FilterPin),
				fromBinding(m.keys.FilterNegate),
				fromBinding(m.keys.FilterClear),
				fromBinding(m.keys.Enter),
				fromBinding(m.keys.DocOpen),
				fromBinding(m.keys.HouseToggle),
				fromBinding(m.keys.ToggleUnits),
				fromBinding(m.keys.Dashboard),
				fromBinding(m.keys.Chat),
				fromBinding(m.keys.EnterEditMode),
				fromBinding(m.keys.Help),
				fromBinding(m.keys.Escape),
			},
		},
		{
			title: "Edit Mode",
			entries: []helpEntry{
				fromBinding(m.keys.Add),
				fromBinding(m.keys.QuickAdd),
				fromBinding(m.keys.EditCell),
				fromBinding(m.keys.EditFull),
				fromBinding(m.keys.Delete),
				fromBinding(m.keys.HardDelete),
				{keyCtrlD, "half page down"},
				fromBinding(m.keys.ShowDeleted),
				fromBinding(m.keys.HouseEdit),
				fromBinding(m.keys.ExitEdit),
			},
		},
		{
			title: "Forms",
			entries: []helpEntry{
				fromBinding(m.keys.FormNextField),
				fromBinding(m.keys.FormPrevField),
				{"1-9", "jump to Nth option"},
				fromBinding(m.keys.FormHiddenFiles),
				fromBinding(m.keys.FormEditor),
				fromBinding(m.keys.FormSave),
				fromBinding(m.keys.FormCancel),
			},
		},
		{
			title: "Chat (" + keyAt + ")",
			entries: []helpEntry{
				fromBinding(m.keys.ChatSend),
				fromBinding(m.keys.ChatToggleSQL),
				fromBinding(m.keys.ChatHistoryUp),
				fromBinding(m.keys.ChatHide),
			},
		},
	}
}

// helpContent generates the full help text as a single string.
// Used by tests that check for specific content across all sections.
func (m *Model) helpContent() string {
	sections := m.helpSections()

	// Pre-render all keycaps and find the global max width.
	type renderedSection struct {
		title string
		keys  []string
		descs []string
	}
	rendered := make([]renderedSection, len(sections))
	globalMaxKeyW := 0
	for i, section := range sections {
		rs := renderedSection{title: section.title}
		for _, e := range section.entries {
			k := m.renderKeysLight(e.keys)
			rs.keys = append(rs.keys, k)
			rs.descs = append(rs.descs, e.desc)
			if w := lipgloss.Width(k); w > globalMaxKeyW {
				globalMaxKeyW = w
			}
		}
		rendered[i] = rs
	}

	sep := m.styles.TextDim().Render(symVLine)
	var b strings.Builder
	b.WriteString(m.styles.HeaderTitle().Render(" Keyboard Shortcuts "))
	b.WriteString("\n\n")
	for i, rs := range rendered {
		b.WriteString(m.styles.HeaderSection().Render(" " + rs.title + " "))
		b.WriteString("\n")
		for j, keys := range rs.keys {
			pad := strings.Repeat(" ", max(0, globalMaxKeyW-lipgloss.Width(keys)))
			desc := m.styles.HeaderHint().Render(rs.descs[j])
			fmt.Fprintf(&b, "  %s%s %s %s\n", pad, keys, sep, desc)
		}
		if i < len(rendered)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// helpView renders the two-pane help overlay.
// Left pane: section names with cursor. Right pane: viewport of bindings.
func (m *Model) helpView() string {
	hs := m.helpState
	if hs == nil {
		return ""
	}

	sections := m.helpSections()

	// Build left pane: section list with cursor indicator.
	var leftLines []string
	for i, sec := range sections {
		cursor := "  "
		if i == hs.section {
			cursor = symTriRightSm + " "
		}
		line := cursor + m.styles.HeaderSection().Render(sec.title)
		leftLines = append(leftLines, line)
	}

	// Measure left pane width.
	leftW := 0
	for _, line := range leftLines {
		if w := lipgloss.Width(line); w > leftW {
			leftW = w
		}
	}

	// Right pane content from viewport.
	rightContent := hs.viewport.View()
	rightLines := strings.Split(rightContent, "\n")
	rightH := hs.viewport.Height()

	// Pad both panes to the same height.
	paneH := rightH
	if len(leftLines) > paneH {
		paneH = len(leftLines)
	}
	for len(leftLines) < paneH {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < paneH {
		rightLines = append(rightLines, "")
	}

	// Build combined pane rows with dim separator.
	dimSep := m.styles.TextDim().Render(" " + symVLine + " ")
	var paneRows []string
	for i := range paneH {
		left := leftLines[i]
		leftPad := strings.Repeat(" ", max(0, leftW-lipgloss.Width(left)))
		right := rightLines[i]
		paneRows = append(paneRows, left+leftPad+dimSep+right)
	}

	// Title.
	title := m.styles.HeaderTitle().Render(" Keyboard Shortcuts ")

	// Compute full content width for scroll rule.
	bodyStr := strings.Join(paneRows, "\n")
	contentW := 0
	for _, row := range paneRows {
		if w := lipgloss.Width(row); w > contentW {
			contentW = w
		}
	}
	if w := lipgloss.Width(title); w > contentW {
		contentW = w
	}

	// Scroll rule for the right pane.
	vp := &hs.viewport
	rule := m.scrollRule(contentW, vp.TotalLineCount(), vp.Height(),
		vp.AtTop(), vp.AtBottom(), vp.ScrollPercent(), symHLine)

	// Bottom hint bar.
	hints := []string{
		m.helpItem(keyJ+"/"+keyK, "sections"),
	}
	if vp.TotalLineCount() > vp.Height() {
		hints = append(hints, m.helpItem(keyG+"/"+keyShiftG, "top/bot"))
	}
	hints = append(hints, m.helpItem(keyEsc, "close"))
	hintStr := joinWithSeparator(m.helpSeparator(), hints...)

	return m.styles.OverlayBox().
		Render(title + "\n\n" + bodyStr + "\n\n" + rule + "\n" + hintStr)
}
