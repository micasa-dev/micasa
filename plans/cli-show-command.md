<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# CLI `show` Command

Design spec for a comprehensive CLI for printing micasa data to the console as
plain text (for LLMs and humans) and JSON (for programmatic processing).

## Commands

```
micasa show <entity> [--json] [--deleted] [database-path]
micasa show all      [--json] [--deleted] [database-path]
micasa query <sql>   [--json] [database-path]
```

The database path is a positional argument (consistent with existing CLI
commands like `micasa backup`, `micasa pro status`). Defaults to
`~/.local/share/micasa/micasa.db` (Linux) or platform equivalent, honors
`MICASA_DB_PATH`.

### Entity names

| Argument                 | Model              | JSON key                 | Notes                          |
|--------------------------|--------------------|--------------------------|--------------------------------|
| `house`                  | HouseProfile       | `house`                  | Singleton, key-value text      |
| `projects`               | Project            | `projects`               | Joins ProjectType for type name|
| `project-types`          | ProjectType        | `project_types`          | Lookup table                   |
| `quotes`                 | Quote              | `quotes`                 | Joins Project + Vendor names   |
| `vendors`                | Vendor             | `vendors`                |                                |
| `maintenance`            | MaintenanceItem    | `maintenance`            | Joins Category + Appliance     |
| `maintenance-categories` | MaintenanceCategory| `maintenance_categories` | Lookup table                   |
| `service-log`            | ServiceLogEntry    | `service_log`            | Joins MaintenanceItem + Vendor |
| `appliances`             | Appliance          | `appliances`             |                                |
| `incidents`              | Incident           | `incidents`              | Joins Appliance + Vendor       |
| `documents`              | Document           | `documents`              | Metadata only, no blob         |
| `all`                    | All of the above   | (top-level keys)         | Sequential dump                |

CLI arguments use hyphens (`service-log`), JSON keys use underscores
(`service_log`).

### Flags

- `--json` -- Output as JSON instead of tabwriter text.
- `--deleted` -- Include soft-deleted rows. In text mode, adds a DELETED
  column. In JSON mode, adds a `deleted_at` field to each object (normally
  omitted by the `json:"-"` tag).

## Text output format

### Collection entities (tabwriter)

```
=== PROJECTS (3) ===
TITLE              TYPE          STATUS     START        END          BUDGET      ACTUAL
Replace HVAC       renovation    planned    -            -            $8,500.00   -
Paint exterior     cosmetic      underway   2026-01-15   -            -           -
Fix leak           repair        completed  2026-02-01   2026-02-10   -           $350.00
```

Formatting rules:
- Money (`*_cents` fields): `$X.XX` (cents divided by 100)
- Dates: `YYYY-MM-DD`
- Null/empty: `-`
- Omitted columns: `id`, `created_at`, `updated_at`, `deleted_at` (unless
  `--deleted`, which adds a DELETED column)
- FK references: resolved to human-readable names (project type name, vendor
  name, etc.)

### Singleton entity (house)

Key-value format since there is only one row:

```
=== HOUSE ===
Nickname:          My Place
Address:           123 Main St, Springfield, IL 62701
Year Built:        1985
Square Feet:       2,400
Bedrooms:          3
Bathrooms:         2.5
...
```

Address is composed from model fields: `AddressLine1`, optionally
`AddressLine2` on a new indented line, then `City, State PostalCode`.
Components are omitted when empty.

### `show all`

Concatenates all entity sections with `=== ENTITY (N) ===` headers. Empty
entities are omitted (silence is success).

## JSON output format

### Individual entity (collection)

```json
[
  {"id": "01J...", "title": "Replace HVAC", "status": "planned", ...},
  ...
]
```

JSON output uses custom marshaling (not raw `json.Marshal` on model structs)
because:
- Association fields (ProjectType, Vendor, etc.) are tagged `json:"-"` on the
  models but need to appear as resolved names in output.
- `DeletedAt` is tagged `json:"-"` but must appear when `--deleted` is set.

Each entity defines a `toJSON` method that returns a `map[string]any` with:
- All scalar fields from the model (using the existing `json` tag names)
- FK references resolved to names (e.g. `"project_type": "renovation"` instead
  of just `"project_type_id": "01J..."`)
- `"deleted_at"` included only when `--deleted` is set and the row is deleted
- Money as cents (callers handle conversion)
- Dates as RFC 3339

### Individual entity (house singleton)

```json
{"id": "01J...", "nickname": "My Place", ...}
```

Single object, not wrapped in array.

### `show all`

```json
{
  "house": {...},
  "projects": [...],
  "project_types": [...],
  "quotes": [...],
  "vendors": [...],
  "maintenance": [...],
  "maintenance_categories": [...],
  "service_log": [...],
  "appliances": [...],
  "incidents": [...],
  "documents": [...]
}
```

## `micasa query`

Wraps the existing `Store.ReadOnlyQuery()` for CLI use.

```
micasa query 'SELECT title, status FROM projects WHERE status = "planned"'
micasa query 'SELECT ...' --json
```

- Text mode: tabwriter with column headers from SQL result columns.
- JSON mode: array of objects with column names as keys, all values as strings
  (matching ReadOnlyQuery's return type).
- Inherits ReadOnlyQuery's 200-row cap and 10s timeout.
- Inherits all 5 validation layers (prefix, semicolon, keyword, EXPLAIN,
  timeout).

## Error handling

- Errors print to stderr and exit with code 1.
- Invalid entity name: `unknown entity "foo"; valid entities: house, projects,
  ...`
- Missing/unreadable database: propagate Store.Open error.
- Empty results: no output, exit 0 (silence is success).
- `query` with invalid SQL: propagate ReadOnlyQuery error message.

## Implementation

### Files

- `cmd/micasa/show.go` -- Cobra `show` parent command + per-entity subcommands
  + `all` subcommand. Flag parsing, Store opening, output formatting.
- `cmd/micasa/query.go` -- Cobra `query` command. Wraps ReadOnlyQuery.

### Data access

Reuses existing Store methods:
- `HouseProfile()` for house
- `ListProjects(includeDeleted)`, `ListQuotes(includeDeleted)`, etc.
- `ReadOnlyQuery(sql)` for the query command

New Store method needed:
- `ListAllServiceLogEntries(includeDeleted bool)` -- lists all service log
  entries across all maintenance items. The existing `ListServiceLog` is scoped
  to a single maintenance item ID.

For FK joins (quotes needing project title + vendor name, etc.), the existing
List methods already use GORM `Preload` on association fields.

### Output formatting

A generic `showTable[T]` function that takes a `[]T`, column definitions, and
an `io.Writer`, and renders either tabwriter or JSON. Each entity registers:
- Column names (for tabwriter header)
- A row-extraction function (model struct -> []string for text)
- A `toJSON` function (model struct -> map[string]any for JSON)

This avoids duplicating the tabwriter boilerplate across 11 entity types.

### Column selection per entity

Each entity defines which fields appear in text mode and their display order.
These are separate from `coldefs.go` (which is TUI-specific) -- the CLI columns
are simpler and don't need sort/filter/width metadata.

Zero-value suppression is per-column, not blanket. Fields like `IntervalMonths`
where 0 is meaningful show `0`; fields like `YearBuilt` where 0 means "not
set" show `-`.

### Formatting helpers

- `fmtMoney(*int64) string` -- nil -> "-", else "$X.XX"
- `fmtDate(*time.Time) string` -- nil -> "-", else "YYYY-MM-DD"
- `fmtStr(string) string` -- empty -> "-", else value
- `fmtOptInt(int, bool) string` -- if suppress && 0 -> "-", else value

## Testing

- Test each entity's text and JSON output with demo data.
- Test `show all` concatenation.
- Test `query` with valid SELECT and rejected mutations.
- Test `--deleted` flag includes soft-deleted rows with deletion timestamp.
- Test empty database produces no output (silence is success).
- Test JSON includes resolved FK names (not just IDs).

## Non-goals

- No entity-specific filter flags (use `micasa query` for filtered queries).
- No CSV output (use `--json | jq -r` for CSV-like output).
- No interactive/pager mode.
- No document blob export (metadata only).
