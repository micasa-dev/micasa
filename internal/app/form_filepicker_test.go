// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendBackKey sends one of the filepicker Back bindings (h, backspace, left).
func sendBackKey(m *Model, key string) {
	var msg tea.KeyMsg
	switch key {
	case "h":
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	case keyLeft:
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	}
	m.Update(msg)
}

// requireFilePicker asserts the focused form field is a *huh.FilePicker and
// returns it.
func requireFilePicker(t *testing.T, m *Model) *huh.FilePicker {
	t.Helper()
	field := m.fs.form.GetFocusedField()
	require.NotNil(t, field, "form should have a focused field")
	fp, ok := field.(*huh.FilePicker)
	require.True(t, ok, "focused field should be a FilePicker")
	return fp
}

func TestFilePickerBackNavigatesUp(t *testing.T) {
	// Build a temp directory tree so we control the structure.
	root := t.TempDir()
	child := filepath.Join(root, "subdir")
	require.NoError(t, os.Mkdir(child, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), nil, 0o600))

	// Point the process CWD at the child so the picker starts there.
	t.Chdir(child)

	m := newTestModelWithStore(t)
	require.NoError(t, m.startQuickDocumentForm())
	require.Equal(t, modeForm, m.mode)

	fp := requireFilePicker(t, m)
	assert.Equal(t, child, filePickerCurrentDir(fp),
		"file picker should start in the CWD")

	for _, key := range []string{"h", "backspace", keyLeft} {
		// Reset to child before each iteration.
		t.Chdir(child)
		require.NoError(t, m.startQuickDocumentForm())
		fp = requireFilePicker(t, m)
		require.Equal(t, child, filePickerCurrentDir(fp))

		sendBackKey(m, key)

		fp = requireFilePicker(t, m)
		got := filePickerCurrentDir(fp)
		assert.Equal(t, root, got,
			"pressing %q should navigate to parent directory", key)
	}
}

func TestFilePickerDefaultHidesHiddenFiles(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	m := newTestModelWithStore(t)
	require.NoError(t, m.startQuickDocumentForm())

	fp := requireFilePicker(t, m)
	assert.False(t, filePickerShowHidden(fp),
		"filepicker should hide hidden files by default")
}

func TestFilePickerToggleHidden(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	m := newTestModelWithStore(t)
	require.NoError(t, m.startQuickDocumentForm())

	fp := requireFilePicker(t, m)
	require.False(t, filePickerShowHidden(fp), "should start hidden")

	// Press "." to show hidden files.
	sendKey(m, keyShiftH)
	fp = requireFilePicker(t, m)
	assert.True(t, filePickerShowHidden(fp),
		"pressing . should show hidden files")

	// Press "." again to hide hidden files.
	sendKey(m, keyShiftH)
	fp = requireFilePicker(t, m)
	assert.False(t, filePickerShowHidden(fp),
		"pressing . again should hide hidden files")
}

func TestFilePickerToggleHiddenStatusMessage(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	m := newTestModelWithStore(t)
	require.NoError(t, m.startQuickDocumentForm())

	// Toggle on.
	sendKey(m, keyShiftH)
	assert.Equal(t, "Showing hidden files.", m.status.Text)

	// Toggle off.
	sendKey(m, keyShiftH)
	assert.Equal(t, "Hiding hidden files.", m.status.Text)
}

func TestFilePickerDescriptionReflectsHiddenState(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	m := newTestModelWithStore(t)
	require.NoError(t, m.startQuickDocumentForm())

	fp := requireFilePicker(t, m)
	desc := filePickerDescription(fp)
	assert.Contains(t, desc, "\x1b[9m",
		"'hidden' should be struck through when hidden files are not shown")

	sendKey(m, keyShiftH)
	fp = requireFilePicker(t, m)
	desc = filePickerDescription(fp)
	assert.NotContains(t, desc, "\x1b[9m",
		"'hidden' should not be struck through when hidden files are shown")
}

func TestFilePickerTitleShowsCurrentDir(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	m := newTestModelWithStore(t)
	require.NoError(t, m.startQuickDocumentForm())

	fp := requireFilePicker(t, m)
	title := filePickerTitle(fp)
	assert.Contains(t, title, "File to attach",
		"title should contain the base label")

	// Navigate up — title should update to reflect the parent.
	sendBackKey(m, "h")
	fp = requireFilePicker(t, m)
	title = filePickerTitle(fp)
	parent := shortenHome(filepath.Dir(root))
	assert.Contains(t, title, parent,
		"title should update to show the parent directory")
}
