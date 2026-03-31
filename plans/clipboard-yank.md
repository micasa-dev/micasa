<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Clipboard Yank (copy cell value)

## Problem

Users cannot copy cell data to the system clipboard from the TUI. They must
visually select text with the mouse, which is error-prone in styled terminal
output.

## Design

Press `y` in normal mode to copy the focused cell's raw value to the system
clipboard via BubbleTea v2's `tea.SetClipboard()` (OSC 52).

### Why OSC 52 / BubbleTea v2 built-in

- **No external process execution** -- no `os/exec`, no PATH hijack surface.
- **No CGo** -- pure Go, no C toolchain needed.
- **No new dependencies** -- already in `charm.land/bubbletea/v2`.
- **Works over SSH** -- the terminal emulator on the user's machine handles it.
- **Works on X11 and Wayland** -- terminal handles the windowing system call.

**Caveat: OSC 52 is best-effort.** The escape sequence is fire-and-forget --
there is no terminal acknowledgment, so we cannot detect failure. Some terminals
disable OSC 52 entirely; tmux requires `set-clipboard on`. This is inherent to
the protocol, not a library deficiency. The `"Copied."` status is optimistic,
matching the convention of every vim-like TUI (vim says "yanked" without
confirming clipboard). If the terminal ignores the sequence, nothing visibly
breaks -- the user just doesn't get clipboard content.

Alternatives considered and rejected:

| Library | Rejection reason |
|---|---|
| `atotto/clipboard` | Unmaintained (2021), shells out to xclip/xsel via os/exec |
| `golang-design/clipboard` | Requires CGo, no Wayland support |

### Keybinding

`y` in normal mode. Already defined as `keyY` in `model.go`. Currently only
used for `ConfirmYes` in confirmation dialogs (separate dispatch path in
`model_status.go`; no conflict with `handleNormalKeys`).

### What gets copied

The raw `cell.Value` string -- e.g. `1500.00`, `2025-03-17`, `Fix kitchen sink`.
Not the display-formatted value (`$1,500.00`, `Mar 17`).

### Edge cases

- **Null cell** (`cell.Null == true`): status `"Nothing to copy."`, no clipboard
  command.
- **Empty string** (`cell.Value == ""`): same as null.
- **`selectedCell` returns `false`** (no rows, cursor out of bounds): same as
  null.
- **Dashboard visible:** `handleDashboardKeys` runs before `handleNormalKeys`.
  `YankCell` must be added to the dashboard block list (`model_keys.go:54`)
  alongside Sort, ColHide, FilterPin, etc. Otherwise `y` falls through and
  copies a cell from the hidden underlying table -- confusing and wrong.

### User feedback

- Success: `setStatusInfo("Copied.")`
- Nothing to copy: `setStatusInfo("Nothing to copy.")`

### Mouse clickability

Keyboard-only. There is no natural mouse target for "copy this cell" beyond
the cell itself, which is already zone-marked for navigation. A context menu
or right-click copy would be a separate feature.

### Flow

1. User presses `y` in normal mode.
2. `handleNormalKeys()` matches `m.keys.YankCell`.
3. Get `tab := m.effectiveTab()`. If nil, return `nil, true` (no-op).
4. `m.selectedCell(tab.ColCursor)` retrieves the current cell.
5. If `ok == false`, or `cell.Null`, or `cell.Value == ""`:
   call `m.setStatusInfo("Nothing to copy.")`, return `nil, true`.
6. Call `m.setStatusInfo("Copied.")` (side effect on model), then
   return `tea.SetClipboard(cell.Value), true`.

### Testing

Primary tests use `sendKey(m, "y")` (user-interaction driver per CLAUDE.md)
and assert on status messages. Supplementary tests call
`m.Update(keyPress("y"))` directly to capture the returned `tea.Cmd`:
verify the clipboard payload matches the focused cell's raw value for the
happy path (call the cmd, `fmt.Sprint` the resulting `tea.Msg`, assert
against expected value), and verify `cmd == nil` for every no-op path to
prevent regressions where a clipboard command leaks through.

| Scenario | Driver | Assert status | Assert cmd |
|---|---|---|---|
| Cell with data | `sendKey` | `"Copied."` | -- |
| Null cell | `sendKey` | `"Nothing to copy."` | -- |
| Empty string cell | `sendKey` | `"Nothing to copy."` | -- |
| Empty table (no rows) | `sendKey` | `"Nothing to copy."` | -- |
| Dashboard visible | `sendKey` | no status change (blocked) | -- |
| Cmd carries correct value | `m.Update` | `"Copied."` | payload == `cell.Value` |
| Null cell no cmd | `m.Update` | `"Nothing to copy."` | `== nil` |
| Empty string no cmd | `m.Update` | `"Nothing to copy."` | `== nil` |
| No rows no cmd | `m.Update` | `"Nothing to copy."` | `== nil` |
| Dashboard no cmd | `m.Update` | no status change | `== nil` |

## Files to change

- `internal/app/keybindings.go` -- add `YankCell key.Binding` field, init in
  `newAppKeyMap()`. NOT added to `normalModeShortHelp()` (secondary action,
  would be truncated first).
- `internal/app/model_keys.go` -- add case in `handleNormalKeys()`. Add
  `m.keys.YankCell` to the dashboard block list in `handleDashboardKeys()`
  (line 54).
- `internal/app/view_help.go` -- add `fromBinding(m.keys.YankCell)` to the
  "Nav Mode" section in `helpSections()`.
- `internal/app/yank_test.go` -- user-interaction tests per the table above.
