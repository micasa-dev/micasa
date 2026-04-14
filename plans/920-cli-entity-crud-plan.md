<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# CLI Entity CRUD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `micasa <entity> <verb>` CLI commands for full CRUD on all entity types, enabling programmatic/scripted access without the TUI.

**Architecture:** Generic command builder using Go generics (`entityDef[T]`) constructs cobra command trees per entity. Each entity provides field definitions, decode/encode functions, and store method wrappers. Tests call entity functions directly with a `*data.Store` (following show_test.go pattern). Shared helpers in `entity.go`, per-entity definitions in `entity_<name>.go`, tests in `entity_test.go`.

**Tech Stack:** Go, cobra, testify, GORM/SQLite, encoding/json

**Spec:** `plans/920-cli-entity-crud.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `cmd/micasa/entity.go` | Generic builder (`buildEntityCmd`), JSON decode helpers, shared flags (`--table`, `--deleted`, `--data`, `--data-file`), `encodeJSON` output, partial-merge logic |
| `cmd/micasa/entity_vendor.go` | Vendor entity definition and handlers |
| `cmd/micasa/entity_project.go` | Project entity definition and handlers |
| `cmd/micasa/entity_appliance.go` | Appliance entity definition and handlers |
| `cmd/micasa/entity_incident.go` | Incident entity definition and handlers |
| `cmd/micasa/entity_quote.go` | Quote entity definition and handlers (vendor resolution) |
| `cmd/micasa/entity_maintenance.go` | Maintenance entity definition and handlers |
| `cmd/micasa/entity_servicelog.go` | Service-log entity definition and handlers (vendor resolution) |
| `cmd/micasa/entity_document.go` | Document entity definition and handlers (`--file` upload) |
| `cmd/micasa/entity_house.go` | House singleton (get/add/edit only) |
| `cmd/micasa/entity_lookup.go` | Project-type and maintenance-category (list only) |
| `cmd/micasa/entity_test.go` | All entity CRUD lifecycle tests |
| `cmd/micasa/main.go` | Register entity commands in `newRootCmd()` |
| `cmd/micasa/show.go` | Add cobra `Deprecated` field to per-entity subcommands |

---

### Task 1: Generic Entity Command Builder

**Files:**
- Create: `cmd/micasa/entity.go`

This task builds the shared infrastructure all entity commands depend on.

- [ ] **Step 1: Create `entity.go` with types and helpers**

Key design points for the `entityDef` struct:
- `tableHeader` field (not derived from `name`) so entities like "maintenance"
  get "MAINTENANCE" not "MAINTENANCES".
- No `extraFlags` field -- document's `--file` flag is handled in its own
  custom `buildAddCmd` override since it needs to inject file data before
  calling `decodeAndCreate`.
- Tests follow show_test.go pattern: entity functions take `(io.Writer, *data.Store, ...)`
  and tests call them directly. Cobra wiring tested once via `executeCLI`.

```go
package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "strings"

    "github.com/micasa-dev/micasa/internal/data"
    "github.com/spf13/cobra"
    "gorm.io/gorm"
)

type entityDef[T any] struct {
    name        string // CLI name: "vendor", "service-log"
    singular    string // display: "vendor", "service log entry"
    tableHeader string // table header: "VENDORS", "SERVICE LOG"

    cols  []showCol[T]
    toMap func(T) map[string]any

    list func(*data.Store, bool) ([]T, error)
    get  func(*data.Store, string) (T, error)

    decodeAndCreate func(*data.Store, json.RawMessage) (T, error)
    decodeAndUpdate func(*data.Store, string, json.RawMessage) (T, error)

    del     func(*data.Store, string) error
    restore func(*data.Store, string) error

    deletedAt func(T) gorm.DeletedAt
}

func readInputData(cmd *cobra.Command) (json.RawMessage, error) {
    dataStr, _ := cmd.Flags().GetString("data")
    dataFile, _ := cmd.Flags().GetString("data-file")

    if dataStr != "" && dataFile != "" {
        return nil, errors.New("--data and --data-file are mutually exclusive")
    }
    if dataStr == "" && dataFile == "" {
        return nil, errors.New("--data or --data-file is required")
    }

    if dataFile != "" {
        b, err := os.ReadFile(dataFile) //nolint:gosec // user-specified input file
        if err != nil {
            return nil, fmt.Errorf("read data file: %w", err)
        }
        return json.RawMessage(b), nil
    }
    return json.RawMessage(dataStr), nil
}

func encodeJSON(w io.Writer, v any) error {
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    return enc.Encode(v)
}

// mergeField unmarshals a field from a raw JSON map into dst if present.
// Returns true if the field was present (even if null).
func mergeField(fields map[string]json.RawMessage, key string, dst any) (bool, error) {
    raw, ok := fields[key]
    if !ok {
        return false, nil
    }
    if err := json.Unmarshal(raw, dst); err != nil {
        return true, fmt.Errorf("field %s: %w", key, err)
    }
    return true, nil
}

func buildEntityCmd[T any](def entityDef[T]) *cobra.Command {
    cmd := &cobra.Command{
        Use:           def.name,
        Short:         fmt.Sprintf("Manage %ss", def.singular),
        SilenceErrors: true,
        SilenceUsage:  true,
    }

    if def.list != nil {
        cmd.AddCommand(buildListCmd(def))
    }
    if def.get != nil {
        cmd.AddCommand(buildGetCmd(def))
    }
    if def.decodeAndCreate != nil {
        cmd.AddCommand(buildAddCmd(def))
    }
    if def.decodeAndUpdate != nil {
        cmd.AddCommand(buildEditCmd(def))
    }
    if def.del != nil {
        cmd.AddCommand(buildDeleteCmd(def))
    }
    if def.restore != nil {
        cmd.AddCommand(buildRestoreCmd(def))
    }

    return cmd
}

func buildListCmd[T any](def entityDef[T]) *cobra.Command {
    var tableFlag bool
    var deletedFlag bool

    cmd := &cobra.Command{
        Use:           "list [database-path]",
        Short:         fmt.Sprintf("List %ss", def.singular),
        Args:          cobra.MaximumNArgs(1),
        SilenceErrors: true,
        SilenceUsage:  true,
        RunE: func(cmd *cobra.Command, args []string) error {
            store, err := openExisting(dbPathFromEnvOrArg(args))
            if err != nil {
                return err
            }
            defer func() { _ = store.Close() }()

            items, err := def.list(store, deletedFlag)
            if err != nil {
                return fmt.Errorf("list %ss: %w", def.singular, err)
            }

            cols, toMap := def.cols, def.toMap
            if deletedFlag && def.deletedAt != nil {
                cols, toMap = withDeletedCol(cols, toMap, true, def.deletedAt)
            }

            if tableFlag {
                return writeTable(cmd.OutOrStdout(),
                    def.tableHeader, items, cols)
            }
            return writeJSON(cmd.OutOrStdout(), items, toMap)
        },
    }

    cmd.Flags().BoolVar(&tableFlag, "table", false, "Output as table")
    cmd.Flags().BoolVar(&deletedFlag, "deleted", false,
        "Include soft-deleted rows")
    return cmd
}

// buildGetCmd, buildAddCmd, buildEditCmd, buildDeleteCmd, buildRestoreCmd
// follow the same pattern -- see entity.go for full implementations.
// Key: get/edit/delete/restore use cobra.RangeArgs(1, 2) where arg[0]
// is the entity ID and optional arg[1] is the database path.

func dbPathFromEnvOrArgStr(s string) string {
    if s != "" {
        return s
    }
    return os.Getenv("MICASA_DB_PATH")
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/micasa/`
Expected: Build succeeds (no entity files reference it yet, but types compile).

- [ ] **Step 3: Commit**

```
feat(cli): add generic entity command builder infrastructure
```

---

### Task 2: Vendor Entity (TDD)

**Files:**
- Create: `cmd/micasa/entity_vendor.go`
- Create: `cmd/micasa/entity_test.go`
- Modify: `cmd/micasa/main.go` -- register `newVendorCmd()`

This is the first entity. Tests drive the pattern: add -> list -> get -> edit -> delete -> restore. All subsequent entities follow this pattern.

- [ ] **Step 1: Write vendor lifecycle test**

Tests call entity functions directly with a `*data.Store` (like show_test.go
pattern). Cobra wiring is verified once via `executeCLI` for one entity.

```go
func TestVendorCRUD(t *testing.T) {
    t.Parallel()
    store := newTestStoreWithMigration(t)
    def := vendorEntityDef()

    // ADD
    created, err := def.decodeAndCreate(store,
        json.RawMessage(`{"name":"Acme Plumbing","phone":"5551234567"}`))
    require.NoError(t, err)
    m := def.toMap(created)
    assert.Equal(t, "Acme Plumbing", m["name"])
    id := m["id"].(string)
    require.NotEmpty(t, id)

    // LIST (JSON)
    items, err := def.list(store, false)
    require.NoError(t, err)
    require.Len(t, items, 1)

    // GET
    got, err := def.get(store, id)
    require.NoError(t, err)
    gm := def.toMap(got)
    assert.Equal(t, "Acme Plumbing", gm["name"])
    assert.Equal(t, "5551234567", gm["phone"])

    // EDIT (partial)
    edited, err := def.decodeAndUpdate(store, id,
        json.RawMessage(`{"phone":"5559876543"}`))
    require.NoError(t, err)
    em := def.toMap(edited)
    assert.Equal(t, "Acme Plumbing", em["name"])
    assert.Equal(t, "5559876543", em["phone"])

    // DELETE
    require.NoError(t, def.del(store, id))
    afterDel, err := def.list(store, false)
    require.NoError(t, err)
    assert.Empty(t, afterDel)

    // RESTORE
    require.NoError(t, def.restore(store, id))
    afterRestore, err := def.list(store, false)
    require.NoError(t, err)
    require.Len(t, afterRestore, 1)
}
```

Error case and output format tests:

```go
func TestVendorAddMissingName(t *testing.T) {
    t.Parallel()
    store := newTestStoreWithMigration(t)
    def := vendorEntityDef()
    _, err := def.decodeAndCreate(store,
        json.RawMessage(`{"phone":"123"}`))
    require.Error(t, err)
}

func TestVendorDeleteWithDeps(t *testing.T) {
    t.Parallel()
    store := newTestStoreWithMigration(t)
    // Create vendor, project, quote -- delete vendor should fail
}

func TestVendorListTable(t *testing.T) {
    t.Parallel()
    store := newTestStoreWithMigration(t)
    require.NoError(t, store.CreateVendor(&data.Vendor{Name: "TableTest"}))
    var buf bytes.Buffer
    def := vendorEntityDef()
    items, err := def.list(store, false)
    require.NoError(t, err)
    require.NoError(t, writeTable(&buf, def.tableHeader, items, def.cols))
    assert.Contains(t, buf.String(), "TableTest")
    assert.Contains(t, buf.String(), "NAME")
}

func TestVendorListDeleted(t *testing.T) {
    t.Parallel()
    store := newTestStoreWithMigration(t)
    def := vendorEntityDef()
    created, err := def.decodeAndCreate(store,
        json.RawMessage(`{"name":"Ghost"}`))
    require.NoError(t, err)
    id := def.toMap(created)["id"].(string)

    require.NoError(t, def.del(store, id))

    live, err := def.list(store, false)
    require.NoError(t, err)
    assert.Empty(t, live)

    all, err := def.list(store, true)
    require.NoError(t, err)
    require.Len(t, all, 1)
}

// TestVendorCobraWiring tests one entity through the full cobra tree
// via executeCLI to verify command registration works.
func TestVendorCobraWiring(t *testing.T) {
    t.Parallel()
    dbPath := createTestDB(t)
    out, err := executeCLI("vendor", "list", dbPath)
    require.NoError(t, err)
    // Empty seeded DB has no vendors -- expect empty JSON array
    assert.Equal(t, "[]\n", out)
}
```

- [ ] **Step 2: Run tests -- verify they fail**

Run: `go test -shuffle=on -run TestVendor ./cmd/micasa/`
Expected: Compile/test failures (entity command not implemented yet).

- [ ] **Step 3: Implement `entity_vendor.go`**

`vendorEntityDef()` returns the definition (used by both `newVendorCmd` and tests).

```go
package main

import (
    "encoding/json"
    "fmt"

    "github.com/micasa-dev/micasa/internal/data"
    "gorm.io/gorm"
)

func vendorEntityDef() entityDef[data.Vendor] {
    return entityDef[data.Vendor]{
        name:        "vendor",
        singular:    "vendor",
        tableHeader: "VENDORS",
        cols:        vendorCols,
        toMap:       vendorToMap,
        list: func(s *data.Store, deleted bool) ([]data.Vendor, error) {
            return s.ListVendors(deleted)
        },
        get: func(s *data.Store, id string) (data.Vendor, error) {
            return s.GetVendor(id)
        },
        decodeAndCreate: vendorCreate,
        decodeAndUpdate: vendorUpdate,
        del: func(s *data.Store, id string) error {
            return s.DeleteVendor(id)
        },
        restore: func(s *data.Store, id string) error {
            return s.RestoreVendor(id)
        },
        deletedAt: func(v data.Vendor) gorm.DeletedAt {
            return v.DeletedAt
        },
    }
}

func newVendorCmd() *cobra.Command {
    return buildEntityCmd(vendorEntityDef())
}

func vendorCreate(store *data.Store, raw json.RawMessage) (data.Vendor, error) {
    var v data.Vendor
    if err := json.Unmarshal(raw, &v); err != nil {
        return data.Vendor{}, fmt.Errorf("invalid JSON: %w", err)
    }
    if v.Name == "" {
        return data.Vendor{}, fmt.Errorf("name is required")
    }
    found, err := store.FindOrCreateVendor(v)
    if err != nil {
        return data.Vendor{}, err
    }
    return found, nil
}

func vendorUpdate(store *data.Store, id string, raw json.RawMessage) (data.Vendor, error) {
    existing, err := store.GetVendor(id)
    if err != nil {
        return data.Vendor{}, fmt.Errorf("get vendor: %w", err)
    }

    var fields map[string]json.RawMessage
    if err := json.Unmarshal(raw, &fields); err != nil {
        return data.Vendor{}, fmt.Errorf("invalid JSON: %w", err)
    }

    // Merge provided fields onto existing using mergeField helper.
    for _, pair := range []struct {
        key string
        dst any
    }{
        {"name", &existing.Name},
        {"contact_name", &existing.ContactName},
        {"email", &existing.Email},
        {"phone", &existing.Phone},
        {"website", &existing.Website},
        {"notes", &existing.Notes},
    } {
        if _, err := mergeField(fields, pair.key, pair.dst); err != nil {
            return data.Vendor{}, err
        }
    }

    if err := store.UpdateVendor(existing); err != nil {
        return data.Vendor{}, err
    }
    return existing, nil
}
```

- [ ] **Step 4: Register in `main.go`**

Add `newVendorCmd()` to `root.AddCommand(...)` in `newRootCmd()`.

- [ ] **Step 5: Run tests -- verify they pass**

Run: `go test -shuffle=on -run TestVendor ./cmd/micasa/`
Expected: All vendor tests pass.

- [ ] **Step 6: Commit**

```
feat(cli): add vendor entity CRUD commands
```

---

### Task 3: Project Entity

**Files:**
- Create: `cmd/micasa/entity_project.go`
- Modify: `cmd/micasa/entity_test.go` -- add project tests
- Modify: `cmd/micasa/main.go` -- register `newProjectCmd()`

Same pattern as vendor but with FK (`project_type_id`), status enum, nullable date and money fields.

- [ ] **Step 1: Write project lifecycle test**

Tests cover: add with required `project_type_id`, edit partial update (status only), edit clearing nullable field (`budget_cents: null`), delete with quote dependency error, list with `--table`.

- [ ] **Step 2: Run tests -- verify they fail**

- [ ] **Step 3: Implement `entity_project.go`**

Key differences from vendor:
- `decodeAndCreate`: validates `project_type_id` is non-empty, calls `store.CreateProject`.
- `decodeAndUpdate`: fetch-merge with nullable field support (`*int64`, `*time.Time`).
  Use `map[string]json.RawMessage` to detect present-but-null vs omitted.

- [ ] **Step 4: Register and verify tests pass**

- [ ] **Step 5: Commit**

```
feat(cli): add project entity CRUD commands
```

---

### Task 4: Appliance Entity

**Files:**
- Create: `cmd/micasa/entity_appliance.go`
- Modify: `cmd/micasa/entity_test.go`
- Modify: `cmd/micasa/main.go`

Simple entity similar to vendor. Has nullable date and money fields.

- [ ] **Step 1: Write appliance lifecycle test**

- [ ] **Step 2: Implement `entity_appliance.go`**

- [ ] **Step 3: Register, test, commit**

```
feat(cli): add appliance entity CRUD commands
```

---

### Task 5: Incident Entity

**Files:**
- Create: `cmd/micasa/entity_incident.go`
- Modify: `cmd/micasa/entity_test.go`
- Modify: `cmd/micasa/main.go`

Has optional FK to appliance and vendor (both `*string`), status/severity enums, required `date_noticed`.

- [ ] **Step 1: Write incident lifecycle test**

Cover: add with minimal fields (title, defaults for status/severity/date_noticed), edit status, delete (sets resolved), restore (recovers previous status).

- [ ] **Step 2: Implement `entity_incident.go`**

Key: `date_noticed` defaults to `time.Now()` if not provided in `add`.

- [ ] **Step 3: Register, test, commit**

```
feat(cli): add incident entity CRUD commands
```

---

### Task 6: Quote Entity (Vendor Resolution)

**Files:**
- Create: `cmd/micasa/entity_quote.go`
- Modify: `cmd/micasa/entity_test.go`
- Modify: `cmd/micasa/main.go`

Has vendor resolution logic: `vendor_name` convenience field, `vendor_id` alternative. Both route through `CreateQuote`/`UpdateQuote` which take a `Vendor` argument.

- [ ] **Step 1: Write quote lifecycle test**

Cover: add with `vendor_name` (find-or-create), add with `vendor_id`, edit preserving vendor (omit both fields), edit changing vendor via `vendor_name`.

- [ ] **Step 2: Implement `entity_quote.go`**

Key logic:
- `decodeAndCreate`: extract `vendor_name`/`vendor_id` from raw JSON. If `vendor_id` provided, fetch vendor by ID. If `vendor_name` provided, pass `Vendor{Name: name}`. Call `store.CreateQuote(&quote, vendor)`.
- `decodeAndUpdate`: if neither vendor field present, fetch current quote's vendor and pass it through to preserve. Otherwise resolve as in create. Call `store.UpdateQuote(quote, vendor)`.

- [ ] **Step 3: Register, test, commit**

```
feat(cli): add quote entity CRUD commands with vendor resolution
```

---

### Task 7: Maintenance Entity

**Files:**
- Create: `cmd/micasa/entity_maintenance.go`
- Modify: `cmd/micasa/entity_test.go`
- Modify: `cmd/micasa/main.go`

FK to `MaintenanceCategory` (required), optional FK to `Appliance`.

- [ ] **Step 1: Write maintenance lifecycle test**

Cover: add with required `category_id`, edit, delete with service-log dependency error.

- [ ] **Step 2: Implement `entity_maintenance.go`**

- [ ] **Step 3: Register, test, commit**

```
feat(cli): add maintenance entity CRUD commands
```

---

### Task 8: Service Log Entity (Vendor Resolution)

**Files:**
- Create: `cmd/micasa/entity_servicelog.go`
- Modify: `cmd/micasa/entity_test.go`
- Modify: `cmd/micasa/main.go`

Same vendor resolution pattern as quote. FK to `MaintenanceItem` (required).

- [ ] **Step 1: Write service-log lifecycle test**

Cover: add with `vendor_name`, edit clearing vendor (`vendor_id: null`), edit preserving vendor.

- [ ] **Step 2: Implement `entity_servicelog.go`**

Key: `CreateServiceLog` takes `*ServiceLogEntry` (pointer), `UpdateServiceLog` takes value + vendor.

- [ ] **Step 3: Register, test, commit**

```
feat(cli): add service-log entity CRUD commands with vendor resolution
```

---

### Task 9: Document Entity

**Files:**
- Create: `cmd/micasa/entity_document.go`
- Modify: `cmd/micasa/entity_test.go`
- Modify: `cmd/micasa/main.go`

Most complex entity. Optional `--file` flag for blob upload. Requires `entity_kind` and `entity_id` for `add`. Max file size enforced via config.

- [ ] **Step 1: Write document lifecycle test**

Cover: add metadata-only, add with `--file`, edit title, delete, restore. Error case: `--file` with oversized file.

- [ ] **Step 2: Implement `entity_document.go`**

Key logic:
- `add` command gets extra `--file` flag.
- When `--file` provided: read file, compute SHA256, detect MIME, set size, default title from filename.
- Must load config and set `store.SetMaxDocumentSize` before create.

- [ ] **Step 3: Register, test, commit**

```
feat(cli): add document entity CRUD commands with file upload
```

---

### Task 10: House Profile (Singleton)

**Files:**
- Create: `cmd/micasa/entity_house.go`
- Modify: `cmd/micasa/entity_test.go`
- Modify: `cmd/micasa/main.go`

Different verb set: get, add, edit. No list/delete/restore. Cannot use `buildEntityCmd` directly -- build manually or extend builder with option flags.

- [ ] **Step 1: Write house lifecycle test**

Cover: get empty (returns `{}`), add, get populated, edit partial, add duplicate error.

- [ ] **Step 2: Implement `entity_house.go`**

`newHouseCmd` builds a custom command with only get/add/edit subcommands.
- `get`: returns profile or `{}`.
- `add`: `store.CreateHouseProfile`. Errors if exists.
- `edit`: fetch, merge, `store.UpdateHouseProfile`.

- [ ] **Step 3: Register, test, commit**

```
feat(cli): add house profile entity commands (get/add/edit)
```

---

### Task 11: Lookup Tables (Read-Only)

**Files:**
- Create: `cmd/micasa/entity_lookup.go`
- Modify: `cmd/micasa/entity_test.go`
- Modify: `cmd/micasa/main.go`

`project-type` and `maintenance-category` -- list only.

- [ ] **Step 1: Write lookup table tests**

Cover: `project-type list` returns seeded types, `maintenance-category list` returns seeded categories. JSON and `--table` output.

- [ ] **Step 2: Implement `entity_lookup.go`**

Two minimal `entityDef` instances with only `name`, `singular`, `tableHeader`, `cols`, `toMap`, and `list` set. All other fields nil.

- [ ] **Step 3: Register, test, commit**

```
feat(cli): add project-type and maintenance-category list commands
```

---

### Task 12: Deprecate `show` Per-Entity Subcommands

**Files:**
- Modify: `cmd/micasa/show.go` -- add `Deprecated` to per-entity subcommands
- Modify: `cmd/micasa/show_test.go` -- verify deprecation behavior

- [ ] **Step 1: Write deprecation test**

```go
func TestShowDeprecationWarning(t *testing.T) {
    t.Parallel()
    store := newTestStoreWithMigration(t)
    // Run show vendors -- should still work but cobra sets Deprecated
    // which prints warning to stderr. Verify command still executes.
    var buf bytes.Buffer
    require.NoError(t, showVendors(&buf, store, false, false))
}
```

- [ ] **Step 2: Add `Deprecated` field to per-entity show subcommands**

Modify `newShowCmd()`: capture each returned command and set `Deprecated`:
```go
projects := newShowEntityCmd("projects", "Show projects", ...)
projects.Deprecated = "use 'micasa project list --table' instead"
cmd.AddCommand(projects)
```

Repeat for: vendors, appliances, incidents, quotes, maintenance,
service-log, documents, project-types, maintenance-categories, house.
Do NOT deprecate `all`.

- [ ] **Step 3: Verify `show all` is NOT deprecated**

- [ ] **Step 4: Test, commit**

```
refactor(cli): deprecate per-entity show subcommands in favor of entity CRUD
```

---

### Task 13: Input Validation and Edge Cases

**Files:**
- Modify: `cmd/micasa/entity_test.go`

Add cross-cutting tests:

- [ ] **Step 1: Test `--data` and `--data-file` mutual exclusivity**
- [ ] **Step 2: Test `--data-file` with file content**
- [ ] **Step 3: Test invalid JSON input**
- [ ] **Step 4: Test missing required fields per entity**
- [ ] **Step 5: Test nonexistent ID for get/edit/delete/restore**
- [ ] **Step 6: Run full test suite**

Run: `go test -shuffle=on ./cmd/micasa/`
Expected: All tests pass.

- [ ] **Step 7: Commit**

```
test(cli): add edge case and validation tests for entity CRUD
```

---

### Task 14: Final Verification

- [ ] **Step 1: Run full project tests**

Run: `go test -shuffle=on ./...`
Expected: All tests pass, no regressions.

- [ ] **Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: No warnings.

- [ ] **Step 3: Run coverage check**

Run: `go test -coverprofile cover.out ./cmd/micasa/`
Run: `go tool cover -func cover.out | grep entity`
Expected: New entity code has test coverage.

- [ ] **Step 4: Final commit if any fixups needed**

---

## Implementation Notes

### Test Helper Design

Tests call entity definition functions (`vendorEntityDef()`, etc.) directly
with a `*data.Store` from `newTestStoreWithMigration`. This follows the
show_test.go pattern where `showVendors(w, store, ...)` is called directly.

Each entity's `*EntityDef()` function returns the `entityDef` struct, giving
tests access to `list`, `get`, `decodeAndCreate`, `decodeAndUpdate`, `del`,
`restore`, `toMap`, `cols`, and `tableHeader`.

Cobra wiring is tested once per entity via `executeCLI` using `createTestDB`
(which writes a real temp file). This verifies the command is registered and
flags work. The bulk of CRUD logic testing goes through the direct functions.

### Partial Update Merge Strategy

Use `map[string]json.RawMessage` to detect which fields the caller provided:
1. Unmarshal raw JSON into `map[string]json.RawMessage`
2. For each field in the map, unmarshal into the corresponding struct field
3. Fields not in the map retain their fetched values

This handles the tri-state correctly:
- Omitted: field not in map -> keep existing
- Explicit value: field in map with value -> update
- Null: field in map with `null` -> set pointer to nil

### Entity Registration Order

Register in `main.go` alphabetically for discoverability:
```go
newApplianceCmd(),
newDocumentCmd(),
newHouseCmd(),
newIncidentCmd(),
newMaintenanceCategoryCmd(),
newMaintenanceCmd(),
newProjectCmd(),
newProjectTypeCmd(),
newQuoteCmd(),
newServiceLogCmd(),
newVendorCmd(),
```
