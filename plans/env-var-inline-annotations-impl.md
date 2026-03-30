<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Inline Env Var Annotations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the configuration reference page to embed env var names as subtle annotations inside each TOML section table, eliminating the redundant standalone env var section.

**Architecture:** Pure docs/CSS change. A one-line Hugo shortcode renders `<span class="env-hint">` for each env var name. The standalone env var section (table + per-var subsections) is deleted entirely. The "Platform data directory" subsection is promoted to a top-level heading.

**Tech Stack:** Hugo (v0.159), CSS, Hugo shortcodes, Markdown

**Spec:** `plans/env-var-inline-annotations.md`

---

### Task 1: Create the `env` shortcode and `.env-hint` CSS

**Files:**
- Create: `docs/layouts/shortcodes/env.html`
- Modify: `docs/static/css/docs.css`

- [ ] **Step 1: Create the shortcode**

Write `docs/layouts/shortcodes/env.html`:

```html
<span class="env-hint">{{ .Get 0 }}</span>
```

- [ ] **Step 2: Add the `.env-hint` CSS class**

Add to `docs/static/css/docs.css`, in the Tables section after the `.docs-main td code` rule (around line 614):

```css
.docs-main .env-hint {
  display: block;
  font-family: "JetBrains Mono", "Fira Code", "Consolas", monospace;
  font-size: 0.7em;
  color: var(--warm-gray);
  margin-top: 0.15em;
  letter-spacing: -0.01em;
  overflow-wrap: anywhere;
}
```

- [ ] **Step 3: Verify Hugo build**

Run: `cd docs && nix develop -c hugo --logLevel warn`

Expected: clean build, no errors or warnings.

- [ ] **Step 4: Commit**

```
docs(website): add env shortcode and env-hint CSS class
```

---

### Task 2: Delete the standalone env var section

**Files:**
- Modify: `docs/content/docs/reference/configuration.md:111-204`

- [ ] **Step 1: Delete lines 111-204**

Remove everything from `## Environment variables` (line 111) through the end of `### MICASA_DOCUMENTS_CACHE_TTL_DAYS` (line 203, the blank line before `### Platform data directory`).

This deletes:
- The `## Environment variables` heading and naming convention explanation
- The big env var table (25 rows)
- All 6 per-env-var subsections (`MICASA_DB_PATH`, `MICASA_CHAT_LLM_MODEL`, `MICASA_CHAT_LLM_TIMEOUT`, `MICASA_DOCUMENTS_MAX_FILE_SIZE`, `MICASA_DOCUMENTS_CACHE_TTL`, `MICASA_DOCUMENTS_CACHE_TTL_DAYS`)

- [ ] **Step 2: Promote "Platform data directory" to h2**

The `### Platform data directory` subsection (formerly under `## Environment variables`) becomes a top-level section. Change:

```markdown
### Platform data directory
```

to:

```markdown
## Platform data directory
```

- [ ] **Step 3: Verify Hugo build and no broken internal links**

Run: `cd docs && nix develop -c hugo --logLevel warn`

Expected: clean build. Check that `#platform-data-directory` anchor still works (used by the env var table row for `MICASA_DB_PATH` -- but that table is now deleted, so no link references remain).

- [ ] **Step 4: Commit**

```
docs(website): remove standalone env var section

The env var table and per-var subsections are redundant with the per-section
TOML tables. Env var names will be added as inline annotations in the next
commit. "Platform data directory" promoted to a top-level section.
```

---

### Task 3: Add env var annotations to all TOML section tables

**Files:**
- Modify: `docs/content/docs/reference/configuration.md` (8 tables)

- [ ] **Step 1: Add annotations to `[chat]` section table**

Change the table at the `### [chat] section` heading. Each Key cell gets a shortcode after the backtick-quoted key:

```markdown
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enable` {{< env "MICASA_CHAT_ENABLE" >}} | bool | `true` | Set to `false` to hide the chat feature from the UI. |
```

- [ ] **Step 2: Add annotations to `[chat.llm]` section table**

```markdown
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `provider` {{< env "MICASA_CHAT_LLM_PROVIDER" >}} | string | `ollama` | LLM provider. Supported: `ollama`, `anthropic`, `openai`, `openrouter`, `deepseek`, `gemini`, `groq`, `mistral`, `llamacpp`, `llamafile`. Auto-detected from `base_url` and `api_key` when not set. |
| `base_url` {{< env "MICASA_CHAT_LLM_BASE_URL" >}} | string | `http://localhost:11434` | Root URL of the provider's API. No `/v1` suffix needed. |
| `model` {{< env "MICASA_CHAT_LLM_MODEL" >}} | string | `qwen3` | Model identifier sent in chat requests. |
| `api_key` {{< env "MICASA_CHAT_LLM_API_KEY" >}} | string | (empty) | Authentication credential. Required for cloud providers. Leave empty for local servers. |
| `timeout` {{< env "MICASA_CHAT_LLM_TIMEOUT" >}} | string | `"5m"` | Inference timeout for chat responses (including streaming). Go duration syntax, e.g. `"10m"`. |
| `thinking` {{< env "MICASA_CHAT_LLM_THINKING" >}} | string | (unset) | Model reasoning effort level. Supported: `none`, `low`, `medium`, `high`, `auto`. Empty = server default. |
| `extra_context` {{< env "MICASA_CHAT_LLM_EXTRA_CONTEXT" >}} | string | (empty) | Custom text appended to chat system prompts. Useful for domain-specific details about your house. Currency is handled automatically via `[locale]`. |
```

- [ ] **Step 3: Add annotations to `[extraction.llm]` section table**

```markdown
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enable` {{< env "MICASA_EXTRACTION_LLM_ENABLE" >}} | bool | `true` | Set to `false` to disable LLM-powered structured extraction. OCR and pdftotext still run. |
| `provider` {{< env "MICASA_EXTRACTION_LLM_PROVIDER" >}} | string | `ollama` | LLM provider for extraction. Same options as `[chat.llm]`. |
| `base_url` {{< env "MICASA_EXTRACTION_LLM_BASE_URL" >}} | string | `http://localhost:11434` | API base URL for extraction. |
| `model` {{< env "MICASA_EXTRACTION_LLM_MODEL" >}} | string | `qwen3` | Model for extraction. Extraction works well with small, fast models optimized for structured JSON output. |
| `api_key` {{< env "MICASA_EXTRACTION_LLM_API_KEY" >}} | string | (empty) | Authentication credential for extraction. |
| `timeout` {{< env "MICASA_EXTRACTION_LLM_TIMEOUT" >}} | string | `"5m"` | Extraction inference timeout. |
| `thinking` {{< env "MICASA_EXTRACTION_LLM_THINKING" >}} | string | (unset) | Reasoning effort level for extraction. |
```

- [ ] **Step 4: Add annotations to `[documents]` section table**

Add the deprecated `MICASA_DOCUMENTS_CACHE_TTL_DAYS` note to the `cache_ttl` description. Also add the missing `file_picker_dir` row -- it exists in the current env var table but was never added to the TOML section table:

```markdown
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `max_file_size` {{< env "MICASA_DOCUMENTS_MAX_FILE_SIZE" >}} | string or integer | `"50 MiB"` | Maximum file size for document imports. Accepts unitized strings (`"50 MiB"`, `"1.5 GiB"`) or bare integers (bytes). Must be positive. |
| `cache_ttl` {{< env "MICASA_DOCUMENTS_CACHE_TTL" >}} | string or integer | `"30d"` | Cache lifetime for extracted documents. Accepts `"30d"`, `"720h"`, or bare integers (seconds). Set to `"0s"` to disable eviction. Replaces the deprecated `MICASA_DOCUMENTS_CACHE_TTL_DAYS` env var. |
| `file_picker_dir` {{< env "MICASA_DOCUMENTS_FILE_PICKER_DIR" >}} | string | (Downloads) | Starting directory for the file picker. Defaults to the platform's Downloads directory. |
```

- [ ] **Step 5: Add annotations to `[extraction]` section table**

```markdown
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `max_pages` {{< env "MICASA_EXTRACTION_MAX_PAGES" >}} | int | `0` | Maximum pages to OCR per scanned document. 0 means no limit. |
```

- [ ] **Step 6: Add annotations to `[extraction.ocr]` section table**

```markdown
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enable` {{< env "MICASA_EXTRACTION_OCR_ENABLE" >}} | bool | `true` | Set to `false` to disable OCR on documents. When disabled, scanned pages and images produce no text. |
```

- [ ] **Step 7: Add annotations to `[extraction.ocr.tsv]` section table**

```markdown
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enable` {{< env "MICASA_EXTRACTION_OCR_TSV_ENABLE" >}} | bool | `true` | Set to `false` to disable spatial annotations sent to the LLM. |
| `confidence_threshold` {{< env "MICASA_EXTRACTION_OCR_TSV_CONFIDENCE_THRESHOLD" >}} | int | `70` | Confidence threshold (0-100). Lines with OCR confidence below this value include a confidence score; lines above omit it to save tokens. Set to 0 to never show confidence. |
```

- [ ] **Step 8: Add annotations to `[locale]` section table**

```markdown
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `currency` {{< env "MICASA_LOCALE_CURRENCY" >}} | string | (auto-detect) | ISO 4217 currency code (e.g. `USD`, `EUR`, `GBP`, `JPY`). Auto-detected from `LC_MONETARY`/`LANG` if not set, falls back to `USD`. Persisted to the database on first run -- after that the DB value is authoritative. |
```

- [ ] **Step 9: Verify Hugo build**

Run: `cd docs && nix develop -c hugo --logLevel warn`

Expected: clean build. If shortcodes inside table cells cause errors, fall back to inline HTML (`<span class="env-hint">MICASA_...</span>`) for all 22 rows.

- [ ] **Step 10: Verify rendered output has env-hint spans**

Run: `rg 'env-hint' docs/public/docs/reference/configuration/index.html | wc -l`

Expected: 23 (one per table row with an annotation).

- [ ] **Step 11: Commit**

```
docs(website): add inline env var annotations to config section tables

Each TOML key now shows its corresponding environment variable name as a
subtle gray annotation below the key. 23 rows across 8 section tables.
Added deprecation note for MICASA_DOCUMENTS_CACHE_TTL_DAYS to the
cache_ttl description.
```

---

### Task 4: Update override precedence section

**Files:**
- Modify: `docs/content/docs/reference/configuration.md` (override precedence subsection)

- [ ] **Step 1: Update the override precedence text**

Find the `### Override precedence` subsection. Replace:

```markdown
Environment variables override config file values. The full precedence order
(highest to lowest):

1. Environment variables (see [table above](#environment-variables))
2. Config file values
3. Built-in defaults
```

With:

```markdown
Environment variables override config file values. The full precedence order
(highest to lowest):

1. Environment variables
2. Config file values
3. Built-in defaults

Each config key has a corresponding env var shown in gray below the key name:
`MICASA_` + uppercase config path with dots replaced by underscores.
```

- [ ] **Step 2: Verify Hugo build**

Run: `cd docs && nix develop -c hugo --logLevel warn`

Expected: clean build. The `#environment-variables` anchor link is gone (deleted in Task 2) and no longer referenced.

- [ ] **Step 3: Verify no broken internal links remain**

Run: `rg '#environment-variables' docs/content/docs/reference/configuration.md`

Expected: no matches.

- [ ] **Step 4: Commit**

```
docs(website): update override precedence with env var naming convention

Remove the broken link to the deleted env var section and add a one-liner
explaining the mechanical naming convention.
```

---

### Task 5: Final verification

**Files:** (none -- verification only)

- [ ] **Step 1: Full Hugo build**

Run: `cd docs && nix develop -c hugo --logLevel warn`

Expected: clean build, zero warnings.

- [ ] **Step 2: Spot-check rendered HTML**

Verify a few key annotations appear correctly in the rendered output:

```bash
rg 'MICASA_CHAT_LLM_MODEL' docs/public/docs/reference/configuration/index.html
rg 'MICASA_EXTRACTION_OCR_TSV_CONFIDENCE_THRESHOLD' docs/public/docs/reference/configuration/index.html
rg 'MICASA_DOCUMENTS_CACHE_TTL_DAYS' docs/public/docs/reference/configuration/index.html
```

Expected:
- `MICASA_CHAT_LLM_MODEL` appears inside a `<span class="env-hint">`
- `MICASA_EXTRACTION_OCR_TSV_CONFIDENCE_THRESHOLD` appears inside a `<span class="env-hint">`
- `MICASA_DOCUMENTS_CACHE_TTL_DAYS` appears in the `cache_ttl` description text (not as an env-hint)

- [ ] **Step 3: Verify the deleted section is truly gone**

Run: `rg '## Environment variables' docs/content/docs/reference/configuration.md`

Expected: no matches.

- [ ] **Step 4: Count env-hint occurrences**

Run: `rg -c 'env-hint' docs/public/docs/reference/configuration/index.html`

Expected: 23 occurrences (one per annotated table row).
