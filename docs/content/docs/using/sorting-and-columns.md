+++
title = "Sorting and Columns"
weight = 2
description = "Multi-column sorting, column hiding, and horizontal scrolling."
linkTitle = "Sorting & Columns"
+++

micasa supports multi-column sorting, a fuzzy column finder, column hiding,
and horizontal scrolling.

<video src="/videos/using-sorting.webm" class="demo-video" autoplay loop muted playsinline></video>

## Multi-column sorting

micasa supports multi-column sorting in Nav mode.

### How it works

1. Navigate to a column with <kbd>h</kbd>/<kbd>l</kbd>
2. Press <kbd>s</kbd> to cycle: **none** -> **ascending** -> **descending** -> **none**
3. Repeat on other columns to add secondary sort keys

The column header shows sort indicators:

- `▲1` = ascending, priority 1 (primary sort)
- `▼2` = descending, priority 2 (secondary sort)

There is no limit on the number of sort columns. Priority is determined by the
order you add sorts: the first column you sort is priority 1, the second is
priority 2, and so on.

### Sort behavior

- **Smart comparators**: sorts are type-aware. Money columns sort numerically,
  date columns sort chronologically, text columns sort lexicographically.
- **Empty values sort last**: regardless of sort direction, empty cells always
  appear at the bottom.
- **Default sort**: when no explicit sorts are active, rows are sorted by ID
  ascending (primary key order).
- **Tiebreaker**: the primary key is always used as an implicit tiebreaker to
  ensure stable ordering.
- **Single-column sorts** skip the priority number in the header indicator for
  a cleaner look.

### Clearing sorts

Press <kbd>S</kbd> (capital S) to clear all sort criteria and return to default PK
ordering.

## Fuzzy column finder

Press <kbd>/</kbd> in Nav mode to open a fuzzy finder overlay. Type to filter
columns by name -- matched characters are highlighted. Use <kbd>up</kbd>/<kbd>down</kbd> to
navigate the list, <kbd>enter</kbd> to jump to the selected column, <kbd>esc</kbd> to cancel.

Jumping to a hidden column automatically unhides it.

## Column hiding

You can hide columns you don't need to reduce noise. This is session-only --
hidden columns come back when you restart.

### Hiding

In Nav mode, navigate to a column and press <kbd>c</kbd> to hide it. The column
disappears from the table. Hidden column names are shown as color-coded badges
below the table and listed in the status bar.

You can't hide the last visible column.

### Showing

Press <kbd>C</kbd> (capital C) to show all hidden columns at once.

## Horizontal scrolling

When the table has more columns than fit on screen, micasa scrolls
horizontally. The viewport follows your column cursor -- as you move right
past the visible edge, the view scrolls to keep the cursor on screen.

Scroll indicators (`◀` / `▶`) appear in the edge column headers when there are
columns off-screen.
