<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Inline env var annotations for configuration docs

Date: 2026-03-30

## Problem

The configuration reference page at `/docs/reference/configuration/` has two
competing information architectures: a large env var table at the top listing
all ~25 environment variables, and per-section TOML docs below with their own
key/type/default/description tables. The env var names are long
(`MICASA_EXTRACTION_OCR_TSV_CONFIDENCE_THRESHOLD`) and cause ugly line wrapping
in the table. The information is duplicated -- every env var maps 1:1 to a TOML
key that's already documented in the section tables.

## Design

### Delete the standalone env var section

Remove entirely:
- The `## Environment variables` heading
- The naming convention explanation
- The big 4-column env var table
- All per-env-var subsections (`### MICASA_DB_PATH`, `### MICASA_CHAT_LLM_MODEL`,
  `### MICASA_CHAT_LLM_TIMEOUT`, `### MICASA_DOCUMENTS_MAX_FILE_SIZE`,
  `### MICASA_DOCUMENTS_CACHE_TTL`, `### MICASA_DOCUMENTS_CACHE_TTL_DAYS`)
- The `### Platform data directory` subsection stays -- it's about file paths,
  not env vars. Promote it to `## Platform data directory` since its parent
  section is gone.
- The `## Database path resolution order` section stays as-is (already
  references `MICASA_DB_PATH` naturally).

### Add env var annotations to TOML section tables

In the markdown content, each row's Key cell gains a second line showing the
env var name. This is rendered as a `<span class="env-hint">` via a Hugo
shortcode or render hook.

Visual treatment (CSS):
- `display: block` below the `<code>` key name
- `font-family: JetBrains Mono` (matches existing code font)
- `font-size: 0.7em` relative to the table cell
- `color: var(--warm-gray)` (#9e958a light, #706760 dark)
- `margin-top: 0.15em`
- `letter-spacing: -0.01em` (slight tightening for long names)
- No background, no border -- just color and size differentiation

Row height impact: ~30% taller per row. Acceptable per user review of mockup.

### Sections receiving annotations

Every TOML section table gets annotations on every row:

- `[chat]` -- 1 row (`enable`)
- `[chat.llm]` -- 7 rows (`provider`, `base_url`, `model`, `api_key`,
  `timeout`, `thinking`, `extra_context`)
- `[extraction]` -- 1 row (`max_pages`)
- `[extraction.llm]` -- 7 rows (`enable`, `provider`, `base_url`, `model`,
  `api_key`, `timeout`, `thinking`)
- `[extraction.ocr]` -- 1 row (`enable`)
- `[extraction.ocr.tsv]` -- 2 rows (`enable`, `confidence_threshold`)
- `[documents]` -- 2 rows (`max_file_size`, `cache_ttl`)
- `[locale]` -- 1 row (`currency`)

Total: 22 rows across 8 section tables.

### Env var name derivation

Each annotation is derived mechanically: `MICASA_` + section path (dots become
underscores) + `_` + uppercase key name.

Examples:
- `[chat.llm]` key `model` -> `MICASA_CHAT_LLM_MODEL`
- `[extraction.ocr.tsv]` key `confidence_threshold` ->
  `MICASA_EXTRACTION_OCR_TSV_CONFIDENCE_THRESHOLD`

### Fold format details into TOML section descriptions

The per-env-var subsections contain useful format information that isn't
redundant with the table descriptions. Fold these into the existing Description
cells:

- `timeout` fields: already mention "Go duration syntax" -- no change needed.
- `max_file_size`: already says "Accepts unitized strings or bare integers" --
  no change needed.
- `cache_ttl`: already says "Accepts day-suffixed strings, Go durations, or
  bare integers" -- no change needed.
- Deprecated `MICASA_DOCUMENTS_CACHE_TTL_DAYS`: add a note to the `cache_ttl`
  description: "Replaces the deprecated `MICASA_DOCUMENTS_CACHE_TTL_DAYS` env
  var."

### Update override precedence section

Current text references `(see [table above](#environment-variables))`. Replace
with:

> Environment variables override config file values. The full precedence order
> (highest to lowest):
>
> 1. Environment variables
> 2. Config file values
> 3. Built-in defaults
>
> Each config key has a corresponding env var shown in gray below the key name:
> `MICASA_` + uppercase config path with dots replaced by underscores.

### Implementation approach: markdown with Hugo shortcode

Use a Hugo shortcode `env` to render the annotation inline in markdown table
cells. This keeps the markdown readable and avoids raw HTML in content files.
Hugo expands shortcodes before markdown rendering, so the resulting
`<span>` is treated as inline HTML by goldmark within the table cell.

Shortcode definition (`docs/layouts/shortcodes/env.html`):
```html
<span class="env-hint">{{ .Get 0 }}</span>
```

Markdown usage in table cells:
```markdown
| `model` {{< env "MICASA_CHAT_LLM_MODEL" >}} | string | `qwen3` | Model identifier. |
```

If shortcodes cause issues inside table cells (historical goldmark edge case),
fall back to inline HTML: `<span class="env-hint">MICASA_...</span>`.

CSS addition to `docs/static/css/docs.css`:
```css
.env-hint {
  display: block;
  font-family: "JetBrains Mono", "Fira Code", "Consolas", monospace;
  font-size: 0.7em;
  color: var(--warm-gray);
  margin-top: 0.15em;
  letter-spacing: -0.01em;
}
```

### Fallback plan

If the annotation approach makes the tables feel too cluttered after
implementation, fall back to a global toggle (option B from brainstorming).
This would require:
- A toggle button near the "Config file" heading
- JS to show/hide `.env-hint` elements
- CSS transition for the reveal animation
- localStorage to persist the toggle state

This fallback is explicitly deferred -- only pursue if the user rejects the
annotation approach after seeing it live.

## Files changed

- `docs/content/docs/reference/configuration.md` -- content restructuring
- `docs/static/css/docs.css` -- `.env-hint` class
- `docs/layouts/shortcodes/env.html` -- new shortcode (1 line)

## Out of scope

- Dark mode adjustments: `var(--warm-gray)` already adapts via the existing
  CSS variable theming.
- Mobile responsive changes: annotations are small enough to not cause issues
  at narrow widths.
- The table horizontal scroll wrapper added earlier in this session stays --
  it's good defensive CSS regardless of this change.
