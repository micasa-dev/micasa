<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Clickable UI Elements

GitHub issue: #450

## Problem

The TUI is entirely keyboard-driven. While the Vim-like keybindings are
powerful, mouse users expect to be able to click on visible UI elements. File
paths and URLs in the interface are plain text with no terminal hyperlink
support.

## Scope

Three categories of clickability, ordered by implementation complexity:

1. **OSC 8 hyperlinks** -- terminal-handled, no mouse tracking needed
2. **Zone-based mouse clicks** -- tabs, table rows, status bar hints, buttons
3. **House profile interactions** -- clickable fields to jump to edit

## Design

### 1. Enable Mouse Support

Add `tea.WithMouseCellMotion()` to the `tea.NewProgram()` call in
`cmd/micasa/main.go`. This enables:

- Click events (press/release)
- Scroll wheel events (already partially handled by viewport components)
- Drag motion events (only while a button is held)

It does *not* enable hover tracking, which avoids noisy motion events.

### 2. Zone-Based Click Detection (bubblezone)

Use `github.com/lrstanley/bubblezone` for mapping clicks to UI regions.

**How it works:**

- `zone.Mark(id, content)` wraps rendered text with invisible zero-width
  markers
- `zone.Scan(output)` in the final `View()` strips markers and records their
  terminal positions
- `zone.Get(id).InBounds(mouseMsg)` checks if a click landed in a zone

**Manager:** Create a single `zone.Manager` on the `Model` struct. Pass it
through rendering. Call `zone.Scan()` at the end of `View()`.

### 3. Clickable Surfaces

#### Tabs (tab bar)

- Zone ID: `tab-<index>` (e.g. `tab-0`, `tab-1`, ...)
- Left click: switch to that tab (same as pressing `f`/`b` to navigate)
- Only active in `modeNormal` (tabs are locked in edit/form mode)

#### Table Rows

- Zone ID: `row-<index>` (visible row index, 0-based from top of table)
- Left click: move cursor to that row
- Double-click (or click on already-selected row): enter/drilldown (same as
  `enter`)
- Scroll wheel: scroll table up/down (same as `j`/`k`)

#### Table Column Headers

- Zone ID: `col-<index>`
- Left click: move column cursor to that column
- This enables quick column navigation for wide tables

#### Status Bar Hints

- Zone ID: `hint-<id>` (e.g. `hint-edit`, `hint-help`, `hint-add`)
- Left click: execute that action (same as pressing the displayed key)
- Provides discoverability for keyboard-only features

#### House Profile Header

- Zone ID: `house-header`
- Left click: toggle expand/collapse (same as `H`)
- When expanded, individual fields could be clickable to jump to the house
  edit form, but defer this to a follow-up

#### Dashboard Sections

- Zone ID: `dash-section-<index>`
- Left click: move cursor to that section
- Left click on selected section: toggle expand (same as `enter`)

#### Breadcrumb Segments

- Zone ID: `breadcrumb-back`
- Left click on "(esc back)": go back (same as `esc`)

#### Overlay Dismiss

- Clicks outside an active overlay dismiss it (same as `esc`)
- The overlay compositing already knows the overlay bounds

### 4. OSC 8 Hyperlinks

OSC 8 is the terminal standard for clickable hyperlinks:

```
\033]8;;URL\033\\DISPLAY_TEXT\033]8;;\033\\
```

Surfaces that emit hyperlinks:

- **Document file paths**: link to the file on disk (using `file://` URI)
- **House address** (expanded view): link to a maps search
- **Config file path**: link to the config file
- **Database path**: link to the containing directory
- **Help overlay URLs**: any http/https URLs in help text

Add an `osc8Link(url, text string) string` helper to `view.go` that wraps
text in the OSC 8 escape sequence. Only emit when stdout is a terminal
(already established by Bubble Tea).

### 5. Mouse Dispatch Architecture

Add a `tea.MouseMsg` case to `Update()`:

```go
case tea.MouseMsg:
    if typed.Action == tea.MouseActionPress && typed.Button == tea.MouseButtonLeft {
        return m.handleLeftClick(typed)
    }
    if typed.Button == tea.MouseButtonWheelUp || typed.Button == tea.MouseButtonWheelDown {
        return m.handleScroll(typed)
    }
```

`handleLeftClick` checks zones in priority order:

1. Active overlay zones (if overlay is open)
2. Tab zones
3. Table row/column zones
4. Status bar zones
5. House header zone

`handleScroll` delegates to:

- Active overlay scroll (if overlay is open)
- Table scroll (in main view)

### 6. Key Constants for Mouse

No new key constants needed -- mouse events are a separate message type
(`tea.MouseMsg`), not `tea.KeyMsg`. Zone IDs are local to the rendering
and click-handling code, not key dispatch.

### 7. Test Strategy

- Add a `sendMouse(m *Model, x, y int, button tea.MouseButton)` helper
  alongside the existing `sendKey` helper
- Test tab clicking: click on each tab, verify `m.active` changes
- Test row clicking: click on a row, verify cursor moves
- Test status bar hint clicking: click on "help" hint, verify help opens
- Test overlay dismiss: click outside overlay bounds
- Test scroll: send wheel events, verify table cursor moves

## Implementation Order

1. Add `bubblezone` dependency, wire `zone.Manager` into Model
2. Enable mouse in `tea.NewProgram()`
3. Add `tea.MouseMsg` handling in `Update()` (dispatch skeleton)
4. Clickable tabs (simplest bounded region)
5. Clickable table rows + scroll wheel
6. Clickable status bar hints
7. OSC 8 hyperlinks (independent of mouse -- can be done in parallel)
8. House header click, breadcrumb click, dashboard clicks
9. Overlay dismiss on outside click
10. Tests for all surfaces

## Risks

- **bubblezone + lipgloss width**: Zero-width markers can confuse
  `lipgloss.Width()` if called *after* marking. Always call `zone.Mark()`
  as the last step in rendering a component, after width calculations.
- **bubblezone + overlay compositing**: The overlay system re-renders base +
  foreground. Need to ensure `zone.Scan()` runs on the final composited
  output, not intermediate renders.
- **Terminal compatibility**: Mouse support varies across terminals. The
  `CellMotion` mode is widely supported (xterm, iTerm2, Windows Terminal,
  Ghostty, kitty, Alacritty). OSC 8 has good but not universal support.
- **Huh forms**: The `huh` form library used for full-screen forms may need
  its own mouse handling. Defer form-field clicking to the huh library's
  built-in support if available.

## Non-Goals (This PR)

- Drag-and-drop reordering of rows or tabs
- Right-click context menus
- Hover highlighting / tooltips
- Mouse support inside `huh` forms (separate concern)
- Clickable cells for inline editing (complex; needs careful UX thought)
