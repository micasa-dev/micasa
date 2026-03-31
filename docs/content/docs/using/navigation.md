+++
title = "Navigation"
weight = 1
description = "Modal keybindings and how to move around."
linkTitle = "Navigation"
+++

micasa uses vim-style modal keybindings. There are three modes: **Nav**,
**Edit**, and **Form**.

<video src="/videos/using-navigation.webm" class="demo-video" autoplay loop muted playsinline></video>

## Nav mode

Nav mode is the default. The status bar shows a blue **NAV** badge. You
have full table navigation:

| Key         | Action               |
|-------------|----------------------|
| <kbd>j</kbd> / <kbd>k</kbd>   | Move row down / up   |
| <kbd>h</kbd> / <kbd>l</kbd>   | Move column left / right (skips hidden columns) |
| <kbd>^</kbd> / <kbd>$</kbd>   | Jump to first / last column |
| <kbd>g</kbd> / <kbd>G</kbd>   | Jump to first / last row |
| <kbd>d</kbd> / <kbd>u</kbd>   | Half-page down / up  |
| <kbd>b</kbd> / <kbd>f</kbd>   | Previous / next tab  |
| <kbd>enter</kbd>     | Drill into detail, follow link, or preview |
| <kbd>s</kbd> / <kbd>S</kbd>   | Sort column / clear sorts |
| <kbd>/</kbd>         | Jump to column (fuzzy find) |
| <kbd>c</kbd> / <kbd>C</kbd>   | Hide column / show all |
| <kbd>n</kbd> / <kbd>N</kbd>   | Pin cell value / toggle filter |
| <kbd>ctrl+n</kbd>    | Clear all pins and filter |
| <kbd>tab</kbd>       | Toggle house profile |
| <kbd>D</kbd>         | Toggle dashboard       |
| <kbd>y</kbd>         | Copy cell value to clipboard |
| <kbd>i</kbd>         | Enter Edit mode      |
| <kbd>@</kbd>         | Open LLM chat        |
| <kbd>?</kbd>         | Help overlay         |

### Clipboard (yank)

Press <kbd>y</kbd> to copy the focused cell's value to the system clipboard.
The status bar briefly shows the copied value. Money values are copied
without the currency symbol.

micasa uses [OSC 52](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html#h3-Operating-System-Commands)
to set the clipboard directly through the terminal. This works over SSH
and doesn't require external tools like `xclip` or `xsel`. Most modern
terminals support it, but some need explicit configuration:

| Terminal | Configuration |
|----------|---------------|
| tmux | `set -g set-clipboard on` in `~/.tmux.conf` |
| GNU Screen | Not supported (Screen does not forward OSC 52) |
| Alacritty | Enabled by default |
| iTerm2 | Preferences → General → Selection → enable "Applications in terminal may access clipboard" |
| kitty | Enabled by default |
| WezTerm | Enabled by default |
| Windows Terminal | Enabled by default |
| foot | Enabled by default |
| GNOME Terminal | Supported since 3.46 |

If your terminal doesn't support OSC 52, the keypress is silently
ignored — nothing breaks, you just don't get clipboard content.

## Edit mode

Press <kbd>i</kbd> from Nav mode to enter Edit mode. The status bar shows an orange
**EDIT** badge. Navigation still works (<kbd>j</kbd>/<kbd>k</kbd>/<kbd>h</kbd>/<kbd>l</kbd>/<kbd>g</kbd>/<kbd>G</kbd>), but <kbd>d</kbd>
is rebound from page navigation to delete:

| Key   | Action                    |
|-------|---------------------------|
| <kbd>a</kbd>   | Add new entry             |
| <kbd>e</kbd>   | Edit cell or full row     |
| <kbd>E</kbd>   | Open full edit form       |
| <kbd>d</kbd>   | Delete or restore item    |
| <kbd>x</kbd>   | Toggle show deleted items |
| <kbd>p</kbd>   | Edit house profile        |
| <kbd>esc</kbd> | Return to Nav mode     |

> **Tip:** <kbd>ctrl+d</kbd> still works for half-page down in Edit mode.

## Form mode

When you add or edit an entry, micasa opens a form. Use <kbd>tab</kbd> / <kbd>shift+tab</kbd>
to move between fields, type to fill them in.

| Key      | Action          |
|----------|-----------------|
| <kbd>ctrl+s</kbd> | Save and close  |
| <kbd>esc</kbd>    | Cancel          |
| <kbd>1</kbd>-<kbd>9</kbd>  | Jump to Nth option in select fields |

The form shows a dirty indicator when you've changed something. After saving
or canceling, you return to whichever mode you were in before (Nav or
Edit).

## Tabs

The main data lives in six tabs: <a href="/docs/guide/projects/" class="tab-pill">Projects</a>, <a href="/docs/guide/quotes/" class="tab-pill">Quotes</a>, <a href="/docs/guide/maintenance/" class="tab-pill">Maintenance</a>,
<a href="/docs/guide/appliances/" class="tab-pill">Appliances</a>, <a href="/docs/guide/vendors/" class="tab-pill">Vendors</a>, and <a href="/docs/guide/documents/" class="tab-pill">Docs</a>. Use <kbd>b</kbd> / <kbd>f</kbd> to cycle between
them. The active tab is highlighted in the tab bar.

## Detail views

Some columns are drill columns (marked `↘` in the header) -- pressing <kbd>enter</kbd> on them opens a sub-table.
For example:

- `Log` column on the <a href="/docs/guide/maintenance/" class="tab-pill">Maintenance</a> tab opens the service log for that item
- `Maint` column on the <a href="/docs/guide/appliances/" class="tab-pill">Appliances</a> tab opens maintenance items linked to
  that appliance
- `Docs` column on the <a href="/docs/guide/projects/" class="tab-pill">Projects</a> or <a href="/docs/guide/appliances/" class="tab-pill">Appliances</a> tab opens linked documents

A breadcrumb bar replaces the tab bar while in a detail view (e.g.,
`Maintenance > HVAC filter replacement`). Press <kbd>esc</kbd> to close the detail
view and return to the parent tab.

## Horizontal scrolling

When a table has more columns than fit on screen, it scrolls horizontally as
you move with <kbd>h</kbd>/<kbd>l</kbd> or <kbd>^</kbd>/<kbd>$</kbd>. Scroll indicators appear in the column
headers: a **◀** on the leftmost header when columns are off-screen to the
left, and a **▶** on the rightmost header when columns are off-screen to the
right.

## Foreign key links

Some columns reference entities in other tabs. When at least one row in the
column has a link, a `→` arrow appears in the column header. When the cursor
is on a linked cell, the status bar shows `follow →`. Press <kbd>enter</kbd> to jump
to the referenced row in the target tab. If the cell has no link (e.g.
"Self" in the `Performed By` column), the status bar shows a brief message
instead.

Examples:
- Quotes `Project` column links to the <a href="/docs/guide/projects/" class="tab-pill">Projects</a> tab
- Quotes `Vendor` column links to the <a href="/docs/guide/vendors/" class="tab-pill">Vendors</a> tab
- Maintenance `Appliance` column links to the <a href="/docs/guide/appliances/" class="tab-pill">Appliances</a> tab
- Service log `Performed By` column links to the <a href="/docs/guide/vendors/" class="tab-pill">Vendors</a> tab
