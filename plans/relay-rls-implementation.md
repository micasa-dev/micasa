<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Relay Row-Level Security Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Postgres row-level security on `ops` and `blobs` tables, with a compile-time-enforced `rlsdb` package that makes RLS-scoped transactions the only way to access the database.

**Architecture:** A new `internal/relay/rlsdb` package encapsulates `*gorm.DB` behind `Tx` (household-scoped transaction) and `WithoutHousehold` (explicit bypass). `PgStore` replaces its `db *gorm.DB` field with `rls *rlsdb.DB`. RLS policies on `ops` and `blobs` enforce `household_id = current_setting('app.household_id')`. Four excluded tables (`households`, `devices`, `invites`, `key_exchanges`) keep application-level checks.

**Tech Stack:** Go, GORM, Postgres RLS, testify

**Spec:** `plans/relay-row-level-security.md`

---

### Task 1: Create `rlsdb` package

**Files:**
- Create: `internal/relay/rlsdb/rlsdb.go`

- [ ] **Step 1: Write the package**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Package rlsdb encapsulates database access behind row-level security
// aware transaction wrappers.
//
// The raw *gorm.DB is unexported, making it structurally inaccessible
// from outside this package at compile time. All database queries must
// go through Tx (household-scoped) or WithoutHousehold (explicit bypass).
package rlsdb

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// DB wraps a *gorm.DB and enforces that all queries go through an
// RLS-scoped transaction.
type DB struct {
	raw *gorm.DB
}

// RLSTable specifies a table name and which column holds the household ID
// for row-level security policy creation.
type RLSTable struct {
	Name   string
	Column string
}

// New wraps a *gorm.DB in an RLS-aware wrapper.
func New(db *gorm.DB) *DB {
	return &DB{raw: db}
}

// Tx opens a transaction scoped to the given household.
// All store methods use this as the standard database access path.
func (d *DB) Tx(ctx context.Context, householdID string, fn func(tx *gorm.DB) error) error {
	return d.raw.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.household_id', ?, true)", householdID).Error; err != nil {
			return fmt.Errorf("set app.household_id: %w", err)
		}
		return fn(tx)
	})
}

// WithoutHousehold opens a transaction without household scoping.
//
// SAFETY: This bypasses row-level security. The caller is responsible for
// ensuring the operation is inherently cross-household (e.g., device
// authentication by token hash). Adding a new call site requires review
// and a comment explaining why household scoping is impossible.
func (d *DB) WithoutHousehold(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return d.raw.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
}

// Migrate runs GORM AutoMigrate. Construction-time only.
func (d *DB) Migrate(models ...any) error {
	return d.raw.AutoMigrate(models...)
}

// InitRLS enables row-level security and creates isolation policies for
// the given tables. Idempotent. Construction-time only.
//
// For each table:
//  1. ENABLE ROW LEVEL SECURITY
//  2. FORCE ROW LEVEL SECURITY (policies apply even to table owner)
//  3. DROP + CREATE policy enforcing column = current_setting('app.household_id')
func (d *DB) InitRLS(tables []RLSTable) error {
	for _, t := range tables {
		stmts := []string{
			fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY", t.Name),
			fmt.Sprintf("ALTER TABLE %s FORCE ROW LEVEL SECURITY", t.Name),
			fmt.Sprintf("DROP POLICY IF EXISTS %s_household_isolation ON %s", t.Name, t.Name),
			fmt.Sprintf(
				"CREATE POLICY %s_household_isolation ON %s "+
					"USING (%s = current_setting('app.household_id')) "+
					"WITH CHECK (%s = current_setting('app.household_id'))",
				t.Name, t.Name, t.Column, t.Column,
			),
		}
		for _, sql := range stmts {
			if err := d.raw.Exec(sql).Error; err != nil {
				return fmt.Errorf("init RLS on %s: %w", t.Name, err)
			}
		}
	}
	return nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	sqlDB, err := d.raw.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/relay/rlsdb/`
Expected: success, no errors

- [ ] **Step 3: Commit**

```
feat(relay): add rlsdb package for RLS-aware database access
```

---

### Task 2: Test the `rlsdb` package

**Files:**
- Create: `internal/relay/rlsdb/rlsdb_test.go`

- [ ] **Step 1: Write wrapper unit tests**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package rlsdb_test

import (
	"context"
	"os"
	"testing"

	"github.com/micasa-dev/micasa/internal/relay/rlsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestDB(t *testing.T) *rlsdb.DB {
	t.Helper()
	dsn := os.Getenv("RELAY_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	raw, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, _ := raw.DB()
		_ = sqlDB.Close()
	})
	return rlsdb.New(raw)
}

func TestTxSetsHouseholdID(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	err := db.Tx(ctx, "hh-123", func(tx *gorm.DB) error {
		var value string
		return tx.Raw("SELECT current_setting('app.household_id')").Scan(&value).Error
	})
	require.NoError(t, err)
}

func TestTxSetsCorrectValue(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	var got string
	err := db.Tx(ctx, "test-household-abc", func(tx *gorm.DB) error {
		return tx.Raw("SELECT current_setting('app.household_id')").Scan(&got).Error
	})
	require.NoError(t, err)
	assert.Equal(t, "test-household-abc", got)
}

func TestWithoutHouseholdDoesNotSetSetting(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	var isNull bool
	err := db.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		// current_setting with missing_ok=true returns NULL when unset.
		return tx.Raw("SELECT current_setting('app.household_id', true) IS NULL").Scan(&isNull).Error
	})
	require.NoError(t, err)
	assert.True(t, isNull)
}

func TestTxPropagatesCallbackError(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	err := db.Tx(ctx, "hh-123", func(_ *gorm.DB) error {
		return assert.AnError
	})
	require.ErrorIs(t, err, assert.AnError)
}

func TestWithoutHouseholdPropagatesCallbackError(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	err := db.WithoutHousehold(ctx, func(_ *gorm.DB) error {
		return assert.AnError
	})
	require.ErrorIs(t, err, assert.AnError)
}

func TestTxSettingIsolatedBetweenCalls(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	// First transaction sets "household-A".
	var first string
	err := db.Tx(ctx, "household-A", func(tx *gorm.DB) error {
		return tx.Raw("SELECT current_setting('app.household_id')").Scan(&first).Error
	})
	require.NoError(t, err)
	assert.Equal(t, "household-A", first)

	// Second transaction sets "household-B" — no leak from first.
	var second string
	err = db.Tx(ctx, "household-B", func(tx *gorm.DB) error {
		return tx.Raw("SELECT current_setting('app.household_id')").Scan(&second).Error
	})
	require.NoError(t, err)
	assert.Equal(t, "household-B", second)
}

func TestTxContextCancellation(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := db.Tx(ctx, "hh-123", func(_ *gorm.DB) error {
		return nil
	})
	require.Error(t, err)
}

func TestInitRLS(t *testing.T) {
	db := openTestDB(t)

	// Create a temporary test table.
	require.NoError(t, db.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE IF NOT EXISTS rlsdb_test_table (id TEXT PRIMARY KEY, household_id TEXT NOT NULL)").Error
	}))
	t.Cleanup(func() {
		_ = db.WithoutHousehold(context.Background(), func(tx *gorm.DB) error {
			return tx.Exec("DROP TABLE IF EXISTS rlsdb_test_table").Error
		})
	})

	// InitRLS should succeed.
	err := db.InitRLS([]rlsdb.RLSTable{
		{Name: "rlsdb_test_table", Column: "household_id"},
	})
	require.NoError(t, err)

	// Insert a row with household "A".
	require.NoError(t, db.Tx(t.Context(), "A", func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO rlsdb_test_table (id, household_id) VALUES ('row-1', 'A')").Error
	}))

	// Household "A" can see the row.
	var countA int64
	require.NoError(t, db.Tx(t.Context(), "A", func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM rlsdb_test_table").Scan(&countA).Error
	}))
	assert.Equal(t, int64(1), countA)

	// Household "B" cannot see the row.
	var countB int64
	require.NoError(t, db.Tx(t.Context(), "B", func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM rlsdb_test_table").Scan(&countB).Error
	}))
	assert.Equal(t, int64(0), countB)
}

func TestInitRLSIdempotent(t *testing.T) {
	db := openTestDB(t)

	require.NoError(t, db.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE IF NOT EXISTS rlsdb_test_idem (id TEXT PRIMARY KEY, household_id TEXT NOT NULL)").Error
	}))
	t.Cleanup(func() {
		_ = db.WithoutHousehold(context.Background(), func(tx *gorm.DB) error {
			return tx.Exec("DROP TABLE IF EXISTS rlsdb_test_idem").Error
		})
	})

	tables := []rlsdb.RLSTable{{Name: "rlsdb_test_idem", Column: "household_id"}}
	require.NoError(t, db.InitRLS(tables))
	require.NoError(t, db.InitRLS(tables)) // second call should be idempotent
}

func TestWithoutHouseholdQueryingRLSTableFails(t *testing.T) {
	db := openTestDB(t)

	require.NoError(t, db.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE IF NOT EXISTS rlsdb_test_failsafe (id TEXT PRIMARY KEY, household_id TEXT NOT NULL)").Error
	}))
	t.Cleanup(func() {
		_ = db.WithoutHousehold(context.Background(), func(tx *gorm.DB) error {
			tx.Exec("ALTER TABLE rlsdb_test_failsafe DISABLE ROW LEVEL SECURITY")
			return tx.Exec("DROP TABLE IF EXISTS rlsdb_test_failsafe").Error
		})
	})

	require.NoError(t, db.InitRLS([]rlsdb.RLSTable{
		{Name: "rlsdb_test_failsafe", Column: "household_id"},
	}))

	// Insert via Tx (works).
	require.NoError(t, db.Tx(t.Context(), "test-hh", func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO rlsdb_test_failsafe (id, household_id) VALUES ('r1', 'test-hh')").Error
	}))

	// Query via WithoutHousehold should fail (current_setting throws).
	err := db.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
		var count int64
		return tx.Raw("SELECT count(*) FROM rlsdb_test_failsafe").Scan(&count).Error
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "app.household_id")
}

func TestInsertMismatchedHouseholdIDFails(t *testing.T) {
	db := openTestDB(t)

	require.NoError(t, db.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE IF NOT EXISTS rlsdb_test_mismatch (id TEXT PRIMARY KEY, household_id TEXT NOT NULL)").Error
	}))
	t.Cleanup(func() {
		_ = db.WithoutHousehold(context.Background(), func(tx *gorm.DB) error {
			tx.Exec("ALTER TABLE rlsdb_test_mismatch DISABLE ROW LEVEL SECURITY")
			return tx.Exec("DROP TABLE IF EXISTS rlsdb_test_mismatch").Error
		})
	})

	require.NoError(t, db.InitRLS([]rlsdb.RLSTable{
		{Name: "rlsdb_test_mismatch", Column: "household_id"},
	}))

	// Tx sets household "A", but INSERT has household_id "B" — WITH CHECK fails.
	err := db.Tx(t.Context(), "A", func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO rlsdb_test_mismatch (id, household_id) VALUES ('r1', 'B')").Error
	})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they pass (requires RELAY_POSTGRES_DSN)**

Run: `go test ./internal/relay/rlsdb/ -v -count=1`
Expected: all tests pass (or skip if RELAY_POSTGRES_DSN unset)

- [ ] **Step 3: Commit**

```
test(relay): add rlsdb wrapper unit tests
```

---

### Task 3: Migrate PgStore to use `rlsdb`

**Files:**
- Modify: `internal/relay/pgstore.go`

This is the largest task. It changes the PgStore struct and every method.

- [ ] **Step 1: Update PgStore struct and constructors**

Replace the struct and constructors:

```go
// Before:
type PgStore struct {
	db            *gorm.DB
	encryptionKey []byte
}

// After:
type PgStore struct {
	rls           *rlsdb.DB
	encryptionKey []byte
}
```

Add the import: `"github.com/micasa-dev/micasa/internal/relay/rlsdb"`

Replace `OpenPgStore`:

```go
// rlsTables defines the tables that have RLS policies.
var rlsTables = []rlsdb.RLSTable{
	{Name: "ops", Column: "household_id"},
	{Name: "blobs", Column: "household_id"},
}

func OpenPgStore(dsn string) (*PgStore, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	rls := rlsdb.New(db)
	return &PgStore{rls: rls}, nil
}
```

Replace `NewPgStore`:

```go
func NewPgStore(rls *rlsdb.DB) *PgStore {
	return &PgStore{rls: rls}
}
```

Replace `AutoMigrate`:

```go
func (s *PgStore) AutoMigrate() error {
	if err := s.rls.Migrate(pgModels...); err != nil {
		return err
	}
	if err := s.rls.InitRLS(rlsTables); err != nil {
		return err
	}
	// Partial unique index: only enforce uniqueness for non-empty customer IDs.
	return s.rls.WithoutHousehold(context.Background(), func(tx *gorm.DB) error {
		if err := tx.Exec(`DROP INDEX IF EXISTS idx_households_stripe_customer_id`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			CREATE UNIQUE INDEX idx_households_stripe_customer_id
			ON households (stripe_customer_id)
			WHERE stripe_customer_id IS NOT NULL
		`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DROP INDEX IF EXISTS idx_ops_dedup`).Error; err != nil {
			return err
		}
		return tx.Exec(`
			CREATE UNIQUE INDEX idx_ops_dedup
			ON ops (id, household_id)
		`).Error
	})
}
```

Replace `Close` to delegate to rlsdb:

```go
func (s *PgStore) Close() error {
	return s.rls.Close()
}
```

- [ ] **Step 2: Migrate all Tx methods**

For each method currently using `s.db.WithContext(ctx).Transaction(...)`, replace with `s.rls.Tx(ctx, householdID, ...)`.

For each method currently using `s.db.WithContext(ctx).<query>` (no transaction), wrap in `s.rls.Tx(ctx, householdID, ...)`.

**Push** — derives householdID from `ops[0].HouseholdID`, early return for empty:

```go
func (s *PgStore) Push(ctx context.Context, ops []sync.Envelope) ([]sync.PushConfirmation, error) {
	if len(ops) == 0 {
		return nil, nil
	}

	confirmed := make([]sync.PushConfirmation, 0, len(ops))

	err := s.rls.Tx(ctx, ops[0].HouseholdID, func(tx *gorm.DB) error {
		// ... existing loop body unchanged ...
	})
	// ... rest unchanged ...
}
```

**Pull** — wrap in Tx (was unwrapped):

```go
func (s *PgStore) Pull(
	ctx context.Context,
	householdID, excludeDeviceID string,
	afterSeq int64,
	limit int,
) ([]sync.Envelope, bool, error) {
	if limit <= 0 {
		limit = 100
	}

	var rows []pgOp
	var hasMore bool

	err := s.rls.Tx(ctx, householdID, func(tx *gorm.DB) error {
		q := tx.Where("household_id = ? AND seq > ?", householdID, afterSeq)
		if excludeDeviceID != "" {
			q = q.Where("device_id != ?", excludeDeviceID)
		}
		if err := q.Order("seq ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
			return fmt.Errorf("pull ops: %w", err)
		}
		hasMore = len(rows) > limit
		if hasMore {
			rows = rows[:limit]
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}

	envs := make([]sync.Envelope, len(rows))
	for i, r := range rows {
		envs[i] = sync.Envelope{
			ID: r.ID, HouseholdID: r.HouseholdID, DeviceID: r.DeviceID,
			Nonce: r.Nonce, Ciphertext: r.Ciphertext, CreatedAt: r.CreatedAt, Seq: r.Seq,
		}
	}
	return envs, hasMore, nil
}
```

**CreateHousehold** — generate UUID before Tx:

```go
func (s *PgStore) CreateHousehold(
	ctx context.Context,
	req sync.CreateHouseholdRequest,
) (sync.CreateHouseholdResponse, error) {
	hhID := uid.New()
	devID := uid.New()
	token, tokenHash, err := generateToken()
	if err != nil {
		return sync.CreateHouseholdResponse{}, err
	}

	err = s.rls.Tx(ctx, hhID, func(tx *gorm.DB) error {
		// ... existing body unchanged, using hhID and devID ...
	})
	// ... rest unchanged ...
}
```

Apply the same pattern to every remaining method. The transformation is mechanical:

- `s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {` → `s.rls.Tx(ctx, householdID, func(tx *gorm.DB) error {`
- `s.db.WithContext(ctx).<query>` → wrap in `s.rls.Tx(ctx, householdID, func(tx *gorm.DB) error { return tx.<query> })`

Methods that need this transformation (all use `Tx`):
- `RegisterDevice` — householdID from `req.HouseholdID`
- `CreateInvite` — householdID from parameter
~~`StartJoin`~~ — moved to WithoutHousehold (see Step 3)
- `CompleteKeyExchange` — householdID from parameter
- `GetPendingExchanges` — householdID from parameter
- `ListDevices` — householdID from parameter
- `RevokeDevice` — householdID from parameter
- `GetHousehold` — householdID from parameter
- `UpdateSubscription` — householdID from parameter
- `UpdateCustomerID` — householdID from parameter
- `OpsCount` — householdID from parameter
- `PutBlob` — householdID from parameter
- `GetBlob` — householdID from parameter
- `HasBlob` — householdID from parameter
- `BlobUsage` — householdID from parameter

- [ ] **Step 3: Migrate WithoutHousehold methods**

**AuthenticateDevice**:

```go
func (s *PgStore) AuthenticateDevice(ctx context.Context, token string) (sync.Device, error) {
	sha := tokenSHA256(token)

	var dev pgDevice
	// SAFETY: Authentication discovers which household a token belongs to.
	// The household ID is unknown before this query completes.
	err := s.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		result := tx.Raw(
			"UPDATE devices SET last_seen = now() "+
				"WHERE token_sha = ? AND revoked = false "+
				"RETURNING *",
			sha,
		).Scan(&dev)
		if result.Error != nil {
			return fmt.Errorf("authenticate: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("invalid token")
		}
		return nil
	})
	if err != nil {
		return sync.Device{}, err
	}
	return pgDeviceToSync(dev), nil
}
```

**GetKeyExchangeResult** — uses `WithoutHousehold` because the Store interface has no householdID parameter:

```go
func (s *PgStore) GetKeyExchangeResult(ctx context.Context, exchangeID string) (sync.KeyExchangeResult, error) {
	var result sync.KeyExchangeResult
	// SAFETY: Polled by an unauthenticated joiner before they receive a
	// device token. The Store interface takes only exchangeID, no
	// householdID. Security relies on exchange IDs being 256-bit random.
	err := s.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		// ... existing transaction body unchanged (already uses tx) ...
	})
	return result, err
}
```

**StartJoin** — unauthenticated endpoint, household ID from URL is attacker-controlled:

```go
func (s *PgStore) StartJoin(
	ctx context.Context,
	householdID, code string,
	req sync.JoinRequest,
) (sync.JoinResponse, error) {
	// SAFETY: The household ID comes from an unauthenticated URL path
	// (attacker-controlled). Using Tx would set the RLS scope to an
	// unvalidated value. None of StartJoin's tables have RLS, and the
	// application-level check validates the household after the invite
	// is fetched.
	var resp sync.JoinResponse
	err := s.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		// ... existing transaction body unchanged (already uses tx) ...
	})
	return resp, err
}
```

**HouseholdBySubscription**:

```go
func (s *PgStore) HouseholdBySubscription(
	ctx context.Context,
	subscriptionID string,
) (sync.Household, error) {
	var hh pgHousehold
	// SAFETY: Stripe webhook handler looks up household by subscription ID.
	// The household ID is unknown; only the Stripe subscription ID is available.
	err := s.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Where("stripe_subscription_id = ?", subscriptionID).First(&hh).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sync.Household{}, fmt.Errorf("no household with subscription %s", subscriptionID)
		}
		return sync.Household{}, fmt.Errorf("find household by subscription: %w", err)
	}
	return pgHouseholdToSync(hh), nil
}
```

**HouseholdByCustomer**:

```go
func (s *PgStore) HouseholdByCustomer(
	ctx context.Context,
	customerID string,
) (sync.Household, error) {
	var h pgHousehold
	// SAFETY: Stripe webhook handler looks up household by customer ID.
	// The household ID is unknown; only the Stripe customer ID is available.
	err := s.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Where("stripe_customer_id = ?", customerID).First(&h).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sync.Household{}, fmt.Errorf("no household with customer %q", customerID)
		}
		return sync.Household{}, fmt.Errorf("household by customer %q: %w", customerID, err)
	}
	return pgHouseholdToSync(h), nil
}
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./internal/relay/...`
Expected: success

- [ ] **Step 5: Commit**

```
feat(relay): migrate PgStore to rlsdb for RLS-scoped database access
```

---

### Task 4: Update test infrastructure

**Files:**
- Modify: `internal/relay/pgstore_test.go`
- Modify: `internal/relay/suite_helpers_test.go`

- [ ] **Step 1: Update `openTestPgStore`**

```go
func openTestPgStore(t *testing.T) *PgStore {
	t.Helper()
	dsn := os.Getenv("RELAY_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("RELAY_POSTGRES_DSN not set, skipping Postgres integration test")
	}

	store, err := OpenPgStore(dsn)
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate())

	// TRUNCATE is DDL — unaffected by RLS policies.
	ctx := t.Context()
	require.NoError(t, store.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		for _, m := range pgModels {
			tabler, ok := m.(schema.Tabler)
			require.True(t, ok, "model %T must implement schema.Tabler", m)
			if err := tx.Exec("TRUNCATE " + tabler.TableName() + " CASCADE").Error; err != nil {
				return err
			}
		}
		return nil
	}))

	store.SetEncryptionKey(defaultTestEncryptionKey)
	t.Cleanup(func() { _ = store.Close() })
	return store
}
```

Remove the `gorm.io/driver/postgres` and `gorm.io/gorm/logger` imports from pgstore_test.go if they become unused (OpenPgStore handles those now). Keep `gorm.io/gorm/schema` for the Tabler interface.

- [ ] **Step 2: Update `expireInvite` in suite_helpers_test.go**

Replace the PgStore case:

```go
case *PgStore:
	require.NoError(t, s.rls.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
		return tx.Exec(
			"UPDATE invites SET expires_at = ? WHERE code = ?",
			time.Now().Add(-time.Hour), code,
		).Error
	}))
```

- [ ] **Step 3: Update `expireKeyExchange` in suite_helpers_test.go**

Replace the PgStore case:

```go
case *PgStore:
	require.NoError(t, s.rls.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
		return tx.Exec(
			"UPDATE key_exchanges SET created_at = ? WHERE id = ?",
			past, exchangeID,
		).Error
	}))
```

- [ ] **Step 4: Run all relay tests**

Run: `go test -shuffle=on ./internal/relay/...`
Expected: all tests pass (PgStore tests skip without RELAY_POSTGRES_DSN)

- [ ] **Step 5: Commit**

```
test(relay): update test infrastructure for rlsdb migration
```

---

### Task 5: Add RLS policy enforcement tests

**Files:**
- Create: `internal/relay/rls_test.go`

These tests verify the RLS policies on `ops` and `blobs` tables directly.

- [ ] **Step 1: Write RLS-specific tests**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"os"
	"testing"

	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRLSOpsIsolation(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	// Push ops to household 1.
	_, err := store.Push(ctx, []sync.Envelope{{
		ID: "rls-op-1", HouseholdID: hh1.HouseholdID,
		DeviceID: hh1.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}})
	require.NoError(t, err)

	// Direct query within household 1's RLS scope sees the op.
	var count1 int64
	require.NoError(t, store.rls.Tx(ctx, hh1.HouseholdID, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM ops").Scan(&count1).Error
	}))
	assert.Equal(t, int64(1), count1)

	// Direct query within household 2's RLS scope sees nothing.
	var count2 int64
	require.NoError(t, store.rls.Tx(ctx, hh2.HouseholdID, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM ops").Scan(&count2).Error
	}))
	assert.Equal(t, int64(0), count2)
}

func TestRLSBlobIsolation(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	require.NoError(t, store.PutBlob(ctx, hh1.HouseholdID, hash, []byte("secret"), 0))

	// Direct query within household 1's RLS scope sees the blob.
	var count1 int64
	require.NoError(t, store.rls.Tx(ctx, hh1.HouseholdID, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM blobs").Scan(&count1).Error
	}))
	assert.Equal(t, int64(1), count1)

	// Direct query within household 2's RLS scope sees nothing.
	var count2 int64
	require.NoError(t, store.rls.Tx(ctx, hh2.HouseholdID, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM blobs").Scan(&count2).Error
	}))
	assert.Equal(t, int64(0), count2)
}

func TestRLSFailsafeOnOps(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)

	// WithoutHousehold + query on RLS-enabled table should error.
	err := store.rls.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
		var count int64
		return tx.Raw("SELECT count(*) FROM ops").Scan(&count).Error
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "app.household_id")
}

func TestRLSFailsafeOnBlobs(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)

	err := store.rls.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
		var count int64
		return tx.Raw("SELECT count(*) FROM blobs").Scan(&count).Error
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "app.household_id")
}

func TestRLSNonRLSTablesUnaffected(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)

	// WithoutHousehold + query on non-RLS tables should work fine.
	tables := []string{"households", "devices", "invites", "key_exchanges"}
	for _, table := range tables {
		err := store.rls.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
			var count int64
			return tx.Raw("SELECT count(*) FROM " + table).Scan(&count).Error
		})
		require.NoError(t, err, "query on non-RLS table %s should succeed without household_id", table)
	}
}

func TestRLSInsertMismatchedHouseholdID(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	// Set RLS to household A, but insert an op with household B.
	err := store.rls.Tx(ctx, hh.HouseholdID, func(tx *gorm.DB) error {
		return tx.Exec(
			"INSERT INTO ops (seq, household_id, id, device_id, nonce, ciphertext) "+
				"VALUES (999, 'wrong-household', 'rls-mismatch', ?, 'n', 'c')",
			hh.DeviceID,
		).Error
	})
	require.Error(t, err, "INSERT with mismatched household_id should violate WITH CHECK")
}

func TestRLSForceAppliesToOwner(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	// Push an op.
	_, err := store.Push(ctx, []sync.Envelope{{
		ID: "force-test", HouseholdID: hh.HouseholdID,
		DeviceID: hh.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}})
	require.NoError(t, err)

	// Even though the app connects as the table owner, FORCE RLS
	// means the query without SET LOCAL should fail.
	err = store.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		var count int64
		return tx.Raw("SELECT count(*) FROM ops").Scan(&count).Error
	})
	require.Error(t, err, "FORCE ROW LEVEL SECURITY should prevent owner bypass")
}
```

- [ ] **Step 2: Run RLS tests**

Run: `go test ./internal/relay/ -run TestRLS -v -count=1`
Expected: all pass (or skip without RELAY_POSTGRES_DSN)

- [ ] **Step 3: Commit**

```
test(relay): add RLS policy enforcement tests
```

---

### Task 6: Update AGENTS.md

**Files:**
- Modify: `AGENTS.md`

- [ ] **Step 1: Add hard rule under "Architecture and code style"**

Add after the existing `- **Context lifecycle**:` entry:

```markdown
- **All relay Postgres access goes through `rlsdb.DB.Tx`**: Every PgStore
  method that touches the database MUST use `s.rls.Tx(ctx, householdID, fn)`.
  This is not a guideline -- it is the ONLY way to obtain a `*gorm.DB` for
  queries. The `rlsdb` package enforces this structurally: the raw `*gorm.DB`
  is unexported and inaccessible from the `relay` package. Do NOT:
  - Store a `*gorm.DB` reference on `PgStore`
  - Pass a `*gorm.DB` through context values
  - Create a second `gorm.Open` connection
  - Import `rlsdb` internals via `unsafe` or reflection
  If a method genuinely cannot know the household ID or receives it from
  an untrusted source, use `s.rls.WithoutHousehold(ctx, fn)` with a
  `// SAFETY:` comment explaining why. The approved call sites are:
  `AuthenticateDevice` (token hash lookup), `GetKeyExchangeResult`
  (unauthenticated exchange poll), `StartJoin` (unauthenticated endpoint,
  household ID from URL is attacker-controlled), `HouseholdBySubscription`
  (Stripe webhook), and `HouseholdByCustomer` (Stripe webhook). New
  `WithoutHousehold` call sites require explicit user approval before
  implementation.
```

- [ ] **Step 2: Commit**

```
docs(relay): add rlsdb hard rule to AGENTS.md [skip ci]
```

---

### Task 7: Final verification

- [ ] **Step 1: Run full relay test suite**

Run: `go test -shuffle=on ./internal/relay/...`
Expected: all tests pass

- [ ] **Step 2: Run full project build**

Run: `go build ./...`
Expected: success

- [ ] **Step 3: Run linter**

Run: `golangci-lint run ./internal/relay/...`
Expected: no warnings

- [ ] **Step 4: Verify coverage of new code**

Run: `go test -coverprofile=cover.out ./internal/relay/... && go tool cover -func cover.out | rg rlsdb`
Expected: rlsdb package functions have test coverage
