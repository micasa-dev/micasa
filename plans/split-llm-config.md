<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Split LLM config into per-pipeline sections

**Issue:** #569
**Status:** Done

## Problem

The extraction pipeline currently shares the chat pipeline's LLM provider,
base URL, and API key. Only the model name and thinking level can be
overridden. Users who want to run chat on a local Ollama server but extraction
on a fast cloud model (e.g. Anthropic Haiku) cannot do so.

## Design

### Config structure

```toml
[llm]
# Base settings -- defaults for both pipelines.
provider = "ollama"
base_url = "http://localhost:11434"
model = "qwen3:0.6b"
api_key = ""
extra_context = "My house is a 1920s craftsman in Portland, OR."
timeout = "5s"
thinking = "medium"

[llm.chat]
# Optional overrides just for the chat pipeline.
# model = "qwen3:30b"
# thinking = "high"

[llm.extraction]
# Optional overrides just for the extraction pipeline.
# provider = "anthropic"
# model = "claude-haiku-3-5-20241022"
# api_key = "sk-ant-..."
```

Resolution order for each pipeline:
1. Start with `[llm]` base fields
2. Overlay with `[llm.chat]` or `[llm.extraction]` (non-empty fields win)

### Go struct changes

```go
// LLMOverride holds per-pipeline fields that override the base LLM config.
// Only non-empty values override.
type LLMOverride struct {
    Provider string `toml:"provider" env:"..."`
    BaseURL  string `toml:"base_url" env:"..."`
    Model    string `toml:"model" env:"..."`
    APIKey   string `toml:"api_key" env:"..."`
    Timeout  string `toml:"timeout" env:"..."`
    Thinking string `toml:"thinking,omitempty" env:"..."`
}

type LLM struct {
    // ... existing base fields ...
    Chat       LLMOverride `toml:"chat"`
    Extraction LLMOverride `toml:"extraction"`
}
```

New methods on `LLM`:
- `ChatConfig() ResolvedLLM` -- merges base + chat overrides
- `ExtractionConfig() ResolvedLLM` -- merges base + extraction overrides

`ResolvedLLM` is a flat struct with all fields resolved (no pointers, no
empty-means-inherit).

### Environment variables

New env vars for per-pipeline overrides:
- `MICASA_LLM_CHAT_PROVIDER`, `MICASA_LLM_CHAT_BASE_URL`, etc.
- `MICASA_LLM_EXTRACTION_PROVIDER`, `MICASA_LLM_EXTRACTION_BASE_URL`, etc.

### Deprecations

Move LLM-related fields out of `[extraction]`:
- `[extraction].model` -> `[llm.extraction].model` (deprecation warning)
- `[extraction].thinking` -> `[llm.extraction].thinking` (deprecation warning)
- `MICASA_EXTRACTION_MODEL` -> `MICASA_LLM_EXTRACTION_MODEL` (deprecation)
- `MICASA_EXTRACTION_THINKING` -> `MICASA_LLM_EXTRACTION_THINKING` (deprecation)

### Validation

- Each pipeline-specific provider (if set) is validated against the provider
  whitelist.
- Each pipeline-specific thinking level is validated.
- Each pipeline-specific timeout is validated (positive duration).
- Auto-detection runs per-pipeline when provider is empty but api_key is set.
- Base URL normalization (/v1 stripping, trailing slash) runs per-pipeline.

### Wiring changes

- `main.go`: Pass two resolved configs to `Options.SetLLM` and
  `Options.SetExtraction` (or a unified setter).
- `app/types.go`: `extractionConfig` gains full LLM fields (provider, base_url,
  api_key, timeout) so `extractionLLMClient()` can create an independent client.
- `app/model.go`: `extractionLLMClient()` uses extraction-specific config
  instead of cloning the chat client's provider.
- If extraction has its own provider, it creates a fully independent `llm.Client`.
  If not, it falls back to the chat client (current behavior).

### Backward compatibility

- A config with no `[llm.chat]` or `[llm.extraction]` sections behaves
  identically to today.
- `[extraction].model` and `[extraction].thinking` still work but produce
  deprecation warnings.

## Execution plan

1. Add `LLMOverride` struct and `ResolvedLLM` to `internal/config/config.go`
2. Add `Chat` and `Extraction` fields to `LLM` struct
3. Implement merge logic (`ChatConfig()`, `ExtractionConfig()`)
4. Migrate `[extraction].model` / `[extraction].thinking` with deprecation
5. Update validation to handle per-pipeline settings
6. Update `ExampleTOML()` to document new sections
7. Update `app/types.go` extractionConfig with full LLM fields
8. Update `main.go` wiring
9. Update `app/model.go` `extractionLLMClient()` to use independent config
10. Add config tests for new sections
11. Update existing tests for deprecation paths
