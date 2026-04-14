<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Mouse Cursor Shape Hints Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a pointer (hand) cursor when hovering over clickable zones in
the TUI, and reset to default elsewhere, using OSC 22 escape sequences.

**Architecture:** Write OSC 22 escape sequences to an `io.Writer` (default
`os.Stdout`) from `Update` when mouse motion enters/leaves clickable zones.
Switch from `MouseModeCellMotion` to `MouseModeAllMotion` in `View()` to
receive hover events. Inject the writer via a field on `Model` for
testability.

**Tech Stack:** Go, Bubble Tea v2 (`MouseMotionMsg`, `MouseModeAllMotion`),
bubblezone v2 (`InBounds`), OSC 22 terminal escape sequences.

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/app/cursor.go` | Create | OSC 22 constants, `setPointerShape`, `resetPointerShape`, `isOverClickableZone` |
| `internal/app/cursor_test.go` | Create | All cursor-shape tests (hover detection, shape transitions, quit reset) |
| `internal/app/mouse.go` | Modify | Add `handleMouseMotion` dispatch method |
| `internal/app/model.go` | Modify | Add `pointerWriter` and `lastPointerShape` fields to `Model`, default writer in `NewModel`, change `MouseMode` in `View()` |
| `internal/app/model_update.go` | Modify | Add `tea.MouseMotionMsg` case to `update()` switch |
| `internal/app/model_status.go` | Modify | Reset pointer shape on quit paths |
| `internal/app/mouse_test.go` | Modify | Add `sendMouseMotion` helper |
| `internal/app/bench_test.go` | Modify | Add `BenchmarkMouseMotion` |

---

### Task 1: OSC 22 constants and writer helpers

**Files:**
- Create: `internal/app/cursor.go`

- [ ] **Step 1: Create `cursor.go` with OSC 22 constants and helper functions**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import "io"

// OSC 22 escape sequences for mouse pointer shape control.
//
// Format: ESC ] 22 ; <shape> ST
// Where ST (String Terminator) is ESC \.
//
// Terminals that do not support OSC 22 silently ignore these sequences.
// See plans/631-mouse-cursor-hints.md for compatibility matrix.
const (
	pointerShapeDefault = ""
	pointerShapePointer = "pointer"

	osc22Prefix = "\x1b]22;"
	osc22Suffix = "\x1b\\"
)

// setPointerShape writes an OSC 22 escape sequence to change the mouse
// pointer shape. It only writes when the shape differs from the last
// written shape, avoiding redundant writes on every motion event.
//
// Returns the new shape value to store as lastPointerShape.
func setPointerShape(w io.Writer, shape, last string) string {
	if shape == last {
		return last
	}
	// Ignore write errors -- the terminal may not support OSC 22,
	// and there is no recovery action. The sequence is purely cosmetic.
	_, _ = io.WriteString(w, osc22Prefix+shape+osc22Suffix)
	return shape
}

// resetPointerShape unconditionally resets the pointer to the terminal
// default. Used during shutdown where we always want to ensure cleanup
// regardless of tracked state.
func resetPointerShape(w io.Writer) {
	_, _ = io.WriteString(w, osc22Prefix+pointerShapeDefault+osc22Suffix)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/app/`
Expected: clean build, no errors.

- [ ] **Step 3: Commit**

```
feat(ux): add OSC 22 cursor shape constants and helpers

Part of #631.
```

---

### Task 2: Add `pointerWriter` and `lastPointerShape` to Model

**Files:**
- Modify: `internal/app/model.go` (Model struct, NewModel, View)

- [ ] **Step 1: Add fields to Model struct**

In `internal/app/model.go`, add two fields to the `Model` struct after
`lastDashClick rowClickState`:

```go
	lastPointerShape string    // "pointer" or "" (default); tracks OSC 22 state
	pointerWriter    io.Writer // target for OSC 22 escape sequences (default os.Stdout)
```

Add `"io"` and `"os"` to the import block if not already present.

- [ ] **Step 2: Set default writer in NewModel**

In `NewModel`, after the model struct literal (after the line
`syncCfg: options.syncCfg,`), but before the `if cfg := options.syncCfg`
block, add:

```go
	model.pointerWriter = os.Stdout
```

- [ ] **Step 3: Change MouseMode in View()**

In the `View()` method, change:

```go
	v.MouseMode = tea.MouseModeCellMotion
```

to:

```go
	v.MouseMode = tea.MouseModeAllMotion
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./internal/app/`
Expected: clean build.

- [ ] **Step 5: Commit**

```
feat(ux): add pointer writer and switch to MouseModeAllMotion

Part of #631.
```

---

### Task 3: Zone hover detection

**Files:**
- Modify: `internal/app/cursor.go`

- [ ] **Step 1: Add `isOverClickableZone` to `cursor.go`**

This method mirrors the dispatch structure of `handleLeftClick` and
`handleOverlayClick` from `mouse.go`, but only checks whether any zone
contains the mouse position.

```go
// isOverClickableZone returns true if the mouse position is within any
// clickable zone. It mirrors the zone checks in handleLeftClick and
// handleOverlayClick but only tests containment, executing no actions.
func (m *Model) isOverClickableZone(msg tea.MouseMotionMsg) bool {
	// Overlay zones take priority when an overlay is active.
	if m.hasActiveOverlay() {
		if m.isOverOverlayZone(msg) {
			return true
		}
		// When an overlay is active, base zones are not clickable.
		return false
	}

	return m.isOverBaseZone(msg)
}

// isOverBaseZone checks non-overlay clickable zones: tabs, rows, columns,
// hints, house header, and breadcrumb back.
func (m *Model) isOverBaseZone(msg tea.MouseMotionMsg) bool {
	// Tab bar.
	if !m.tabsLocked() && !m.inDetail() {
		for i := range m.tabs {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneTab, i)).InBounds(msg) {
				return true
			}
		}
	}

	// Breadcrumb back.
	if m.inDetail() {
		if m.zones.Get(zoneBreadcrumb).InBounds(msg) {
			return true
		}
	}

	// House header.
	if m.zones.Get(zoneHouse).InBounds(msg) {
		return true
	}

	// Column headers and table rows.
	if tab := m.effectiveTab(); tab != nil {
		vp := m.tabViewport(tab)
		for i := range vp.Specs {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneCol, i)).InBounds(msg) {
				return true
			}
		}

		total := len(tab.CellRows)
		if total > 0 {
			cursor := tab.Table.Cursor()
			height := tab.Table.Height()
			badges := renderHiddenBadges(tab.Specs, tab.ColCursor)
			if badges != "" {
				height--
			}
			if len(tab.Rows) > 0 {
				height--
			}
			if height < 2 {
				height = 2
			}
			start, end := visibleRange(total, height, cursor)
			for i := start; i < end; i++ {
				if m.zones.Get(fmt.Sprintf("%s%d", zoneRow, i)).InBounds(msg) {
					return true
				}
			}
		}
	}

	// Status bar hints.
	hintIDs := []string{"edit", "help", "add", "exit", "enter", "del", "open", "search", "ask"}
	for _, id := range hintIDs {
		if m.zones.Get(zoneHint + id).InBounds(msg) {
			return true
		}
	}

	return false
}

// isOverOverlayZone checks clickable zones within active overlays:
// dashboard rows, house fields, search results, ops tree nodes/tabs,
// and extraction preview elements.
func (m *Model) isOverOverlayZone(msg tea.MouseMotionMsg) bool {
	// House overlay fields.
	if m.houseOverlay != nil {
		for _, d := range houseFieldDefs() {
			if d.section == houseSectionIdentity {
				continue
			}
			if m.zones.Get(zoneHouseField + d.key).InBounds(msg) {
				return true
			}
		}
	}

	// Dashboard rows.
	if m.dashboardVisible() {
		for i := range m.dash.nav {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneDashRow, i)).InBounds(msg) {
				return true
			}
		}
	}

	// Search results.
	if ds := m.docSearch; ds != nil {
		for i := range ds.Results {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneSearchRow, i)).InBounds(msg) {
				return true
			}
		}
	}

	// Ops tree nodes.
	if tree := m.opsTree; tree != nil {
		nodes := tree.visibleNodes()
		for i := range nodes {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneOpsNode, i)).InBounds(msg) {
				return true
			}
		}
		for i := range tree.previewGroups {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneOpsTab, i)).InBounds(msg) {
				return true
			}
		}
	}

	// Extraction preview.
	if ex := m.ex.extraction; ex != nil && ex.Visible && ex.exploring {
		for i := range ex.previewGroups {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneExtTab, i)).InBounds(msg) {
				return true
			}
		}
		if g := ex.activePreviewGroup(); g != nil {
			for i := range g.cells {
				if m.zones.Get(fmt.Sprintf("%s%d", zoneExtRow, i)).InBounds(msg) {
					return true
				}
			}
			for i := range g.specs {
				if m.zones.Get(fmt.Sprintf("%s%d", zoneExtCol, i)).InBounds(msg) {
					return true
				}
			}
		}
	}

	return false
}
```

Add `"fmt"` and `tea "charm.land/bubbletea/v2"` to the import block in
`cursor.go`.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/app/`
Expected: clean build.

- [ ] **Step 3: Commit**

```
feat(ux): add zone hover detection for cursor hints

Part of #631.
```

---

### Task 4: Motion dispatch and pointer shape update

**Files:**
- Modify: `internal/app/mouse.go` (add `handleMouseMotion`)
- Modify: `internal/app/model_update.go` (add `MouseMotionMsg` case)

- [ ] **Step 1: Add `handleMouseMotion` to `mouse.go`**

Add after the `handleMouseWheel` method:

```go
// handleMouseMotion updates the mouse pointer shape based on whether the
// cursor is over a clickable zone. This writes OSC 22 escape sequences
// directly to pointerWriter (typically stdout) outside of the View cycle.
func (m *Model) handleMouseMotion(msg tea.MouseMotionMsg) {
	if m.isOverClickableZone(msg) {
		m.lastPointerShape = setPointerShape(m.pointerWriter, pointerShapePointer, m.lastPointerShape)
	} else {
		m.lastPointerShape = setPointerShape(m.pointerWriter, pointerShapeDefault, m.lastPointerShape)
	}
}
```

- [ ] **Step 2: Add `MouseMotionMsg` case to `update()`**

In `internal/app/model_update.go`, add a new case after the
`tea.MouseWheelMsg` case (line ~131):

```go
	case tea.MouseMotionMsg:
		m.handleMouseMotion(typed)
		return m, nil
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/app/`
Expected: clean build.

- [ ] **Step 4: Commit**

```
feat(ux): dispatch mouse motion events for cursor shape hints

Part of #631.
```

---

### Task 5: Reset pointer shape on quit

**Files:**
- Modify: `internal/app/model_update.go` (quit in `update()`)
- Modify: `internal/app/model_status.go` (quit in `handleConfirmDiscard`)

- [ ] **Step 1: Add pointer reset to normal quit path**

In `internal/app/model_update.go`, in the `tea.KeyPressMsg` handler for
`m.keys.Quit`, add a pointer reset before returning `tea.Quit`. The block
currently reads:

```go
			if m.appCancel != nil {
				m.appCancel()
			}
			m.cancelChatOperations()
			m.cancelAllExtractions()
			m.cancelPull()
			if m.syncCancel != nil {
				m.syncCancel()
			}
			return m, tea.Quit
```

Add the reset call before the return:

```go
			if m.appCancel != nil {
				m.appCancel()
			}
			m.cancelChatOperations()
			m.cancelAllExtractions()
			m.cancelPull()
			if m.syncCancel != nil {
				m.syncCancel()
			}
			resetPointerShape(m.pointerWriter)
			return m, tea.Quit
```

- [ ] **Step 2: Add pointer reset to confirm-discard quit path**

In `internal/app/model_status.go`, in `handleConfirmDiscard`, the
`confirmFormQuitDiscard` branch currently reads:

```go
		if m.confirm == confirmFormQuitDiscard {
			m.confirm = confirmNone
			if m.appCancel != nil {
				m.appCancel()
			}
			m.cancelChatOperations()
			m.cancelPull()
			return m, tea.Quit
		}
```

Add the reset before the return:

```go
		if m.confirm == confirmFormQuitDiscard {
			m.confirm = confirmNone
			if m.appCancel != nil {
				m.appCancel()
			}
			m.cancelChatOperations()
			m.cancelPull()
			resetPointerShape(m.pointerWriter)
			return m, tea.Quit
		}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/app/`
Expected: clean build.

- [ ] **Step 4: Commit**

```
feat(ux): reset pointer shape on quit

Part of #631.
```

---

### Task 6: Test helper and hover detection tests

**Files:**
- Modify: `internal/app/mouse_test.go` (add `sendMouseMotion`)
- Create: `internal/app/cursor_test.go`

- [ ] **Step 1: Add `sendMouseMotion` helper to `mouse_test.go`**

Add after the `sendClick` function:

```go
// sendMouseMotion sends a mouse motion event to the model at the given position.
func sendMouseMotion(m *Model, x, y int) {
	m.Update(tea.MouseMotionMsg{X: x, Y: y})
}
```

- [ ] **Step 2: Write cursor shape tests in `cursor_test.go`**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetPointerShape_SkipsRedundantWrite(t *testing.T) {
	var buf bytes.Buffer
	result := setPointerShape(&buf, pointerShapePointer, pointerShapePointer)
	assert.Equal(t, pointerShapePointer, result)
	assert.Empty(t, buf.String(), "should not write when shape unchanged")
}

func TestSetPointerShape_WritesOnChange(t *testing.T) {
	var buf bytes.Buffer
	result := setPointerShape(&buf, pointerShapePointer, pointerShapeDefault)
	assert.Equal(t, pointerShapePointer, result)
	assert.Equal(t, "\x1b]22;pointer\x1b\\", buf.String())
}

func TestResetPointerShape(t *testing.T) {
	var buf bytes.Buffer
	resetPointerShape(&buf)
	assert.Equal(t, "\x1b]22;\x1b\\", buf.String())
}

func TestHoverOverTab_SetsPointerShape(t *testing.T) {
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneTab+"0")
	sendMouseMotion(m, z.StartX, z.StartY)

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
	assert.Contains(t, buf.String(), "pointer")
}

func TestHoverOffZone_ResetsPointerShape(t *testing.T) {
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneTab+"0")

	// Move onto zone.
	sendMouseMotion(m, z.StartX, z.StartY)
	require.Equal(t, pointerShapePointer, m.lastPointerShape)

	// Move off all zones (far corner).
	buf.Reset()
	sendMouseMotion(m, 0, 0)
	assert.Equal(t, pointerShapeDefault, m.lastPointerShape)
	assert.Contains(t, buf.String(), "\x1b]22;\x1b\\")
}

func TestHoverOverRow_SetsPointerShape(t *testing.T) {
	m := newTestModelWithDemoData(t, 42)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)

	z := requireZone(t, m, zoneRow+"0")
	sendMouseMotion(m, z.StartX, z.StartY)

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
}

func TestHoverOverHint_SetsPointerShape(t *testing.T) {
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneHint+"help")
	sendMouseMotion(m, z.StartX, z.StartY)

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
}

func TestHoverOverHouseHeader_SetsPointerShape(t *testing.T) {
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneHouse)
	sendMouseMotion(m, z.StartX, z.StartY)

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
}

func TestHoverOverColumnHeader_SetsPointerShape(t *testing.T) {
	m := newTestModelWithDemoData(t, 42)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneCol+"0")
	sendMouseMotion(m, z.StartX, z.StartY)

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
}

func TestHoverRedundantMotion_NoExtraWrite(t *testing.T) {
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneTab+"0")

	// First motion onto zone.
	sendMouseMotion(m, z.StartX, z.StartY)
	firstLen := buf.Len()
	require.Greater(t, firstLen, 0)

	// Second motion on same zone -- no additional write.
	sendMouseMotion(m, z.StartX+1, z.StartY)
	assert.Equal(t, firstLen, buf.Len(), "redundant motion should not write")
}

func TestQuitResetsPointerShape(t *testing.T) {
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	// Set pointer to non-default first.
	z := requireZone(t, m, zoneTab+"0")
	sendMouseMotion(m, z.StartX, z.StartY)
	require.Equal(t, pointerShapePointer, m.lastPointerShape)

	buf.Reset()

	// Quit via ctrl+q.
	sendKey(m, keyCtrlQ)

	assert.Contains(t, buf.String(), "\x1b]22;\x1b\\", "quit should reset pointer")
}

func TestConfirmQuitResetsPointerShape(t *testing.T) {
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	// Enter form mode to trigger confirm-discard path.
	m.enterEditMode()
	m.startAddForm()
	require.Equal(t, modeForm, m.mode)
	m.fs.formDirty = true

	// Set pointer to non-default.
	m.lastPointerShape = pointerShapePointer

	buf.Reset()

	// ctrl+q triggers confirmation.
	sendKey(m, keyCtrlQ)
	require.Equal(t, confirmFormQuitDiscard, m.confirm)

	// Confirm with 'y'.
	sendKey(m, "y")

	assert.Contains(t, buf.String(), "\x1b]22;\x1b\\", "confirm quit should reset pointer")
}

func TestMouseMotionMsg_DispatchedInUpdate(t *testing.T) {
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	// Dispatch a raw MouseMotionMsg through Update.
	z := requireZone(t, m, zoneTab+"0")
	m.Update(tea.MouseMotionMsg{X: z.StartX, Y: z.StartY})

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
}

func TestViewUsesAllMotion(t *testing.T) {
	m := newTestModelWithStore(t)
	v := m.View()
	assert.Equal(t, tea.MouseModeAllMotion, v.MouseMode)
}

func TestOverlayBlocksBaseZones(t *testing.T) {
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	// Open dashboard overlay.
	m.showDashboard = true
	_ = m.loadDashboardAt(time.Now())

	tabZone := requireZone(t, m, zoneTab+"0")

	// Hover over where a tab would be -- overlay blocks it.
	sendMouseMotion(m, tabZone.StartX, tabZone.StartY)
	assert.Equal(t, pointerShapeDefault, m.lastPointerShape,
		"base zones should not be clickable when overlay is active")
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `go test -shuffle=on -run 'TestSetPointerShape|TestResetPointerShape|TestHover|TestQuit|TestConfirmQuit|TestMouseMotionMsg|TestViewUsesAllMotion|TestOverlayBlocksBaseZones' ./internal/app/`
Expected: all pass.

- [ ] **Step 4: Commit**

```
test(ux): add cursor shape hover detection tests

Part of #631.
```

---

### Task 7: Benchmark

**Files:**
- Modify: `internal/app/bench_test.go`

- [ ] **Step 1: Add `BenchmarkMouseMotion` to `bench_test.go`**

Add after `BenchmarkSelectClickedColumn`:

```go
func BenchmarkMouseMotion(b *testing.B) {
	m := benchModel(b)
	// Pre-populate zone cache.
	_ = m.View()
	_ = m.View()
	var buf bytes.Buffer
	m.pointerWriter = &buf
	msg := tea.MouseMotionMsg{X: 20, Y: 5}
	b.ResetTimer()
	for b.Loop() {
		buf.Reset()
		m.handleMouseMotion(msg)
	}
}
```

Add `"bytes"` to the import block.

- [ ] **Step 2: Run benchmark to verify it works**

Run: `go test -bench BenchmarkMouseMotion -benchmem -run ^$ ./internal/app/`
Expected: benchmark runs, reports ns/op and allocs/op.

- [ ] **Step 3: Commit**

```
test(ux): add mouse motion benchmark

Part of #631.
```

---

### Task 8: Full test suite verification

- [ ] **Step 1: Run full test suite**

Run: `go test -shuffle=on ./...`
Expected: all tests pass.

- [ ] **Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: no warnings.

- [ ] **Step 3: Final commit (if any lint fixes needed)**

Only if linter finds issues in new code. Otherwise skip.
