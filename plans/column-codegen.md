<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Column Code Generation from Declarative Definitions

<!-- closes #741 -->

## Problem

Column iota constants (e.g. `projectColID`, `maintenanceColSeason`) and their
corresponding `columnSpecs()` slices are maintained separately by hand. When a
column is added or reordered, both must be updated in sync. The naming mapping
from spec titles to iota names is non-mechanical (e.g. "Last Serviced" ->
`ColLast`, "#" -> `ColLog`), making it error-prone.

## Design

### Single source of truth: `columnDef` slices

Introduce a `columnDef` struct that pairs a const-name suffix with its
`columnSpec`. Each entity defines a `var xxxColumnDefs = []columnDef{...}`
slice that is the **single source of truth** for both column ordering and
metadata.

```go
type columnDef struct {
    name string     // iota suffix: "ID" -> projectColID
    spec columnSpec
}

var projectColumnDefs = []columnDef{
    {"ID", idColumnSpec()},
    {"Type", columnSpec{Title: "Type", Min: 8, Max: 14, Flex: true}},
    // ...
}

func projectColumnSpecs() []columnSpec { return defsToSpecs(projectColumnDefs) }
```

### Code generator: `gencolumns`

A new generator at `internal/app/cmd/gencolumns/main.go` parses `coldefs.go`
via Go AST, finds all `var xxxColumnDefs` declarations, extracts the entity
prefix and const suffixes, and generates `columns_generated.go` containing:

- Typed `xxxCol int` type declarations
- `const (...)` iota blocks with all column constants

The generator follows the same pattern as the existing `genmeta` generator
in `internal/data/cmd/genmeta/`.

### Derived column sets

Detail-view column sets that remove a parent column (e.g.
`applianceMaintenanceColumnSpecs()` = maintenance specs minus "Appliance")
continue to use the `withoutColumn()` helper. They do not get their own
`columnDef` slices since they derive from base definitions.

Exception: `vendorJobsColumnDefs` has its own typed iota block because it
maps to different column positions than `serviceLogCol`.

## File changes

| File | Change |
|---|---|
| `internal/app/coldefs.go` | **New.** `columnDef` type, `defsToSpecs()`, all `xxxColumnDefs` vars, all `xxxColumnSpecs()` one-liners, `go:generate` directive |
| `internal/app/columns_generated.go` | **New (generated).** All `type xxxCol int` and `const (...)` iota blocks |
| `internal/app/cmd/gencolumns/main.go` | **New.** AST-based generator |
| `internal/app/tables.go` | **Remove** all hand-maintained iota blocks and `columnSpecs()` functions |

## What stays the same

- All switch statements in `forms.go` (inline edit dispatch)
- All handler `InlineEdit` methods in `handlers.go`
- All tests referencing column constants
- `withoutColumn()` helper and derived spec functions
- Row-building functions (`projectRows`, `vendorRows`, etc.)

## Verification

- `go generate ./internal/app/` produces `columns_generated.go`
- `go build ./...` succeeds
- `go test -shuffle=on ./...` passes
- Generator freshness test: re-run generator, diff output, fail if stale
