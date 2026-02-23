<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Extraction: JSON Operations via Store API

Issue: #474

## Status: Implemented

Originally planned as raw SQL output, pivoted to JSON operations dispatched
through the Store API. Raw SQL bypassed GORM lifecycle hooks (soft-delete
tracking, timestamps, FK validation) and Store-layer business logic. JSON
operations give the same extensibility while keeping all writes going through
the existing Store methods.

## Motivation

Every new extractable field previously required changes in four places:

1. `ExtractionHints` struct (`hints.go`)
2. The JSON extraction prompt (`llmextract.go`)
3. JSON parsing code (`ParseExtractionResponse`)
4. Form pre-fill logic (`applyExtractionHints`)

With operation-based output, the LLM emits structured operations that dispatch
to existing Store methods. New columns just need the dispatch function updated
-- no separate hints struct, no parsing, no mapping.

## Design

### LLM output format

The LLM outputs a JSON array of operation objects:

```json
[
  {"action": "create", "table": "vendors", "data": {"name": "Garcia Plumbing"}},
  {"action": "update", "table": "documents", "data": {"id": 42, "title": "Invoice"}},
  {"action": "create", "table": "quotes", "data": {"total_cents": 150000, "vendor_id": 1}}
]
```

- `action`: `"create"` or `"update"`
- `table`: one of the allowed tables
- `data`: column-value map (keys are DB column names)

### Validation

Operations are validated against a strict allowlist:

| Table                | create | update |
|----------------------|--------|--------|
| `documents`          | yes    | yes    |
| `vendors`            | yes    | no     |
| `quotes`             | yes    | no     |
| `maintenance_items`  | yes    | no     |
| `appliances`         | yes    | no     |

Unknown tables, disallowed actions, and unrecognized action verbs are rejected.

### Store API dispatch

Each operation maps to an existing Store method:

- `create vendors` -> `store.CreateVendor`
- `create quotes` -> `store.CreateQuote`
- `create maintenance_items` -> `store.CreateMaintenance`
- `create appliances` -> `store.CreateAppliance`
- `create documents` -> `store.CreateDocument`
- `update documents` -> `store.UpdateDocument`

Unknown data keys are silently ignored (defense in depth).

### Operation preview

The extraction overlay shows a tabbed table preview of proposed operations
below the pipeline steps. Each tab corresponds to an affected table, using the
same column specs and rendering functions as the main UI tables.

Two modes:
- **Pipeline mode**: dimmed preview, navigate pipeline steps with j/k
- **Explore mode** (press x): interactive table with row/col cursors, tab
  switching with b/f, pipeline steps dimmed

### Deferred document creation (magic-add)

The `A` keybinding creates a document without persisting it. The document is
held in memory (`pendingDoc`) until the user accepts the extraction results.
On accept, LLM-produced document fields are applied to the pending doc before
creation. On cancel, nothing touches the database.

## What was added

- `internal/extract/operations.go` -- Operation type, ParseOperations,
  ValidateOperations
- `internal/extract/sqlcontext.go` -- SchemaContext, FormatDDLBlock,
  FormatEntityRows, ExtractionTables
- `internal/data/ddl.go` -- Store.TableDDL()
- `internal/data/entity_rows.go` -- Store.EntityRows()

## What was modified

- `internal/extract/llmextract.go` -- prompt rewrite (DDL + entity rows +
  JSON operation rules)
- `internal/app/extraction.go` -- operation dispatch, tabbed preview,
  explore mode, deferred creation
- `internal/app/forms.go` -- DeferCreate flag for magic-add
- `internal/app/model.go` -- saveDeferredDocumentForm intercept

## What remains from the old system

- `hints.go` -- ExtractionHints types still used by Pipeline.Result; will be
  removed when Pipeline is updated to return operations directly.
- `ParseExtractionResponse` -- still referenced by pipeline tests; removal
  deferred to cleanup pass.

## Next steps

- Cross-referencing newly created entities (e.g., link document to a
  just-created vendor) needs a dependency ordering system (future issue)
- shift+R to rerun the entire extraction pipeline (issue #499)
- Remove legacy ExtractionHints once Pipeline returns operations directly
