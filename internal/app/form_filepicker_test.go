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
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	}
	m.Update(msg)
}

// requireFilePicker asserts the focused form field is a *huh.FilePicker and
// returns it.
func requireFilePicker(t *testing.T, m *Model) *huh.FilePicker {
	t.Helper()
	field := m.form.GetFocusedField()
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

	for _, key := range []string{"h", "backspace", "left"} {
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
