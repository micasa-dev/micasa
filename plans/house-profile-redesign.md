<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# House Profile Overlay Redesign

**Issue:** [#842](https://github.com/micasa-dev/micasa/issues/842)
**Date:** 2026-04-09

## Problem

The expanded house profile renders as three label/value sections
(Structure, Utilities, Financial) in a horizontal middot-separated run
inside the header box. It's hard to scan, has no visual hierarchy, no
edit affordances, and the layout is identical to every other header strip
in the app.

## Design Decisions

### Collapsed Header

The collapsed view replaces the `House` pill with the house **nickname**
as the pill label. Vitals follow in a dot-separated run with bright
values and dim labels.

```
┌─────────────────────────────────────────────────────────────────────┐
│ [The Craftsman] ▸  Portland, OR · 3bd/2ba · 2.4ksf · 1928          │
└─────────────────────────────────────────────────────────────────────┘
```

- Nickname in accent pill styling (same style as old `House` pill).
- Location is dim (`HeaderHint`), numeric values are bright
  (`HeaderValue`), unit suffixes are dim.
- Square footage rounds to `k` suffix for values >= 1,000 (e.g. 2,400
  → `2.4k`, 5,000 → `5k`, 850 → `850`). Rule: round to nearest 100,
  divide by 1,000, drop trailing `.0`.
- When fields are empty, append `· ○ N` in warning color (e.g. `○ 4`).
  Hidden when all fields are populated.
- Toggle: `Tab` key or click on nickname pill zone opens the overlay.
- No-house state unchanged: `[House] setup  H` with hint text below.

### Expanded Overlay

The expanded view moves from the header box to a **centered overlay**
with dimmed background, matching the dashboard/chat/help pattern.

```
┌─────────────────────────────────────────────────────────────────────┐
│ [The Craftsman] ▾  742 Oak Ave, Portland OR 97201 🔗        18/22   │
│                                                                     │
│ Structure              Utilities              Financial             │
│ ──────────────────── ──────────────────── ────────────────────      │
│ ▸ Year Built   1928    Heating  Gas Forced   Insurance  State Farm  │
│   Living Area  2.4ksf  Cooling  Central AC   Policy     HO-12345   │
│   Lot          0.18ac  Water    Municipal    Renewal    2026-08-15  │
│   Bedrooms     3       Sewer    Municipal    Prop Tax   $4,200/yr   │
│   Bathrooms    2       Parking  Garage       HOA        ○ —         │
│   Foundation   Concrete                                             │
│   Roof         Comp Shingle                                         │
│   Exterior     Wood Siding                                          │
│   Wiring       Copper                                               │
│   Basement     ○ not set                                            │
│                                                                     │
│ ↑↓ navigate  ←→ section  enter edit  esc close                     │
└─────────────────────────────────────────────────────────────────────┘
```

- **Identity line**: nickname pill (with ▾ indicator) + full address as
  OSC8 Google Maps link + completion fraction (`N/M`) right-aligned.
- **Three-column grid**: Structure, Utilities, Financial. Each column
  has a section header in accent color with a horizontal rule below.
- **Field rendering**: dim label left-aligned, bright value
  right-aligned within label/value pair. Empty fields show `○ —` or
  `○ not set` in warning color.
- **Overlay sizing**: width fits content up to a max (e.g. 80 cols
  inner). Height fits content. Centered via `compositeOverlay` +
  `dimBackground` pattern.
- **Narrow width (< 80 cols)**: columns collapse to single-column
  stacked sections (Structure, then Utilities, then Financial).
- **Close**: `Esc` or `Tab` toggle. Click outside overlay zone also
  closes.

### Keyboard Navigation

Column-major navigation within the overlay:

- **↑/↓**: move cursor within current section (column). Wraps at
  section boundaries.
- **←/→**: jump between columns. Row position is remembered; if the
  target column is shorter, cursor clamps to its last field.
- **Enter**: open inline edit for the focused field.
- **Esc**: if editing, cancel edit. If browsing, close overlay.

### Mouse Interaction

- Every field is zone-marked with a unique ID (e.g.
  `house-field-year-built`).
- **Single click**: move cursor to that field.
- **Double-click**: open inline edit for that field.
- Section headers are not interactive.
- Address link is a separate zone (OSC8 hyperlink).

### Inline Edit

When the user presses Enter on a field, a single-field `huh.Form`
renders in-place, replacing the value display with an input/select
widget.

- **Text fields**: text input with current value pre-filled.
- **Select fields** (foundation, heating, etc.): select dropdown.
- **Number fields**: text input with numeric validation.
- **Date fields**: text input with date validation (or calendar picker
  if already wired).
- **Money fields**: text input with currency parsing.
- **Confirm**: Enter submits, writes to DB, refreshes overlay.
- **Cancel**: Esc reverts, returns to browse mode.

### Shared Field Definitions

A `houseFieldDef` slice is the single source of truth for field
metadata:

```go
type houseFieldDef struct {
    key       string          // e.g. "year_built"
    label     string          // e.g. "Year Built"
    section   houseSection    // structure, utilities, financial
    buildField func(value string) huh.Field
    get       func(HouseProfile) string
    set       func(*HouseProfile, string) error
}
```

- **Initial full form** (no house exists): iterates all defs, groups by
  section, builds full `huh.Form`.
- **Inline edit** (overlay): builds single-field `huh.Form` from
  focused field's def.
- Validators, options, and data mapping are defined once in the def.

### Pixel Art

Removed from the house profile. The retro house art and its style
methods (`HouseRoof`, `HouseWall`, `HouseWindow`, `HouseDoor`) can be
cleaned up. A smoking chimney animation is tracked separately in
[#917](https://github.com/micasa-dev/micasa/issues/917) for the
startup screen.

### Overlay Priority

The house profile overlay slots into the overlay stack in `buildView`.
It should render below dashboard (dashboard takes priority if both are
open) but above other overlays. Suggested position:

```go
overlays := []struct{ active bool; render func() string }{
    {m.dashboardVisible(), m.buildDashboardOverlay},
    {m.houseOverlayVisible(), m.buildHouseOverlay},      // new
    {m.calendar != nil, m.buildCalendarOverlay},
    // ... rest unchanged
}
```

Opening the house overlay while dashboard is visible should close the
dashboard first (or be blocked — match existing mutual-exclusion
behavior).

## Acceptance Criteria

Per issue #842:

- Side-by-side before/after demo recording (`/record-demo`)
- Tests cover keyboard navigation and click-to-edit on at least one
  field per section
- Narrow-width rendering verified down to 80 cols
- No regressions in collapsed header rendering
