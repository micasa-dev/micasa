<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->
<!-- verified: 2026-03-10 -->

# Code Patterns & Conventions

## Architecture Patterns

### Model-Update-View (Bubble Tea)
- Model.Update(msg) dispatches by message type, then by mode
- Model.View() -> buildView() -> baseView + overlay stack
- Overlays: dashboard, calendar, chat, help, column finder, note preview, extraction log
- overlay.Composite() for centering with dimmed background

### TabHandler Polymorphism
Each entity tab implements TabHandler interface. All entity-specific logic
(Load, Delete, StartAddForm, SubmitForm, etc.) lives in the handler.
No scattered FormKind/TabKind switches outside the handler.

### Rendering Pipeline
```
View() -> buildView()
  -> buildBaseView() [house + tab bar + table/form + status bar]
  + overlays (priority order: dashboard > calendar > notes > colFinder > extraction > chat > help)
    -> overlay.Composite()
```

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
