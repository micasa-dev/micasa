<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Phase 2: Change Tracking (Oplog)

## Goal

Every data mutation (insert, update, soft-delete, restore) produces an
append-only oplog entry in `sync_oplog`. This is the unit of sync -- later
phases encrypt and relay these entries. Ships in the free product; no
server dependency.

## New Models

### SyncOplogEntry

```go
type SyncOplogEntry struct {
    ID        string  `gorm:"primaryKey;size:26"` // ULID
    TableName string  `gorm:"not null;index:idx_oplog_table_row"`
    RowID     string  `gorm:"not null;index:idx_oplog_table_row"`
    OpType    string  `gorm:"not null"` // "insert", "update", "delete", "restore"
    Payload   string  `gorm:"type:text;not null"` // JSON snapshot of full row
    DeviceID  string  `gorm:"not null"`
    CreatedAt time.Time
    AppliedAt *time.Time // NULL = received but not applied (conflict loser)
    SyncedAt  *time.Time `gorm:"index"` // NULL = not yet pushed to relay
}
```

### SyncDevice

```go
type SyncDevice struct {
    ID          string `gorm:"primaryKey;size:26"` // ULID, generated once
    Name        string `gorm:"not null"`           // hostname
    HouseholdID string
    LastSeq     int64  `gorm:"default:0"`
    CreatedAt   time.Time
}
```

Both are local-only (excluded from oplog tracking).

## Implementation Plan

### Step 1: Models + Migration

- Add `SyncOplogEntry` and `SyncDevice` to `internal/data/models.go`
- Add to `Models()` so AutoMigrate picks them up
- Add `SyncDevice` initialization: on first app start, generate device ID
  and store hostname
- Add helper `Store.DeviceID() string` that lazily initializes

### Step 2: Oplog Writer

- Add `internal/data/oplog.go` with:
  - `writeOplogEntry(tx *gorm.DB, tableName, rowID, opType string, payload any) error`
  - Serializes payload to JSON (omitting Document.Data BLOB field)
  - Gets device ID from store
  - Creates SyncOplogEntry with applied_at = now
- Context flag `syncApplyingKey{}` to skip oplog writes during remote apply
- `isSyncApplying(tx *gorm.DB) bool` helper

### Step 3: GORM AfterCreate/AfterUpdate/AfterDelete Hooks

Add hooks to all 11 syncable models. Each hook:
1. Checks `isSyncApplying(tx)` -- skip if remote apply
2. Calls `writeOplogEntry(tx, tableName, model.ID, opType, model)`

Tables excluded: Setting, ChatInput, DeletionRecord, SyncOplogEntry, SyncDevice

Special cases:
- **Document**: Omit `Data []byte` from payload JSON (use `json:"-"` tag
  or explicit field exclusion). Include `blob_ref` = ChecksumSHA256.
- **HouseProfile**: Syncable singleton, gets hooks like everything else.
- **ProjectType / MaintenanceCategory**: Syncable seed data, get hooks.

### Step 4: Explicit Oplog for Restoration

GORM's `Unscoped().Update("deleted_at", nil)` may not fire AfterUpdate
hooks reliably. The `restoreSoftDeleted()` and `restoreEntity()` functions
must explicitly call `writeOplogEntry(tx, table, id, "restore", model)`
after clearing deleted_at.

Same for `RestoreIncident` which has fully custom logic.

### Step 5: CASCADE Child Enumeration

For `DeleteMaintenance` (if it ever becomes a hard-delete): before writing
the parent's delete op, enumerate all ServiceLogEntries with that
maintenance_item_id and write "delete" oplog entries for each child.

Currently only soft-deletes happen (and `checkDependencies` prevents
deleting with active children), but the oplog hook on MaintenanceItem
AfterDelete should still enumerate and log child soft-deletes defensively.

For `HardDeleteIncident`: enumerate and log document detachments.

### Step 6: find-or-create Oplog Entries

`findOrCreate[T]()` can produce:
- "insert" (new entity created)
- "restore" (soft-deleted entity restored)
- no-op (existing entity found alive)

The generic helper must write the appropriate oplog entry based on which
path was taken.

### Step 7: Tests

Test-first approach. For each syncable entity:
- Create -> verify "insert" oplog entry with correct payload
- Update -> verify "update" oplog entry
- Delete -> verify "delete" oplog entry
- Restore -> verify "restore" oplog entry
- find-or-create paths -> verify correct op type

Special tests:
- CASCADE child enumeration for MaintenanceItem delete
- Document oplog excludes BLOB data
- Context flag skips oplog writes
- HardDeleteIncident produces correct oplog entries
- Device ID lazy initialization

### Step 8: Query Helpers

- `Store.UnsyncedOps() ([]SyncOplogEntry, error)` -- WHERE synced_at IS NULL
- `Store.MarkSynced(ids []string) error` -- bulk update synced_at

These are for Phase 4+ but cheap to add now.

## File Changes

| File | Change |
|------|--------|
| `internal/data/models.go` | Add SyncOplogEntry, SyncDevice models |
| `internal/data/oplog.go` | New: oplog writer, context flag, helpers |
| `internal/data/oplog_test.go` | New: comprehensive oplog tests |
| `internal/data/store.go` | Add DeviceID(), UnsyncedOps(), MarkSynced() |

## Design Decisions

- **AfterX hooks on models, not Store methods**: Hooks fire regardless of
  call site (Store, Transaction callback, extraction commit). No mutation
  can bypass the oplog.
- **JSON payload = full row snapshot**: Simpler than diffs. Small dataset
  means storage is not a concern.
- **Document BLOB exclusion**: Tag Document.Data with custom JSON tag or
  build payload struct without it. Include ChecksumSHA256 as blob_ref.
- **Device ID in Store**: Lazy-init on first oplog write. Stored in
  sync_device table (single row).
