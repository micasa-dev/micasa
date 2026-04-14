<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Issue #892: Explore Mode Jumpy Toggle -- Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix two visual glitches when toggling explore mode in the extraction
preview overlay: (1) the `x` key jumping position in the hint bar, and
(2) the overlay height changing due to varying preview tab row counts.

**Architecture:** Two isolated fixes in `internal/app/extraction_render.go`.
Fix 1 reorders hints so `a`/`x`/`esc` are always the trailing three items.
Fix 2 computes preview line reservation from the tallest tab across all
groups and uses `ex.previewTab` in pipeline mode too.

**Tech Stack:** Go, Bubble Tea, lipgloss, testify

---

## File Map

- **Modify:** `internal/app/extraction_render.go` -- hint bar reorder in
  `buildExtractionPipelineOverlay`, preview height stabilization (new
  `stablePreviewLines` helper), and tab selection in
  `renderOperationPreviewSection`
- **Modify:** `internal/app/extraction_test.go` -- new tests for hint bar
  trailing order, overlay height stability, and tab persistence across
  toggle

---

### Task 1: Test hint bar trailing hint order stability

**Files:**
- Modify: `internal/app/extraction_test.go`

Note: tests need `"strings"` added to the import block.

- [ ] **Step 1: Write failing test -- a/x/esc trailing order in both modes**

Add this test after the existing `TestExploreMode_XTogglesExploring`:

```go
func TestExploreMode_HintBarTrailingOrder(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "A"}},
	})
	ex := m.ex.extraction

	// Pipeline mode: x should come after a and before esc.
	pipelineOut := m.buildExtractionPipelineOverlay(100, 90, "test")
	pipelinePlain := ansi.Strip(pipelineOut)

	pipelineAcceptIdx := strings.Index(pipelinePlain, "a accept")
	pipelineExploreIdx := strings.Index(pipelinePlain, "x explore")
	pipelineEscIdx := strings.LastIndex(pipelinePlain, "esc discard")
	require.Greater(t, pipelineAcceptIdx, 0, "a accept should appear")
	require.Greater(t, pipelineExploreIdx, 0, "x explore should appear")
	require.Greater(t, pipelineEscIdx, 0, "esc discard should appear")
	assert.Less(t, pipelineAcceptIdx, pipelineExploreIdx,
		"a accept should come before x explore")
	assert.Less(t, pipelineExploreIdx, pipelineEscIdx,
		"x explore should come before esc discard")

	// Enter explore mode: x should still come after a and before esc.
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)

	exploreOut := m.buildExtractionPipelineOverlay(100, 90, "test")
	explorePlain := ansi.Strip(exploreOut)

	exploreAcceptIdx := strings.Index(explorePlain, "a accept")
	exploreBackIdx := strings.Index(explorePlain, "x back")
	exploreEscIdx := strings.LastIndex(explorePlain, "esc discard")
	require.Greater(t, exploreAcceptIdx, 0, "a accept should appear")
	require.Greater(t, exploreBackIdx, 0, "x back should appear")
	require.Greater(t, exploreEscIdx, 0, "esc discard should appear")
	assert.Less(t, exploreAcceptIdx, exploreBackIdx,
		"a accept should come before x back")
	assert.Less(t, exploreBackIdx, exploreEscIdx,
		"x back should come before esc discard")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestExploreMode_HintBarTrailingOrder -count=1 ./internal/app/`

Expected: FAIL -- in pipeline mode `x explore` currently appears before
`a accept` (x is added at line 213 before the Done block adds a/esc).

- [ ] **Step 3: Write failing test -- multi-tab explore same trailing order**

```go
func TestExploreMode_HintBarTrailingOrderMultiTab(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "A"}},
		{Action: "create", Table: data.TableQuotes, Data: map[string]any{"total_cents": float64(100)}},
	})
	ex := m.ex.extraction

	// Explore mode with multiple tabs: b/f tabs hint appears but
	// a/x/esc must still be the trailing three.
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)
	require.Greater(t, len(ex.previewGroups), 1, "need multiple tabs")

	out := m.buildExtractionPipelineOverlay(100, 90, "test")
	plain := ansi.Strip(out)

	tabsIdx := strings.Index(plain, "b/f tabs")
	acceptIdx := strings.Index(plain, "a accept")
	backIdx := strings.Index(plain, "x back")
	escIdx := strings.LastIndex(plain, "esc discard")
	require.Greater(t, tabsIdx, 0, "b/f tabs should appear")
	require.Greater(t, acceptIdx, 0)
	require.Greater(t, backIdx, 0)
	require.Greater(t, escIdx, 0)

	assert.Less(t, tabsIdx, acceptIdx,
		"b/f tabs should come before the stable trailing group")
	assert.Less(t, acceptIdx, backIdx,
		"a accept should come before x back")
	assert.Less(t, backIdx, escIdx,
		"x back should come before esc discard")
}
```

- [ ] **Step 4: Run test to verify it passes (regression guard)**

Run: `go test -run TestExploreMode_HintBarTrailingOrderMultiTab -count=1 ./internal/app/`

Expected: PASS -- explore mode already has correct a/x/esc trailing
order. This test guards against regressions after the pipeline-mode fix.

- [ ] **Step 5: Commit failing tests**

```
test(extraction): add hint bar trailing order tests

Verify a/x/esc always appear as the trailing three hints in both
pipeline and explore modes, including multi-tab. Fails due to #892.
```

---

### Task 2: Fix hint bar ordering

**Files:**
- Modify: `internal/app/extraction_render.go`

- [ ] **Step 1: Reorder hints so a/x/esc are always trailing**

Replace the hint-building block in `buildExtractionPipelineOverlay`
(the `else` branch for pipeline mode, starting at `} else {` after
`} else if ex.exploring {`):

```go
	// Hint line varies by mode.
	var hints []string
	if ex.modelPicker != nil {
		hints = append(hints,
			m.helpItem(symUp+"/"+symDown, "navigate"),
			m.helpItem(symReturn, "select"),
			m.helpItem(keyEsc, "cancel"),
		)
	} else if ex.exploring {
		hints = append(hints, m.helpItem(keyJ+"/"+keyK, "rows"), m.helpItem(keyH+"/"+keyL, "cols"))
		if len(ex.previewGroups) > 1 {
			hints = append(hints, m.helpItem(keyB+"/"+keyF, "tabs"))
		}
		hints = append(hints, m.helpItem(keyA, "accept"), m.helpItem(keyX, "back"), m.helpItem(keyEsc, "discard"))
	} else {
		hints = append(hints, m.helpItem(keyJ+"/"+keyK, "navigate"))
		cursorStatus := ex.Steps[ex.cursorStep()].Status
		if cursorStatus != stepPending {
			hints = append(hints, m.helpItem(symReturn, "expand"))
		}
		if ex.Done {
			if ex.hasLLM {
				label := "layout on"
				if m.ex.ocrTSV {
					label = "layout off"
				}
				hints = append(hints, m.helpItem(keyT, label))
			}
			if hasOps {
				hints = append(hints, m.helpItem(keyA, "accept"), m.helpItem(keyX, "explore"), m.helpItem(keyEsc, "discard"))
			} else {
				hints = append(hints, m.helpItem(keyA, "accept"), m.helpItem(keyEsc, "discard"))
			}
		} else {
			hints = append(hints,
				m.helpItem(symCtrlC, "int"),
				m.helpItem(symCtrlB, "bg"),
				m.helpItem(keyEsc, "cancel"),
			)
		}
	}
```

Key changes from before:
- Pipeline done+ops: `a accept . x explore . esc discard` as a trailing
  triple (previously `x explore` was inserted before `t layout`, then
  `a accept` and `esc discard` came later separately).
- Pipeline done without ops: `a accept . esc discard` (no x, same as
  before but combined into the Done block).
- Explore mode: unchanged (already had `a . x . esc` trailing).

- [ ] **Step 2: Run trailing order tests**

Run: `go test -run 'TestExploreMode_HintBarTrailingOrder' -count=1 ./internal/app/`

Expected: PASS -- both single-tab and multi-tab tests pass.

- [ ] **Step 3: Run all extraction tests**

Run: `go test -run 'TestExtraction|TestExploreMode|TestPipelineMode|TestRenderOperation' -count=1 ./internal/app/`

Expected: PASS -- no regressions.

- [ ] **Step 4: Commit**

```
fix(extraction): stabilize x key position in hint bar

Reorder pipeline-mode hints so a/x/esc are always the trailing three
items, matching explore mode. Mode-specific hints (enter/expand,
t/layout) come before the stable tail. This prevents the x key from
visually jumping when toggling explore mode.
```

---

### Task 3: Test overlay height stability across tabs

**Files:**
- Modify: `internal/app/extraction_test.go`

- [ ] **Step 1: Write failing test -- overlay height stable across toggle**

```go
func TestExploreMode_OverlayHeightStableAcrossToggle(t *testing.T) {
	t.Parallel()
	// Two tables with different row counts to trigger height change.
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "A"}},
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "B"}},
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "C"}},
		{Action: "create", Table: data.TableQuotes, Data: map[string]any{"total_cents": float64(100)}},
	})
	ex := m.ex.extraction

	// Pipeline mode overlay.
	pipelineOut := m.buildExtractionPipelineOverlay(100, 90, "test")
	pipelineLines := strings.Count(pipelineOut, "\n")

	// Enter explore mode and switch to tab with fewer rows.
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)
	sendExtractionKey(m, "f") // switch to quotes tab (1 row vs 3)
	require.Equal(t, 1, ex.previewTab)

	exploreOut := m.buildExtractionPipelineOverlay(100, 90, "test")
	exploreLines := strings.Count(exploreOut, "\n")

	assert.Equal(t, pipelineLines, exploreLines,
		"overlay height should be identical regardless of active tab row count")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestExploreMode_OverlayHeightStableAcrossToggle -count=1 ./internal/app/`

Expected: FAIL -- tab 0 has 3 vendor rows, tab 1 has 1 quote row,
so `previewLines` changes and overlay height differs.

- [ ] **Step 3: Commit failing test**

```
test(extraction): add overlay height stability test

Verifies overlay line count stays constant when toggling explore
mode between tabs with different row counts. Fails due to #892.
```

---

### Task 4: Test tab persistence across explore toggle

**Files:**
- Modify: `internal/app/extraction_test.go`

- [ ] **Step 1: Write failing test -- tab persists when exiting explore**

```go
func TestExploreMode_TabPersistsOnExit(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "A"}},
		{Action: "create", Table: data.TableQuotes, Data: map[string]any{"total_cents": float64(100)}},
	})
	ex := m.ex.extraction

	// Enter explore, switch to tab 1, exit.
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)
	sendExtractionKey(m, "f")
	require.Equal(t, 1, ex.previewTab)
	sendExtractionKey(m, "x") // exit explore
	require.False(t, ex.exploring)

	// Pipeline mode should now show tab 1 (dimmed), not reset to 0.
	out := m.renderOperationPreviewSection(80, false)
	plain := ansi.Strip(out)
	// Tab 1 is Quotes -- its data should appear in the rendered table.
	assert.Contains(t, plain, "Total",
		"pipeline mode should show the last-explored tab's content")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestExploreMode_TabPersistsOnExit -count=1 ./internal/app/`

Expected: FAIL -- pipeline mode currently hardcodes `tabIdx = 0`,
so it always shows vendors tab regardless of which tab was explored.

- [ ] **Step 3: Commit failing test**

```
test(extraction): add tab persistence test for explore toggle

Verifies pipeline mode shows the same tab that was active during
explore, not always tab 0. Fails due to #892.
```

---

### Task 5: Fix overlay height -- stable preview line reservation

**Files:**
- Modify: `internal/app/extraction_render.go`

- [ ] **Step 1: Replace dynamic previewLines with stable max-across-tabs**

In `buildExtractionPipelineOverlay`, replace the `previewLines`
computation inside the `if hasOps` block:

```go
	// Determine available height for the viewport, reserving space for the
	// operation preview section when operations are available.
	hasOps := ex.Done && len(ex.operations) > 0
	previewSection := ""
	previewLines := 0
	if hasOps {
		previewSection = m.renderOperationPreviewSection(innerW, ex.exploring)
		previewLines = strings.Count(previewSection, "\n") + 2 // +2 for separator + blank
	}
```

With:

```go
	// Determine available height for the viewport, reserving space for the
	// operation preview section when operations are available.
	hasOps := ex.Done && len(ex.operations) > 0
	previewSection := ""
	previewLines := 0
	if hasOps {
		previewSection = m.renderOperationPreviewSection(innerW, ex.exploring)
		// Reserve height for the tallest preview tab so the overlay
		// doesn't jump when switching tabs in explore mode.
		previewLines = stablePreviewLines(ex.previewGroups)
	}
```

- [ ] **Step 2: Add `stablePreviewLines` helper**

Add below `previewNaturalWidth` (around line 56):

```go
// stablePreviewLines returns a stable line reservation for the preview
// section based on the tallest tab's row count. This prevents the overlay
// height from changing when switching between tabs with different row counts.
func stablePreviewLines(groups []previewTableGroup) int {
	if len(groups) == 0 {
		return 0
	}
	maxRows := 0
	for _, g := range groups {
		if len(g.cells) > maxRows {
			maxRows = len(g.cells)
		}
	}
	// The preview section string contains maxRows+3 newlines:
	//   tabBar \n underline \n header \n divider \n row0 \n ... \n rowN-1
	// The caller's +2 accounts for the separator + blank above the section.
	return maxRows + 3 + 2
}
```

The breakdown: the rendered preview string has `maxRows+3` newlines
(3 separators between tab-bar/underline/header/divider, plus
`maxRows-1` between data rows, plus 1 between divider and first row =
`maxRows+3`). Then `+2` for the separator rule and blank line above
the preview section, matching the original code's `+2`.

- [ ] **Step 3: Run height stability test**

Run: `go test -run TestExploreMode_OverlayHeightStableAcrossToggle -count=1 ./internal/app/`

Expected: PASS.

- [ ] **Step 4: Commit**

```
fix(extraction): stabilize overlay height across preview tabs

Compute preview line reservation from the tallest tab's row count
instead of counting newlines in the currently rendered tab. This
prevents the overlay from jumping when toggling explore mode or
switching between tabs with different row counts.
```

---

### Task 6: Fix tab persistence -- show `previewTab` in pipeline mode

**Files:**
- Modify: `internal/app/extraction_render.go`

- [ ] **Step 1: Remove tab-0 reset in pipeline mode**

In `renderOperationPreviewSection`, replace the `tabIdx` selection
block:

```go
	// Always render a single tab: the active one in explore mode,
	// the first one in pipeline mode.
	tabIdx := 0
	if interactive {
		tabIdx = ex.previewTab
	}
	if tabIdx >= len(groups) {
		tabIdx = 0
	}
```

With:

```go
	// Always render the active preview tab. In explore mode it follows
	// user navigation; in pipeline mode it preserves the last-explored
	// tab so the view doesn't reset on toggle.
	tabIdx := ex.previewTab
	if tabIdx >= len(groups) {
		tabIdx = 0
	}
```

- [ ] **Step 2: Run tab persistence test**

Run: `go test -run TestExploreMode_TabPersistsOnExit -count=1 ./internal/app/`

Expected: PASS.

- [ ] **Step 3: Run all extraction tests**

Run: `go test -run 'TestExtraction|TestExploreMode|TestPipelineMode|TestRenderOperation' -count=1 ./internal/app/`

Expected: PASS -- no regressions.

- [ ] **Step 4: Commit**

```
fix(extraction): show last-explored tab in pipeline mode

Use ex.previewTab in both modes instead of resetting to tab 0 in
pipeline mode. The user sees the same tab they were exploring,
just dimmed, maintaining visual continuity across toggle.

closes #892
```

---

### Task 7: Final verification

- [ ] **Step 1: Run full test suite**

Run: `go test -shuffle=on ./...`

Expected: PASS, zero failures.

- [ ] **Step 2: Run linter**

Run: `golangci-lint run ./...`

Expected: Clean, no warnings.

- [ ] **Step 3: Verify build compiles**

Run: `go build ./...`

Expected: Clean, no errors.
