<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Comprehensive Refactoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose the 7 largest files and eliminate ~1,000 lines of structural duplication, all behavior-preserving.

**Architecture:** Bottom-up approach: split data layer first (zero callers change), then DRY entity boilerplate in handlers/forms, then split UI overlay files, then decompose model.go. Every intermediate state compiles and passes all tests.

**Tech Stack:** Go 1.25, Bubble Tea v2, GORM, SQLite, testify

**Spec:** `plans/comprehensive-refactoring.md`

---

## Task 1: Split store.go — Core Infrastructure

Split the core infrastructure (Open, Close, Transaction, generic helpers) into
a lean `store.go`, moving everything else out in subsequent tasks.

**Files:**
- Modify: `internal/data/store.go`

- [ ] **Step 1: Verify baseline — all tests pass**

Run: `go test -shuffle=on -count=1 ./internal/data/...`
Expected: PASS

- [ ] **Step 2: Verify baseline — full test suite**

Run: `go test -shuffle=on -count=1 ./...`
Expected: PASS

- [ ] **Step 3: Identify exact boundaries**

Read `internal/data/store.go` and confirm the core infrastructure lines:
- Package declaration + imports: 1–25
- Store struct: 27–32
- Generic helpers: `unscopedPreload` (34), `identity` (36), `listQuery` (38–52), `getByID` (54–58), `findOrCreate` (63–97), `dependencyCheck` (99–103), `checkDependencies` (105–116)
- Core Store methods: `Open` (118–139), `GormDB` (143–145), `MaxDocumentSize` (148–150), `SetMaxDocumentSize` (154–160), `Currency` (163–165), `SetCurrency` (169–171), `ResolveCurrency` (178–201), `ExpandHome` (206–221), `ValidateDBPath` (229–263), `isLetterOnly` (265–275), `WalCheckpoint` (280–282), `Close` (285–291), `Transaction` (297–307), `IsMicasaDB` (310–320), `AutoMigrate` (322–330), `SeedDefaults` (332–337), `SeedDemoData` (341–343)
- Shared delete/restore helpers: `countDependents` (1800–1804), `softDeleteWith` (1806–1828), `softDelete` (1830–1834), `restoreSoftDeleted` (1840–1862), `restoreEntity` (1864–1868), `parentCheck` (1773–1777), `checkParentsAlive` (1779–1789), `requireParentAlive` (1753–1766), `parentRestoreError` (1791–1796), `countByFK` (1929–1951), `updateByIDWith` (1956–1967), `updateByID` (1969–1971), `LastDeletion` (1870–1880)

Also note that `MaxIDs` (715–727) and `RowCounts` (731–745) are general
utility methods used by the sync engine. They stay in `store.go` as core
infrastructure.

No code changes in this step — just verification.

- [ ] **Step 4: Proceed to Task 2**

This step is a no-op checkpoint.

---

## Task 2: Extract store_seed.go

Move `SeedDemoDataFrom` (the largest single function at 291 lines) into its own file.

**Files:**
- Create: `internal/data/store_seed.go`
- Modify: `internal/data/store.go` (remove lines 347–638)

- [ ] **Step 1: Create store_seed.go**

Move `SeedDemoDataFrom` (lines 347–638) and the two seed helpers `seedProjectTypes` (1882–1904) and `seedMaintenanceCategories` (1906–1925) into `internal/data/store_seed.go`. Include only the imports needed by these functions.

- [ ] **Step 2: Remove moved functions from store.go**

Delete the moved function bodies from store.go. Keep `SeedDemoData` (341–343) in store.go since it's a one-liner calling `SeedDemoDataFrom`.

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/data/...`
Expected: success

- [ ] **Step 4: Run tests**

Run: `go test -shuffle=on -count=1 ./internal/data/...`
Expected: PASS

- [ ] **Step 5: Commit**

```
refactor(data): extract SeedDemoDataFrom into store_seed.go
```

---

## Task 3: Extract store_house.go

**Files:**
- Create: `internal/data/store_house.go`
- Modify: `internal/data/store.go`

- [ ] **Step 1: Create store_house.go**

Move these methods (all on `*Store`):
- `HouseProfile` (640–647)
- `CreateHouseProfile` (649–658)
- `UpdateHouseProfile` (660–674)

- [ ] **Step 2: Remove from store.go, verify build + tests**

Run: `go build ./internal/data/...`
Run: `go test -shuffle=on -count=1 ./internal/data/...`
Expected: both pass

- [ ] **Step 3: Commit**

```
refactor(data): extract house profile methods into store_house.go
```

---

## Task 4: Extract store_vendor.go

**Files:**
- Create: `internal/data/store_vendor.go`
- Modify: `internal/data/store.go`

- [ ] **Step 1: Create store_vendor.go**

Move these methods/functions:
- `ListVendors` (692–696)
- `GetVendor` (698–700)
- `CreateVendor` (702–704)
- `FindOrCreateVendor` (709–711)
- `findOrCreateVendor` (package-level, 1973–2017)
- `UpdateVendor` (747–749)
- `DeleteVendor` (1547–1555)
- `RestoreVendor` (1557–1559)
- `CountQuotesByVendor` (752–754)
- `CountServiceLogsByVendor` (757–759)
- `ListQuotesByVendor` (767–774)
- `ListServiceLogsByVendor` (787–797)

- [ ] **Step 2: Remove from store.go, verify build + tests**

Run: `go build ./internal/data/...`
Run: `go test -shuffle=on -count=1 ./internal/data/...`

- [ ] **Step 3: Commit**

```
refactor(data): extract vendor methods into store_vendor.go
```

---

## Task 5: Extract store_project.go

**Files:**
- Create: `internal/data/store_project.go`
- Modify: `internal/data/store.go`

- [ ] **Step 1: Create store_project.go**

Move:
- `ProjectTypes` (676–682)
- `ListProjects` (799–803)
- `GetProject` (834–838)
- `CreateProject` (840–842)
- `UpdateProject` (844–846)
- `DeleteProject` (1561–1568)
- `RestoreProject` (1708–1710)
- `CountQuotesByProject` (762–764)

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(data): extract project methods into store_project.go
```

---

## Task 6: Extract store_quote.go with shared preload builder

**Files:**
- Create: `internal/data/store_quote.go`
- Modify: `internal/data/store.go`

- [ ] **Step 1: Extract shared quote preload builders**

There are two distinct preload patterns for quotes:

1. **Full preload** (used by `ListQuotes` and `GetQuote`): includes nested
   `ProjectType` on the Project preload.
2. **Simple preload** (used by `ListQuotesByVendor` and `ListQuotesByProject`):
   just `unscopedPreload` on both Vendor and Project, no nested ProjectType.

Create two helpers:

```go
// prepareQuoteRelationsFull preloads Vendor + Project with nested ProjectType.
// Used by ListQuotes and GetQuote.
func prepareQuoteRelationsFull(db *gorm.DB) *gorm.DB {
    return db.
        Preload("Vendor", unscopedPreload).
        Preload("Project", func(q *gorm.DB) *gorm.DB {
            return q.Unscoped().Preload("ProjectType")
        })
}

// prepareQuoteRelations preloads Vendor + Project (unscoped, no nested types).
// Used by scoped list queries (by vendor, by project).
func prepareQuoteRelations(db *gorm.DB) *gorm.DB {
    return db.
        Preload("Vendor", unscopedPreload).
        Preload("Project", unscopedPreload)
}
```

- [ ] **Step 2: Create store_quote.go**

Move:
- `prepareQuoteRelations` (new helper)
- `ListQuotes` (805–813) — refactor to use `prepareQuoteRelationsFull`
- `GetQuote` (848–855) — refactor to use `prepareQuoteRelationsFull`
- `CreateQuote` (857–866)
- `UpdateQuote` (868–877)
- `DeleteQuote` (1570–1572)
- `RestoreQuote` (1712–1724)
- `ListQuotesByProject` (777–784) — refactor to use `prepareQuoteRelations`
- `ListQuotesByVendor` (767–774) — already in store_vendor.go from Task 4

Wait — `ListQuotesByVendor` was moved to store_vendor.go in Task 4. That's
fine; it stays there since it's a vendor-scoped query. Just refactor it in
store_vendor.go to also use `prepareQuoteRelations` (which will be in
store_quote.go, same package).

- [ ] **Step 3: Verify build + tests**

- [ ] **Step 4: Commit**

```
refactor(data): extract quote methods into store_quote.go, DRY preload chain
```

---

## Task 7: Extract store_appliance.go

**Files:**
- Create: `internal/data/store_appliance.go`
- Modify: `internal/data/store.go`

- [ ] **Step 1: Create store_appliance.go**

Move:
- `ListAppliances` (907–911)
- `GetAppliance` (913–915)
- `CreateAppliance` (917–919)
- `FindOrCreateAppliance` (924–933)
- `UpdateAppliance` (935–937)
- `DeleteAppliance` (1698–1706)
- `RestoreAppliance` (1739–1741)
- `CountMaintenanceByAppliance` (1064–1066)
- `CountIncidentsByAppliance` (1237–1239)

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(data): extract appliance methods into store_appliance.go
```

---

## Task 8: Extract store_maintenance.go

**Files:**
- Create: `internal/data/store_maintenance.go`
- Modify: `internal/data/store.go`

- [ ] **Step 1: Create store_maintenance.go**

Move:
- `MaintenanceCategories` (684–690)
- `ListMaintenance` (815–821)
- `ListMaintenanceByAppliance` (823–832)
- `GetMaintenance` (879–883)
- `CreateMaintenance` (885–887)
- `FindOrCreateMaintenance` (892–901)
- `UpdateMaintenance` (903–905)
- `DeleteMaintenance` (1574–1581)
- `HardDeleteMaintenance` (1588–1696)
- `RestoreMaintenance` (1726–1737)

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(data): extract maintenance methods into store_maintenance.go
```

---

## Task 9: Extract store_servicelog.go

**Files:**
- Create: `internal/data/store_servicelog.go`
- Modify: `internal/data/store.go`

- [ ] **Step 1: Create store_servicelog.go**

Move:
- `syncLastServiced` (946–960, package-level helper)
- `ListServiceLog` (962–971)
- `GetServiceLog` (973–977)
- `CreateServiceLog` (979–993)
- `UpdateServiceLog` (995–1022)
- `DeleteServiceLog` (1024–1035)
- `RestoreServiceLog` (1037–1054)
- `CountServiceLogs` (1058–1060)

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(data): extract service log methods into store_servicelog.go
```

---

## Task 10: Extract store_incident.go

**Files:**
- Create: `internal/data/store_incident.go`
- Modify: `internal/data/store.go`

- [ ] **Step 1: Create store_incident.go**

Move:
- `ListIncidents` (1072–1078)
- `GetIncident` (1080–1084)
- `CreateIncident` (1086–1088)
- `UpdateIncident` (1090–1092)
- `DeleteIncident` (1094–1129)
- `RestoreIncident` (1131–1177)
- `HardDeleteIncident` (1179–1235)
- `CountIncidentsByVendor` (1241–1243)

Note: `CountIncidentsByAppliance` already moved to store_appliance.go in Task 7.

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(data): extract incident methods into store_incident.go
```

---

## Task 11: Extract store_document.go

**Files:**
- Create: `internal/data/store_document.go`
- Modify: `internal/data/store.go`

- [ ] **Step 1: Create store_document.go**

Move:
- `listDocumentColumns` (package-level var, 1251–1256)
- `metadataDocumentColumns` (package-level var, 1343–1346)
- `ListDocuments` (1258–1262)
- `ListDocumentsByEntity` (1266–1276)
- `CountDocumentsByEntity` (1281–1306)
- `GetDocument` (1308–1310)
- `GetDocumentMetadata` (1315–1319)
- `PendingBlobDocuments` (1326–1333)
- `UpdateDocumentData` (1336–1338)
- `CreateDocument` (1348–1360)
- `UpdateDocument` (1366–1396)
- `UpdateDocumentExtraction` (1401–1436)
- `EnsureDocumentAlive` (1442–1450)
- `DeleteDocument` (1452–1454)
- `RestoreDocument` (1456–1465)
- `validateDocumentParent` (1468–1500)
- `TitleFromFilename` (1505–1545)

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(data): extract document methods into store_document.go
```

---

## Task 12: Extract hardDeleteWithDocuments helper

Deduplicate the nearly identical hard-delete logic in `HardDeleteMaintenance`
and `HardDeleteIncident`.

**Files:**
- Create: `internal/data/store_hard_delete.go`
- Modify: `internal/data/store_maintenance.go`
- Modify: `internal/data/store_incident.go`

- [ ] **Step 1: Read both hard-delete functions side by side**

Read `HardDeleteMaintenance` in store_maintenance.go and `HardDeleteIncident`
in store_incident.go. Identify the shared pattern:
1. Begin transaction
2. Find entity (unscoped)
3. Find and detach linked documents (set EntityKind/EntityID to empty)
4. Write oplog entries for detached documents
5. Find and hard-delete child service logs (maintenance) or just documents (incident)
6. Delete DeletionRecords
7. Write oplog delete entry
8. Hard-delete the entity

- [ ] **Step 2: Write a failing test**

Add a test in `internal/data/store_test.go` (or a new test file) that verifies
the shared helper works for both entity types. Test that after
hard-deleting an incident with linked documents, the documents are detached
(EntityKind and EntityID cleared) and the incident is gone.

Run: `go test -shuffle=on -count=1 -run TestHardDelete ./internal/data/...`
Expected: FAIL (helper doesn't exist yet)

- [ ] **Step 3: Extract shared helper into store_hard_delete.go**

Create `hardDeleteWithDocuments(tx *gorm.DB, tableName, entity string, id string) error`
that handles the document-detach + oplog + DeletionRecord cleanup pattern.

- [ ] **Step 4: Refactor HardDeleteMaintenance to use the helper**

The maintenance version also cascades to service log children, so it calls
the shared helper for document detachment but keeps its service-log-specific
cascade logic.

- [ ] **Step 5: Refactor HardDeleteIncident to use the helper**

- [ ] **Step 6: Run full test suite**

Run: `go test -shuffle=on -count=1 ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```
refactor(data): extract shared hardDeleteWithDocuments helper
```

---

## Task 13: Extract restoreWithParentChecks helper

Deduplicate the restore + parent-validation pattern for the two methods that
cleanly fit: `RestoreQuote` and `RestoreMaintenance`. Both follow: check parents
alive → call `restoreEntity`.

`RestoreServiceLog` and `RestoreIncident` are **out of scope** — they have
custom transaction bodies (syncLastServiced for service logs, status-reset +
bespoke oplog for incidents) that can't be absorbed into a simple wrapper. They
already use `checkParentsAlive` directly, which is the right factoring for them.

**Files:**
- Modify: `internal/data/store.go` (add helper near existing restore helpers)
- Modify: `internal/data/store_quote.go`
- Modify: `internal/data/store_maintenance.go`

- [ ] **Step 1: Read RestoreQuote and RestoreMaintenance**

Verify both follow: check parents alive → call restoreEntity. Confirm that
RestoreServiceLog and RestoreIncident have custom logic that prevents reuse.

- [ ] **Step 2: Add restoreWithParentChecks to store.go**

```go
func (s *Store) restoreWithParentChecks(model any, entity string, id string, checks []parentCheck) error {
    if err := s.checkParentsAlive(checks); err != nil {
        return parentRestoreError(entity, err)
    }
    return s.restoreEntity(model, entity, id)
}
```

- [ ] **Step 3: Refactor RestoreQuote and RestoreMaintenance to use the helper**

- [ ] **Step 4: Run full test suite**

Run: `go test -shuffle=on -count=1 ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```
refactor(data): extract restoreWithParentChecks helper, DRY 2 restore methods
```

---

## Task 14: Final data layer verification + commit

**Files:**
- Verify: `internal/data/store.go` (should be ~500 lines now)

- [ ] **Step 1: Count lines in store.go**

Run: `wc -l internal/data/store.go`
Expected: ~400-600 lines (core infra + generic helpers only)

- [ ] **Step 2: Count lines in all store_*.go files**

Run: `wc -l internal/data/store*.go | sort -n`
Verify all new files exist and sum to roughly the original 2,017 lines.

- [ ] **Step 3: Run full test suite**

Run: `go test -shuffle=on -count=1 ./...`
Expected: PASS

- [ ] **Step 4: Run linter**

Run: `golangci-lint run ./internal/data/...`
Expected: no warnings

- [ ] **Step 5: Commit Phase 1 completion marker**

Use `/commit` skill. Message:

```
refactor(data): complete store.go decomposition (Phase 1)
```

---

## Task 15: Handler consolidation — baseHandler

DRY the 8 handler types that each have 6 identical one-liner delegation methods.

**Files:**
- Modify: `internal/app/handlers.go`

- [ ] **Step 1: Read handlers.go and confirm the delegation pattern**

Verify that Delete, Restore, StartAddForm, StartEditForm, InlineEdit, and
SubmitForm are pure one-liner delegations for all 8 concrete handler types
(projectHandler through documentHandler). FormKind is also a one-liner but
returns a constant.

Load and SyncFixedValues have real logic and vary per handler.

- [ ] **Step 2: Define baseHandler struct**

Add to handlers.go:

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

func (b baseHandler) FormKind() FormKind                        { return b.kind }
func (b baseHandler) Delete(s *data.Store, id string) error     { return b.deleteFn(s, id) }
func (b baseHandler) Restore(s *data.Store, id string) error    { return b.restoreFn(s, id) }
func (b baseHandler) StartAddForm(m *Model) error               { return b.startAddFn(m) }
func (b baseHandler) StartEditForm(m *Model, id string) error   { return b.startEditFn(m, id) }
func (b baseHandler) InlineEdit(m *Model, id string, col int) error { return b.inlineEditFn(m, id, col) }
func (b baseHandler) SubmitForm(m *Model) error                 { return b.submitFormFn(m) }
```

Note: `baseHandler` does NOT implement `Load` or `SyncFixedValues` — those
stay on the concrete types.

- [ ] **Step 3: Refactor projectHandler to embed baseHandler**

```go
type projectHandler struct {
    baseHandler
}
```

Remove the 6 delegation methods from projectHandler. Keep `Load` and
`SyncFixedValues` as methods on `projectHandler`. Wire the constructor:

```go
func newProjectHandler() projectHandler {
    return projectHandler{baseHandler{
        kind:         formProject,
        deleteFn:     (*data.Store).DeleteProject,
        restoreFn:    (*data.Store).RestoreProject,
        startAddFn:   (*Model).startProjectForm,
        startEditFn:  (*Model).startEditProjectForm,
        inlineEditFn: func(m *Model, id string, col int) error { return m.inlineEditProject(id, projectCol(col)) },
        submitFormFn: (*Model).submitProjectForm,
    }}
}
```

- [ ] **Step 4: Verify build + tests**

Run: `go build ./internal/app/...`
Run: `go test -shuffle=on -count=1 ./internal/app/...`

- [ ] **Step 5: Refactor remaining 7 handlers the same way**

Apply the same pattern to: quoteHandler, maintenanceHandler, applianceHandler,
incidentHandler, serviceLogHandler, vendorHandler, documentHandler.

Note serviceLogHandler has a `maintenanceItemID` field — it becomes:
```go
type serviceLogHandler struct {
    baseHandler
    maintenanceItemID string
}
```

- [ ] **Step 6: Verify every handler satisfies TabHandler**

Check that every concrete handler type still provides `Load` and
`SyncFixedValues` (the two methods NOT on `baseHandler`). Handlers with no-op
`SyncFixedValues` (quoteHandler, applianceHandler, vendorHandler,
documentHandler) must still have the stub method or the type won't satisfy
`TabHandler`.

- [ ] **Step 7: Verify build + full test suite**

Run: `go test -shuffle=on -count=1 ./...`
Expected: PASS

- [ ] **Step 8: Commit**

```
refactor(app): consolidate handler boilerplate with baseHandler embedding
```

---

## Task 16: Extract coloredOptions helper

DRY the 4 options builders that share identical structure.

**Files:**
- Modify: `internal/app/forms.go`

- [ ] **Step 1: Read the 4 duplicated builders**

Read: `incidentStatusOptions` (852–869), `incidentSeverityOptions` (871–888),
`seasonOptions` (890–912), `statusOptions` (2099–2120).

All follow: define `entry{value, color}` slice → loop → `lipgloss.NewStyle().
Foreground(color.resolve(appIsDark)).Render(label)` → `huh.NewOption` →
`withOrdinals`.

- [ ] **Step 2: Create coloredOptions helper**

```go
type colorEntry struct {
    value string
    color adaptiveColor
}

func coloredOptions(entries []colorEntry) []huh.Option[string] {
    opts := make([]huh.Option[string], len(entries))
    for i, e := range entries {
        label := statusLabel(e.value)
        colored := lipgloss.NewStyle().Foreground(e.color.resolve(appIsDark)).Render(label)
        opts[i] = huh.NewOption(colored, e.value)
    }
    return withOrdinals(opts)
}
```

- [ ] **Step 3: Refactor all 4 builders to use coloredOptions**

Each becomes a one-liner or short function returning `coloredOptions([]colorEntry{...})`.

- [ ] **Step 4: Verify build + tests**

Run: `go test -shuffle=on -count=1 ./internal/app/...`

- [ ] **Step 5: Commit**

```
refactor(app): DRY colored options builders with shared helper
```

---

## Task 17: Table-driven inline edit dispatch (Phase 2c)

DRY the 8 per-entity `inlineEditX` switch statements with a declarative spec.

**Files:**
- Modify: `internal/app/forms.go`

- [ ] **Step 1: Read all 8 inlineEdit functions**

Read: `inlineEditIncident` (765–826), `inlineEditVendor` (1103–1125),
`inlineEditProject` (1137–1190), `inlineEditQuote` (1192–1261),
`inlineEditMaintenance` (1262–1320), `inlineEditAppliance` (1322–1357),
`inlineEditServiceLog` (1460–1492), `inlineEditDocument` (2617–2656).

Identify the shared pattern: load entity → get form values → switch on column
→ open inline input / date picker / select / full form.

- [ ] **Step 2: Define inlineEditSpec type**

```go
type inlineEditSpec struct {
    textCols   []int
    selectCols map[int]func(*Model) ([]huh.Option[string], error)
    dateCols   []int
    moneyCols  []int
    notesCols  []int
}
```

- [ ] **Step 3: Create dispatchInlineEdit helper**

```go
func (m *Model) dispatchInlineEdit(id string, col int, spec inlineEditSpec, values formData) error {
    switch {
    case slices.Contains(spec.textCols, col):
        // open text inline input
    case slices.Contains(spec.dateCols, col):
        // open date picker
    case slices.Contains(spec.moneyCols, col):
        // open money inline input
    case slices.Contains(spec.notesCols, col):
        // open notes editor
    default:
        if optsFn, ok := spec.selectCols[col]; ok {
            opts, err := optsFn(m)
            if err != nil {
                return err
            }
            // open select inline edit
        }
        // fall through to full form
    }
    return nil
}
```

- [ ] **Step 4: Convert each inlineEdit function to use the spec**

Start with one (e.g., inlineEditVendor, the simplest) and verify it works.
Then convert the rest. Each becomes: load entity → build values → call
`dispatchInlineEdit(id, col, vendorEditSpec, values)`.

- [ ] **Step 5: Run tests**

Run: `go test -shuffle=on -count=1 ./internal/app/...`
Expected: PASS

- [ ] **Step 6: Commit**

```
refactor(app): DRY inline edit dispatch with table-driven spec
```

---

## Task 18: Consolidate test model factories (Phase 2d)

**Files:**
- Modify: `internal/app/model_with_store_test.go`
- Modify: `internal/app/model_with_demo_data_test.go`
- Modify: `internal/app/currency_flow_test.go`

- [ ] **Step 1: Read all test model factory functions**

Identify: `newTestModelWithStore`, `newTestModelWithDemoData`,
`newTestModelWithCurrency`. Confirm they share: open store → set max doc size
→ set currency → create house → create Model → set dimensions → exit form →
hide dashboard.

Leave `newTestModel` (mode_test.go, struct literal bypass) and
`newTestModelWithDetailRows` (detail-stack seeding) unchanged — they serve
different initialization paths.

- [ ] **Step 2: Create unified testModelOpts builder**

```go
type testModelOpts struct {
    seed     int64
    currency locale.Currency
    withDemo bool
}

func newTestModelWith(t *testing.T, opts testModelOpts) *Model {
    // shared initialization logic
}
```

- [ ] **Step 3: Refactor the 3 factory functions to use the builder**

Each becomes a thin wrapper or direct call to `newTestModelWith`.

- [ ] **Step 4: Run tests**

Run: `go test -shuffle=on -count=1 ./internal/app/...`
Expected: PASS

- [ ] **Step 5: Commit**

```
refactor(app): consolidate test model factories into testModelOpts builder
```

---

## Task 19: Split extraction.go — extraction_render.go

**Files:**
- Create: `internal/app/extraction_render.go`
- Modify: `internal/app/extraction.go`

- [ ] **Step 1: Create extraction_render.go**

Move all rendering functions (lines 1366–2272 of current extraction.go):
- `buildExtractionOverlay` (1369–1383)
- `previewNaturalWidth` (1387–1403)
- `buildExtractionPipelineOverlay` (1408–1599)
- `renderOperationPreviewSection` (1604–1659)
- `renderPreviewTable` (1662–1704)
- `renderExtractionStep` (1714–1950)
- `renderPageRatio` (1955–1974)
- `stepName` (1976–1988)
- `marshalOps` (1994–2003)
- `extractionModelUsed` (2007–2012)
- `extractionModelLabel` (2015–2020)
- `truncateRight` (2022–2030)
- `previewColumns` (2044–2102)
- `groupOperationsByTable` (2125–2206)
- `fmtAnyText`, `fmtAnyFK`, `fmtAnyInterval` (2210–2237)
- `extractionOverlayWidth` (2241–2272)

- [ ] **Step 2: Remove from extraction.go, verify build**

Run: `go build ./internal/app/...`

- [ ] **Step 3: Run tests**

Run: `go test -shuffle=on -count=1 ./internal/app/...`

- [ ] **Step 4: Commit**

```
refactor(app): split extraction rendering into extraction_render.go
```

---

## Task 20: Split chat.go into chat.go + chat_render.go

**Files:**
- Create: `internal/app/chat_render.go`
- Modify: `internal/app/chat.go`

- [ ] **Step 1: Create chat_render.go**

Move rendering functions (lines 1148–1571 of current chat.go):
- `waitForChunk` (1148–1152)  — keep in chat.go (it's streaming, not rendering)
- `refreshChatViewport` (1155–1162)
- `renderChatMessages` (1165–1258)
- `llmModelLabel` (1260–1265)
- `handleChatKey` (1268–1343) — keep in chat.go (it's dispatch, not rendering)
- `syncCompleter` (1348–1366) — keep in chat.go
- `buildChatOverlay` (1370–1430)
- `renderModelCompleter` (1434–1437)
- `renderModelCompleterFor` (1440–1511)
- `highlightModelMatch` (1519–1533)
- `chatOverlayWidth` (1537–1546)
- `chatViewportWidth` (1548–1550)
- `chatViewportHeight` (1552–1566)
- `chatInputWidth` (1568–1570)

So rendering = refreshChatViewport, renderChatMessages, llmModelLabel,
buildChatOverlay, renderModelCompleter*, highlightModelMatch, and all layout
helpers.

- [ ] **Step 2: Remove from chat.go, verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(app): split chat rendering into chat_render.go
```

---

## Task 21: Split view.go — extract view_help.go

**Files:**
- Create: `internal/app/view_help.go`
- Modify: `internal/app/view.go`

- [ ] **Step 1: Create view_help.go**

Move help-related rendering:
- `helpContent` (768–868)
- `helpView` (871–890)

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(app): extract help rendering into view_help.go
```

---

## Task 22: Overlay interface + dispatchOverlay refactor

Introduce the `overlay` interface and refactor `dispatchOverlay` to use it.

**Files:**
- Modify: `internal/app/model.go` (add interface, refactor dispatchOverlay)
- Modify: `internal/app/extraction.go` (implement overlay)
- Modify: `internal/app/chat.go` (implement overlay)
- Modify: `internal/app/model.go` (other overlay types — inline wrappers)

- [ ] **Step 1: Define overlay interface**

Add to model.go (near existing overlay-related code):

```go
type overlay interface {
    isVisible() bool
    view(width, height int) string
    handleKey(key tea.KeyPressMsg) tea.Cmd
}
```

- [ ] **Step 2: Create overlay adapter methods for each subsystem**

For overlays that already have `handleXKey` methods, create thin wrappers
implementing the interface. For simple overlays (notePreview, opsTree), create
inline structs.

The simplest approach: create a `modelOverlays()` method that returns
`[]overlay` in priority order, where each overlay is a small adapter struct.

- [ ] **Step 3: Refactor dispatchOverlay to use the interface**

Replace the 9-case switch with a loop over `m.modelOverlays()`:

```go
func (m *Model) dispatchOverlay(msg tea.Msg) (tea.Cmd, bool) {
    for _, o := range m.modelOverlays() {
        if !o.isVisible() {
            continue
        }
        keyMsg, ok := msg.(tea.KeyPressMsg)
        if !ok {
            return nil, false
        }
        return o.handleKey(keyMsg), true
    }
    return nil, false
}
```

- [ ] **Step 4: Refactor hasActiveOverlay to use the interface**

```go
func (m *Model) hasActiveOverlay() bool {
    for _, o := range m.modelOverlays() {
        if o.isVisible() {
            return true
        }
    }
    return false
}
```

- [ ] **Step 5: Verify build + full test suite**

Run: `go test -shuffle=on -count=1 ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```
refactor(app): introduce overlay interface, unify dispatch
```

---

## Task 23: Split model.go — extract model_keys.go

**Files:**
- Create: `internal/app/model_keys.go`
- Modify: `internal/app/model.go`

- [ ] **Step 1: Create model_keys.go**

Move keyboard dispatch functions:
- `handleDashboardKeys` (744–788)
- `handleCommonKeys` (790–858)
- `handleNormalKeys` (860–972)
- `handleNormalEnter` (974–1042)
- `handleEditKeys` (1044–1094)
- `handleCalendarKey` (1096–1123)
- `confirmCalendar` (1125–1138)
- `openCalendar` (1140–1158)

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(app): extract keyboard dispatch into model_keys.go
```

---

## Task 24: Split model.go — extract model_tabs.go

**Files:**
- Create: `internal/app/model_tabs.go`
- Modify: `internal/app/model.go`

- [ ] **Step 1: Create model_tabs.go**

Move tab and detail management:
- `activeTab` (1177–1184)
- `effectiveTab` (1200–1207)
- `inDetail` (1194–1198)
- `detail` (1186–1192)
- `detailDef` struct (1209–1217)
- `stdBreadcrumb` (1220–1366)
- `getVendorName` (1368–1374)
- `getIncidentTitle` (1376–1382)
- `getProjectTitle` (1384–1390)
- `getMaintenanceName` (1392–1398)
- `getQuoteDisplayName` (1400–1407)
- `openDetailFromDef` (1409–1423)
- Convenience routes (1425–1449)
- `detailRoute` struct (1458–1502)
- `openDetailForRow` (1505–1525)
- `openDetailWith` (1527–1536)
- `closeDetail` (1538–1587)
- `closeAllDetails` (1589–1610)
- `reloadDetailTab` (1612–1620)
- `switchToTab` (1711–1720)
- `nextTab` (1722–1731)
- `prevTab` (1733–1741)

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(app): extract tab/detail management into model_tabs.go
```

---

## Task 25: Split model.go — extract model_status.go

**Files:**
- Create: `internal/app/model_status.go`
- Modify: `internal/app/model.go`

- [ ] **Step 1: Create model_status.go**

Move status, confirm, and form lifecycle:
- `handleConfirmDiscard` (724–743)
- `startAddForm` (1743–1751)
- `startEditForm` (1753–1766)
- `startCellOrFormEdit` (1768–1799)
- `toggleDeleteSelected` (1838–1878)
- `promptHardDelete` (1880–1905)
- `handleConfirmHardDelete` (1907–1927)
- `setStatusInfo` (2661–2663)
- `setStatusSaved` (2665–2667)
- `setStatusError` (2669–2673)
- `surfaceError` (2675–2679)

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(app): extract status/confirm/delete into model_status.go
```

---

## Task 26: Split model.go — extract model_update.go

**Files:**
- Create: `internal/app/model_update.go`
- Modify: `internal/app/model.go`

- [ ] **Step 1: Create model_update.go**

Move the core message dispatch:
- `update` (365–617) — the 250-line switch on message types
- `updateForm` (618–720) — form-specific key handling
- `formInitCmd` (2681–2692)

Keep `Update` (356–363, the top-level wrapper) in model.go since it's the
Bubble Tea interface method.

- [ ] **Step 2: Verify build + tests**

- [ ] **Step 3: Commit**

```
refactor(app): extract message dispatch into model_update.go
```

---

## Task 27: Final verification and lint

**Files:**
- All modified files

- [ ] **Step 1: Count lines in model.go**

Run: `wc -l internal/app/model.go`
Target: ~1,200-1,400 lines (down from 3,226)

- [ ] **Step 2: Count lines in all split files**

Run: `wc -l internal/app/model*.go internal/app/extraction*.go internal/app/chat*.go internal/app/view*.go internal/data/store*.go | sort -n`

- [ ] **Step 3: Run full test suite**

Run: `go test -shuffle=on -count=1 ./...`
Expected: PASS

- [ ] **Step 4: Run linter**

Run: `golangci-lint run ./...`
Expected: no warnings

- [ ] **Step 5: Final commit**

```
refactor: complete codebase decomposition (Phases 1-4)
```

---

## Execution Notes

- **Each task is one commit.** If a task fails mid-way, fix before moving on.
- **Line numbers are from the original files.** As earlier tasks move code, later
  task line numbers shift. Always search for the function name, not the line.
- **No behavioral changes.** Every function signature stays identical. Tests
  should pass without modification.
- **Phase 5 (style injection) is deferred.** Not included in this plan per the
  spec — do opportunistically in future work.
- **Run tests after every move.** The `go test` command catches compilation
  errors and behavioral regressions.
