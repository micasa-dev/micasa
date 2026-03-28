// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// switchToDocsTab moves the active tab to the Documents tab.
func switchToDocsTab(m *Model) {
	for i, tab := range m.tabs {
		if tab.Kind == tabDocuments {
			m.active = i
			return
		}
	}
}

func TestDocSearchOpenClose(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	switchToDocsTab(m)

	// ctrl+f opens search overlay.
	sendKey(m, keyCtrlF)
	require.NotNil(t, m.docSearch, "ctrl+f should open search overlay")
	assert.True(t, m.hasActiveOverlay())

	// esc closes it.
	sendKey(m, keyEsc)
	assert.Nil(t, m.docSearch, "esc should close search overlay")
}

func TestDocSearchTypingFilters(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	switchToDocsTab(m)

	// Create documents with distinct text.
	require.NoError(t, m.store.CreateDocument(&data.Document{
		Title:         "Plumber Receipt",
		FileName:      "plumber.pdf",
		ExtractedText: "Invoice from ABC Plumbing for kitchen sink repair",
	}))
	require.NoError(t, m.store.CreateDocument(&data.Document{
		Title:         "HVAC Manual",
		FileName:      "hvac.pdf",
		ExtractedText: "Installation guide for central air conditioning",
	}))

	sendKey(m, keyCtrlF)
	require.NotNil(t, m.docSearch)

	// Type "plumb" to filter.
	for _, r := range "plumb" {
		sendKey(m, string(r))
	}
	require.Len(t, m.docSearch.Results, 1, "should find one match for 'plumb'")
	assert.Equal(t, "Plumber Receipt", m.docSearch.Results[0].Title)
}

func TestDocSearchNavigateToDocument(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	switchToDocsTab(m)

	require.NoError(t, m.store.CreateDocument(&data.Document{
		Title:         "Test Invoice",
		FileName:      "test.pdf",
		ExtractedText: "unique searchable content xyz123",
	}))
	require.NoError(t, m.reloadAllTabs())

	sendKey(m, keyCtrlF)
	require.NotNil(t, m.docSearch)

	// Search for the document.
	for _, r := range "xyz123" {
		sendKey(m, string(r))
	}
	require.Len(t, m.docSearch.Results, 1)

	// Press enter to navigate.
	sendKey(m, keyEnter)

	// Overlay should be closed.
	assert.Nil(t, m.docSearch, "search overlay should close after navigation")

	// Should still be on the Documents tab.
	assert.Equal(t, tabDocuments, m.tabs[m.active].Kind,
		"should stay on Documents tab")
}

func TestDocSearchEmptyQueryShowsHint(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	switchToDocsTab(m)

	sendKey(m, keyCtrlF)
	require.NotNil(t, m.docSearch)

	// With empty query, view should show placeholder text.
	view := m.buildDocSearchOverlay()
	assert.Contains(t, view, "type to search")
}

func TestDocSearchNoMatchesShowsMessage(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	switchToDocsTab(m)

	sendKey(m, keyCtrlF)
	require.NotNil(t, m.docSearch)

	// Type something with no matches.
	for _, r := range "zzzznonexistent" {
		sendKey(m, string(r))
	}

	view := m.buildDocSearchOverlay()
	assert.Contains(t, view, "no matches")
}

func TestDocSearchCursorNavigation(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	switchToDocsTab(m)

	// Create multiple documents.
	for i := range 3 {
		require.NoError(t, m.store.CreateDocument(&data.Document{
			Title:         "Common Doc",
			FileName:      "doc.pdf",
			ExtractedText: "shared keyword alpha",
			Notes:         string(rune('A' + i)),
		}))
	}

	sendKey(m, keyCtrlF)
	require.NotNil(t, m.docSearch)

	for _, r := range "alpha" {
		sendKey(m, string(r))
	}
	require.Len(t, m.docSearch.Results, 3)
	assert.Equal(t, 0, m.docSearch.Cursor)

	// Move down with arrow keys.
	sendKey(m, keyDown)
	assert.Equal(t, 1, m.docSearch.Cursor)

	sendKey(m, keyDown)
	assert.Equal(t, 2, m.docSearch.Cursor)

	// Should clamp at bottom.
	sendKey(m, keyDown)
	assert.Equal(t, 2, m.docSearch.Cursor)

	// Move up.
	sendKey(m, keyUp)
	assert.Equal(t, 1, m.docSearch.Cursor)
}

func TestDocSearchCursorCtrlJK(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	switchToDocsTab(m)

	for i := range 3 {
		require.NoError(t, m.store.CreateDocument(&data.Document{
			Title:         "Common Doc",
			FileName:      "doc.pdf",
			ExtractedText: "shared keyword beta",
			Notes:         string(rune('A' + i)),
		}))
	}

	sendKey(m, keyCtrlF)
	require.NotNil(t, m.docSearch)

	for _, r := range "beta" {
		sendKey(m, string(r))
	}
	require.Len(t, m.docSearch.Results, 3)

	// ctrl+j moves down, ctrl+k moves up.
	sendKey(m, keyCtrlJ)
	assert.Equal(t, 1, m.docSearch.Cursor)

	sendKey(m, keyCtrlK)
	assert.Equal(t, 0, m.docSearch.Cursor)
}

func TestDocSearchOverlayInView(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	switchToDocsTab(m)

	sendKey(m, keyCtrlF)
	view := m.buildView()
	assert.Contains(t, view, "Search Documents")
}

func TestDocSearchHelpEntry(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	content := m.helpContent()
	assert.Contains(t, content, "search documents")
}

func TestDocSearchStatusHint(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.showDashboard = false
	switchToDocsTab(m)

	view := m.statusView()
	assert.Contains(t, view, "search")
}

func TestDocSearchStatusHintHiddenOnOtherTabs(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.showDashboard = false

	// Projects tab (default).
	view := m.statusView()
	assert.NotContains(t, view, "search")
}

func TestDocSearchBlockedInEditMode(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	switchToDocsTab(m)

	sendKey(m, keyI) // Enter edit mode.
	assert.Equal(t, modeEdit, m.mode)

	sendKey(m, keyCtrlF)
	assert.Nil(t, m.docSearch, "ctrl+f should not open search in edit mode")
}

func TestDocSearchBlockedOnNonDocTab(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Start on Projects tab (default).
	assert.NotEqual(t, tabDocuments, m.tabs[m.active].Kind)

	sendKey(m, keyCtrlF)
	assert.Nil(t, m.docSearch, "ctrl+f should not open search on non-document tab")
}

func TestDocSearchSnippetHighlighting(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.docSearch = &docSearchState{}

	snippet := m.formatSnippet("before >>>matched<<< after", 80)
	assert.Contains(t, snippet, "matched")
	assert.Contains(t, snippet, "before")
	assert.Contains(t, snippet, "after")
}
