<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Bubbles Help + Key Binding Refactor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace string-based key dispatch with `key.Binding`/`key.Matches()`, build a two-pane navigable help overlay, and trial `help.ShortHelpView()` for the status bar.

**Architecture:** Three independent commits. Phase 1 creates `AppKeyMap` and migrates all 24 dispatch sites mechanically. Phase 2 replaces the help overlay with a two-pane layout. Phase 3 trials `help.ShortHelpView()` for the status bar (revertible).

**Tech Stack:** `charm.land/bubbles/v2/key`, `charm.land/bubbles/v2/help`, `charm.land/bubbles/v2/viewport`

**Spec:** `plans/bubbles-help-key-refactor.md`

---

## Background for the implementer

### Current key dispatch pattern

All key handling uses string comparison via `.String()`:

```go
// switch form (17 sites)
switch key.String() {
case keyJ, keyDown:
    m.dashDown()
case keyK, keyUp:
    m.dashUp()
}

// inline if-guard form (7 sites)
if typed.String() == keyCtrlQ { ... }
if key.String() != keyEnter { ... }
```

### Target pattern

```go
// switch form
switch {
case key.Matches(msg, m.keys.DashDown):
    m.dashDown()
case key.Matches(msg, m.keys.DashUp):
    m.dashUp()
}

// inline if-guard form
if key.Matches(typed, m.keys.Quit) { ... }
if !key.Matches(key, m.keys.DashJump) { ... }
```

### Key files

| File | Role | Dispatch sites |
|------|------|---------------|
| `internal/app/model.go` | Key constants, Model struct, overlay types, helpOverlayKey, handleInlineInputKey | 2 switches |
| `internal/app/model_keys.go` | handleDashboardKeys, handleCommonKeys, handleNormalKeys, handleEditKeys, handleCalendarKey | 5 switches + 1 guard |
| `internal/app/model_update.go` | Global ctrl+q/ctrl+c, form key intercepts (ctrl+s, ctrl+e, shift+h, esc) | 2 global + 4 form guards |
| `internal/app/chat.go` | handleChatKey (completer + main) | 2 switches |
| `internal/app/model_status.go` | handleConfirmDiscard, handleConfirmHardDelete | 2 switches |
| `internal/app/search.go` | handleDocSearchKey | 1 switch |
| `internal/app/column_finder.go` | handleColumnFinderKey | 1 switch |
| `internal/app/ops_tree.go` | handleOpsTreeKey | 1 switch |
| `internal/app/extraction.go` | handleExtractionPipelineKey, handleExtractionModelPickerKey, handleExtractionExploreKey | 3 switches |
| `internal/app/view_help.go` | Custom help renderer (Phase 2 replaces) | — |
| `internal/app/view.go` | Status bar hints (Phase 3 replaces) | — |

### Test infrastructure

Tests use `sendKey(m, "j")` which calls `m.Update(keyPress("j"))`. The `keyPress` helper in `mode_test.go:55-102` converts strings to `tea.KeyPressMsg`. This infrastructure does NOT change — `key.Matches()` works on the same `tea.KeyPressMsg` type.

### What NOT to change

- Key constants in `model.go` (lines 31-142) — stay as private helpers for `WithKeys()`
- Display symbols (`symReturn`, `symUp`, etc.) — stay for help text
- `helpItem()`/`renderKeys()`/`renderKeysLight()` in `view.go` — stay for overlay-local hint bars
- Table KeyMap handling (`normalTableKeyMap`/`editTableKeyMap`/`setAllTableKeyMaps` in `tables.go`) — stays as-is
- The `keyPress` / `sendKey` test helpers — stay as-is

---

## Phase 1: key.Binding Migration

### Task 1: Verify existing test baseline

**Files:** None (read-only)

- [ ] **Step 1: Run all tests to establish green baseline**

Run: `go test -shuffle=on -count=1 ./internal/app/...`
Expected: All tests pass. Record the count. This is the baseline that must remain green throughout Phase 1.

- [ ] **Step 2: Verify key dispatch test coverage**

Spot-check that these handlers have test coverage by confirming these tests exist and pass:

- `mode_test.go`: `TestHelpToggle`, `TestHelpViewportScrolling`, `TestEnterEditMode`, `TestColumnNavH`, `TestHouseToggle`, `TestNextTabAdvances`
- `dashboard_test.go`: `TestDashboardNavigation`, `TestDashboardBlocksTableKeys`, `TestDashboardSectionNavWithShiftJK`
- `calendar_test.go`: `TestCalendarKeyNavigation`
- `ops_tree_test.go`: `TestOpsTreeNavigateJK`, `TestOpsTreeExpandCollapse`
- `search_test.go`: doc search navigation tests
- `extraction_test.go`: pipeline key tests

Run: `go test -shuffle=on -count=1 -run 'TestHelp|TestEnterEdit|TestColumnNav|TestHouseToggle|TestNextTab|TestDashboard|TestCalendar|TestOpsTree' ./internal/app/...`
Expected: All pass.

---

### Task 2: Create keybindings.go with AppKeyMap

**Files:**
- Create: `internal/app/keybindings.go`

- [ ] **Step 1: Create the file with the complete AppKeyMap struct and constructor**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import "charm.land/bubbles/v2/key"

// AppKeyMap defines all keybindings as structured key.Binding values.
// Each binding carries the actual keys for dispatch (via key.Matches)
// and display text for the help overlay (via key.Binding.Help).
//
// Bindings are grouped by the dispatch handler that uses them.
// Some bindings are shared across handlers (noted in comments).
// Some bindings are help-display-only (no WithKeys; never matched).
// Table-delegated bindings (j/k row nav, g/G, d/u half-page) are
// NOT here — they live in the bubbles table.KeyMap.
type AppKeyMap struct {
	// --- Global (pre-overlay, model_update.go) ---
	Quit   key.Binding
	Cancel key.Binding

	// --- Common (handleCommonKeys — both normal + edit) ---
	ColLeft     key.Binding
	ColRight    key.Binding
	ColStart    key.Binding
	ColEnd      key.Binding
	Help        key.Binding
	HouseToggle key.Binding
	MagToggle   key.Binding // also used in handleChatKey
	FgExtract   key.Binding

	// --- Normal mode (handleNormalKeys) ---
	TabNext       key.Binding
	TabPrev       key.Binding
	TabFirst      key.Binding
	TabLast       key.Binding
	EnterEditMode key.Binding
	Enter         key.Binding
	Dashboard     key.Binding
	Sort          key.Binding
	SortClear     key.Binding
	ToggleSettled key.Binding
	FilterPin     key.Binding
	FilterToggle  key.Binding
	FilterClear   key.Binding
	FilterNegate  key.Binding
	ColHide       key.Binding
	ColShowAll    key.Binding
	ColFinder     key.Binding
	DocSearch     key.Binding
	DocOpen       key.Binding // also used in handleEditKeys
	ToggleUnits   key.Binding
	Chat          key.Binding
	Escape        key.Binding

	// --- Edit mode (handleEditKeys) ---
	Add         key.Binding
	QuickAdd    key.Binding
	EditCell    key.Binding
	EditFull    key.Binding
	Delete      key.Binding
	HardDelete  key.Binding
	ReExtract   key.Binding
	ShowDeleted key.Binding
	HouseEdit   key.Binding
	ExitEdit    key.Binding

	// --- Forms (model_update.go:updateForm if-guards) ---
	FormSave        key.Binding
	FormCancel      key.Binding
	FormNextField   key.Binding // help-only; huh dispatches internally
	FormPrevField   key.Binding // help-only; huh dispatches internally
	FormEditor      key.Binding
	FormHiddenFiles key.Binding

	// --- Chat (handleChatKey main) ---
	ChatSend      key.Binding
	ChatToggleSQL key.Binding
	ChatHistoryUp key.Binding
	ChatHistoryDn key.Binding
	ChatHide      key.Binding

	// --- Chat completer (handleChatKey completer) ---
	CompleterUp      key.Binding
	CompleterDown    key.Binding
	CompleterConfirm key.Binding
	CompleterCancel  key.Binding

	// --- Calendar (handleCalendarKey) ---
	CalLeft      key.Binding
	CalRight     key.Binding
	CalUp        key.Binding
	CalDown      key.Binding
	CalPrevMonth key.Binding
	CalNextMonth key.Binding
	CalPrevYear  key.Binding
	CalNextYear  key.Binding
	CalToday     key.Binding
	CalConfirm   key.Binding
	CalCancel    key.Binding

	// --- Dashboard (handleDashboardKeys) ---
	DashUp          key.Binding
	DashDown        key.Binding
	DashNextSection key.Binding
	DashPrevSection key.Binding
	DashTop         key.Binding
	DashBottom      key.Binding
	DashToggle      key.Binding
	DashToggleAll   key.Binding
	DashJump        key.Binding

	// --- Doc search (handleDocSearchKey) ---
	DocSearchUp      key.Binding
	DocSearchDown    key.Binding
	DocSearchConfirm key.Binding
	DocSearchCancel  key.Binding

	// --- Column finder (handleColumnFinderKey) ---
	ColFinderUp        key.Binding
	ColFinderDown      key.Binding
	ColFinderConfirm   key.Binding
	ColFinderCancel    key.Binding
	ColFinderClear     key.Binding
	ColFinderBackspace key.Binding

	// --- Ops tree (handleOpsTreeKey) ---
	OpsUp       key.Binding
	OpsDown     key.Binding
	OpsExpand   key.Binding
	OpsCollapse key.Binding
	OpsTabNext  key.Binding
	OpsTabPrev  key.Binding
	OpsTop      key.Binding
	OpsBottom   key.Binding
	OpsClose    key.Binding

	// --- Extraction pipeline (handleExtractionPipelineKey) ---
	ExtCancel     key.Binding
	ExtInterrupt  key.Binding
	ExtUp         key.Binding
	ExtDown       key.Binding
	ExtToggle     key.Binding
	ExtRemodel    key.Binding
	ExtToggleTSV  key.Binding
	ExtAccept     key.Binding
	ExtExplore    key.Binding
	ExtBackground key.Binding

	// --- Extraction explore (handleExtractionExploreKey) ---
	ExploreUp       key.Binding
	ExploreDown     key.Binding
	ExploreLeft     key.Binding
	ExploreRight    key.Binding
	ExploreColStart key.Binding
	ExploreColEnd   key.Binding
	ExploreTop      key.Binding
	ExploreBottom   key.Binding
	ExploreTabNext  key.Binding
	ExploreTabPrev  key.Binding
	ExploreAccept   key.Binding
	ExploreExit     key.Binding

	// --- Extraction model picker (handleExtractionModelPickerKey) ---
	ExtModelUp        key.Binding
	ExtModelDown      key.Binding
	ExtModelConfirm   key.Binding
	ExtModelCancel    key.Binding
	ExtModelBackspace key.Binding

	// --- Help overlay (helpOverlayKey; Phase 2 adds two-pane) ---
	HelpSectionUp   key.Binding
	HelpSectionDown key.Binding
	HelpClose       key.Binding

	// --- Confirmations (handleConfirmDiscard, handleConfirmHardDelete) ---
	ConfirmYes key.Binding
	ConfirmNo  key.Binding

	// --- Inline input (handleInlineInputKey) ---
	InlineConfirm key.Binding
	InlineCancel  key.Binding
}

func newAppKeyMap() AppKeyMap {
	return AppKeyMap{
		// Global
		Quit:   key.NewBinding(key.WithKeys(keyCtrlQ), key.WithHelp("ctrl+q", "quit")),
		Cancel: key.NewBinding(key.WithKeys(keyCtrlC), key.WithHelp("ctrl+c", "cancel LLM operation")),

		// Common
		ColLeft:     key.NewBinding(key.WithKeys(keyH, keyLeft), key.WithHelp(keyH+"/"+keyL+"/"+symLeft+"/"+symRight, "columns")),
		ColRight:    key.NewBinding(key.WithKeys(keyL, keyRight)),
		ColStart:    key.NewBinding(key.WithKeys(keyCaret), key.WithHelp(keyCaret+"/"+keyDollar, "first/last column")),
		ColEnd:      key.NewBinding(key.WithKeys(keyDollar)),
		Help:        key.NewBinding(key.WithKeys(keyQuestion), key.WithHelp(keyQuestion, "help")),
		HouseToggle: key.NewBinding(key.WithKeys(keyTab), key.WithHelp("tab", "house profile")),
		MagToggle:   key.NewBinding(key.WithKeys(keyCtrlO)),
		FgExtract:   key.NewBinding(key.WithKeys(keyCtrlB)),

		// Normal mode
		TabNext:       key.NewBinding(key.WithKeys(keyF), key.WithHelp(keyB+"/"+keyF, "switch tabs")),
		TabPrev:       key.NewBinding(key.WithKeys(keyB)),
		TabFirst:      key.NewBinding(key.WithKeys(keyShiftB), key.WithHelp(keyShiftB+"/"+keyShiftF, "first/last tab")),
		TabLast:       key.NewBinding(key.WithKeys(keyShiftF)),
		EnterEditMode: key.NewBinding(key.WithKeys(keyI), key.WithHelp(keyI, "edit mode")),
		Enter:         key.NewBinding(key.WithKeys(keyEnter), key.WithHelp(symReturn, "drill / follow / preview")),
		Dashboard:     key.NewBinding(key.WithKeys(keyShiftD), key.WithHelp(keyShiftD, "summary")),
		Sort:          key.NewBinding(key.WithKeys(keyS), key.WithHelp(keyS+"/"+keyShiftS, "sort / clear sorts")),
		SortClear:     key.NewBinding(key.WithKeys(keyShiftS)),
		ToggleSettled: key.NewBinding(key.WithKeys(keyT), key.WithHelp(keyT, "toggle settled projects")),
		FilterPin:     key.NewBinding(key.WithKeys(keyN), key.WithHelp(keyN, "pin/unpin")),
		FilterToggle:  key.NewBinding(key.WithKeys(keyShiftN), key.WithHelp(keyShiftN, "toggle filter")),
		FilterClear:   key.NewBinding(key.WithKeys(keyCtrlN), key.WithHelp("ctrl+n", "clear pins and filter")),
		FilterNegate:  key.NewBinding(key.WithKeys(keyBang), key.WithHelp(keyBang, "invert filter")),
		ColHide:       key.NewBinding(key.WithKeys(keyC), key.WithHelp(keyC+"/"+keyShiftC, "toggle column visibility")),
		ColShowAll:    key.NewBinding(key.WithKeys(keyShiftC)),
		ColFinder:     key.NewBinding(key.WithKeys(keySlash), key.WithHelp(keySlash, "find column")),
		DocSearch:     key.NewBinding(key.WithKeys(keyCtrlF), key.WithHelp("ctrl+f", "search documents")),
		DocOpen:       key.NewBinding(key.WithKeys(keyO), key.WithHelp(keyO, "open document")),
		ToggleUnits:   key.NewBinding(key.WithKeys(keyShiftU), key.WithHelp(keyShiftU, "toggle units")),
		Chat:          key.NewBinding(key.WithKeys(keyAt), key.WithHelp(keyAt, "ask LLM")),
		Escape:        key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "close detail / clear status")),

		// Edit mode
		Add:         key.NewBinding(key.WithKeys(keyA), key.WithHelp(keyA, "add entry")),
		QuickAdd:    key.NewBinding(key.WithKeys(keyShiftA), key.WithHelp(keyShiftA, "add document with extraction")),
		EditCell:    key.NewBinding(key.WithKeys(keyE), key.WithHelp(keyE, "edit cell or row")),
		EditFull:    key.NewBinding(key.WithKeys(keyShiftE), key.WithHelp(keyShiftE, "edit row (full form)")),
		Delete:      key.NewBinding(key.WithKeys(keyD), key.WithHelp(keyD, "delete / restore")),
		HardDelete:  key.NewBinding(key.WithKeys(keyShiftD), key.WithHelp(keyShiftD, "permanently delete")),
		ReExtract:   key.NewBinding(key.WithKeys(keyR), key.WithHelp(keyR, "re-extract")),
		ShowDeleted: key.NewBinding(key.WithKeys(keyX), key.WithHelp(keyX, "show deleted")),
		HouseEdit:   key.NewBinding(key.WithKeys(keyP), key.WithHelp(keyP, "house profile")),
		ExitEdit:    key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "nav mode")),

		// Forms
		FormSave:        key.NewBinding(key.WithKeys(keyCtrlS), key.WithHelp("ctrl+s", "save")),
		FormCancel:      key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "cancel")),
		FormNextField:   key.NewBinding(key.WithHelp("tab", "next field")),
		FormPrevField:   key.NewBinding(key.WithHelp("shift+tab", "previous field")),
		FormEditor:      key.NewBinding(key.WithKeys(keyCtrlE), key.WithHelp("ctrl+e", "open notes in $EDITOR")),
		FormHiddenFiles: key.NewBinding(key.WithKeys(keyShiftH), key.WithHelp(keyShiftH, "toggle hidden files")),

		// Chat
		ChatSend:      key.NewBinding(key.WithKeys(keyEnter), key.WithHelp(symReturn, "send message")),
		ChatToggleSQL: key.NewBinding(key.WithKeys(keyCtrlS), key.WithHelp("ctrl+s", "toggle SQL display")),
		ChatHistoryUp: key.NewBinding(key.WithKeys(keyUp, keyCtrlP), key.WithHelp(symUp+"/"+symDown, "prompt history")),
		ChatHistoryDn: key.NewBinding(key.WithKeys(keyDown, keyCtrlN)),
		ChatHide:      key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "hide chat")),

		// Chat completer
		CompleterUp:      key.NewBinding(key.WithKeys(keyUp, keyCtrlP)),
		CompleterDown:    key.NewBinding(key.WithKeys(keyDown, keyCtrlN)),
		CompleterConfirm: key.NewBinding(key.WithKeys(keyEnter)),
		CompleterCancel:  key.NewBinding(key.WithKeys(keyEsc)),

		// Calendar
		CalLeft:      key.NewBinding(key.WithKeys(keyH, keyLeft)),
		CalRight:     key.NewBinding(key.WithKeys(keyL, keyRight)),
		CalUp:        key.NewBinding(key.WithKeys(keyK, keyUp)),
		CalDown:      key.NewBinding(key.WithKeys(keyJ, keyDown)),
		CalPrevMonth: key.NewBinding(key.WithKeys(keyShiftH)),
		CalNextMonth: key.NewBinding(key.WithKeys(keyShiftL)),
		CalPrevYear:  key.NewBinding(key.WithKeys(keyLBracket)),
		CalNextYear:  key.NewBinding(key.WithKeys(keyRBracket)),
		CalToday:     key.NewBinding(key.WithKeys(keyT)),
		CalConfirm:   key.NewBinding(key.WithKeys(keyEnter)),
		CalCancel:    key.NewBinding(key.WithKeys(keyEsc)),

		// Dashboard
		DashUp:          key.NewBinding(key.WithKeys(keyK, keyUp)),
		DashDown:        key.NewBinding(key.WithKeys(keyJ, keyDown)),
		DashNextSection: key.NewBinding(key.WithKeys(keyShiftJ, keyShiftDown)),
		DashPrevSection: key.NewBinding(key.WithKeys(keyShiftK, keyShiftUp)),
		DashTop:         key.NewBinding(key.WithKeys(keyG)),
		DashBottom:      key.NewBinding(key.WithKeys(keyShiftG)),
		DashToggle:      key.NewBinding(key.WithKeys(keyE)),
		DashToggleAll:   key.NewBinding(key.WithKeys(keyShiftE)),
		DashJump:        key.NewBinding(key.WithKeys(keyEnter)),

		// Doc search
		DocSearchUp:      key.NewBinding(key.WithKeys(keyUp, keyCtrlP, keyCtrlK)),
		DocSearchDown:    key.NewBinding(key.WithKeys(keyDown, keyCtrlN, keyCtrlJ)),
		DocSearchConfirm: key.NewBinding(key.WithKeys(keyEnter)),
		DocSearchCancel:  key.NewBinding(key.WithKeys(keyEsc)),

		// Column finder
		ColFinderUp:        key.NewBinding(key.WithKeys(keyUp, keyCtrlP)),
		ColFinderDown:      key.NewBinding(key.WithKeys(keyDown, keyCtrlN)),
		ColFinderConfirm:   key.NewBinding(key.WithKeys(keyEnter)),
		ColFinderCancel:    key.NewBinding(key.WithKeys(keyEsc)),
		ColFinderClear:     key.NewBinding(key.WithKeys(keyCtrlU)),
		ColFinderBackspace: key.NewBinding(key.WithKeys(keyBackspace)),

		// Ops tree
		OpsUp:       key.NewBinding(key.WithKeys(keyK, keyUp)),
		OpsDown:     key.NewBinding(key.WithKeys(keyJ, keyDown)),
		OpsExpand:   key.NewBinding(key.WithKeys(keyEnter, keyL, keyRight)),
		OpsCollapse: key.NewBinding(key.WithKeys(keyH, keyLeft)),
		OpsTabNext:  key.NewBinding(key.WithKeys(keyF)),
		OpsTabPrev:  key.NewBinding(key.WithKeys(keyB)),
		OpsTop:      key.NewBinding(key.WithKeys(keyG)),
		OpsBottom:   key.NewBinding(key.WithKeys(keyShiftG)),
		OpsClose:    key.NewBinding(key.WithKeys(keyEsc)),

		// Extraction pipeline
		ExtCancel:     key.NewBinding(key.WithKeys(keyEsc)),
		ExtInterrupt:  key.NewBinding(key.WithKeys(keyCtrlC)),
		ExtUp:         key.NewBinding(key.WithKeys(keyK, keyUp)),
		ExtDown:       key.NewBinding(key.WithKeys(keyJ, keyDown)),
		ExtToggle:     key.NewBinding(key.WithKeys(keyEnter)),
		ExtRemodel:    key.NewBinding(key.WithKeys(keyR)),
		ExtToggleTSV:  key.NewBinding(key.WithKeys(keyT)),
		ExtAccept:     key.NewBinding(key.WithKeys(keyA)),
		ExtExplore:    key.NewBinding(key.WithKeys(keyX)),
		ExtBackground: key.NewBinding(key.WithKeys(keyCtrlB)),

		// Extraction explore
		ExploreUp:       key.NewBinding(key.WithKeys(keyK, keyUp)),
		ExploreDown:     key.NewBinding(key.WithKeys(keyJ, keyDown)),
		ExploreLeft:     key.NewBinding(key.WithKeys(keyH, keyLeft)),
		ExploreRight:    key.NewBinding(key.WithKeys(keyL, keyRight)),
		ExploreColStart: key.NewBinding(key.WithKeys(keyCaret)),
		ExploreColEnd:   key.NewBinding(key.WithKeys(keyDollar)),
		ExploreTop:      key.NewBinding(key.WithKeys(keyG)),
		ExploreBottom:   key.NewBinding(key.WithKeys(keyShiftG)),
		ExploreTabNext:  key.NewBinding(key.WithKeys(keyF)),
		ExploreTabPrev:  key.NewBinding(key.WithKeys(keyB)),
		ExploreAccept:   key.NewBinding(key.WithKeys(keyA)),
		ExploreExit:     key.NewBinding(key.WithKeys(keyEsc, keyX)),

		// Extraction model picker
		ExtModelUp:        key.NewBinding(key.WithKeys(keyUp, keyCtrlP)),
		ExtModelDown:      key.NewBinding(key.WithKeys(keyDown, keyCtrlN)),
		ExtModelConfirm:   key.NewBinding(key.WithKeys(keyEnter)),
		ExtModelCancel:    key.NewBinding(key.WithKeys(keyEsc)),
		ExtModelBackspace: key.NewBinding(key.WithKeys(keyBackspace)),

		// Help overlay
		HelpSectionUp:   key.NewBinding(key.WithKeys(keyK, keyUp)),
		HelpSectionDown: key.NewBinding(key.WithKeys(keyJ, keyDown)),
		HelpClose:       key.NewBinding(key.WithKeys(keyEsc, keyQuestion)),

		// Confirmations
		ConfirmYes: key.NewBinding(key.WithKeys(keyY)),
		ConfirmNo:  key.NewBinding(key.WithKeys(keyN, keyEsc)),

		// Inline input
		InlineConfirm: key.NewBinding(key.WithKeys(keyEnter)),
		InlineCancel:  key.NewBinding(key.WithKeys(keyEsc)),
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/app/...`
Expected: Success. The struct and constructor are defined but not yet used.

---

### Task 3: Wire AppKeyMap into Model

**Files:**
- Modify: `internal/app/model.go`

- [ ] **Step 1: Add `keys` field to Model struct**

After the `cur` field (line 217), add:

```go
	keys AppKeyMap
```

- [ ] **Step 2: Initialize keys in NewModel**

In `NewModel`, inside the `model := &Model{...}` literal (after `cur: store.Currency(),` at line 323), add:

```go
		keys: newAppKeyMap(),
```

- [ ] **Step 3: Verify tests still pass**

Run: `go test -shuffle=on -count=1 ./internal/app/...`
Expected: All tests pass (same count as baseline). The field is added but dispatch hasn't changed yet.

- [ ] **Step 4: Commit**

```
refactor(ui): add AppKeyMap struct with key.Binding definitions

Introduce keybindings.go with the complete AppKeyMap struct and
newAppKeyMap() constructor. Every key dispatched via .String()
comparisons now has a corresponding key.Binding field. The Model
struct carries a `keys AppKeyMap` field initialized in NewModel.

No dispatch sites are migrated yet — this commit only adds the
definitions. All existing tests pass unchanged.
```

---

### Task 4: Migrate model_update.go dispatch (7 inline guards)

**Files:**
- Modify: `internal/app/model_update.go`

- [ ] **Step 1: Add key import**

Add `"charm.land/bubbles/v2/key"` to the import block in `model_update.go`.

- [ ] **Step 2: Migrate global quit guard (line 28)**

Before:
```go
	if typed.String() == keyCtrlQ {
```

After:
```go
	if key.Matches(typed, m.keys.Quit) {
```

- [ ] **Step 3: Migrate global cancel guard (line 44)**

Before:
```go
	if typed.String() == keyCtrlC {
```

After:
```go
	if key.Matches(typed, m.keys.Cancel) {
```

- [ ] **Step 4: Migrate form save guard (line 278)**

Before:
```go
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == keyCtrlS {
```

After:
```go
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && key.Matches(keyMsg, m.keys.FormSave) {
```

- [ ] **Step 5: Migrate form editor guard (line 281)**

Before:
```go
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == keyCtrlE &&
```

After:
```go
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && key.Matches(keyMsg, m.keys.FormEditor) &&
```

- [ ] **Step 6: Migrate form hidden files guard (line 294)**

Before:
```go
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == keyShiftH {
```

After:
```go
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && key.Matches(keyMsg, m.keys.FormHiddenFiles) {
```

- [ ] **Step 7: Migrate form esc guard (line 325)**

Before:
```go
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == keyEsc {
```

After:
```go
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && key.Matches(keyMsg, m.keys.FormCancel) {
```

- [ ] **Step 8: Verify tests pass**

Run: `go test -shuffle=on -count=1 ./internal/app/...`
Expected: All tests pass.

---

### Task 5: Migrate model_keys.go dispatch (5 switches + 1 guard)

**Files:**
- Modify: `internal/app/model_keys.go`

This is the largest single file. Each switch changes from `switch key.String() { case keyX: }` to `switch { case key.Matches(key, m.keys.X): }`. The parameter name `key` shadows the `key` package — rename the parameter to `msg` in each handler signature.

- [ ] **Step 1: Add key import**

Add `"charm.land/bubbles/v2/key"` to the import block.

- [ ] **Step 2: Rename key parameter to msg in all handlers**

Every handler in this file takes `key tea.KeyPressMsg`. Rename to `msg tea.KeyPressMsg` to avoid shadowing the `key` package:

- `handleDashboardKeys(key tea.KeyPressMsg)` → `handleDashboardKeys(msg tea.KeyPressMsg)`
- `handleCommonKeys(key tea.KeyPressMsg)` → `handleCommonKeys(msg tea.KeyPressMsg)`
- `handleNormalKeys(key tea.KeyPressMsg)` → `handleNormalKeys(msg tea.KeyPressMsg)`
- `handleEditKeys(key tea.KeyPressMsg)` → `handleEditKeys(msg tea.KeyPressMsg)`
- `handleCalendarKey(key tea.KeyPressMsg)` → `handleCalendarKey(msg tea.KeyPressMsg)`

Also update all callers in `model_update.go` (lines 235, 239, 243, 248) from `typed` to match — these already pass `typed` which is the KeyPressMsg, so the rename is only in the function signatures.

- [ ] **Step 3: Migrate handleDashboardKeys**

The guard on line 16 changes from:
```go
if msg.String() != keyEnter {
```
to:
```go
if !key.Matches(msg, m.keys.DashJump) {
```

The switch on line 19 changes from `switch msg.String() {` to `switch {` and each case changes to `key.Matches`. Full replacement:

```go
func (m *Model) handleDashboardKeys(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if !key.Matches(msg, m.keys.DashJump) {
		m.dash.flash = ""
	}
	switch {
	case key.Matches(msg, m.keys.DashDown):
		m.dashDown()
		return nil, true
	case key.Matches(msg, m.keys.DashUp):
		m.dashUp()
		return nil, true
	case key.Matches(msg, m.keys.DashNextSection):
		m.dashNextSection()
		return nil, true
	case key.Matches(msg, m.keys.DashPrevSection):
		m.dashPrevSection()
		return nil, true
	case key.Matches(msg, m.keys.DashTop):
		m.dashTop()
		return nil, true
	case key.Matches(msg, m.keys.DashBottom):
		m.dashBottom()
		return nil, true
	case key.Matches(msg, m.keys.DashToggle):
		m.dashToggleCurrent()
		return nil, true
	case key.Matches(msg, m.keys.DashToggleAll):
		m.dashToggleAll()
		return nil, true
	case key.Matches(msg, m.keys.DashJump):
		m.dashJump()
		return nil, true
	case key.Matches(msg, m.keys.HouseToggle):
		// Block house profile toggle on dashboard.
		return nil, true
	case key.Matches(msg, m.keys.ColLeft, m.keys.ColRight):
		// Block column movement on dashboard.
		return nil, true
	case key.Matches(msg, m.keys.Sort, m.keys.SortClear,
		m.keys.ColHide, m.keys.ColShowAll,
		m.keys.EnterEditMode, m.keys.ColFinder,
		m.keys.FilterPin, m.keys.FilterToggle, m.keys.FilterNegate):
		// Block table-specific keys on dashboard.
		return nil, true
	}
	return nil, false
}
```

- [ ] **Step 4: Migrate handleCommonKeys**

```go
func (m *Model) handleCommonKeys(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Help):
		m.openHelp()
		return nil, true
	case key.Matches(msg, m.keys.HouseToggle):
		m.showHouse = !m.showHouse
		m.resizeTables()
		return nil, true
	case key.Matches(msg, m.keys.MagToggle):
		m.magMode = !m.magMode
		if m.chat != nil && m.chat.Visible {
			m.refreshChatViewport()
		}
		for i := range m.tabs {
			tab := &m.tabs[i]
			if !hasPins(tab) {
				continue
			}
			translatePins(tab, m.magMode, m.cur.Symbol())
			applyRowFilter(tab, m.magMode, m.cur.Symbol())
			applySorts(tab)
		}
		for _, dc := range m.detailStack {
			if hasPins(&dc.Tab) {
				translatePins(&dc.Tab, m.magMode, m.cur.Symbol())
				applyRowFilter(&dc.Tab, m.magMode, m.cur.Symbol())
				applySorts(&dc.Tab)
			}
		}
		if tab := m.effectiveTab(); tab != nil {
			m.updateTabViewport(tab)
		}
		return nil, true
	case key.Matches(msg, m.keys.ColLeft):
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = nextVisibleCol(tab.Specs, tab.ColCursor, false)
			m.updateTabViewport(tab)
		}
		return nil, true
	case key.Matches(msg, m.keys.ColRight):
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = nextVisibleCol(tab.Specs, tab.ColCursor, true)
			m.updateTabViewport(tab)
		}
		return nil, true
	case key.Matches(msg, m.keys.ColStart):
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = firstVisibleCol(tab.Specs)
			m.updateTabViewport(tab)
		}
		return nil, true
	case key.Matches(msg, m.keys.ColEnd):
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = lastVisibleCol(tab.Specs)
			m.updateTabViewport(tab)
		}
		return nil, true
	case key.Matches(msg, m.keys.FgExtract):
		if len(m.ex.bgExtractions) > 0 {
			m.foregroundExtraction()
			return nil, true
		}
	}
	return nil, false
}
```

- [ ] **Step 5: Migrate handleNormalKeys**

Same pattern. Replace `switch key.String() {` with `switch {` and each `case keyXxx:` with `case key.Matches(msg, m.keys.Xxx):`. Keep all body code identical.

Full handler: too long to inline here. Apply the mechanical transformation to every case. The cases map as:
- `keyShiftD` → `m.keys.Dashboard`
- `keyF` → `m.keys.TabNext`
- `keyB` → `m.keys.TabPrev`
- `keyShiftF` → `m.keys.TabLast`
- `keyShiftB` → `m.keys.TabFirst`
- `keyN` → `m.keys.FilterPin`
- `keyShiftN` → `m.keys.FilterToggle`
- `keyCtrlN` → `m.keys.FilterClear`
- `keyBang` → `m.keys.FilterNegate`
- `keyS` → `m.keys.Sort`
- `keyShiftS` → `m.keys.SortClear`
- `keyShiftU` → `m.keys.ToggleUnits`
- `keyT` → `m.keys.ToggleSettled`
- `keyC` → `m.keys.ColHide`
- `keyShiftC` → `m.keys.ColShowAll`
- `keySlash` → `m.keys.ColFinder`
- `keyCtrlF` → `m.keys.DocSearch`
- `keyO` → `m.keys.DocOpen`
- `keyI` → `m.keys.EnterEditMode`
- `keyEnter` → `m.keys.Enter`
- `keyAt` → `m.keys.Chat`
- `keyEsc` → `m.keys.Escape`

- [ ] **Step 6: Migrate handleEditKeys**

Same pattern. Case mappings:
- `keyA` → `m.keys.Add`
- `keyShiftA` → `m.keys.QuickAdd`
- `keyE` → `m.keys.EditCell`
- `keyShiftE` → `m.keys.EditFull`
- `keyD` → `m.keys.Delete`
- `keyShiftD` → `m.keys.HardDelete`
- `keyO` → `m.keys.DocOpen`
- `keyR` → `m.keys.ReExtract`
- `keyX` → `m.keys.ShowDeleted`
- `keyP` → `m.keys.HouseEdit`
- `keyEsc` → `m.keys.ExitEdit`

- [ ] **Step 7: Migrate handleCalendarKey**

Same pattern. Case mappings:
- `keyH, keyLeft` → `m.keys.CalLeft`
- `keyL, keyRight` → `m.keys.CalRight`
- `keyJ, keyDown` → `m.keys.CalDown`
- `keyK, keyUp` → `m.keys.CalUp`
- `keyShiftH` → `m.keys.CalPrevMonth`
- `keyShiftL` → `m.keys.CalNextMonth`
- `keyLBracket` → `m.keys.CalPrevYear`
- `keyRBracket` → `m.keys.CalNextYear`
- `"t"` → `m.keys.CalToday` (fixes the bare string literal)
- `keyEnter` → `m.keys.CalConfirm`
- `keyEsc` → `m.keys.CalCancel`

- [ ] **Step 8: Verify tests pass**

Run: `go test -shuffle=on -count=1 ./internal/app/...`
Expected: All tests pass.

---

### Task 6: Migrate remaining dispatch files

**Files:**
- Modify: `internal/app/chat.go`, `internal/app/model_status.go`, `internal/app/search.go`, `internal/app/column_finder.go`, `internal/app/ops_tree.go`, `internal/app/extraction.go`, `internal/app/model.go`

Apply the same mechanical transformation to each file. For every file:
1. Add `"charm.land/bubbles/v2/key"` import
2. Rename `key tea.KeyPressMsg` parameter to `msg tea.KeyPressMsg` (where it shadows the package)
3. Change `switch key.String() {` to `switch {`
4. Change each `case keyXxx:` to `case key.Matches(msg, m.keys.Xxx):`
5. Leave `default:` branches (text input, viewport delegation) unchanged

- [ ] **Step 1: Migrate chat.go — completer switch (line 1162)**

Case mappings:
- `keyEsc` → `m.keys.CompleterCancel`
- `keyUp, keyCtrlP` → `m.keys.CompleterUp`
- `keyDown, keyCtrlN` → `m.keys.CompleterDown`
- `keyEnter` → `m.keys.CompleterConfirm`
- `keyCtrlQ` → `m.keys.Quit`

- [ ] **Step 2: Migrate chat.go — main switch (line 1192)**

Case mappings:
- `keyEsc` → `m.keys.ChatHide`
- `keyEnter` → `m.keys.ChatSend`
- `keyCtrlS` → `m.keys.ChatToggleSQL`
- `keyCtrlO` → `m.keys.MagToggle`
- `keyCtrlC` → `m.keys.Cancel` (unreachable but kept)
- `keyUp, keyCtrlP` → `m.keys.ChatHistoryUp`
- `keyDown, keyCtrlN` → `m.keys.ChatHistoryDn`

- [ ] **Step 3: Migrate model_status.go — handleConfirmDiscard (line 15)**

- `keyY` → `m.keys.ConfirmYes`
- `keyN, keyEsc` → `m.keys.ConfirmNo`

- [ ] **Step 4: Migrate model_status.go — handleConfirmHardDelete (line 160)**

- `keyY` → `m.keys.ConfirmYes`
- `keyN, keyEsc` → `m.keys.ConfirmNo`

- [ ] **Step 5: Migrate search.go — handleDocSearchKey**

- `keyEsc` → `m.keys.DocSearchCancel`
- `keyEnter` → `m.keys.DocSearchConfirm`
- `keyUp, keyCtrlP, keyCtrlK` → `m.keys.DocSearchUp`
- `keyDown, keyCtrlN, keyCtrlJ` → `m.keys.DocSearchDown`

- [ ] **Step 6: Migrate column_finder.go — handleColumnFinderKey**

- `keyEsc` → `m.keys.ColFinderCancel`
- `keyEnter` → `m.keys.ColFinderConfirm`
- `keyUp, keyCtrlP` → `m.keys.ColFinderUp`
- `keyDown, keyCtrlN` → `m.keys.ColFinderDown`
- `keyBackspace` → `m.keys.ColFinderBackspace`
- `keyCtrlU` → `m.keys.ColFinderClear`

- [ ] **Step 7: Migrate ops_tree.go — handleOpsTreeKey**

- `keyEsc` → `m.keys.OpsClose`
- `keyJ, keyDown` → `m.keys.OpsDown`
- `keyK, keyUp` → `m.keys.OpsUp`
- `keyEnter, keyL, keyRight` → `m.keys.OpsExpand`
- `keyH, keyLeft` → `m.keys.OpsCollapse`
- `keyB` → `m.keys.OpsTabPrev`
- `keyF` → `m.keys.OpsTabNext`
- `keyG` → `m.keys.OpsTop`
- `keyShiftG` → `m.keys.OpsBottom`

- [ ] **Step 8: Migrate extraction.go — handleExtractionPipelineKey**

- `keyEsc` → `m.keys.ExtCancel`
- `keyCtrlC` → `m.keys.ExtInterrupt`
- `keyJ, keyDown` → `m.keys.ExtDown`
- `keyK, keyUp` → `m.keys.ExtUp`
- `keyEnter` → `m.keys.ExtToggle`
- `keyR` → `m.keys.ExtRemodel`
- `keyT` → `m.keys.ExtToggleTSV`
- `keyA` → `m.keys.ExtAccept`
- `keyX` → `m.keys.ExtExplore`
- `keyCtrlB` → `m.keys.ExtBackground`

- [ ] **Step 9: Migrate extraction.go — handleExtractionModelPickerKey**

- `keyEsc` → `m.keys.ExtModelCancel`
- `keyUp, keyCtrlP` → `m.keys.ExtModelUp`
- `keyDown, keyCtrlN` → `m.keys.ExtModelDown`
- `keyEnter` → `m.keys.ExtModelConfirm`
- `keyBackspace` → `m.keys.ExtModelBackspace`

- [ ] **Step 10: Migrate extraction.go — handleExtractionExploreKey**

- `keyEsc` → `m.keys.ExploreExit` (note: ExploreExit has both esc and x)
- `keyJ, keyDown` → `m.keys.ExploreDown`
- `keyK, keyUp` → `m.keys.ExploreUp`
- `keyH, keyLeft` → `m.keys.ExploreLeft`
- `keyL, keyRight` → `m.keys.ExploreRight`
- `keyB` → `m.keys.ExploreTabPrev`
- `keyF` → `m.keys.ExploreTabNext`
- `keyG` → `m.keys.ExploreTop`
- `keyShiftG` → `m.keys.ExploreBottom`
- `keyCaret` → `m.keys.ExploreColStart`
- `keyDollar` → `m.keys.ExploreColEnd`
- `keyA` → `m.keys.ExploreAccept`
- `keyX` → `m.keys.ExploreExit`

Note: `keyEsc` and `keyX` both map to `ExploreExit`. Since `ExploreExit` has `WithKeys(keyEsc, keyX)`, both will match. But `keyX` currently has its own case with different behavior (`ex.exploring = false` vs the esc case which also does `ex.exploring = false`). Both do the same thing, so collapsing into one binding works.

Wait — check the actual code. esc at line 1278 does `ex.exploring = false`. keyX at line 1329 also does `ex.exploring = false`. Same action. But keyA at line 1325 calls `m.acceptExtraction()`. So the ExploreExit binding (esc, x) can share one case. The keyA case is separate (ExploreAccept). This is correct.

- [ ] **Step 11: Migrate model.go — handleInlineInputKey (line 1178)**

- `keyEsc` → `m.keys.InlineCancel`
- `keyEnter` → `m.keys.InlineConfirm`

- [ ] **Step 12: Migrate model.go — helpOverlayKey (line 1476)**

Current code uses a hybrid: `keyMsg.String()` checks plus `key.Matches()` for g/G. Migrate fully:

```go
func (m *Model) helpOverlayKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.HelpClose):
		m.helpViewport = nil
	case key.Matches(msg, helpGotoTop):
		m.helpViewport.GotoTop()
	case key.Matches(msg, helpGotoBottom):
		m.helpViewport.GotoBottom()
	default:
		vp, _ := m.helpViewport.Update(msg)
		m.helpViewport = &vp
	}
	return nil
}
```

Note: `helpGotoTop`/`helpGotoBottom` are the existing `var` bindings at model.go:147-148. Keep them for now — Phase 2 will replace this entire function.

- [ ] **Step 13: Remove the existing helpGotoTop/helpGotoBottom vars**

Actually, keep them — they're still used. Phase 2 will remove them when replacing helpOverlayKey entirely.

- [ ] **Step 14: Verify ALL tests pass**

Run: `go test -shuffle=on -count=1 ./internal/app/...`
Expected: All tests pass (same count as baseline).

- [ ] **Step 15: Commit**

```
refactor(ui): migrate all key dispatch to key.Matches()

Replace .String() comparisons with key.Matches() across all 24
dispatch sites (17 switches + 7 inline guards). Each handler's
parameter renamed from `key` to `msg` to avoid shadowing the key
package.

This is a mechanical transformation: every `case keyXxx:` becomes
`case key.Matches(msg, m.keys.Xxx):`. No behavioral changes.

Fixes bare "t" string literal in handleCalendarKey (now uses
m.keys.CalToday binding).
```

---

### Task 7: Wire help renderer to AppKeyMap

**Files:**
- Modify: `internal/app/view_help.go`

- [ ] **Step 1: Replace manual binding struct with key.Binding data**

The current `helpContent()` builds sections from a local `binding` struct. Replace it to read help text from the `AppKeyMap` bindings. The renderer stays the same (custom formatting with `renderKeysLight` + `HeaderHint`), but the data source changes.

```go
func (m *Model) helpContent() string {
	type entry struct {
		keys string
		desc string
	}
	fromBinding := func(b key.Binding) entry {
		h := b.Help()
		return entry{keys: h.Key, desc: h.Desc}
	}

	sections := []struct {
		title   string
		entries []entry
	}{
		{
			title: "Global",
			entries: []entry{
				fromBinding(m.keys.Cancel),
				fromBinding(m.keys.Quit),
			},
		},
		{
			title: "Nav Mode",
			entries: []entry{
				{keyJ + "/" + keyK + "/" + symUp + "/" + symDown, "rows"},
				fromBinding(m.keys.ColLeft),
				fromBinding(m.keys.ColStart),
				{keyG + "/" + keyShiftG, "first/last row"},
				{keyD + "/" + keyU, "half page down/up"},
				fromBinding(m.keys.TabNext),
				fromBinding(m.keys.TabFirst),
				fromBinding(m.keys.Sort),
				fromBinding(m.keys.ToggleSettled),
				fromBinding(m.keys.DocSearch),
				fromBinding(m.keys.ColFinder),
				fromBinding(m.keys.ColHide),
				fromBinding(m.keys.FilterToggle),
				fromBinding(m.keys.FilterPin),
				fromBinding(m.keys.FilterNegate),
				fromBinding(m.keys.FilterClear),
				fromBinding(m.keys.Enter),
				fromBinding(m.keys.DocOpen),
				fromBinding(m.keys.HouseToggle),
				fromBinding(m.keys.ToggleUnits),
				fromBinding(m.keys.Dashboard),
				fromBinding(m.keys.Chat),
				fromBinding(m.keys.EnterEditMode),
				fromBinding(m.keys.Help),
				fromBinding(m.keys.Escape),
			},
		},
		{
			title: "Edit Mode",
			entries: []entry{
				fromBinding(m.keys.Add),
				fromBinding(m.keys.QuickAdd),
				fromBinding(m.keys.EditCell),
				fromBinding(m.keys.EditFull),
				fromBinding(m.keys.Delete),
				fromBinding(m.keys.HardDelete),
				{keyCtrlD, "half page down"},
				fromBinding(m.keys.ShowDeleted),
				fromBinding(m.keys.HouseEdit),
				fromBinding(m.keys.ExitEdit),
			},
		},
		{
			title: "Forms",
			entries: []entry{
				fromBinding(m.keys.FormNextField),
				fromBinding(m.keys.FormPrevField),
				{"1-9", "jump to Nth option"},
				fromBinding(m.keys.FormHiddenFiles),
				fromBinding(m.keys.FormEditor),
				fromBinding(m.keys.FormSave),
				fromBinding(m.keys.FormCancel),
			},
		},
		{
			title: "Chat (" + keyAt + ")",
			entries: []entry{
				fromBinding(m.keys.ChatSend),
				fromBinding(m.keys.ChatToggleSQL),
				fromBinding(m.keys.ChatHistoryUp),
				fromBinding(m.keys.ChatHide),
			},
		},
	}

	var b strings.Builder
	b.WriteString(m.styles.HeaderTitle().Render(" Keyboard Shortcuts "))
	b.WriteString("\n\n")
	for i, section := range sections {
		b.WriteString(m.styles.HeaderSection().Render(" " + section.title + " "))
		b.WriteString("\n")
		for _, e := range section.entries {
			keys := m.renderKeysLight(e.keys)
			desc := m.styles.HeaderHint().Render(e.desc)
			fmt.Fprintf(&b, "  %s  %s\n", keys, desc)
		}
		if i < len(sections)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
```

- [ ] **Step 2: Add key import to view_help.go**

Add `"charm.land/bubbles/v2/key"` to the import block.

- [ ] **Step 3: Verify tests pass**

Run: `go test -shuffle=on -count=1 -run 'TestHelp' ./internal/app/...`
Expected: All help tests pass. The help content should be visually identical since the `WithHelp` text in `newAppKeyMap()` matches the current manual strings.

- [ ] **Step 4: Run full test suite**

Run: `go test -shuffle=on -count=1 ./internal/app/...`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```
refactor(ui): wire help renderer to key.Binding help text

Replace manual binding struct in helpContent() with data read from
AppKeyMap bindings via key.Binding.Help(). Table-delegated bindings
(j/k rows, g/G, d/u) and display-only entries (1-9) remain as
inline entries. Visual output is identical.
```

---

## Phase 2: Two-Pane Help Overlay

### Task 8: Write tests for two-pane help navigation

**Files:**
- Modify: `internal/app/mode_test.go`

- [ ] **Step 1: Write test for section navigation**

```go
func TestHelpTwoPaneSectionNavigation(t *testing.T) {
	m := newTestModel(t)
	sendKey(m, "?")
	require.NotNil(t, m.helpState, "help overlay should be open")
	assert.Equal(t, 0, m.helpState.section, "should start on first section")

	// Move down to second section.
	sendKey(m, "j")
	assert.Equal(t, 1, m.helpState.section)

	// Move down again.
	sendKey(m, "j")
	assert.Equal(t, 2, m.helpState.section)

	// Move back up.
	sendKey(m, "k")
	assert.Equal(t, 1, m.helpState.section)
}
```

- [ ] **Step 2: Write test for section wraparound clamping**

```go
func TestHelpSectionClampsBounds(t *testing.T) {
	m := newTestModel(t)
	sendKey(m, "?")
	require.NotNil(t, m.helpState)

	// k at top should stay at 0.
	sendKey(m, "k")
	assert.Equal(t, 0, m.helpState.section)

	// Navigate to last section.
	sections := m.helpSections()
	for range len(sections) - 1 {
		sendKey(m, "j")
	}
	assert.Equal(t, len(sections)-1, m.helpState.section)

	// j at bottom should stay at last.
	sendKey(m, "j")
	assert.Equal(t, len(sections)-1, m.helpState.section)
}
```

- [ ] **Step 3: Write test for close with esc and ?**

```go
func TestHelpTwoPaneClose(t *testing.T) {
	m := newTestModel(t)

	// Close with esc.
	sendKey(m, "?")
	require.NotNil(t, m.helpState)
	sendKey(m, "esc")
	assert.Nil(t, m.helpState)

	// Close with ? (toggle).
	sendKey(m, "?")
	require.NotNil(t, m.helpState)
	sendKey(m, "?")
	assert.Nil(t, m.helpState)
}
```

- [ ] **Step 4: Write test for right-pane viewport content changes on section switch**

```go
func TestHelpRightPaneUpdatesOnSectionChange(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	sendKey(m, "?")
	require.NotNil(t, m.helpState)

	contentBefore := m.helpState.viewport.View()

	sendKey(m, "j") // switch section
	contentAfter := m.helpState.viewport.View()

	assert.NotEqual(t, contentBefore, contentAfter,
		"right pane content should change when section changes")
}
```

- [ ] **Step 5: Run tests to verify they fail**

Run: `go test -shuffle=on -count=1 -run 'TestHelpTwoPane|TestHelpSectionClamps|TestHelpRightPane' ./internal/app/...`
Expected: FAIL — `helpState` field and `helpSections` method don't exist yet.

---

### Task 9: Implement helpState and two-pane renderer

**Files:**
- Modify: `internal/app/model.go` (replace `helpViewport` field)
- Modify: `internal/app/view_help.go` (new renderer)

- [ ] **Step 1: Add helpState struct and replace helpViewport field**

In `model.go`, replace the `helpViewport *viewport.Model` field (line 195) with:

```go
	helpState *helpState
```

Add the struct definition near the other overlay state types:

```go
type helpState struct {
	section  int
	viewport viewport.Model
}
```

- [ ] **Step 2: Add helpSections method**

In `view_help.go`, add:

```go
type helpSection struct {
	title   string
	entries []helpEntry
}

type helpEntry struct {
	keys string
	desc string
}

func helpEntryFromBinding(b key.Binding) helpEntry {
	h := b.Help()
	return helpEntry{keys: h.Key, desc: h.Desc}
}

func (m *Model) helpSections() []helpSection {
	return []helpSection{
		{
			title: "Global",
			entries: []helpEntry{
				helpEntryFromBinding(m.keys.Cancel),
				helpEntryFromBinding(m.keys.Quit),
			},
		},
		{
			title: "Nav Mode",
			entries: []helpEntry{
				{keyJ + "/" + keyK + "/" + symUp + "/" + symDown, "rows"},
				helpEntryFromBinding(m.keys.ColLeft),
				helpEntryFromBinding(m.keys.ColStart),
				{keyG + "/" + keyShiftG, "first/last row"},
				{keyD + "/" + keyU, "half page down/up"},
				helpEntryFromBinding(m.keys.TabNext),
				helpEntryFromBinding(m.keys.TabFirst),
				helpEntryFromBinding(m.keys.Sort),
				helpEntryFromBinding(m.keys.ToggleSettled),
				helpEntryFromBinding(m.keys.DocSearch),
				helpEntryFromBinding(m.keys.ColFinder),
				helpEntryFromBinding(m.keys.ColHide),
				helpEntryFromBinding(m.keys.FilterToggle),
				helpEntryFromBinding(m.keys.FilterPin),
				helpEntryFromBinding(m.keys.FilterNegate),
				helpEntryFromBinding(m.keys.FilterClear),
				helpEntryFromBinding(m.keys.Enter),
				helpEntryFromBinding(m.keys.DocOpen),
				helpEntryFromBinding(m.keys.HouseToggle),
				helpEntryFromBinding(m.keys.ToggleUnits),
				helpEntryFromBinding(m.keys.Dashboard),
				helpEntryFromBinding(m.keys.Chat),
				helpEntryFromBinding(m.keys.EnterEditMode),
				helpEntryFromBinding(m.keys.Help),
				helpEntryFromBinding(m.keys.Escape),
			},
		},
		{
			title: "Edit Mode",
			entries: []helpEntry{
				helpEntryFromBinding(m.keys.Add),
				helpEntryFromBinding(m.keys.QuickAdd),
				helpEntryFromBinding(m.keys.EditCell),
				helpEntryFromBinding(m.keys.EditFull),
				helpEntryFromBinding(m.keys.Delete),
				helpEntryFromBinding(m.keys.HardDelete),
				{keyCtrlD, "half page down"},
				helpEntryFromBinding(m.keys.ShowDeleted),
				helpEntryFromBinding(m.keys.HouseEdit),
				helpEntryFromBinding(m.keys.ExitEdit),
			},
		},
		{
			title: "Forms",
			entries: []helpEntry{
				helpEntryFromBinding(m.keys.FormNextField),
				helpEntryFromBinding(m.keys.FormPrevField),
				{"1-9", "jump to Nth option"},
				helpEntryFromBinding(m.keys.FormHiddenFiles),
				helpEntryFromBinding(m.keys.FormEditor),
				helpEntryFromBinding(m.keys.FormSave),
				helpEntryFromBinding(m.keys.FormCancel),
			},
		},
		{
			title: "Chat (" + keyAt + ")",
			entries: []helpEntry{
				helpEntryFromBinding(m.keys.ChatSend),
				helpEntryFromBinding(m.keys.ChatToggleSQL),
				helpEntryFromBinding(m.keys.ChatHistoryUp),
				helpEntryFromBinding(m.keys.ChatHide),
			},
		},
	}
}
```

- [ ] **Step 3: Rewrite openHelp to create helpState**

```go
func (m *Model) openHelp() {
	sections := m.helpSections()
	if len(sections) == 0 {
		return
	}
	hs := &helpState{section: 0}
	m.helpState = hs
	m.updateHelpViewport()
}
```

- [ ] **Step 4: Add updateHelpViewport to populate right pane**

```go
func (m *Model) updateHelpViewport() {
	hs := m.helpState
	if hs == nil {
		return
	}
	sections := m.helpSections()
	if hs.section >= len(sections) {
		hs.section = len(sections) - 1
	}

	// Size: 60% width, 70% height, clamped.
	overlayW := max(40, m.effectiveWidth()*60/100)
	overlayH := max(12, m.effectiveHeight()*70/100)
	frameW := m.styles.OverlayBox().GetHorizontalFrameSize()
	frameH := m.styles.OverlayBox().GetVerticalFrameSize()
	innerW := overlayW - frameW
	innerH := overlayH - frameH

	// Left pane width: widest section title + cursor + padding.
	leftW := 0
	for _, s := range sections {
		if w := lipgloss.Width(s.title); w > leftW {
			leftW = w
		}
	}
	leftW += 5 // "  ▸ " prefix + " " suffix

	// Right pane: remaining width minus separator.
	rightW := innerW - leftW - 3 // " │ " separator
	if rightW < 20 {
		rightW = 20
	}

	// Render right-pane content for current section.
	section := sections[hs.section]
	var b strings.Builder
	for _, e := range section.entries {
		keys := m.renderKeysLight(e.keys)
		desc := m.styles.HeaderHint().Render(e.desc)
		fmt.Fprintf(&b, "  %s  %s\n", keys, desc)
	}
	content := strings.TrimRight(b.String(), "\n")

	// Bottom chrome: rule + hint bar + gap = 4 lines.
	vpH := innerH - len(sections) - 4
	if vpH < 3 {
		vpH = 3
	}

	vp := viewport.New(viewport.WithWidth(rightW), viewport.WithHeight(vpH))
	vp.SetContent(content)
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)
	hs.viewport = vp
}
```

- [ ] **Step 5: Rewrite helpOverlayKey for two-pane navigation**

```go
func (m *Model) helpOverlayKey(msg tea.KeyPressMsg) tea.Cmd {
	hs := m.helpState
	if hs == nil {
		return nil
	}
	switch {
	case key.Matches(msg, m.keys.HelpClose):
		m.helpState = nil
	case key.Matches(msg, m.keys.HelpSectionDown):
		sections := m.helpSections()
		if hs.section < len(sections)-1 {
			hs.section++
			m.updateHelpViewport()
		}
	case key.Matches(msg, m.keys.HelpSectionUp):
		if hs.section > 0 {
			hs.section--
			m.updateHelpViewport()
		}
	default:
		vp, _ := hs.viewport.Update(msg)
		hs.viewport = vp
	}
	return nil
}
```

- [ ] **Step 6: Update helpOverlay isVisible check**

Change `helpOverlay.isVisible()` from:
```go
func (o helpOverlay) isVisible() bool { return o.m.helpViewport != nil }
```
to:
```go
func (o helpOverlay) isVisible() bool { return o.m.helpState != nil }
```

- [ ] **Step 7: Rewrite helpView renderer**

```go
func (m *Model) helpView() string {
	hs := m.helpState
	if hs == nil {
		return ""
	}
	sections := m.helpSections()

	// Left pane: section list with cursor.
	var left strings.Builder
	for i, s := range sections {
		if i == hs.section {
			left.WriteString("  " + symTriRightSm + " ")
		} else {
			left.WriteString("    ")
		}
		left.WriteString(m.styles.HeaderSection().Render(s.title))
		if i < len(sections)-1 {
			left.WriteString("\n")
		}
	}

	// Right pane: viewport.
	rightContent := hs.viewport.View()

	// Compose left | right.
	sep := m.styles.TextDim().Render(symVLine)
	leftLines := strings.Split(left.String(), "\n")
	rightLines := strings.Split(rightContent, "\n")

	// Pad to same height.
	maxLines := max(len(leftLines), len(rightLines))
	for len(leftLines) < maxLines {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < maxLines {
		rightLines = append(rightLines, "")
	}

	leftW := 0
	for _, l := range leftLines {
		if w := lipgloss.Width(l); w > leftW {
			leftW = w
		}
	}

	var body strings.Builder
	for i := range maxLines {
		l := leftLines[i]
		padded := l + strings.Repeat(" ", max(0, leftW-lipgloss.Width(l)))
		body.WriteString(padded + " " + sep + " " + rightLines[i])
		if i < maxLines-1 {
			body.WriteString("\n")
		}
	}

	// Title + body + scroll rule + hints.
	title := m.styles.HeaderTitle().Render(" Keyboard Shortcuts ")
	vp := &hs.viewport
	contentW := lipgloss.Width(body.String())
	rule := m.scrollRule(contentW, vp.TotalLineCount(), vp.Height(),
		vp.AtTop(), vp.AtBottom(), vp.ScrollPercent(), symHLine)

	hints := []string{
		m.helpItem(keyJ+"/"+keyK, "nav"),
		m.helpItem(keyEsc, "close"),
	}
	hintStr := joinWithSeparator(m.helpSeparator(), hints...)

	return m.styles.OverlayBox().
		Render(title + "\n\n" + body.String() + "\n\n" + rule + "\n" + hintStr)
}
```

- [ ] **Step 8: Remove old helpContent function**

Delete the old `helpContent()` function from `view_help.go`. The `helpSections()` method replaces it.

- [ ] **Step 9: Remove helpGotoTop/helpGotoBottom vars from model.go**

Delete lines 144-148 in `model.go`:
```go
var (
	helpGotoTop    = key.NewBinding(key.WithKeys(keyG))
	helpGotoBottom = key.NewBinding(key.WithKeys(keyShiftG))
)
```

These are no longer needed — the viewport handles g/G via its own KeyMap.

- [ ] **Step 10: Update all references from helpViewport to helpState**

Search for `helpViewport` in the codebase and update:
- `m.helpViewport` → `m.helpState` (check nil) or `m.helpState.viewport` (access viewport)
- The `updateAllViewports` function may reference `helpViewport` — update it.

- [ ] **Step 11: Verify tests pass**

Run: `go test -shuffle=on -count=1 ./internal/app/...`
Expected: All tests pass, including the new two-pane tests from Task 8.

- [ ] **Step 12: Commit**

```
feat(ui): two-pane navigable help overlay

Replace single-pane scrollable help with a two-pane layout. Left
pane shows section names (Global, Nav Mode, Edit Mode, Forms, Chat)
navigable with j/k. Right pane shows bindings for the selected
section in a scrollable viewport.

Overlay is sized as 60% width x 70% height of the terminal, centered.
Section switching resets scroll position. Bindings styled with
existing keycap rendering (renderKeysLight + HeaderHint).
```

---

## Phase 3: Status Bar Hints (Trivially Revertible)

### Task 10: Trial help.ShortHelpView for status bar

**Files:**
- Modify: `internal/app/model.go` (add help.Model field)
- Modify: `internal/app/view.go` (replace hint system)

- [ ] **Step 1: Add help.Model to Model struct**

Add import `"charm.land/bubbles/v2/help"` and field:

```go
	helpModel help.Model
```

Initialize in `NewModel` after `keys: newAppKeyMap(),`:

```go
		helpModel: newHelpModel(),
```

Add constructor:

```go
func newHelpModel() help.Model {
	h := help.New()
	h.Styles = help.Styles{
		ShortKey:       appStyles.Keycap(),
		ShortDesc:      appStyles.HeaderHint(),
		ShortSeparator: appStyles.HeaderHint(),
		FullKey:        appStyles.KeycapLight(),
		FullDesc:       appStyles.HeaderHint(),
		FullSeparator:  appStyles.HeaderHint(),
	}
	h.ShortSeparator = " · "
	return h
}
```

- [ ] **Step 2: Implement ShortHelp on Model**

```go
func (m *Model) ShortHelp() []key.Binding {
	if m.mode == modeEdit {
		bindings := []key.Binding{m.keys.Add, m.keys.EditCell}
		bindings = append(bindings, m.keys.Delete)
		if m.effectiveTab().isDocumentTab() {
			bindings = append(bindings, m.keys.DocOpen, m.keys.ReExtract)
		}
		bindings = append(bindings, m.keys.ExitEdit)
		return bindings
	}
	// Normal mode.
	var bindings []key.Binding
	bindings = append(bindings, m.keys.Help)
	bindings = append(bindings, m.keys.Enter)
	bindings = append(bindings, m.keys.EnterEditMode)
	if m.effectiveTab().isDocumentTab() {
		bindings = append(bindings, m.keys.DocOpen, m.keys.DocSearch)
	}
	if m.llmClient != nil {
		bindings = append(bindings, m.keys.Chat)
	}
	if m.inDetail() {
		bindings = append(bindings, m.keys.Escape)
	}
	return bindings
}

func (m *Model) FullHelp() [][]key.Binding {
	sections := m.helpSections()
	groups := make([][]key.Binding, len(sections))
	for i, s := range sections {
		group := make([]key.Binding, len(s.entries))
		for j, e := range s.entries {
			group[j] = key.NewBinding(key.WithHelp(e.keys, e.desc))
		}
		groups[i] = group
	}
	return groups
}
```

- [ ] **Step 3: Replace normalModeStatusHelp**

```go
func (m *Model) normalModeStatusHelp(modeBadge string) string {
	m.helpModel.SetWidth(m.effectiveWidth() - lipgloss.Width(modeBadge) - 1)
	return modeBadge + " " + m.helpModel.ShortHelpView(m.ShortHelp())
}
```

- [ ] **Step 4: Replace editModeStatusHelp**

```go
func (m *Model) editModeStatusHelp(modeBadge string) string {
	m.helpModel.SetWidth(m.effectiveWidth() - lipgloss.Width(modeBadge) - 1)
	return modeBadge + " " + m.helpModel.ShortHelpView(m.ShortHelp())
}
```

- [ ] **Step 5: Remove old statusHint type and renderStatusHints**

Delete the `statusHint` struct, `normalModeStatusHints()`, the old `editModeStatusHelp()`, `renderStatusHints()`, and `statusHintIndex()` from `view.go`.

- [ ] **Step 6: Verify tests pass**

Run: `go test -shuffle=on -count=1 ./internal/app/...`
Expected: Tests pass. Some view snapshot tests may need updating if they assert exact status bar content.

- [ ] **Step 7: Manual visual check**

Run the app and verify:
- Status bar shows keybindings in both normal and edit mode
- Hints truncate gracefully on narrow terminals
- Mode badge still appears
- Switching modes updates the hints

- [ ] **Step 8: Commit**

```
feat(ui): trial help.ShortHelpView for status bar hints

Replace the custom statusHint priority system with
help.ShortHelpView(). Bindings are ordered by importance —
ShortHelpView truncates from the right with ellipsis.

Mode badge is prepended manually (not a keybinding).

This commit is designed to be trivially revertible if the UX
degrades compared to the old priority/compact system.
```

---

## Post-Implementation

- [ ] Run full test suite: `go test -shuffle=on -count=1 ./internal/app/...`
- [ ] Run linter: `golangci-lint run ./...`
- [ ] Run coverage: `nix run '.#coverage'`
- [ ] Visual smoke test: open the TUI, try ?, j/k in help, status bar at various widths
- [ ] Record demo: `/record-demo`
