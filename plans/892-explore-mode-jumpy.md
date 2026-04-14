<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Issue #892: Explore mode toggle causes jumpy x key and overlay height change

## Root Cause Analysis

Two visual glitches occur when toggling explore mode via `x` in the
extraction preview overlay:

### 1. Hint bar content changes cause x key to jump

The hint bar is completely rebuilt each render based on `ex.exploring`.
The two states show different hints in different order:

**Pipeline mode** (non-explore, done with ops and LLM):
```
j/k navigate . enter expand . x explore . t layout on . a accept . esc discard
```

**Explore mode** (single tab):
```
j/k rows . h/l cols . a accept . x back . esc discard
```

**Explore mode** (multiple tabs):
```
j/k rows . h/l cols . b/f tabs . a accept . x back . esc discard
```

The `x` key shifts from position 3 ("explore") to position 4 or 5
("back"), depending on tab count. The set of hints differs
(enter/expand and t/layout disappear; h/l cols appears). This makes the
key visually jump when the user presses it.

### 2. Overlay height changes on toggle

The overlay height depends on the preview section height, which depends
on which tab is rendered:

```go
previewSection = m.renderOperationPreviewSection(innerW, ex.exploring)
previewLines = strings.Count(previewSection, "\n") + 2
maxH = max(m.effectiveHeight()*2/3 - 6 - previewLines, 4)
vpH = min(contentLines, maxH)
```

In pipeline mode (`interactive=false`), `tabIdx=0` always. In explore
mode (`interactive=true`), `tabIdx=ex.previewTab`. If the active tab
has a different row count than tab 0, the preview section height
differs, changing `previewLines`, `maxH`, and `vpH`, which changes the
total overlay height and causes visual jumping.

Additionally, even with the same tab, the hint bar itself may have a
different rendered width (different number of hints), which could cause
wrapping differences and contribute to height changes.

### Code locations

- `internal/app/extraction_render.go:192-232` -- hint bar construction
- `internal/app/extraction_render.go:131-141` -- viewport height
  calculation depending on `previewLines`
- `internal/app/extraction_render.go:251-306` -- preview section render
  with `interactive` parameter controlling tab selection
- `internal/app/extraction.go:1149-1152` -- `x` enters explore
- `internal/app/extraction.go:1298-1299` -- `x`/esc exits explore

## Fix

### Fix 1: Stabilize the hint bar

Render the same hint items in both modes, only changing the `x` label
and the visual emphasis. Both modes need: navigation hint, action hints
(`a` accept, `x` toggle, `esc` exit). Mode-specific hints
(enter/expand, h/l cols, b/f tabs, t layout) can be placed BEFORE the
stable tail.

Target layout -- the trailing three hints always in the same position:

**Pipeline mode:**
```
j/k navigate . enter expand . [t layout] . a accept . x explore . esc discard
```

**Explore mode:**
```
j/k rows . h/l cols . [b/f tabs] . a accept . x back . esc discard
```

This keeps `a`, `x`, and `esc` at the end in both modes, so the `x`
key stays in a consistent position. The leading hints can vary because
the user's eye focuses on the key they just pressed (`x`).

### Fix 2: Stabilize the overlay height

Two changes:

**a) Stable preview line reservation.** Always reserve vertical space
for the tallest preview tab, not just the currently displayed one.
The preview section height is: tab bar (1) + underline (1) + header
(1) + divider (1) + data rows. Only the row count varies across tabs.
Compute the max row count across all groups and use it for
`previewLines`, independent of which tab is rendered:

```go
maxRows := 0
for _, g := range ex.previewGroups {
    if len(g.cells) > maxRows {
        maxRows = len(g.cells)
    }
}
// The preview section string has N+3 newlines for N data rows:
//   tabBar \n underline \n header \n divider \n row0 \n ... \n rowN-1
// The caller adds +2 for the blank line and separator above the section.
previewLines = maxRows + 3 + 2
```

Still render the preview section normally for display, but use this
stable `previewLines` for the height reservation instead of counting
newlines in the rendered string. This ensures `maxH` and `vpH` stay
constant regardless of which tab is visible.

**b) Consistent tab across modes.** Show `ex.previewTab` in pipeline
mode too (dimmed), instead of always resetting to tab 0. When the
user exits explore mode, they see the same tab they were exploring,
just dimmed -- maintaining visual continuity. This also avoids the
height change that would occur if tab 0 has a different row count
than the explored tab (though the stable reservation in (a) already
prevents layout shifts, this avoids content jumping).

## Risks

- Changing the stable preview height to max-across-tabs could waste
  vertical space if one tab has many more rows than others. Acceptable
  tradeoff for visual stability.
- Reordering hints changes muscle memory slightly, but the `x` key
  position is the one that matters most.
