<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Ops Tree Table Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a compact table preview below the JSON tree in the ops tree overlay, grouped by entity table.

**Architecture:** Add `previewGroups` and `previewTab` to `opsTreeState`. Parse ops in `openOpsTree()` using existing `groupOperationsByTable()`. Render table below tree using `renderPreviewTable` in non-interactive mode (which passes -1 for cursors when `interactive=false`, so the coupling to `m.ex.extraction` is safe). Add `b`/`f` tab switching and `ops-tab-N` mouse zones.

**Tech Stack:** Go, Bubble Tea, lipgloss, bubblezone

---

### Task 1: Add state fields and parse ops in openOpsTree

**Files:**
- Modify: `internal/app/ops_tree.go:59-65` (opsTreeState struct)
- Modify: `internal/app/ops_tree.go:279-326` (openOpsTree method)

- [ ] **Step 1: Write failing test — preview groups are populated when opening overlay**

Add to `internal/app/ops_tree_test.go`:

```go
func TestOpsTreePreviewGroupsPopulated(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")

	require.NotNil(t, m.opsTree)
	require.NotEmpty(t, m.opsTree.previewGroups, "preview groups should be populated")

	// testOpsJSON has vendors create + documents update -> 2 groups.
	assert.Len(t, m.opsTree.previewGroups, 2)
	assert.Equal(t, 0, m.opsTree.previewTab)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -shuffle=on -run TestOpsTreePreviewGroupsPopulated ./internal/app/`
Expected: FAIL — `opsTreeState` has no `previewGroups` field.

- [ ] **Step 3: Add fields to opsTreeState and parse in openOpsTree**

In `internal/app/ops_tree.go`, add fields to `opsTreeState`:

```go
type opsTreeState struct {
	root          []*jsonTreeNode
	cursor        int
	expanded      map[string]bool
	docTitle      string
	maxNodes      int
	previewGroups []previewTableGroup
	previewTab    int
}
```

In `openOpsTree()`, after setting `m.opsTree`, add ops parsing. Add
`"encoding/json"` and the extract import to the file imports:

```go
import (
	// ... existing imports ...
	"encoding/json"

	"github.com/micasa-dev/micasa/internal/extract"
)
```

After `m.opsTree = &opsTreeState{...}`, parse and build preview groups:

```go
	var ops []extract.Operation
	if err := json.Unmarshal(doc.ExtractionOps, &ops); err == nil && len(ops) > 0 {
		m.opsTree.previewGroups = groupOperationsByTable(ops, m.cur)
	}

	// Account for table preview section height in maxNodes so the overlay
	// doesn't jump when collapsing tree nodes.
	if groups := m.opsTree.previewGroups; len(groups) > 0 {
		maxRows := 0
		for _, g := range groups {
			if len(g.cells) > maxRows {
				maxRows = len(g.cells)
			}
		}
		// divider(1) + header(1) + table-divider(1) + rows
		extra := 3 + maxRows
		if len(groups) > 1 {
			// tab-bar(1) + underline(1)
			extra += 2
		}
		m.opsTree.maxNodes += extra
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -shuffle=on -run TestOpsTreePreviewGroupsPopulated ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

Use `/commit`.

---

### Task 2: Render table preview below tree

**Files:**
- Modify: `internal/app/ops_tree.go:386-429` (buildOpsTreeOverlay)

- [ ] **Step 1: Write failing tests — table preview content appears in overlay**

Add to `internal/app/ops_tree_test.go`:

```go
func TestOpsTreeTablePreviewRendersInView(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)
	require.NotEmpty(t, m.opsTree.previewGroups)

	view := m.buildView()

	// Table preview should show column headers from the vendor preview.
	assert.Contains(t, view, "Name")
	assert.Contains(t, view, "Email")
	assert.Contains(t, view, "Phone")

	// And the data values.
	assert.Contains(t, view, "Garcia Plumbing")
	assert.Contains(t, view, "info@garcia.com")
	assert.Contains(t, view, "555-1234")
}

func TestOpsTreeNoTablePreviewForUnknownTables(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Ops targeting a table with no previewColumns mapping.
	unknownOps := []byte(`[{"action":"create","table":"unknown_table","data":{"foo":"bar"}}]`)
	doc := &data.Document{
		Title:         "Unknown Table Doc",
		FileName:      "unknown.pdf",
		MIMEType:      "application/pdf",
		ExtractionOps: unknownOps,
	}
	require.NoError(t, m.store.CreateDocument(doc))

	m.active = tabIndex(tabDocuments)
	require.NoError(t, m.reloadTab(m.effectiveTab()))

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	// previewGroups should be empty since unknown_table has no column defs.
	assert.Empty(t, m.opsTree.previewGroups)

	// Should still render without crashing.
	view := m.buildView()
	assert.Contains(t, view, "operations")
}

func TestOpsTreeSingleGroupNoTabBar(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Single-table ops (vendors only).
	singleOps := []byte(`[{"action":"create","table":"vendors","data":{"name":"Solo Vendor","email":"solo@test.com"}}]`)
	doc := &data.Document{
		Title:         "Single Table Doc",
		FileName:      "single.pdf",
		MIMEType:      "application/pdf",
		ExtractionOps: singleOps,
	}
	require.NoError(t, m.store.CreateDocument(doc))

	m.active = tabIndex(tabDocuments)
	require.NoError(t, m.reloadTab(m.effectiveTab()))

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)
	require.Len(t, m.opsTree.previewGroups, 1)

	view := m.buildView()
	// Should show the table data but no tab bar.
	assert.Contains(t, view, "Solo Vendor")
	// b/f hint should not appear for single group.
	assert.NotContains(t, view, "tabs")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -shuffle=on -run 'TestOpsTreeTablePreviewRendersInView|TestOpsTreeNoTablePreviewForUnknownTables|TestOpsTreeSingleGroupNoTabBar' ./internal/app/`
Expected: FAIL — table preview not rendered yet.

- [ ] **Step 3: Add table rendering to buildOpsTreeOverlay**

Add the `zoneOpsTab` constant near `zoneOpsNode`:

```go
const zoneOpsTab = "ops-tab-"
```

In `buildOpsTreeOverlay`, after the tree padding loop and before the hint
bar, add the table preview section. Use `renderPreviewTable` in
non-interactive mode:

```go
	// Table preview section.
	if groups := tree.previewGroups; len(groups) > 0 {
		// Divider.
		b.WriteString(appStyles.TextDim().Render(strings.Repeat(symHLine, innerW)))
		b.WriteString("\n")

		// Tab bar (only if multiple groups).
		if len(groups) > 1 {
			tabParts := make([]string, 0, len(groups)*2)
			for i, g := range groups {
				var rendered string
				if i == tree.previewTab {
					rendered = m.styles.TabActive().Render(g.name)
				} else {
					rendered = m.styles.TabInactive().Render(g.name)
				}
				tabParts = append(tabParts, m.zones.Mark(
					fmt.Sprintf("%s%d", zoneOpsTab, i), rendered,
				))
				if i < len(groups)-1 {
					tabParts = append(tabParts, "   ")
				}
			}
			b.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, tabParts...))
			b.WriteString("\n")
			b.WriteString(m.styles.TabUnderline().Render(
				strings.Repeat(symHLineHeavy, innerW),
			))
			b.WriteString("\n")
		}

		// Active table (non-interactive, dimmed).
		tabIdx := tree.previewTab
		if tabIdx >= len(groups) {
			tabIdx = 0
		}
		g := groups[tabIdx]
		sep := m.styles.TableSeparator().Render(" " + symVLine + " ")
		divSep := m.styles.TableSeparator().Render(symHLine + symCross + symHLine)
		sepW := lipgloss.Width(sep)
		tableStr := m.renderPreviewTable(g, innerW, sepW, sep, divSep, false)
		b.WriteString(appStyles.TextDim().Render(tableStr))
		b.WriteString("\n")
	}
```

Also update the hint bar to include tab hints when multiple groups exist.
Replace the existing `hints` assignment:

```go
	hintParts := []string{
		m.helpItem(keyJ+"/"+keyK, "nav"),
		m.helpItem(symReturn, "toggle"),
		m.helpItem(keyH, "collapse"),
	}
	if len(tree.previewGroups) > 1 {
		hintParts = append(hintParts, m.helpItem(keyB+"/"+keyF, "tabs"))
	}
	hintParts = append(hintParts, m.helpItem(keyEsc, "close"))
	hints := joinWithSeparator(m.helpSeparator(), hintParts...)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -shuffle=on -run 'TestOpsTreeTablePreviewRendersInView|TestOpsTreeNoTablePreviewForUnknownTables|TestOpsTreeSingleGroupNoTabBar' ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

Use `/commit`.

---

### Task 3: Add b/f tab switching keys

**Files:**
- Modify: `internal/app/ops_tree.go:329-383` (handleOpsTreeKey)

- [ ] **Step 1: Write failing test — tab switching with b/f**

Add to `internal/app/ops_tree_test.go`. Requires a model with ops spanning
multiple tables:

```go
var testMultiTableOpsJSON = []byte(`[
	{"action":"create","table":"vendors","data":{"name":"Garcia Plumbing","email":"info@garcia.com","phone":"555-1234"}},
	{"action":"create","table":"appliances","data":{"name":"Dishwasher","brand":"Bosch","model_number":"SHP65"}},
	{"action":"update","table":"documents","data":{"title":"Invoice #42"}}
]`)

func newMultiTableOpsTreeModel(t *testing.T) *Model {
	t.Helper()

	m := newTestModelWithStore(t)

	doc := &data.Document{
		Title:         "Multi-Table Invoice",
		FileName:      "invoice.pdf",
		MIMEType:      "application/pdf",
		ExtractionOps: testMultiTableOpsJSON,
	}
	require.NoError(t, m.store.CreateDocument(doc))

	m.active = tabIndex(tabDocuments)
	require.NoError(t, m.reloadTab(m.effectiveTab()))

	return m
}

func TestOpsTreeTabSwitchBF(t *testing.T) {
	t.Parallel()
	m := newMultiTableOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)
	// vendors + appliances + documents = 3 groups.
	require.Len(t, m.opsTree.previewGroups, 3)

	assert.Equal(t, 0, m.opsTree.previewTab)

	sendKey(m, "f")
	assert.Equal(t, 1, m.opsTree.previewTab)

	sendKey(m, "f")
	assert.Equal(t, 2, m.opsTree.previewTab)

	// f at last should clamp (no wrap).
	sendKey(m, "f")
	assert.Equal(t, 2, m.opsTree.previewTab)

	sendKey(m, "b")
	assert.Equal(t, 1, m.opsTree.previewTab)

	sendKey(m, "b")
	assert.Equal(t, 0, m.opsTree.previewTab)

	// b at 0 should clamp (no wrap).
	sendKey(m, "b")
	assert.Equal(t, 0, m.opsTree.previewTab)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -shuffle=on -run TestOpsTreeTabSwitchBF ./internal/app/`
Expected: FAIL — `b`/`f` keys not handled in ops tree.

- [ ] **Step 3: Add b/f key handling**

In `handleOpsTreeKey`, add cases before the closing `}`:

```go
	case keyB:
		if len(tree.previewGroups) > 1 && tree.previewTab > 0 {
			tree.previewTab--
		}
	case keyF:
		if len(tree.previewGroups) > 1 && tree.previewTab < len(tree.previewGroups)-1 {
			tree.previewTab++
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -shuffle=on -run TestOpsTreeTabSwitchBF ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

Use `/commit`.

---

### Task 4: Add mouse click handling for tab bar

**Files:**
- Modify: `internal/app/mouse.go:268-281` (ops tree click section)

- [ ] **Step 1: Write failing test — clicking ops tab switches previewTab**

Add to `internal/app/ops_tree_test.go`:

```go
func TestOpsTreeMouseClickTab(t *testing.T) {
	t.Parallel()
	m := newMultiTableOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)
	require.GreaterOrEqual(t, len(m.opsTree.previewGroups), 2)

	// Render to populate zones.
	m.View()

	// Click on second tab.
	z := m.zones.Get(fmt.Sprintf("%s%d", zoneOpsTab, 1))
	if z == nil || z.IsZero() {
		t.Skip("ops tab zone not rendered (terminal too small)")
	}
	sendClick(m, z.StartX, z.StartY)

	assert.Equal(t, 1, m.opsTree.previewTab)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -shuffle=on -run TestOpsTreeMouseClickTab ./internal/app/`
Expected: FAIL — no mouse dispatch for `ops-tab-N` zones.

- [ ] **Step 3: Add mouse dispatch for ops tab clicks**

In `internal/app/mouse.go`, inside the `if tree := m.opsTree; tree != nil`
block (after the existing node loop at line 281), add tab click dispatch:

```go
		// Ops tree tab clicks: switch preview tab.
		for i := range tree.previewGroups {
			if m.zones.Get(fmt.Sprintf("%s%d", zoneOpsTab, i)).InBounds(msg) {
				tree.previewTab = i
				return m, nil
			}
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -shuffle=on -run TestOpsTreeMouseClickTab ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

Use `/commit`.

---

### Task 5: Full test suite verification

- [ ] **Step 1: Run all ops tree tests**

Run: `go test -shuffle=on -run TestOpsTree ./internal/app/`
Expected: All pass (existing + new).

- [ ] **Step 2: Run full test suite**

Run: `go test -shuffle=on ./internal/...`
Expected: All pass.

- [ ] **Step 3: Run linter**

Run: `golangci-lint run ./internal/...`
Expected: No warnings.

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: Clean build.

- [ ] **Step 5: Commit any fixups, then clean up plan file**

Delete `plan-issue-775.md` if it exists (working artifact). Keep
`plans/ops-tree-table-preview.md` (design record) and
`plans/ops-tree-table-preview-impl.md` (implementation record).
