+++
title = "Date Input"
weight = 5
description = "Relative dates, natural language, and the calendar picker."
linkTitle = "Date Input"
+++

micasa accepts dates in strict `YYYY-MM-DD` format or as natural language
expressions. Both work anywhere a date field appears -- forms, inline edits,
and the calendar picker.

## Accepted formats

### Strict

Type a date in `YYYY-MM-DD` format (e.g., `2026-03-15`). This is always tried
first.

### Natural language

If strict parsing fails, micasa tries natural language via
[go-naturaldate](https://github.com/tj/go-naturaldate). Ambiguous expressions
default to the past.

| Expression | Resolves to |
|------------|-------------|
| `today` | Current date |
| `yesterday` | Previous day |
| `tomorrow` | Next day |
| `last friday` | Most recent Friday |
| `next tuesday` | Coming Tuesday |
| `3 days ago` | Three days before today |
| `2 weeks ago` | Two weeks before today |
| `last month` | Same day, previous month |
| `last year` | Same day, previous year |
| `december 25th` | December 25 (past direction) |

Expressions are case-insensitive and ignore surrounding whitespace. Only the
date portion is kept -- time components are discarded.

## Calendar picker

When inline editing a date column (<kbd>e</kbd> in Edit mode), a calendar widget opens
instead of a text input. Navigate with <kbd>h</kbd>/<kbd>j</kbd>/<kbd>k</kbd>/<kbd>l</kbd> (day/week),
<kbd>H</kbd>/<kbd>L</kbd> (month), <kbd>[</kbd>/<kbd>]</kbd> (year).

Press <kbd>t</kbd> to jump the calendar cursor to today's date.

Press <kbd>enter</kbd> to pick the highlighted date, <kbd>esc</kbd> to cancel.

## Where dates appear

Date fields are used throughout micasa:

- **Projects**: `Start`, `End`
- **Maintenance**: `Last` serviced date
- **Service logs**: `Date`
- **Quotes**: `Date`
- **Appliances**: `Purchased`, `Warranty`
