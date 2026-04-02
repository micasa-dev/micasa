<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Claude CLI Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `claude-cli` as a new LLM provider that shells out to the `claude` binary, starting with extraction support.

**Architecture:** Extract `llm.Provider` interface from `llm.Client`, decouple `ChatOption` from `any-llm-go` types, add `internal/claudecli/` package that implements `Provider` via subprocess execution, wire into config and app construction.

**Tech Stack:** Go stdlib (`os/exec`, `encoding/json`), existing `internal/llm` types, `claude` CLI binary.

**Spec:** `plans/claude-cli-backend.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/llm/provider.go` (create) | `Provider` interface definition |
| `internal/llm/options.go` (create) | `ExtractJSONSchema` accessor, `jsonSchemaParam` type |
| `internal/llm/client.go` (modify) | Replace `chatParams.responseFormat` with `jsonSchemaParam`, update `completionParams()` |
| `internal/claudecli/client.go` (create) | `Client` struct, `NewClient`, `ChatComplete`, `ChatStream`, trivial method stubs |
| `internal/claudecli/client_test.go` (create) | Unit tests with `TestHelperProcess` mock binary |
| `internal/claudecli/testdata/` (create) | Captured real CLI JSON/stream-json fixtures |
| `internal/config/config.go` (modify) | Add `"claude-cli"` to providers, validation |
| `internal/config/config_test.go` (modify) | Test claude-cli config validation |
| `internal/app/model.go` (modify) | `llmClient` type change, construction branching |
| `internal/app/types.go` (modify) | `extractionClient` type change |
| `internal/extract/pipeline.go` (modify) | `LLMClient` type change |

---

### Task 1: Capture CLI test fixtures

**Files:**
- Create: `internal/claudecli/testdata/complete.json`
- Create: `internal/claudecli/testdata/complete_schema.json`
- Create: `internal/claudecli/testdata/stream.jsonl`
- Create: `internal/claudecli/testdata/stream_schema.jsonl`

These fixtures are captured from real `claude -p` invocations and committed as the source of truth for parser tests. **This is a manual local step** -- requires a `claude` binary with valid auth. Run before automated tasks begin.

**Sanitize before committing:** strip volatile metadata (session IDs, UUIDs, cost/usage, timestamps) and any account-identifiable fields. Keep only the fields the parser consumes (`type`, `result`, `structured_output`, `subtype`, `event`, `delta`, `content`). Use `jq` to select/redact.

- [ ] **Step 1: Capture non-streaming JSON response**

Use stdin for all captures to match the production code path:

```bash
echo "respond with just the word hello" | claude -p --output-format json \
  --system-prompt "you are a test" 2>/dev/null > internal/claudecli/testdata/complete.json
```

- [ ] **Step 2: Capture JSON schema response**

```bash
echo "say hello" | claude -p --output-format json \
  --system-prompt "you are a test" \
  --json-schema '{"type":"object","properties":{"greeting":{"type":"string"}},"required":["greeting"]}' \
  2>/dev/null > internal/claudecli/testdata/complete_schema.json
```

- [ ] **Step 3: Capture stream-json response (plain text)**

```bash
echo "respond with just the word hello" | claude -p \
  --output-format stream-json --verbose --include-partial-messages \
  --system-prompt "you are a test" \
  2>/dev/null > internal/claudecli/testdata/stream.jsonl
```

- [ ] **Step 4: Capture stream-json response with JSON schema**

This is the format used by the extraction UI (`extraction.go:544`).

```bash
echo "say hello" | claude -p \
  --output-format stream-json --verbose --include-partial-messages \
  --system-prompt "you are a test" \
  --json-schema '{"type":"object","properties":{"greeting":{"type":"string"}},"required":["greeting"]}' \
  2>/dev/null > internal/claudecli/testdata/stream_schema.jsonl
```

- [ ] **Step 5: Verify fixtures are valid**

```bash
jq '.result' internal/claudecli/testdata/complete.json
jq '.structured_output' internal/claudecli/testdata/complete_schema.json
head -3 internal/claudecli/testdata/stream.jsonl
head -3 internal/claudecli/testdata/stream_schema.jsonl
```

- [ ] **Step 6: Commit**

```
git add internal/claudecli/testdata/
```

Use `/commit`.

---

### Task 2: Extract `llm.Provider` interface

**Files:**
- Create: `internal/llm/provider.go`
- Modify: `internal/app/model.go:189` (`llmClient` field type)
- Modify: `internal/app/model.go:259` (`var client` local type)
- Modify: `internal/app/model.go:877` (`extractionLLMClient()` return type)
- Modify: `internal/app/types.go:106` (`extractionClient` field type)
- Modify: `internal/extract/pipeline.go:18` (`LLMClient` field type)

- [ ] **Step 1: Create the interface file**

Create `internal/llm/provider.go` with the 12-method `Provider` interface. `*Client` already satisfies it -- no changes to `Client` needed.

```go
package llm

import (
	"context"
	"time"
)

// Provider is the interface consumed by the app layer for both chat and
// extraction pipelines, as well as model management (listing, switching).
type Provider interface {
	ChatComplete(ctx context.Context, messages []Message, opts ...ChatOption) (string, error)
	ChatStream(ctx context.Context, messages []Message, opts ...ChatOption) (<-chan StreamChunk, error)
	Ping(ctx context.Context) error
	ListModels(ctx context.Context) ([]string, error)
	Model() string
	SetModel(model string)
	SetThinking(level string)
	ProviderName() string
	BaseURL() string
	Timeout() time.Duration
	IsLocalServer() bool
	SupportsModelListing() bool
}
```

- [ ] **Step 2: Verify `*Client` satisfies `Provider`**

Add a compile-time check at the bottom of `provider.go`:

```go
var _ Provider = (*Client)(nil)
```

Run: `go build ./internal/llm/`

- [ ] **Step 3: Change caller field types**

In `internal/app/model.go:189`, change:
```go
llmClient *llm.Client
```
to:
```go
llmClient llm.Provider
```

In `internal/app/model.go:259`, change:
```go
var client *llm.Client
```
to:
```go
var client llm.Provider
```

In `internal/app/model.go:877`, change return type:
```go
func (m *Model) extractionLLMClient() *llm.Client {
```
to:
```go
func (m *Model) extractionLLMClient() llm.Provider {
```

In `internal/app/types.go:106`, change:
```go
extractionClient *llm.Client
```
to:
```go
extractionClient llm.Provider
```

In `internal/extract/pipeline.go:18`, change:
```go
LLMClient *llm.Client
```
to:
```go
LLMClient llm.Provider
```

- [ ] **Step 4: Verify everything compiles**

Run: `go build ./...`

Fix any compilation errors. The key concern: `autoDetectModel` at `model.go:1882` takes `*llm.Client` -- the Ollama branch at line 268 creates a `tempClient` via `llm.NewClient` which returns `*llm.Client`, so this compiles without changes.

- [ ] **Step 5: Run tests**

Run: `go test -shuffle=on ./...`

All existing tests should pass since `*llm.Client` satisfies `llm.Provider`.

- [ ] **Step 6: Commit**

Use `/commit`. Message: `refactor(llm): extract Provider interface from Client`

---

### Task 3: Decouple `ChatOption` from `any-llm-go` types

**Files:**
- Create: `internal/llm/options.go`
- Modify: `internal/llm/client.go:55-74` (`chatParams`, `WithJSONSchema`, `completionParams`)

- [ ] **Step 1: Write test for `ExtractJSONSchema`**

Add to an existing test file or create `internal/llm/options_test.go`:

```go
func TestExtractJSONSchema(t *testing.T) {
	t.Parallel()

	t.Run("no options", func(t *testing.T) {
		t.Parallel()
		_, _, ok := ExtractJSONSchema(nil)
		assert.False(t, ok)
	})

	t.Run("with schema", func(t *testing.T) {
		t.Parallel()
		schema := map[string]any{"type": "object"}
		name, s, ok := ExtractJSONSchema([]ChatOption{WithJSONSchema("test", schema)})
		require.True(t, ok)
		assert.Equal(t, "test", name)
		assert.Equal(t, schema, s)
	})
}
```

Run: `go test ./internal/llm/ -run TestExtractJSONSchema`
Expected: FAIL (function doesn't exist yet)

- [ ] **Step 2: Create `options.go` with `jsonSchemaParam` and `ExtractJSONSchema`**

Create `internal/llm/options.go`:

```go
package llm

// jsonSchemaParam holds a provider-agnostic JSON schema constraint.
type jsonSchemaParam struct {
	Name   string
	Schema map[string]any
}

// ExtractJSONSchema applies the given options and returns the JSON
// schema constraint, if any.
func ExtractJSONSchema(opts []ChatOption) (name string, schema map[string]any, ok bool) {
	var cp chatParams
	for _, opt := range opts {
		opt(&cp)
	}
	if cp.jsonSchema == nil {
		return "", nil, false
	}
	return cp.jsonSchema.Name, cp.jsonSchema.Schema, true
}
```

- [ ] **Step 3: Refactor `chatParams` and `WithJSONSchema` in `client.go`**

In `client.go:55-58`, replace:
```go
type chatParams struct {
	responseFormat *anyllm.ResponseFormat
}
```
with:
```go
type chatParams struct {
	jsonSchema *jsonSchemaParam
}
```

In `client.go:64-74`, replace `WithJSONSchema`:
```go
func WithJSONSchema(name string, schema map[string]any) ChatOption {
	return func(p *chatParams) {
		p.jsonSchema = &jsonSchemaParam{Name: name, Schema: schema}
	}
}
```

In `client.go`, lines 226-229 (`var cp` + option loop) stay unchanged.
Replace only lines 230-232 (the `responseFormat` conditional):
```go
var cp chatParams
for _, opt := range opts {
	opt(&cp)
}
if cp.jsonSchema != nil {
	params.ResponseFormat = &anyllm.ResponseFormat{
		Type: "json_schema",
		JSONSchema: &anyllm.JSONSchema{
			Name:   cp.jsonSchema.Name,
			Schema: cp.jsonSchema.Schema,
		},
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test -shuffle=on ./...`

All tests should pass -- the behavior is identical, just the internal representation changed.

- [ ] **Step 5: Commit**

Use `/commit`. Message: `refactor(llm): decouple ChatOption from any-llm-go types`

---

### Task 4: Add `claude-cli` to config validation

**Files:**
- Modify: `internal/config/config.go:689-700` (providers list)
- Modify: `internal/config/validate.go:111` (cross-field validation)
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write config validation tests**

Follow the existing `writeConfig`/`LoadFromPath` pattern used by all
other tests in `config_test.go`:

```go
func TestClaudeCLIProviderValidation(t *testing.T) {
	t.Parallel()

	t.Run("explicit claude model passes", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `[extraction.llm]
provider = "claude-cli"
model = "claude-sonnet-4-5-latest"
`)
		_, err := LoadFromPath(path)
		require.NoError(t, err)
	})

	t.Run("empty model rejected", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `[extraction.llm]
provider = "claude-cli"
model = ""
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "claude-cli")
	})

	t.Run("default model rejected", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `[extraction.llm]
provider = "claude-cli"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
	})

	t.Run("chat provider rejected by default", func(t *testing.T) {
		t.Parallel()
		path := writeConfig(t, `[chat.llm]
provider = "claude-cli"
model = "claude-sonnet-4-5-latest"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chat")
	})
}
```

Run tests to verify they fail.

- [ ] **Step 2: Add `"claude-cli"` to providers list**

In `config.go:689`, add `"claude-cli"` to the `providers` slice.

- [ ] **Step 3: Add model validation for claude-cli**

In `validate.go:111` (after `configValidator.Struct(c)`, before
`checkFilePermissions`), add cross-field checks for both pipelines:

```go
// claude-cli is extraction-only by default. Chat support is gated on
// Task 6 Step 0 (multi-turn transport verification). Remove this check
// only after multi-turn transport is confirmed.
if c.Chat.LLM.Provider == "claude-cli" {
	return fmt.Errorf("claude-cli does not yet support chat; use it under [extraction.llm] only")
}
if c.Extraction.LLM.Provider == "claude-cli" && (c.Extraction.LLM.Model == "" || c.Extraction.LLM.Model == DefaultModel) {
	return fmt.Errorf("claude-cli requires an explicit model (e.g. claude-sonnet-4-5-latest), got %q", c.Extraction.LLM.Model)
}
// Warn about ignored fields (per "no silent failures" rule)
if c.Extraction.LLM.Provider == "claude-cli" && (c.Extraction.LLM.BaseURL != "" || c.Extraction.LLM.APIKey != "") {
	slog.Warn("claude-cli ignores base_url and api_key; authentication is handled by the claude CLI")
}
```

- [ ] **Step 4: Run tests**

Run: `go test -shuffle=on ./internal/config/...`

- [ ] **Step 5: Commit**

Use `/commit`. Message: `feat(config): add claude-cli provider with model validation`

---

### Task 5: Implement `claudecli.Client` -- construction and `ChatComplete`

**Files:**
- Create: `internal/claudecli/client.go`
- Create: `internal/claudecli/client_test.go`

- [ ] **Step 1: Write `NewClient` tests**

Test cases:
- `NewClient` with `WithBinPath` pointing to a real executable succeeds
- `NewClient` with `WithBinPath` pointing to nonexistent path fails
- `NewClient` without `WithBinPath` when `claude` is not on PATH fails
- `Model()`, `SetModel()`, `SetThinking()`, `ProviderName()` return correct values
- `Ping()` returns nil
- `ListModels()` returns error
- `SupportsModelListing()` returns false
- `IsLocalServer()` returns false
- `BaseURL()` returns ""
- `Timeout()` returns `llm.QuickOpTimeout` (distinct from the `c.timeout` inference deadline used by `exec.CommandContext`)

Use the test binary's own path (`os.Args[0]`) as a valid executable for `WithBinPath` tests. Verify `Timeout()` returns `QuickOpTimeout` regardless of the construction-time timeout value.

- [ ] **Step 2: Implement `NewClient` and trivial methods**

Create `internal/claudecli/client.go` per the spec. Include:
- `Client` struct with `binPath`, `model`, `thinking`, `timeout`
- `Option` type, `WithBinPath`
- `NewClient` with `exec.LookPath` validation
- All 12 `Provider` interface methods (trivial stubs for now, real `ChatComplete`/`ChatStream` in next steps)
- Compile-time check: `var _ llm.Provider = (*Client)(nil)`

- [ ] **Step 3: Run construction tests**

Run: `go test ./internal/claudecli/ -run TestNewClient`

- [ ] **Step 4: Write `ChatComplete` tests using `TestHelperProcess`**

Create a `TestHelperProcess` that reads env vars to decide what to output:
- `CLAUDE_MOCK_MODE=complete`: output contents of `testdata/complete.json`
- `CLAUDE_MOCK_MODE=complete_schema`: output contents of `testdata/complete_schema.json`
- `CLAUDE_MOCK_MODE=error`: write to stderr and exit 1

Test cases:
- ChatComplete without schema returns `result` field
- ChatComplete with schema returns re-serialized `structured_output`
- ChatComplete with non-zero exit returns wrapped error with stderr
- ChatComplete enforces exact `[system, user]` or `[user]` shape:
  rejects assistant turns, multiple user messages, multiple system
  messages, empty slices, and non-leading system messages
- ChatComplete passes `--system-prompt` and `--model` flags
- ChatComplete passes `--json-schema` when schema option present
- ChatComplete maps thinking to `--effort` correctly (none/auto omit, low/medium/high pass through)

- [ ] **Step 5: Implement `ChatComplete`**

Build args, set `cmd.Stdin = strings.NewReader(userContent)`, set `cmd.Stderr = &stderrBuf`, run, parse JSON response with `json.Decoder` + `UseNumber()`. Handle `result` vs `structured_output` paths.

- [ ] **Step 6: Run all tests**

Run: `go test -shuffle=on ./internal/claudecli/...`

- [ ] **Step 7: Commit**

Use `/commit`. Message: `feat(claudecli): add Client with ChatComplete for extraction`

---

### Task 6: Implement `ChatStream`

**Files:**
- Modify: `internal/claudecli/client.go`
- Modify: `internal/claudecli/client_test.go`

**Per the spec, ChatStream is conditional.** Before writing tests:

- [ ] **Step 0: Verify multi-turn transport**

Test whether `claude -p --input-format stream-json` accepts structured
multi-turn NDJSON on stdin. The probe must exercise a full
system + user + assistant + user sequence to prove turn preservation:

First, verify the CLI accepts the format at all:

```bash
printf '{"role":"user","content":"say hello"}\n' \
  | claude -p --input-format stream-json --output-format json 2>&1 | head -3
```

If accepted, run two requests -- one WITH and one WITHOUT the
assistant turn -- and compare results. The assistant turn should
contain an arbitrary token (e.g. "xyzzy42") that does not appear in
any user message. If the response with the assistant turn reproduces
that token and the response without it does not, multi-turn transport
preserves assistant turns. The implementer designs the exact probe;
the key requirements are, both verified via differential tests
(with/without the role, compare results):
1. **Assistant-turn preservation**: expected answer depends on content
   only present in the assistant turn.
2. **System-turn preservation**: expected behavior depends on an
   instruction only present in the system turn. Use the same
   differential methodology: run with system message, run without,
   compare outputs.

Both non-user role types must be confirmed before enabling multi-turn. If not (or if the CLI rejects the
input format), `ChatStream` supports single-turn only. In that case,
also update Task 4's config validation to reject `provider =
"claude-cli"` under `[chat.llm]`.

Document the outcome in a comment at the top of `ChatStream`.

**If multi-turn is NOT supported:** implement `ChatStream` for
single-turn only (system + user). Add a message validation check that
rejects multi-turn inputs (assistant turns) with a clear error. Then
go back to Task 4 and add config validation that rejects
`provider = "claude-cli"` under `[chat.llm]`. Task 7 Step 1 (chat
wiring) should be skipped.

- [ ] **Step 1: Write `ChatStream` tests**

Create a `TestHelperProcess` mode that outputs the stream fixture line by line:
- `CLAUDE_MOCK_MODE=stream`: output contents of `testdata/stream.jsonl`
- `CLAUDE_MOCK_MODE=stream_schema`: output contents of `testdata/stream_schema.jsonl`
- `CLAUDE_MOCK_MODE=stream_error`: output partial stream then exit 1

Test cases:
- ChatStream emits content deltas from `content_block_delta` events
- ChatStream emits `Done: true` after `cmd.Wait()` succeeds
- ChatStream skips `system`, `assistant`, non-delta `stream_event` types
- ChatStream surfaces stderr on non-zero exit
- ChatStream respects context cancellation
- ChatStream with `--json-schema` streams JSON text deltas
- ChatStream builds correct flags (`--verbose`, `--include-partial-messages`)

- [ ] **Step 2: Implement `ChatStream`**

Get stdout pipe before `cmd.Start()`. Goroutine reads `json.Decoder`, dispatches on `type` field. Defer Done until after `cmd.Wait()`. Handle decoder error vs cancellation vs process exit precedence.

- [ ] **Step 3: Run all tests**

Run: `go test -shuffle=on ./internal/claudecli/...`

- [ ] **Step 4: Commit**

Use `/commit`. Message: `feat(claudecli): add ChatStream with NDJSON event parsing`

---

### Task 7: Wire `claude-cli` into app construction

**Files:**
- Modify: `internal/app/model.go:259-289` (chat client construction)
- Modify: `internal/app/model.go:877-901` (extraction client construction)

- [ ] **Step 1: Add `claudecli` import and construction branch for chat client**

**Conditional:** This step depends on the outcome of Task 6 Step 0
(multi-turn transport discovery). If multi-turn is NOT supported, skip
this step entirely -- `claude-cli` should only be wired for extraction.
Config validation in Task 4 should already reject `claude-cli` under
`[chat.llm]` in that case. If multi-turn IS supported, proceed.

In `model.go`, around line 276, change:
```go
var err error
client, err = llm.NewClient(
    chatCfg.Provider, chatCfg.BaseURL, model, chatCfg.APIKey, chatCfg.Timeout,
)
```
to:
```go
var err error
if chatCfg.Provider == "claude-cli" {
    client, err = claudecli.NewClient(model, chatCfg.Timeout)
} else {
    client, err = llm.NewClient(
        chatCfg.Provider, chatCfg.BaseURL, model, chatCfg.APIKey, chatCfg.Timeout,
    )
}
```

Apply `SetThinking` after construction (already happens at line 287-289).

- [ ] **Step 2: Add construction branch for extraction client**

In `extractionLLMClient()` around line 892, apply the same branching
pattern. **Interface-nil pitfall:** since `extractionClient` is now
`llm.Provider` (an interface), never assign a typed nil pointer to it.
Always check `err != nil` and return `nil` (untyped) before assigning.
The existing pattern at line 892-894 already does this correctly --
preserve it for the new branch:

```go
if provider == "claude-cli" {
    c, err := claudecli.NewClient(model, timeout)
    if err != nil {
        return nil // untyped nil, not (*claudecli.Client)(nil)
    }
    // ...
    m.ex.extractionClient = c
    return c
}
```

- [ ] **Step 3: Add `!isLocal` guard to `switchExtractionModel`**

In `internal/app/extraction.go`, before the `startPull` call at line
1266, add a `!client.IsLocalServer()` guard matching the pattern in
`cmdSwitchModel` at `chat.go:667`. Without this, a `claude-cli` user
switching extraction models would trigger an Ollama HTTP pull against
an empty URL.

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`

- [ ] **Step 5: Run full test suite**

Run: `go test -shuffle=on ./...`

- [ ] **Step 6: Commit**

Use `/commit`. Message: `feat(app): wire claude-cli provider into chat and extraction construction`

---

### Task 8: Integration and pipeline tests

**Files:**
- Create/modify: `internal/claudecli/client_test.go` (integration test)
- Modify: `internal/extract/pipeline_llm_test.go` (pipeline test with claudecli)
- Modify: `internal/app/extraction_test.go` (TUI-level test)

- [ ] **Step 1: Add opt-in integration test**

Add a test that calls real `claude -p` with a trivial prompt, gated
behind an explicit env var AND binary availability. Skipped by default
to avoid consuming paid usage on routine local runs:

```go
func TestIntegration_RealCLI(t *testing.T) {
	if os.Getenv("MICASA_TEST_CLAUDE_CLI") != "1" {
		t.Skip("set MICASA_TEST_CLAUDE_CLI=1 to run real CLI tests")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not available")
	}
	// ...
}
```

- [ ] **Step 2: Add pipeline-level test**

In `internal/extract/pipeline_llm_test.go`, add a test that constructs
a `Pipeline` with `LLMClient` set to a `claudecli.Client` using the
`TestHelperProcess` mock binary. Verify the extraction pipeline
produces valid operations from the mock response.

- [ ] **Step 3: Add TUI-level extraction test**

In `internal/app/extraction_test.go`, add a test using
`newTestModelWithStore` that constructs a model with `claude-cli`
provider (using `WithBinPath` pointed at the `TestHelperProcess` mock).
Exercise extraction through user-driven interactions (keypresses, form
submissions) to verify the wiring at the level users actually exercise.
This is required by the project's testing rules ("tests simulate real
user interaction").

- [ ] **Step 4: (Conditional) Add chat TUI test if multi-turn is enabled**

If Task 6 Step 0 confirmed multi-turn transport and chat wiring was
added in Task 7, add a chat-focused TUI test that exercises system +
assistant + user history through `ChatStream`. If chat is not enabled,
skip this step.

- [ ] **Step 5: Run tests**

Run: `go test -shuffle=on ./...`

- [ ] **Step 5: Commit**

Use `/commit`. Message: `test(claudecli): add integration, pipeline, and TUI-level tests`

**Step numbering note:** Step 4 is conditional; adjust commit message
accordingly (include "chat" only if the chat TUI test was added).

---

### Task 9: Edge-case coverage and linting

**Files:**
- Modify: `internal/claudecli/client_test.go` (add any missing edge cases)

- [ ] **Step 1: Run coverage**

Run: `nix run '.#coverage'` or `go test -coverprofile cover.out ./internal/claudecli/... && go tool cover -func cover.out`

Verify new code in `internal/claudecli/` has adequate coverage.

- [ ] **Step 2: Add missing edge-case tests**

Based on coverage gaps:
- Message validation: empty message slice, multiple system messages, non-leading system messages
- Response parsing: missing `result` field, null `structured_output`, malformed JSON
- Effort mapping: each thinking value maps correctly

- [ ] **Step 3: Run full test suite**

Run: `go test -shuffle=on ./...`

- [ ] **Step 4: Run linter**

Run: `golangci-lint run`

Fix any warnings.

- [ ] **Step 5: Commit**

Use `/commit`. Message: `test(claudecli): add edge-case coverage`

---

### Task 10: Update codebase docs

**Files:**
- Modify: `.claude/codebase/structure.md`
- Modify: `.claude/codebase/types.md`

- [ ] **Step 1: Add `claudecli` to structure.md**

Add under the `internal/` section:
```
  claudecli/              Claude CLI subprocess backend
    client.go             Client implementing llm.Provider via claude binary
    client_test.go        Tests with TestHelperProcess mock
    testdata/             Captured real CLI response fixtures
```

- [ ] **Step 2: Add `Provider` interface to types.md**

Add under `LLM Types`:
```
### Provider (provider.go)
- 12-method interface consumed by app layer
- *Client satisfies it (any-llm-go wrapper)
- claudecli.Client satisfies it (subprocess wrapper)
```

- [ ] **Step 3: Commit**

Use `/commit`. Message: `docs: update codebase map for claudecli package and Provider interface`
