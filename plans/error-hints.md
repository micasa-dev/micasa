<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Error Hints: Lightweight User-Facing Error Improvement

GitHub issue: #543

## Problem

~19 `setStatusError(err.Error())` call sites show bare technical errors.
Validation sentinel errors (`ErrInvalidInt`, `ErrInvalidDate`, etc.) lack
field context -- users see "invalid integer value" instead of "Year Built
should be a whole number".

The `moneyError` helper is the gold standard but only covers money fields.

## Design

### `WithHint` / `Hint` (generic wrapper)

A `hintError` struct wraps any error with a user-facing hint string.
`.Error()` returns the hint. The original error is preserved via `Unwrap()`
for `errors.Is`/`errors.As`.

```go
type hintError struct {
    hint string
    err  error
}
func (e *hintError) Error() string { return e.hint }
func (e *hintError) Unwrap() error { return e.err }

func WithHint(err error, hint string) error
func Hint(err error) string  // walks chain via errors.As
```

### `FieldError` (validation-specific)

Maps sentinel validation errors to field-specific messages:

```go
func FieldError(label string, err error) error
```

Produces messages like:
- `ErrInvalidInt`      -> "Year Built should be a whole number"
- `ErrInvalidDate`     -> "Start Date should be YYYY-MM-DD or a relative date"
- `ErrNegativeMoney`   -> "Budget must be a positive amount"
- `ErrInvalidMoney`    -> "Budget should look like 1250.00"
- `ErrInvalidFloat`    -> "Bathrooms should be a number like 2.5"
- `ErrInvalidInterval` -> "Interval should be months (6), or 6m, 1y, 2y 6m"
- Unknown errors       -> "Label: original message"

Key property: `errors.Is(FieldError("X", ErrInvalidInt), ErrInvalidInt)` is
true, preserving the error chain.

## Changes

1. **New file: `internal/data/errors.go`** -- `hintError`, `WithHint`, `Hint`, `FieldError`
2. **New file: `internal/data/errors_test.go`** -- unit tests for chain preservation and extraction
3. **`internal/app/forms.go`** -- form submit handlers: `return err` -> `return data.FieldError(label, err)`
4. **`internal/app/forms.go`** -- inline validators: refactor `optionalInt`, `optionalDate`, etc. to use `FieldError`
5. **`internal/app/forms.go`** -- remove `moneyError` (subsumed by `FieldError`)
6. **`internal/app/model.go`** -- operational error sites: add `WithHint` where context helps

## Non-goals

- No external dependencies
- No stack traces (not needed for TUI)
- No error domains or categories
- No changes to error wrapping in `internal/data/` store methods (already good)
