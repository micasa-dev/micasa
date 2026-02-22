// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestModel creates a minimal Model for lightweight mode tests.
func newTestModel() *Model {
	store, err := data.Open(":memory:")
	if err != nil {
		panic(err)
	}
	if err := store.AutoMigrate(); err != nil {
		panic(err)
	}
	if err := store.SeedDefaults(); err != nil {
		panic(err)
	}
	store.SetCurrency(locale.DefaultCurrency())
	m := &Model{
		store:  store,
		styles: appStyles,
		tabs:   NewTabs(),
		active: 0,
		mode:   modeNormal,
		width:  120,
		height: 40,
		cur:    locale.DefaultCurrency(),
	}
	// Seed minimal rows so cursor operations don't panic.
	for i := range m.tabs {
		m.tabs[i].Table.SetRows([]table.Row{{"1", "test"}})
		m.tabs[i].Rows = []rowMeta{{ID: 1}}
	}
	return m
}

func sendKey(m *Model, key string) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	// Some keys need special types.
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEscape}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	case "ctrl+c":
		msg = tea.KeyMsg{Type: tea.KeyCtrlC}
	case keyCtrlN:
		msg = tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+o":
		msg = tea.KeyMsg{Type: tea.KeyCtrlO}
	case "ctrl+s":
		msg = tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+e":
		msg = tea.KeyMsg{Type: tea.KeyCtrlE}
	}
	m.Update(msg)
}

func TestStartsInNormalMode(t *testing.T) {
	m := newTestModel()
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.statusView(), "NAV")
}

func TestEnterEditMode(t *testing.T) {
	m := newTestModel()
	sendKey(m, "i")
	assert.Equal(t, modeEdit, m.mode)
	assert.Contains(t, m.statusView(), "EDIT")
}

func TestEnterOnPlainColumnShowsGuidance(t *testing.T) {
	m := newTestModel()
	m.showDashboard = false
	tab := m.effectiveTab()

	// Move to a plain text column (Type, col 1 on Projects).
	tab.ColCursor = 1
	require.Equal(t, cellText, tab.Specs[1].Kind, "column 1 should be a plain text column")

	// Pressing enter on a plain column should NOT enter edit mode,
	// but should show guidance in the status bar.
	sendKey(m, "enter")
	assert.Equal(t, modeNormal, m.mode, "enter on plain column should stay in normal mode")
	assert.Contains(t, m.status.Text, "Press i to edit.", "status should guide user to press i")
}

func TestEnterOnDocumentsTabShowsOpenHint(t *testing.T) {
	m := newTestModel()
	m.showDashboard = false
	m.active = tabIndex(tabDocuments)
	tab := m.effectiveTab()
	require.Equal(t, tabDocuments, tab.Kind)

	// Move to a plain text column (Title, col 1).
	tab.ColCursor = 1
	require.Equal(t, cellText, tab.Specs[1].Kind)

	sendKey(m, "enter")
	assert.Equal(t, modeNormal, m.mode, "should stay in normal mode")
	assert.Contains(t, m.status.Text, "Press o to open.")
}

func TestExitEditModeWithEsc(t *testing.T) {
	m := newTestModel()
	sendKey(m, "i")
	sendKey(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.statusView(), "NAV")
}

func TestTableKeyMapNormalMode(t *testing.T) {
	m := newTestModel()
	// In normal mode, HalfPageDown should include "d".
	tab := m.activeTab()
	require.NotNil(t, tab)
	keys := tab.Table.KeyMap.HalfPageDown.Keys()
	assert.True(
		t,
		containsKey(keys, "d"),
		"expected 'd' in HalfPageDown keys for normal mode, got %v",
		keys,
	)
}

func TestTableKeyMapEditMode(t *testing.T) {
	m := newTestModel()
	sendKey(m, "i")
	tab := m.activeTab()
	require.NotNil(t, tab)
	keys := tab.Table.KeyMap.HalfPageDown.Keys()
	assert.False(
		t,
		containsKey(keys, "d"),
		"'d' should not be in HalfPageDown keys in edit mode, got %v",
		keys,
	)
	assert.True(
		t,
		containsKey(keys, "ctrl+d"),
		"expected 'ctrl+d' in HalfPageDown keys for edit mode, got %v",
		keys,
	)
}

func TestTableKeyMapRestoredOnNormalReturn(t *testing.T) {
	m := newTestModel()
	sendKey(m, "i")
	sendKey(m, "esc")
	tab := m.activeTab()
	require.NotNil(t, tab)
	keys := tab.Table.KeyMap.HalfPageDown.Keys()
	assert.True(
		t,
		containsKey(keys, "d"),
		"expected 'd' restored in HalfPageDown after returning to normal, got %v",
		keys,
	)
}

func TestColumnNavH(t *testing.T) {
	m := newTestModel()
	tab := m.activeTab()
	initial := tab.ColCursor
	sendKey(m, "l")
	if len(tab.Specs) > 1 {
		assert.NotEqual(t, initial, tab.ColCursor, "expected column cursor to advance on 'l'")
	}
}

func TestColumnNavClampsLeft(t *testing.T) {
	m := newTestModel()
	tab := m.activeTab()
	tab.ColCursor = 0
	sendKey(m, "h")
	assert.Equal(t, 0, tab.ColCursor)
}

func TestCaretJumpsToFirstColumn(t *testing.T) {
	m := newTestModel()
	tab := m.activeTab()
	tab.ColCursor = len(tab.Specs) - 1
	sendKey(m, "^")
	assert.Equal(t, 0, tab.ColCursor)
}

func TestDollarJumpsToLastColumn(t *testing.T) {
	m := newTestModel()
	tab := m.activeTab()
	tab.ColCursor = 0
	sendKey(m, "$")
	assert.Equal(t, len(tab.Specs)-1, tab.ColCursor)
}

func TestNextTabAdvances(t *testing.T) {
	m := newTestModel()
	// Verify mode transitions via sendKey don't reset the active tab.
	m.active = 0
	sendKey(m, "i")
	assert.Equal(t, modeEdit, m.mode)
	assert.Contains(t, m.statusView(), "EDIT")
	assert.Equal(t, 0, m.active, "entering edit mode should not change active tab")
	m.active = 2
	sendKey(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.statusView(), "NAV")
	assert.Equal(t, 2, m.active, "entering normal mode should not change active tab")
}

func TestQuitOnlyInNormalMode(t *testing.T) {
	m := newTestModel()

	// In edit mode, 'ctrl+q' should quit (returns tea.Quit).
	sendKey(m, "i")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	assert.NotNil(t, cmd, "'ctrl+q' should quit even in edit mode")
}

func TestIKeyDoesNothingInEditMode(t *testing.T) {
	m := newTestModel()
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)
	require.Contains(t, m.statusView(), "EDIT")
	// Press 'i' again — should not switch mode or do anything unexpected.
	sendKey(m, "i")
	assert.Equal(t, modeEdit, m.mode)
	assert.Contains(t, m.statusView(), "EDIT", "expected to stay in edit mode")
}

func TestHouseToggle(t *testing.T) {
	m := newTestModel()
	m.hasHouse = true
	assert.False(t, m.showHouse)

	// House starts collapsed (shows ▸).
	view := m.buildView()
	assert.Contains(t, view, "▸", "expected collapsed house initially")

	// Tab toggles house in both modes.
	sendKey(m, "tab")
	assert.True(t, m.showHouse)
	view = m.buildView()
	assert.Contains(t, view, "▾", "expected expanded house after tab")

	sendKey(m, "tab")
	assert.False(t, m.showHouse)
	view = m.buildView()
	assert.Contains(t, view, "▸", "expected collapsed house after second tab")
}

func TestHelpToggle(t *testing.T) {
	m := newTestModel()
	sendKey(m, "?")
	assert.NotNil(t, m.helpViewport)
	assert.Contains(t, m.buildView(), "Keyboard Shortcuts", "expected help visible after '?'")
	sendKey(m, "?")
	assert.Nil(t, m.helpViewport)
	assert.NotContains(
		t,
		m.buildView(),
		"Keyboard Shortcuts",
		"expected help hidden after second '?'",
	)
}

func TestHelpViewportScrolling(t *testing.T) {
	m := newTestModel()
	sendKey(m, "?")
	require.NotNil(t, m.helpViewport)
	require.Contains(t, m.buildView(), "Keyboard Shortcuts", "expected help visible")

	// Scroll down and verify offset moves.
	sendKey(m, "j")
	if m.helpViewport.TotalLineCount() > m.helpViewport.Height {
		assert.NotZero(t, m.helpViewport.YOffset, "expected viewport to scroll down on 'j'")
	}

	// Scroll back up.
	sendKey(m, "k")
	assert.Equal(t, 0, m.helpViewport.YOffset, "expected viewport at top after scrolling back up")

	// Go to bottom with G.
	sendKey(m, "G")
	if m.helpViewport.TotalLineCount() > m.helpViewport.Height {
		assert.True(t, m.helpViewport.AtBottom(), "expected viewport at bottom after 'G'")
	}

	// Go to top with g.
	sendKey(m, "g")
	assert.True(t, m.helpViewport.AtTop(), "expected viewport at top after 'g'")

	// Esc dismisses.
	sendKey(m, "esc")
	assert.Nil(t, m.helpViewport)
	assert.NotContains(t, m.buildView(), "Keyboard Shortcuts", "expected help hidden after esc")
}

func TestHelpOverlayFixedWidthOnScroll(t *testing.T) {
	m := newTestModel()
	m.height = 20 // Small height forces scrolling.
	sendKey(m, "?")
	require.NotNil(t, m.helpViewport, "expected help visible")
	if m.helpViewport.TotalLineCount() <= m.helpViewport.Height {
		t.Skip("help content fits without scrolling at this height")
	}

	// Measure width at top.
	widthAtTop := lipgloss.Width(m.helpView())

	// Scroll to middle.
	for i := 0; i < 5; i++ {
		sendKey(m, "j")
	}
	widthAtMiddle := lipgloss.Width(m.helpView())

	// Scroll to bottom.
	sendKey(m, "G")
	widthAtBottom := lipgloss.Width(m.helpView())

	assert.Equal(t, widthAtTop, widthAtMiddle, "help width changed from top to middle")
	assert.Equal(t, widthAtTop, widthAtBottom, "help width changed from top to bottom")
}

func TestHelpScrollIndicatorChanges(t *testing.T) {
	m := newTestModel()
	m.height = 20
	sendKey(m, "?")
	require.NotNil(t, m.helpViewport, "expected help visible")
	if m.helpViewport.TotalLineCount() <= m.helpViewport.Height {
		t.Skip("help content fits without scrolling at this height")
	}

	viewAtTop := m.helpView()
	assert.Contains(t, viewAtTop, "Top")

	sendKey(m, "G")
	viewAtBottom := m.helpView()
	assert.Contains(t, viewAtBottom, "Bot")

	// Scroll back up one line from bottom -- should show percentage.
	sendKey(m, "k")
	viewAtMiddle := m.helpView()
	assert.NotContains(t, viewAtMiddle, "Top")
	assert.NotContains(t, viewAtMiddle, "Bot")
	assert.Contains(t, viewAtMiddle, "%")
}

func TestHelpAbsorbsOtherKeys(t *testing.T) {
	m := newTestModel()
	sendKey(m, "?")
	require.NotNil(t, m.helpViewport)
	require.Contains(t, m.buildView(), "Keyboard Shortcuts", "expected help visible")

	// Keys that would normally affect the model should be absorbed.
	sendKey(m, "i")
	assert.Equal(t, modeNormal, m.mode)
	// After pressing 'i', the help overlay should still be open and
	// the status bar should not show EDIT mode.
	assert.Contains(t, m.buildView(), "Keyboard Shortcuts",
		"'i' should not close help overlay")
}

func TestDeleteRequiresEditMode(t *testing.T) {
	m := newTestModel()
	// In normal mode, 'd' is half-page-down (table handles it).
	// It should NOT trigger delete.
	sendKey(m, "d")
	assert.Empty(t, m.status.Text)
	status := m.statusView()
	assert.Contains(t, status, "NAV", "'d' in normal mode should not change mode")
}

func TestEscClearsStatusInNormalMode(t *testing.T) {
	m := newTestModel()
	m.status = statusMsg{Text: "something", Kind: statusInfo}
	require.Contains(t, m.statusView(), "something")
	sendKey(m, "esc")
	assert.Empty(t, m.status.Text)
	assert.NotContains(t, m.statusView(), "something")
}

func TestProjectStatusFilterToggleKeys(t *testing.T) {
	m := newTestModel()
	tab := m.activeTab()
	require.NotNil(t, tab)
	require.Equal(t, tabProjects, tab.Kind, "expected projects tab to be active")
	col := statusColumnIndex(tab.Specs)
	require.GreaterOrEqual(t, col, 0, "expected a Status column in project specs")
	assert.False(t, hasColumnPins(tab, col), "should start with no status pins")
	assert.False(t, tab.FilterActive, "filter should start inactive")

	sendKey(m, "t")
	assert.True(t, hasColumnPins(tab, col), "expected status pins after t")
	assert.True(t, tab.FilterActive, "expected filter active after t")
	assert.Contains(t, m.status.Text, "hidden")
	assert.Contains(t, m.statusView(), "hidden")

	sendKey(m, "t")
	assert.False(t, hasColumnPins(tab, col), "expected no status pins after second t")
	assert.False(t, tab.FilterActive, "expected filter inactive after second t")
	assert.Contains(t, m.status.Text, "shown")
	assert.Contains(t, m.statusView(), "shown")
}

func TestProjectStatusFilterToggleIgnoredOutsideProjects(t *testing.T) {
	m := newTestModel()
	m.active = tabIndex(tabQuotes)
	tab := m.activeTab()
	require.NotNil(t, tab)
	require.Equal(t, tabQuotes, tab.Kind, "expected quotes tab to be active")

	sendKey(m, "t")
	assert.False(t, tab.FilterActive,
		"filter should not activate on non-project tabs")
	assert.Empty(t, m.status.Text)
	assert.NotContains(t, m.statusView(), "hidden")
	assert.NotContains(t, m.statusView(), "shown")
}

func TestKeyDispatchEditModeOnly(t *testing.T) {
	m := newTestModel()

	// 'p' should not change mode in normal mode.
	sendKey(m, "p")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.statusView(), "NAV", "'p' should not change mode in normal mode")

	// 'esc' should be handled in edit mode (back to normal).
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)
	require.Contains(t, m.statusView(), "EDIT")
	sendKey(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.statusView(), "NAV")
}

func TestModeAfterFormExit(t *testing.T) {
	m := newTestModel()
	// Enter edit mode via key, open a form, then exit.
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)
	require.Contains(t, m.statusView(), "EDIT")
	m.prevMode = m.mode
	m.mode = modeForm
	// Simulate exitForm (no key to close a form without a database).
	m.exitForm()
	assert.Equal(t, modeEdit, m.mode)
	assert.Contains(t, m.statusView(), "EDIT",
		"expected edit mode after exitForm (was in edit before form)")

	// Return to normal mode via key, then form again.
	sendKey(m, "esc")
	require.Equal(t, modeNormal, m.mode)
	require.Contains(t, m.statusView(), "NAV")
	m.prevMode = m.mode
	m.mode = modeForm
	m.exitForm()
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.statusView(), "NAV",
		"expected normal mode after exitForm (was in normal before form)")
}

func TestTabTogglesHouseInEditMode(t *testing.T) {
	m := newTestModel()
	m.hasHouse = true
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)
	require.Contains(t, m.statusView(), "EDIT")
	assert.False(t, m.showHouse)
	view := m.buildView()
	assert.Contains(t, view, "▸", "house should start collapsed")
	sendKey(m, "tab")
	assert.True(t, m.showHouse)
	view = m.buildView()
	assert.Contains(t, view, "▾", "tab should toggle house to expanded in edit mode")
}

func TestTabSwitchKeysBlockedInEditMode(t *testing.T) {
	m := newTestModel()
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)
	require.Contains(t, m.statusView(), "EDIT")
	startTab := m.active
	// b/f (tab-switch keys) should not switch tabs in edit mode.
	sendKey(m, "b")
	assert.Equal(t, startTab, m.active, "b should not switch tabs in edit mode")
	sendKey(m, "f")
	assert.Equal(t, startTab, m.active, "f should not switch tabs in edit mode")
}

func TestModeBadgeFixedWidth(t *testing.T) {
	styles := DefaultStyles()
	normalBadge := styles.ModeNormal().Render("NAV")
	normalWidth := lipgloss.Width(normalBadge)

	editBadge := styles.ModeEdit().
		Width(normalWidth).
		Align(lipgloss.Center).
		Render("EDIT")
	editWidth := lipgloss.Width(editBadge)

	assert.Equal(t, normalWidth, editWidth, "badge widths should match")
}

func TestKeycapPreservesCase(t *testing.T) {
	m := newTestModel()
	// Uppercase "H" stays as "H" (not "SHIFT+H").
	rendered := m.keycap("H")
	assert.Contains(t, rendered, "H")
	assert.NotContains(t, rendered, "SHIFT")
	// Lowercase "h" stays as "h" (not uppercased).
	rendered = m.keycap("h")
	assert.Contains(t, rendered, "h")
	assert.NotContains(t, rendered, "SHIFT")
}

func containsKey(keys []string, target string) bool {
	for _, k := range keys {
		if k == target {
			return true
		}
	}
	return false
}

func TestDeleteAutoShowsDeletedAndRestoreWorks(t *testing.T) {
	m := newTestModelWithStore(t)

	// Create a vendor (no FK children to block deletion).
	h := vendorHandler{}
	m.formData = &vendorFormData{Name: "Test Vendor", Phone: "555-0000"}
	require.NoError(t, h.SubmitForm(m))

	// Switch to vendors tab and reload so the row is visible.
	m.active = tabIndex(tabVendors)
	require.NoError(t, m.reloadActiveTab())

	tab := m.activeTab()
	tab.Table.SetCursor(0)
	require.NotNil(t, tab)
	require.Len(t, tab.Rows, 1, "should have one vendor row")
	assert.False(t, tab.ShowDeleted, "ShowDeleted should start off")

	// User enters edit mode and deletes the selected row.
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)
	sendKey(m, "d")

	tab = m.activeTab()
	assert.True(t, tab.ShowDeleted,
		"ShowDeleted should auto-enable after delete")
	assert.Contains(t, m.status.Text, "Deleted",
		"status should confirm deletion")
	assert.Len(t, tab.Rows, 1,
		"deleted row should still be visible because ShowDeleted is on")

	// User presses d again on the same row to restore it.
	sendKey(m, "d")
	tab = m.activeTab()
	assert.Contains(t, m.status.Text, "Restored",
		"status should confirm restoration")
	assert.Len(t, tab.Rows, 1, "restored row should remain visible")
	assert.False(t, tab.Rows[0].Deleted, "row should no longer be marked deleted")
}
