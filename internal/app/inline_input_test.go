// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenInlineInputSetsState(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	var field string
	m.openInlineInput(42, formVendor, "Name", "Acme", &field, nil, &vendorFormData{})

	require.NotNil(t, m.inlineInput)
	assert.Equal(t, "Name", m.inlineInput.Title)
	assert.Equal(t, uint(42), m.inlineInput.EditID)
	assert.Equal(t, formVendor, m.fs.formKind)
	require.NotNil(t, m.fs.editID)
	assert.Equal(t, uint(42), *m.fs.editID)
	// The inline input prompt should be visible in the status bar.
	status := m.statusView()
	assert.Contains(t, status, "Name:")
}

func TestInlineInputEscCloses(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	var field string
	m.openInlineInput(1, formVendor, "Name", "", &field, nil, &vendorFormData{})

	sendKey(m, "esc")

	assert.Nil(t, m.inlineInput)
	assert.Equal(t, formNone, m.fs.formKind)
	assert.Nil(t, m.fs.editID)
	// After esc, the inline input prompt should be gone and normal hints visible.
	status := m.statusView()
	assert.NotContains(t, status, "Name:")
	assert.Contains(t, status, "NAV")
}

func TestInlineInputAbsorbsKeys(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	var field string
	m.openInlineInput(1, formVendor, "Name", "", &field, nil, &vendorFormData{})

	// Keys that would normally toggle house profile or switch tabs should be absorbed.
	showHouseBefore := m.showHouse
	viewBefore := m.buildView()
	sendKey(m, "tab")
	assert.Equal(t, showHouseBefore, m.showHouse, "tab should be absorbed by inline input")
	viewAfter := m.buildView()
	// The house toggle indicator should not change.
	assert.Equal(t, strings.Contains(viewBefore, "▾"), strings.Contains(viewAfter, "▾"),
		"tab should be absorbed by inline input")
}

func TestInlineInputTypingUpdatesValue(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	var field string
	m.openInlineInput(1, formVendor, "Name", "", &field, nil, &vendorFormData{})

	// Type some characters.
	for _, ch := range "hello" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	assert.Equal(t, "hello", m.inlineInput.Input.Value())
	// The typed text should appear in the status bar (inline input view).
	assert.Contains(t, m.statusView(), "hello")
}

func TestInlineInputValidationBlocksSubmit(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	var field string
	validate := func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("name is required")
		}
		return nil
	}
	m.openInlineInput(1, formVendor, "Name", "", &field, validate, &vendorFormData{})

	// Try to submit with empty value -- should fail validation.
	sendKey(m, "enter")

	require.NotNil(t, m.inlineInput)
	// Inline input should still be open (validation failed) with error visible.
	status := m.statusView()
	require.Contains(t, status, "Name:", "inline input should still be open")
	assert.Contains(t, status, "required")
}

func TestInlineInputStatusViewRendersPrompt(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	var field string
	m.openInlineInput(1, formVendor, "Name", "", &field, nil, &vendorFormData{})

	status := m.statusView()
	assert.Contains(t, status, "Name:")
}

func TestInlineInputPreservesExistingValue(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	field := "existing value"
	m.openInlineInput(1, formVendor, "Name", "", &field, nil, &vendorFormData{})

	assert.Equal(t, "existing value", m.inlineInput.Input.Value())
	// The existing value should appear in the inline input view.
	assert.Contains(t, m.statusView(), "existing value")
}

func TestInlineInputPlaceholder(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	var field string
	m.openInlineInput(1, formAppliance, "Cost", "899.00", &field, nil, &applianceFormData{})

	assert.Equal(t, "899.00", m.inlineInput.Input.Placeholder)
}

func TestInlineInputTableStaysVisible(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	var field string
	m.openInlineInput(1, formVendor, "Name", "", &field, nil, &vendorFormData{})

	// The model should NOT be in form mode, so the table stays visible.
	assert.NotEqual(t, modeForm, m.mode)
	view := m.buildBaseView()
	tab := m.activeTab()
	if tab != nil && len(tab.Specs) > 0 {
		assert.Contains(
			t,
			view,
			tab.Specs[0].Title,
			"expected table to be visible during inline input",
		)
	}
}
