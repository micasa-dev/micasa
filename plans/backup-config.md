<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Backup Config Defaults

Issue: https://github.com/cpcloud/micasa/issues/477

## Problem

The backup command requires specifying the destination path every time. Users
want to configure a default destination in the config file, and optionally
auto-stamp backups with a timestamp so successive runs don't collide.

## Design

### New `[backup]` config section

```toml
[backup]
# Default destination path for backups. Accepts ~ for home directory.
# If the path is a directory, the backup is placed inside it using the
# source filename with a .backup extension.
# dest = "~/backups/micasa"

# Automatically insert a timestamp into the backup filename.
# The timestamp is inserted before the file extension:
#   micasa.db -> micasa-20260223T103045.db
# Default: false.
# timestamp = false
```

### New struct in `internal/config/config.go`

```go
type Backup struct {
    Dest      string `toml:"dest"      env:"MICASA_BACKUP_DEST"`
    Timestamp *bool  `toml:"timestamp,omitempty" env:"MICASA_BACKUP_TIMESTAMP"`
}
```

Add `Backup Backup` field to `Config`.

### New CLI flag

Add `--timestamp` (`-t`) flag to `backupCmd` that overrides the config value
for a single invocation.

### Destination resolution order

1. CLI positional arg (explicit dest)
2. Config `backup.dest`
3. Default: `<source>.backup`

If the resolved dest is a directory, place the file inside it using the
source basename with `.backup` extension.

### Timestamp insertion

When enabled (config or CLI flag), insert a UTC timestamp before the file
extension. Format: `20060102T150405` (compact, filesystem-safe, sorts
chronologically).

- `backup.db` -> `backup-20260223T103045.db`
- `micasa.db.backup` -> `micasa.db-20260223T103045.backup`
- No extension -> `filename-20260223T103045`

### Validation

- `backup.dest` is validated via `data.ValidateDBPath()` if it looks like a
  file path (not a directory).
- The destination-already-exists check runs on the final resolved path (after
  timestamp insertion).

## Files to change

1. `internal/config/config.go` -- add `Backup` struct, wire into `Config`,
   add defaults, update `ExampleTOML()`
2. `cmd/micasa/main.go` -- load config in `backupCmd.Run()`, add `--timestamp`
   flag, implement resolution logic
3. `internal/config/config_test.go` -- tests for new config fields
4. `cmd/micasa/main_test.go` -- tests for backup with config and timestamp

## Non-goals

- Scheduled/automatic backups (separate feature)
- Backup rotation or pruning
