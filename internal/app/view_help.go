// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"strings"
)

// helpContent generates the static help text (keyboard shortcuts).
// Separated from rendering so it can be set once on the viewport.
func (m *Model) helpContent() string {
	type binding struct {
		key  string
		desc string
	}
	sections := []struct {
		title    string
		bindings []binding
	}{
		{
			title: "Global",
			bindings: []binding{
				{keyCtrlC, "Cancel LLM operation"},
				{keyCtrlQ, "Quit"},
			},
		},
		{
			title: "Nav Mode",
			bindings: []binding{
				{keyJ + "/" + keyK + "/" + symUp + "/" + symDown, "Rows"},
				{keyH + "/" + keyL + "/" + symLeft + "/" + symRight, "Columns"},
				{keyCaret + "/" + keyDollar, "First/last column"},
				{keyG + "/" + keyShiftG, "First/last row"},
				{keyD + "/" + keyU, "Half page down/up"},
				{keyB + "/" + keyF, "Switch tabs"},
				{keyShiftB + "/" + keyShiftF, "First/last tab"},
				{keyS + "/" + keyShiftS, "Sort / clear sorts"},
				{keyT, "Toggle settled projects"},
				{keyCtrlF, "Search documents"},
				{keySlash, "Find column"},
				{keyC + "/" + keyShiftC, "Toggle column visibility"},
				{keyShiftN, "Toggle filter"},
				{keyN, "Pin/unpin"},
				{keyBang, "Invert filter"},
				{keyCtrlN, "Clear pins and filter"},
				{symReturn, drilldownArrow + " drill / " + linkArrow + " follow / preview"},
				{keyO, "Open document"},
				{keyTab, "House profile"},
				{keyShiftU, "Toggle units"},
				{keyShiftD, "Summary"},
				{keyAt, "Ask LLM"},
				{keyI, "Edit mode"},
				{keyQuestion, "Help"},
				{keyEsc, "Close detail / clear status"},
			},
		},
		{
			title: "Edit Mode",
			bindings: []binding{
				{keyA, "Add entry"},
				{keyShiftA, "Add document with extraction"},
				{keyE, "Edit cell or row"},
				{keyShiftE, "Edit row (full form)"},
				{keyD, "Delete / restore"},
				{keyShiftD, "Permanently delete (incidents)"},
				{keyCtrlD, "Half page down"},
				{keyX, "Show deleted"},
				{keyP, "House profile"},
				{keyEsc, "Nav mode"},
			},
		},
		{
			title: "Forms",
			bindings: []binding{
				{keyTab, "Next field"},
				{keyShiftTab, "Previous field"},
				{"1-9", "Jump to Nth option"},
				{keyShiftH, "Toggle hidden files (file picker)"},
				{keyCtrlE, "Open notes in $EDITOR"},
				{keyCtrlS, "Save"},
				{keyEsc, "Cancel"},
			},
		},
		{
			title: "Chat (" + keyAt + ")",
			bindings: []binding{
				{symReturn, "Send message"},
				{keyCtrlS, "Toggle SQL display"},
				{symUp + "/" + symDown, "Prompt history"},
				{keyEsc, "Hide chat"},
			},
		},
	}

	var b strings.Builder
	b.WriteString(m.styles.HeaderTitle().Render(" Keyboard Shortcuts "))
	b.WriteString("\n\n")
	for i, section := range sections {
		b.WriteString(m.styles.HeaderSection().Render(" " + section.title + " "))
		b.WriteString("\n")
		for _, bind := range section.bindings {
			keys := m.renderKeysLight(bind.key)
			desc := m.styles.HeaderHint().Render(bind.desc)
			fmt.Fprintf(&b, "  %s  %s\n", keys, desc)
		}
		if i < len(sections)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// helpView renders the help overlay using a viewport for scrolling.
func (m *Model) helpView() string {
	vp := m.helpViewport
	if vp == nil {
		return ""
	}

	content := vp.View()
	contentW := vp.Width()
	rule := m.scrollRule(contentW, vp.TotalLineCount(), vp.Height(),
		vp.AtTop(), vp.AtBottom(), vp.ScrollPercent(), "─")

	hints := []string{m.helpItem(keyEsc, "close")}
	if vp.TotalLineCount() > vp.Height() {
		hints = append([]string{m.helpItem(keyJ+"/"+keyK, "scroll")}, hints...)
	}
	closeHintStr := joinWithSeparator(m.helpSeparator(), hints...)

	return m.styles.OverlayBox().
		Render(content + "\n\n" + rule + "\n" + closeHintStr)
}
