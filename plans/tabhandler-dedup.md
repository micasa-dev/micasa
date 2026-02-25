<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# TabHandler Implementation Deduplication

Issue: #520

## Problem

The 8 `TabHandler` implementations in `handlers.go` share near-identical
patterns in two areas:

1. **Snapshot()** -- 8 implementations with the same get-check-build structure.
   Only the getter, FormKind, description formatter, and updater vary.
2. **Load() count-fetch blocks** -- ~15 instances of
   `counts, err := fn(ids); if err != nil { counts = map[uint]int{} }` across
   main handlers and scoped handler constructors.

## Approach

Two targeted helpers. No framework, no config structs, no interface changes.

### 1. `makeSnapshot[T any]` generic helper

```go
func makeSnapshot[T any](
    store *data.Store,
    id    uint,
    get   func(uint) (T, error),
    kind  FormKind,
    desc  func(T) string,
    restore func(T) error,
) (undoEntry, bool)
```

Each handler's `Snapshot()` becomes a one-liner delegating to this. The two
handlers with extra state (quoteHandler captures `vendor`, serviceLogHandler
captures `vendor`) use closures in their `restore` argument:

```go
func (quoteHandler) Snapshot(s *data.Store, id uint) (undoEntry, bool) {
    return makeSnapshot(s, id, s.GetQuote, formQuote,
        func(q data.Quote) string { return fmt.Sprintf("quote from %s", q.Vendor.Name) },
        func(q data.Quote) error  { return s.UpdateQuote(q, q.Vendor) },
    )
}
```

### 2. `fetchCounts` helper

```go
func fetchCounts(fn func([]uint) (map[uint]int, error), ids []uint) map[uint]int
```

Returns the count map on success, empty map on error. Replaces the 3-line
pattern everywhere it appears (~15 call sites in main and scoped handlers).

Before:
```go
quoteCounts, err := store.CountQuotesByProject(ids)
if err != nil {
    quoteCounts = map[uint]int{}
}
```

After:
```go
quoteCounts := fetchCounts(store.CountQuotesByProject, ids)
```

### 3. Scoped handler constructors -- no structural change

The scoped constructors (`newApplianceMaintenanceHandler`, etc.) vary enough in
their overrides (inlineEditFn, startAddFn, submitFn) that a factory abstraction
would not materially reduce complexity. These stay as-is, but their internal
`loadFn` closures benefit from `fetchCounts`.

## Out of scope

- No changes to the `TabHandler` interface.
- No changes to `scopedHandler` struct.
- No changes to Load() structure beyond replacing count-fetch boilerplate.
- Scoped handler factory (approach 3 from the issue) -- deferred; the
  `fetchCounts` cleanup is sufficient for now.

## Verification

- `go build ./...`
- `go test -shuffle=on ./...`
- Existing tests cover Snapshot and Load paths through integration tests.
