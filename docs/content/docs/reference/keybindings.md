+++
title = "Keybindings"
weight = 1
description = "Complete reference of every keybinding."
linkTitle = "Keybindings"
+++

Complete reference of every keybinding in micasa, organized by mode.

## Global (all modes)

| Key       | Action |
|-----------|--------|
| `ctrl+q`  | Quit (exit code 0) |
| `ctrl+c`  | Cancel in-flight LLM operation |
| `ctrl+o`  | Toggle [mag mode](https://magworld.pw) for numeric values |

## Nav mode

### Movement

| Key             | Action |
|-----------------|--------|
| `j` / `down`    | Move row down |
| `k` / `up`      | Move row up |
| `h` / `left`    | Move column left (skips hidden columns) |
| `l` / `right`   | Move column right (skips hidden columns) |
| `^`             | Jump to first column |
| `$`             | Jump to last column |
| `g`             | Jump to first row |
| `G`             | Jump to last row |
| `d` / `ctrl+d`  | Half-page down |
| `u` / `ctrl+u`  | Half-page up |
| `pgdown`         | Full page down |
| `pgup`           | Full page up |

### Tabs and views

| Key             | Action |
|-----------------|--------|
| `b` / `f`       | Previous / next tab |
| `B` / `F`       | First / last tab |
| `tab`           | Toggle house profile |
| `D`             | Toggle dashboard       |

### Table operations

| Key | Action |
|-----|--------|
| `s` | Cycle sort on current column (none -> asc -> desc -> none) |
| `S` | Clear all sorts |
| `t` | Projects tab: toggle hiding settled projects (`completed` + `abandoned`) |
| `/` | Jump to column (fuzzy find) |
| `c` | Hide current column |
| `C` | Show all hidden columns |

### Row filtering

| Key | Action |
|-----|--------|
| `n` | Toggle pin on current cell value (preview: dim non-matching rows) |
| `N` | Toggle filter activation (hide/show non-matching rows) |
| `ctrl+n` | Clear all pins and deactivate filter |

### Actions

| Key     | Action |
|---------|--------|
| `enter` | Drill into detail view, follow FK link, or preview notes |
| `o`     | Open selected document with OS viewer (Docs tab only) |
| `i`     | Enter Edit mode |
| `@`     | Open LLM chat overlay |
| `?`     | Open help overlay |
| `esc`   | Close detail view, or clear status message |

## Edit mode

### Movement

Same as Nav mode, except `d` and `u` are rebound:

| Key            | Action |
|----------------|--------|
| `j`/`k`/`h`/`l`/`g`/`G` | Same as Nav |
| `ctrl+d`       | Half-page down |
| `ctrl+u`       | Half-page up |
| `pgdown`/`pgup` | Full page down/up |

### Data operations

| Key   | Action |
|-------|--------|
| `a`   | Add new entry to current tab |
| `A`   | Add document with extraction (Docs tab only) |
| `e`   | Edit current cell inline (date columns open calendar picker), or full form if cell is read-only |
| `d`   | Toggle delete/restore on selected row |
| `x`   | Toggle visibility of soft-deleted rows |
| `p`   | Edit house profile |
| `u`   | Undo last edit |
| `r`   | Redo undone edit |
| `esc` | Return to Nav mode |

## Chat overlay

Press `@` from Nav or Edit mode to open the LLM chat. The overlay
captures all keyboard input until dismissed. See the
[LLM Chat guide]({{< ref "/docs/guide/llm-chat" >}}) for full details.

### Text input

| Key              | Action |
|------------------|--------|
| `enter`          | Submit query or slash command |
| `up` / `ctrl+p`  | Previous prompt from history |
| `down` / `ctrl+n` | Next prompt from history |
| `esc`            | Hide chat overlay (session is preserved) |
| `ctrl+s`         | Toggle SQL query display |

### Model picker

When typing `/model `, an autocomplete picker appears:

| Key              | Action |
|------------------|--------|
| `up` / `ctrl+p`  | Move cursor up |
| `down` / `ctrl+n` | Move cursor down |
| `enter`          | Select model (pulls if not downloaded) |
| `esc`            | Dismiss picker |

## Form mode

| Key       | Action |
|-----------|--------|
| `tab`     | Next field |
| `shift+tab` | Previous field |
| `ctrl+s`  | Save form |
| `esc`     | Cancel form (return to previous mode) |
| `1`-`9`   | Jump to Nth option in a select field |

## Dashboard

When the dashboard overlay is open:

| Key       | Action |
|-----------|--------|
| `j`/`k`   | Move cursor down/up through items |
| `g`/`G`   | Jump to first/last item |
| `enter`   | Jump to highlighted item in its tab |
| `D`       | Close dashboard |
| `b`/`f`   | Dismiss dashboard and switch tab |
| `?`       | Open help overlay (stacks on dashboard) |

## Date picker

When inline editing a date column, a calendar widget opens instead of a text
input:

| Key       | Action |
|-----------|--------|
| `h`/`l`   | Move one day left/right |
| `j`/`k`   | Move one week down/up |
| `H`/`L`   | Move one month back/forward |
| `[`/`]` | Move one year back/forward |
| `enter`   | Pick the highlighted date |
| `esc`     | Cancel (keep original value) |

## Note preview

Press `enter` on a notes column (e.g., service log Notes) to open a read-only
overlay showing the full text. Any key dismisses it.

## Extraction overlay

When a document extraction is in progress or complete, an overlay shows
per-step progress (text, OCR, LLM). See the
[Documents guide]({{< ref "/docs/guide/documents" >}}) for details.

| Key       | Action |
|-----------|--------|
| `j` / `k` | Navigate between extraction steps |
| `enter`   | Expand/collapse current step logs |
| `a`       | Accept results (when extraction is done with no errors) |
| `r`       | Rerun LLM step (when LLM step is complete) |
| `esc`     | Cancel extraction and close overlay |

## Help overlay

| Key       | Action |
|-----------|--------|
| `esc`     | Close help |
| `?`       | Close help |
