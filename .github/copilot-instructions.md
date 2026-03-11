<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Copilot Coding Agent Instructions

## Project Overview

**micasa** is a terminal UI (TUI) application for managing home projects,
maintenance schedules, appliances, vendors, incidents, and expenses. It uses a
single SQLite database file with no cloud dependencies.

- **Language**: Go 1.25.5
- **TUI framework**: Charmbracelet Bubble Tea + lipgloss + huh forms
- **ORM**: GORM with pure-Go SQLite (`modernc.org/sqlite`)
- **LLM**: Optional integration via `any-llm-go` (Ollama, OpenAI-compatible)
- **License**: Apache-2.0

## Repository Layout

```
cmd/micasa/          CLI entry point (Kong parser: run, backup, config)
internal/
  app/               TUI layer (~30K lines): model, tabs, tables, forms,
                     overlays, styles, key dispatch, mouse zones
  data/              Persistence: GORM models (14 entities), CRUD, soft-delete,
                     schema introspection, seeding, backup, full-text search
  config/            TOML config with env var overrides (MICASA_*)
  extract/           Document extraction pipeline (text -> OCR -> LLM)
  llm/               LLM client wrapper and SQL prompts
  locale/            Currency formatting
  safeconv/          Safe int64->int narrowing (overflow check)
  fake/              Demo data generation
  ollama/            Ollama model pull API
docs/                Hugo static site (micasa.dev)
nix/                 NixOS module
plans/               Committed design documents
.claude/codebase/    Codebase map (structure.md, types.md, patterns.md)
```

## Building and Testing

### Build

```bash
go build ./cmd/micasa
```

Or with Nix:

```bash
nix build '.#micasa'
```

### Run Tests

```bash
go test -race -shuffle on -timeout 5m ./...
```

- Tests require extraction tools on some platforms (poppler-utils,
  tesseract-ocr, imagemagick). Tests in `internal/extract/ci_test.go` are
  skipped locally if these tools are missing.
- Do not use `-v` flag for normal test runs.
- Always use `-shuffle on` to catch order-dependent bugs.

### Lint

The project uses `golangci-lint` with strict settings (`.golangci.yml`):
exhaustive enum checks, wrapcheck, goconst (min 5 occurrences), type safety
linters, and more.

```bash
golangci-lint run ./...
```

Pre-commit hooks run automatically on `git commit` and include formatting,
import ordering, and license header checks.

### CI

GitHub Actions workflows:

- **ci.yml**: Tests on 5 OS/arch combinations (Ubuntu x86/ARM, macOS, Windows
  x86/ARM) with race detector. Includes benchmarks, Nix build, and docs build.
- **lint.yml**: golangci-lint, deadcode detection, pre-commit hooks.
- **security.yml**: govulncheck, osv-scanner, CodeQL, TruffleHog secrets scan.
- **release.yml**: goreleaser for binary/container releases on GitHub Release.
- **pages.yml**: Hugo site deploy to GitHub Pages.

CI uses change detection (`.github/detect-go-changes.bash`) to skip Go jobs
when only non-Go files changed.

## Coding Conventions

### Go Style

- **Unexported by default**: Only export types/functions/fields when another
  package needs access.
- **Error handling**: No broad catches or silent defaults. Propagate errors
  explicitly. Use `testify/assert` and `testify/require` in tests (`require`
  for preconditions, `assert` for assertions). Prefer `require`/`assert` for
  assertions; avoid bare `t.Fatal`/`t.Error` except for truly unreachable
  branches or specialized test harness helpers.
- **Type safety**: Never cast `int64` to `int` directly. Use
  `safeconv.Int()` from `internal/safeconv` which returns an error on overflow.
- **Enum switches**: Define typed `iota` constants. The `exhaustive` linter
  catches missing cases. Never switch on bare integers.
- **Constants over magic numbers**: Use `math.MaxInt64` or codebase constants.
- **DRY**: Search for existing helpers before creating new ones.

### Architecture

- **Bubble Tea Model-Update-View**: Central `Model` struct dispatches messages
  by type, then by mode. Each entity tab implements the `TabHandler` interface.
- **Rendering pipeline**: `View() -> buildView() -> baseView + overlay stack`
  (dashboard, calendar, notes, finder, extraction, chat, help).
- **Soft delete**: GORM `Delete()` sets `deleted_at` and creates a
  `DeletionRecord` for audit. `checkDependencies()` prevents orphans.
- **Single-file backup**: `micasa backup backup.db` must produce a complete
  backup. Never store app state outside SQLite.
- **LLM is opt-in**: Every feature must work fully without the LLM.
- **Deterministic ordering**: Every `ORDER BY` clause must include a tiebreaker
  (typically `id DESC`).

### UI/UX

- **Styles singleton**: All styles live as private fields on the `Styles`
  struct in `styles.go` with public accessor methods, referenced via the
  package-level `appStyles` singleton (e.g., `appStyles.Money()`). Never
  inline `lipgloss.NewStyle()` in rendering functions.
- **Wong palette**: Colorblind-safe colors using
  `lipgloss.AdaptiveColor{Light, Dark}`. See `styles.go`.
- **Key constants**: All keyboard key strings used in dispatch, `key.WithKeys`,
  `SetKeys`, `helpItem`, and display hints must use constants defined in
  `internal/app/model.go`. No bare key string literals.
- **Mouse support**: Zone-mark all interactive elements with
  `m.zones.Mark(id, content)`. Zone ID conventions: `tab-N`, `row-N`, `col-N`,
  `hint-ID`, `house-header`, `breadcrumb-back`, `dash-N`, `overlay`.
- **Concise labels**: Use the shortest clear label ("drill" not "drilldown",
  "del" not "delete").
- **Actionable errors**: Include failure, likely cause, and remediation step.
- **Silence is success**: No empty-state placeholders or success confirmations.

### Testing

- **User-interaction tests**: Every test for a feature or bug fix must drive
  behavior through simulated user input: keypresses via `sendKey`, form
  submissions via `openAddForm` + `ctrl+s`. Never test only internal APIs or
  set model fields directly.
- **Test observable output**: Assert on rendered UI content, not internal state.
  This was a hard-learned lesson (see `POSTMORTEMS.md`).
- **Test-first**: Write failing tests before implementation for features and
  bug fixes.
- **Test error paths**: Every function that can fail needs at least one test
  exercising that failure.
- **Template DB**: Tests use a pre-migrated template database cloned per test
  for speed. Use `newTestModel(t)`, `newTestModelWithStore(t)`, or
  `newTestModelWithDemoData(t, seed)` to set up test fixtures.
- **Parallel tests**: Use `t.Parallel()` for test parallelization.

### Adding New Entities

Adding a new entity model requires changes across multiple files. Key
touchpoints:

1. GORM model struct in `internal/data/models.go`
2. CRUD methods in `internal/data/store.go`
3. Tab definition and column specs in `internal/app/tables.go`
4. Form definitions in `internal/app/forms.go`
5. TabHandler implementation in `internal/app/handlers.go`
6. Status enum registration (if applicable): `styles.go` switch case,
   `compact.go` labels map, `forms.go` options helper, `models.go` constants

### Adding Status-Like Enums

New status or season-like enum values require updates in four places:

1. `const` block in `internal/data/models.go`
2. `StatusStyle` switch case in `internal/app/styles.go`
3. `statusLabels` map in `internal/app/compact.go`
4. Options helper function in `internal/app/forms.go`

### Configuration

- TOML config file at `$XDG_CONFIG_HOME/micasa/config.toml`
- Environment variable overrides with `MICASA_` prefix
- Config structure: orthogonal sections `[chat]` with `[chat.llm]`,
  `[extraction]` with `[extraction.llm]` and `[extraction.ocr]`
- Defaults are constants in `internal/config/config.go`

## Git and CI Conventions

- **Conventional commits**: Use types like `feat`, `fix`, `docs`, `chore`,
  `refactor`, `test`, `perf`, `ci` with scopes like `app`, `data`, `extract`,
  `config`, `llm`, `docs`, `nix`.
- **Never use `git commit --no-verify`**: Fix all hook failures before
  committing.
- **Treat all linter/compiler warnings as bugs**: Fix all warnings before
  committing.
- **Pin GitHub Actions to commit SHAs**: Use `actions/checkout@<sha> # v6`,
  never `@main` or `@latest`.
- **No `=` in CI go commands**: PowerShell misparses `=`. Use `-bench .` not
  `-bench=.`.
- **Respect native shells**: Don't switch Windows CI steps to `bash`.

## Common Pitfalls

- **Concurrency bugs**: In Bubble Tea's message-passing system, debug by
  tracing what messages are sent, not what flags are set. See `POSTMORTEMS.md`
  for the cancellation bug postmortem.
- **Two-strike rule**: If your second attempt at a bug fix doesn't work, stop
  and re-read the full code path end-to-end. Fix the root cause, not symptoms.
- **CI test matrix**: Tests must pass on Linux, macOS, and Windows. The Windows
  CI uses PowerShell, not bash.
- **Extraction tests**: Some tests in `internal/extract/` require external
  tools (poppler, tesseract, imagemagick) and are skipped if unavailable.

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `charmbracelet/bubbletea` | TUI framework |
| `charmbracelet/lipgloss` | Terminal styling |
| `charmbracelet/huh` | Form components |
| `charmbracelet/bubbles` | Table, textinput, spinner |
| `gorm.io/gorm` | ORM |
| `modernc.org/sqlite` | Pure-Go SQLite |
| `mozilla-ai/any-llm-go` | Multi-provider LLM client |
| `lrstanley/bubblezone` | Mouse zone tracking |
| `stretchr/testify` | Test assertions |
| `alecthomas/kong` | CLI parsing |

## Nix Development Environment

The project uses Nix flakes for reproducible builds and dev environments:

```bash
nix develop              # Enter dev shell with all tools
nix build '.#micasa'     # Build binary
nix run '.#docs'         # Build Hugo docs site
```

The dev shell includes Go, golangci-lint, pre-commit hooks, extraction tools,
and all other development dependencies.
