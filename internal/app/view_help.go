// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
)

// helpContent generates the static help text (keyboard shortcuts).
// Separated from rendering so it can be set once on the viewport.
func (m *Model) helpContent() string {
	type entry struct {
		keys string
		desc string
	}
	fromBinding := func(b key.Binding) entry {
		h := b.Help()
		return entry{keys: h.Key, desc: h.Desc}
	}

	sections := []struct {
		title   string
		entries []entry
	}{
		{
			title: "Global",
			entries: []entry{
				fromBinding(m.keys.Cancel),
				fromBinding(m.keys.Quit),
			},
		},
		{
			title: "Nav Mode",
			entries: []entry{
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
			entries: []entry{
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
			entries: []entry{
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
			entries: []entry{
				fromBinding(m.keys.ChatSend),
				fromBinding(m.keys.ChatToggleSQL),
				fromBinding(m.keys.ChatHistoryUp),
				fromBinding(m.keys.ChatHide),
			},
		},
	}

	var b strings.Builder
	b.WriteString(m.styles.HeaderTitle().Render(" Keyboard Shortcuts "))
	b.WriteString("\n\n")
	for i, section := range sections {
		b.WriteString(m.styles.HeaderSection().Render(" " + section.title + " "))
		b.WriteString("\n")
		for _, e := range section.entries {
			keys := m.renderKeysLight(e.keys)
			desc := m.styles.HeaderHint().Render(e.desc)
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
