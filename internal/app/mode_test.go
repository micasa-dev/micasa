// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"os"
	"path/filepath"
	"testing"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestModel creates a minimal Model for lightweight mode tests.
func newTestModel(t *testing.T) *Model {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, templateBytes, 0o600))

	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	store.SetCurrency(locale.DefaultCurrency())
	m := &Model{
		zones:  zone.New(),
		store:  store,
		styles: appStyles,
		tabs:   NewTabs(),
		active: 0,
		mode:   modeNormal,
		width:  120,
		height: 40,
		cur:    locale.DefaultCurrency(),
		keys:   newAppKeyMap(),
	}
	// Seed minimal rows so cursor operations don't panic.
	for i := range m.tabs {
		m.tabs[i].Table.SetRows([]table.Row{{"1", "test"}})
		m.tabs[i].Rows = []rowMeta{{ID: "01JTEST00000000000000001"}}
	}
	return m
}

// keyPress builds a tea.KeyPressMsg from a human-readable key string (the
// same format used by the key constants in model.go). This replaces the v1
// tea.KeyPressMsg struct construction.
func keyPress(key string) tea.KeyPressMsg {
	switch key {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "shift+up":
		return tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}
	case "shift+down":
		return tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	// ctrl+<letter> combos
	if len(key) > 5 && key[:5] == "ctrl+" {
		ch := rune(key[5])
		return tea.KeyPressMsg{Code: ch, Mod: tea.ModCtrl}
	}
	// Single printable character.
	runes := []rune(key)
	if len(runes) == 1 {
		return tea.KeyPressMsg{Code: runes[0], Text: key}
	}
	// Fallback: multi-rune string treated as text.
	return tea.KeyPressMsg{Code: runes[0], Text: key}
}

func sendKey(m *Model, key string) {
	m.Update(keyPress(key))
}

func TestStartsInNormalMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.statusView(), "NAV")
}

func TestEnterEditMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "i")
	assert.Equal(t, modeEdit, m.mode)
	assert.Contains(t, m.statusView(), "EDIT")
}

func TestEnterOnPlainColumnShowsGuidance(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "i")
	sendKey(m, "esc")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.statusView(), "NAV")
}

func TestTableKeyMapNormalMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)
	tab := m.activeTab()
	initial := tab.ColCursor
	sendKey(m, "l")
	if len(tab.Specs) > 1 {
		assert.NotEqual(t, initial, tab.ColCursor, "expected column cursor to advance on 'l'")
	}
}

func TestColumnNavClampsLeft(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	tab := m.activeTab()
	tab.ColCursor = 0
	sendKey(m, "h")
	assert.Equal(t, 0, tab.ColCursor)
}

func TestCaretJumpsToFirstColumn(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	tab := m.activeTab()
	tab.ColCursor = len(tab.Specs) - 1
	sendKey(m, "^")
	assert.Equal(t, 0, tab.ColCursor)
}

func TestDollarJumpsToLastColumn(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	tab := m.activeTab()
	tab.ColCursor = 0
	sendKey(m, "$")
	assert.Equal(t, len(tab.Specs)-1, tab.ColCursor)
}

func TestNextTabAdvances(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)

	// In edit mode, 'ctrl+q' should quit (returns tea.Quit).
	sendKey(m, "i")
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
	assert.NotNil(t, cmd, "'ctrl+q' should quit even in edit mode")
}

func TestIKeyDoesNothingInEditMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)
	require.Contains(t, m.statusView(), "EDIT")
	// Press 'i' again — should not switch mode or do anything unexpected.
	sendKey(m, "i")
	assert.Equal(t, modeEdit, m.mode)
	assert.Contains(t, m.statusView(), "EDIT", "expected to stay in edit mode")
}

func TestHouseToggle(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "?")
	assert.NotNil(t, m.helpState)
	assert.Contains(t, m.buildView(), "Keyboard Shortcuts", "expected help visible after '?'")
	sendKey(m, "?")
	assert.Nil(t, m.helpState)
	assert.NotContains(
		t,
		m.buildView(),
		"Keyboard Shortcuts",
		"expected help hidden after second '?'",
	)
}

func TestHelpSectionNavigation(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "?")
	require.NotNil(t, m.helpState)
	require.Contains(t, m.buildView(), "Keyboard Shortcuts", "expected help visible")

	sections := m.helpSections()
	require.Greater(t, len(sections), 1, "need at least 2 sections for navigation test")

	// Starts at section 0.
	assert.Equal(t, 0, m.helpState.section)

	// j moves to next section.
	sendKey(m, "j")
	assert.Equal(t, 1, m.helpState.section)

	// k moves back up.
	sendKey(m, "k")
	assert.Equal(t, 0, m.helpState.section)

	// Esc dismisses.
	sendKey(m, "esc")
	assert.Nil(t, m.helpState)
	assert.NotContains(t, m.buildView(), "Keyboard Shortcuts", "expected help hidden after esc")
}

func TestHelpOverlayFixedWidthAcrossSections(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "?")
	require.NotNil(t, m.helpState, "expected help visible")

	sections := m.helpSections()
	require.Greater(t, len(sections), 1, "need multiple sections")

	// Measure width at first section.
	widthAtFirst := lipgloss.Width(m.helpView())

	// Move to a different section.
	sendKey(m, "j")
	widthAtSecond := lipgloss.Width(m.helpView())

	assert.Equal(t, widthAtFirst, widthAtSecond,
		"help overlay width should stay constant across sections")
}

func TestHelpSectionClampsBounds(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "?")
	require.NotNil(t, m.helpState, "expected help visible")

	sections := m.helpSections()

	// k at section 0 stays at 0.
	sendKey(m, "k")
	assert.Equal(t, 0, m.helpState.section, "k at top should stay at 0")

	// Navigate to last section.
	for range len(sections) - 1 {
		sendKey(m, "j")
	}
	assert.Equal(t, len(sections)-1, m.helpState.section)

	// j at last section stays at last.
	sendKey(m, "j")
	assert.Equal(t, len(sections)-1, m.helpState.section, "j at bottom should stay at last")
}

func TestHelpAbsorbsOtherKeys(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "?")
	require.NotNil(t, m.helpState)
	require.Contains(t, m.buildView(), "Keyboard Shortcuts", "expected help visible")

	// Keys that would normally affect the model should be absorbed.
	sendKey(m, "i")
	assert.Equal(t, modeNormal, m.mode)
	// After pressing 'i', the help overlay should still be open and
	// the status bar should not show EDIT mode.
	assert.Contains(t, m.buildView(), "Keyboard Shortcuts",
		"'i' should not close help overlay")
}

func TestHelpTwoPaneClose(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)

	// ? opens.
	sendKey(m, "?")
	require.NotNil(t, m.helpState, "? should open help")

	// esc closes.
	sendKey(m, "esc")
	assert.Nil(t, m.helpState, "esc should close help")

	// ? opens again, then ? closes (toggle).
	sendKey(m, "?")
	require.NotNil(t, m.helpState)
	sendKey(m, "?")
	assert.Nil(t, m.helpState, "? should toggle help closed")
}

func TestHelpRightPaneUpdatesOnSectionChange(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "?")
	require.NotNil(t, m.helpState)

	sections := m.helpSections()
	require.Greater(t, len(sections), 1)

	// Capture right pane content at section 0.
	contentAtZero := m.helpState.viewport.View()

	// Move to section 1.
	sendKey(m, "j")
	assert.Equal(t, 1, m.helpState.section)

	// Right pane should show different content.
	contentAtOne := m.helpState.viewport.View()
	assert.NotEqual(t, contentAtZero, contentAtOne,
		"right pane content should change when section changes")
}

func TestHelpTwoPaneShowsCursorIndicator(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "?")
	require.NotNil(t, m.helpState)

	view := m.helpView()
	assert.Contains(t, view, symTriRightSm,
		"help overlay should show cursor indicator")
}

func TestHelpTwoPaneShowsSectionNames(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "?")
	require.NotNil(t, m.helpState)

	view := m.helpView()
	sections := m.helpSections()
	for _, sec := range sections {
		assert.Contains(t, view, sec.title,
			"help overlay should show section name: "+sec.title)
	}
}

func TestHelpTwoPaneShowsHintBar(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	sendKey(m, "?")
	require.NotNil(t, m.helpState)

	view := m.helpView()
	assert.Contains(t, view, "sections",
		"help overlay should show sections navigation hint")
	assert.Contains(t, view, "close",
		"help overlay should show close hint")
}

func TestHelpOverlayAdaptiveResize(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 50
	sendKey(m, "?")
	require.NotNil(t, m.helpState)

	// Switch to Nav Mode (section 1) which has many entries.
	sendKey(m, "j")
	require.Equal(t, 1, m.helpState.section)

	// Record viewport height at full size.
	fullH := m.helpState.viewport.Height()
	require.Greater(t, fullH, 3, "viewport should have meaningful height at 50-row terminal")

	// Shrink terminal significantly — help should adapt.
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 25})
	require.NotNil(t, m.helpState, "help should stay open after resize")
	smallH := m.helpState.viewport.Height()
	assert.Less(t, smallH, fullH,
		"viewport should shrink when terminal shrinks (was %d, now %d)", fullH, smallH)

	// Grow terminal back — help should grow.
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	require.NotNil(t, m.helpState)
	restoredH := m.helpState.viewport.Height()
	assert.Equal(t, fullH, restoredH,
		"viewport should restore to original height (was %d, now %d)", fullH, restoredH)
}

func TestDeleteRequiresEditMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	// In normal mode, 'd' is half-page-down (table handles it).
	// It should NOT trigger delete.
	sendKey(m, "d")
	assert.Empty(t, m.status.Text)
	status := m.statusView()
	assert.Contains(t, status, "NAV", "'d' in normal mode should not change mode")
}

func TestEscClearsStatusInNormalMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.status = statusMsg{Text: "something", Kind: statusInfo}
	require.Contains(t, m.statusView(), "something")
	sendKey(m, "esc")
	assert.Empty(t, m.status.Text)
	assert.NotContains(t, m.statusView(), "something")
}

func TestProjectStatusFilterToggleKeys(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)

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
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	styles := DefaultStyles(true)
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
	t.Parallel()
	m := newTestModel(t)
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
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create a vendor (no FK children to block deletion).
	h := newVendorHandler()
	m.fs.formData = &vendorFormData{Name: "Test Vendor", Phone: "555-0000"}
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

func TestDeleteRespectsExplicitHideDeleted(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create a vendor.
	h := newVendorHandler()
	m.fs.formData = &vendorFormData{Name: "Test Vendor", Phone: "555-0000"}
	require.NoError(t, h.SubmitForm(m))

	// Switch to vendors tab and reload.
	m.active = tabIndex(tabVendors)
	require.NoError(t, m.reloadActiveTab())

	tab := m.activeTab()
	tab.Table.SetCursor(0)
	require.Len(t, tab.Rows, 1)
	assert.False(t, tab.ShowDeleted, "ShowDeleted should start off")

	// Enter edit mode and explicitly toggle hide-deleted: x shown, x hidden.
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)

	sendKey(m, "x")
	tab = m.activeTab()
	assert.True(t, tab.ShowDeleted, "first x should show deleted")
	sendKey(m, "x")
	tab = m.activeTab()
	assert.False(t, tab.ShowDeleted, "second x should hide deleted")

	// Delete the row while hide-deleted is active.
	sendKey(m, "d")

	tab = m.activeTab()
	assert.False(t, tab.ShowDeleted,
		"ShowDeleted must stay off when user explicitly hid deleted rows")
	assert.Empty(t, tab.Rows,
		"deleted row should be hidden because user explicitly chose to hide deleted")
	assert.Contains(t, m.status.Text, "Deleted",
		"status should confirm deletion")
}
