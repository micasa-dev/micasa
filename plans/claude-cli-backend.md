<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Claude CLI LLM Backend

Date: 2026-04-01

## Problem

Using Claude models in micasa currently requires an Anthropic API key
configured in `config.toml`. Users who have Claude Code installed (the
`claude` CLI) already have authenticated access to Claude models through
their Anthropic subscription. This design adds `claude-cli` as a new LLM
provider that shells out to the `claude` binary, eliminating the need for
a separate API key.

This is also the template for future CLI-based providers (e.g. Codex CLI).

## Design

### Interface extraction

Extract a `Provider` interface from `llm.Client` that both the existing
`any-llm-go` wrapper and CLI backends implement:

```go
// internal/llm/provider.go
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

`llm.Client` already has all these methods (the method is already named
`ProviderName()` at `client.go:163` -- no rename needed).

### Callers

These fields and return types change from `*llm.Client` to `llm.Provider`:

| Location | Field/Function |
|---|---|
| `internal/app/model.go:189` | `llmClient *llm.Client` field |
| `internal/app/types.go:106` | `extractionClient *llm.Client` field in `extractState` |
| `internal/app/model.go:877` | `extractionLLMClient()` return type |
| `internal/extract/pipeline.go:18` | `LLMClient *llm.Client` field in `Pipeline` |

Callers that use wider methods (`BaseURL()`, `Timeout()`,
`IsLocalServer()`, `SupportsModelListing()`, `ListModels()`) are
satisfied by the interface -- the CLI backend returns safe defaults
(see below).

The `autoDetectModel()` function at `model.go:1882` keeps its
`*llm.Client` parameter type -- it is only reachable from the
`chatCfg.Provider == "ollama"` gate at `model.go:266` and never
receives a CLI backend. The auto-detection block requires no changes
for `claude-cli`.

Test helpers in `chat_coverage_test.go`, `extraction_test.go`, and
`extract/pipeline_llm_test.go` that return `*llm.Client` continue to
work since `*llm.Client` satisfies `llm.Provider`. No changes needed
to test helper bodies; only explicit type annotations in struct fields
need updating.

### ChatOption adaptation

`ChatOption` applies to `chatParams` which currently holds
`*anyllm.ResponseFormat`. The CLI backend does not use `any-llm-go`
types, so `chatParams` needs a provider-agnostic representation:

```go
type chatParams struct {
    jsonSchema *jsonSchemaParam // nil = no schema constraint
}

type jsonSchemaParam struct {
    Name   string
    Schema map[string]any
}
```

`WithJSONSchema` is rewritten to populate `jsonSchemaParam`:

```go
// Before (current):
func WithJSONSchema(name string, schema map[string]any) ChatOption {
    return func(p *chatParams) {
        p.responseFormat = &anyllm.ResponseFormat{
            Type: "json_schema",
            JSONSchema: &anyllm.JSONSchema{Name: name, Schema: schema},
        }
    }
}

// After:
func WithJSONSchema(name string, schema map[string]any) ChatOption {
    return func(p *chatParams) {
        p.jsonSchema = &jsonSchemaParam{Name: name, Schema: schema}
    }
}
```

`Client.completionParams()` is updated to convert `jsonSchemaParam`
back to `anyllm.ResponseFormat` for the `any-llm-go` path:

```go
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

The CLI backend marshals `Schema` to JSON for `--json-schema`.

`chatParams` is unexported (lowercase), so `claudecli` cannot
instantiate it directly. Add an exported accessor in `internal/llm/`:

```go
// ExtractJSONSchema applies the given options and returns the JSON
// schema constraint, if any. This allows external Provider
// implementations to interpret ChatOption values without accessing
// unexported types.
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

This keeps `chatParams` and `jsonSchemaParam` unexported while giving
CLI backends a clean way to extract the schema.

### New package: `internal/claudecli/`

```go
// internal/claudecli/client.go
package claudecli

type Client struct {
    binPath  string // resolved via exec.LookPath("claude") at construction
    model    string
    thinking string // passed as --effort <value>
    timeout  time.Duration
}

// Option configures the client. Used for testing.
type Option func(*Client)

// WithBinPath overrides the default exec.LookPath resolution.
// Tests pass the TestHelperProcess binary path here.
func WithBinPath(path string) Option { ... }

func NewClient(model string, timeout time.Duration, opts ...Option) (*Client, error) {
    c := &Client{model: model, timeout: timeout}
    for _, o := range opts { o(c) }
    if c.binPath == "" {
        bin, err := exec.LookPath("claude")
        if err != nil {
            return nil, fmt.Errorf("claude CLI not found on PATH: %w", err)
        }
        c.binPath = bin
    } else {
        // Validate injected path using LookPath (cross-platform,
        // handles PATHEXT on Windows)
        resolved, err := exec.LookPath(c.binPath)
        if err != nil {
            return nil, fmt.Errorf("claude binary at %s: %w", c.binPath, err)
        }
        c.binPath = resolved
    }
    return c, nil
}
```

**`ChatComplete` (extraction path)**:

1. Build args: `[claude, -p, --output-format, json, --model, <model>]`
2. Format messages per "Message formatting" section: system to
   `--system-prompt`, user content on stdin. ChatComplete is always
   single-turn (validated to reject multi-turn inputs).
3. If `jsonSchemaParam` present: append `--json-schema` with
   `json.Marshal(schema.Schema)`
4. If `thinking` non-empty: translate and append `--effort`:
   - `""` or `"none"` -> omit `--effort` entirely
   - `"low"`, `"medium"`, `"high"` -> pass through as-is
   - `"auto"` -> omit `--effort` (let the CLI use its own adaptive
     default, preserving `auto` semantics without forcing max effort)
   (The config validates `thinking` as `none|low|medium|high|auto` for
   the Anthropic API, but the CLI accepts `low|medium|high|max`.)
5. Derive a timeout context: `cmdCtx, cancel := context.WithTimeout(
   ctx, c.timeout)`. Create `exec.CommandContext(cmdCtx, ...)`
6. Pipe user message to stdin via `cmd.Stdin = strings.NewReader(...)`
   (finite reader sends EOF after content, preventing the CLI from
   blocking on stdin). Alternatively, if using `StdinPipe`, close it
   immediately after writing.
7. Capture stdout, parse JSON response:
   - With `--json-schema`: validate that `structured_output` exists
     and is non-null (any JSON type is valid -- object, array, string,
     number, boolean). If missing or null, return error with raw
     response for debugging.
     Re-serialize via `json.Marshal` to match the `string` return
     type. The initial parse must use `json.Decoder` with
     `UseNumber()` to preserve integer fidelity (callers like
     `ParseOperations` also use `UseNumber`)
   - Without `--json-schema`: validate that `result` is a string (may
     be empty -- emptiness handling is the caller's concern). If the
     field is missing or not a string, return error with raw response
8. Stderr is captured via `cmd.Stderr = &stderrBuf` (a `bytes.Buffer`)
   before calling `cmd.Run`. This drains stderr concurrently with
   stdout without a separate goroutine, avoids pipe backpressure, and
   keeps stdout parseable. On non-zero exit, return wrapped error with
   `stderrBuf.String()` content

The `--output-format json` response structure:

```json
// Without --json-schema:
{"result": "SELECT * FROM vendors WHERE ..."}

// With --json-schema:
{"result": "", "structured_output": {"operations": [...]}}
```

**`ChatStream` (chat and extraction streaming path)**:

1. Build args:
   ```
   [claude, -p, --output-format, stream-json, --verbose,
    --include-partial-messages]
   ```
   `--verbose` is required for stream-json in `-p` mode.
   `--include-partial-messages` is required to get token-level deltas
   (without it, only cumulative `assistant` snapshots are emitted).
2. Format messages per "Message formatting" section: system goes to
   `--system-prompt`. Multi-turn history uses lossless structured
   input (verified at implementation time). Single-turn (first chat
   message) uses `--system-prompt` + user content on stdin.
3. Same model/effort handling as above. If `ChatOption` includes a
   JSON schema constraint, append `--json-schema` (the extraction UI
   at `extraction.go:544` calls `ChatStream` with `WithJSONSchema`).
   With schema, the model streams the JSON text token-by-token via
   `content_block_delta` events; callers accumulate and parse after
   Done. This matches the existing `any-llm-go` streaming behavior.
4. Set `cmd.Stderr = &stderrBuf`, get stdout pipe via
   `cmd.StdoutPipe()`, then call `cmd.Start()` (pipe must be obtained
   before Start; no separate stderr pipe needed)
5. Goroutine (stdout): parse events using `json.Decoder` (not
   `bufio.Scanner`, whose default 64 KiB token limit would be exceeded
   by cumulative `assistant` snapshot lines on long replies). Each
   decoded object has a top-level
   `type` field. Event dispatch:
   - `type: "system"` -- hooks, init; skip
   - `type: "assistant"` -- cumulative turn snapshots; skip
   - `type: "stream_event"` -- contains nested `.event` object.
     Dispatch on `.event.type`:
     - `"content_block_delta"` -- extract `.event.delta.text`, forward
       as `StreamChunk{Content: text}`
     - `"message_stop"` -- note completion seen (do NOT send Done yet)
     - Other subtypes -- skip
   - `type: "result"` -- note completion seen (do NOT send Done yet)
   - `type: "rate_limit_event"` and others -- skip
6. After stdout goroutine exits (EOF or decoder error): if the exit
   was due to a decoder error, use a separate `cancel` function (not
   the caller's context) to kill the subprocess, and record the
   decoder error in a `decErr` variable. Then call `cmd.Wait()`.
   Stderr is already drained into `stderrBuf` by `cmd.Stderr`.
   Terminal chunk decision: the goroutine records whether the caller
   context was already done before the decoder error occurred (check
   `ctx.Err()` before entering the kill path). This distinguishes
   "caller cancelled, decoder got truncated input" from "CLI sent
   malformed output":
   - Caller ctx was done before decoder error: cancellation wins
   - `decErr != nil` and caller ctx was NOT done: malformed stream
   - Non-zero exit + no decoder error: `StreamChunk{Err: stderr}`
   - Exit 0 + completion seen: `StreamChunk{Done: true}`
   - Exit 0 + no completion: `StreamChunk{Err: unexpected EOF}`
7. Context cancellation kills the subprocess via `cmd.Cancel`

**Methods with safe defaults for CLI backends:**

| Method | Return value | Rationale |
|---|---|---|
| `Ping` | `nil` | Binary existence verified at construction. No runtime auth or model check -- matches the existing Anthropic provider pattern. Auth/model issues surface on the first real request. |
| `ListModels` | `nil, error` | CLI does not support model listing |
| `SupportsModelListing` | `false` | Callers gate `ListModels` behind this check. Note: `switchExtractionModel` (`extraction.go:1221`) relies on `!canList` to exit early before reaching `startPull`, while `cmdSwitchModel` (`chat.go:667`) has an explicit `!isLocal` guard. Both are safe for CLI backends, but as a defensive fix add a `!client.IsLocalServer()` guard to `switchExtractionModel` before `startPull` to match the chat path. |
| `IsLocalServer` | `false` | Not a local HTTP server; no pull/start logic applies |
| `BaseURL` | `""` | No HTTP endpoint |
| `Timeout` | `llm.QuickOpTimeout` | Returns the same 30s quick-op deadline as `llm.Client.Timeout()`, keeping `Provider.Timeout()` semantics uniform. Verified: all `Timeout()` call sites (`chat.go:444,526,640`, `extraction.go:504,1167,1242`, `model.go:741,1883`) use it only for quick ops (Ping, ListModels, model checks), never for inference deadlines. The inference timeout is internal to `exec.CommandContext`. |
| `Model`/`SetModel`/`SetThinking`/`ProviderName` | Trivial getters/setters | `ProviderName()` returns `"claude-cli"` |

### Config changes

`internal/config/config.go`:

- Add `"claude-cli"` to the `providers` validation list (used by
  `validProvider()`)
- Do NOT add to `localProviders` in `client.go` -- that map controls
  loopback URL stripping and "start the server" error messages, neither
  of which applies to CLI backends
- When `provider == "claude-cli"`: skip `llm.NewClient`, call
  `claudecli.NewClient` instead
- `base_url` and `api_key` are ignored for this provider
- `model` maps to `--model`
- `thinking` maps to `--effort`
- `timeout` maps to `exec.CommandContext` deadline (reuses existing field)
- `model` is required for `claude-cli` (no empty default). Config
  validation rejects `provider = "claude-cli"` when the model is empty
  or still set to the provider-agnostic default (currently `qwen3`).
  The check compares against the default model constant, not a
  hardcoded string, so it adapts if the default changes. Example:
  `model = "claude-sonnet-4-5-latest"`

### Client construction wiring

`internal/app/model.go` currently calls `llm.NewClient(provider, ...)`.
Add a branch:

```go
var client llm.Provider
if chatCfg.Provider == "claude-cli" {
    client, err = claudecli.NewClient(model, chatCfg.Timeout)
} else {
    client, err = llm.NewClient(...)
}
```

The Ollama auto-detection block at `model.go:266-275` is already gated
behind `chatCfg.Provider == "ollama"` and requires no changes.

Same branching pattern for extraction client construction in
`model.go:extractionLLMClient()`.

### Message formatting

**System prompt**: exactly one leading `system` message is passed to
`--system-prompt`. Multiple leading system messages are rejected (not
concatenated, to preserve message boundary semantics). Non-leading
system messages (after a user or assistant turn) are also rejected.
No current caller sends multiple or non-leading system messages.

**Single-turn** (ChatComplete / extraction): user content piped to
stdin as plain text. No collision risk.

**Multi-turn** (ChatStream / chat): implementation MUST use a lossless
structured input format. Verify whether `--input-format stream-json`
accepts a structured message array; if so, use it. If not, find an
alternative lossless mechanism. Flattened role-marker text is NOT
acceptable because it cannot guarantee collision-free encoding. If no
lossless multi-turn transport exists, `claude-cli` supports extraction
(ChatComplete and single-turn ChatStream) but not multi-turn chat
until the CLI adds structured input support. Single-turn ChatStream
(system + user, as used by extraction UI) works because it uses
`--system-prompt` + stdin with no history to flatten.

### Error handling

| Condition | Behavior |
|---|---|
| `claude` not on PATH | `NewClient` returns error at construction |
| Non-zero exit | Wrap exec error with stderr: `fmt.Errorf("claude-cli: %w: %s", err, stderr)` (preserves exit status/signal info) |
| Context timeout | `exec.CommandContext` kills process, returns deadline error |
| Auth expired | CLI prints error to stderr, captured and surfaced |
| Invalid model | CLI prints error to stderr, captured and surfaced |
| Malformed JSON output | Return parse error with raw output snippet for debugging |

### Testing

- Unit tests mock the `claude` binary using the `TestHelperProcess`
  re-exec pattern (see `internal/app/docopen_test.go` for the existing
  convention). The test binary re-executes itself with an env var flag,
  echoing captured real `claude -p` payloads (committed as test
  fixtures under `internal/claudecli/testdata/`). This is cross-platform
  (CI runs on Windows) unlike shell script mocks. Test both
  `--output-format json` and `--output-format stream-json` output paths.
- Integration test (skipped in CI): real `claude -p` call with a trivial
  prompt to verify end-to-end
- Extraction pipeline tests: swap `LLMClient` to a `claudecli.Client`
  pointing at the mock binary
- App-level TUI tests: use `newTestModelWithStore` with a mock
  `claude` binary to exercise the `model.go` provider branching and
  chat/extraction flows through user-driven interactions (keypresses,
  form submissions). This verifies the wiring at the level users
  actually exercise, not just the `claudecli` package in isolation.
- Serialization edge-case tests: histories with non-leading system
  messages verify rejection. Missing or null `structured_output`
  responses verify error handling. If structured multi-turn transport
  is implemented, test round-trip fidelity of arbitrary message content
  including role-marker-like strings.
- Test helpers in `chat_coverage_test.go`, `extraction_test.go`, and
  `pipeline_llm_test.go` that return `*llm.Client` continue to compile
  since `*llm.Client` satisfies `llm.Provider`; only struct field type
  annotations need updating

### Future: Codex CLI

The same pattern applies: `internal/codexcli/client.go` implementing
`llm.Provider` by shelling out to `codex`. The interface, config
branching, and caller changes are already in place.

## Scope

**In scope:**
- `llm.Provider` interface extraction (12 methods)
- `internal/claudecli/` package: ChatComplete (extraction) is
  unconditional. ChatStream (chat) is conditional on confirming a
  lossless structured multi-turn transport during implementation; if
  unavailable, `claude-cli` ships as extraction-only.
- Config support for `provider = "claude-cli"`
- Wiring in `model.go` for extraction client; chat client wiring is
  conditional on `ChatStream` support (if unsupported, config
  validation rejects `provider = "claude-cli"` under `[chat.llm]` with
  a clear error, while `[extraction.llm]` remains valid)
- ChatOption/chatParams decoupling from `any-llm-go` types
- Tests with mock binary

**Out of scope:**
- Tool/function calling (forward-compatible but not implemented)
- Codex CLI backend (follow-up)
- `--tools` flag support (follow-up)
- Auto-detection/auto-fallback from `anthropic` to `claude-cli`
