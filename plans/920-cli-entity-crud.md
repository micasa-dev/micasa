<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# CLI Entity CRUD Commands

Design spec for full create/read/update/delete/restore CLI support for all
entity types, enabling AI agents and scripts to interact with micasa
programmatically without the TUI.

Tracks: [#920](https://github.com/micasa-dev/micasa/issues/920)

## Motivation

The `show` command already provides read-only entity listing, and `query`
allows ad-hoc SELECT queries. This feature adds the write side: add, edit,
delete, and restore operations for every entity. The primary consumer is AI
agents (e.g., OpenClaw) that need a CLI surface for full home management
automation, but the commands are equally useful for scripting and bulk
operations.

## Command Structure

Use `micasa <entity> <verb>` structure, registering each entity as a
subcommand of the root command with verbs as sub-subcommands. This matches
the existing `micasa show <entity>` pattern and keeps the top-level
namespace flat and discoverable.

```
micasa <entity> list    [--table] [--deleted] [database-path]
micasa <entity> get     <id>      [--table]   [database-path]
micasa <entity> add     --data <json>         [database-path]
micasa <entity> edit    <id> --data <json>    [database-path]
micasa <entity> delete  <id>                  [database-path]
micasa <entity> restore <id>                  [database-path]
```

### Alternative Considered: `micasa entity <entity> <verb>`

Grouping under a single `entity` parent command was considered but rejected:
- Adds unnecessary depth (`micasa entity vendor list` vs `micasa vendor list`)
- Less discoverable -- users must know the `entity` namespace exists
- Inconsistent with `show` which uses `micasa show <entity>` directly

### Alternative Considered: Reusing `show` for `list`

The existing `micasa show <entity>` already provides listing. Options:
1. **Deprecate `show` in favor of per-entity `list`** -- breaking change
2. **Keep `show` and add `list` as alias** -- duplicate code paths
3. **Keep `show` as-is, add only write verbs** -- less uniform but non-breaking

**Decision**: Option A -- deprecate `show` now. Add deprecation notice to
`show --help` output. Plan removal in next major version. The new
`<entity> list` commands subsume `show`'s functionality with JSON-default
output and a `--table` flag for human-readable format. The `show` command
continues to work but prints a deprecation warning to stderr on each
invocation.

## Entity Inventory

### Soft-deletable entities (full CRUD + restore)

| Entity CLI name | Model             | Verbs                           |
|-----------------|-------------------|---------------------------------|
| `project`       | Project           | list, get, add, edit, delete, restore |
| `vendor`        | Vendor            | list, get, add, edit, delete, restore |
| `appliance`     | Appliance         | list, get, add, edit, delete, restore |
| `incident`      | Incident          | list, get, add, edit, delete, restore |
| `quote`         | Quote             | list, get, add, edit, delete, restore |
| `maintenance`   | MaintenanceItem   | list, get, add, edit, delete, restore |
| `service-log`   | ServiceLogEntry   | list, get, add, edit, delete, restore |
| `document`      | Document          | list, get, add, edit, delete, restore |

### Singleton entity

| Entity CLI name | Model        | Verbs         |
|-----------------|--------------|---------------|
| `house`         | HouseProfile | get, add, edit |

House has no `list` (singleton), no `delete` (not soft-deletable), no
`restore`. `get` returns the profile or an empty object. `add` creates if
none exists; errors if one already exists. `edit` updates the existing one.

### Lookup tables (read-only)

| Entity CLI name          | Model               | Verbs |
|--------------------------|----------------------|-------|
| `project-type`           | ProjectType          | list  |
| `maintenance-category`   | MaintenanceCategory  | list  |

These are seeded at startup and not user-mutable. Expose `list` only so
agents can discover valid FK values.

## Entity Naming Convention

- CLI arguments use **singular** entity names: `micasa vendor list`, not
  `micasa vendors list`. Singular is more natural with `add`/`get`/`delete`
  verbs ("add a vendor", "get a vendor").
- Hyphenated for multi-word: `service-log`, `project-type`,
  `maintenance-category`.
- JSON keys in input/output use **snake_case** matching the existing JSON
  tag conventions from `models.go`.

## Input Format

### `--data` flag (JSON string)

All `add` and `edit` commands accept a `--data` flag containing a JSON
object with field values. Fields use the same `json:"..."` tag names from
`models.go`.

```bash
micasa vendor add --data '{"name":"Acme Plumbing","phone":"5551234567"}'
micasa vendor edit 01JQKX... --data '{"phone":"5559876543"}'
```

### `--data-file` flag (JSON file path)

For larger payloads or when constructing JSON in a separate step:

```bash
micasa project add --data-file project.json
```

`--data` and `--data-file` are mutually exclusive. Exactly one must be
provided for `add` and `edit` verbs.

### Partial updates for `edit`

The `edit` command performs a **partial update**: only fields present in the
JSON payload are modified. Missing fields retain their current values. To
explicitly clear an optional field, set it to `null`.

```bash
# Clear the phone number
micasa vendor edit 01JQKX... --data '{"phone":""}'

# Set only the status
micasa project edit 01JQKX... --data '{"status":"completed"}'
```

### Money fields

Money fields accept **cents** as integers, matching the internal storage
format and the existing JSON output from `show`. This avoids floating-point
ambiguity. Field names end in `_cents`: `budget_cents`, `cost_cents`,
`total_cents`, etc.

```bash
micasa project add --data '{"title":"Fence","project_type_id":"01JQKX...","budget_cents":500000}'
```

### Date fields

Dates accept `YYYY-MM-DD` strings, matching the existing `DateLayout` in
`validation.go`.

```bash
micasa incident add --data '{"title":"Pipe burst","date_noticed":"2026-04-14"}'
```

### FK references

FK fields accept the target entity's ID string:

```bash
micasa quote add --data '{"project_id":"01JQKX...","vendor_id":"01JQKX...","total_cents":750000}'
```

For `quote` and `service-log`, a `vendor_name` convenience field is also
accepted. When provided, it triggers the existing `FindOrCreateVendor`
logic -- matching the TUI form behavior:

```bash
micasa quote add --data '{"project_id":"01JQKX...","vendor_name":"TopRoof Inc","total_cents":750000}'
```

If both `vendor_id` and `vendor_name` are provided, `vendor_id` takes
precedence and `vendor_name` is ignored.

## Output Format

### `list` and `get`

Default output is **JSON** (not table). JSON is the natural format for
programmatic consumers (the primary audience for CLI CRUD).

A `--table` flag switches to human-readable tabwriter output, matching
the existing `show` format.

```bash
# JSON (default)
micasa vendor list
micasa vendor get 01JQKX...

# Table
micasa vendor list --table
```

The `list` output is a JSON array. The `get` output is a single JSON
object (not wrapped in an array).

### `add` and `edit`

On success, print the created/updated entity as a JSON object to stdout.
This allows callers to capture the generated ID and verify the result.

### `delete` and `restore`

On success, print nothing (Unix silence-is-success). On error, print the
error message to stderr and exit non-zero.

### Error output

All errors go to stderr. Exit code 1 for all errors. Error messages are
the store-layer error strings, which already include actionable hints
(e.g., "vendor has 3 active quote(s) -- delete them first").

## Validation

Input validation happens in two layers:

1. **JSON decode**: Malformed JSON or type mismatches produce clear errors
   ("invalid JSON: ...", "field X: expected string, got number").
2. **Store layer**: FK validation, uniqueness constraints, dependency
   checks. These errors propagate as-is since they already have good
   messages.

Date fields are validated through `ParseRequiredDate`/`ParseOptionalDate`.
Money fields are validated as integers (no float parsing needed since
input is cents).

## Database Path Resolution

Same precedence as all existing CLI commands:
1. Positional argument (last arg)
2. `MICASA_DB_PATH` environment variable
3. Platform default (`data.DefaultDBPath()`)

Uses `openExisting()` from `pro.go` -- the database must already exist.
The CLI CRUD commands are not intended to bootstrap a new database.

## Entity-Specific Notes

### House

- `get`: Returns the singleton or `{}` if none exists (not an error)
- `add`: Creates. Errors if profile already exists.
- `edit`: Requires existing profile. All fields optional (partial update).
- No `list`, `delete`, `restore`.

### Quote

- `add`/`edit`: Accept `vendor_name` as convenience field. When provided,
  pass a `Vendor{Name: vendorName}` to `CreateQuote`/`UpdateQuote`, which
  internally calls `findOrCreateVendor` (in `store_vendor.go`).
- When `vendor_id` is provided instead, the CLI fetches the vendor by ID
  first, then passes the fetched `Vendor` struct to `CreateQuote`/
  `UpdateQuote`. This ensures `findOrCreateVendor` finds the existing
  vendor by name rather than creating a duplicate.
- `project_id` is required for `add`.
- Either `vendor_id` or `vendor_name` is required for `add`.

### Service Log

- `add`/`edit`: Accept `vendor_name` as convenience field. When provided,
  pass a `Vendor{Name: vendorName}` to `CreateServiceLog`/
  `UpdateServiceLog`, which internally calls `findOrCreateVendor`.
- When `vendor_id` is provided, the CLI fetches the vendor by ID first,
  then passes the fetched `Vendor` struct (same pattern as Quote).
- `maintenance_item_id` is required for `add`.
- `serviced_at` is required for `add`.

### Document

- `add`: Accepts `--file <path>` to upload a file. When provided, reads
  the file data, computes SHA256, detects MIME type, and sets size. The
  `title` defaults to the file's base name if not explicitly provided.
  Without `--file`, creates a metadata-only record.
- `entity_kind` and `entity_id` are required for `add` to link the
  document to a parent entity.
- `maxDocumentSize` is enforced via config. The CLI loads config to set
  this limit before creating documents.

### Maintenance

- `category_id` is required for `add`.

## `show` Deprecation Plan

The existing `micasa show <entity>` command is deprecated in favor of the
new `micasa <entity> list --table` commands.

### Changes to `show`

1. Add a deprecation notice to every `show` subcommand's `--help` output:
   `Deprecated: use 'micasa <entity> list --table' instead.`
2. On each invocation, print a one-line deprecation warning to stderr:
   `Warning: 'show' is deprecated. Use 'micasa <entity> list --table' instead.`
3. Mark each show subcommand as deprecated via cobra's `Deprecated` field
   (which automatically hides the command from parent help and prints
   the deprecation message).

### Removal timeline

`show` will be removed in the next major version. No code changes are
needed until then -- cobra's `Deprecated` field handles the warnings
automatically.

## Implementation Approach

### File Organization

- `cmd/micasa/entity.go` -- Generic helpers: JSON decode, entity command
  builder, shared flags, openExisting wrapper.
- `cmd/micasa/entity_<name>.go` -- Per-entity: field definitions, list/get
  rendering (reusing show.go functions), add/edit/delete/restore handlers.
- `cmd/micasa/entity_test.go` -- Integration tests exercising full
  lifecycle (add -> list -> get -> edit -> delete -> restore) per entity.

### Generic Command Builder

A builder function constructs the cobra command tree for each entity,
reducing boilerplate:

```go
type entityDef[T any] struct {
    name     string         // CLI name: "vendor"
    singular string         // Display: "vendor"
    // list/get/add/edit/delete/restore handlers
}

func buildEntityCmd[T any](def entityDef[T]) *cobra.Command
```

Each entity provides its own handlers for decode, create, update, and
JSON rendering. The builder wires them into a uniform command tree with
shared flags.

### Registration

Entity commands are registered in `main.go`'s `newRootCmd()`:

```go
root.AddCommand(
    // ... existing commands ...
    newVendorCmd(),
    newProjectCmd(),
    newApplianceCmd(),
    // ...
)
```

## Implementation Order

1. **vendor** -- Simplest entity, good for proving the pattern
2. **project** -- Adds FK (project_type_id), status enum
3. **appliance** -- Similar to vendor, validates the pattern scales
4. **incident** -- Multiple FKs (appliance_id, vendor_id), enums
5. **quote** -- Vendor resolution (vendor_name convenience field)
6. **maintenance** -- FK to category + optional appliance
7. **service-log** -- FK to maintenance item + vendor resolution
8. **document** -- Polymorphic entity link, optional `--file` blob upload
9. **house** -- Singleton, different verb set
10. **project-type**, **maintenance-category** -- Read-only list

## Testing

Each entity gets lifecycle tests exercising the full CRUD flow through
the CLI command layer (not direct store calls). Tests use in-memory
SQLite via `newTestStoreWithMigration`. Coverage for:

- Add + verify via list/get
- Edit partial update + verify
- Delete + verify excluded from list
- Restore + verify reappears in list
- Error cases: missing required fields, invalid FK, dependency blocking
  delete, duplicate names (vendor)

## Design Decisions

### Q1: Should `list`/`get` default to JSON or table output?

**Decision**: JSON default with `--table` override. This is a new command
surface designed for programmatic use. The existing `show` remains for
human-friendly table-default output (until deprecated).

### Q2: `show` deprecation timeline

**Decision**: Deprecate now. Add deprecation notice to `show --help`. Print
stderr warning on each invocation. Plan removal in next major version. The
new `<entity> list` commands subsume `show`'s functionality.

### Q3: Singular vs plural entity names

**Decision**: Singular. Natural with all verbs ("add vendor", "delete vendor").

### Q4: Should `delete` require `--force` for entities with dependencies?

**Decision**: No `--force`. Store layer returns dependency error. User must
delete children first. Consistent with TUI behavior.

### Q5: Should `edit` fetch-then-merge or pass raw JSON to store?

**Decision**: Fetch-merge-save. The store layer's `updateByIDWith` uses
`Select("*")` which updates all columns. Without fetch-merge, omitted
fields would be zeroed.

### Q6: Document blob upload

**Decision**: Include `--file` in initial scope. Without it, document `add`
is of limited use. The `--file` flag is optional: omitting it creates a
metadata-only record.
