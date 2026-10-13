<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Issue #923: `go install` error

## Root cause

The repository was migrated from `github.com/cpcloud/micasa` to
`github.com/micasa-dev/micasa`. Old version tags (v1.0.0 through v1.80.0 and
v2.0.0 through v2.3.0) still have `module github.com/cpcloud/micasa` in their
`go.mod`. When a user runs:

```sh
go install github.com/micasa-dev/micasa/cmd/micasa@latest
```

Go resolves `@latest` and finds these old tags. The mismatch between the
requested module path (`github.com/micasa-dev/micasa`) and the module path
declared in the old tags (`github.com/cpcloud/micasa`) causes Go to error.

## Fix

Add a `retract` directive to `go.mod` covering the v1.x range:

```go
retract [v1.0.0, v1.80.0]
```

This tells Go to skip those versions when resolving `@latest`.

The v2.x tags (v2.0.0 through v2.3.0) also declared
`module github.com/cpcloud/micasa` without a `/v2` suffix. Because the module
path differs, Go treats them as a completely different module -- they do not
conflict with `github.com/micasa-dev/micasa` and cannot be retracted from this
module's `go.mod`.

Starting from v2.4.0, all tags use the correct module path
`github.com/micasa-dev/micasa`, so no further action is needed.

## What was NOT changed

- Module path stays `github.com/micasa-dev/micasa` (no `/v2` suffix)
- No import path changes anywhere in the codebase
- Install docs already used `@latest`, no changes needed
