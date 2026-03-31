// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createProjectAndReload creates a project via user-interaction keypresses
// (openAddForm + ctrl+s) and reloads the model so CellRows are populated.
// Returns the model to normal mode so yank (handleNormalKeys) is reachable.
func createProjectAndReload(t *testing.T, m *Model, title string) {
	t.Helper()
	openAddForm(m)
	values, ok := m.fs.formData.(*projectFormData)
	require.True(t, ok)
	values.Title = title
	sendKey(m, "ctrl+s")
	sendKey(m, "esc") // exits form → back to edit mode
	sendKey(m, "esc") // exits edit mode → back to normal mode
	require.Equal(t, modeNormal, m.mode, "must be in normal mode for yank tests")
	m.reloadAll()
}

func TestYankCellCopiesValue(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	createProjectAndReload(t, m, "Kitchen Remodel")

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows, "need data rows")

	// Navigate to the Title column.
	tab.ColCursor = int(projectColTitle)

	sendKey(m, keyY)
	assert.Contains(t, m.status.Text, "Copied")
	assert.Contains(t, m.status.Text, "Kitchen Remodel")
	assert.Equal(t, statusStyled, m.status.Kind)
}

func TestYankCellNullShowsNothing(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	createProjectAndReload(t, m, "Roof Repair")

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)

	// Budget is NULL for a new project with no budget set.
	tab.ColCursor = int(projectColBudget)

	// Verify the cell is actually null before testing yank.
	c, ok := m.selectedCell(tab.ColCursor)
	require.True(t, ok)
	require.True(t, c.Null, "expected NULL cell for budget on a new project")

	sendKey(m, keyY)
	assert.Equal(t, "Nothing to copy.", m.status.Text)
}

func TestYankCellEmptyStringShowsNothing(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	createProjectAndReload(t, m, "Fence Install")

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)

	// Force a cell to empty string to test this edge case reliably.
	// The schema may not naturally produce empty-string cells (most are
	// NULL), so we set one directly like filter_test.go and notes_test.go.
	tab.ColCursor = int(projectColTitle)
	tab.CellRows[0][tab.ColCursor] = cell{Value: "", Kind: cellText}

	sendKey(m, keyY)
	assert.Equal(t, "Nothing to copy.", m.status.Text)
}

func TestYankCellNoRowsShowsNothing(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	// No data created -- tabs are empty.
	tab := m.activeTab()
	require.Empty(t, tab.CellRows, "expected no data rows")

	sendKey(m, keyY)
	assert.Equal(t, "Nothing to copy.", m.status.Text)
}

func TestYankCellBlockedOnDashboard(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.showDashboard = true
	m.dash.data = nonEmptyDashboard() // dashboardVisible requires non-empty data

	// Set a status to verify it is NOT changed.
	m.setStatusInfo("before")

	sendKey(m, keyY)
	assert.Equal(t, "before", m.status.Text, "dashboard should block yank")
}

func TestYankCellTruncatesLongValue(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	createProjectAndReload(t, m, "Short")
	m.width = 40 // narrow viewport forces truncation

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)

	// Inject a value with wide CJK glyphs. Each CJK character is 2 columns
	// wide, so 40 of them occupy 80 columns — well over the 40-column viewport.
	long := strings.Repeat("\u4e16", 40) // 世 repeated 40 times = 80 columns
	tab.ColCursor = int(projectColTitle)
	tab.CellRows[0][tab.ColCursor] = cell{Value: long, Kind: cellText}

	_, cmd := m.Update(keyPress(keyY))
	require.NotNil(t, cmd)

	// Status should be truncated with ellipsis.
	assert.Contains(t, m.status.Text, "…")
	assert.NotContains(t, m.status.Text, long, "full value should not appear in status")

	// Clipboard should contain the full raw value.
	msg := cmd()
	assert.Equal(t, long, fmt.Sprint(msg))
}

func TestYankCellTruncatesGraphemeClusters(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	createProjectAndReload(t, m, "Short")
	m.width = 40 // narrow viewport forces truncation

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)

	// ZWJ family emoji: each cluster is multiple runes but 2 columns wide.
	// 35 clusters = 70 display columns, exceeding the 40-column viewport.
	family := "\U0001F468\u200D\U0001F469\u200D\U0001F467" // 👨‍👩‍👧
	long := strings.Repeat(family, 35)
	tab.ColCursor = int(projectColTitle)
	tab.CellRows[0][tab.ColCursor] = cell{Value: long, Kind: cellText}

	_, cmd := m.Update(keyPress(keyY))
	require.NotNil(t, cmd)

	// Status should be truncated without splitting a grapheme cluster.
	assert.Contains(t, m.status.Text, "…")
	assert.NotContains(t, m.status.Text, long, "full value should not appear in status")

	// The truncated value (between "Copied: " prefix and "…") should contain
	// only complete family emoji clusters -- no partial runes.
	assert.NotContains(t, m.status.Text, "\uFFFD",
		"truncation should not produce replacement characters")

	// Clipboard should contain the full raw value.
	msg := cmd()
	assert.Equal(t, long, fmt.Sprint(msg))
}

func TestYankCellSanitizesStatusButNotClipboard(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	createProjectAndReload(t, m, "Test Project")

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)

	// Inject a cell value with ASCII and Unicode control/separator characters.
	// U+0085 (NEL), U+2028 (line separator), U+2029 (paragraph separator).
	raw := "line one\nline two\ttab\x00null\u0085nel\u2028lsep\u2029psep"
	tab.ColCursor = int(projectColTitle)
	tab.CellRows[0][tab.ColCursor] = cell{Value: raw, Kind: cellText}

	_, cmd := m.Update(keyPress(keyY))
	require.NotNil(t, cmd)

	// Status should be sanitized (single line, no control chars).
	assert.Contains(t, m.status.Text, "line one line two tab null nel lsep psep")
	assert.NotContains(t, m.status.Text, "\n")
	assert.NotContains(t, m.status.Text, "\t")

	// Clipboard should contain the exact raw value.
	msg := cmd()
	assert.Equal(t, raw, fmt.Sprint(msg))
}

func TestYankCellMoneyStripsSymbol(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	createProjectAndReload(t, m, "Window Replace")

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)

	// Set a money cell with a currency-formatted value.
	tab.ColCursor = int(projectColBudget)
	tab.CellRows[0][tab.ColCursor] = cell{Value: m.cur.FormatCents(77653), Kind: cellMoney}

	_, cmd := m.Update(keyPress(keyY))
	require.NotNil(t, cmd)

	// Clipboard should contain the number without the currency symbol.
	msg := cmd()
	clipValue := fmt.Sprint(msg)
	assert.NotContains(t, clipValue, m.cur.Symbol(), "clipboard should not contain currency symbol")
	assert.Contains(t, clipValue, "776.53")

	// Status display also shows the stripped value.
	assert.Contains(t, m.status.Text, "776.53")
}

func TestYankOpsCellCopiesJSON(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t) // creates a document with testOpsJSON

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	tab.ColCursor = int(documentColOps)

	_, cmd := m.Update(keyPress(keyY))
	require.NotNil(t, cmd, "expected clipboard command for ops cell")

	// Clipboard should contain valid pretty-printed JSON, not the count.
	clipValue := fmt.Sprint(cmd())
	assert.NotEqual(t, "2", clipValue, "should not copy the count")

	var ops []map[string]any
	require.NoError(t, json.Unmarshal([]byte(clipValue), &ops), "clipboard should be valid JSON")
	require.Len(t, ops, 2)
	assert.Equal(t, "create", ops[0]["action"])
	assert.Equal(t, "vendors", ops[0]["table"])
	data0, ok := ops[0]["data"].(map[string]any)
	require.True(t, ok, "ops[0].data should be a map")
	assert.Equal(t, "Garcia Plumbing", data0["name"])
	assert.Equal(t, "update", ops[1]["action"])
	assert.Equal(t, "documents", ops[1]["table"])
	data1, ok := ops[1]["data"].(map[string]any)
	require.True(t, ok, "ops[1].data should be a map")
	assert.Equal(t, "Invoice #42", data1["title"])

	// Status should show the JSON summary label.
	assert.Contains(t, m.status.Text, "JSON")
	assert.Contains(t, m.status.Text, "2 ops")
}

// --- Supplementary: verify the returned tea.Cmd ---

func TestYankCellCmdCarriesCorrectValue(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	createProjectAndReload(t, m, "Bathroom Tile")

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)
	tab.ColCursor = int(projectColTitle)

	c, ok := m.selectedCell(tab.ColCursor)
	require.True(t, ok)
	require.NotEmpty(t, c.Value)

	_, cmd := m.Update(keyPress(keyY))
	require.NotNil(t, cmd, "expected clipboard command")
	assert.Contains(t, m.status.Text, "Copied")
	assert.Contains(t, m.status.Text, "Bathroom Tile")

	// Execute the command and verify the clipboard payload.
	// tea.SetClipboard returns a Cmd whose Msg is setClipboardMsg (a named
	// string type), so fmt.Sprint yields the underlying string value.
	msg := cmd()
	assert.Equal(t, c.Value, fmt.Sprint(msg))
}

func TestYankCellNullReturnsNilCmd(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	createProjectAndReload(t, m, "Siding Replace")

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)
	tab.ColCursor = int(projectColBudget)

	c, ok := m.selectedCell(tab.ColCursor)
	require.True(t, ok)
	require.True(t, c.Null)

	_, cmd := m.Update(keyPress(keyY))
	assert.Nil(t, cmd, "null cell should not produce clipboard command")
	assert.Equal(t, "Nothing to copy.", m.status.Text)
}

func TestYankCellEmptyStringReturnsNilCmd(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	createProjectAndReload(t, m, "Gutter Clean")

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)
	tab.ColCursor = int(projectColTitle)
	tab.CellRows[0][tab.ColCursor] = cell{Value: "", Kind: cellText}

	_, cmd := m.Update(keyPress(keyY))
	assert.Nil(t, cmd, "empty string cell should not produce clipboard command")
	assert.Equal(t, "Nothing to copy.", m.status.Text)
}

func TestYankCellNoRowsReturnsNilCmd(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	tab := m.activeTab()
	require.Empty(t, tab.CellRows)

	_, cmd := m.Update(keyPress(keyY))
	assert.Nil(t, cmd, "no rows should not produce clipboard command")
	assert.Equal(t, "Nothing to copy.", m.status.Text)
}

func TestYankCellDashboardReturnsNilCmd(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.showDashboard = true
	m.dash.data = nonEmptyDashboard() // dashboardVisible requires non-empty data
	m.setStatusInfo("before")

	_, cmd := m.Update(keyPress(keyY))
	assert.Nil(t, cmd, "dashboard should block yank")
	assert.Equal(t, "before", m.status.Text)
}
