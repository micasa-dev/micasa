// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/cpcloud/micasa/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendBackKey sends one of the filepicker Back bindings (h, backspace, left).
func sendBackKey(m *Model, key string) {
	var msg tea.KeyPressMsg
	switch key {
	case "h":
		msg = tea.KeyPressMsg{Code: 'h', Text: "h"}
	case "backspace":
		msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
	case keyLeft:
		msg = tea.KeyPressMsg{Code: tea.KeyLeft}
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

	// Press H to show hidden files.
	sendKey(m, keyShiftH)
	fp = requireFilePicker(t, m)
	assert.True(t, filePickerShowHidden(fp),
		"pressing H should show hidden files")

	// Press H again to hide hidden files.
	sendKey(m, keyShiftH)
	fp = requireFilePicker(t, m)
	assert.False(t, filePickerShowHidden(fp),
		"pressing H again should hide hidden files")
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

func TestFilePickerToggleHiddenPersistsAcrossNavigation(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "subdir")
	require.NoError(t, os.Mkdir(child, 0o750))
	t.Chdir(child)

	m := newTestModelWithStore(t)
	require.NoError(t, m.startQuickDocumentForm())

	// Toggle to show hidden files.
	sendKey(m, keyShiftH)
	fp := requireFilePicker(t, m)
	require.True(t, filePickerShowHidden(fp))

	// Navigate up to parent directory.
	sendBackKey(m, "h")
	fp = requireFilePicker(t, m)
	assert.True(t, filePickerShowHidden(fp),
		"ShowHidden should persist after navigating to parent directory")
}

func TestFilePickerToggleHiddenNoOpOnNonFilePicker(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	m := newTestModelWithStore(t)
	// Full document form starts with the Title input focused, not the picker.
	require.NoError(t, m.startDocumentForm(""))
	require.Equal(t, modeForm, m.mode)

	field := m.fs.form.GetFocusedField()
	require.NotNil(t, field)
	_, isFP := field.(*huh.FilePicker)
	require.False(t, isFP, "focused field should not be a FilePicker")

	// Pressing H on a non-FilePicker field should not panic or set status.
	m.status = statusMsg{}
	sendKey(m, keyShiftH)
	assert.Empty(t, m.status.Text,
		"H on non-FilePicker should not produce a status message")
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

func TestQuickDocumentCtrlSSavesWithoutExtraction(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "invoice.txt"), []byte("hello"), 0o600))
	t.Chdir(root)

	m := newTestModelWithStore(t)

	// Configure an LLM client so extraction WOULD be triggered if the
	// deferred path ran. This is the key condition that exposes the bug.
	client, err := llm.NewClient("ollama", "http://localhost:11434", "test", "", 30*time.Second)
	require.NoError(t, err)
	m.ex.extractionClient = client
	m.ex.extractionEnabled = true
	m.ex.extractionReady = true

	require.NoError(t, m.startQuickDocumentForm())
	require.Equal(t, modeForm, m.mode)

	// Simulate file selection by setting the form data path directly.
	fd, ok := m.fs.formData.(*documentFormData)
	require.True(t, ok)
	require.True(t, fd.DeferCreate)
	fd.FilePath = filepath.Join(root, "invoice.txt")

	// ctrl+s should save the document without triggering extraction.
	sendKey(m, keyCtrlS)

	assert.Nil(t, m.ex.extraction,
		"ctrl+s on quick-add form should not start extraction overlay")
	assert.NotEqual(t, modeForm, m.mode,
		"form should close after ctrl+s save")

	// Document should be persisted in the DB.
	docs, err := m.store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "invoice.txt", docs[0].FileName)
}

func TestQuickDocumentCtrlSParseErrorShowsStatus(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	m := newTestModelWithStore(t)
	require.NoError(t, m.startQuickDocumentForm())

	// Point at a non-existent file to trigger a parse error.
	fd, ok := m.fs.formData.(*documentFormData)
	require.True(t, ok)
	fd.FilePath = filepath.Join(root, "no-such-file.txt")

	sendKey(m, keyCtrlS)

	assert.Equal(t, statusError, m.status.Kind,
		"parse failure should surface a status error")
	assert.Equal(t, modeForm, m.mode,
		"form should remain open on error")
}

func TestQuickDocumentCtrlSFileTooLargeShowsStatus(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "huge.txt")
	require.NoError(t, os.WriteFile(file, []byte("data"), 0o600))
	t.Chdir(root)

	m := newTestModelWithStore(t)
	// Set max size to 1 byte so the 4-byte file exceeds it.
	require.NoError(t, m.store.SetMaxDocumentSize(1))

	require.NoError(t, m.startQuickDocumentForm())
	fd, ok := m.fs.formData.(*documentFormData)
	require.True(t, ok)
	fd.FilePath = file

	sendKey(m, keyCtrlS)

	assert.Equal(t, statusError, m.status.Kind,
		"oversized file should surface a status error")
	assert.Equal(t, modeForm, m.mode,
		"form should remain open on error")
}
