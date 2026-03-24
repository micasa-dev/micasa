<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Context Lifecycle Audit

Closes #803.

## Problem

Many TUI operations (LLM extraction, chat inference, model pulls, postal
code lookups) use `context.Background()` directly. When the user quits
the app, these goroutines continue running until their HTTP timeouts
expire because nothing cancels them. The sync engine already has a
dedicated `syncCtx`, but other features lack a parent context tied to
the app lifecycle.

## Design

Add an `appCtx` / `appCancel` pair to the Model struct. All feature
contexts derive from `appCtx` instead of `context.Background()`.
Quitting the app cancels `appCtx`, which cascades to every in-flight
operation.

```
context.Background()
  └── appCtx  (cancelled on Ctrl+Q)
        ├── syncCtx  (Pro sync engine)
        ├── extraction WithCancel  (per-extraction, user-cancellable)
        ├── chat WithTimeout  (per-inference, timeout-scoped)
        ├── pull WithCancel  (model download, user-cancellable)
        └── postal code WithTimeout  (address lookup)
```

Per-feature cancellation (cancel one extraction, stop a pull) continues
to work because each feature creates a sub-context from `appCtx`. The
difference: `appCtx` cancellation is the safety net that catches
everything on quit.

Library functions that currently embed `context.Background()` get a
`ctx context.Context` parameter so callers can thread the app context
through.

## Changes

### 1. Model struct (`internal/app/model.go`)

- Add `appCtx context.Context` and `appCancel context.CancelFunc` fields.
- In `NewModel`: `model.appCtx, model.appCancel = context.WithCancel(context.Background())`.
- Change `syncCtx` creation (line 319) from
  `context.WithCancel(context.Background())` to
  `context.WithCancel(model.appCtx)`.
- Line 696: replace `context.Background()` fallback with `m.appCtx`.
- Line 2148: replace `context.Background()` with `m.appCtx`.
- Line 3218: replace `context.Background()` with `m.appCtx`.

### 2. Quit handlers (`internal/app/model.go`)

**Primary quit** (lines 384-390): add `m.appCancel()` first.

```go
m.appCancel()  // cascades to all child contexts
m.cancelChatOperations()
m.cancelAllExtractions()
m.cancelPull()
if m.syncCancel != nil { m.syncCancel() }
return m, tea.Quit
```

**Confirm-discard quit** (lines 727-731 in `handleConfirmDiscard`):
same treatment — add `m.appCancel()` before existing cancels.

```go
case keyY:
    if m.confirm == confirmFormQuitDiscard {
        m.confirm = confirmNone
        m.appCancel()
        m.cancelChatOperations()
        m.cancelPull()
        return m, tea.Quit
    }
```

Per-feature cancels remain for state cleanup (clearing UI flags, etc.)
even though they're now redundant for context cancellation.

### 3. Extraction (`internal/app/extraction.go`)

Replace 5 `context.Background()` calls:

- Line 282: `context.WithCancel(context.Background())` ->
  `context.WithCancel(m.appCtx)` (extraction start)
- Line 513: `context.WithTimeout(context.Background(), ...)` ->
  `context.WithTimeout(m.appCtx, ...)` (LLM ping check)
- Line 993: `context.WithCancel(context.Background())` ->
  `context.WithCancel(m.appCtx)` (extraction rerun)
- Line 1176: `context.WithTimeout(context.Background(), ...)` ->
  `context.WithTimeout(m.appCtx, ...)` (list models)
- Line 1259: `context.WithTimeout(context.Background(), ...)` ->
  `context.WithTimeout(m.appCtx, ...)` (verify model)

Check whether these are methods on `*Model` (direct access to
`m.appCtx`) or standalone `tea.Cmd` functions that need `appCtx`
passed as a parameter.

### 4. Chat (`internal/app/chat.go`)

Replace 7 `context.Background()` calls:

- Line 363: SQL generation streaming context
- Line 439: list chat models
- Line 520: reactivate model completer
- Line 644: verify switched model
- Line 670: start model pull
- Line 779: summarize extraction results
- Line 804: fallback chat response

Same pattern as extraction — check method vs standalone function.

### 5. Library APIs

**`extract.ExtractText`** (`internal/extract/text.go:29`): Add `ctx
context.Context` as first parameter. The `Extractor` interface already
accepts `ctx` — the fix is that `ExtractText` passes `ctx` through
instead of `context.Background()`.

Callers:
- `internal/app/forms.go:2575`

**`Store.ReadOnlyQuery`** (`internal/data/query.go:96`): Add `ctx
context.Context` as first parameter. The existing timeout (line 131)
becomes `context.WithTimeout(ctx, readOnlyQueryTimeout)`.

Callers:
- `internal/app/chat.go:1006`

**`Config.Query`** (`internal/config/query.go:30`): Add `ctx
context.Context` as first parameter. The existing timeout (line 56)
becomes `context.WithTimeout(ctx, queryTimeout)`.

Callers:
- `cmd/micasa/main.go:436` (CLI handler — pass
  `context.Background()` or a signal context)

### 6. Allowlist after completion

After all changes, `context.Background()` in production (non-test)
code should only appear at these roots:

- `internal/app/model.go` NewModel: single `appCtx` root
- `cmd/micasa/pro.go`: 8 `signal.NotifyContext` roots in CLI handlers
- `cmd/micasa/main.go`: `signal.NotifyContext` for backup, editor
  exec `CommandContext`, config query CLI handler
- `cmd/relay/main.go`: graceful shutdown timeout
- `internal/data/sqlite/sqlite.go`: one-shot version check at init

## Verification

1. `go build ./...` -- no compilation errors.
2. `go test -shuffle=on ./internal/app/... ./internal/data/...
   ./internal/extract/... ./internal/config/... ./cmd/...` -- all pass.
3. `go vet ./...` -- clean.
4. Grep for `context.Background()` in production code (excluding
   `_test.go` and `plans/`). Compare against the allowlist above.
5. Manual smoke test: launch TUI, start an LLM extraction, press
   Ctrl+Q mid-extraction. The process should exit promptly without
   waiting for the HTTP timeout.
