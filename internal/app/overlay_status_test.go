// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusBarHiddenWhenDashboardActive(t *testing.T) {
	m := newTestModel()
	m.showDashboard = true
	m.dash.data = nonEmptyDashboard()

	status := m.statusView()

	// Main tab keybindings should be hidden.
	assert.NotContains(t, status, "NAV")
	assert.NotContains(t, status, "switch")
	assert.NotContains(t, status, "sort")
}

func TestStatusBarHiddenWhenHelpActive(t *testing.T) {
	m := newTestModel()
	sendKey(m, "?")
	require.Contains(t, m.buildView(), "Keyboard Shortcuts")

	status := m.statusView()

	// Main tab keybindings should be hidden.
	assert.NotContains(t, status, "NAV")
	assert.NotContains(t, status, "switch")
	assert.NotContains(t, status, "sort")
}

func TestStatusBarHiddenWhenNotePreviewActive(t *testing.T) {
	m := newTestModel()
	m.showNotePreview = true
	m.notePreviewText = "test note"

	status := m.statusView()

	// Main tab keybindings should be hidden.
	assert.NotContains(t, status, "NAV")
	assert.NotContains(t, status, "switch")
	assert.NotContains(t, status, "sort")
}

func TestStatusBarHiddenWhenColumnFinderActive(t *testing.T) {
	m := newTestModel()
	sendKey(m, "/")
	require.Contains(t, m.buildView(), "Jump to Column")

	status := m.statusView()

	// Main tab keybindings should be hidden.
	assert.NotContains(t, status, "NAV")
	assert.NotContains(t, status, "switch")
	assert.NotContains(t, status, "sort")
}

func TestStatusBarHiddenWhenCalendarActive(t *testing.T) {
	m := newTestModel()
	fieldValue := ""
	m.openCalendar(&fieldValue, nil)
	require.Contains(t, m.buildView(), "Su Mo Tu We Th Fr Sa")

	status := m.statusView()

	// Main tab keybindings should be hidden.
	assert.NotContains(t, status, "NAV")
	assert.NotContains(t, status, "switch")
	assert.NotContains(t, status, "sort")
}

func TestStatusBarShownWhenNoOverlayActive(t *testing.T) {
	m := newTestModel()
	m.showDashboard = false
	m.showNotePreview = false
	m.helpViewport = nil
	m.columnFinder = nil
	m.calendar = nil

	status := m.statusView()

	// Main tab keybindings should be visible.
	assert.Contains(t, status, "NAV")
}
