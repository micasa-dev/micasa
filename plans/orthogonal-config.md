<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Orthogonal Config Architecture

Closes #731, addresses #737.

## Principle

No config value affects any other config value across sections.

## Current structure (broken)

```
[llm]                    <- shared base, cascades into chat/extraction
  [llm.chat]             <- per-pipeline override (inherits from [llm])
  [llm.extraction]       <- per-pipeline override (inherits from [llm])
[extraction]             <- feature config, split from its LLM config
  [extraction.ocr]
```

Problems:
- Setting `llm.model` silently changes what chat uses unless `llm.chat.model`
  is also set
- Extraction LLM settings split across `[extraction]` and `[llm.extraction]`
- `extraction.ocr.confidence_threshold` meaningless when `extraction.ocr.tsv`
  is false (hidden dependency)

## New structure

```toml
[chat]
enable = true

[chat.llm]
provider = "ollama"
base_url = "http://localhost:11434"
model = "qwen3:0.6b"
api_key = ""
timeout = "5m"
thinking = ""
extra_context = ""

[extraction]
max_pages = 0

[extraction.llm]
enable = true
provider = "ollama"
base_url = "http://localhost:11434"
model = "qwen3:0.6b"
api_key = ""
timeout = "5m"
thinking = ""

[extraction.ocr]
enable = true

[extraction.ocr.tsv]
enable = true
confidence_threshold = 70

[documents]
max_file_size = "50 MiB"
cache_ttl = "30d"
file_picker_dir = ""

[locale]
currency = "USD"
```

## Go types

```go
Config {
    Chat       Chat
    Extraction Extraction
    Documents  Documents
    Locale     Locale
}

Chat {
    Enable *bool
    LLM    ChatLLM
}

ChatLLM {
    Provider, BaseURL, Model, APIKey, Timeout, Thinking, ExtraContext string
}

Extraction {
    MaxPages int
    LLM      ExtractionLLM
    OCR      OCR
}

ExtractionLLM {
    Enable *bool
    Provider, BaseURL, Model, APIKey, Timeout, Thinking string
}

OCR {
    Enable *bool
    TSV    OCRTSV
}

OCRTSV {
    Enable              *bool
    ConfidenceThreshold *int
}
```

## Key decisions

- **No `[llm]` base section**: each pipeline owns its full LLM config with
  independent hardcoded defaults. No inheritance.
- **`extra_context` in `[chat.llm]`**: it's an LLM prompt-level concern.
- **`[extraction.ocr.tsv]` is its own section**: `enable` and
  `confidence_threshold` scoped together.
- **Explicit `enable` per sub-feature**: `chat.enable`, `extraction.llm.enable`,
  `extraction.ocr.enable`, `extraction.ocr.tsv.enable`. Each controls only
  itself. All default true.
- **No backward compatibility**: old keys removed, not migrated.
- **Within-section auto-detection stays**: provider inferred from base_url +
  api_key within a single section.

## Env vars (new canonical names)

```
MICASA_CHAT_ENABLE
MICASA_CHAT_LLM_PROVIDER
MICASA_CHAT_LLM_BASE_URL
MICASA_CHAT_LLM_MODEL
MICASA_CHAT_LLM_API_KEY
MICASA_CHAT_LLM_TIMEOUT
MICASA_CHAT_LLM_THINKING
MICASA_CHAT_LLM_EXTRA_CONTEXT
MICASA_EXTRACTION_MAX_PAGES
MICASA_EXTRACTION_LLM_ENABLE
MICASA_EXTRACTION_LLM_PROVIDER
MICASA_EXTRACTION_LLM_BASE_URL
MICASA_EXTRACTION_LLM_MODEL
MICASA_EXTRACTION_LLM_API_KEY
MICASA_EXTRACTION_LLM_TIMEOUT
MICASA_EXTRACTION_LLM_THINKING
MICASA_EXTRACTION_OCR_ENABLE
MICASA_EXTRACTION_OCR_TSV_ENABLE
MICASA_EXTRACTION_OCR_TSV_CONFIDENCE_THRESHOLD
MICASA_DOCUMENTS_MAX_FILE_SIZE
MICASA_DOCUMENTS_CACHE_TTL
MICASA_DOCUMENTS_FILE_PICKER_DIR
MICASA_LOCALE_CURRENCY
```

## Implementation sequence

1. Rewrite `internal/config/config.go` types + LoadFromPath
2. Rewrite `internal/config/show.go`
3. Rewrite `internal/config/config_test.go` + `show_test.go`
4. Update `cmd/micasa/main.go`
5. Update `internal/app/types.go` (Options, SetChat, SetExtraction)
6. Update `internal/app/model.go` (init, extractionLLMClient)
7. Update `internal/app/extraction.go`
8. Update docs
9. Update `.claude/codebase/types.md`
