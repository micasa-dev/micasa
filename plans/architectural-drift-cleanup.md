<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Architectural Drift Cleanup

Comprehensive audit of codebase drift from conventions in CLAUDE.md and
internal patterns.

## Findings

### Hard rule violations

1. **ORDER BY without tiebreakers** (`store.go:508,516,524`) -- Three GORM
   queries order by `ColName` only: `ProjectTypes()`,
   `MaintenanceCategories()`, `ListVendors()`. The `entityRows` and
   `entityRowsUnscoped` functions in `entity_rows.go` already use the correct
   pattern (`nameCol + " ASC, " + ColID + " DESC"`).

2. **Duplicate line in flake.nix** (line 362-363) -- `ntapes=$(fd ...)` is
   written twice consecutively in the `record-animated` package.

3. **`-ldflags=` in release.yml** (line 61) -- Uses `=` in a `go build`
   command. Rule: "No `=` in CI go commands."

### Silent error discards in production code

4. **`ListAppliances` errors silently dropped in form builders**
   (`forms.go:437,471,528,570`) -- Four instances of
   `appliances, _ := m.store.ListAppliances(false)`. These are in functions
   that either return `error` or are called by functions that do. A failed DB
   read means the form opens with a broken dropdown. Should propagate.

5. **Best-effort DB writes** (`chat.go:273`, `forms.go:2532`,
   `model.go:129,1803`) -- `AppendChatInput`, `MarkTesseractHintSeen`,
   `PutLastModel`. Primary operations succeed regardless. These are acceptable
   silent discards but deserve brief documentation comments.

6. **Handler count-fetch fallbacks** (`handlers.go`: ~15 instances) -- Count
   queries fall back to empty maps on error. This is intentional degradation
   (show entities with 0 counts rather than failing the tab), but violates the
   letter of "no silent failures." Adding brief comments to document the
   intent.

7. **`models, _ := client.ListModels(ctx)`** (`chat.go:604`) -- Error
   discarded when checking model availability before pull. Actually correct:
   failure means "model not found locally, proceed to pull."

8. **`_ = model.loadDashboard()`** (`model.go:177`) and
   **`show, _ := store.GetShowDashboard()`** (`model.go:174`) -- Initialization
   code where failure means "start without dashboard." Acceptable degradation;
   add comments.

### Not fixing (intentional patterns)

- **DISTINCT queries in `query.go`** (lines 214-225) -- These use
  `SELECT DISTINCT name ... ORDER BY name`. DISTINCT on a single column cannot
  produce ties, so no tiebreaker needed.
- **`sqlite_master` query** (`query.go:29`) -- Table names are unique.
- **Handler code duplication** (Snapshot, Load patterns) -- Consistent within
  the pattern; abstracting changes the TabHandler interface. Future refactor.
- **`any` usage in GORM wrappers** (`store.go`) -- Required by GORM's API.
- **File cleanup `_ = f.Close()`** (`model.go:2543,2548,2577`) -- Standard Go
  idiom for deferred close/remove.

## Plan

1. Done -- Add ORDER BY tiebreakers to 3 GORM queries in `store.go`
2. Done -- Remove duplicate line in `flake.nix`
3. Done -- Fix `-ldflags=` in `release.yml`
4. Done -- Propagate `ListAppliances` errors in `forms.go` (4 instances)
5. Done -- Add comments on intentional best-effort discards
6. Done -- Add comment on handler count-fetch fallback pattern
7. Done -- All tests pass
