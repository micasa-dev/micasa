<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Extraction: Temp Shadow Tables for Cross-Reference Resolution

Issue: #585

## Status: Implemented

## Problem

When the LLM produces a batch of extraction operations like "create vendor"
followed by "create quote with vendor\_id: 1", the `vendor_id` is a fictional
cross-reference -- the LLM assumes its own auto-increment sequence, but real DB
IDs are unpredictable.

The current fix tracks batch-created entities in `dispatchContext` and falls
through on failed FK lookups (e.g. `dispatchCreateQuote` tries vendor\_id as a
real DB ID, falls back to vendor\_name, then falls back to the last
batch-created vendor). This works for vendor->quote but is brittle:

- Every new FK relationship needs its own fallback path
- Multi-hop references compound the fragility
- The dispatch code accumulates entity-specific special cases

## Design: in-memory shadow DB

### Core idea

Stage the entire LLM operation batch in a separate in-memory SQLite database.
The shadow DB seeds its auto-increment counters from the real DB's max IDs so
shadow IDs start at `max(real_id) + 1` per table. This ensures shadow IDs
occupy a disjoint range from real IDs, making cross-references unambiguous: the
LLM sees the real DB state in the schema context so it naturally produces IDs
continuing from the last real ID. On accept, copy rows from shadow -> real DB,
remapping shadow IDs to real IDs.

### New type: `extract.ShadowDB`

Lives in `internal/extract/shadow.go`.

```go
type ShadowDB struct {
    db      *gorm.DB
    // created tracks shadow entries per table in insertion order.
    created map[string][]shadowEntry
}

type shadowEntry struct {
    shadowID uint
    opData   map[string]any // original operation data for synthetic fields
}
```

### Shadow DB schema

The shadow DB auto-migrates the extraction-relevant GORM models:
`project_types`, `maintenance_categories`, `projects`, `vendors`,
`appliances`, `quotes`, `maintenance_items`, `documents`.

All tables start empty but `sqlite_sequence` is seeded from the real DB:
for each creatable table, `NewShadowDB` queries `MAX(id)` and inserts the
value into the shadow's `sqlite_sequence`. Auto-increment then continues
from `max_real_id + 1`, matching the LLM's expected ID sequence (the LLM
sees current DB state in the schema context and generates IDs accordingly).

This eliminates ambiguity: if the real DB has vendors 1-5, shadow IDs start
at 6. A quote referencing `vendor_id: 3` is clearly an existing vendor;
`vendor_id: 6` is clearly the batch-created one.

FK constraints: **OFF** (SQLite default). The shadow DB is a staging area;
validation happens during the commit phase against the real DB. This avoids
the complexity of seeding reference tables just to satisfy shadow FK checks,
while still resolving cross-references correctly.

Original operation data is preserved alongside shadow entries so synthetic
fields (e.g. `vendor_name`, which isn't a real DB column) survive the
staging round-trip and are available during commit.

### Lifecycle

```
LLM completes -> ParseOperations -> NewShadowDB(store) -> Stage(ops)
                                                            |
                     User presses 'a' (accept) ---------> Commit(store)
                     User presses ESC (cancel) ---------> discard (GC)
```

1. **NewShadowDB(store)**: Opens `:memory:` SQLite, auto-migrates extraction
   tables, queries `MAX(id)` per creatable table from the real DB via
   `store.MaxIDs()`, and seeds `sqlite_sequence` so auto-increment starts
   at `max_real_id + 1`.

2. **Stage(ops)**: Inserts each operation into the shadow DB in order.
   - Creates: INSERT into shadow table, shadow auto-increment assigns ID,
     record in `created[table]`
   - Updates: recorded for later application (these reference real DB rows)

3. **Commit(store)**: Copies shadow -> real in dependency order:

   ```
   vendors, appliances     (no FK deps among creatables)
       |         |
       v         v
     quotes  maintenance_items  (depend on vendors/appliances)
       |         |
       v         v
         documents              (polymorphic, depends on anything)
   ```

   For each table:
   a. For each shadow row in creation order:
      - Build entity struct from shadow row data
      - For FK columns: look up `idMap[fkTable][shadowValue]`; if mapping
        exists, replace with real ID; otherwise keep as-is (real DB reference)
      - Create in real DB via existing Store methods
      - Record `idMap[thisTable][shadowID] = realID`
   b. Wrap the entire commit in a transaction

4. **Discard**: On reject/cancel/commit, `closeShadowDB()` explicitly closes
   the underlying `:memory:` connection. Called from `cancelExtraction`,
   `cancelAllExtractions`, `commitShadowOperations`, and the LLM rerun path.

### ID remapping detail

```go
type idMap map[string]map[uint]uint // table -> shadowID -> realID
```

FK columns per table:
- quotes: `vendor_id` -> vendors
- maintenance\_items: `category_id` -> maintenance\_categories (seeded, no
  remap needed), `appliance_id` -> appliances
- documents: `entity_id` -> polymorphic (remap based on `entity_kind`)

When remapping a FK value for table T:
1. Look up `idMap[T][value]` -- if found, this was a batch-created entity,
   use the mapped real ID
2. If not found, the value is a real DB ID (either from a seeded reference
   table or an existing entity) -- keep as-is

### Entity resolution (duplicate names)

When committing a batch-created vendor, use the existing `findOrCreateVendor`
pattern: if a vendor with the same name already exists in the real DB, return
the existing one. The shadow -> real ID mapping records this correctly regardless
of whether the entity was newly created or found existing.

### What this replaces

All legacy dispatch code has been removed:

- `dispatchContext` struct and its `createdVendors` tracking
- `dispatchOperations`, `dispatchOneOperation` router
- `dispatchCreateVendor`, `dispatchCreateQuote`, `dispatchCreateMaintenance`,
  `dispatchCreateAppliance`, `dispatchCreateDocument`, `dispatchUpdateDocument`
- `parseIntFromData`, `parseInt64FromData` helper functions
- All per-entity FK resolution hacks

The shadow staging + commit flow is now the only path. `commitShadowOperations`
errors if no shadow DB is present (no fallback).

### What this preserves

- Store methods: `CreateVendor`, `CreateQuote`, `CreateMaintenance`,
  `CreateAppliance`, `CreateDocument`, `UpdateDocument` remain the
  authoritative write path
- `findOrCreateVendor` remains the vendor resolution strategy
- Operation parsing and validation are unchanged
- The extraction overlay UI, pipeline steps, explore mode, and accept/reject
  keybindings are unchanged (only the accept handler changes internally)

### Open questions (from issue)

1. **`:memory:` vs TEMP tables** -> `:memory:` separate connection. Cleaner
   isolation, zero risk of polluting main DB, auto-cleanup on GC.

2. **Entity resolution** -> Use existing `findOrCreateVendor` during commit.
   For other entities, name-match or create-new as appropriate per table.

3. **Editable preview overlay (#471)** -> Not in this implementation. Shadow
   tables lay the groundwork (queryable staged data) but the UI integration
   is a separate concern.

## Implementation plan

### Phase 1: ShadowDB core (`internal/extract/shadow.go`) -- Done

- [x] `NewShadowDB(store)` -- create `:memory:` DB, migrate extraction tables,
  seed `sqlite_sequence` from real DB max IDs via `store.MaxIDs()`
- [x] `Stage(ops)` -- insert operations into shadow tables
- [x] `Commit(store, ops)` -- copy shadow -> real with ID remapping
- [x] `FindOrCreateVendor` exposed on Store for dedup during commit
- [x] `MaxIDs` exposed on Store for querying per-table max auto-increment IDs
- [x] Unit tests for staging, remapping, commit, auto-increment offset

### Phase 2: Wire into extraction flow (`internal/app/extraction.go`) -- Done

- [x] Add `shadowDB *extract.ShadowDB` field to `extractionLogState`
- [x] Create shadow DB + stage ops after successful LLM parse
- [x] `commitShadowOperations` calls `shadowDB.Commit(store, ops)` with
  fallback to legacy `dispatchOperations` if no shadow DB
- [x] On LLM rerun, close and nil shadowDB
- [x] Remove legacy `dispatchContext` and per-entity dispatch methods
- [x] `Commit` wrapped in `store.Transaction` for atomic rollback on failure
- [x] `FindOrCreateAppliance` and `FindOrCreateMaintenance` for entity dedup
- [x] Deterministic `Close()` on shadow DB at all disposal sites

### Phase 3: Tests -- Done

- [x] Unit tests: shadow DB staging, ID remapping, dependency ordering
- [x] Commit tests: vendor+quote cross-ref, appliance+maintenance cross-ref,
  multiple vendors+quotes, document entity_id remapping
- [x] Edge cases: duplicate vendor names, existing vendor by ID, vendor_name
  only, mixed creates+updates, empty batches
- [x] Full test suite passes (all 10 packages)

### Future work (not in this PR)

- Shadow DB query for structured preview in explore mode
- FK constraint validation feedback in the overlay UI
- Integration with editable preview overlay (#471)
- Multi-hop reference chains (vendor -> category -> project)
