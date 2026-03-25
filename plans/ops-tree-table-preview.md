<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Ops Tree Shadow Table Preview

Closes #775.

## Summary

When viewing the ops tree overlay for a document's extraction operations, show
a compact table preview below the JSON tree. The table interprets the raw
operations and renders them as familiar column/row tables, grouped by target
entity (vendors, quotes, etc.).

## Design

### State changes (`opsTreeState`)

Two new fields:

- `previewGroups []previewTableGroup` -- built once in `openOpsTree()` by
  decoding `ExtractionOps` JSON into `[]extract.Operation`, then calling the
  existing `groupOperationsByTable(ops, m.cur)`.
- `previewTab int` -- index into `previewGroups` for tabbed navigation when
  operations span multiple tables.

Note: `extractionLogState` already has identically-named `previewGroups` and
`previewTab` fields for the extraction explore mode. These are distinct --
`opsTreeState` owns its own copy for the ops tree overlay.

### Opening the overlay (`openOpsTree`)

After building the JSON tree, also:

1. Decode `doc.ExtractionOps` via `json.Unmarshal` into `[]extract.Operation`.
   (`extract.ParseOperations` is not used here because it expects the LLM
   wrapper format `{"operations": [...]}`, while `ExtractionOps` stores the
   raw array directly.)
2. Call `groupOperationsByTable(ops, m.cur)` to get preview groups.
3. Store both in `opsTreeState`.
4. Compute extra height for the table section and add to `maxNodes`:
   `1 (divider) + 1 (tab bar, if >1 group) + 1 (underline) + 1 (header) + 1 (divider) + max_rows`
   where `max_rows` is the largest row count across all groups.

### Rendering (`buildOpsTreeOverlay`)

After the tree node lines and padding, append (when `previewGroups` is
non-empty):

1. A horizontal divider line separating tree from table.
2. A tab bar (if >1 group) using the same `TabActive`/`TabInactive` styles as
   the extraction preview. Each tab is zone-marked with `zoneOpsTab` prefix
   for mouse clickability.
3. The active table via `renderPreviewTable` in non-interactive mode
   (`interactive=false`): dimmed, no row/col cursor.

The overlay width stays at `overlayContentWidth()` (max 72). Preview tables
will truncate columns to fit, which is fine for a read-only summary.

If `previewGroups` is empty (all ops target unknown tables), the table
section is omitted entirely -- just the tree is shown.

The hint bar gains `b/f tabs` when multiple groups exist.

### Key handling (`handleOpsTreeKey`)

- `b` -- decrement `previewTab`, clamped at 0 (no wrap, matches extraction
  explore mode behavior).
- `f` -- increment `previewTab`, clamped at `len-1` (no wrap).

Both only active when `len(previewGroups) > 1`.

### Mouse handling

- Tab bar items zone-marked as `ops-tab-N` -- clicking switches `previewTab`.
- Dispatched in the ops tree mouse handler alongside existing `ops-node-N`
  zones.

### Files

| File | Changes |
|------|---------|
| `internal/app/ops_tree.go` | State fields, parsing in `openOpsTree`, table rendering in `buildOpsTreeOverlay`, `b`/`f` keys in `handleOpsTreeKey`, `ops-tab-N` zone const |
| `internal/app/ops_tree_test.go` | Tests: table renders in view, tab switching, empty ops, preview group construction |
| `internal/app/mouse.go` | `ops-tab-N` click dispatch |

No new files. Full reuse of `groupOperationsByTable`, `renderPreviewTable`,
`previewColumns`, and `previewTableGroup` from `extraction.go`.
