<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Audit: LLM Ping/Model Listing Error Surfacing

GitHub issue: #739

## Scope

Trace every code path where `Ping()` and `ListModels()` are called from the
TUI layer and verify that errors propagate to a user-visible surface (chat
message, status bar, extraction overlay log). Catalog swallowed errors and
propose fixes.

## Findings

### 1. `extractionLLMClient()` silently swallows client creation errors

**File:** `internal/app/model.go:893-904`

When `claudecli.NewClient` or `llm.NewClient` returns an error,
`extractionLLMClient()` returns `nil` with no logging or status message. The
caller sees "no LLM client" and silently skips the LLM step. The user has no
idea why extraction isn't using the LLM -- could be a misconfigured provider
name, missing API key, or `claude` binary not on PATH.

**Current behavior:** Silent degradation. LLM step never appears in the
extraction overlay.

**Desired behavior:** Surface the client creation error via `setStatusError`
so the user knows their extraction LLM config is broken and how to fix it.
Cache the error to avoid spamming on every call.

### 2. `cmdSwitchModel` discards `ListModels` error on line chat.go:657

**File:** `internal/app/chat.go:657`

```go
models, _ := client.ListModels(ctx) // best-effort, like chat path
```

When checking whether a model is already available before pulling, a
`ListModels` error is discarded. If the server is unreachable, this silently
falls through to a pull attempt, which may also fail with a different error.

**Current behavior:** Falls through to pull attempt. If the server is down,
the pull error surfaces ("cannot reach ollama -- start it with `ollama serve`").

**Verdict:** Acceptable. The pull attempt's error is more actionable than a
redundant ListModels error. The comment already documents intent. No change
needed.

### 3. `switchExtractionModel` discards `ListModels` error on extraction.go:1271

**File:** `internal/app/extraction.go:1271`

```go
models, _ := client.ListModels(ctx) // best-effort, like chat path
```

Same pattern as finding 2. Falls through to pull attempt for local servers,
or returns "model not found" for cloud providers. The downstream errors are
adequate.

**Verdict:** Acceptable. Same reasoning as finding 2. No change needed.

### 4. `handleModelsListMsg` silently degrades completer on error

**File:** `internal/app/chat.go:463-469`

When the completer is active and `ListModels` returns an error, the completer
shows only well-known models without any indication that the server was
unreachable.

**Current behavior:** Completer shows well-known models only. No error
indication.

**Desired behavior:** Show well-known models (current behavior is fine for
usability) but also surface a status bar error (`setStatusError`) indicating
the server was unreachable. A chat notice would interfere with the active
completer; the status bar is visible regardless.

### 5. Extraction model picker silently degrades on error

**File:** `internal/app/model_update.go:93-101`

When the extraction overlay's model picker receives a `modelsListMsg` with
an error, it silently shows only well-known models without telling the user
the server is unreachable.

**Current behavior:** Shows well-known models. No error indication.

**Desired behavior:** Same as finding 4: show well-known models but surface
a status bar error (`setStatusError`) indicating the server was unreachable.

### 6. `autoDetectModel` silently returns "" on error

**File:** `internal/app/model.go:1893-1902`

When `ListModels` fails during startup auto-detection, the function returns
"" and the caller falls through to use the config model. This is at startup
time only and the config model is always available as fallback.

**Verdict:** Acceptable. Auto-detection is a convenience feature. The
configured model is used as fallback. No change needed.

### 7. Extraction LLM ping error surfaces correctly

**File:** `internal/app/extraction.go:714-738`

The `handleExtractionLLMPing` handler correctly marks the LLM step as
`stepSkipped`, stores the error in the step's logs, and renders a
strikethrough in the UI. The error message from `wrapError` is actionable.

**Verdict:** Working correctly. No change needed.

### 8. Chat `/models` command surfaces errors correctly

**File:** `internal/app/chat.go:474-480`

When `/models` fails, the error is rendered as a `roleError` chat message.
The error comes from `wrapError` which produces actionable messages.

**Verdict:** Working correctly. No change needed.

### 9. `checkExtractionModelCmd` surfaces errors via `pullProgressMsg`

**File:** `internal/app/model.go:750-759`

ListModels errors are wrapped and delivered as `pullProgressMsg{Err: ...}`.
The `handlePullProgress` handler shows these via `setStatusError`.

**Verdict:** Working correctly. No change needed.

### 10. Chat `Ping()` is never called proactively

The issue asks whether `Ping()` should run proactively when any LLM feature
is first used. Currently:

- **Chat:** No ping before first query. If the server is down, the
  `ChatStream` call fails and the error surfaces via `sqlStreamStartedMsg.Err`
  → `handleSQLStreamStarted` → `replaceAssistantWithError`.

- **Extraction:** Ping runs concurrently with OCR (good). If extraction
  starts without OCR, there is no proactive ping -- the `ExtractStream` call
  surfaces errors via `extractionLLMChunkMsg.Err`.

Connection-refused errors surface almost immediately (TCP RST). Timeouts
surface at the configured timeout. The `wrapError` function already produces
actionable messages for both cases. Adding a proactive ping would introduce a
new message type, a race between ping result and stream start, and new state
management -- significant complexity for marginal UX gain.

**Verdict:** Not worth the complexity. The existing error paths surface
actionable messages within acceptable latency. No change needed.

### 11. claudecli `Ping()` is a no-op

**File:** `internal/claudecli/client.go:90`

```go
func (c *Client) Ping(_ context.Context) error { return nil }
```

This means extraction with `claude-cli` never detects an unreachable binary
until the actual `ExtractStream` call. Since `claude-cli` validates the binary
exists at `NewClient` time via `exec.LookPath`, this is acceptable -- the
binary either exists or client creation fails (finding 1).

**Verdict:** Acceptable. No change needed.

## Summary of required changes

| # | Finding | File | Action |
|---|---------|------|--------|
| 1 | `extractionLLMClient` swallows errors | `model.go` | Surface error via `setStatusError`, cache to avoid spam |
| 4 | Completer degrades silently on error | `chat.go` | Add chat notice when server unreachable |
| 5 | Extraction picker degrades silently | `model_update.go` | Add status message when server unreachable |

## Design decisions

### `extractionLLMClient` error caching (finding 1)

Add an `extractionClientErr error` field to `extractState`. When client
creation fails, store the error. Return it on the first call that triggers
client creation. Clear it when the user changes the model (which retries
creation). Surface via `setStatusError` at the call sites that would
otherwise silently skip LLM: `afterDocumentSave` and
`startExtractionOverlay`.
