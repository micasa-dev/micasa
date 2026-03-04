<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Schema-Derived Table and Column Names

Issue: #603

## Problem

Table names (`"vendors"`, `"projects"`, ...) and column names (`ColVendorID =
"vendor_id"`) were scattered as bare strings or hand-maintained constants. The
GORM model structs already define the schema via `AutoMigrate`, but there was no
mechanism to derive the actual DB names from them. Two sources of truth existed
and could drift.

## Design

Single source of truth: **GORM model structs**. A `go:generate` tool parses
model structs via `gorm.io/gorm/schema.Parse` and emits typed `const` blocks.

### Generator (`internal/data/cmd/genmeta/main.go`)

The generator:

1. Iterates over all model structs (HouseProfile, Project, Vendor, ...)
2. Parses each with `schema.Parse` to get the GORM-derived table and column names
3. Emits `Table*` constants (e.g. `TableVendors = "vendors"`)
4. Emits `Col*` constants (e.g. `ColVendorID = "vendor_id"`, `ColChecksumSHA256 = "sha256"`)
5. Detects column name conflicts (same Go field name, different DB name) and fails

Output: `internal/data/meta_generated.go` (marked `// Code generated; DO NOT EDIT.`)

### Trigger

```go
// internal/data/meta.go
//go:generate go run ./cmd/genmeta/
package data
```

### Usage

```go
// Table names
data.TableVendors    // "vendors"
data.TableDocuments  // "documents"

// Column names (including custom gorm:"column:" mappings)
data.ColVendorID        // "vendor_id"
data.ColChecksumSHA256  // "sha256"
data.ColExtractData     // "ocr_data"
```

### Migration

1. Created `internal/data/cmd/genmeta/main.go` generator
2. Added `//go:generate` directive in `internal/data/meta.go`
3. Generated `meta_generated.go` with all Table* and Col* constants
4. Deleted hand-written Col* constant block from `models.go`
5. Replaced bare table name strings across `internal/extract/`, `internal/data/`,
   `internal/app/`, and test files with `Table*` constants
6. Replaced bare column name strings with `Col*` constants where used as
   map keys or function arguments
7. Converted raw SQL in `columnHints` to use `fmt.Sprintf` with constants

### Remaining bare strings

- LLM prompt templates (`operationExtractionRules`, `operationExtractionPreamble`)
  contain table names inside `const` string blocks. These are documentation for
  the LLM, not Go identifiers.
- JSON test data (raw JSON strings parsed by tests) contains table names as JSON
  values. These simulate LLM output and must match what the LLM produces.
- `plural()` method returns display labels (e.g. "maintenance items") that happen
  to resemble table names but are UI strings with different formatting.

## Non-goals

- Changing GORM model definitions or struct tags.
- Replacing table/column names inside raw SQL test inputs or LLM prompt templates.
