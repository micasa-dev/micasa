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
| <kbd>ctrl+q</kbd>  | Quit (exit code 0) |
| <kbd>ctrl+c</kbd>  | Cancel in-flight LLM operation |
| <kbd>ctrl+o</kbd>  | Toggle [mag mode](https://magworld.pw) for numeric values |

## Nav mode

### Movement

| Key             | Action |
|-----------------|--------|
| <kbd>j</kbd> / <kbd>down</kbd>    | Move row down |
| <kbd>k</kbd> / <kbd>up</kbd>      | Move row up |
| <kbd>h</kbd> / <kbd>left</kbd>    | Move column left (skips hidden columns) |
| <kbd>l</kbd> / <kbd>right</kbd>   | Move column right (skips hidden columns) |
| <kbd>^</kbd>             | Jump to first column |
| <kbd>$</kbd>             | Jump to last column |
| <kbd>g</kbd>             | Jump to first row |
| <kbd>G</kbd>             | Jump to last row |
| <kbd>d</kbd> / <kbd>ctrl+d</kbd>  | Half-page down |
| <kbd>u</kbd> / <kbd>ctrl+u</kbd>  | Half-page up |
| <kbd>pgdown</kbd>         | Full page down |
| <kbd>pgup</kbd>           | Full page up |

### Tabs and views

| Key             | Action |
|-----------------|--------|
| <kbd>b</kbd> / <kbd>f</kbd>       | Previous / next tab |
| <kbd>B</kbd> / <kbd>F</kbd>       | First / last tab |
| <kbd>tab</kbd>           | Toggle house profile |
| <kbd>D</kbd>             | Toggle dashboard       |

### Table operations

| Key | Action |
|-----|--------|
| <kbd>s</kbd> | Cycle sort on current column (none -> asc -> desc -> none) |
| <kbd>S</kbd> | Clear all sorts |
| <kbd>t</kbd> | <a href="/docs/guide/projects/" class="tab-pill">Projects</a> tab: toggle hiding settled projects (`completed` + `abandoned`) |
| <kbd>/</kbd> | Jump to column (fuzzy find) |
| <kbd>c</kbd> | Hide current column |
| <kbd>C</kbd> | Show all hidden columns |

### Row filtering

| Key | Action |
|-----|--------|
| <kbd>n</kbd> | Toggle pin on current cell value (preview: dim non-matching rows) |
| <kbd>N</kbd> | Toggle filter activation (hide/show non-matching rows) |
| <kbd>ctrl+n</kbd> | Clear all pins and deactivate filter |

### Actions

| Key     | Action |
|---------|--------|
| <kbd>enter</kbd> | Drill into detail view, follow FK link, or preview notes |
| <kbd>o</kbd>     | Open selected document with OS viewer (<a href="/docs/guide/documents/" class="tab-pill">Docs</a> tab only) |
| <kbd>i</kbd>     | Enter Edit mode |
| <kbd>ctrl+f</kbd> | Search documents (<a href="/docs/guide/documents/" class="tab-pill">Docs</a> tab only) |
| <kbd>@</kbd>     | Open LLM chat overlay |
| <kbd>?</kbd>     | Open help overlay |
| <kbd>esc</kbd>   | Close detail view, or clear status message |

## Edit mode

### Movement

Same as Nav mode, except <kbd>d</kbd> is rebound:

| Key            | Action |
|----------------|--------|
| <kbd>j</kbd>/<kbd>k</kbd>/<kbd>h</kbd>/<kbd>l</kbd>/<kbd>g</kbd>/<kbd>G</kbd> | Same as Nav |
| <kbd>ctrl+d</kbd>       | Half-page down |
| <kbd>pgdown</kbd>/<kbd>pgup</kbd> | Full page down/up |

### Data operations

| Key   | Action |
|-------|--------|
| <kbd>a</kbd>   | Add new entry to current tab |
| <kbd>A</kbd>   | Add document with extraction (<a href="/docs/guide/documents/" class="tab-pill">Docs</a> tab only) |
| <kbd>e</kbd>   | Edit current cell inline (date columns open calendar picker), or full form if cell is read-only |
| <kbd>E</kbd>   | Open full edit form for the selected row (regardless of column) |
| <kbd>R</kbd>   | Re-extract selected document (<a href="/docs/guide/documents/" class="tab-pill">Docs</a> tab only) |
| <kbd>d</kbd>   | Toggle delete/restore on selected row |
| <kbd>x</kbd>   | Toggle visibility of soft-deleted rows |
| <kbd>p</kbd>   | Edit house profile |
| <kbd>esc</kbd> | Return to Nav mode |

## Chat overlay

Press <kbd>@</kbd> from Nav or Edit mode to open the LLM chat. The overlay
captures all keyboard input until dismissed. See the
[LLM Chat guide]({{< ref "/docs/guide/llm-chat" >}}) for full details.

### Text input

| Key              | Action |
|------------------|--------|
| <kbd>enter</kbd>          | Submit query or slash command |
| <kbd>up</kbd> / <kbd>ctrl+p</kbd>  | Previous prompt from history |
| <kbd>down</kbd> / <kbd>ctrl+n</kbd> | Next prompt from history |
| <kbd>esc</kbd>            | Hide chat overlay (session is preserved) |
| <kbd>ctrl+s</kbd>         | Toggle SQL query display |

### Model picker

When typing `/model `, an autocomplete picker appears:

| Key              | Action |
|------------------|--------|
| <kbd>up</kbd> / <kbd>ctrl+p</kbd>  | Move cursor up |
| <kbd>down</kbd> / <kbd>ctrl+n</kbd> | Move cursor down |
| <kbd>enter</kbd>          | Select model (pulls if not downloaded) |
| <kbd>esc</kbd>            | Dismiss picker |

## Form mode

| Key       | Action |
|-----------|--------|
| <kbd>tab</kbd>     | Next field |
| <kbd>shift+tab</kbd> | Previous field |
| <kbd>ctrl+s</kbd>  | Save form |
| <kbd>esc</kbd>     | Cancel form (return to previous mode) |
| <kbd>1</kbd>-<kbd>9</kbd>   | Jump to Nth option in a select field |

### File picker

When a form field opens a file picker (e.g., <kbd>A</kbd> on the <a href="/docs/guide/documents/" class="tab-pill">Docs</a> tab):

| Key       | Action |
|-----------|--------|
| <kbd>j</kbd> / <kbd>down</kbd> | Move down the file list |
| <kbd>k</kbd> / <kbd>up</kbd>   | Move up the file list |
| <kbd>h</kbd> / <kbd>left</kbd> / <kbd>backspace</kbd> | Navigate to parent directory |
| <kbd>enter</kbd>   | Open directory or select file |
| <kbd>g</kbd> / <kbd>G</kbd> | Jump to first/last entry |
| <kbd>H</kbd> | Toggle hidden files (dotfiles) |

The picker title shows the current directory path. Hidden files are not shown
by default; press <kbd>H</kbd> to reveal them.

## Dashboard

When the dashboard overlay is open:

| Key       | Action |
|-----------|--------|
| <kbd>j</kbd>/<kbd>k</kbd>   | Move cursor down/up through items |
| <kbd>J</kbd> / <kbd>shift+down</kbd> | Jump to next section |
| <kbd>K</kbd> / <kbd>shift+up</kbd>   | Jump to previous section |
| <kbd>g</kbd>/<kbd>G</kbd>   | Jump to first/last item |
| <kbd>e</kbd>       | Toggle expand/collapse current section |
| <kbd>E</kbd>       | Toggle expand/collapse all sections |
| <kbd>enter</kbd>   | Jump to highlighted item in its tab |
| <kbd>D</kbd>       | Close dashboard |
| <kbd>b</kbd>/<kbd>f</kbd>   | Dismiss dashboard and switch tab |
| <kbd>?</kbd>       | Open help overlay (stacks on dashboard) |

## Date picker

When inline editing a date column, a calendar widget opens instead of a text
input:

| Key       | Action |
|-----------|--------|
| <kbd>h</kbd>/<kbd>l</kbd>   | Move one day left/right |
| <kbd>j</kbd>/<kbd>k</kbd>   | Move one week down/up |
| <kbd>H</kbd>/<kbd>L</kbd>   | Move one month back/forward |
| <kbd>[</kbd>/<kbd>]</kbd> | Move one year back/forward |
| <kbd>t</kbd>       | Jump to today |
| <kbd>enter</kbd>   | Pick the highlighted date |
| <kbd>esc</kbd>     | Cancel (keep original value) |

## Note preview

Press <kbd>enter</kbd> on a notes column (e.g., service log Notes) to open a read-only
overlay showing the full text. Any key dismisses it.

## Extraction overlay

When a document extraction is in progress or complete, an overlay shows
per-step progress (text, OCR, LLM) with a tabbed operation preview. The
overlay has two modes. See the
[extraction pipeline]({{< ref "/docs/guide/documents#extraction-pipeline" >}}) guide for details.

### Pipeline mode

| Key       | Action |
|-----------|--------|
| <kbd>j</kbd> / <kbd>k</kbd> | Navigate between extraction steps |
| <kbd>enter</kbd>   | Expand/collapse current step logs |
| <kbd>x</kbd>       | Enter explore mode (when operations are available) |
| <kbd>a</kbd>       | Accept results (when extraction is done with no errors) |
| <kbd>r</kbd>       | Rerun LLM step (when LLM step is complete) |
| <kbd>ctrl+b</kbd>  | Background the extraction (continue working while it runs) |
| <kbd>esc</kbd>     | Cancel extraction and close overlay |

### Explore mode

| Key       | Action |
|-----------|--------|
| <kbd>j</kbd> / <kbd>k</kbd> | Navigate rows in the active table |
| <kbd>h</kbd> / <kbd>l</kbd> | Navigate columns |
| <kbd>b</kbd> / <kbd>f</kbd> | Switch between table tabs |
| <kbd>g</kbd> / <kbd>G</kbd> | Jump to first/last row |
| <kbd>^</kbd> / <kbd>$</kbd> | Jump to first/last column |
| <kbd>a</kbd>       | Accept results |
| <kbd>x</kbd>       | Return to pipeline mode |
| <kbd>esc</kbd>     | Return to pipeline mode |

## Ops tree overlay

Press <kbd>enter</kbd> on the `Ops` column (Docs tab) to open an interactive
JSON tree of the stored extraction operations. See the
[ops tree overlay]({{< ref "/docs/guide/documents#ops-tree-overlay" >}}) guide
for details.

| Key       | Action |
|-----------|--------|
| <kbd>j</kbd> / <kbd>k</kbd> | Move cursor down/up |
| <kbd>enter</kbd> / <kbd>l</kbd> | Expand node |
| <kbd>h</kbd> | Collapse node (or jump to parent) |
| <kbd>g</kbd> / <kbd>G</kbd> | Jump to first/last visible node |
| <kbd>esc</kbd> | Close overlay |

## Document search overlay

Press <kbd>ctrl+f</kbd> on the Docs tab to open the full-text search overlay.

| Key       | Action |
|-----------|--------|
| <kbd>up</kbd> / <kbd>ctrl+k</kbd>   | Move cursor up |
| <kbd>down</kbd> / <kbd>ctrl+j</kbd> | Move cursor down |
| <kbd>enter</kbd>   | Jump to selected document |
| <kbd>esc</kbd>     | Close search |

## Help overlay

| Key       | Action |
|-----------|--------|
| <kbd>esc</kbd>     | Close help |
| <kbd>?</kbd>       | Close help |
