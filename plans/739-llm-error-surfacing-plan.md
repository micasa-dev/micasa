<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# LLM Error Surfacing Fixes - Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface 3 silently-swallowed LLM errors so users get actionable
feedback when their extraction config is broken, their model server is
unreachable, or the extraction model picker cannot list models.

**Architecture:** Each fix adds a `setStatusError` call at the point where
the error is currently discarded. Finding 1 adds an `extractionClientErr`
field to `extractState` so client creation errors are cached (avoiding
spam on repeated calls) and surfaced once. Findings 4 and 5 add
`setStatusError` calls in the existing `modelsListMsg` error branches.

**Tech Stack:** Go, Bubble Tea, testify

**Spec:** `plans/739-llm-error-surfacing.md`

---

## File Map

| File | Change | Purpose |
|------|--------|---------|
| `internal/app/types.go` | Modify | Add `extractionClientErr error` field to `extractState` |
| `internal/app/model.go` | Modify | `extractionLLMClient()`: cache + return error; callers surface it |
| `internal/app/extraction.go` | Modify | Clear `extractionClientErr` on model switch |
| `internal/app/chat.go` | Modify | `handleModelsListMsg`: add `setStatusError` on completer error path |
| `internal/app/model_update.go` | Modify | Extraction picker: add `setStatusError` on error path |
| `internal/app/extraction_test.go` | Modify | Tests for findings 1 and 5 |
| `internal/app/chat_coverage_test.go` | Modify | Tests for finding 4 |

---

### Task 1: Finding 1 - Surface `extractionLLMClient()` errors

This is the biggest change. `extractionLLMClient()` currently returns `nil`
when client creation fails, with no indication to the user. The fix:

1. Add `extractionClientErr error` to `extractState`
2. When client creation fails, store the error in `extractionClientErr` and
   return `nil` (preserving existing nil-return behavior for callers that
   check `!= nil`)
3. At the two call sites that gate user-visible behavior
   (`afterDocumentSave` and `startExtractionOverlay`), surface the cached
   error via `setStatusError`
4. Clear `extractionClientErr` when the model changes (in
   `switchExtractionModel`) so retries work

**Files:**
- Modify: `internal/app/types.go:94-113` (add field)
- Modify: `internal/app/model.go:877-911` (cache error)
- Modify: `internal/app/model.go:931` (surface error in `afterDocumentSave`)
- Modify: `internal/app/extraction.go:264` (surface error in `startExtractionOverlay`)
- Modify: `internal/app/extraction.go:1236-1238` (clear error on model switch)
- Test: `internal/app/extraction_test.go`

- [ ] **Step 1: Write failing tests**

Add two tests in `internal/app/extraction_test.go`:

```go
// TestExtractionClient_SurfacesCreationError verifies that when the
// extraction provider is invalid, extractionLLMClient() caches the error
// and the status bar shows an actionable message.
func TestExtractionClient_SurfacesCreationError(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	m.ex.extraction.Done = true

	m.ex.extractionProvider = "bogus-provider"
	m.ex.extractionModel = "some-model"
	m.ex.extractionTimeout = 5 * time.Second
	m.ex.extractionEnabled = true

	// Client creation should fail and cache the error.
	assert.Nil(t, m.extractionLLMClient())
	require.Error(t, m.ex.extractionClientErr)
	assert.Contains(t, m.ex.extractionClientErr.Error(), "bogus-provider")
}

// TestExtractionClient_ClearsErrorOnModelSwitch verifies that switching
// models clears the cached client creation error so retries work.
func TestExtractionClient_ClearsErrorOnModelSwitch(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	m.ex.extraction.Done = true

	// Force a cached error.
	m.ex.extractionProvider = "bogus-provider"
	m.ex.extractionModel = "some-model"
	m.ex.extractionTimeout = 5 * time.Second
	m.ex.extractionEnabled = true
	m.extractionLLMClient()
	require.Error(t, m.ex.extractionClientErr)

	// Switching model should clear the error.
	m.switchExtractionModel("new-model", true)
	assert.NoError(t, m.ex.extractionClientErr)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestExtractionClient_SurfacesCreationError -shuffle=on ./internal/app/`
Expected: FAIL -- `extractionClientErr` field does not exist.

- [ ] **Step 3: Add `extractionClientErr` field to `extractState`**

In `internal/app/types.go`, add `extractionClientErr` to the `extractState`
struct, after the `extractionClient` field:

```go
extractionClient   llm.ExtractionProvider
extractionClientErr error // cached client creation error (finding 1)
```

- [ ] **Step 4: Cache errors in `extractionLLMClient()`**

In `internal/app/model.go`, modify `extractionLLMClient()` to cache errors:

```go
func (m *Model) extractionLLMClient() llm.ExtractionProvider {
	if m.ex.extractionClient != nil {
		return m.ex.extractionClient
	}

	// Return early if we already tried and failed.
	if m.ex.extractionClientErr != nil {
		return nil
	}

	provider := m.ex.extractionProvider
	baseURL := m.ex.extractionBaseURL
	apiKey := m.ex.extractionAPIKey
	timeout := m.ex.extractionTimeout
	model := m.ex.extractionModel

	if model == "" {
		return nil
	}

	var client llm.ExtractionProvider
	if provider == "claude-cli" {
		cc, err := claudecli.NewClient(model, timeout)
		if err != nil {
			m.ex.extractionClientErr = err
			return nil
		}
		client = cc
	} else {
		cc, err := llm.NewClient(provider, baseURL, model, apiKey, timeout)
		if err != nil {
			m.ex.extractionClientErr = err
			return nil
		}
		client = cc
	}
	if m.ex.extractionEffort != "" {
		client.SetEffort(m.ex.extractionEffort)
	}
	m.ex.extractionClient = client
	return client
}
```

- [ ] **Step 5: Surface error in `afterDocumentSave`**

In `internal/app/model.go`, after the `extractionLLMClient()` call on the
line that computes `llmReady`, add:

```go
llmReady := m.ex.extractionEnabled && m.extractionLLMClient() != nil && m.ex.extractionReady
if m.ex.extractionEnabled && m.ex.extractionClientErr != nil {
	m.setStatusError("extraction LLM: " + m.ex.extractionClientErr.Error())
}
```

The `extractionEnabled` guard prevents surfacing errors when the user has
not opted into LLM extraction. Without this guard, a stale cached error
from a previous config could fire on every document save even with
extraction disabled.

- [ ] **Step 6: Surface error in `startExtractionOverlay`**

In `internal/app/extraction.go`, after `needsLLM := m.extractionLLMClient() != nil`,
add:

```go
if m.ex.extractionEnabled && m.ex.extractionClientErr != nil {
	m.setStatusError("extraction LLM: " + m.ex.extractionClientErr.Error())
}
```

Same `extractionEnabled` guard as `afterDocumentSave`.

- [ ] **Step 7: Clear cached error on model switch**

In `internal/app/extraction.go` `switchExtractionModel`, after
`m.ex.extractionClient = nil`, add:

```go
m.ex.extractionClientErr = nil
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test -run 'TestExtractionClient_SurfacesCreationError|TestExtractionClient_ClearsErrorOnModelSwitch' -shuffle=on ./internal/app/`
Expected: PASS

- [ ] **Step 9: Commit**

Use `/commit`.

---

### Task 2: Finding 4 - Surface completer `ListModels` error in status bar

When the chat completer receives a `modelsListMsg` with an error, it shows
well-known models but gives no indication the server was unreachable.
Add `setStatusError` so the user knows.

**Files:**
- Modify: `internal/app/chat.go:462-469`
- Test: `internal/app/chat_coverage_test.go`

- [ ] **Step 1: Write failing test**

Modify `TestHandleModelsListMsgCompleterError` in
`internal/app/chat_coverage_test.go` to also assert a status error:

```go
func TestHandleModelsListMsgCompleterError(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.openChat()
	m.chat.Input.SetValue("/model ")
	m.chat.Completer = &modelCompleter{Loading: true}

	m.handleModelsListMsg(modelsListMsg{Err: errors.New("oops")})
	assert.False(t, m.chat.Completer.Loading)
	// Should fall back to well-known models only.
	require.NotEmpty(t, m.chat.Completer.All)
	// Should surface error in status bar.
	assert.Equal(t, statusError, m.status.Kind)
	assert.Contains(t, m.status.Text, "oops")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestHandleModelsListMsgCompleterError -shuffle=on ./internal/app/`
Expected: FAIL -- `m.status.Kind` is not `statusError`.

- [ ] **Step 3: Add `setStatusError` in completer error path**

In `internal/app/chat.go`, in `handleModelsListMsg`, after the
`mc.All = mergeModelLists(nil)` line in the error branch, add:

```go
m.setStatusError("list models: " + msg.Err.Error())
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestHandleModelsListMsgCompleterError -shuffle=on ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

Use `/commit`.

---

### Task 3: Finding 5 - Surface extraction model picker `ListModels` error

Same pattern as finding 4, but in the extraction model picker path in
`model_update.go`.

**Files:**
- Modify: `internal/app/model_update.go:91-102`
- Test: `internal/app/extraction_test.go`

- [ ] **Step 1: Write failing test**

Add test in `internal/app/extraction_test.go`:

```go
// TestExtractionModelPicker_SurfacesListModelsError verifies that when
// the extraction model picker receives a ListModels error, the status bar
// shows an actionable error message.
func TestExtractionModelPicker_SurfacesListModelsError(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Set up a model picker in loading state.
	ex.modelPicker = &modelCompleter{Loading: true}

	// Deliver a modelsListMsg with an error.
	m.Update(modelsListMsg{Err: errors.New("connection refused")})

	// Picker should be populated with well-known models.
	assert.False(t, ex.modelPicker.Loading)
	require.NotEmpty(t, ex.modelPicker.All)

	// Status bar should show an error.
	assert.Equal(t, statusError, m.status.Kind)
	assert.Contains(t, m.status.Text, "connection refused")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestExtractionModelPicker_SurfacesListModelsError -shuffle=on ./internal/app/`
Expected: FAIL -- `m.status.Kind` is not `statusError`.

- [ ] **Step 3: Add `setStatusError` in extraction picker error path**

In `internal/app/model_update.go`, in the `modelsListMsg` case, after
`ex.modelPicker.All = mergeModelLists(nil)`, add:

```go
m.setStatusError("list models: " + typed.Err.Error())
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestExtractionModelPicker_SurfacesListModelsError -shuffle=on ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

Use `/commit`.
