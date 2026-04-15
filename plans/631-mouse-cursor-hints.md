<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Mouse Cursor Shape Hints on Hover

GitHub issue: #631
Predecessor: #450 (clickable UI)

## Problem

The TUI has extensive mouse click support (tabs, rows, columns, hints,
breadcrumbs, overlays, dashboard items) via bubblezone, but users get no
visual feedback about which elements are clickable. The mouse cursor stays
as the terminal's default (usually a text/I-beam cursor) everywhere, making
interactive elements indistinguishable from static text.

## Goal

Change the mouse pointer cursor to a "pointer" (hand) shape when hovering
over clickable zones, and reset it to "default" elsewhere. This gives users
an immediate visual signal that an element is interactive.

## Mechanism: OSC 22

Terminal emulators support **OSC 22** escape sequences to change the mouse
pointer shape. The format is:

```
\x1b]22;<shape>\x1b\\
```

Where `<shape>` is a CSS cursor name. The two shapes we need:

- `pointer` -- pointing hand, indicates a clickable element
- `` (empty) -- resets to terminal default

OSC 22 is independent of mouse reporting mode. It works regardless of
whether `MouseModeCellMotion` or `MouseModeAllMotion` is active.

### Terminal compatibility

| Terminal   | OSC 22 | Notes |
|-----------|--------|-------|
| xterm     | yes    | Origin of the escape sequence |
| foot      | yes    | Early adopter |
| kitty     | yes    | Extended spec (push/pop/query) |
| Ghostty   | yes    | Limited shapes on macOS (`pointer` works) |
| Alacritty | opt-in | Disabled by default (`terminal.osc22 = true`) |
| WezTerm   | partial| PR in progress |
| iTerm2    | no     | No OSC 22 support |
| tmux      | yes*   | Requires `allow-passthrough on`; app wraps OSC 22 in DCS passthrough when `$TMUX` is set. Nested tmux (tmux-inside-tmux) unsupported. |
| Windows Terminal | no | No OSC 22 support |

**Graceful degradation**: Terminals that don't support OSC 22 silently
ignore the escape sequence. No visible artifacts, no errors. The feature
is purely additive. Write errors from `io.WriteString` are intentionally
ignored because cursor shape is cosmetic — failure has no functional impact.

## Design

### Mouse tracking mode

Currently the app uses `tea.MouseModeCellMotion`, which generates motion
events only while a button is held (drag). To detect hover over zones
without clicking, we need `tea.MouseModeAllMotion`, which generates motion
events for every cell the mouse moves through.

**Performance**: `MouseModeAllMotion` generates significantly more events
than `MouseModeCellMotion`. Every cell transition triggers a
`tea.MouseMotionMsg`. Mitigation:

- The motion handler does zero allocations on the hot path -- just
  coordinate comparison and zone lookups.
- Zone bounds checks are simple integer comparisons (no string parsing).
- We only emit an OSC 22 escape when the cursor shape actually changes
  (track `lastPointerShape`), not on every motion event.
- The `View()` method already runs `zones.Scan()` on every frame; the
  motion handler reuses cached zone data.

### Architecture

```
MouseMotionMsg
  -> handleMouseMotion(msg)
    -> check all clickable zones for InBounds(msg)
    -> if any match: emit OSC 22 "pointer" (if not already pointer)
    -> if none match: emit OSC 22 "" (if not already default)
    -> store lastPointerShape to avoid redundant writes
```

Bubbletea's `tea.View` struct does not have a dedicated field for mouse
pointer shape (it has `CursorShape` but that controls the text cursor via
DECSCUSR, not the mouse pointer). The View content string is parsed by
ultraviolet's `StyledString` which only handles SGR and OSC 8 (hyperlinks);
injecting OSC 22 into the content would be treated as cell content and
corrupt layout.

Instead, write the OSC 22 sequence to a configurable `io.Writer` (defaults
to `os.Stdout`) from the `handleMouseMotion` method in Update. Terminal
emulators process escape sequences as they arrive, independent of
Bubbletea's rendering cycle. This is safe because Update runs synchronously
and the write is a single short sequence with no layout effect. The writer
is injected via a field on Model for testability.

#### tmux DCS passthrough

When running inside tmux, raw OSC 22 sequences are intercepted and never
reach the outer terminal. The app detects tmux at model creation via
`os.Getenv("TMUX") != ""` and wraps sequences in DCS passthrough:

```
Raw:     \x1b]22;pointer\x1b\\
Wrapped: \x1bPtmux;\x1b\x1b]22;pointer\x1b\x1b\\\x1b\\
```

tmux (with `allow-passthrough on`) strips the DCS wrapper, un-doubles the
ESC bytes, and forwards the inner OSC 22 to the real terminal. The
`buildOSC22(shape, tmux)` function handles this wrapping. The `inTmux`
flag is set once at startup and threaded through `setPointerShape` and
`resetPointerShape`.

Detection is env-based (`$TMUX`), not pty-based. This is sufficient because:
- `$TMUX` is set by tmux in every child process
- Stale `$TMUX` (tmux exited but env inherited) causes harmless DCS wrapping
  that non-tmux terminals ignore
- No other multiplexer sets `$TMUX`

### Zone hover detection

Reuse the existing zone infrastructure from `mouse.go`. The clickable zones
are already defined with `m.zones.Mark()` and looked up with
`m.zones.Get()`. The hover check is structurally identical to click
dispatch but returns a boolean instead of executing an action.

Clickable zone prefixes to check (from `mouse.go`):

- `tab-N` -- tab bar items
- `row-N` -- table rows (visible range only)
- `col-N` -- column headers
- `hint-ID` -- status bar action hints
- `dash-N` -- dashboard nav entries
- `house-header` -- house profile toggle
- `breadcrumb-back` -- drilldown back link
- `ext-tab-N`, `ext-row-N`, `ext-col-N` -- extraction preview
- `house-field-KEY` -- house overlay fields
- `search-N` -- doc search results
- `ops-node-N`, `ops-tab-N` -- ops tree overlay

Rather than iterate all possible zone IDs on every motion event, we use
`zones.Get(id).InBounds(msg)` which is O(1) per zone. The total number of
visible zones at any time is bounded by the viewport (typically <50), so
the full scan is fast.

A helper `isOverClickableZone(msg tea.MouseMotionMsg) bool` encapsulates
the check. It mirrors the dispatch structure of `handleLeftClick` and
`handleOverlayClick` but only returns whether any zone matched.

### State tracking

Add to `Model`:

```go
lastPointerShape string // "pointer" or "" (default)
```

### Cleanup on exit

The mouse pointer shape must reset to default when the app exits.
Bubbletea's renderer close resets mouse tracking mode but does NOT emit
OSC 22 reset. Terminals are not guaranteed to reset pointer shape when
mouse tracking is disabled.

Reset strategy:

- **Normal quit** (`ctrl+q`): The quit handler in `update()` writes
  `\x1b]22;\x1b\\` to the pointer writer before returning `tea.Quit`.
  Bubbletea calls `View()` one final time via `p.render(model)` on
  graceful shutdown, but the OSC 22 reset happens in Update.
- **SIGINT/kill/panic**: Bubbletea's signal handler converts SIGINT to
  `InterruptMsg` which exits the run loop without a final render.
  `shutdown(true)` is called, skipping flush. The pointer shape may be
  stuck. This is acceptable -- exiting alt screen mode typically resets
  the pointer in most terminals. Note: `ctrl+c` in this app is mapped to
  `Cancel` (cancels LLM operations, does not quit), so it does not
  trigger this path.

The reset is a single write of `\x1b]22;\x1b\\` to the pointer writer.
No sentinel values, no View-side logic.

### Integration with View()

```go
func (m *Model) View() tea.View {
    v := tea.NewView(m.zones.Scan(m.buildView()))
    v.AltScreen = true
    v.MouseMode = tea.MouseModeAllMotion  // Changed from CellMotion
    return v
}
```

Only the mouse mode changes in View. OSC 22 writes happen in Update via
`handleMouseMotion`, not in View.

### Test approach

- **Zone hover detection**: Send `tea.MouseMotionMsg` at known zone
  coordinates, verify `lastPointerShape` state changes.
- **Shape reset**: Verify that moving off a zone resets the shape.
- **Quit cleanup**: Verify that the quit handler resets pointer shape.
- **Test helper**: Add `sendMouseMotion(m *Model, x, y int)` alongside
  existing `sendClick` and `sendMouseWheel`.

For stdout-based OSC 22 writes in tests, inject a `pointerWriter io.Writer`
field on Model (defaulting to `os.Stdout` in production). Tests supply a
`bytes.Buffer` to capture and assert on the written sequences. This is
standard dependency injection, not test-only infrastructure -- both
production and test callers supply different writers.

No VHS/visual testing needed -- the feature is invisible in recordings
since VHS doesn't render mouse pointer shapes.

## Risks

- **Event volume**: `MouseModeAllMotion` generates many more events. The
  motion handler must be allocation-free on the hot path. Mitigated by
  short-circuit comparison (`if x == lastX && y == lastY return`).
- **Terminal compatibility**: Some terminals ignore OSC 22. The feature
  degrades silently. Alacritty requires opt-in config.
- **tmux passthrough**: tmux intercepts raw OSC 22 sequences. When `$TMUX`
  is set, the app wraps sequences in DCS passthrough
  (`ESC P tmux; <escaped> ST`) so tmux forwards them to the outer terminal.
  This requires `allow-passthrough on` in the user's tmux config (tmux 3.3a+).
  Without it, the feature silently degrades — no errors, no cursor changes.
  Nested tmux (two or more layers) is explicitly unsupported because each
  layer needs its own DCS wrapping; this is an uncommon setup.
  Other multiplexers (GNU screen, Zellij) are unsupported and untested.
- **Overlay compositing**: Zone bounds may shift during overlay transitions.
  The motion handler uses the same zone data as click handlers, so if zones
  are stale, the cursor shape may briefly be wrong. This is acceptable --
  it self-corrects on the next mouse move.
- **Form mode**: During form input (`modeForm`), mouse motion events still
  arrive but no zones are clickable. The hover check naturally returns false
  for all zones, so the cursor stays default. No special handling needed.

## Non-Goals

- Custom cursor shapes per zone type (e.g., resize cursor for column
  borders). Only `pointer` vs `default`.
- Configuration toggle to disable the feature. The issue mentions
  considering opt-in due to heavier event load, but the motion handler is
  cheap (no allocations, no re-renders, just integer comparisons and a
  conditional stdout write). OSC 22 is harmless on unsupported terminals.
  Adding config for this violates the "resist configuration" principle.
- Push/pop cursor shape stack (kitty extension). Simple set/reset is
  sufficient.
- Hover highlighting (visual change to the hovered element). Separate
  concern from cursor shape.
