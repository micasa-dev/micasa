<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Phase 5: Client Sync

## Goal

Push local ops to the relay, pull remote ops and apply them to the local
DB with last-write-wins conflict resolution. The app works fully offline;
sync is opportunistic.

## Implementation Plan

### Step 1: Sync client (`internal/sync/client.go`)

HTTP client that talks to the relay:
- `Push()` -- batch unsynced ops, encrypt, POST to relay
- `Pull()` -- GET from relay, decrypt, return ops
- Uses `internal/crypto` for encrypt/decrypt

### Step 2: Op applier (`internal/sync/apply.go`)

Applies remote ops to the local SQLite database:
- `ApplyOps()` -- for each op: decode payload, upsert/delete row
- LWW conflict resolution: compare `created_at` timestamps
- Uses `WithSyncApplying(ctx)` to suppress local oplog writes
- Rebuilds local `DeletionRecord` entries from delete/restore ops

### Step 3: Conflict detection

When a local unsynced op conflicts with an incoming remote op on the
same (table, row_id):
- Later `created_at` wins
- Tiebreaker: lexicographic device_id
- Losing op gets `applied_at = NULL`

### Step 4: Tests

- Push serializes and encrypts correctly
- Pull decrypts and deserializes correctly
- LWW resolution picks correct winner
- Apply creates/updates/deletes rows correctly
- Conflict loser preserved with applied_at = NULL
- Sync-applying flag suppresses oplog writes (already tested in Phase 2)

## File Changes

| File | Change |
|------|--------|
| `internal/sync/client.go` | New: relay HTTP client |
| `internal/sync/apply.go` | New: op applier with LWW |
| `internal/sync/client_test.go` | New: client tests |
| `internal/sync/apply_test.go` | New: apply tests |
