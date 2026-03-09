<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Struct Tag Defaults

## Problem

Default values for entity creation and config initialization are scattered
across form initialization code (`internal/app/forms.go`) and a manual
`defaults()` constructor (`internal/config/config.go`). This has two issues:

1. **Duplication**: defaults live only in the UI/init layer; programmatic
   creation (extraction pipeline, seed data, tests) must duplicate them.
2. **Discoverability**: reading the struct definition doesn't reveal what
   values a new instance should have.

## Design

### `default` struct tag + `ApplyDefaults`

A `default:"value"` tag on struct fields declares the default value. The
`data.ApplyDefaults(ptr)` function walks the struct via reflection and sets
zero-valued fields to the tag value. Nested structs without a `default` tag
are recursed into automatically.

Supported types: string, int, int64, uint, uint64, float64, time.Time
(sentinel `"now"`), string dates (sentinel `"today"`), and named types with
int/uint underlying kind.

### `StructDefault[T](field)` generic accessor

Returns the raw tag string for a named field, useful for deriving fallback
values without constructing an instance (e.g. `RestoreIncident` status
fallback).

## Scope

### Data models (`internal/data/models.go`)

| Model    | Field    | Tag               |
|----------|----------|-------------------|
| Project  | Status   | `default:"planned"` |
| Incident | Status   | `default:"open"`    |
| Incident | Severity | `default:"soon"`    |
| Incident | DateNoticed | `default:"now"`  |

### Form data structs (`internal/app/forms.go`)

| Struct              | Field       | Tag                |
|---------------------|-------------|--------------------|
| projectFormData     | Status      | `default:"planned"` |
| incidentFormData    | Status      | `default:"open"`    |
| incidentFormData    | Severity    | `default:"soon"`    |
| incidentFormData    | DateNoticed | `default:"today"`   |
| serviceLogFormData  | ServicedAt  | `default:"today"`   |

### Config structs (`internal/config/config.go`)

| Struct    | Field       | Tag                                  |
|-----------|-------------|--------------------------------------|
| LLM       | BaseURL     | `default:"http://localhost:11434"`   |
| LLM       | Model       | `default:"qwen3"`                    |
| LLM       | Timeout     | `default:"5m"`                       |
| Documents | MaxFileSize | `default:"52428800"` (50 MiB)        |

Replaces the hand-written `defaults()` function and `data.MaxDocumentSize`
constant -- the tag is the single source of truth for the 50 MiB default.

### Store (`internal/data/store.go`)

- `RestoreIncident` status fallback uses `StructDefault[Incident]("Status")`
  instead of a hardcoded `IncidentStatusOpen` constant.
- `Store.Open()` no longer bakes in a document size default; callers set it
  from config via `SetMaxDocumentSize`.
