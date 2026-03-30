<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Relay Row-Level Security for Household Data Isolation

<!-- GitHub issue: #847 -->
<!-- Created: 2026-03-29 -->

## Problem

All relay tables (ops, devices, invites, key_exchanges, blobs) are keyed by
`household_id`, but household isolation is enforced only at the application
layer (handler checks `dev.HouseholdID`). A bug in the handler or a direct
database connection could leak data across households.

## Solution

Add Postgres row-level security (RLS) as a defense-in-depth layer on `ops`
and `blobs` (the tables containing encrypted household data). The database
itself prevents cross-household access to these tables, regardless of
application-layer bugs. Other tables remain protected by application-level
checks.

RLS only applies to the Postgres-backed relay (cloud or self-hosted). Local-
only users (SQLite) are unaffected.

## Architecture

### `internal/relay/rlsdb` package

A separate package that encapsulates all database access behind RLS-aware
transaction wrappers. The raw `*gorm.DB` is an unexported field, making it
structurally inaccessible from the `relay` package at compile time.

```go
package rlsdb

type DB struct {
    raw *gorm.DB // unexported, inaccessible from relay package
}

func New(db *gorm.DB) *DB

// Tx opens a transaction scoped to the given household.
// All store methods use this as the standard database access path.
func (d *DB) Tx(ctx context.Context, householdID string, fn func(tx *gorm.DB) error) error

// WithoutHousehold opens a transaction without household scoping.
//
// SAFETY: This bypasses row-level security. The caller is responsible for
// ensuring the operation is inherently cross-household (e.g., device
// authentication by token hash). Adding a new call site requires review
// and a comment explaining why household scoping is impossible.
func (d *DB) WithoutHousehold(ctx context.Context, fn func(tx *gorm.DB) error) error

// Migrate runs GORM AutoMigrate. Construction-time only.
func (d *DB) Migrate(models ...any) error

// InitRLS enables row-level security and creates policies for the given
// tables. Idempotent. Construction-time only.
func (d *DB) InitRLS(tables []RLSTable) error
```

`RLSTable` specifies the table name and which column holds the household ID:

```go
type RLSTable struct {
    Name   string // e.g., "ops"
    Column string // e.g., "household_id"
}
```

**Key property**: The Go compiler prevents any code outside `rlsdb` from
directly accessing `raw` -- there is no `s.db` field to misuse. `Tx` and
`WithoutHousehold` pass a transaction-scoped `*gorm.DB` to their callbacks;
this handle is invalid after the callback returns (the transaction commits or
rolls back). Code inside the callback can run arbitrary SQL on that handle,
but leaking it would cause immediate query failures, not silent RLS bypass.

### RLS DDL setup

`InitRLS` runs once inside `PgStore.AutoMigrate()`, after GORM schema
migration completes. Callers must invoke `AutoMigrate()` after
`OpenPgStore()` to establish RLS enforcement. The setup is
idempotent via `DROP POLICY IF EXISTS` + `CREATE POLICY`:

For each table in the `RLSTable` list:

1. `ALTER TABLE <t> ENABLE ROW LEVEL SECURITY`
2. `ALTER TABLE <t> FORCE ROW LEVEL SECURITY` (policies apply even to owner)
3. `DROP POLICY IF EXISTS <t>_household_isolation ON <t>`
4. `CREATE POLICY <t>_household_isolation ON <t> USING (<col> = current_setting('app.household_id')) WITH CHECK (<col> = current_setting('app.household_id'))`

The `USING` clause controls which existing rows are visible (SELECT, UPDATE,
DELETE). The `WITH CHECK` clause controls which new row values are permitted
(INSERT, UPDATE). Both enforce the same predicate, so the combined effect is:
a transaction can only read, write, and delete rows matching its household.

**Fail-safe behavior**: `current_setting('app.household_id')` (without the
`missing_ok` flag) throws an error if the setting has not been set in the
current session. This means:

- `Tx` sets the value before any query -- works as expected
- `WithoutHousehold` does NOT set the value, so if code inside that callback
  accidentally queries an RLS-enabled table, Postgres raises an error instead
  of silently returning all rows. This is intentional defense-in-depth.
- The error only fires when the policy expression is evaluated (i.e., when
  querying an RLS-enabled table). Queries against non-RLS tables (`households`,
  `devices`, `invites`, `key_exchanges`) in a `WithoutHousehold` callback work
  fine.

### Tables and policies

| Table            | Column         | Policy predicate                                        |
|------------------|----------------|---------------------------------------------------------|
| `ops`            | `household_id` | `household_id = current_setting('app.household_id')`    |
| `blobs`          | `household_id` | `household_id = current_setting('app.household_id')`    |

These two tables contain all sensitive household data: encrypted sync
operations and encrypted documents/attachments. Every method that touches
them knows the household ID and uses `Tx`.

**Excluded tables** (4):

- **`households`**: `HouseholdBySubscription` and `HouseholdByCustomer` look
  up household rows by Stripe IDs (not by household ID) during webhook
  processing. With RLS on `households` and `WithoutHousehold` (no
  `app.household_id` set), `current_setting` throws -- these methods would
  break. The household table contains billing metadata and sync counters, not
  the actual encrypted household data.
- **`devices`**: `AuthenticateDevice` looks up devices by token hash to
  discover which household a token belongs to. This is inherently cross-
  household. `StartJoin` also reads the inviter's device by ID to get its
  public key.
- **`invites`**: The join flow is cross-household by design. `StartJoin` is
  an unauthenticated endpoint -- the joining device has no household yet, so
  there is no household_id to set for RLS. Invites are looked up by invite
  code, not by household.
- **`key_exchanges`**: Also part of the cross-household join flow.
  `GetKeyExchangeResult` is unauthenticated (the joiner polls it before
  receiving a device token). Key exchanges are looked up by exchange ID, and
  the encrypted data is useless without the corresponding private key.

### Role strategy

Single role with `FORCE ROW LEVEL SECURITY`. The application connects as the
table owner, but `FORCE` ensures policies apply even to the owner. This avoids
the deployment complexity of managing two roles/DSNs.

### PgStore changes

`PgStore` replaces its `db *gorm.DB` field with `rls *rlsdb.DB`. Every store
method migrates from `s.db.WithContext(ctx).Transaction(...)` to
`s.rls.Tx(ctx, householdID, ...)`.

Methods that currently skip transactions (reads like `Pull`, `GetBlob`,
`HasBlob`, `BlobUsage`, `OpsCount`) get wrapped in `Tx`. Postgres read-only
transactions are cheap and `SET LOCAL` requires a transaction.

**Methods using `Tx` (household-scoped):**

- `Push`, `Pull`, `CreateHousehold`, `RegisterDevice`, `CreateInvite`,
  `CompleteKeyExchange`, `GetPendingExchanges`, `ListDevices`, `RevokeDevice`,
  `GetHousehold`, `UpdateSubscription`, `UpdateCustomerID`, `PutBlob`,
  `GetBlob`, `HasBlob`, `BlobUsage`, `OpsCount`

For methods that only touch non-RLS tables (e.g., `CreateInvite` touches
`invites`, `CompleteKeyExchange` touches `key_exchanges` and `devices`), `Tx`
is still used as the default path when the household ID comes from a trusted
source (authenticated device). The `SET LOCAL` is harmless -- it sets a
session variable that non-RLS tables ignore. `StartJoin` is the exception: it
uses `WithoutHousehold` because its household ID comes from an unauthenticated
URL path (see below).

**Methods using `WithoutHousehold` (explicit bypass):**

- `AuthenticateDevice` -- looks up device by token hash, discovers household
  ID. Cannot know the household ID before querying.
- `GetKeyExchangeResult` -- polled by an unauthenticated joiner before they
  receive a device token. The Store interface takes only `exchangeID`, no
  `householdID` parameter. Security relies on the exchange ID being a 256-bit
  crypto-random hex string.
- `StartJoin` -- unauthenticated endpoint; the household ID comes from the
  URL path (attacker-controlled). Using `Tx` would set the RLS scope to an
  unvalidated value. `WithoutHousehold` ensures that if `StartJoin` is later
  modified to touch an RLS table, the fail-safe error fires instead of
  silently applying attacker-chosen scope. The application-level check
  (`inv.HouseholdID != householdID`) validates the household after the invite
  is fetched.
- `HouseholdBySubscription` -- Stripe webhook handler looks up household by
  `stripe_subscription_id`. The caller only knows the Stripe ID, not the
  household ID, so it cannot use `Tx`.
- `HouseholdByCustomer` -- Stripe webhook handler looks up household by
  `stripe_customer_id`. Same pattern -- no household ID available.

**Methods that do not touch the database:**

- `SetEncryptionKey` -- sets an in-memory encryption key, no DB access.
- `Close` -- releases resources (closes the DB connection).

**Note on `Push`**: the Store interface takes `[]sync.Envelope` with no
explicit `householdID` parameter. The handler guarantees all ops have the same
`HouseholdID` (it overwrites them from the authenticated device). `Push` uses
`ops[0].HouseholdID` for `Tx`. An empty batch returns early before `Tx`.

**Note on `CreateHousehold`**: currently generates the household UUID inside
the transaction. With `Tx`, UUID generation moves before the call so the ID
can be passed as the `householdID` parameter. Neither `households` nor
`devices` has RLS, so the `SET LOCAL` is harmless -- `Tx` is used for
consistency.

**Note on `StartJoin`**: uses `WithoutHousehold` because the household ID
comes from an unauthenticated URL path (attacker-controlled input). None of
the tables `StartJoin` touches have RLS, so `WithoutHousehold` works
correctly. If `StartJoin` is later modified to touch an RLS-enabled table,
the fail-safe error from `WithoutHousehold` forces the developer to address
the trust boundary explicitly.

### Application-level checks

All existing store-level household checks in `PgStore` are **retained**:

- `CompleteKeyExchange`: `ex.HouseholdID != householdID` -- `key_exchanges`
  has no RLS, so this check is load-bearing
- `StartJoin`: `inv.HouseholdID != householdID` -- `invites` has no RLS
- `RevokeDevice`: `dev.HouseholdID != householdID` -- `devices` has no RLS

These checks also remain in `MemStore`. Handler-level 403 checks remain
unchanged in both implementations.

RLS protects `ops` and `blobs` (the encrypted household data) at the database
level. The remaining tables rely on application-level checks (same as today),
now verified by the store contract test suite.

## Testing

### Store contract test suite (`store_contract_test.go`)

A shared test suite that runs against both `MemStore` and `PgStore`, verifying
household isolation as a `Store` interface contract:

- Create two households, create invite in household 1, attempt to join with
  household 2's credentials -- verify error
- Push ops to household 1, pull from household 2 -- verify empty
- Upload blob to household 1, get blob from household 2 -- verify not found
- Revoke device in household 1 from household 2 -- verify error
- Complete key exchange for household 1 from household 2 -- verify error

These tests do not care HOW isolation is enforced (application checks vs RLS).
They verify the WHAT. The existing cross-household tests in `handler_test.go`
remain unchanged (they test handler-level 403 enforcement).

### RLS-specific tests (`rls_test.go`)

Postgres-only tests that verify RLS policies directly:

- `SET LOCAL app.household_id` correctly scopes SELECT/INSERT/UPDATE/DELETE
- Queries without `SET LOCAL` on an RLS-enabled table raise an error
  (`current_setting` throws when `app.household_id` is unset), confirming the
  fail-safe behavior
- `FORCE ROW LEVEL SECURITY` works -- even the table owner cannot bypass
  policies
- Attempting to INSERT with mismatched `household_id` violates `WITH CHECK`
  and returns an error
- Queries on non-RLS tables (`households`, `devices`, `invites`,
  `key_exchanges`) work regardless of whether `app.household_id` is set

### Wrapper unit tests (`rlsdb_test.go`)

Tests in the `rlsdb` package verifying:

- `Tx` sets `app.household_id` correctly (verify via `current_setting` in the
  callback)
- `WithoutHousehold` does NOT set `app.household_id`
- Both methods properly open and close transactions
- Error propagation from callbacks
- Context cancellation handling

## AGENTS.md hard rule

New entry under "Architecture and code style":

```
- **All relay Postgres access goes through `rlsdb.DB.Tx`**: Every PgStore
  method that touches the database MUST use `s.rls.Tx(ctx, householdID, fn)`.
  This is not a guideline -- it is the ONLY way to obtain a `*gorm.DB` for
  queries. The `rlsdb` package enforces this structurally: the raw `*gorm.DB`
  is unexported and inaccessible from the `relay` package. Do NOT:
  - Store a `*gorm.DB` reference on `PgStore`
  - Pass a `*gorm.DB` through context values
  - Create a second `gorm.Open` connection
  - Import `rlsdb` internals via `unsafe` or reflection
  `WithoutHousehold` is for methods that ONLY touch non-RLS tables and
  genuinely have no household ID available. It is NOT a fallback for
  untrusted input. Each call site MUST have a `// SAFETY:` comment.
  The approved call sites are: `AutoMigrate` (construction-time DDL),
  `AuthenticateDevice` (token hash lookup), `GetKeyExchangeResult`
  (unauthenticated joiner), `StartJoin` (unauthenticated, non-RLS
  tables only), `HouseholdBySubscription` (Stripe webhook), and
  `HouseholdByCustomer` (Stripe webhook). New call sites require
  explicit user approval before implementation.
```

## Scope boundaries

**In scope:**

- New `internal/relay/rlsdb` package
- `initRLS` function for DDL setup
- Migrate all `PgStore` methods to `rlsdb.DB.Tx` / `rlsdb.DB.WithoutHousehold`
- Retain all existing store-level household checks (they protect non-RLS tables)
- Store contract test suite (`store_contract_test.go`)
- `rls_test.go` for Postgres-specific policy tests
- `rlsdb_test.go` for wrapper unit tests
- AGENTS.md hard rule

**Out of scope:**

- Handler-level changes (handler checks remain as-is)
- MemStore changes (keeps its application-level checks)
- Multi-role/multi-DSN deployment changes
- Connection pool configuration
