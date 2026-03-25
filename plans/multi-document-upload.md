<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Multi-Document Upload

GitHub issue: #489

## Problem

The file picker (`Shift+A` on the Docs tab) only supports selecting one file
at a time. Uploading multiple documents requires repeating the full
add-then-extract flow for each file, which is tedious for bulk operations like
scanning a stack of receipts or uploading warranty PDFs for a new appliance.

## Design

### Architecture: new mode, not a form

The staging overlay is a new state struct with its own Update handler and View
function, gated by a pointer-nil check (`m.batchDoc != nil`) rather than a new
`Mode` constant. This matches the existing `extractionLogState` pattern where
overlays compose on top of any mode. It does **not** use `huh.Form`. The
overlay:

- Owns an embedded `huh.FilePicker` instance directly (not wrapped in a form).
- Intercepts `enter` at the overlay level: when the file picker has a selected
  file, the overlay captures the path, reads the file eagerly (size + MIME
  detection + SHA256), appends to the staged list, and resets the picker by
  re-creating it at the same directory. Directory navigation (`enter` on a
  directory) is passed through to the picker unchanged.
- Manages its own focus state (picker vs. list), cursor position in the staged
  list, and cancel-confirmation flag.
- Dispatches to the existing extraction pipeline on confirm.
- The existing `form_filepicker.go` helpers (`syncFilePickerDescription`,
  `syncFilePickerTitle`, `filePickerCurrentDir`, `filePickerShowHidden`)
  assume the picker lives inside a `huh.Form`. These must be refactored to
  accept a bare `*huh.FilePicker` so the overlay can reuse them for the
  `Shift+H` toggle and directory label display.

This follows the same pattern as `extractionLogState` -- a self-contained
overlay with custom key handling that composes with the main model.

### Staging overlay

`Shift+A` on the Docs tab opens the multi-doc staging overlay. The overlay has
two focus zones toggled with `tab`:

**Top zone -- file picker.** An embedded `huh.FilePicker` instance (not inside
a `huh.Form`). When `enter` is pressed on a file (not a directory), the overlay
intercepts the keypress, reads the file eagerly to capture size, MIME type, and
SHA256 checksum, appends a `stagedFile` entry to the list, and re-creates the
file picker at the same directory so it is ready for the next pick.

**Bottom zone -- staged files list.** A scrollable, navigable list of files
queued for upload. Each row shows filename, size, and detected MIME type (known
at stage time because files are read eagerly). The list is keyboard-navigable
(`j`/`k`/arrows) with `x` or `d` to remove the highlighted entry.

```
+-- Add Documents -------------------------+
|                                          |
|  ~/Downloads/                            |
|  > invoice-jan.pdf                       |
|    receipt-plumber.jpg                   |
|    warranty-hvac.pdf                     |
|                                          |
|  -- Staged (2) ------------------------- |
|  [x] invoice-jan.pdf          2.1 MB     |
|  [x] receipt-plumber.jpg      340 KB     |
|                                          |
|  [enter] add  [ctrl+s] upload all        |
|  [x] remove   [esc] cancel              |
+------------------------------------------+
```

### Staged file struct

Each staged file holds eagerly-read data so submission is instant:

```go
type stagedFile struct {
    Path     string // absolute path, used for dedup
    Name     string // base filename
    Data     []byte // file contents (read at stage time)
    Size     int64
    MIME     string // detected at stage time
    Checksum string // SHA256, computed at stage time
    Err      error  // non-nil if read/checksum/size-limit failed
}
```

Files that fail at stage time (unreadable, exceeds `store.MaxDocumentSize`)
are still added to the list but rendered with an error badge. The user can
remove them or proceed -- errored files are skipped at submission.

### Keybindings

| Focus   | Key          | Action                              |
|---------|--------------|-------------------------------------|
| Picker  | `j`/`k`      | Navigate files                      |
| Picker  | `enter`      | Stage highlighted file              |
| Picker  | `Shift+H`    | Toggle hidden files                 |
| List    | `j`/`k`      | Navigate staged files               |
| List    | `x` or `d`   | Remove highlighted file             |
| List    | `enter`      | Preview file details                |
| Either  | `tab`        | Toggle focus between zones          |
| Either  | `ctrl+s`     | Confirm batch (no-op if list empty) |
| Either  | `esc`        | Cancel (confirm if files staged)    |

### Entity scoping

- **Scoped view** (inside a project/appliance/etc): the parent entity is
  inherited automatically. No picker shown.
- **Top-level Docs tab**: the staging overlay includes an entity selector as
  its **first page**. This is a single `huh.Select` field (kind + ID, same
  data as the full document form's entity dropdown) rendered inside the overlay
  before the picker appears. `enter` confirms the selection and transitions to
  the picker+list view. `esc` cancels the entire flow. Once confirmed, the
  entity is locked for the batch and displayed in the overlay header.

### Submission

When the user presses `ctrl+s`:

1. Each staged file with `Err == nil` creates a `Document` record via
   `store.CreateDocument`. Title defaults to the filename (sans extension).
   Notes left empty. The file data is already in memory from eager reading.
2. Each `CreateDocument` fires its own `AfterCreate` GORM hook, writing an
   independent oplog entry. The relay picks these up naturally with no protocol
   changes.
3. Files where `CreateDocument` fails (e.g., constraint violation) are marked
   as failed in the batch state. Successful files proceed to extraction. No
   rollback of successful uploads.
4. Successfully created documents are collected into a batch for extraction.

### Batch extraction

After all documents are saved, the overlay transitions to a batch extraction
progress view:

1. Extraction uses the currently configured model (`m.ex.extractionModel`),
   same as `startExtractionOverlay` does today. No new model-picker modal. If
   the user wants a different model, they configure it before starting the
   batch (or re-extract individual docs later with `r`).
2. Each document's extraction results are **auto-accepted** as they complete.
   `UpdateDocumentExtraction` is called immediately when a document's pipeline
   finishes, before starting the next document. This avoids holding N sets of
   results in memory and means partial progress is persisted even if the batch
   is cancelled mid-way. **LLM-proposed operations** (vendor/quote creation
   via `commitShadowOperations`) are **not auto-committed** in batch mode.
   Only text, OCR, and structured extraction data are persisted. The user can
   review and accept LLM operations per-document after the batch completes by
   selecting individual documents and triggering the existing extraction
   overlay. This avoids conflicts (e.g., two invoices both proposing
   `create vendor "ABC"`) and keeps the batch flow focused on ingestion.
3. Extraction runs **sequentially** -- one document at a time through the
   existing `extract.Pipeline` (text -> OCR -> LLM). Parallelism is avoided
   because OCR (tesseract) is memory-heavy and LLM APIs enforce rate limits.
4. The progress view shows:
   - Per-document status: checkmark (done), spinner (in-progress), dot
     (waiting), X (failed)
   - Current document's step detail (same 3-step breakdown as today)
   - Overall progress counter ("2/5 complete")
5. On completion, the overlay shows a summary (N succeeded, M failed with
   reasons) and dismisses on `enter` or `esc`. The table reloads to show all
   new documents.
6. Failed extractions do not block the batch. The document record exists; the
   user can retry extraction later from the Docs tab.

```
+-- Extracting 3 documents ----------------+
|                                          |
|  Model: gpt-4o-mini                      |
|                                          |
|  [done] invoice-jan.pdf                  |
|  [OCR]  receipt-plumber.jpg              |
|  [wait] warranty-hvac.pdf               |
|                                          |
|  Overall: 1/3 complete                   |
+------------------------------------------+
```

### Edge cases

- **Duplicate checksum**: At stage time, the overlay queries for existing
  documents with the same SHA256 on the target entity. This requires a new
  store method: `DocumentExistsByChecksum(entityKind, entityID, checksum)
  bool`. If a match is found, the staged file gets a "duplicate" warning
  badge but is not blocked. The entity is always known at stage time in both
  flows (scoped: inherited; top-level: confirmed on the first page before the
  picker appears), so the query runs immediately when the user stages a file.
- **Same path staged twice**: Deduplicate by absolute path in the staged list.
  Silently ignore the second `enter`.
- **Empty batch**: `ctrl+s` is disabled (hint text dimmed) when the list is
  empty.
- **Cumulative size**: The staged list shows total size. A status-line warning
  appears if the total exceeds `store.MaxDocumentSize * 10` (not a hardcoded
  value), but upload is not blocked since the per-file limit is already
  enforced at stage time.
- **Cancel with staged files**: `esc` shows a confirmation ("Discard N staged
  files?") before closing.
- **Cancel during extraction**: `esc` during the extraction phase stops
  processing remaining documents. Already-saved documents and their completed
  extractions are kept. The summary view shows which documents were processed
  and which were skipped.

### What stays unchanged

- **Single-file add** (`a` lowercase): full document form with title, entity,
  file picker, notes -- untouched.
- **Edit document form**: still single-file replacement.
- **Extraction overlay for single docs**: unchanged when triggered on an
  existing document from the Docs tab.
- **Relay/oplog**: no protocol changes. Each document gets its own oplog entry
  via `AfterCreate`.

### Relationship to #490

Issue #490 (background extraction) is a natural follow-up. Once extraction can
run in the background, the batch extraction phase here can kick off all
extractions without blocking the UI. This spec assumes extraction blocks the
overlay for now; background extraction would let the user dismiss the overlay
and continue working while extractions complete.
