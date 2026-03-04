+++
title = "Row Filtering"
weight = 3
description = "Pin cell values to filter rows interactively."
linkTitle = "Row Filtering"
+++

micasa lets you filter table rows by pinning cell values, then activating to hide non-matching rows.

<video src="/videos/using-filtering.webm" class="demo-video" autoplay loop muted playsinline></video>

## Quick start

1. Navigate to a cell whose value you want to filter by (e.g., "Plan" in the
   Status column)
2. Press `n` to pin it -- matching rows stay bright, others dim
3. Press `N` to activate -- non-matching rows disappear
4. Press `N` again to deactivate (rows return, dimming resumes)
5. Press `ctrl+n` to clear all pins and deactivate the filter at once

## Pin logic

- **OR within a column**: pinning "Plan" and "Active" in the Status column
  matches rows with *either* value
- **AND across columns**: pinning Status = "Plan" and Vendor = "Bob's Plumbing"
  matches rows where *both* conditions hold

Matching is case-insensitive and exact (the full cell value, not a substring).

## Visual states

| State | Matching rows | Non-matching rows | Pinned cells |
|-------|--------------|-------------------|--------------|
| No pins | Normal | Normal | Normal |
| Preview (pins, filter off) | Normal | Dimmed | Mauve foreground |
| Active (filter on) | Normal | Hidden | Mauve foreground |

Pinned cell values render in mauve to make it easy to see what you've selected.
Non-matching rows in preview mode are dimmed but still visible so you can verify
the filter before committing.

## Eager filter mode

You can press `N` to arm the filter *before* pinning anything. A `◀` triangle
appears to the right of the active tab to indicate filtering is on. Subsequent
`n` presses immediately filter (no preview step) because the filter is already
active.

## Per-tab persistence

Pins and filter state are stored per tab. Switching tabs preserves your filter
exactly as you left it -- switch away to check another tab and come back
without losing your selection.

## Mag mode interaction

When [mag mode](https://magworld.pw) (`ctrl+o`) is active, pins operate on the
mag value rather than the underlying number. Because mag compresses dollar
amounts to their order of magnitude, values that look different normally
(e.g. $1,200 and $1,800) collapse into the same bucket -- pin one and
you effectively filter by price range. Toggling mag mode translates
existing pins between representations, so your filter stays meaningful
across display modes without manual re-pinning.

## Keybindings

| Key | Action |
|-----|--------|
| `n` | Toggle pin on current cell value |
| `N` | Toggle filter activation (preview <-> active) |
| `ctrl+n` | Clear all pins and deactivate filter |

## Edge cases

- **Empty cells**: pinning an empty cell matches all rows with empty values in
  that column
- **Hidden columns**: hiding a column with `c` clears any pins on that column
- **Sorting**: sorts apply to whatever rows are visible (filtered or full)
- **Settled project toggle** (`t`): on the <a href="/docs/guide/projects/" class="tab-pill">Projects</a> tab, `t` hides completed
  and abandoned projects using the pin/filter mechanism internally
- **All rows filtered**: shows "No matches." instead of the table
