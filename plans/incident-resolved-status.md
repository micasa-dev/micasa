<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Incident Resolved Status

Closes #561.

## Problem

Soft-deleting an incident leaves its status as "open" or "in_progress" -- the
status column lies about the incident's actual state. Users see a
strikethrough row labeled "open" which is confusing.

Additionally there is no way to permanently delete an incident.

## Design

### 1. Add `resolved` status constant

Add `IncidentStatusResolved = "resolved"` to `internal/data/models.go`.

### 2. Delete sets status to "resolved"

`Store.DeleteIncident` becomes a transaction that:
1. Sets `status = "resolved"` via an `Unscoped` update (must be unscoped
   because GORM's soft-delete may have already set `deleted_at` by the time
   we try to update)
2. Calls the existing `softDelete` helper

Actually, cleaner: do both in a single transaction. Update status first, then
soft-delete. The `softDelete` helper already wraps in a transaction, so we
need to inline its logic or extend it.

### 3. Restore resets status to "open"

`Store.RestoreIncident` sets `status = "open"` in the same transaction that
clears `deleted_at`.

### 4. UI surfaces

- `incidentStatusOptions()`: add `resolved` entry (dim color, like
  "completed" for projects)
- `statusLabels`: add `"resolved" -> "res"`
- `SyncFixedValues`: add `IncidentStatusResolved` to the fixed values list
- `incidentSeverityOptions()`: no change
- Dashboard `ListOpenIncidents`: already filters by status IN (open,
  in_progress), so resolved incidents are automatically excluded -- no change
  needed

### 5. Hard delete

Add `Store.HardDeleteIncident(id uint)` that permanently removes the row.
Wire a new keybinding (`shift+D` / `D`) in edit mode for incidents only.
This requires:
- A `HardDelete` method on `TabHandler` (optional -- returns
  `ErrNotSupported` by default)
- Dispatch in `toggleDeleteSelected` or a separate handler for `D`
- Confirmation prompt before permanent deletion

### 6. Docs

Update `docs/content/docs/guide/incidents.md` to describe the new resolved
status and hard delete capability.

### 7. Tests

- Store-level: delete sets status to resolved, restore resets to open
- Handler-level: round-trip with resolved status verification
- User-flow: keypress `d` to resolve, `d` to restore, `D` to hard delete

## Files touched

- `internal/data/models.go` -- new constant
- `internal/data/store.go` -- modified Delete/Restore, new HardDelete
- `internal/app/compact.go` -- new label mapping
- `internal/app/forms.go` -- updated status options
- `internal/app/handlers.go` -- updated SyncFixedValues, new HardDelete
- `internal/app/model.go` -- hard delete dispatch + key constant
- `internal/app/styles.go` -- resolved already handled (dim), no change
- `internal/data/store_test.go` -- new tests
- `internal/app/handler_crud_test.go` -- updated + new tests
- `docs/content/docs/guide/incidents.md` -- updated docs
