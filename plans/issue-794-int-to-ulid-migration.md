<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Plan: Fix int-to-ULID migration for existing databases (Issue #794)

## Problem

Commit dc12376 changed all entity primary keys from `uint` (INTEGER) to
`string` (ULID, 26-char TEXT). When a user upgrades from v2.2 to v2.3,
GORM's AutoMigrate attempts to alter column types but existing rows contain
integer IDs. SQLite's INTEGER PRIMARY KEY is a rowid alias — inserting a
ULID string into it triggers SQLITE_MISMATCH (error code 20).

Additionally, all foreign key columns (e.g. `project_type_id`, `vendor_id`)
also changed from integer to text, requiring coordinated conversion.

## Approach

Add a pre-migration step in `AutoMigrate()` that:

1. **Detects** whether the database uses the old integer-ID schema by
   checking `typeof(id)` on a row in the `vendors` table (a table that
   always has rows after SeedDefaults via the project_types/categories).
   Actually, better: check PRAGMA table_info for the id column type.
2. **Converts** all tables by:
   a. Building an old-int → new-ULID mapping for each table's IDs
   b. Using SQLite's table-recreation pattern (create temp, copy with
      conversion, drop original, rename temp)
   c. Processing tables in topological order (parents before children)
      so FK references can be resolved
3. **Runs before** GORM AutoMigrate, which then handles any remaining
   schema differences (new tables like sync_oplog_entries, etc.)

## FK Relationship Map

Parent tables (no FK dependencies):
- house_profiles, project_types, maintenance_categories, vendors, appliances

Child tables and their FK columns:
- projects: project_type_id → project_types
- quotes: project_id → projects, vendor_id → vendors
- maintenance_items: category_id → maintenance_categories,
  appliance_id → appliances (nullable)
- incidents: appliance_id → appliances (nullable),
  vendor_id → vendors (nullable)
- service_log_entries: maintenance_item_id → maintenance_items,
  vendor_id → vendors (nullable)
- documents: entity_id → polymorphic (entity_kind determines table)
- deletion_records: target_id → any entity (entity column determines table)

Tables that keep integer PKs (no migration needed):
- settings (key is text PK)
- chat_inputs (uint PK, local-only)

New tables (don't exist in old schema, AutoMigrate creates them):
- sync_oplog_entries, sync_devices

## Migration Algorithm

```
PRAGMA foreign_keys = OFF
BEGIN TRANSACTION

For each table in topological order:
  1. Read all rows, build id_map[old_int_string] = new_ulid
  2. CREATE TABLE temp with TEXT id column (same schema as new models)
  3. INSERT INTO temp SELECT ... FROM original
     - Replace id with ULID from id_map
     - Replace FK columns using parent table's id_map
  4. DROP TABLE original
  5. ALTER TABLE temp RENAME TO original

COMMIT
PRAGMA foreign_keys = ON
```

## Files to Create/Modify

- `internal/data/migrate.go` (new) — migration logic
- `internal/data/migrate_test.go` (new) — tests
- `internal/data/store.go` — call migration from AutoMigrate()

## Risks

- **Data loss on failed migration**: Mitigated by wrapping in a transaction.
  SQLite's transaction semantics ensure atomicity.
- **Large databases**: The migration reads all rows into memory for ID
  mapping. For micasa's use case (home maintenance) this is fine — even
  heavy users won't have more than a few thousand rows.
- **Edge case: empty database**: If database has tables but no rows,
  migration should still change column types. AutoMigrate handles this.
