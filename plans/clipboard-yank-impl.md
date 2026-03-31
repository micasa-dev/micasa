<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Clipboard Yank Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Press `y` in normal mode to copy the focused cell's raw value to the system clipboard via BubbleTea v2's OSC 52 support.

**Architecture:** Add a `YankCell` keybinding to `AppKeyMap`, handle it in `handleNormalKeys` by reading the cell at the cursor and returning `tea.SetClipboard(value)`. Block it on the dashboard. Show status feedback.

**Tech Stack:** BubbleTea v2 (`tea.SetClipboard`), existing `cell`/`selectedCell` infrastructure.

**Spec:** `plans/clipboard-yank.md`

---

### Task 1: Add `YankCell` keybinding to `AppKeyMap`

**Files:**
- Modify: `internal/app/keybindings.go:54` (struct field), `keybindings.go:267` (init)

- [ ] **Step 1: Add field to `AppKeyMap` struct**

In `internal/app/keybindings.go`, add `YankCell` to the Normal mode section (after `Escape`, before the Edit mode comment):

```go
	Chat          key.Binding
	Escape        key.Binding
	YankCell      key.Binding

	// --- Edit mode (handleEditKeys) ---
```

- [ ] **Step 2: Initialize in `newAppKeyMap()`**

In `internal/app/keybindings.go`, add after the `Escape` binding init (after line 271, before the `// Edit mode` comment):

```go
		YankCell: key.NewBinding(key.WithKeys(keyY), key.WithHelp(keyY, "copy cell")),
```

- [ ] **Step 3: Build to verify**

Run: `go build ./internal/app/`

Expected: compiles (unused field is fine in a struct).

- [ ] **Step 4: Commit**

```
feat(ux): add YankCell keybinding to AppKeyMap

Wires keyY to a new YankCell binding in the normal mode section.
Handler comes in the next commit.
```

---

### Task 2: Block `YankCell` on the dashboard

**Files:**
- Modify: `internal/app/model_keys.go:54`

- [ ] **Step 1: Add `m.keys.YankCell` to the dashboard block list**

In `internal/app/model_keys.go`, line 54 has the block list for table-specific keys on the dashboard. Append `m.keys.YankCell`:

```go
	case key.Matches(msg, m.keys.Sort, m.keys.SortClear, m.keys.ColHide, m.keys.ColShowAll, m.keys.EnterEditMode, m.keys.ColFinder, m.keys.FilterPin, m.keys.FilterToggle, m.keys.FilterNegate, m.keys.YankCell):
		// Block table-specific keys on dashboard.
		return nil, true
```

- [ ] **Step 2: Build to verify**

Run: `go build ./internal/app/`

Expected: compiles.

- [ ] **Step 3: Commit**

```
feat(ux): block YankCell on dashboard

Prevents y from copying a cell from the hidden underlying table
while the dashboard is visible.
```

---

### Task 3: Write failing tests for yank

**Files:**
- Create: `internal/app/yank_test.go`

- [ ] **Step 1: Write all test cases**

Create `internal/app/yank_test.go`:

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
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
	assert.Equal(t, "Copied.", m.status.Text)
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
	assert.Equal(t, "Copied.", m.status.Text)

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -shuffle=on -run TestYank ./internal/app/`

Expected: most tests FAIL because `YankCell` is not handled yet (key falls
through, no status change). The two dashboard tests
(`TestYankCellBlockedOnDashboard`, `TestYankCellDashboardReturnsNilCmd`) PASS
because Task 2 already blocks `y` on the dashboard.

- [ ] **Step 3: Commit**

```
test(ux): add failing tests for clipboard yank

Tests cover: happy path with sendKey, null cell, empty string, no rows,
dashboard blocking, and supplementary m.Update tests verifying the
returned tea.Cmd payload and nil for no-op paths.
```

---

### Task 4: Implement the yank handler

**Files:**
- Modify: `internal/app/model_keys.go:231-239` (add case before `Escape`)

- [ ] **Step 1: Add `YankCell` case to `handleNormalKeys`**

In `internal/app/model_keys.go`, add before the `Escape` case (before line 233):

```go
	case key.Matches(msg, m.keys.YankCell):
		// Guard nil tab before accessing ColCursor. selectedCell also checks
		// internally, but we need the tab reference for the column index.
		tab := m.effectiveTab()
		if tab == nil {
			return nil, true
		}
		c, ok := m.selectedCell(tab.ColCursor)
		if !ok || c.Null || c.Value == "" {
			m.setStatusInfo("Nothing to copy.")
			return nil, true
		}
		m.setStatusInfo("Copied.")
		return tea.SetClipboard(c.Value), true
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test -shuffle=on -run TestYank ./internal/app/`

Expected: all tests PASS.

- [ ] **Step 3: Run full test suite**

Run: `go test -shuffle=on ./...`

Expected: all tests PASS. No regressions.

- [ ] **Step 4: Commit**

```
feat(ux): implement clipboard yank via OSC 52

Press y in normal mode to copy the focused cell's raw value to the
system clipboard using BubbleTea v2's tea.SetClipboard (OSC 52).
No external process execution, no new dependencies.
```

---

### Task 5: Add yank to the help overlay

**Files:**
- Modify: `internal/app/view_help.go:48-56`

- [ ] **Step 1: Add `YankCell` to the Nav Mode help section**

In `internal/app/view_help.go`, add `fromBinding(m.keys.YankCell)` to the "Nav Mode" entries. Place it after `Enter` and before `DocOpen` (after line 48):

```go
				fromBinding(m.keys.Enter),
				fromBinding(m.keys.YankCell),
				fromBinding(m.keys.DocOpen),
```

- [ ] **Step 2: Verify help text renders**

Run: `go test -shuffle=on -run TestHelp ./internal/app/`

Expected: PASS (or no existing test -- verify build succeeds).

Run: `go build ./internal/app/`

Expected: compiles.

- [ ] **Step 3: Commit**

```
docs(ux): add yank to help overlay

Shows y → copy cell in the Nav Mode help section.
```

---

### Task 6: Verify coverage and run linters

- [ ] **Step 1: Check coverage**

Run: `nix run '.#coverage'` (or `go test -coverprofile cover.out ./internal/app/ && go tool cover -func cover.out | rg yank`)

Expected: new code in `model_keys.go` (the `YankCell` case) is covered by tests.

- [ ] **Step 2: Run linter**

Run: `golangci-lint run ./internal/app/`

Expected: no warnings.

- [ ] **Step 3: Run pre-commit**

Run: `nix run '.#pre-commit'`

Expected: all checks pass.

- [ ] **Step 4: Final commit if any fixups needed**

Only if linters/coverage required changes. Otherwise skip.
