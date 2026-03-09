<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Seasonal Tagging for Maintenance Items

GitHub issue: #686

## Motivation

Homeowners think in seasons, not intervals. "Spring: clean gutters, service
AC. Fall: winterize sprinklers, inspect roof." Today maintenance items have
`IntervalMonths` or `DueDate` but no seasonal tagging. Adding a `Season`
field lets users tag items by season and filter the maintenance tab to answer
"what should I do this spring?"

## Design

### Data model

Add a `Season` string field to `MaintenanceItem` in `internal/data/models.go`.
Empty string means "no season" (item is year-round or untagged). Constants:

```go
const (
    SeasonSpring = "spring"
    SeasonSummer = "summer"
    SeasonFall   = "fall"
    SeasonWinter = "winter"
)
```

This follows the existing pattern for `IncidentStatus` / `IncidentSeverity`:
plain string constants, stored as-is in SQLite. GORM AutoMigrate handles
adding the column.

### Table column

Add `maintenanceColSeason` to the iota block between `maintenanceColCategory`
and `maintenanceColAppliance`. Column spec: `{Title: "Season", Min: 6, Max: 8,
Kind: cellStatus}`. Using `cellStatus` gives it the colored-badge rendering and
makes it a pin-filterable column (like Incident status/severity).

Also update `applianceMaintenanceRows` (the detail sub-table that drops the
Appliance column) to include the Season cell.

### Form

- Add `Season string` to `maintenanceFormData`.
- Add a `huh.Select[string]` field in both `startMaintenanceForm` (new) and
  `openMaintenanceForm` (edit) with options: (none), spring, summer, fall,
  winter. Default: (none).
- Wire through `maintenanceFormValues` (edit pre-fill) and
  `parseMaintenanceFormData` (submit).

### Inline edit

Add `maintenanceColSeason` case to `inlineEditMaintenance` using
`openInlineEdit` with a `huh.Select[string]`.

### Filter (pin system)

Add `setFixedValues(specs, "Season", ...)` in
`maintenanceHandler.SyncFixedValues` so the pin-filter dropdown knows all
possible season values. The existing filter machinery handles the rest.

### Dashboard

Add a "Seasonal" section to `dashboardData` showing maintenance items tagged
with the current calendar season. This gives users the "what should I do this
spring?" view the issue asks for. Determining current season from the date:

- Spring: Mar-May
- Summer: Jun-Aug
- Fall: Sep-Nov
- Winter: Dec-Feb

(Northern hemisphere default; good enough for v1.)

### Tests

- Row-building test: verify Season cell appears at the correct index
- Form round-trip test: create item with season via `openAddForm` + keypress,
  confirm it persists
- Inline edit test: change season via inline edit
- Dashboard test: verify seasonal items appear in the dashboard section

## Files to modify

| File | Change |
|------|--------|
| `internal/data/models.go` | Add `Season` field + constants |
| `internal/app/forms.go` | Form data, builders, submit, inline edit |
| `internal/app/tables.go` | Column enum, specs, row builders (both views) |
| `internal/app/handlers.go` | `SyncFixedValues` for Season |
| `internal/app/dashboard.go` | New "Seasonal" section |
| `internal/data/dashboard.go` | Query for items by season |
| `internal/app/*_test.go` | Tests per above |

## Non-goals

- Multi-season tagging (one item in both spring and fall). Can revisit if
  users request it; for now a single season keeps the model simple.
- Hemisphere-aware season detection. Northern hemisphere is the default.
