<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Checklist for adding a new entity model to the application.

Adding an entity touches many files across data, app, and generated code.
Follow every step; missing one leaves the entity partially wired.

## 1. Data layer (`internal/data/`)

- Add GORM struct to `models.go` (ID, CreatedAt, UpdatedAt, DeletedAt for
  soft-deletable entities)
- Add CRUD functions to `store.go` (List, GetByID, Create, Update, Delete,
  Restore) using `listQuery`/`getByID` generics
- Add dependency checks (`checkDependencies`) if this entity is a FK parent
- Add `requireParentAlive` calls if this entity has FK parents
- Run `go generate ./internal/data/...` to regenerate `meta_generated.go`
  (Table*/Col* constants)
- Add entity to `entity_context.go` (LLM entity names) and `entity_rows.go`
  (FK resolution tuples)

## 2. Enums (`internal/app/types.go`)

- Add `tabXxx` to `TabKind` iota block
- Add `formXxx` to `FormKind` iota block
- Update `TabKind.String()`, `.singular()`, `.plural()` switch statements

## 3. Handler (`internal/app/handlers.go`)

- Create `xxxHandler` struct implementing `TabHandler`:
  - `FormKind()`, `Load()`, `Delete()`, `Restore()`
  - `StartAddForm()`, `StartEditForm()`, `InlineEdit()`
  - `SubmitForm()`, `Snapshot()`, `SyncFixedValues()`
- Use an existing handler (e.g. `applianceHandler`) as a template

## 4. Table definition (`internal/app/tables.go`)

- Add column specs (`[]columnSpec`) for the new tab
- Register the tab in the `tabs` slice with its `TabKind`, name, and handler

## 5. Form definition (`internal/app/forms.go`)

- Add form builder function matching the handler's `StartAddForm`/`StartEditForm`
- Define form data struct and validators

## 6. Mouse zones

- Zone-mark interactive elements with `m.zones.Mark()`
- Follow zone ID conventions in `mouse.go`

## 7. Tests

- User-flow tests (mandatory): form add via `openAddForm` + `ctrl+s`,
  inline edit, delete/restore
- FK lifecycle tests if soft-deletable with relationships (see
  `/new-fk-relationship`)
- Mouse click tests in `mouse_test.go`

## 8. Generated code and docs

- `go generate ./internal/data/...` for meta constants
- Run `/audit-docs` to update Hugo docs, README, website
- Update `.claude/codebase/*.md` if the entity changes project structure

## 9. LLM context

- Add the entity to `entity_context.go` so the chat overlay can query it
- Add to `entity_rows.go` for FK resolution in extraction pipeline
