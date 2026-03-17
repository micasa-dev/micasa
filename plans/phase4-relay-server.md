<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Phase 4: Relay Server MVP

## Goal

A Go HTTP service that stores and relays encrypted sync operations
between household devices. The server cannot read the data it stores.

## Architecture

- `internal/sync/` -- shared types between client and relay
- `cmd/relay/` -- relay server binary
- PostgreSQL for relay metadata
- S3-compatible storage for encrypted blobs (Phase 4 MVP uses local FS)

## Database Schema (PostgreSQL)

### households

| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | ULID |
| created_at | TIMESTAMPTZ | |

### devices

| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | ULID |
| household_id | TEXT FK | references households |
| name | TEXT | hostname |
| token_hash | TEXT | bcrypt hash of bearer token |
| public_key | BYTEA | Curve25519 public key |
| created_at | TIMESTAMPTZ | |

### ops

| Column | Type | Notes |
|--------|------|-------|
| id | TEXT PK | ULID (client-assigned) |
| household_id | TEXT FK | references households |
| device_id | TEXT FK | references devices |
| seq | BIGINT | per-household monotonic |
| nonce | BYTEA | 24-byte XSalsa20 nonce |
| ciphertext | BYTEA | encrypted oplog entry |
| created_at | TIMESTAMPTZ | client timestamp |
| received_at | TIMESTAMPTZ | server timestamp |

`seq` is assigned server-side via a per-household sequence.

## Implementation Plan

### Step 1: Shared types (`internal/sync`)

- `Envelope` struct (op encrypted for transit)
- `PushRequest` / `PushResponse`
- `PullResponse`

### Step 2: Relay store interface + PostgreSQL implementation

- `RelayStore` interface with Push, Pull, CreateHousehold,
  RegisterDevice, AuthenticateDevice methods
- PostgreSQL implementation with `pgx`

### Step 3: HTTP handlers

- `POST /sync/push` -- push encrypted ops
- `GET /sync/pull?after={seq}&limit={n}` -- pull ops
- `POST /households` -- create household
- `POST /devices` -- register device
- Bearer token auth middleware

### Step 4: Server main (`cmd/relay`)

- Config from environment (DATABASE_URL, PORT)
- Graceful shutdown
- Health check endpoint

### Step 5: Tests

- Handler tests with mock store (httptest)
- Store integration tests (require PostgreSQL, skip if unavailable)

## Dependencies (new)

- `github.com/jackc/pgx/v5` -- PostgreSQL driver
- `golang.org/x/crypto/bcrypt` -- token hashing (already have x/crypto)

## File Changes

| File | Change |
|------|--------|
| `internal/sync/types.go` | New: shared types |
| `internal/relay/store.go` | New: relay store interface |
| `internal/relay/postgres.go` | New: PostgreSQL implementation |
| `internal/relay/handler.go` | New: HTTP handlers |
| `internal/relay/handler_test.go` | New: handler tests |
| `cmd/relay/main.go` | New: server entry point |
