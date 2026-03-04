+++
title = "Navigation"
weight = 1
description = "Modal keybindings and how to move around."
linkTitle = "Navigation"
+++

micasa uses vim-style modal keybindings. There are three modes: **Nav**,
**Edit**, and **Form**.

<video src="/videos/using-navigation.webm" autoplay loop muted playsinline style="max-width:100%;border-radius:8px"></video>

## Nav mode

Nav mode is the default. The status bar shows a blue **NAV** badge. You
have full table navigation:

| Key         | Action               |
|-------------|----------------------|
| `j` / `k`   | Move row down / up   |
| `h` / `l`   | Move column left / right (skips hidden columns) |
| `^` / `$`   | Jump to first / last column |
| `g` / `G`   | Jump to first / last row |
| `d` / `u`   | Half-page down / up  |
| `b` / `f`   | Previous / next tab  |
| `enter`     | Drill into detail, follow link, or preview |
| `s` / `S`   | Sort column / clear sorts |
| `/`         | Jump to column (fuzzy find) |
| `c` / `C`   | Hide column / show all |
| `n` / `N`   | Pin cell value / toggle filter |
| `ctrl+n`    | Clear all pins and filter |
| `tab`       | Toggle house profile |
| `D`         | Toggle dashboard       |
| `i`         | Enter Edit mode      |
| `@`         | Open LLM chat        |
| `?`         | Help overlay         |

## Edit mode

Press `i` from Nav mode to enter Edit mode. The status bar shows an orange
**EDIT** badge. Navigation still works (`j`/`k`/`h`/`l`/`g`/`G`), but `d`
and `u` are rebound from page navigation to data actions:

| Key   | Action                    |
|-------|---------------------------|
| `a`   | Add new entry             |
| `e`   | Edit cell or full row     |
| `E`   | Open full edit form       |
| `d`   | Delete or restore item    |
| `x`   | Toggle show deleted items |
| `p`   | Edit house profile        |
| `u`   | Undo last edit            |
| `r`   | Redo undone edit          |
| `esc` | Return to Nav mode     |

> **Tip:** `ctrl+d` and `ctrl+u` still work for half-page navigation in Edit
> mode.

## Form mode

When you add or edit an entry, micasa opens a form. Use `tab` / `shift+tab`
to move between fields, type to fill them in.

| Key      | Action          |
|----------|-----------------|
| `ctrl+s` | Save and close  |
| `esc`    | Cancel          |
| `1`-`9`  | Jump to Nth option in select fields |

The form shows a dirty indicator when you've changed something. After saving
or canceling, you return to whichever mode you were in before (Nav or
Edit).

## Tabs

The main data lives in six tabs: **Projects**, **Quotes**, **Maintenance**,
**Appliances**, **Vendors**, and **Docs**. Use `b` / `f` to cycle between
them. The active tab is highlighted in the tab bar.

## Detail views

Some columns are drill columns (marked `↘` in the header) -- pressing `enter` on them opens a sub-table.
For example:

- `Log` column on the Maintenance tab opens the service log for that item
- `Maint` column on the Appliances tab opens maintenance items linked to
  that appliance
- `Docs` column on the Projects or Appliances tab opens linked documents

A breadcrumb bar replaces the tab bar while in a detail view (e.g.,
`Maintenance > HVAC filter replacement`). Press `esc` to close the detail
view and return to the parent tab.

## Horizontal scrolling

When a table has more columns than fit on screen, it scrolls horizontally as
you move with `h`/`l` or `^`/`$`. Scroll indicators appear in the column
headers: a **◀** on the leftmost header when columns are off-screen to the
left, and a **▶** on the rightmost header when columns are off-screen to the
right.

## Foreign key links

Some columns reference entities in other tabs. When at least one row in the
column has a link, a `→` arrow appears in the column header. When the cursor
is on a linked cell, the status bar shows `follow →`. Press `enter` to jump
to the referenced row in the target tab. If the cell has no link (e.g.
"Self" in the `Performed By` column), the status bar shows a brief message
instead.

Examples:
- Quotes `Project` column links to the Projects tab
- Quotes `Vendor` column links to the Vendors tab
- Maintenance `Appliance` column links to the Appliances tab
- Service log `Performed By` column links to the Vendors tab
