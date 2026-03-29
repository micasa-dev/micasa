<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Fix: drilldown count text not visible when overlay dims background

Issue: micasa-dev/micasa#848

## Problem

Drilldown count pill text (e.g. "2" in Quotes/Docs columns) becomes
unreadable when `dimBackground()` applies ANSI faint behind an overlay.
Same root cause as #833/#834: `dimBackground` converts bold (SGR 1) to
faint (SGR 2), leaving dark foreground text invisible on the surviving
bright background.

## Initial approach (rejected)

Thread an `overlayActive` bool through the render chain
(`renderRows` → `renderRow` → `renderCell` → `renderPillCell`) and
conditionally switch from `Drilldown()` to `AccentOutline()`. This
worked but introduced two problems:

- Pill padding caused a geometry shift when toggling overlays (3-char
  padded pill vs 1-char accent text)
- Threading a bool through 4 function signatures for a single style
  decision was heavy-handed

## Final approach

Replace filled pill badges with bold accent-foreground text
(`AccentBold()`) unconditionally. Non-zero drilldown counts now render
through the normal `renderCell` right-alignment path, identical in
geometry to zero-count cells. This:

- Fixes overlay dimming: bold accent fg converts to faint under
  `dimBackground`, staying legible
- Eliminates geometry shifts: same width in both normal and overlay modes
- Simplifies code: removes `renderPillCell` entirely, removes dead
  `cellDrilldown/cellOps` arm from `cellStyle`

## Files modified

- `internal/app/table.go` — replace pill rendering with `AccentBold()`,
  remove `renderPillCell`, remove dead `cellStyle` branch
- `internal/app/dashboard_test.go` — test verifying accent fg (not bg)
  for drilldown counts in both normal and overlay modes
- `plans/issue-848-drilldown-pill-overlay-dim.md` — this plan
