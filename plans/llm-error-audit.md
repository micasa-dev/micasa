<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# LLM Ping/Model Listing Error Audit

**Issue**: #739

## Findings and fixes

### 1. `cmdSwitchModel` swallowed `ListModels` error (`chat.go`)

`models, _ := client.ListModels(ctx)` discarded errors. For cloud providers
this masked connectivity errors as "model not found"; for local providers it
triggered a doomed pull attempt.

**Fix**: Propagate the error directly instead of discarding it.

### 2. `submitChat` silently dropped input when `llmClient` is nil (`chat.go`)

User's input was consumed but nothing happened — no error, no response.

**Fix**: Show the user's message followed by an error explaining no LLM
is configured.

### 3. `extractionLLMClient` swallowed `NewClient` error (`model.go`)

Configuration errors (bad provider name, etc.) were invisible — extraction
silently skipped LLM with no feedback.

**Fix**: Cache the error in `extractionClientErr`; surface it in the status
bar when `startExtractionOverlay` runs. Clear the cached error when the model
is switched so retry works.

### 4. `Ping()` was a no-op for non-ModelLister providers (`client.go`)

For Anthropic and similar, `Ping()` returned nil unconditionally, so
extraction's early-bail logic never detected misconfigured cloud providers.

**Fix**: Return `ErrPingNotSupported` sentinel. Extraction handler treats it
as "proceed optimistically" (same UX as before, but explicit). Genuine
connectivity errors still skip the LLM step early.

### 5. `no choices in response` was not actionable (`client.go`)

The error message didn't include provider or model info.

**Fix**: Include provider name and model in the message with remediation
guidance.

### 6. Completer silently fell back on ListModels error (`chat.go`)

When the model completer's ListModels call failed, it showed only well-known
models with no indication of failure.

**Fix**: Show a notice for genuine connectivity errors. Suppress the notice
for `ErrModelListingNotSupported` (expected for Anthropic etc.).

## Files changed

- `internal/llm/client.go` — sentinels, Ping behavior, error messages
- `internal/llm/client_test.go` — updated/new tests
- `internal/app/chat.go` — fixes 1, 2, 6
- `internal/app/chat_coverage_test.go` — tests for fixes 1, 2, 6
- `internal/app/extraction.go` — fixes 3, 4
- `internal/app/extraction_test.go` — tests for fixes 3, 4
- `internal/app/model.go` — fix 3 (cached error in extractionLLMClient)
- `internal/app/types.go` — new `extractionClientErr` field
