<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# CLI `show` Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `micasa show <entity>` and `micasa query` CLI commands for dumping data as tabwriter text or JSON.

**Architecture:** New Cobra subcommands in `cmd/micasa/` that open the Store read-only and render entity data via a generic `showTable[T]` helper. Each entity defines column metadata for text mode and a `toMap` function for JSON mode. A new `ListAllServiceLogEntries` Store method fills the gap for service logs.

**Tech Stack:** Go, Cobra, `text/tabwriter`, `encoding/json`, existing GORM Store layer

**Spec:** `plans/cli-show-command.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `cmd/micasa/show.go` | Create | Cobra `show` command, per-entity subcommands, `showTable[T]` generic helper, column defs per entity, formatting helpers |
| `cmd/micasa/show_test.go` | Create | Tests for all `show` subcommands (text + JSON, empty DB, `--deleted`) |
| `cmd/micasa/query.go` | Create | Cobra `query` command wrapping `ReadOnlyQuery` |
| `cmd/micasa/query_test.go` | Create | Tests for `query` (valid SELECT, rejected mutations, `--json`) |
| `cmd/micasa/main.go` | Modify | Register `newShowCmd()` and `newQueryCmd()` on root |
| `internal/data/store.go` | Modify | Add `ListAllServiceLogEntries(includeDeleted bool)` |

---

### Task 1: Add `ListAllServiceLogEntries` Store method

**Files:**
- Modify: `internal/data/store.go` (near existing `ListServiceLog` at line ~962)

- [ ] **Step 1: Write the test**

Add to an existing Store test file. The test creates a maintenance item, two service log entries, and verifies `ListAllServiceLogEntries` returns both.

```go
func TestListAllServiceLogEntries(t *testing.T) {
	store := newTestStore(t)

	// SeedDefaults creates maintenance categories; look one up.
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	item := data.MaintenanceItem{Name: "Filter", CategoryID: cats[0].ID, Season: "spring"}
	require.NoError(t, store.CreateMaintenance(&item))

	item2 := data.MaintenanceItem{Name: "Coils", CategoryID: cats[0].ID, Season: "fall"}
	require.NoError(t, store.CreateMaintenance(&item2))

	entry1 := data.ServiceLogEntry{MaintenanceItemID: item.ID, ServicedAt: time.Now()}
	require.NoError(t, store.CreateServiceLog(&entry1, data.Vendor{}))

	entry2 := data.ServiceLogEntry{MaintenanceItemID: item2.ID, ServicedAt: time.Now()}
	require.NoError(t, store.CreateServiceLog(&entry2, data.Vendor{}))

	entries, err := store.ListAllServiceLogEntries(false)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestListAllServiceLogEntries ./internal/data/`
Expected: compile error — `ListAllServiceLogEntries` undefined.

- [ ] **Step 3: Implement `ListAllServiceLogEntries`**

Add to `internal/data/store.go` near `ListServiceLog`:

```go
// ListAllServiceLogEntries returns all service log entries across all
// maintenance items, with vendor and maintenance item preloaded.
func (s *Store) ListAllServiceLogEntries(includeDeleted bool) ([]ServiceLogEntry, error) {
	return listQuery[ServiceLogEntry](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Preload("MaintenanceItem", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
			Preload("Vendor", unscopedPreload).
			Order(ColServicedAt + " desc, " + ColID + " desc")
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestListAllServiceLogEntries ./internal/data/`
Expected: PASS

- [ ] **Step 5: Commit**

```
feat(data): add ListAllServiceLogEntries for CLI show command
```

---

### Task 2: Create the generic show infrastructure and formatting helpers

**Files:**
- Create: `cmd/micasa/show.go`

- [ ] **Step 1: Write the formatting helpers and generic `showTable` function**

```go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

// showCol defines a single column in text output for entity type T.
type showCol[T any] struct {
	header string
	text   func(T) string
}

// showEntity renders a slice of entities as either tabwriter text or JSON.
// sectionName is used for the "=== NAME (N) ===" header in text mode.
// toMap converts each entity to a map for JSON output.
func showEntity[T any](
	w io.Writer,
	items []T,
	sectionName string,
	cols []showCol[T],
	toMap func(T) map[string]any,
	asJSON bool,
) error {
	if asJSON {
		return writeJSON(w, items, toMap)
	}
	return writeTable(w, items, sectionName, cols)
}

func writeTable[T any](
	w io.Writer,
	items []T,
	sectionName string,
	cols []showCol[T],
) error {
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "=== %s (%d) ===\n", sectionName, len(items)); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.header
	}
	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, item := range items {
		vals := make([]string, len(cols))
		for i, c := range cols {
			vals[i] = c.text(item)
		}
		if _, err := fmt.Fprintln(tw, strings.Join(vals, "\t")); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}

func writeJSON[T any](w io.Writer, items []T, toMap func(T) map[string]any) error {
	out := make([]map[string]any, len(items))
	for i, item := range items {
		out[i] = toMap(item)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func fmtMoney(cents *int64) string {
	if cents == nil {
		return "-"
	}
	return fmt.Sprintf("$%.2f", float64(*cents)/100)
}

func fmtDate(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

func fmtDateVal(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

func fmtStr(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func fmtInt(n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

// fmtIntAlways renders even zero values. Use for fields where 0 is
// meaningful (e.g. IntervalMonths) rather than "not set".
func fmtIntAlways(n int) string {
	return fmt.Sprintf("%d", n)
}

func fmtFloat(f float64) string {
	if f == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", f)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/micasa/`
Expected: compiles (no commands registered yet, just helpers)

- [ ] **Step 3: Commit**

```
feat(cli): add show command infrastructure and formatting helpers
```

---

### Task 3: Implement `show house` subcommand

**Files:**
- Modify: `cmd/micasa/show.go` (add house rendering)
- Create: `cmd/micasa/show_test.go` (first test)
- Modify: `cmd/micasa/main.go` (register `newShowCmd()`)

- [ ] **Step 1: Write the test**

```go
func TestShowHouseText(t *testing.T) {
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{
		Nickname:     "Test House",
		AddressLine1: "123 Main St",
		City:         "Springfield",
		State:        "IL",
		PostalCode:   "62701",
		YearBuilt:    1985,
		SquareFeet:   2400,
		Bedrooms:     3,
		Bathrooms:    2.5,
	}))

	var buf bytes.Buffer
	err := runShow(&buf, store, "house", false, false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "=== HOUSE ===")
	assert.Contains(t, out, "Nickname:")
	assert.Contains(t, out, "Test House")
	assert.Contains(t, out, "123 Main St")
	assert.Contains(t, out, "Springfield, IL 62701")
}

func TestShowHouseJSON(t *testing.T) {
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{
		Nickname: "Test House",
	}))

	var buf bytes.Buffer
	err := runShow(&buf, store, "house", true, false)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "Test House", result["nickname"])
}

func TestShowHouseEmpty(t *testing.T) {
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	err := runShow(&buf, store, "house", false, false)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}
```

The test helper `newTestStoreWithMigration` opens a temp DB, runs AutoMigrate + SeedDefaults. The `runShow` function is the testable core: `func runShow(w io.Writer, store *data.Store, entity string, asJSON, includeDeleted bool) error`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestShowHouse ./cmd/micasa/`
Expected: compile error — `runShow` undefined

- [ ] **Step 3: Implement `newShowCmd`, `newShowHouseCmd`, `runShow` dispatch, and house rendering**

In `cmd/micasa/show.go`, add:

```go
func newShowCmd() *cobra.Command {
	var jsonFlag bool
	var deletedFlag bool

	cmd := &cobra.Command{
		Use:   "show <entity>",
		Short: "Display data as text or JSON",
		Long: `Print entity data to stdout. Entities: house, projects, project-types,
quotes, vendors, maintenance, maintenance-categories, service-log,
appliances, incidents, documents, all.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output as JSON")
	cmd.PersistentFlags().BoolVar(&deletedFlag, "deleted", false, "Include soft-deleted rows")

	cmd.AddCommand(newShowHouseCmd(&jsonFlag, &deletedFlag))
	// ... more subcommands added in later tasks

	return cmd
}

func newShowHouseCmd(jsonFlag, deletedFlag *bool) *cobra.Command {
	return &cobra.Command{
		Use:           "house [database-path]",
		Short:         "Show house profile",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			return runShow(cmd.OutOrStdout(), store, "house", *jsonFlag, *deletedFlag)
		},
	}
}

func runShow(w io.Writer, store *data.Store, entity string, asJSON, includeDeleted bool) error {
	switch entity {
	case "house":
		return showHouse(w, store, asJSON)
	default:
		return fmt.Errorf("unknown entity %q; valid entities: %s",
			entity, strings.Join(validEntities, ", "))
	}
}
```

House rendering (key-value text or JSON object):

```go
func showHouse(w io.Writer, store *data.Store, asJSON bool) error {
	h, err := store.HouseProfile()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // silence is success
		}
		return fmt.Errorf("load house profile: %w", err)
	}

	if asJSON {
		return showHouseJSON(w, h)
	}
	return showHouseText(w, h)
}

func showHouseText(w io.Writer, h data.HouseProfile) error {
	if _, err := fmt.Fprintln(w, "=== HOUSE ==="); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	kv := func(label, value string) {
		if value != "" && value != "-" {
			fmt.Fprintf(tw, "%s:\t%s\n", label, value)
		}
	}

	kv("Nickname", h.Nickname)
	kv("Address", formatAddress(h))
	kv("Year Built", fmtInt(h.YearBuilt))
	kv("Square Feet", fmtInt(h.SquareFeet))
	kv("Lot Size", fmtInt(h.LotSquareFeet))
	kv("Bedrooms", fmtInt(h.Bedrooms))
	kv("Bathrooms", fmtFloat(h.Bathrooms))
	kv("Foundation", h.FoundationType)
	kv("Wiring", h.WiringType)
	kv("Roof", h.RoofType)
	kv("Exterior", h.ExteriorType)
	kv("Heating", h.HeatingType)
	kv("Cooling", h.CoolingType)
	kv("Water Source", h.WaterSource)
	kv("Sewer", h.SewerType)
	kv("Parking", h.ParkingType)
	kv("Basement", h.BasementType)
	kv("Insurance Carrier", h.InsuranceCarrier)
	kv("Insurance Policy", h.InsurancePolicy)
	kv("Insurance Renewal", fmtDate(h.InsuranceRenewal))
	kv("Property Tax", fmtMoney(h.PropertyTaxCents))
	kv("HOA", h.HOAName)
	kv("HOA Fee", fmtMoney(h.HOAFeeCents))

	return tw.Flush()
}

func formatAddress(h data.HouseProfile) string {
	var lines []string
	if h.AddressLine1 != "" {
		lines = append(lines, h.AddressLine1)
	}
	if h.AddressLine2 != "" {
		lines = append(lines, h.AddressLine2)
	}
	var cityState []string
	if h.City != "" {
		cityState = append(cityState, h.City)
	}
	if h.State != "" {
		cityState = append(cityState, h.State)
	}
	csStr := strings.Join(cityState, ", ")
	if csStr != "" && h.PostalCode != "" {
		csStr += " " + h.PostalCode
	} else if h.PostalCode != "" {
		csStr = h.PostalCode
	}
	if csStr != "" {
		lines = append(lines, csStr)
	}
	// Join with newline + padding so multi-line addresses align
	// under the tabwriter's value column.
	return strings.Join(lines, "\n                   ")
}

// validEntities lists entity names for error messages.
var validEntities = []string{
	"house", "projects", "project-types", "quotes", "vendors",
	"maintenance", "maintenance-categories", "service-log",
	"appliances", "incidents", "documents", "all",
}

func showHouseJSON(w io.Writer, h data.HouseProfile) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(h)
}
```

Register in `cmd/micasa/main.go`:

```go
root.AddCommand(
	newDemoCmd(),
	newBackupCmd(),
	newConfigCmd(),
	newCompletionCmd(root),
	newProCmd(),
	newShowCmd(),   // new
	newQueryCmd(),  // added in task 8
)
```

Also create the test helper in `cmd/micasa/show_test.go`:

```go
func newTestStoreWithMigration(t *testing.T) *data.Store {
	t.Helper()
	store, err := data.Open(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())
	t.Cleanup(func() { _ = store.Close() })
	return store
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestShowHouse ./cmd/micasa/`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```
feat(cli): add micasa show house command
```

---

### Task 4: Implement collection entity subcommands (projects, vendors, appliances, incidents)

**Files:**
- Modify: `cmd/micasa/show.go` (add column defs, toMap funcs, subcommands)
- Modify: `cmd/micasa/show_test.go` (add tests)

These 4 entities are the simplest collections — no complex FK joins needed beyond what `Preload` already does in the existing `List*` methods.

- [ ] **Step 1: Write tests for projects (text + JSON)**

```go
func TestShowProjectsText(t *testing.T) {
	store := newTestStoreWithMigration(t)
	// SeedDefaults creates project types; create a project.
	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, types)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Fix roof",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))

	var buf bytes.Buffer
	err = runShow(&buf, store, "projects", false, false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "PROJECTS")
	assert.Contains(t, out, "Fix roof")
	assert.Contains(t, out, "planned")
}

func TestShowProjectsJSON(t *testing.T) {
	store := newTestStoreWithMigration(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Fix roof",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))

	var buf bytes.Buffer
	err = runShow(&buf, store, "projects", true, false)
	require.NoError(t, err)

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Fix roof", result[0]["title"])
	assert.Equal(t, "planned", result[0]["status"])
	// JSON includes resolved project type name
	assert.NotEmpty(t, result[0]["project_type"])
}

func TestShowEmptyCollection(t *testing.T) {
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	err := runShow(&buf, store, "projects", false, false)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}
```

Similarly write tests for `vendors`, `appliances`, `incidents` following the same pattern.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestShow(Projects|Vendors|Appliances|Incidents|Empty)' ./cmd/micasa/`
Expected: fail — entities not handled in `runShow`

- [ ] **Step 3: Implement column defs and subcommands**

For each entity, define column defs and a `toMap` function. Wire them into `runShow` dispatch and add Cobra subcommands.

Example for projects:

```go
var projectShowCols = []showCol[data.Project]{
	{header: "TITLE", text: func(p data.Project) string { return fmtStr(p.Title) }},
	{header: "TYPE", text: func(p data.Project) string { return fmtStr(p.ProjectType.Name) }},
	{header: "STATUS", text: func(p data.Project) string { return fmtStr(p.Status) }},
	{header: "START", text: func(p data.Project) string { return fmtDate(p.StartDate) }},
	{header: "END", text: func(p data.Project) string { return fmtDate(p.EndDate) }},
	{header: "BUDGET", text: func(p data.Project) string { return fmtMoney(p.BudgetCents) }},
	{header: "ACTUAL", text: func(p data.Project) string { return fmtMoney(p.ActualCents) }},
	{header: "DESCRIPTION", text: func(p data.Project) string { return fmtStr(p.Description) }},
}

func projectToMap(p data.Project) map[string]any {
	m := map[string]any{
		"id":           p.ID,
		"title":        p.Title,
		"project_type": p.ProjectType.Name,
		"status":       p.Status,
		"description":  p.Description,
		"start_date":   p.StartDate,
		"end_date":     p.EndDate,
		"budget_cents": p.BudgetCents,
		"actual_cents": p.ActualCents,
	}
	return m
}
```

Follow the same pattern for vendors (name, contact, email, phone, website), appliances (name, brand, model, serial, location, purchase date, warranty, cost), incidents (title, status, severity, date noticed, location, cost, appliance name, vendor name).

In `runShow`, add cases:

```go
case "projects":
	items, err := store.ListProjects(includeDeleted)
	if err != nil { return err }
	return showEntity(w, items, "PROJECTS", projectShowCols, projectToMap, asJSON)
case "vendors":
	// similar
case "appliances":
	// similar
case "incidents":
	// similar
```

Add Cobra subcommands using same factory pattern as `newShowHouseCmd`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run 'TestShow(Projects|Vendors|Appliances|Incidents|Empty)' ./cmd/micasa/`
Expected: PASS

- [ ] **Step 5: Commit**

```
feat(cli): add show projects/vendors/appliances/incidents subcommands
```

---

### Task 5: Implement remaining collection subcommands (quotes, maintenance, service-log, documents, lookup tables)

**Files:**
- Modify: `cmd/micasa/show.go`
- Modify: `cmd/micasa/show_test.go`

These entities have FK joins that need resolved names.

- [ ] **Step 1: Write tests for quotes, maintenance, service-log, documents, project-types, maintenance-categories**

Quotes test creates a project + vendor + quote, asserts text output includes project title and vendor name.

Maintenance test creates a category + appliance + maintenance item, asserts category name and appliance name appear.

Service-log test creates full chain (category + item + service log entry), asserts maintenance item name appears.

Documents test creates a document, asserts metadata columns appear.

Lookup table tests (project-types, maintenance-categories) assert the name appears.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestShow(Quotes|Maintenance|ServiceLog|Documents|ProjectTypes|MaintenanceCategories)' ./cmd/micasa/`
Expected: fail

- [ ] **Step 3: Implement column defs, toMap functions, and subcommands for all 6 entities**

Quotes: columns = PROJECT, VENDOR, TOTAL, LABOR, MATERIALS, RECEIVED, NOTES.
`toMap` includes resolved `project` (title) and `vendor` (name).

Maintenance: columns = NAME, CATEGORY, APPLIANCE, SEASON, LAST SERVICED, INTERVAL, DUE, COST.
`toMap` includes resolved `category` and `appliance` names.

Service-log: columns = ITEM, VENDOR, SERVICED, COST, NOTES.
`toMap` includes resolved `maintenance_item` and `vendor` names.

Documents: columns = TITLE, FILE, ENTITY, MIME, SIZE, NOTES.
No FK joins needed (entity_kind is a string).

Project-types: columns = NAME.
Maintenance-categories: columns = NAME.

Wire all into `runShow` dispatch. Add Cobra subcommands.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run 'TestShow(Quotes|Maintenance|ServiceLog|Documents|ProjectTypes|MaintenanceCategories)' ./cmd/micasa/`
Expected: PASS

- [ ] **Step 5: Commit**

```
feat(cli): add show quotes/maintenance/service-log/documents/lookup subcommands
```

---

### Task 6: Implement `--deleted` flag support

**Files:**
- Modify: `cmd/micasa/show.go`
- Modify: `cmd/micasa/show_test.go`

- [ ] **Step 1: Write test for `--deleted` flag**

```go
func TestShowDeletedProjects(t *testing.T) {
	store := newTestStoreWithMigration(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Active",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))

	p2 := &data.Project{
		Title:         "Deleted",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusAbandoned,
	}
	require.NoError(t, store.CreateProject(p2))
	require.NoError(t, store.DeleteProject(p2.ID))

	// Without --deleted: only active
	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "projects", false, false))
	assert.Contains(t, buf.String(), "Active")
	assert.NotContains(t, buf.String(), "Deleted")

	// With --deleted: both, plus DELETED column
	buf.Reset()
	require.NoError(t, runShow(&buf, store, "projects", false, true))
	assert.Contains(t, buf.String(), "Active")
	assert.Contains(t, buf.String(), "Deleted")
	assert.Contains(t, buf.String(), "DELETED")
}

func TestShowDeletedJSON(t *testing.T) {
	store := newTestStoreWithMigration(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	p := &data.Project{
		Title:         "Gone",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))
	require.NoError(t, store.DeleteProject(p.ID))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "projects", true, true))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.NotNil(t, result[0]["deleted_at"])
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestShowDeleted ./cmd/micasa/`
Expected: fail — `--deleted` not passing `includeDeleted` through, DELETED column not added

- [ ] **Step 3: Implement `--deleted` support**

Two changes needed:

1. In `showEntity`, when `includeDeleted` is true for text mode, append a DELETED column. This requires accessing the `DeletedAt` field. Since `gorm.DeletedAt` is a wrapper around `*time.Time`, add a `deletedAt` accessor interface or pass a separate function.

Best approach: add a `deletedCol` parameter to `showEntity` that optionally appends the DELETED column. The column extractor uses a generic `deletedAtFunc` per entity type.

2. In `toMap`, when `includeDeleted` is true and the entity has a non-nil `DeletedAt`, include `"deleted_at"` in the map.

Extend `showEntity` signature:
```go
func showEntity[T any](
	w io.Writer,
	items []T,
	sectionName string,
	cols []showCol[T],
	toMap func(T, bool) map[string]any,
	asJSON bool,
	includeDeleted bool,
) error
```

The `toMap` now receives `includeDeleted` so it can conditionally add `deleted_at`.

For text mode, add a conditional DELETED column using `gorm.DeletedAt`:
```go
if includeDeleted {
	cols = append(cols, showCol[T]{
		header: "DELETED",
		text:   deletedAtFn,
	})
}
```

Each entity's `deletedAtFn` extracts the `DeletedAt` field. Use an interface:
```go
type softDeletable interface {
	GetDeletedAt() gorm.DeletedAt
}
```

Or simpler: pass the deletedAt extractor as a function to `showEntity`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestShowDeleted ./cmd/micasa/`
Expected: PASS

- [ ] **Step 5: Commit**

```
feat(cli): support --deleted flag in show subcommands
```

---

### Task 7: Implement `show all` subcommand

**Files:**
- Modify: `cmd/micasa/show.go`
- Modify: `cmd/micasa/show_test.go`

- [ ] **Step 1: Write test**

```go
func TestShowAllText(t *testing.T) {
	store := newTestStoreWithMigration(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Test Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Test Vendor"}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "all", false, false))

	out := buf.String()
	assert.Contains(t, out, "PROJECTS")
	assert.Contains(t, out, "Test Project")
	assert.Contains(t, out, "VENDORS")
	assert.Contains(t, out, "Test Vendor")
}

func TestShowAllJSON(t *testing.T) {
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Test Vendor"}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "all", true, false))

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Contains(t, result, "vendors")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestShowAll ./cmd/micasa/`
Expected: fail — `all` not handled

- [ ] **Step 3: Implement `show all`**

For text mode: call each entity's show function in sequence (house first, then collections). Each writes to the same writer with section headers.

For JSON mode: build a `map[string]any` with each entity's key, then encode once.

```go
func showAll(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	if asJSON {
		return showAllJSON(w, store, includeDeleted)
	}
	return showAllText(w, store, includeDeleted)
}

func showAllText(w io.Writer, store *data.Store, includeDeleted bool) error {
	// House (special: key-value format)
	if err := showHouse(w, store, false); err != nil {
		return err
	}
	// Each collection entity
	for _, fn := range allCollectionShows(store, includeDeleted) {
		if err := fn(w, false); err != nil {
			return err
		}
	}
	return nil
}

func showAllJSON(w io.Writer, store *data.Store, includeDeleted bool) error {
	result := make(map[string]any)
	// house (singleton -- marshal struct directly)
	h, err := store.HouseProfile()
	if err == nil {
		result["house"] = h
	}
	// Each collection entity: use toMap functions so FK names
	// are resolved and deleted_at is included when includeDeleted.
	// Example for projects:
	//   projects, _ := store.ListProjects(includeDeleted)
	//   result["projects"] = mapSlice(projects, func(p data.Project) map[string]any {
	//       return projectToMap(p, includeDeleted)
	//   })
	// Repeat for all collection entities.
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
```

Add the `all` Cobra subcommand and wire into `runShow`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestShowAll ./cmd/micasa/`
Expected: PASS

- [ ] **Step 5: Commit**

```
feat(cli): add show all subcommand
```

---

### Task 8: Implement `micasa query` command

**Files:**
- Create: `cmd/micasa/query.go`
- Create: `cmd/micasa/query_test.go`
- Modify: `cmd/micasa/main.go` (register `newQueryCmd()`)

- [ ] **Step 1: Write tests**

```go
func TestQueryText(t *testing.T) {
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Acme"}))

	var buf bytes.Buffer
	err := runQuery(&buf, store, "SELECT name FROM vendors", false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "name")
	assert.Contains(t, out, "Acme")
}

func TestQueryJSON(t *testing.T) {
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Acme"}))

	var buf bytes.Buffer
	err := runQuery(&buf, store, "SELECT name FROM vendors", true)
	require.NoError(t, err)

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Acme", result[0]["name"])
}

func TestQueryRejectsMutation(t *testing.T) {
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	err := runQuery(&buf, store, "DELETE FROM vendors", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disallowed")
}

func TestQueryEmpty(t *testing.T) {
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	err := runQuery(&buf, store, "SELECT name FROM vendors", false)
	require.NoError(t, err)
	// Header row only, no data rows
	assert.Contains(t, buf.String(), "name")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestQuery ./cmd/micasa/`
Expected: compile error — `runQuery` undefined

- [ ] **Step 3: Implement `cmd/micasa/query.go`**

```go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
)

func newQueryCmd() *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "query <sql> [database-path]",
		Short: "Run a read-only SQL query",
		Long: `Execute a validated SELECT query against the database.
Only SELECT/WITH statements are allowed. Results are capped at 200 rows
with a 10-second timeout.`,
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var dbPath string
			if len(args) > 1 {
				dbPath = args[1]
			}
			store, err := openExisting(dbPath)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			return runQuery(cmd.OutOrStdout(), store, args[0], jsonFlag)
		},
	}

	cmd.Flags().BoolVar(&jsonFlag, "json", false, "Output as JSON")
	return cmd
}

func runQuery(w io.Writer, store *data.Store, sql string, asJSON bool) error {
	columns, rows, err := store.ReadOnlyQuery(sql)
	if err != nil {
		return err
	}

	if asJSON {
		return writeQueryJSON(w, columns, rows)
	}
	return writeQueryText(w, columns, rows)
}

func writeQueryText(w io.Writer, columns []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(columns, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func writeQueryJSON(w io.Writer, columns []string, rows [][]string) error {
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		obj := make(map[string]any, len(columns))
		for j, col := range columns {
			obj[col] = row[j]
		}
		out[i] = obj
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
```

Register `newQueryCmd()` in `main.go`'s `root.AddCommand(...)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestQuery ./cmd/micasa/`
Expected: PASS

- [ ] **Step 5: Commit**

```
feat(cli): add micasa query command for ad-hoc SQL
```

---

### Task 9: Integration test and final polish

**Files:**
- Modify: `cmd/micasa/show_test.go`

- [ ] **Step 1: Write an integration test that exercises the full CLI invocation**

Test that the Cobra command tree works end-to-end with `cmd.Execute()`:

```go
func TestShowCLIIntegration(t *testing.T) {
	// Create a temp DB file with demo data.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())
	require.NoError(t, store.SeedDemoData())
	require.NoError(t, store.Close())

	tests := []struct {
		args     []string
		contains string
	}{
		{[]string{"show", "house", dbPath}, "HOUSE"},
		{[]string{"show", "projects", dbPath}, "PROJECTS"},
		{[]string{"show", "projects", "--json", dbPath}, `"title"`},
		{[]string{"show", "all", dbPath}, "VENDORS"},
		{[]string{"query", "SELECT count(*) as n FROM vendors", dbPath}, "n"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var buf bytes.Buffer
			root := newRootCmd()
			root.SetOut(&buf)
			root.SetArgs(tt.args)
			require.NoError(t, root.Execute())
			assert.Contains(t, buf.String(), tt.contains)
		})
	}
}
```

- [ ] **Step 2: Run the full test suite**

Run: `go test -shuffle=on ./cmd/micasa/`
Expected: all pass

- [ ] **Step 3: Run linters**

Run: `golangci-lint run ./cmd/micasa/`
Expected: no warnings

- [ ] **Step 4: Verify the full build**

Run: `go build ./cmd/micasa/`
Expected: compiles cleanly

- [ ] **Step 5: Commit any polish fixes**

```
test(cli): add show/query integration tests
```

---

### Task 10: Update docs audit

- [ ] **Step 1: Run `/audit-docs`**

Check if any documentation needs updating for the new CLI commands.

- [ ] **Step 2: Commit doc changes if any**
