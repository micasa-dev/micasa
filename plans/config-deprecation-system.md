<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Config Deprecation System: Struct-Tag-Driven Key Migration

## Problem

Config keys get renamed over time (e.g., `thinking` -> `effort`). Each
rename currently requires hand-editing multiple maps (`removedKeys`,
`deprecatedPaths`), updating docs manually, and hoping everything stays
in sync. There is no systematic way to declare a deprecation once and
have it surface consistently in error messages and the Hugo docs.

## Design

### Single source of truth: struct tags

A `deprecated` tag on the **new** config field declares the old TOML key
name. The new name comes from the `toml` tag on the same field.

```go
Effort string `toml:"effort,omitempty" deprecated:"thinking" validate:"..."`
```

Everything else -- runtime error maps and docs data -- is derived from
these tags.

### Tag format

**Simple rename (same values):**

```go
deprecated:"old_key"
```

The old key is relative to the same TOML section. On `ChatLLM.Effort`,
the full deprecated path is `chat.llm.thinking`.

**Rename with value transform:**

```go
deprecated:"old_key" deprecated_transform:"days_to_duration"
```

The transform name references a registered hint string (not a function)
that describes the value conversion rule. Appended to the error message
so the user knows how to translate their old value.

### Runtime behavior

#### Init-time tag walking

A shared `Deprecations()` function in `deprecated.go` walks `Config{}`
via reflection and returns `[]Deprecation` structs:

```go
type Deprecation struct {
    OldPath   string // e.g. "chat.llm.thinking"
    NewPath   string // e.g. "chat.llm.effort"
    Transform string // e.g. "days_to_duration" (empty = no transform)
}
```

The `Transform` field comes from the optional `deprecated_transform`
struct tag. Both runtime init (for error message hints) and the code
generator (for docs) consume it from the same struct.

Both the runtime `init()` and the code generator consume this single
function — no duplicated tag-walking logic.

`Deprecations()` returns entries in struct-field reflection order
(depth-first walk of `Config{}`), which is deterministic.

`init()` calls `Deprecations()` once and builds two derived maps:

- `removedKeys map[string]string` -- old TOML path -> replacement
  message. Replaces the current hand-maintained map in `validate.go`.
- `deprecatedEnvVars map[string]Deprecation` -- old env var name ->
  full deprecation metadata (new env var name + transform hint). Built
  by applying `EnvVarName()` to old and new paths from each
  `Deprecation`.

#### Why no `config get` annotations

Replacement info does **not** appear in `ShowConfig` / `config get .`
output. The existing `deprecatedPaths` map and its rendering logic in
`show.go` are removed (the map was always empty). Showing badges in
`config get .` is problematic: fields like `cache_ttl` get default
values populated by `forDisplay()`, which would make fresh installs
always render "Replaces `cache_ttl_days`" even when the user never used
the old key. The migration surfaces are:

- Hard error at load time (primary)
- Hugo docs badge (reference)

#### Load-time checks

`LoadFromPath()` flow (unchanged structure, new data sources):

1. `toml.DecodeFile()` -- old keys land in undecoded metadata
2. `checkRemovedKeys(md)` -- matches undecoded keys against the
   derived `removedKeys` map. Hard error:
   `"chat.llm.thinking was removed -- use chat.llm.effort instead"`
3. `checkDeprecatedEnvVars()` -- pre-scans deprecated env vars before
   canonical overrides run. Uses `os.Getenv` (empty = unset, consistent
   with `applyEnvOverrides`). Iterates over `Deprecations()` slice
   (deterministic order). Hard error on the first non-empty deprecated
   env var:
   `"MICASA_CHAT_LLM_THINKING was removed -- use MICASA_CHAT_LLM_EFFORT instead"`
   Errors even if the replacement env var is also set -- the user must
   remove the deprecated one.
4. `applyEnvOverrides()` -- applies canonical env var overrides (unchanged)
5. `validate()` -- unchanged

#### Error messages

| Scenario | Message |
|----------|---------|
| TOML key `thinking = "medium"` | `chat.llm.thinking was removed -- use chat.llm.effort instead` |
| Env var `MICASA_CHAT_LLM_THINKING=high` | `MICASA_CHAT_LLM_THINKING was removed -- use MICASA_CHAT_LLM_EFFORT instead` |
| TOML key `cache_ttl_days = 30` | `documents.cache_ttl_days was removed -- use documents.cache_ttl instead (integer days become duration strings, e.g. 30 becomes 30d)` |
| Env var `MICASA_DOCUMENTS_CACHE_TTL_DAYS=30` | `MICASA_DOCUMENTS_CACHE_TTL_DAYS was removed -- use MICASA_DOCUMENTS_CACHE_TTL instead (integer days become duration strings, e.g. 30 becomes 30d)` |

### Docs generation

A `go:generate` directive in `internal/config/deprecated.go` invokes
the generator (`internal/config/cmd/gendeprecations/main.go`). The
generator accepts an `-output` flag for the target path. `go generate`
runs from the package directory (`internal/config`), so the path is
relative to that working directory:

```go
//go:generate go run ./cmd/gendeprecations -output ../../docs/data/deprecations.json
```

The generator calls the shared `Deprecations()` function and writes
`docs/data/deprecations.json`:

```json
[
  {
    "old_path": "chat.llm.thinking",
    "new_path": "chat.llm.effort"
  },
  {
    "old_path": "extraction.llm.thinking",
    "new_path": "extraction.llm.effort"
  },
  {
    "old_path": "documents.cache_ttl_days",
    "new_path": "documents.cache_ttl",
    "hint": "integer days become duration strings, e.g. 30 becomes 30d"
  }
]
```

Records use fully qualified paths to avoid ambiguity when different
sections rename different old keys to the same leaf name.

A `{{< replaces >}}` Hugo shortcode reads this data file and renders an
inline badge in the config reference table:

```
| `effort` {{< env "..." >}} {{< replaces "chat.llm.effort" >}} | string | ...
```

Renders as: "Replaces `thinking`."

When the deprecation has a `hint` field (transform guidance), the
shortcode appends it: "Replaces `cache_ttl_days`. Integer days become
duration strings, e.g. 30 becomes 30d."

The shortcode takes the full new path, looks it up in the deprecations
data, and extracts the old leaf name and optional hint for display.

### Value transforms

For renames where the value representation changes, an additional
`deprecated_transform` tag names a registered hint string:

```go
var transformHints = map[string]string{
    "days_to_duration": "integer days become duration strings, e.g. 30 becomes 30d",
}
```

The hint is appended to the error message as a rule description, not a
value-specific conversion. `checkRemovedKeys` only has access to the
key name from undecoded TOML metadata, not the old value, so the
message describes the transformation rule rather than the specific
value:

```
documents.cache_ttl_days was removed -- use documents.cache_ttl instead
(integer days become duration strings, e.g. 30 becomes 30d)
```

The existing `cache_ttl_days` -> `cache_ttl` deprecation is modeled
with this system to prove it works:

```go
CacheTTL *Duration `toml:"cache_ttl,omitempty" deprecated:"cache_ttl_days" deprecated_transform:"days_to_duration" validate:"..."`
```

This replaces the hand-maintained `removedKeys` entry for
`cache_ttl_days` and serves as the reference implementation for
value-changing renames.

## Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Rename `Thinking` to `Effort`, add `deprecated:"thinking"` tag, update `ExampleTOML` comments |
| `internal/config/deprecated.go` | New: shared `Deprecations()` walker + `Deprecation` type (validates no duplicate old paths or unknown transforms); `init()` derives `removedKeys`/`deprecatedEnvVars` (validates no env var collisions with each other or with canonical env vars from `EnvVars()`) |
| `internal/config/validate.go` | Remove hand-maintained `removedKeys` map; use derived one |
| `internal/config/show.go` | Remove `deprecatedPaths` map and its rendering logic in `walkSections` |
| `internal/config/cmd/gendeprecations/main.go` | New: JSON generator consuming shared `Deprecations()` |
| `docs/data/deprecations.json` | New: generated deprecation metadata |
| `docs/layouts/shortcodes/replaces.html` | New: inline badge shortcode |
| `docs/content/docs/reference/configuration.md` | `thinking` -> `effort`, add `{{< replaces >}}` |
| `internal/llm/client.go` | Rename `thinking` field to `effort`, `SetThinking` to `SetEffort` |
| `internal/app/types.go` | Rename `Thinking` fields to `Effort`, update `extractionThinking` to `extractionEffort` |
| `internal/app/model.go` | Update references |
| `cmd/micasa/main.go` | `.Thinking` -> `.Effort` |
| Tests | All `*_test.go` files referencing `thinking` updated |

## Testing

- **Tag walker:** Verify `removedKeys` and `deprecatedEnvVars` maps are
  correctly built from struct tags on `Config`.
- **TOML load:** Config with `thinking = "medium"` returns the expected
  hard error mentioning `effort`.
- **Env var load:** `MICASA_CHAT_LLM_THINKING=high` returns the expected
  hard error mentioning `MICASA_CHAT_LLM_EFFORT`.
- **Both sections:** Test `chat.llm.thinking` and
  `extraction.llm.thinking` independently.
- **Transform hint (TOML):** Config with `cache_ttl_days = 30` returns
  hard error including the transform hint string.
- **Transform hint (env var):** `MICASA_DOCUMENTS_CACHE_TTL_DAYS=30`
  returns hard error including the same transform hint.
- **Validation:** `Deprecations()` panics on duplicate `OldPath`
  entries and unregistered `deprecated_transform` names. `init()`
  panics on duplicate derived env var names and on collisions between
  deprecated env var names and canonical env var names from `EnvVars()`.
- **Both env vars set:** Test that setting both `MICASA_CHAT_LLM_THINKING`
  and `MICASA_CHAT_LLM_EFFORT` returns a hard error on the deprecated
  var.
- **Deterministic scan order:** Test that setting two deprecated env vars
  simultaneously (e.g. `MICASA_CHAT_LLM_THINKING` and
  `MICASA_EXTRACTION_LLM_THINKING`) reports the first one in
  `Deprecations()` slice order.
- **Generator freshness:** CI check that `docs/data/deprecations.json`
  matches what the generator produces (same pattern as other generated
  files).
- **Hugo shortcode (no hint):** Verify `{{< replaces "chat.llm.effort" >}}`
  resolves to "Replaces `thinking`" from the generated data file.
- **Hugo shortcode (with hint):** Verify `{{< replaces "documents.cache_ttl" >}}`
  renders "Replaces `cache_ttl_days`. Integer days become duration
  strings, e.g. 30 becomes 30d."
- **Empty deprecated env var:** Verify `MICASA_CHAT_LLM_THINKING=""`
  does not trigger the hard error (consistent with empty = unset).
- **Existing tests:** All references to `thinking` become `effort`.

## Known limitations

- **Single predecessor per field.** The `deprecated` tag supports one
  old key name. If a key is renamed a second time (A -> B -> C), the
  tag on C can only reference B. The A -> B entry would need to stay
  in a manual map or the tag format extended to comma-separated values
  (`deprecated:"B,A"`). This is not needed now — extend when it is.

- **Same-section renames only.** The old key is relative to the same
  TOML section as the new field. Cross-section moves (e.g.,
  `chat.thinking` -> `chat.llm.effort`) cannot be expressed with the
  tag format. No cross-section move has occurred or is planned. If one
  arises, the tag format can accept absolute paths (starting with a
  section prefix) as an escape hatch.

## Non-goals

- No runtime migration (silently copying old value to new field). The
  user must update their config.
- No deprecation warnings. Old keys are hard errors, consistent with the
  existing `cache_ttl_days` -> `cache_ttl` precedent.
- No backward-compatible shims or aliases.
