// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package rlsdb_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/micasa-dev/micasa/internal/relay/rlsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openTestDB opens a GORM connection and wraps it in an rlsdb.DB.
// It skips the test if RELAY_POSTGRES_DSN is not set.
func openTestDB(t *testing.T) *rlsdb.DB {
	t.Helper()
	dsn := os.Getenv("RELAY_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("RELAY_POSTGRES_DSN not set, skipping Postgres integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	wrapped := rlsdb.New(db)
	t.Cleanup(func() { _ = wrapped.Close() })
	return wrapped
}

func TestTxSetsHouseholdID(t *testing.T) {
	d := openTestDB(t)
	ctx := t.Context()

	err := d.Tx(ctx, "test-household", func(tx *gorm.DB) error {
		var val string
		return tx.Raw("SELECT current_setting('app.household_id')").Scan(&val).Error
	})
	require.NoError(t, err)
}

func TestTxSetsCorrectValue(t *testing.T) {
	d := openTestDB(t)
	ctx := t.Context()

	var got string
	err := d.Tx(ctx, "test-household-abc", func(tx *gorm.DB) error {
		return tx.Raw("SELECT current_setting('app.household_id')").Scan(&got).Error
	})
	require.NoError(t, err)
	assert.Equal(t, "test-household-abc", got)
}

func TestWithoutHouseholdClearsHouseholdID(t *testing.T) {
	d := openTestDB(t)
	ctx := t.Context()

	// WithoutHousehold explicitly clears app.household_id to empty string.
	var value string
	err := d.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Raw("SELECT current_setting('app.household_id', true)").
			Scan(&value).
			Error
	})
	require.NoError(t, err)
	assert.Empty(t, value)
}

func TestTxPropagatesCallbackError(t *testing.T) {
	d := openTestDB(t)
	ctx := t.Context()

	err := d.Tx(ctx, "test-household", func(_ *gorm.DB) error {
		return assert.AnError
	})
	require.ErrorIs(t, err, assert.AnError)
}

func TestWithoutHouseholdPropagatesCallbackError(t *testing.T) {
	d := openTestDB(t)
	ctx := t.Context()

	err := d.WithoutHousehold(ctx, func(_ *gorm.DB) error {
		return assert.AnError
	})
	require.ErrorIs(t, err, assert.AnError)
}

func TestTxSettingIsolatedBetweenCalls(t *testing.T) {
	d := openTestDB(t)
	ctx := t.Context()

	var first, second string

	err := d.Tx(ctx, "household-one", func(tx *gorm.DB) error {
		return tx.Raw("SELECT current_setting('app.household_id')").Scan(&first).Error
	})
	require.NoError(t, err)

	err = d.Tx(ctx, "household-two", func(tx *gorm.DB) error {
		return tx.Raw("SELECT current_setting('app.household_id')").Scan(&second).Error
	})
	require.NoError(t, err)

	assert.Equal(t, "household-one", first)
	assert.Equal(t, "household-two", second)
}

func TestTxContextCancellation(t *testing.T) {
	d := openTestDB(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := d.Tx(ctx, "test-household", func(_ *gorm.DB) error {
		return nil
	})
	require.Error(t, err)
}

func TestInitRLS(t *testing.T) {
	d := openTestDB(t)
	ctx := t.Context()

	table := "rlsdb_test_init_rls"

	// Clean up stale state from interrupted prior runs, then create.
	err := d.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec("ALTER TABLE IF EXISTS " + table + " DISABLE ROW LEVEL SECURITY").Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE IF EXISTS " + table).Error; err != nil {
			return err
		}
		return tx.Exec(
			"CREATE TABLE " + table + " (id TEXT PRIMARY KEY, household_id TEXT NOT NULL)",
		).Error
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = d.WithoutHousehold(context.Background(), func(tx *gorm.DB) error {
			_ = tx.Exec("ALTER TABLE " + table + " DISABLE ROW LEVEL SECURITY").Error
			return tx.Exec("DROP TABLE IF EXISTS " + table).Error
		})
	})

	// Enable RLS.
	require.NoError(t, d.InitRLS([]rlsdb.RLSTable{{Name: table, Column: "household_id"}}))

	// Insert a row scoped to household "A".
	err = d.Tx(ctx, "A", func(tx *gorm.DB) error {
		return tx.Exec(fmt.Sprintf(
			"INSERT INTO %s (id, household_id) VALUES ('row-a', 'A')", table,
		)).Error
	})
	require.NoError(t, err)

	// Tx("A") should see exactly 1 row.
	var countA int64
	err = d.Tx(ctx, "A", func(tx *gorm.DB) error {
		return tx.Raw("SELECT COUNT(*) FROM " + table).Scan(&countA).Error
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), countA)

	// Tx("B") should see 0 rows.
	var countB int64
	err = d.Tx(ctx, "B", func(tx *gorm.DB) error {
		return tx.Raw("SELECT COUNT(*) FROM " + table).Scan(&countB).Error
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), countB)
}

func TestInitRLSIdempotent(t *testing.T) {
	d := openTestDB(t)
	ctx := t.Context()

	table := "rlsdb_test_idempotent"

	err := d.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec("ALTER TABLE IF EXISTS " + table + " DISABLE ROW LEVEL SECURITY").Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE IF EXISTS " + table).Error; err != nil {
			return err
		}
		return tx.Exec(
			"CREATE TABLE " + table + " (id TEXT PRIMARY KEY, household_id TEXT NOT NULL)",
		).Error
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = d.WithoutHousehold(context.Background(), func(tx *gorm.DB) error {
			_ = tx.Exec("ALTER TABLE " + table + " DISABLE ROW LEVEL SECURITY").Error
			return tx.Exec("DROP TABLE IF EXISTS " + table).Error
		})
	})

	tables := []rlsdb.RLSTable{{Name: table, Column: "household_id"}}
	require.NoError(t, d.InitRLS(tables))
	require.NoError(t, d.InitRLS(tables))
}

func TestWithoutHouseholdQueryingRLSTableFails(t *testing.T) {
	d := openTestDB(t)
	ctx := t.Context()

	table := "rlsdb_test_woh_fails"

	err := d.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec("ALTER TABLE IF EXISTS " + table + " DISABLE ROW LEVEL SECURITY").Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE IF EXISTS " + table).Error; err != nil {
			return err
		}
		return tx.Exec(
			"CREATE TABLE " + table + " (id TEXT PRIMARY KEY, household_id TEXT NOT NULL)",
		).Error
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = d.WithoutHousehold(context.Background(), func(tx *gorm.DB) error {
			_ = tx.Exec("ALTER TABLE " + table + " DISABLE ROW LEVEL SECURITY").Error
			return tx.Exec("DROP TABLE IF EXISTS " + table).Error
		})
	})

	require.NoError(t, d.InitRLS([]rlsdb.RLSTable{{Name: table, Column: "household_id"}}))

	// Insert a row via Tx so there is data to query.
	err = d.Tx(ctx, "A", func(tx *gorm.DB) error {
		return tx.Exec(fmt.Sprintf(
			"INSERT INTO %s (id, household_id) VALUES ('row-a', 'A')", table,
		)).Error
	})
	require.NoError(t, err)

	// WithoutHousehold querying an RLS-enabled table must not leak data.
	// Two possible behaviors depending on connection state:
	//   1. GUC never set on this connection → current_setting throws → error
	//   2. GUC was set in a prior Tx → current_setting returns '' → zero rows
	// Both prevent data leakage.
	var count int64
	err = d.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Raw("SELECT COUNT(*) FROM " + table).Scan(&count).Error
	})
	if err != nil {
		assert.Contains(t, err.Error(), "app.household_id")
	} else {
		assert.Equal(t, int64(0), count, "WithoutHousehold must not see any rows")
	}
}

func TestInsertMismatchedHouseholdIDFails(t *testing.T) {
	d := openTestDB(t)
	ctx := t.Context()

	table := "rlsdb_test_mismatch"

	err := d.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		if err := tx.Exec("ALTER TABLE IF EXISTS " + table + " DISABLE ROW LEVEL SECURITY").Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE IF EXISTS " + table).Error; err != nil {
			return err
		}
		return tx.Exec(
			"CREATE TABLE " + table + " (id TEXT PRIMARY KEY, household_id TEXT NOT NULL)",
		).Error
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = d.WithoutHousehold(context.Background(), func(tx *gorm.DB) error {
			_ = tx.Exec("ALTER TABLE " + table + " DISABLE ROW LEVEL SECURITY").Error
			return tx.Exec("DROP TABLE IF EXISTS " + table).Error
		})
	})

	require.NoError(t, d.InitRLS([]rlsdb.RLSTable{{Name: table, Column: "household_id"}}))

	// Tx("A") inserting a row with household_id="B" violates the WITH CHECK policy.
	err = d.Tx(ctx, "A", func(tx *gorm.DB) error {
		return tx.Exec(fmt.Sprintf(
			"INSERT INTO %s (id, household_id) VALUES ('row-b', 'B')", table,
		)).Error
	})
	require.Error(t, err)
}

func TestTxEmptyHouseholdIDFails(t *testing.T) {
	// This test does not need a Postgres connection — the empty check
	// fires before the DB is touched.
	d := rlsdb.New(nil)

	err := d.Tx(t.Context(), "", func(_ *gorm.DB) error {
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}
