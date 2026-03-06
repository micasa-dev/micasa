+++
title = "Architecture"
weight = 2
description = "How micasa is built: Bubble Tea, TabHandler, overlays."
linkTitle = "Architecture"
+++

micasa is a [Bubble Tea](https://github.com/charmbracelet/bubbletea)
application following The Elm Architecture (TEA): Model, Update, View.

## Package layout

```
cmd/micasa/          CLI entry point (kong argument parsing)
internal/
  app/               Bubble Tea application layer
    model.go         Model struct, Init, Update, key dispatch
    types.go         Mode, Tab, cell, columnSpec, etc.
    handlers.go      TabHandler interface + entity implementations
    tables.go        Column specs, row builders, table construction
    forms.go         Form builders, validators, submit logic
    styles.go        Wong colorblind-safe palette, all lipgloss styles
    view.go          Main View() assembly, overlays
    table.go         Table rendering (headers, rows, viewport)
    collapse.go      Hidden column badges
    house.go         House profile rendering
    dashboard.go     Dashboard data loading + view
    sort.go          Multi-column sort logic
    undo.go          Undo/redo stack
    form_select.go   Select field ordinal jumping
    calendar.go      Inline date picker overlay
    column_finder.go Fuzzy column jump overlay
    extraction.go    Extraction pipeline overlay (OCR + LLM progress)
  extract/           Document extraction pipeline
    extractor.go     Extractor interface and concrete implementations
    text.go          PDF text extraction (pdftotext from poppler-utils)
    ocr.go           OCR via tesseract + pdftoppm
    ocr_progress.go  Channel-based OCR progress for async overlay
    llmextract.go    LLM prompt construction + response parsing
    operations.go    Operation type, JSON Schema, parsing, and validation
    sqlcontext.go    Schema context (DDL + entity rows) for prompts
    pipeline.go      Pipeline orchestrator (text -> OCR -> LLM)
    tools.go         External tool availability checks
  data/              Data access layer
    models.go        GORM models (HouseProfile, Project, Document, etc.)
    store.go         Store struct, CRUD methods, queries
    doccache.go      Document BLOB extraction + XDG cache
    dashboard.go     Dashboard-specific queries
    path.go          DB path resolution (XDG)
    validation.go    Parsing helpers (dates, money, ints)
```

## Key design decisions

### TabHandler interface

Entity-specific operations (load, delete, add form, edit form, inline edit,
submit, snapshot, etc.) are encapsulated in the `TabHandler` interface.
Each entity type (projects, quotes, maintenance, appliances, vendors,
documents) implements this interface as a stateless struct.

This eliminates scattered `switch tab.Kind` dispatch. Adding a new entity type
means implementing one interface -- no shotgun surgery across the codebase.

Detail views (service log, appliance maintenance) also implement `TabHandler`,
so they get all the same capabilities (add, edit, delete, sort, undo) for
free.

### Modal key handling

micasa uses three modes: Nav, Edit, and Form. The key dispatch chain in
`Update()` is:

1. Window resize handling
2. <kbd>ctrl+q</kbd> always quits
3. <kbd>ctrl+c</kbd> cancels in-flight LLM operations
4. Chat chunk messages (streaming responses)
5. Help overlay intercepts <kbd>esc</kbd>/<kbd>?</kbd> when open
6. Chat overlay: absorbs all keys when open
7. Note preview overlay: any key dismisses
8. Calendar date picker: absorbs all keys when open
9. Column finder overlay: absorbs all keys when open
10. Inline input: absorbs keys when editing a cell
11. Form mode delegates to `huh` form library
12. Dashboard intercepts nav keys when visible
13. Common keys (shared by Nav and Edit)
14. Mode-specific keys

The `bubbles/table` widget has its own vim keybindings. In Edit mode, <kbd>d</kbd> and
<kbd>u</kbd> are stripped from the table's KeyMap so they can be used for delete/undo
without conflicting with half-page navigation.

### Effective tab

The `effectiveTab()` method returns the detail tab when a detail view is open,
or the main active tab otherwise. All interaction code uses this method, so
detail views work identically to top-level tabs.

### Cell-based rendering

Table cells carry type information (`cellKind`): text, money, date, status,
readonly, drill. The renderer uses this to apply per-kind styling (green
for money, colored for status, accent for drill). Sort comparators are
also kind-aware.

### Colorblind-safe palette

All colors use the Wong palette with `lipgloss.AdaptiveColor{Light, Dark}`
variants, so the UI works on both dark and light terminal backgrounds. Color
roles are defined in `styles.go`.

## Data flow

```
User keystroke
  -> tea.KeyMsg
  -> Model.Update()
  -> key dispatch (mode-aware)
  -> data mutation (Store CRUD)
  -> reloadAfterMutation() (refreshes effective tab, marks others stale)
  -> Model.View()
  -> rendered string to terminal
```

All data mutations go through the Store, which uses GORM for SQLite access.
After any mutation, `reloadAfterMutation()` refreshes the effective tab and
marks all other tabs as stale. Stale tabs are lazily reloaded when navigated
to. The dashboard is refreshed when it becomes visible.

## Overlays

Dashboard, help, calendar, column finder, extraction progress, and note preview
are rendered as overlays using
[bubbletea-overlay](https://github.com/rmhubbert/bubbletea-overlay). They
composite on top of the live table view with dimmed backgrounds. Overlays can
stack (e.g. help on top of dashboard).
