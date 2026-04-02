<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Config Deprecation System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename `thinking` to `effort` in the config system using a new struct-tag-driven deprecation framework that derives error maps and docs data from a single source of truth.

**Architecture:** A `deprecated` struct tag on the new config field declares the old TOML key. A shared `Deprecations()` function walks `Config{}` via reflection and returns structured metadata. Runtime `init()` builds error maps from this; a code generator writes Hugo data from it. Existing `cache_ttl_days` removal migrates to the same system.

**Tech Stack:** Go (reflection, struct tags), Hugo shortcodes, `go generate`

**Spec:** `plans/config-deprecation-system.md`

---

### Task 1: Create `deprecated.go` with tag walker and types

**Files:**
- Create: `internal/config/deprecated.go`
- Create: `internal/config/deprecated_test.go`

This is the core of the system. Everything else depends on it.

- [ ] **Step 1: Write the `Deprecation` type and `transformHints` map**

Create `internal/config/deprecated.go`:

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"fmt"
	"os"
	"reflect"
)

// Deprecation describes a config key that was renamed.
type Deprecation struct {
	OldPath   string // e.g. "chat.llm.thinking"
	NewPath   string // e.g. "chat.llm.effort"
	Transform string // e.g. "days_to_duration" (empty = no transform)
}

// transformHints maps transform names to human-readable hint strings
// appended to error messages for value-changing renames.
var transformHints = map[string]string{
	"days_to_duration": "integer days become duration strings, e.g. 30 becomes 30d",
}

// Deprecations walks the Config struct via reflection and returns all
// deprecation entries derived from `deprecated` struct tags. Entries are
// returned in struct-field reflection order (depth-first walk), which is
// deterministic. Panics on duplicate OldPath entries or unregistered
// deprecated_transform names.
func Deprecations() []Deprecation {
	var deps []Deprecation
	seen := make(map[string]bool)
	walkDeprecatedTags(reflect.TypeOf(Config{}), "", &deps, seen)
	return deps
}

func walkDeprecatedTags(t reflect.Type, prefix string, deps *[]Deprecation, seen map[string]bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		tomlName := tomlTagName(f)
		if tomlName == "" {
			continue
		}

		path := tomlName
		if prefix != "" {
			path = prefix + "." + tomlName
		}

		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		// Recurse into nested config sections.
		if isConfigSection(ft) {
			walkDeprecatedTags(ft, path, deps, seen)
			continue
		}

		oldKey := f.Tag.Get("deprecated")
		if oldKey == "" {
			continue
		}

		oldPath := oldKey
		if prefix != "" {
			oldPath = prefix + "." + oldKey
		}

		if seen[oldPath] {
			panic(fmt.Sprintf("duplicate deprecated old path: %q", oldPath))
		}
		seen[oldPath] = true

		transform := f.Tag.Get("deprecated_transform")
		if transform != "" {
			if _, ok := transformHints[transform]; !ok {
				panic(fmt.Sprintf(
					"unregistered deprecated_transform %q on %s",
					transform, path,
				))
			}
		}

		*deps = append(*deps, Deprecation{
			OldPath:   oldPath,
			NewPath:   path,
			Transform: transform,
		})
	}
}

// removedKeys is derived from struct tags at init time. Maps old TOML
// path -> error message. Replaces the hand-maintained map in validate.go.
var removedKeys map[string]string

// deprecatedEnvVars is derived from struct tags at init time. Maps old
// env var name -> Deprecation metadata.
var deprecatedEnvVars map[string]Deprecation

// deprecatedEnvVarOrder preserves Deprecations() slice order for
// deterministic iteration in checkDeprecatedEnvVars.
var deprecatedEnvVarOrder []string

func init() {
	deps := Deprecations()

	removedKeys = make(map[string]string, len(deps))
	for _, d := range deps {
		msg := fmt.Sprintf("%s was removed -- use %s instead", d.OldPath, d.NewPath)
		if d.Transform != "" {
			msg += " (" + transformHints[d.Transform] + ")"
		}
		removedKeys[d.OldPath] = msg
	}

	deprecatedEnvVars, deprecatedEnvVarOrder = buildDeprecatedEnvVars(deps, EnvVars())
}

// buildDeprecatedEnvVars constructs the deprecated env var map and
// ordered key slice. Panics on duplicate env var names or collisions
// with canonical env vars. Extracted from init() for testability.
func buildDeprecatedEnvVars(deps []Deprecation, canonical map[string]string) (
	map[string]Deprecation, []string,
) {
	envVars := make(map[string]Deprecation, len(deps))
	var order []string
	seen := make(map[string]bool, len(deps))

	for _, d := range deps {
		oldEnv := EnvVarName(d.OldPath)
		if seen[oldEnv] {
			panic(fmt.Sprintf("duplicate deprecated env var: %s", oldEnv))
		}
		if _, ok := canonical[oldEnv]; ok {
			panic(fmt.Sprintf(
				"deprecated env var %s collides with canonical env var",
				oldEnv,
			))
		}
		seen[oldEnv] = true
		envVars[oldEnv] = d
		order = append(order, oldEnv)
	}
	return envVars, order
}

// removedKeyMessage returns the error message for a removed TOML key,
// or empty string if the key is not deprecated.
func removedKeyMessage(path string) string {
	return removedKeys[path]
}

// checkDeprecatedEnvVars scans deprecated env var names and returns a
// hard error if any non-empty deprecated env var is set. Iterates in
// Deprecations() slice order for deterministic error messages.
func checkDeprecatedEnvVars() error {
	for _, oldEnv := range deprecatedEnvVarOrder {
		if val := os.Getenv(oldEnv); val != "" {
			d := deprecatedEnvVars[oldEnv]
			newEnv := EnvVarName(d.NewPath)
			msg := fmt.Sprintf(
				"%s was removed -- use %s instead",
				oldEnv, newEnv,
			)
			if d.Transform != "" {
				msg += " (" + transformHints[d.Transform] + ")"
			}
			return fmt.Errorf("%s", msg)
		}
	}
	return nil
}

```

Note: `checkDeprecatedEnvVars` uses `os.Getenv` directly (see the function body above). `"os"` is already in the import block. Tests use `t.Setenv` which works with `os.Getenv`.

- [ ] **Step 2: Write tests for `Deprecations()` and init maps**

Create `internal/config/deprecated_test.go`:

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeprecationsReturnsEntries(t *testing.T) {
	deps := Deprecations()
	require.NotEmpty(t, deps)

	// Verify thinking -> effort appears for both LLM sections.
	var chatEffort, exEffort bool
	for _, d := range deps {
		switch d.OldPath {
		case "chat.llm.thinking":
			chatEffort = true
			assert.Equal(t, "chat.llm.effort", d.NewPath)
			assert.Empty(t, d.Transform)
		case "extraction.llm.thinking":
			exEffort = true
			assert.Equal(t, "extraction.llm.effort", d.NewPath)
			assert.Empty(t, d.Transform)
		}
	}
	assert.True(t, chatEffort, "expected chat.llm.thinking deprecation")
	assert.True(t, exEffort, "expected extraction.llm.thinking deprecation")
}

func TestDeprecationsIncludesCacheTTLDays(t *testing.T) {
	deps := Deprecations()
	var found bool
	for _, d := range deps {
		if d.OldPath == "documents.cache_ttl_days" {
			found = true
			assert.Equal(t, "documents.cache_ttl", d.NewPath)
			assert.Equal(t, "days_to_duration", d.Transform)
		}
	}
	assert.True(t, found, "expected documents.cache_ttl_days deprecation")
}

func TestRemovedKeysMapBuiltFromTags(t *testing.T) {
	// removedKeys is populated by init().
	require.NotEmpty(t, removedKeys)

	msg, ok := removedKeys["chat.llm.thinking"]
	require.True(t, ok)
	assert.Contains(t, msg, "chat.llm.effort")

	msg, ok = removedKeys["documents.cache_ttl_days"]
	require.True(t, ok)
	assert.Contains(t, msg, "documents.cache_ttl")
	assert.Contains(t, msg, "integer days become duration strings")
}

func TestDeprecatedEnvVarsMapBuiltFromTags(t *testing.T) {
	require.NotEmpty(t, deprecatedEnvVars)

	d, ok := deprecatedEnvVars["MICASA_CHAT_LLM_THINKING"]
	require.True(t, ok)
	assert.Equal(t, "chat.llm.effort", d.NewPath)

	d, ok = deprecatedEnvVars["MICASA_EXTRACTION_LLM_THINKING"]
	require.True(t, ok)
	assert.Equal(t, "extraction.llm.effort", d.NewPath)

	d, ok = deprecatedEnvVars["MICASA_DOCUMENTS_CACHE_TTL_DAYS"]
	require.True(t, ok)
	assert.Equal(t, "documents.cache_ttl", d.NewPath)
	assert.Equal(t, "days_to_duration", d.Transform)
}

func TestDeprecationsOrderIsDeterministic(t *testing.T) {
	d1 := Deprecations()
	d2 := Deprecations()
	require.Equal(t, d1, d2, "Deprecations() must return deterministic order")
}

func TestWalkDeprecatedTagsPanicsOnDuplicateOldPath(t *testing.T) {
	// Use a synthetic struct with duplicate deprecated old keys.
	type dupSection struct {
		A string `toml:"a" deprecated:"old"`
		B string `toml:"b" deprecated:"old"`
	}
	assert.Panics(t, func() {
		var deps []Deprecation
		seen := make(map[string]bool)
		walkDeprecatedTags(reflect.TypeOf(dupSection{}), "test", &deps, seen)
	}, "duplicate old path should panic")
}

func TestWalkDeprecatedTagsPanicsOnUnknownTransform(t *testing.T) {
	type badTransform struct {
		A string `toml:"a" deprecated:"old" deprecated_transform:"nonexistent"`
	}
	assert.Panics(t, func() {
		var deps []Deprecation
		seen := make(map[string]bool)
		walkDeprecatedTags(reflect.TypeOf(badTransform{}), "test", &deps, seen)
	}, "unregistered transform should panic")
}

func TestBuildDeprecatedEnvVarsPanicsOnCanonicalCollision(t *testing.T) {
	// buildDeprecatedEnvVars is the extracted helper called by init().
	// Pass a synthetic deprecation whose old env var collides with a
	// canonical one (MICASA_CHAT_LLM_MODEL).
	deps := []Deprecation{{OldPath: "chat.llm.model", NewPath: "chat.llm.new_model"}}
	canonical := EnvVars()
	assert.Panics(t, func() {
		buildDeprecatedEnvVars(deps, canonical)
	}, "deprecated env var colliding with canonical should panic")
}

func TestBuildDeprecatedEnvVarsPanicsOnEnvVarCollision(t *testing.T) {
	// Two distinct TOML paths that normalize to the same env var:
	// EnvVarName("a.b_c") == EnvVarName("a_b.c") == "MICASA_A_B_C"
	deps := []Deprecation{
		{OldPath: "a.b_c", NewPath: "a.new1"},
		{OldPath: "a_b.c", NewPath: "a.new2"},
	}
	assert.Panics(t, func() {
		buildDeprecatedEnvVars(deps, map[string]string{})
	}, "distinct paths colliding to same env var should panic")
}
```

Add `"reflect"` to the test imports.

**Important:** This requires extracting the env var map building from `init()` into a `buildDeprecatedEnvVars` helper. In `deprecated.go`, the init function should call:

```go
func buildDeprecatedEnvVars(deps []Deprecation, canonical map[string]string) (
	map[string]Deprecation, []string,
) {
	envVars := make(map[string]Deprecation, len(deps))
	var order []string
	seen := make(map[string]bool, len(deps))

	for _, d := range deps {
		oldEnv := EnvVarName(d.OldPath)
		if seen[oldEnv] {
			panic(fmt.Sprintf("duplicate deprecated env var: %s", oldEnv))
		}
		if _, ok := canonical[oldEnv]; ok {
			panic(fmt.Sprintf(
				"deprecated env var %s collides with canonical env var",
				oldEnv,
			))
		}
		seen[oldEnv] = true
		envVars[oldEnv] = d
		order = append(order, oldEnv)
	}
	return envVars, order
}
```

And `init()` calls it:

```go
deprecatedEnvVars, deprecatedEnvVarOrder = buildDeprecatedEnvVars(deps, EnvVars())
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestDeprecations -v`

Expected: FAIL — `Effort` field with `deprecated` tag doesn't exist yet. Do not commit yet — proceed to Task 2 to make the tests pass.

---

### Task 2: Rename `Thinking` to `Effort` in config structs

**Files:**
- Modify: `internal/config/config.go` (ChatLLM, ExtractionLLM, ExampleTOML)

- [ ] **Step 1: Rename ChatLLM.Thinking to Effort and add deprecated tag**

In `internal/config/config.go`, change the `ChatLLM` struct field:

```go
// Before:
Thinking string `toml:"thinking,omitempty" validate:"omitempty,oneof=none low medium high auto"`

// After:
Effort string `toml:"effort,omitempty" deprecated:"thinking" validate:"omitempty,oneof=none low medium high auto"`
```

- [ ] **Step 2: Rename ExtractionLLM.Thinking to Effort and add deprecated tag**

Same change in the `ExtractionLLM` struct:

```go
// Before:
Thinking string `toml:"thinking,omitempty" validate:"omitempty,oneof=none low medium high auto"`

// After:
Effort string `toml:"effort,omitempty" deprecated:"thinking" validate:"omitempty,oneof=none low medium high auto"`
```

- [ ] **Step 3: Add deprecated tag to Documents.CacheTTL**

In the `Documents` struct, add the deprecated tag:

```go
// Before:
CacheTTL *Duration `toml:"cache_ttl,omitempty" validate:"omitempty,nonneg_duration"`

// After:
CacheTTL *Duration `toml:"cache_ttl,omitempty" deprecated:"cache_ttl_days" deprecated_transform:"days_to_duration" validate:"omitempty,nonneg_duration"`
```

- [ ] **Step 4: Update ExampleTOML comments**

In `ExampleTOML()`, replace all `thinking` references with `effort`:

```go
// Before:
# thinking = "medium"
// After:
# effort = "medium"

// Before:
# thinking = "low"
// After:
# effort = "low"
```

Also update the comment text:

```go
// Before:
# Model reasoning effort level. Supported: none, low, medium, high, auto.
# Empty = don't send (server default).
# thinking = "medium"

// After:
# Model reasoning effort level. Supported: none, low, medium, high, auto.
# Empty = don't send (server default).
# effort = "medium"
```

- [ ] **Step 5: Update field doc comments**

Change the doc comment on both fields from:

```go
// Thinking controls the model's reasoning effort level.
```

to:

```go
// Effort controls the model's reasoning effort level.
```

- [ ] **Step 6: Run deprecated_test.go**

Run: `go test ./internal/config/ -run TestDeprecations -v`

Expected: PASS — the tag walker now finds the `deprecated:"thinking"` tags.

- [ ] **Step 7: Commit**

---

### Task 3: Remove hand-maintained maps and wire derived maps

**Files:**
- Modify: `internal/config/validate.go`
- Modify: `internal/config/show.go`
- Modify: `internal/config/config.go` (LoadFromPath)

- [ ] **Step 1: Remove the hand-maintained `removedKeys` map from validate.go**

In `internal/config/validate.go`, delete:

```go
// removedKeys maps TOML key paths that were removed to their replacement.
// checkRemovedKeys returns an actionable error if any are present.
var removedKeys = map[string]string{
	"documents.cache_ttl_days": "documents.cache_ttl",
}
```

The `removedKeys` var is now declared in `deprecated.go` and populated by `init()`.

- [ ] **Step 2: Update `checkRemovedKeys` to use derived map format**

The derived `removedKeys` maps old path -> full error message (not just the replacement key). Update `checkRemovedKeys`:

```go
// Before:
func checkRemovedKeys(md toml.MetaData) error {
	for _, key := range md.Undecoded() {
		path := key.String()
		if replacement, ok := removedKeys[path]; ok {
			return fmt.Errorf(
				"%s was removed -- use %s instead",
				path, replacement,
			)
		}
	}
	return nil
}

// After:
func checkRemovedKeys(md toml.MetaData) error {
	for _, key := range md.Undecoded() {
		path := key.String()
		if msg, ok := removedKeys[path]; ok {
			return fmt.Errorf("%s", msg)
		}
	}
	return nil
}
```

- [ ] **Step 3: Remove `deprecatedPaths` map and rendering logic from show.go**

In `internal/config/show.go`, delete:

```go
// deprecatedPaths maps deprecated TOML key paths to a human-readable
// replacement hint shown in ShowConfig output.
var deprecatedPaths = map[string]string{}
```

And in `walkSections`, remove the deprecation annotation block:

```go
		if replacement, ok := deprecatedPaths[path]; ok {
			dep := "DEPRECATED: use " + replacement
			if comment != "" {
				comment = comment + "; " + dep
			} else {
				comment = dep
			}
		}
```

- [ ] **Step 4: Add `checkDeprecatedEnvVars()` call to `LoadFromPath`**

In `internal/config/config.go`, in the `LoadFromPath` function, add the check after `checkRemovedKeys` and before `applyEnvOverrides`:

```go
// Before:
	if err := applyEnvOverrides(&cfg, nil); err != nil {
		return cfg, err
	}

// After:
	if err := checkDeprecatedEnvVars(); err != nil {
		return cfg, err
	}

	if err := applyEnvOverrides(&cfg, nil); err != nil {
		return cfg, err
	}
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./internal/config/`

Expected: Compiles (tests won't compile yet due to `.Thinking` references). Do not commit yet — proceed to Task 4 to fix the tests.

---

### Task 4: Update all existing tests for thinking -> effort

**Files:**
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/show_test.go`

- [ ] **Step 1: Update config_test.go**

All references to `Thinking` field, `thinking` TOML key, and `MICASA_*_THINKING` env vars:

1. `TestInvalidThinkingLevelReturnsError` — rename to `TestInvalidEffortLevelReturnsError`. Change TOML keys from `thinking` to `effort`. Change error assertions from `chat.llm.thinking` to `chat.llm.effort`. Change field access from `.Thinking` to `.Effort`.

2. `TestChatAndExtractionAreIndependent` — change TOML `thinking = "high"` to `effort = "high"`, `thinking = "low"` to `effort = "low"`. Change assertions from `.Thinking` to `.Effort`.

3. `TestEnvVars` — change `"MICASA_CHAT_LLM_THINKING": "chat.llm.thinking"` to `"MICASA_CHAT_LLM_EFFORT": "chat.llm.effort"`. Same for extraction.

4. `TestCacheTTLDaysRemovedReturnsError` — this test should now also check for the transform hint. Add:
```go
assert.Contains(t, err.Error(), "days_to_duration")
```
Wait — the message uses the hint text, not the transform key. Update to:
```go
assert.Contains(t, err.Error(), "integer days become duration strings")
```

- [ ] **Step 2: Update show_test.go**

1. `TestShowConfigOutputIsValidTOML` — change `thinking = "medium"` to `effort = "medium"`.

2. `TestShowConfigRoundTrip` — change `thinking = "medium"` to `effort = "medium"`. Change field assertions from `.Thinking` to `.Effort`.

3. `TestShowConfigOmitsEmptyThinking` — rename to `TestShowConfigOmitsEmptyEffort`. Change assertion from `"thinking ="` to `"effort ="`.

4. `TestShowConfigShowsNonEmptyThinking` — rename to `TestShowConfigShowsNonEmptyEffort`. Change TOML from `thinking = "high"` to `effort = "high"`. Change assertion from `thinking = "high"` to `effort = "high"`.

- [ ] **Step 3: Add new deprecation-specific tests**

Add to `internal/config/config_test.go`:

```go
func TestThinkingRemovedReturnsError(t *testing.T) {
	t.Run("chat.llm", func(t *testing.T) {
		path := writeConfig(t, "[chat.llm]\nthinking = \"medium\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chat.llm.thinking")
		assert.Contains(t, err.Error(), "removed")
		assert.Contains(t, err.Error(), "chat.llm.effort")
	})
	t.Run("extraction.llm", func(t *testing.T) {
		path := writeConfig(t, "[extraction.llm]\nthinking = \"low\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extraction.llm.thinking")
		assert.Contains(t, err.Error(), "removed")
		assert.Contains(t, err.Error(), "extraction.llm.effort")
	})
}

func TestThinkingEnvVarRemovedReturnsError(t *testing.T) {
	t.Setenv("MICASA_CHAT_LLM_THINKING", "high")
	_, err := LoadFromPath(noConfig(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MICASA_CHAT_LLM_THINKING")
	assert.Contains(t, err.Error(), "removed")
	assert.Contains(t, err.Error(), "MICASA_CHAT_LLM_EFFORT")
}

func TestBothDeprecatedAndNewEnvVarSetErrorsOnDeprecated(t *testing.T) {
	t.Setenv("MICASA_CHAT_LLM_THINKING", "high")
	t.Setenv("MICASA_CHAT_LLM_EFFORT", "medium")
	_, err := LoadFromPath(noConfig(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MICASA_CHAT_LLM_THINKING")
}

func TestEmptyDeprecatedEnvVarDoesNotError(t *testing.T) {
	t.Setenv("MICASA_CHAT_LLM_THINKING", "")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Empty(t, cfg.Chat.LLM.Effort)
}

func TestDeprecatedEnvVarDeterministicOrder(t *testing.T) {
	t.Setenv("MICASA_CHAT_LLM_THINKING", "high")
	t.Setenv("MICASA_EXTRACTION_LLM_THINKING", "low")
	_, err := LoadFromPath(noConfig(t))
	require.Error(t, err)
	// chat.llm comes before extraction.llm in struct field order.
	assert.Contains(t, err.Error(), "MICASA_CHAT_LLM_THINKING")
}

func TestCacheTTLDaysEnvVarRemovedReturnsError(t *testing.T) {
	t.Setenv("MICASA_DOCUMENTS_CACHE_TTL_DAYS", "30")
	_, err := LoadFromPath(noConfig(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MICASA_DOCUMENTS_CACHE_TTL_DAYS")
	assert.Contains(t, err.Error(), "removed")
	assert.Contains(t, err.Error(), "MICASA_DOCUMENTS_CACHE_TTL")
	assert.Contains(t, err.Error(), "integer days become duration strings")
}
```

- [ ] **Step 4: Run all config tests**

Run: `go test ./internal/config/ -shuffle=on`

Expected: PASS

- [ ] **Step 5: Commit**

---

### Task 5: Rename `thinking` to `effort` in LLM client and app layer

**Files:**
- Modify: `internal/llm/client.go`
- Modify: `internal/llm/client_test.go`
- Modify: `internal/app/types.go`
- Modify: `internal/app/model.go`
- Modify: `cmd/micasa/main.go`

- [ ] **Step 1: Rename in `internal/llm/client.go`**

Change the struct field and method:

```go
// Before:
thinking     string // reasoning effort: none|low|medium|high|auto

// After:
effort       string // reasoning effort: none|low|medium|high|auto

// Before:
func (c *Client) SetThinking(level string) {
	c.thinking = level
}

// After:
func (c *Client) SetEffort(level string) {
	c.effort = level
}

// In completionParams, update:
// Before:
if c.thinking != "" {
	params.ReasoningEffort = anyllm.ReasoningEffort(c.thinking)
}

// After:
if c.effort != "" {
	params.ReasoningEffort = anyllm.ReasoningEffort(c.effort)
}
```

- [ ] **Step 2: Update `internal/llm/client_test.go`**

Rename `TestSetThinking` to `TestSetEffort`:

```go
func TestSetEffort(t *testing.T) {
	client := &Client{}
	client.SetEffort("medium")
	assert.Equal(t, "medium", client.effort)
}
```

Update `TestChatCompleteWithThinking` — rename to `TestChatCompleteWithEffort` and change `client.SetThinking("medium")` to `client.SetEffort("medium")`.

- [ ] **Step 3: Rename in `internal/app/types.go`**

Three changes:

1. `extractState.extractionThinking` -> `extractState.extractionEffort`
2. `chatConfig.Thinking` -> `chatConfig.Effort`
3. `extractionConfig.Thinking` -> `extractionConfig.Effort`
4. In `SetExtraction()`: parameter `thinking string` and `Thinking: thinking` -> `Effort: thinking` (keep param name `thinking` would be confusing — rename param to `effort`)
5. In `SetChat()`: same parameter rename

Update `SetExtraction`:
```go
func (o *Options) SetExtraction(
	provider, baseURL, model, apiKey string,
	timeout time.Duration,
	effort string,       // was: thinking
	...
) {
	o.ExtractionConfig = extractionConfig{
		...
		Effort: effort,  // was: Thinking: thinking
		...
	}
}
```

Update `SetChat`:
```go
func (o *Options) SetChat(
	enabled bool,
	provider, baseURL, model, apiKey, extraContext string,
	timeout time.Duration,
	effort string,       // was: thinking
) {
	o.ChatConfig = chatConfig{
		...
		Effort: effort,  // was: Thinking: thinking
	}
}
```

- [ ] **Step 4: Update `internal/app/model.go`**

Two changes:

1. In `NewModel`, change `chatCfg.Thinking` to `chatCfg.Effort` and `client.SetThinking` to `client.SetEffort`:

```go
// Before:
if chatCfg.Thinking != "" {
	client.SetThinking(chatCfg.Thinking)
}

// After:
if chatCfg.Effort != "" {
	client.SetEffort(chatCfg.Effort)
}
```

2. In extraction state init, change `extractionThinking` to `extractionEffort`:

```go
// Before:
extractionThinking: options.ExtractionConfig.Thinking,

// After:
extractionEffort: options.ExtractionConfig.Effort,
```

3. In the extraction client creation method (~line 896):

```go
// Before:
if m.ex.extractionThinking != "" {
	c.SetThinking(m.ex.extractionThinking)
}

// After:
if m.ex.extractionEffort != "" {
	c.SetEffort(m.ex.extractionEffort)
}
```

- [ ] **Step 5: Update `cmd/micasa/main.go`**

Change `.Thinking` to `.Effort` in both `SetChat` and `SetExtraction` calls:

```go
// Before:
chatLLM.Thinking,

// After:
chatLLM.Effort,

// Before:
exLLM.Thinking,

// After:
exLLM.Effort,
```

- [ ] **Step 6: Search for any remaining `thinking` references in Go code**

Run: `rg -i 'thinking' --type go -l` and verify the only remaining references are in `chat_render.go` (UI text "thinking" which is unrelated) and test files for the deprecation error path.

- [ ] **Step 7: Run full test suite**

Run: `go test -shuffle=on ./...`

Expected: PASS (or app-level tests may need minor fixes — check the output).

- [ ] **Step 8: Commit**

---

### Task 6: Update app-level tests referencing `thinking`

**Files:**
- Modify: `internal/app/chat_coverage_test.go` (if needed)
- Modify: `internal/app/chat_test.go` (if needed)

The `chat_render.go` uses "thinking" as a UI label (spinner text), which is unrelated to the config field. But tests may reference the config field.

- [ ] **Step 1: Search for config-related thinking references in app tests**

Run: `rg 'Thinking|SetThinking|extractionThinking' --type go internal/app/`

Fix any references found. The string "thinking" in `chat_render.go:80` is a UI label and should NOT be changed.

- [ ] **Step 2: Run app tests**

Run: `go test -shuffle=on ./internal/app/`

Expected: PASS

- [ ] **Step 3: Commit if any changes were needed**

---

### Task 7: Create the code generator and Hugo shortcode

**Files:**
- Create: `internal/config/cmd/gendeprecations/main.go`
- Modify: `internal/config/deprecated.go` (add `go:generate` directive)
- Create: `docs/data/deprecations.json`
- Create: `docs/layouts/shortcodes/replaces.html`

- [ ] **Step 1: Create the generator**

Create `internal/config/cmd/gendeprecations/main.go`:

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// gendeprecations produces docs/data/deprecations.json from Config struct tags.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/micasa-dev/micasa/internal/config"
)

type entry struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Hint    string `json:"hint,omitempty"`
}

func main() {
	output := flag.String("output", "", "output file path")
	flag.Parse()

	if *output == "" {
		fmt.Fprintln(os.Stderr, "usage: gendeprecations -output <path>")
		os.Exit(1)
	}

	deps := config.Deprecations()
	entries := make([]entry, len(deps))
	for i, d := range deps {
		entries[i] = entry{
			OldPath: d.OldPath,
			NewPath: d.NewPath,
			Hint:    d.HintText(),
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*output, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Add `HintText()` method to Deprecation**

In `internal/config/deprecated.go`, add:

```go
// HintText returns the human-readable transform hint, or empty string
// if no transform is registered.
func (d Deprecation) HintText() string {
	if d.Transform == "" {
		return ""
	}
	return transformHints[d.Transform]
}
```

Also capitalize `TransformHints` — wait, no. The generator imports `config` and calls `Deprecations()` which is exported. `HintText()` is on the exported `Deprecation` struct. `transformHints` stays unexported — `HintText()` provides the public API.

- [ ] **Step 3: Add the `go:generate` directive**

At the top of `internal/config/deprecated.go`, after the package doc/imports, add:

```go
//go:generate go run ./cmd/gendeprecations -output ../../docs/data/deprecations.json
```

- [ ] **Step 4: Run the generator**

Run: `go generate ./internal/config/`

Verify the output at `docs/data/deprecations.json` matches the expected JSON from the spec.

- [ ] **Step 5: Create the Hugo shortcode**

Create `docs/layouts/shortcodes/replaces.html`:

```html
{{- $path := .Get 0 -}}
{{- $deps := .Site.Data.deprecations -}}
{{- range $deps -}}
  {{- if eq (index . "new_path") $path -}}
    {{- $oldPath := index . "old_path" -}}
    {{- $parts := split $oldPath "." -}}
    {{- $old := index $parts (sub (len $parts) 1) -}}
    {{- $hint := index . "hint" -}}
    <span class="replaces-hint">Replaces <code>{{ $old }}</code>.{{ with $hint }} {{ . }}.{{ end }}</span>
  {{- end -}}
{{- end -}}
```

- [ ] **Step 6: Add CSS for `.replaces-hint`**

In `docs/static/css/docs.css`, add styling adjacent to the existing `.env-hint` rule:

```css
.docs-main .replaces-hint {
  display: block;
  font-size: 0.75em;
  color: var(--hint-color, #888);
  margin-top: 0.15em;
}
```

- [ ] **Step 7: Commit**

---

### Task 8: Update Hugo docs

**Files:**
- Modify: `docs/content/docs/reference/configuration.md`

- [ ] **Step 1: Replace `thinking` with `effort` in both LLM config tables**

In the `[chat.llm]` table:

```markdown
<!-- Before: -->
| `thinking` {{< env "MICASA_CHAT_LLM_THINKING" >}} | string | (unset) | Model reasoning effort level. Supported: `none`, `low`, `medium`, `high`, `auto`. Empty = server default. |

<!-- After: -->
| `effort` {{< env "MICASA_CHAT_LLM_EFFORT" >}} {{< replaces "chat.llm.effort" >}} | string | (unset) | Model reasoning effort level. Supported: `none`, `low`, `medium`, `high`, `auto`. Empty = server default. |
```

Same for the `[extraction.llm]` table:

```markdown
<!-- Before: -->
| `thinking` {{< env "MICASA_EXTRACTION_LLM_THINKING" >}} | string | (unset) | Reasoning effort level for extraction. |

<!-- After: -->
| `effort` {{< env "MICASA_EXTRACTION_LLM_EFFORT" >}} {{< replaces "extraction.llm.effort" >}} | string | (unset) | Reasoning effort level for extraction. |
```

- [ ] **Step 2: Update the example config block**

In the example TOML block in the docs:

```toml
# Before:
# thinking = "medium"
# After:
# effort = "medium"

# Before:
# thinking = "low"
# After:
# effort = "low"
```

- [ ] **Step 3: Add `{{< replaces >}}` to the `cache_ttl` row in `[documents]` table**

The `cache_ttl` row currently has a hard-coded deprecation note in its description ("Replaces the deprecated `MICASA_DOCUMENTS_CACHE_TTL_DAYS` env var."). Replace that text with the shortcode:

```markdown
<!-- Before: -->
| `cache_ttl` {{< env "MICASA_DOCUMENTS_CACHE_TTL" >}} | string or integer | `"30d"` | Cache lifetime for extracted documents. Accepts `"30d"`, `"720h"`, or bare integers (seconds). Set to `"0s"` to disable eviction. Replaces the deprecated `MICASA_DOCUMENTS_CACHE_TTL_DAYS` env var. |

<!-- After: -->
| `cache_ttl` {{< env "MICASA_DOCUMENTS_CACHE_TTL" >}} {{< replaces "documents.cache_ttl" >}} | string or integer | `"30d"` | Cache lifetime for extracted documents. Accepts `"30d"`, `"720h"`, or bare integers (seconds). Set to `"0s"` to disable eviction. |
```

- [ ] **Step 4: Commit**

---

### Task 9: Update codebase docs

**Files:**
- Modify: `.claude/codebase/types.md`

- [ ] **Step 1: Update Config types section**

Change `Thinking` to `Effort` in the Config type descriptions:

```markdown
<!-- Before: -->
- ChatLLM (Provider, BaseURL, Model, APIKey, Timeout, Thinking, ExtraContext)

<!-- After: -->
- ChatLLM (Provider, BaseURL, Model, APIKey, Timeout, Effort, ExtraContext)
```

Same for `ExtractionLLM`.

- [ ] **Step 2: Commit**

---

### Task 10: Final verification

- [ ] **Step 1: Run full test suite**

Run: `go test -shuffle=on ./...`

Expected: PASS with zero failures.

- [ ] **Step 2: Run linter**

Run: `golangci-lint run`

Expected: No warnings.

- [ ] **Step 3: Verify generated file is fresh**

Run: `go generate ./internal/config/`

Then: `git diff docs/data/deprecations.json`

Expected: No diff (file already up to date).

- [ ] **Step 4: Add generated-file freshness check to pre-commit**

The existing pre-commit/CI checks verify `internal/data` and `internal/app` generated files by running `go generate` then `git diff --exit-code` on the outputs. Extend this check to:
1. Run `go generate ./internal/config/`
2. Include `docs/data/deprecations.json` in the `git diff --exit-code` assertion

Both the generate step AND the diff target must be added — running the generator alone won't catch drift unless the diff covers the output file. The exact file to modify depends on how the existing checks are wired (likely `flake.nix` or `.pre-commit-config.yaml`).

- [ ] **Step 5: Search for stale references**

Run: `rg 'SetThinking|\.Thinking\b|extractionThinking|MICASA_CHAT_LLM_THINKING|MICASA_EXTRACTION_LLM_THINKING' --type go`

Expected: Only hits in deprecation tests (which test the error path) and `chat_render.go` (UI text, unrelated).

- [ ] **Step 6: Verify Hugo site builds**

Run: `cd docs && hugo --minify` (or Nix equivalent)

Expected: No errors. The `{{< replaces >}}` shortcodes resolve correctly.

- [ ] **Step 7: Commit any final fixes**

---

## Self-Review

**Spec coverage:**
- Struct tags as single source of truth: Task 1 + Task 2 ✓
- `Deprecations()` shared function: Task 1 ✓
- Runtime `removedKeys` derived from tags: Task 1 init, Task 3 wiring ✓
- Runtime `deprecatedEnvVars` derived from tags: Task 1 init ✓
- `checkDeprecatedEnvVars()` with `os.Getenv`: Task 1 ✓
- Deterministic scan order: Task 1 (uses `deprecatedEnvVarOrder` slice) ✓
- `cache_ttl_days` modeled with transform: Task 2 Step 3 ✓
- Error messages match spec: Task 4 Step 3 tests ✓
- Code generator + Hugo data: Task 7 ✓
- Hugo shortcode with hint: Task 7 Step 5 ✓
- Docs updated: Task 8 ✓
- All thinking -> effort renames: Tasks 2, 4, 5, 6, 8, 9 ✓
- Validation (duplicates, unknown transforms, env var collisions): Task 1 init ✓

**Placeholder scan:** No TBDs, TODOs, or "implement later". All steps have code.

**Type consistency:** `Deprecation`, `HintText()`, `Effort`, `SetEffort` used consistently across all tasks.
