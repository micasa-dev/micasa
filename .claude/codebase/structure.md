<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->
<!-- verified: 2026-05-21 -->

# Project Structure

~80K lines of Go. Bubble Tea TUI for home maintenance tracking with optional
multi-device sync via an encrypted relay server. SQLite backend (TUI),
Postgres backend (relay).

## Directory Layout

```
cmd/micasa/                 CLI entry points (cobra + fang)
  main.go                   Root cmd, runOpts/demoOpts/backupOpts
  query.go, show.go         Read-only inspection subcommands
  mcp.go                    Launch MCP server subcommand
  pro.go                    Pro/subscription entitlement
  sync_config.go            Sync configuration
  theme.go                  TUI theme application
  cliref.go                 CLI reference generation

internal/
  app/                      TUI package (~30K lines, largest package)
    model.go                Model struct, Init/Update/View, key dispatch
    types.go                Enums (Mode, FormKind, TabKind), Tab/cell/columnSpec
    handlers.go             TabHandler interface + per-entity implementations
    view.go                 Render pipeline: buildView -> baseView + overlays
    table.go                Table rendering, viewport, header/row rendering
    coldefs.go              Column definitions: single source of truth
    columns_generated.go    Generated typed iota column constants
    tables.go               Tab definitions, row builders
    forms.go                Form definitions per entity, submitForm flow
    form_select.go          Select/dropdown form fields
    form_filepicker.go      File picker for document uploads
    styles.go               Styles struct (singleton appStyles), Wong palette
    chat.go                 LLM chat overlay, NL->SQL->summary
    extraction.go           Document extraction UI, step tracking
    extraction_render.go    Extraction UI rendering
    dashboard.go            Dashboard overlay, metrics, nav entries
    calendar.go             Modal date picker
    filter.go               Pin-and-filter system (AND across cols, OR within)
    sort.go                 Multi-column sort with tiebreaker
    column_finder.go        Fuzzy column search (/ key)
    fuzzy.go                Generic fuzzy matching
    mouse.go                Zone-based click dispatch
    house.go, house_fields.go    House profile view + field metadata
    undo.go                 Undo/redo stacks (max 50, snapshot-based)
    mag.go                  Order-of-magnitude easter egg (m key)
    compact.go              Column width optimization
    collapse.go             Column hiding
    stream.go               Streaming utilities
    docopen.go              Open documents in external viewer
    chat_render.go          Chat overlay rendering
    notes.go                Notes preview overlay
    detail.go               Detail view drill-down
    inline_edit.go          Inline cell editing dispatch
    model_tabs.go           Tab construction
    cmd/gencolumns/         Code generator for columns_generated.go

  data/                     Persistence layer (SQLite)
    store.go                Store struct (gorm.DB), open/close/migrate
    store_*.go              Per-entity CRUD (vendor, appliance, project, ...)
    store_seed.go           Seed lookups
    store_hard_delete.go    Hard-delete operations (compaction)
    models.go               GORM entity structs (16 models incl. SyncOplogEntry, SyncDevice)
    oplog.go                Sync oplog: hooks, syncableTable, WithSyncApplying
    fts.go                  Full-text search (SQLite FTS5)
    defaults.go             Default-tag reflection helper (StructDefault)
    query.go                Schema inspection, ReadOnlyQuery, DataDump
    backup.go               SQLite Online Backup API
    dashboard.go            Dashboard queries (overdue, upcoming, spending)
    validation.go           Date/int/float/interval parsing
    settings.go             Key-value store, chat history
    errors.go               hintError, FieldError, sentinels
    meta.go, meta_generated.go    Table*/Col* constants (generated)
    entity_context.go       Entity names for LLM
    entity_rows.go          (id, name) tuples for LLM FK resolution
    doccache.go             Document cache (XDG_CACHE_HOME)
    path.go                 DB path resolution, home expansion
    units.go                UnitSystem (metric/imperial)
    seed_scaled.go          Scaled demo data (N years)
    ddl.go                  Table DDL retrieval
    migrate.go              Migration orchestration
    sqlite/                 Custom SQLite dialect (inlined from glebarez/sqlite)
      sqlite.go             Dialect, PRAGMA connector, type mapping
      ddlmod.go             DDL parsing & manipulation
      migrator.go           GORM Migrator override
    cmd/genmeta/            Code generator for meta_generated.go

  config/                   TOML config with env var overrides
    config.go               Config struct, Load(), provider auto-detect
    bytesize.go             ByteSize custom type ("50 MiB")
    duration.go             Duration custom type ("30d")
    show.go                 Config display/dump
    query.go                Config inspection helpers
    cmd/                    Config-related code generators

  extract/                  Document extraction pipeline
    extractor.go            Extractor interface, PDFTextExtractor, PDFOCRExtractor
    pipeline.go             Pipeline orchestration (text -> OCR -> LLM)
    text.go                 pdftotext extraction
    ocr.go                  tesseract OCR (parallel image acquisition)
    ocr_progress.go         OCR progress tracking (toolPDFToCairo, toolTesseract)
    llmextract.go           LLM-powered structured extraction
    operations.go           INSERT/UPDATE/DELETE operation types
    shadow.go               Shadow DB for staging
    sqlcontext.go           ExtractionTableDefs: schema for LLM
    tools.go                Tool availability checks (pdftocairo, tesseract, pdftotext)

  llm/                      LLM interfaces and any-llm-go client
    provider.go             Base, ChatProvider, ExtractionProvider interfaces
    client.go               Client wrapping any-llm-go; provider constants
    prompt.go               System prompts (SQL gen, summary, fallback)

  claudecli/                Claude CLI subprocess backend
    client.go               Client implementing ExtractionProvider via claude binary

  sync/                     Multi-device sync (encrypted oplog)
    types.go                Envelope, PushRequest, Household, BlobStorage, ...
    client.go               Client: HTTP push/pull against relay
    engine.go               Engine.Sync orchestrates push+pull+blob transfer
    apply.go                ApplyOps with LWW conflict resolution
    household.go            Household creation, key exchange flow
    blob.go                 Blob (binary) upload/download

  relay/                    Relay server (Postgres backend, encrypted ops)
    handler.go              HTTP handlers (push, pull, invite, blob, ...)
    store.go                Store interface (24 methods); constants
    pgstore.go              Postgres implementation; lockForUpdate const
    memstore.go             In-memory implementation (tests)
    blob.go                 Blob storage abstraction
    tokencrypt.go           Token encryption for invite codes
    stripe.go               Stripe webhook handling
    rlsdb/                  Row-level security DB wrapper
                            DB.Tx(ctx, householdID, fn) enforces RLS

  crypto/                   Cryptographic primitives
    keys.go                 HouseholdKey, DeviceKeyPair, key persistence
    encrypt.go              Encrypt/Decrypt (NaCl secretbox + key derivation)
    box.go                  BoxSeal/BoxOpen (NaCl box for key exchange)
    token.go                Device bearer token persistence

  mcp/                      MCP (Model Context Protocol) server
    server.go               Server struct, stdio JSON-RPC loop
    tools.go                MCP tools (query, get_schema, search_documents, ...)

  fake/                     Demo data generator
    fake.go                 HomeFaker (seeded gofakeit)
    words.go                Word lists

  ollama/                   Ollama model pull API
    pull.go                 PullModel(), PullScanner (streaming)

  locale/                   Currency formatting
    currency.go             Currency type, FormatMoney, ParseMoney

  sqlfmt/                   SQL pretty-printer (extracted from llm)
    sqlfmt.go               FormatSQL: tokenizer + layout + wrap

  address/                  Postal code -> city/state lookup
    lookup.go               Lookup() against zippopotam.us

  uid/                      ULID wrapper
    uid.go                  New(), IsValid() (oklog/ulid v2)

  safeconv/                 Safe int64->int narrowing
    narrow.go               Int() with overflow check
```

## Build & CI

- `flake.nix` - Nix build (buildGoModule), dev shell, pre-commit hooks
- `nix/module.nix` - NixOS module
- `.github/workflows/ci.yml` - Multi-OS matrix (Ubuntu x86/ARM, macOS, Windows)
- `.golangci.yml` - Linter config: `default: all` with errorlint strict
  (errorf + errorf-multi + comparison + asserts), goconst/gomodguard
  disabled
- `go.mod` - Go 1.25.5; deps include bubbletea v2 / lipgloss v2 / huh v2,
  gorm + modernc sqlite, any-llm-go, modelcontextprotocol/go-sdk
- `docs/` - Hugo site (guides, reference, blog)
- `plans/` - Design documents (committed to repo)

## Key Dependencies

- `charm.land/bubbletea/v2` - TUI framework (v2 series)
- `charm.land/lipgloss/v2` - Styling
- `charm.land/huh/v2` - Form components
- `charm.land/bubbles/v2` - Table, viewport, textinput, spinner
- `charm.land/fang/v2` - cobra wrapper with styling
- `gorm.io/gorm` + `modernc.org/sqlite` - Pure Go SQLite
- `gorm.io/driver/postgres` - Postgres driver (relay only)
- `mozilla-ai/any-llm-go` - Multi-provider LLM client
- `modelcontextprotocol/go-sdk` - MCP server
- `lrstanley/bubblezone/v2` - Mouse zone tracking
- `oklog/ulid/v2` - ULIDs
- `brianvoe/gofakeit` - Random data generation
- `spf13/cobra` - CLI parsing
- `stretchr/testify` - Test assertions
- `stripe/stripe-go` - Stripe webhooks (relay)
