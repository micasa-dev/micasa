// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"errors"
	"os"
	"testing"

	"charm.land/bubbles/v2/table"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotePreviewOpensOnEnter(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	// Open service log detail (has Notes column).
	_ = m.openServiceLogDetail("01JNOTEXIST000000000000001", "Test")
	tab := m.effectiveTab()
	require.NotNil(t, tab, "expected detail tab")

	// Seed a row with a note.
	tab.Table.SetRows(
		[]table.Row{
			{"1", "2026-01-15", "Self", "$50.00", "Changed the filter and checked pressure"},
		},
	)
	tab.Table.SetCursor(0)
	tab.Rows = []rowMeta{{ID: "01JTEST00000000000000001"}}
	tab.CellRows = [][]cell{
		{
			{Value: "1", Kind: cellReadonly},
			{Value: "2026-01-15", Kind: cellDate},
			{Value: "Self", Kind: cellText},
			{Value: "$50.00", Kind: cellMoney},
			{Value: "Changed the filter and checked pressure", Kind: cellNotes},
		},
	}

	// Move cursor to Notes column (col 4).
	tab.ColCursor = 4

	// Press enter in Normal mode.
	sendKey(m, "enter")

	require.NotNil(t, m.notePreview)
	assert.Equal(t, "Changed the filter and checked pressure", m.notePreview.text)
	assert.Equal(t, "Notes", m.notePreview.title)
	// Note preview overlay should be visible in the rendered view.
	view := m.buildView()
	assert.Contains(t, view, "Changed the filter and checked pressure")
	assert.Contains(t, view, "Notes")
}

func TestNotePreviewDismissesOnAnyKey(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.notePreview = &notePreviewState{text: "some note", title: "Notes"}

	sendKey(m, "q")

	assert.Nil(t, m.notePreview)
	// After dismissal, the note overlay should not be in the view and
	// the normal tab hints should be visible.
	view := m.buildView()
	assert.NotContains(t, view, "Press any key to close")
	assert.Contains(t, m.statusView(), "NAV")
}

func TestNotePreviewDoesNotOpenOnEmptyNote(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JNOTEXIST000000000000001", "Test")
	tab := m.effectiveTab()

	tab.Table.SetRows([]table.Row{{"1", "2026-01-15", "Self", "", ""}})
	tab.Rows = []rowMeta{{ID: "01JTEST00000000000000001"}}
	tab.CellRows = [][]cell{
		{
			{Value: "1", Kind: cellReadonly},
			{Value: "2026-01-15", Kind: cellDate},
			{Value: "Self", Kind: cellText},
			{Value: "", Kind: cellMoney},
			{Value: "", Kind: cellNotes},
		},
	}
	tab.ColCursor = 4

	sendKey(m, "enter")

	assert.Nil(t, m.notePreview)
	// Tab hints should still be visible (no overlay opened).
	assert.Contains(t, m.statusView(), "NAV")
}

func TestNotePreviewRendersInView(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.notePreview = &notePreviewState{
		text:  "This is a test note with some content.",
		title: "Notes",
	}

	view := m.buildView()
	assert.Contains(t, view, "This is a test note")
	assert.Contains(t, view, "Press any key to close")
}

func TestNotePreviewBlocksOtherKeys(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.notePreview = &notePreviewState{text: "test"}
	initialTab := m.active

	// These should all be absorbed by the note preview.
	sendKey(m, "j")
	assert.Equal(t, initialTab, m.active, "expected tab not to change while note preview is open")
}

func TestWordWrap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{"empty", "", 40, ""},
		{"fits", "hello world", 40, "hello world"},
		{"wraps", "hello world foo bar", 11, "hello world\nfoo bar"},
		{
			"long word",
			"superlongword fits",
			20,
			"superlongword fits",
		},
		{
			"preserves newlines",
			"line one\nline two",
			40,
			"line one\nline two",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wordWrap(tt.input, tt.width)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEnterHintShowsPreviewOnNotesColumn(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JNOTEXIST000000000000001", "Test")
	tab := m.effectiveTab()
	tab.ColCursor = 4 // Notes column
	tab.Table.SetRows([]table.Row{{"1", "2026-01-15", "Self", "", "some note"}})
	tab.Rows = []rowMeta{{ID: "01JTEST00000000000000001"}}
	tab.CellRows = [][]cell{
		{
			{Value: "1", Kind: cellReadonly},
			{Value: "2026-01-15", Kind: cellDate},
			{Value: "Self", Kind: cellText},
			{Value: "", Kind: cellMoney},
			{Value: "some note", Kind: cellNotes},
		},
	}

	hint := m.enterHint()
	assert.Equal(t, "preview", hint)
}

func TestFirstLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"no newlines", "hello world", "hello world"},
		{"leading/trailing space", "  hello  ", "hello"},
		{"single newline", "line one\nline two", "line one"},
		{"crlf", "line one\r\nline two", "line one"},
		{"multiple newlines", "a\n\nb\n\nc", "a"},
		{"tabs and newlines", "a\t\nb", "a"},
		{"only whitespace", "  \n\t  ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, firstLine(tt.input))
		})
	}
}

func TestExtraLineCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"single line", "hello", 0},
		{"two lines", "a\nb", 1},
		{"three lines", "a\nb\nc", 2},
		{"trailing newline trimmed", "a\nb\n", 1},
		{"blank lines count", "a\n\nb", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extraLineCount(tt.input))
		})
	}
}

func TestMultilineNotesRenderedAsSingleLineInTable(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JNOTEXIST000000000000001", "Test")
	tab := m.effectiveTab()
	require.NotNil(t, tab)

	multilineNote := "Changed the filter\nand checked pressure"

	tab.Table.SetRows([]table.Row{
		{"1", "2026-01-15", "Self", "$50.00", multilineNote},
	})
	tab.Rows = []rowMeta{{ID: "01JTEST00000000000000001"}}
	tab.CellRows = [][]cell{
		{
			{Value: "1", Kind: cellReadonly},
			{Value: "2026-01-15", Kind: cellDate},
			{Value: "Self", Kind: cellText},
			{Value: "$50.00", Kind: cellMoney},
			{Value: multilineNote, Kind: cellNotes},
		},
	}

	view := m.buildView()

	// The table should NOT show a raw newline in the rendered note.
	assert.NotContains(t, view, "filter\nand",
		"table should not render literal newlines in notes cells")
	assert.Contains(t, view, "Changed the filter\u2026",
		"table should show the first line with ellipsis")
	assert.NotContains(t, view, "and checked pressure",
		"table should not show subsequent lines of a multi-line note")
	// Right-aligned grayed-out line count indicator.
	assert.Contains(t, view, "+1",
		"table should show line count indicator for multi-line notes")
}

func TestMultilineNotesPreservedInPreviewOverlay(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.notePreview = &notePreviewState{
		text:  "Changed the filter\nand checked pressure",
		title: "Notes",
	}

	view := m.buildView()
	assert.Contains(t, view, "Changed the filter")
	assert.Contains(t, view, "and checked pressure")
}

func TestNaturalWidthsMultilineNotesFirstLine(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "N", Min: 1, Max: 40, Flex: true, Kind: cellNotes},
	}
	rows := [][]cell{
		{{Value: "short\nvery long second line here", Kind: cellNotes}},
	}
	widths := naturalWidths(specs, rows, "$")
	// Width: first line ("short" = 5) + "…" (1) + gap (1) + "+1" (2) = 9.
	// Not the longer second line (26).
	require.Len(t, widths, 1)
	assert.Equal(t, len("short")+1+1+noteSuffixWidth(1), widths[0])
}

func TestLongNotesTruncatedInTableView(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JNOTEXIST000000000000001", "Test")
	tab := m.effectiveTab()
	require.NotNil(t, tab)

	longNote := "This is a very long single-line note from an LLM extraction that goes way beyond the max column width and keeps going"

	tab.Table.SetRows([]table.Row{
		{"1", "2026-01-15", "Self", "$50.00", longNote},
	})
	tab.Table.SetCursor(0)
	tab.Rows = []rowMeta{{ID: "01JTEST00000000000000001"}}
	tab.CellRows = [][]cell{
		{
			{Value: "1", Kind: cellReadonly},
			{Value: "2026-01-15", Kind: cellDate},
			{Value: "Self", Kind: cellText},
			{Value: "$50.00", Kind: cellMoney},
			{Value: longNote, Kind: cellNotes},
		},
	}

	view := m.buildView()

	// The full note text should NOT appear in the rendered table view.
	assert.NotContains(t, view, longNote,
		"table should truncate long notes, not show the full text")
	// The truncation ellipsis should appear.
	assert.Contains(t, view, symEllipsis,
		"table should show ellipsis for truncated notes")

	// Full text should still be accessible via the preview overlay.
	tab.ColCursor = 4
	sendKey(m, "enter")
	require.NotNil(t, m.notePreview)
	assert.Equal(t, longNote, m.notePreview.text)
}

func TestNaturalWidthsNotesCappedAtMax(t *testing.T) {
	t.Parallel()
	specs := []columnSpec{
		{Title: "N", Min: 1, Max: 40, Flex: true, Kind: cellNotes},
	}
	rows := [][]cell{
		{
			{
				Value: "This is a very long single-line note from an LLM extraction that goes way beyond the max column width",
				Kind:  cellNotes,
			},
		},
	}
	widths := naturalWidths(specs, rows, "$")
	require.Len(t, widths, 1)
	assert.Equal(t, 40, widths[0], "notes column natural width should be capped at Max")
}

// --- Notes textarea overlay tests ---

func TestOpenNotesEditOpensTextareaOverlay(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	values := &serviceLogFormData{Notes: "existing note"}
	m.openNotesEdit("01JNOTEXIST000000000000001", &values.Notes, values)

	assert.Equal(t, modeForm, m.mode)
	assert.True(t, m.fs.notesEditMode)
	require.NotNil(t, m.fs.notesFieldPtr)
	assert.Equal(t, &values.Notes, m.fs.notesFieldPtr)
	assert.NotNil(t, m.fs.form)
	assert.Nil(t, m.inlineInput, "should not use inline input")
}

func TestNotesEditModeShowsEditorHint(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	values := &serviceLogFormData{Notes: "test"}
	m.openNotesEdit("01JNOTEXIST000000000000001", &values.Notes, values)

	status := m.statusView()
	assert.Contains(t, status, "CTRL+E")
}

func TestNotesEditModeClearedOnExitForm(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	values := &serviceLogFormData{Notes: "test"}
	m.openNotesEdit("01JNOTEXIST000000000000001", &values.Notes, values)
	require.True(t, m.fs.notesEditMode)

	m.exitForm()

	assert.False(t, m.fs.notesEditMode)
	assert.Nil(t, m.fs.notesFieldPtr)
}

func TestCtrlEWithoutEditorShowsError(t *testing.T) {
	m := newTestModel(t)
	values := &serviceLogFormData{Notes: "test"}
	m.openNotesEdit("01JNOTEXIST000000000000001", &values.Notes, values)

	// Ensure no editor is set.
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	sendKey(m, "ctrl+e")

	assert.Equal(t, statusError, m.status.Kind)
	assert.Contains(t, m.status.Text, "$EDITOR")
}

func TestEditorFinishedMsgUpdatesFieldAndReopensTextarea(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	values := &serviceLogFormData{Notes: "original"}

	// Simulate a pending editor returning edited content.
	tmpFile := t.TempDir() + "/notes.txt"
	require.NoError(t, os.WriteFile(tmpFile, []byte("edited content\n"), 0o600))

	m.fs.pendingEditor = &editorState{
		EditID:   "01JNOTEXIST000000000000042",
		FormData: values,
		FieldPtr: &values.Notes,
		TempFile: tmpFile,
	}

	m.handleEditorFinished(editorFinishedMsg{})

	// Field should be updated with trailing newline stripped.
	assert.Equal(t, "edited content", values.Notes)
	// Textarea should be reopened.
	assert.Equal(t, modeForm, m.mode)
	assert.True(t, m.fs.notesEditMode)
	assert.NotNil(t, m.fs.form)
}

func TestEditorFinishedMsgStripsTrailingNewlines(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	values := &serviceLogFormData{Notes: "original"}

	tmpFile := t.TempDir() + "/notes.txt"
	require.NoError(t, os.WriteFile(tmpFile, []byte("line one\nline two\n\n\n"), 0o600))

	m.fs.pendingEditor = &editorState{
		FormData: values,
		FieldPtr: &values.Notes,
		TempFile: tmpFile,
	}

	m.handleEditorFinished(editorFinishedMsg{})

	assert.Equal(t, "line one\nline two", values.Notes)
}

func TestEditorFinishedWithErrorReopensTextarea(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	values := &serviceLogFormData{Notes: "original"}

	tmpFile := t.TempDir() + "/notes.txt"
	require.NoError(t, os.WriteFile(tmpFile, []byte("original"), 0o600))

	m.fs.pendingEditor = &editorState{
		FormData: values,
		FieldPtr: &values.Notes,
		TempFile: tmpFile,
	}

	m.handleEditorFinished(editorFinishedMsg{Err: errors.New("exit status 1")})

	// Original text should be preserved.
	assert.Equal(t, "original", values.Notes)
	// Textarea should still be reopened for retry.
	assert.Equal(t, modeForm, m.mode)
	assert.True(t, m.fs.notesEditMode)
	assert.Equal(t, statusError, m.status.Kind)
}

func TestNotePreviewStillWorksAfterNotesEditChanges(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JNOTEXIST000000000000001", "Test")
	tab := m.effectiveTab()
	require.NotNil(t, tab)

	tab.Table.SetRows([]table.Row{{"1", "2026-01-15", "Self", "", "read-only preview"}})
	tab.Table.SetCursor(0)
	tab.Rows = []rowMeta{{ID: "01JTEST00000000000000001"}}
	tab.CellRows = [][]cell{
		{
			{Value: "1", Kind: cellReadonly},
			{Value: "2026-01-15", Kind: cellDate},
			{Value: "Self", Kind: cellText},
			{Value: "", Kind: cellMoney},
			{Value: "read-only preview", Kind: cellNotes},
		},
	}
	tab.ColCursor = 4

	sendKey(m, "enter")

	require.NotNil(t, m.notePreview)
	assert.Equal(t, "read-only preview", m.notePreview.text)
}

func TestDocumentNotesSaveDoesNotTriggerExtraction(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	values := &documentFormData{Notes: "original note"}
	m.openNotesEdit("01JNOTEXIST000000000000001", &values.Notes, values)

	require.True(t, m.fs.notesEditMode)
	require.Equal(t, formDocument, m.fs.formKind())

	cmd := m.saveFormInPlace()

	assert.Nil(t, cmd, "saving document notes should not return extraction command")
	assert.Nil(t, m.ex.extraction, "saving document notes should not start extraction")
}
