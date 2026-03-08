<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Derive ExtractionTableDefs columns from genmeta

Issue: #666

## Problem

`ExtractionTableDefs` in `internal/extract/sqlcontext.go` manually lists column
names and types for each extractable table. This duplicates information already
available from the GORM model structs that `genmeta` processes. Adding or
renaming a model field requires updating both `models.go` and
`ExtractionTableDefs`, and they can drift silently.

## Design

### 1. Extend genmeta to emit per-table column metadata

Add a new type and map to `meta_generated.go`:

```go
type MetaColumn struct {
    Name     string
    JSONType string // "string" or "integer"
}

var TableExtractColumns = map[string][]MetaColumn{...}
```

**Column inclusion rules** (applied in genmeta AST walk):
- Exclude `ID`, `CreatedAt`, `UpdatedAt`, `DeletedAt` fields (infrastructure)
- Exclude `[]byte` fields (binary data)
- Exclude `gorm.DeletedAt` type (already caught by name, belt-and-suspenders)
- Exclude associations (already excluded by `isAssociation`)
- Exclude `gorm:"-"` tagged fields (already excluded)

**JSON Schema type mapping** (from AST type expressions):
- `string` -> `"string"`
- `int`, `int64`, `uint`, `float64` and `*` variants -> `"integer"`
- `time.Time`, `*time.Time` -> `"string"` (dates are strings for the LLM)

### 2. ~~Add table-level Omit to TableDef~~ Use `extract:"-"` struct tags

~~Initially implemented as a hand-maintained `Omit` list on `TableDef`.~~
Replaced in #678: fields tagged `extract:"-"` in `models.go` are skipped by
`genmeta` at generation time, so they never appear in `TableExtractColumns`.
This eliminates `TableDef.Omit` entirely -- the visibility policy lives next
to the field definition. Action-level `ActionDef.Omit` remains for per-action
exclusions (e.g. `file_name` excluded from document updates).

### 3. Refactor ExtractionTableDefs

Replace manual column lists with a helper that reads generated metadata:

```go
func columnsFromMeta(table string) []ColumnDef {
    metas := data.TableExtractColumns[table]
    cols := make([]ColumnDef, len(metas))
    for i, m := range metas {
        cols[i] = ColumnDef{Name: m.Name, Type: ColType(m.JSONType)}
    }
    return cols
}
```

Plus helpers for enum overrides and synthetic columns:

```go
func withEnum(cols []ColumnDef, name string, values []any) []ColumnDef
func withSynthetic(cols []ColumnDef, extra ...ColumnDef) []ColumnDef
```

Each table definition becomes generated columns + policy annotations. Example:

```go
{
    Table:   data.TableVendors,
    Columns: columnsFromMeta(data.TableVendors),
    Actions: []ActionDef{
        {Action: ActionCreate, Required: []string{"name"}},
        {Action: ActionUpdate, Required: []string{"id"}, Extra: []ColumnDef{
            {Name: "id", Type: ColTypeInteger},
        }},
    },
},
```

### 4. Table-by-table exclusion plan (behavioral parity)

Fields not yet exposed to the LLM are tagged `extract:"-"` on the model
struct. Removing the tag makes the column automatically appear in the LLM
schema once the corresponding commit function handles it.

| Table | Fields tagged `extract:"-"` |
|---|---|
| vendors | (none) |
| appliances | PurchaseDate, WarrantyExpiry |
| projects | StartDate, EndDate, ActualCents |
| quotes | OtherCents, ReceivedDate |
| maintenance_items | LastServicedAt, DueDate, ManualURL, ManualText |
| incidents | PreviousStatus, DateResolved |
| service_log_entries | (none) |
| documents | MIMEType, SizeBytes, ChecksumSHA256, ExtractedText |

### 5. Consistency test

Add a test that verifies every non-omitted, non-Extra column in each
`ExtractionTableDefs` entry exists in `TableExtractColumns[table]` (or is
marked synthetic). This catches future drift between models and extraction
config.

## Files changed

- `internal/data/cmd/genmeta/main.go` - emit `MetaColumn` type + `TableExtractColumns` map
- `internal/data/meta_generated.go` - regenerated output
- `internal/extract/sqlcontext.go` - refactored `ExtractionTableDefs`, new helpers
- `internal/extract/sqlcontext_test.go` - consistency test

## Non-goals

- Changing which columns the LLM can write (behavioral parity)
- Updating commit functions to handle newly-available columns (future work)
- Generating Actions/Required/Enum/Omit annotations (these stay manual)
