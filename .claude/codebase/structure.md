<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->
<!-- verified: 2026-03-07 -->

# Project Structure

~65K lines of Go. Bubble Tea TUI for home maintenance tracking. SQLite backend.

## Directory Layout

```
cmd/micasa/main.go          CLI entry (cobra). runOpts, backupOpts, newRootCmd
internal/
  app/                      TUI package (~30K lines, largest package)
    model.go                Model struct, Init/Update/View, key dispatch
    types.go                Enums (Mode, FormKind, TabKind), Tab/cell/columnSpec structs
    handlers.go             TabHandler interface + per-entity implementations
    view.go                 Render pipeline: buildView -> baseView + overlays
    table.go                Table rendering, viewport, header/row rendering
    coldefs.go              Column definitions: single source of truth (columnDef slices + columnSpecs funcs)
    columns_generated.go    Generated typed iota column constants (from coldefs.go via gencolumns)
    tables.go               Tab definitions, row builders, table helpers
    forms.go                Form definitions per entity, submitForm flow
    form_select.go          Select/dropdown form fields
    form_filepicker.go      File picker for document uploads
    styles.go               Styles struct (singleton appStyles), Wong palette
    chat.go                 LLM chat overlay, two-stage NL->SQL->summary
    extraction.go           Document extraction UI, step tracking
    dashboard.go            Dashboard overlay, metrics, nav entries
    calendar.go             Modal date picker
    filter.go               Pin-and-filter system (AND across cols, OR within)
    sort.go                 Multi-column sort with tiebreaker
    column_finder.go        Fuzzy column search (/ key)
    fuzzy.go                Generic fuzzy matching
    mouse.go                Zone-based click dispatch
    house.go                House profile view (collapsed/expanded + ASCII art)
    undo.go                 Undo/redo stacks (max 50, snapshot-based)
    mag.go                  Order-of-magnitude easter egg (m key)
    compact.go              Column width optimization
    collapse.go             Column hiding
    stream.go               Streaming utilities
    docopen.go              Open documents in external viewer
    testmain_test.go        Global test setup, template DB
    model_with_store_test.go    newTestModelWithStore(t)
    model_with_demo_data_test.go    newTestModelWithDemoData(t, seed)
  data/                     Persistence layer
    store.go                Store struct (gorm.DB), all CRUD, soft-delete, seeding
    models.go               GORM entity structs (14 models)
    query.go                Schema inspection, ReadOnlyQuery, DataDump for LLM
    backup.go               SQLite Online Backup API
    dashboard.go            Dashboard queries (overdue, upcoming, spending)
    validation.go           Date/int/float/interval parsing
    settings.go             Key-value store, chat history
    errors.go               hintError, FieldError, sentinels
    meta.go                 go:generate directive
    meta_generated.go       Table*/Col* constants (generated)
    entity_context.go       Entity names for LLM
    entity_rows.go          (id, name) tuples for LLM FK resolution
    doccache.go             Document cache (XDG_CACHE_HOME)
    path.go                 DB path resolution, home expansion
    units.go                UnitSystem (metric/imperial)
    seed_scaled.go          Scaled demo data (N years)
    ddl.go                  Table DDL retrieval
    sqlite/                 Custom SQLite dialect (inlined from glebarez/sqlite)
      sqlite.go             Dialect, PRAGMA connector, type mapping
      ddlmod.go             DDL parsing & manipulation
      migrator.go           GORM Migrator override (table recreation for ALTER)
    cmd/genmeta/main.go     Code generator for meta_generated.go
  app/cmd/gencolumns/main.go  Code generator for columns_generated.go (from coldefs.go)
  config/                   TOML config with env var overrides
    config.go               Config struct, Load(), provider auto-detect
    bytesize.go             ByteSize custom type ("50 MiB")
    duration.go             Duration custom type ("30d")
    show.go                 Config display/dump
  extract/                  Document extraction pipeline
    extractor.go            Extractor interface
    pipeline.go             Pipeline orchestration (text -> OCR -> LLM)
    text.go                 pdftotext extraction
    ocr.go                  tesseract OCR (parallel image acquisition)
    llmextract.go           LLM-powered structured extraction
    operations.go           INSERT/UPDATE/DELETE operation types
    shadow.go               Shadow DB for staging
    sqlcontext.go           Schema context for LLM
    tools.go                Tool availability checks
    ocr_progress.go         OCR progress tracking
  llm/                      LLM client
    client.go               Client wrapping any-llm-go
    prompt.go               System prompts (SQL gen, summary, fallback)
    sqlfmt.go               SQL formatting
  fake/                     Demo data generator
    fake.go                 HomeFaker (seeded gofakeit)
    words.go                Word lists
  ollama/                   Ollama model pull API
    pull.go                 PullModel(), PullScanner (streaming)
  locale/                   Currency formatting
    currency.go             Currency type, FormatMoney, ParseMoney
  safeconv/                 Safe int64->int narrowing
    narrow.go               Int() with overflow check
```

## Build & CI

- `flake.nix` - Nix build (buildGoModule), dev shell, pre-commit hooks
- `nix/module.nix` - NixOS module
- `.github/workflows/ci.yml` - Multi-OS matrix (Ubuntu x86/ARM, macOS, Windows)
- `.golangci.yml` - Linter config (exhaustive, wrapcheck, goconst min 5, etc.)
- `go.mod` - Go 1.25.5, key deps: bubbletea/lipgloss/huh, gorm+modernc sqlite, any-llm-go
- `docs/` - Hugo site (guides, reference, blog)
- `plans/` - Design documents (committed to repo)

## Key Dependencies

- `charmbracelet/bubbletea` - TUI framework
- `charmbracelet/lipgloss` - Styling
- `charmbracelet/huh` - Form components
- `charmbracelet/bubbles` - Table, viewport, textinput, spinner
- `gorm.io/gorm` + `modernc.org/sqlite` - Pure Go SQLite
- `mozilla-ai/any-llm-go` - Multi-provider LLM client
- `lrstanley/bubblezone` - Mouse zone tracking
- `rmhubbert/bubbletea-overlay` - Overlay compositing
- `brianvoe/gofakeit` - Random data generation
- `spf13/cobra` - CLI parsing
- `stretchr/testify` - Test assertions
