<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Bubbletea/Lipgloss v2 Upgrade

Issue: https://github.com/micasa-dev/micasa/issues/622

## Current State

| Package | Current Version | v2 Status | v2 Version |
|---------|----------------|-----------|------------|
| bubbletea | v1.3.10 | **Stable** | v2.0.1 (Mar 2 2026) |
| lipgloss | v1.1.1-pre | **Stable** | v2.0.0 (Feb 24 2026) |
| bubbles | v1.0.0 | **Stable** | v2.0.0 (Feb 24 2026) |
| huh | v0.8.0 | **Unmerged PR** | pseudo `v2.0.0-20260226...` |
| glamour | v0.10.0 | **Untagged** | pseudo `v2.0.0-20260302...` |
| bubbletea-overlay | v0.6.5 | **No v2 planned** | use lipgloss v2 compositing |

Import paths change from `github.com/charmbracelet/<pkg>` to
`charm.land/<pkg>/v2`.

## Blockers

### huh v2 (form library) -- HIGH risk

Used in 7 files for all entity forms. PR #609 has been open since Mar 2025
with 40 commits but is **unmerged**. Pinning to a pseudo-version means:
- No semver stability guarantees
- API could change before merge
- Our forms are the most complex surface in the app

**Options:**
1. **Pin to huh pseudo-version** -- risky but lets us move forward. If the
   API shifts we eat the churn.
2. **Fork huh at the PR commit** -- more control, but maintenance burden.
3. **Wait for huh v2 stable** -- safest, blocks the entire upgrade.
4. **Replace huh** -- enormous effort (9 form types, custom theming,
   filepicker, validation).

**Recommendation:** Wait. The huh v2 PR is active (last commit Feb 26 2026)
and likely to land soon. We can prepare everything else and flip huh last, or
pin the pseudo-version if it stabilizes. Do not fork or replace.

### glamour v2 (markdown renderer) -- LOW risk

Used in 1 file (`view.go`) for markdown rendering. Pseudo-version exists
(Mar 2 2026). The API surface we use is tiny (`NewTermRenderer` +
`WithAutoStyle` + `WithWordWrap` + `Render`). Low risk to pin to pseudo.

### bubbletea-overlay -- NO risk

Maintainer says "use lipgloss v2 compositing." We use exactly one function:
`overlay.Composite(fg, bg, Center, Center, 0, 0)`. Lipgloss v2 provides
`NewLayer`/`NewCompositor` with X/Y/Z positioning. Straightforward
replacement.

## Migration Surface Area

### bubbletea changes (19 files)

| Change | Occurrences | Effort |
|--------|-------------|--------|
| `tea.KeyMsg` -> `tea.KeyPressMsg` | ~96 | Mechanical find/replace |
| `View() string` -> `View() tea.View` | 1 (main) + ~25 helpers | Main View wraps with `tea.NewView()`; helpers stay `string` |
| `tea.WithAltScreen()` program option | 1 | Move to `view.AltScreen = true` |
| `tea.WindowSizeMsg` field access | 2 | Verify field names unchanged |
| Space key `" "` -> `"space"` | Audit needed | Check key constants in `model.go` |

### lipgloss changes (17 files)

| Change | Occurrences | Effort |
|--------|-------------|--------|
| `AdaptiveColor{}` -> `LightDark()` or `compat.AdaptiveColor` | 13 | Moderate -- centralized in `styles.go` |
| `lipgloss.Color` type -> function | 0 bare uses | N/A |
| Renderer removal | 0 uses | N/A |
| `JoinVertical`/`JoinHorizontal`/`Place` | ~26 | Verify API unchanged |

### bubbles changes (11+ files)

| Change | Occurrences | Effort |
|--------|-------------|--------|
| `spinner.NewModel()` -> `spinner.New()` | 3 | Trivial |
| `viewport.New(w,h)` -> `viewport.New()` + setters | 3 | Small |
| Width/Height fields -> methods | Multiple | Moderate |
| `DefaultKeyMap` var -> function | 2 | Trivial |
| `DefaultStyles(isDark)` parameter | Audit needed | Moderate |
| `textinput` style restructuring | 3 files | Moderate |

### bubbletea-overlay -> lipgloss compositing (1 file)

Replace `overlay.Composite(fg, bg, Center, Center, 0, 0)` with lipgloss v2
`NewCompositor`/`NewLayer` API. Need to compute center X/Y ourselves or use
`Place` for centering.

## Phased Approach

The upgrade must be atomic (all charm deps move together since v2 packages
cross-import each other), but we can prepare incrementally.

### Phase 0: Pre-migration prep (can do now on v1)

- Audit all key string constants in `model.go` for the space key (`" "` ->
  `"space"` in v2). Add/update constants proactively.
- Audit all `spinner.NewModel()` call sites.
- Audit all `viewport.New()` call sites and Width/Height field access.
- Replace any `tea.Sequentially` usage (none found, but verify).
- Document every `AdaptiveColor` definition and its light/dark hex values.

### Phase 1: Core swap (one big commit)

1. Update `go.mod`: replace all `github.com/charmbracelet/*` with
   `charm.land/*/v2`. Pin huh + glamour to pseudo-versions (or wait).
2. Update all import paths across every `.go` file.
3. `tea.KeyMsg` -> `tea.KeyPressMsg` (mechanical).
4. `View() string` -> `View() tea.View` on `Model`. Helper view functions
   keep returning `string`; only the top-level `View()` wraps with
   `tea.NewView(result)` and sets `AltScreen = true`.
5. `tea.WithAltScreen()` removed from `main.go` program creation.
6. `AdaptiveColor` -> `lipgloss.LightDark()` pattern or `compat` package.
   Detect dark background once at startup, thread `isDark` through styles.
7. `spinner.NewModel()` -> `spinner.New()`.
8. `viewport.New(w,h)` -> `viewport.New()` + `SetWidth`/`SetHeight`.
9. Width/Height field -> method migration on all bubbles models.
10. Replace `bubbletea-overlay` with lipgloss v2 compositing.
11. Remove `bubbletea-overlay` dependency.

### Phase 2: Verify

- `go build ./...`
- `go test -shuffle=on ./...`
- `nix run '.#pre-commit'`
- Manual smoke test of every overlay (dashboard, calendar, notes, column
  finder, extraction, chat, help)
- Manual smoke test of every form (project, quote, maintenance, appliance,
  vendor, document, incident, service log, room)
- Verify light and dark terminal themes

### Phase 3: Polish

- Switch from `compat.AdaptiveColor` to native `LightDark()` if using compat.
- Explore new v2 features: progressive keyboard enhancements, hyperlinks,
  underline styles, native clipboard.
- Consider `tea.View` fields for cursor positioning, window title, etc.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| huh pseudo-version breaks | Medium | High | Wait for stable, or isolate form code behind interface |
| glamour pseudo-version breaks | Low | Low | Tiny API surface; easy to vendor or replace |
| bubbletea-overlay replacement misses edge cases | Low | Medium | Only 1 call site; test all overlays manually |
| Key constant `" "` -> `"space"` missed | Medium | Medium | Grep for space key usage; add test coverage |
| `isDark` threading breaks adaptive colors | Low | Medium | Test both light and dark terminals |
| Bubbles Width/Height method migration missed | Medium | Low | Compiler will catch (type mismatch) |

## Decision

**Recommend waiting** until huh v2 lands a tagged release before starting the
upgrade. The huh PR is active and the rest of the ecosystem is stable. When
huh v2 ships, execute Phase 1 as a single atomic change.

If huh v2 doesn't ship within a reasonable window, the fallback is to pin
its pseudo-version and accept the churn risk.

In the meantime, Phase 0 prep work can happen on v1 without any dependency
changes.
