<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Comprehensive Codebase Refactoring

Full-codebase refactoring audit and execution plan. Bottom-up approach: data
layer first, then entity boilerplate, then UI overlays, then Model
decomposition.

## Motivation

The codebase is clean (zero lint warnings, zero dead code, all tests pass) but
showing structural strain from feature accumulation:

- 7 files over 1,000 lines, largest 3,226 (model.go)
- ~2,700 lines of structural duplication across 8 entity types
- `store.go` has 120 methods mixing CRUD for all entities
- 9 overlays checked with ad-hoc nil/visible patterns
- `model.update()` is 250 lines dispatching 20+ message types

All refactoring is behavior-preserving. No API changes, no new features.

## Phase 1: Data Layer Decomposition

Split `store.go` (2,017 lines, 120 methods) into per-entity files. The
`*Store` receiver stays the same -- pure file moves plus helper extraction.

### File splits

| New file | Contents | Est. lines |
|----------|----------|------------|
| `store.go` | Core: Open, Close, Transaction, AutoMigrate, Backup, IsMicasaDB, generic helpers (listQuery, getByID, countByFK, checkDependencies, restoreEntity) | ~500 |
| `store_house.go` | HouseProfile CRUD | ~35 |
| `store_project.go` | Project + ProjectType CRUD | ~80 |
| `store_quote.go` | Quote CRUD + shared preload builder | ~70 |
| `store_vendor.go` | Vendor CRUD + find-or-create | ~60 |
| `store_maintenance.go` | MaintenanceItem + MaintenanceCategory CRUD + hard-delete | ~150 |
| `store_servicelog.go` | ServiceLogEntry CRUD + sync logic | ~120 |
| `store_incident.go` | Incident CRUD + hard-delete | ~170 |
| `store_appliance.go` | Appliance CRUD | ~50 |
| `store_document.go` | Document CRUD + polymorphic handling | ~250 |
| `store_seed.go` | SeedDemoDataFrom | ~290 |

### Shared helpers to extract

1. **`hardDeleteWithDocuments(tx, tableName, id)`** -- consolidates identical
   logic from HardDeleteIncident and HardDeleteMaintenance. Both detach linked
   documents, write oplog entries for detached docs, delete DeletionRecords,
   write delete oplog entry, and permanently remove the row. ~100 lines deduped.

2. **`restoreWithParentChecks`** -- consolidates restore methods that all fetch
   unscoped, validate parent liveness, then call restoreEntity. Concrete
   methods: RestoreQuote (checks Project + Vendor), RestoreMaintenance (checks
   Appliance). RestoreServiceLog and RestoreIncident are intentionally excluded
   because they have custom transaction bodies (syncLastServiced for service
   logs, status-reset + bespoke oplog for incidents). The helper signature:
   ```go
   type parentCheck struct {
       model any       // GORM model to check
       id    string    // FK value to look up
       name  string    // human-readable name for error message
   }
   func (s *Store) restoreWithParentChecks(entity any, id string, checks []parentCheck) error
   ```
   ~50 lines deduped.

3. **Quote preload builder** -- named function replacing 4 duplicated
   `Preload("Vendor", unscopedPreload).Preload("Project", ...)` chains.

### Result

`store.go` drops from 2,017 to ~500 lines. ~150 lines of duplication
eliminated. All methods stay on `*Store` so callers are unaffected.

## Phase 2: Entity Boilerplate DRY

Targets ~2,700 lines of structural repetition across 8 entity types.

### 2a: Handler consolidation (handlers.go)

Replace 8 handler structs x 7 one-liner delegation methods with a `baseHandler`
struct holding function references matching the real TabHandler signatures:

```go
type baseHandler struct {
    kind         FormKind
    deleteFn     func(*data.Store, string) error
    restoreFn    func(*data.Store, string) error
    startAddFn   func(*Model) error
    startEditFn  func(*Model, string) error
    inlineEditFn func(*Model, string, int) error
    submitFormFn func(*Model) error
}
```

Each handler becomes a constructor wiring entity-specific functions.
Entity-specific logic (Load, SyncFixedValues) stays overridden per handler
as embedded struct methods. ~310 lines eliminated.

### 2b: Shared options builder (forms.go)

Extract common pattern from 8 options builders:

```go
func coloredOptions(entries []struct{ Value, Label string; Color adaptiveColor }) []huh.Option[string]
```

~80 lines consolidated.

### 2c: Table-driven inline edit dispatch (forms.go)

Replace 8 per-entity `inlineEditX` switch statements with a declarative spec:

```go
type inlineEditSpec struct {
    textCols   []int
    selectCols map[int]func(*Model) ([]huh.Option[string], error)
    dateCols   []int
    moneyCols  []int
    notesCols  []int
}
```

One shared `dispatchInlineEdit` handles the switch. ~400 lines consolidated.

### 2d: Test model factory (test files)

Consolidate the full-initialization test model variants (`newTestModelWithStore`,
`newTestModelWithDemoData`, `newTestModelWithCurrency`) into one parametric
builder:

```go
type testModelOpts struct {
    seed     int64
    currency locale.Currency
    withDemo bool
}
func newTestModel(t *testing.T, opts testModelOpts) *Model
```

Leave `newTestModel` (mode_test.go, struct-literal path bypassing NewModel)
and `newTestModelWithDetailRows` (detail-stack seeding) as-is since they serve
fundamentally different initialization paths.

~60 lines consolidated.

### Result

~890 lines of duplication eliminated. Adding a new entity type becomes
filling in a descriptor instead of copy-pasting 7 methods + form lifecycle +
inline edit switch.

## Phase 3: UI Overlay Extraction

### 3a: Overlay interface

```go
type overlay interface {
    isVisible() bool
    view(width, height int) string
    handleKey(key tea.KeyPressMsg) tea.Cmd
}
```

Model gains `activeOverlays() []overlay` returning the stack in priority order.
`dispatchOverlay` iterates this list: it finds the first visible overlay, checks
if the message is a `tea.KeyPressMsg`, and dispatches to `handleKey`. Non-key
messages still fall through (preserving the current contract in
`dispatchOverlay` at model.go:2776).

**Dashboard exception**: The dashboard has special nav-key interception that
runs before normal handlers (model.go:583-585). Dashboard implements the
overlay interface but its `handleKey` is only used for the standard overlay
dispatch path. The pre-handler nav interception stays as explicit code in
`update()` rather than being shoehorned into the interface.

~60 lines of scattered nil/visible checks consolidated. Adding a 10th overlay
becomes a one-step process.

### 3b: Split extraction.go (2,272 lines -> 3 files)

| File | Contents | Est. lines |
|------|----------|------------|
| `extraction.go` | State types, lifecycle, message handling | ~800 |
| `extraction_render.go` | buildExtractionPipelineOverlay, renderExtractionStep, column width, operation preview | ~900 |
| `extraction_accept.go` | Commit/discard staged operations UI | ~570 |

The 236-line `renderExtractionStep` breaks into 3 helpers: metric bar, tool
output, operation summary.

### 3c: Split chat.go (1,570 lines -> 2 files)

| File | Contents | Est. lines |
|------|----------|------------|
| `chat.go` | State types, message handling, streaming | ~800 |
| `chat_render.go` | Overlay rendering, message bubbles, SQL results | ~770 |

### 3d: Split view.go (1,408 lines -> 2 files)

| File | Contents | Est. lines |
|------|----------|------------|
| `view.go` | buildView, buildBaseView, overlay composition, status bar | ~600 |
| `view_help.go` | Help overlay rendering | ~400 |

(House rendering already lives in house.go.)

### Result

No lines eliminated (decomposition, not deduplication). Largest UI files drop
to ~800 lines each. Overlay dispatch becomes systematic.

## Phase 4: Model Decomposition

Split `model.go` (3,226 lines) into focused files.

| File | Contents | Est. lines |
|------|----------|------------|
| `model.go` | Model struct, NewModel, Init, View, top-level Update | ~400 |
| `model_update.go` | Message type dispatch: handleWindowSize, handleBackgroundColor, handleFormMsg, handleSyncMsg, handleExtractionMsg, handleChatMsg | ~600 |
| `model_keys.go` | handleNormalKeys, handleEditKeys, keybinding setup | ~500 |
| `model_tabs.go` | Tab management: reloadTab, reloadAllTabs, switchTab, pushDetail, popDetail | ~400 |
| `model_status.go` | setStatusInfo, setStatusError, surfaceError, confirmKind type, confirm/discard flows (handleConfirmDiscard, handleConfirmHardDelete, handleConfirmFormQuit) | ~300 |

Remaining ~1,100 lines of helpers stay in model.go or migrate to the file
where they are called.

## Phase 5: Style Injection (Deferred)

Move `appStyles` from package-level var to Model field. Rendering functions
get styles passed as parameter or via `m.styles`. Mechanical but touches many
call sites. Can be done opportunistically as other phases touch rendering code.
Not worth a dedicated PR.

**Migration safety**: During the transition, both `appStyles` (package-level)
and `m.styles` must resolve to the same value. This is already true today
(`NewModel` sets `m.styles = appStyles` and `update` reassigns both on
`BackgroundColorMsg`). Opportunistic migration means some rendering code uses
`m.styles` while other code still reads `appStyles` -- this is safe as long as
the dual-assignment in `update()` is preserved until migration is complete.

## Execution Order

Each phase is one PR (except Phase 2 which may split into 2a-b and 2c-d).

1. Phase 1 -- data layer (lowest risk, pure file moves + helper extraction)
2. Phase 2a-b -- handler consolidation + options builder
3. Phase 2c-d -- inline edit dispatch + test factory
4. Phase 3 -- overlay interface + file splits
5. Phase 4 -- model decomposition
6. Phase 5 -- deferred / opportunistic

## Constraints

- Every intermediate state compiles and passes all tests
- No behavioral changes -- all refactoring is structure-only
- No new packages (stays in internal/app/ and internal/data/)
- Follow existing conventions: unexported by default, no speculative features
- Commit at each logical stopping point
