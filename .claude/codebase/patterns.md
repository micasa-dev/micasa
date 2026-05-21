<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->
<!-- verified: 2026-04-23 -->

# Code Patterns & Conventions

## Architecture Patterns

### Model-Update-View (Bubble Tea)
- Model.Update(msg) dispatches by message type, then by mode
- Model.View() -> buildView() -> baseView + overlay stack
- Overlays (bottom-up; later renders on top): dashboard, house profile, calendar,
  note preview, ops tree, column finder, doc search, extraction, chat, help
- overlay.Composite() for centering with dimmed background

### TabHandler Polymorphism
Each entity tab implements TabHandler interface. All entity-specific logic
(Load, Delete, StartAddForm, SubmitForm, etc.) lives in the handler.
No scattered FormKind/TabKind switches outside the handler.

### Drill-down Tab Kind Inheritance
Detail (drill-down) tabs inherit the parent tab's Kind, not the semantic
entity Kind (see `openDetailFromDef` in `model_tabs.go`). E.g. an
Appliance > Documents drill-down has `Tab.Kind = tabAppliances`, not
`tabDocuments`. Always identify entity semantics via the Handler's
`FormKind()` (or a helper like `Tab.isDocumentTab()`) -- never
`tab.Kind == tabX`, which silently breaks drill-down parity.

### Rendering Pipeline
```
View() -> buildView()
  -> buildBaseView() [collapsed house strip + tab bar + table/form + status bar]
  + overlays in priority order (top of stack wins):
      dashboard > house > calendar > notePreview > opsTree > colFinder
        > docSearch > extraction > chat > help
    -> overlay.Composite() with dimmed base
```
Full-screen first-run house form short-circuits buildView before any overlay.

### Data Flow
```
Store (SQLite) -> TabHandler.Load() -> rows/cells/meta
  -> Full* (pre-filter) -> applyRowFilter (pins) -> applySorts -> CellRows (displayed)
    -> renderTableRows (with styles, links, drilldowns)
```

### Message Dispatch
```
tea.Msg -> Update(msg)
  KeyMsg -> mode-specific handler (handleNormalKeys/handleEditKeys/etc.)
  MouseMsg -> handleMouse() (zone-based dispatch)
  chatChunkMsg/sqlChunkMsg -> chat handlers
  extractionStepCompleted -> extraction handlers
  WindowSizeMsg -> resize tables/viewports
```

## CRUD Patterns (internal/data/)

### Generic Helpers
- listQuery[T](store, includeDeleted, prepare) - generic list with optional soft-delete scope
- getByID[T](store, id, prepare) - generic single fetch

### Soft Delete Flow
1. GORM Delete() sets deleted_at = NOW()
2. Insert DeletionRecord{Entity, TargetID, DeletedAt}
3. Undo: restore via undoStack snapshot

### Restoration Flow
1. Unscoped.Update(deleted_at = NULL)
2. Set DeletionRecord.RestoredAt = NOW()

### Dependency Checking
- checkDependencies(id, checks) before delete
- countDependents() only counts non-deleted children
- Returns error if any deps exist (prevents orphans)

### Find-or-Create with Restore
- Used for Vendor, Appliance, MaintenanceItem
- Searches including deleted; restores if soft-deleted; creates if not found

### Parent Alive Validation
- requireParentAlive(model, id): checks if FK parent is alive/deleted/gone

## Form Lifecycle
```
StartAddForm()/StartEditForm()
  -> Build huh.Form with fields + validators
  -> mode = modeForm
User edits (keyboard, calendar picker)
  -> submitForm()
    -> Validate all fields
    -> Snapshot for undo
    -> handler.SubmitForm() (create/update DB)
    -> Reload affected tabs
    -> mode = previous mode
```

## Test Patterns

### Test Setup
- testmain_test.go: pre-migrated template DB (fast per-test cloning)
- newTestModelWithStore(t): Model with real SQLite, sized 120x40
- newTestModelWithDemoData(t, seed): Model with faker-seeded data

### Test Helpers
- sendKey(m, key) - sends KeyMsg
- sendMouse(x, y, button, action) - sends MouseMsg
- sendClick(x, y) - left-click shortcut
- openAddForm(m) - enters form mode
- requireZone(t, m, zoneID) - asserts zone exists

### Mandatory Patterns (see AGENTS.md "Testing" for authoritative rules)
- User-flow tests via keystrokes, never just internal API calls
- Form tests: openAddForm(m) + set values + sendKey(m, "ctrl+s")
- Inline edit: sendKey(m, "i") + set ColCursor + sendKey(m, "e")
- Dashboard: loadDashboardAt(now)
- Regression: write failing test first, then fix root cause

## Style Conventions

### Styles Singleton
- All styles in Styles struct (styles.go) with private fields + public accessors
- Package-level appStyles singleton, never mutated
- Never inline lipgloss.NewStyle() in render functions
- Wong colorblind-safe palette with AdaptiveColor{Light, Dark}

### Key Constants
- All key strings defined as constants in model.go
- Used in dispatch (case), key.WithKeys, SetKeys, helpItem, renderKeys
- Never bare key string literals

### Zone IDs for Mouse
- tab-N, row-N, col-N, hint-ID, dash-N
- house-header, breadcrumb-back, overlay

## Config Resolution
1. Defaults (constants in config.go)
2. TOML file (XDG_CONFIG_HOME/micasa/config.toml)
3. Environment variables (MICASA_* overrides via reflection)
4. Validation & provider auto-detection

## Money Handling
- Stored as int64 cents in DB
- Formatted via locale.FormatMoney(cents, currency)
- Currency resolved: config -> DB setting -> env/locale -> persisted
