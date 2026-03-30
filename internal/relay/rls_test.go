// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"os"
	"testing"

	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/micasa-dev/micasa/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// requireRLSEnforced skips the test if the connected role has superuser or
// BYPASSRLS privileges, which bypass RLS regardless of policy configuration.
func requireRLSEnforced(t *testing.T, store *PgStore) {
	t.Helper()
	var isSuperOrBypass bool
	require.NoError(t, store.rls.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
		return tx.Raw(
			"SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user",
		).Scan(&isSuperOrBypass).Error
	}))
	if isSuperOrBypass {
		t.Skip("connected role bypasses RLS (superuser or BYPASSRLS)")
	}
}

// requireNonPrivilegedOwner skips the test if the connected role does not own
// the given table, or if it has superuser or BYPASSRLS privileges (which
// bypass RLS regardless of FORCE).
func requireNonPrivilegedOwner(t *testing.T, store *PgStore, table string) {
	t.Helper()
	ctx := t.Context()

	var isOwner bool
	require.NoError(t, store.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Raw(
			"SELECT tableowner = current_user FROM pg_tables WHERE schemaname = 'public' AND tablename = $1",
			table,
		).Scan(&isOwner).Error
	}))
	if !isOwner {
		t.Skipf("connected role does not own %s, cannot verify FORCE RLS", table)
	}

	var isSuperOrBypass bool
	require.NoError(t, store.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Raw(
			"SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user",
		).Scan(&isSuperOrBypass).Error
	}))
	if isSuperOrBypass {
		t.Skip("connected role is superuser or has BYPASSRLS, cannot verify FORCE RLS")
	}
}

// TestRLSOpsIsolation verifies that a household can only see its own ops rows.
func TestRLSOpsIsolation(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	_, err := store.Push(ctx, []sync.Envelope{{
		ID:          "op-rls-isolation",
		HouseholdID: hh1.HouseholdID,
		DeviceID:    hh1.DeviceID,
		Nonce:       []byte("n"),
		Ciphertext:  []byte("c"),
	}})
	require.NoError(t, err)

	var count1 int64
	require.NoError(t, store.rls.Tx(ctx, hh1.HouseholdID, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM ops").Scan(&count1).Error
	}))
	assert.Equal(t, int64(1), count1, "hh1 should see its own op")

	var count2 int64
	require.NoError(t, store.rls.Tx(ctx, hh2.HouseholdID, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM ops").Scan(&count2).Error
	}))
	assert.Equal(t, int64(0), count2, "hh2 should see no ops")
}

// TestRLSBlobIsolation verifies that a household can only see its own blobs rows.
func TestRLSBlobIsolation(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	require.NoError(t, store.PutBlob(ctx, hh1.HouseholdID, hash, []byte("data"), 0))

	var count1 int64
	require.NoError(t, store.rls.Tx(ctx, hh1.HouseholdID, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM blobs").Scan(&count1).Error
	}))
	assert.Equal(t, int64(1), count1, "hh1 should see its own blob")

	var count2 int64
	require.NoError(t, store.rls.Tx(ctx, hh2.HouseholdID, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM blobs").Scan(&count2).Error
	}))
	assert.Equal(t, int64(0), count2, "hh2 should see no blobs")
}

// TestRLSFailsafeOnOps verifies that querying ops without a household ID set
// returns an error — the failsafe policy blocks unrestricted access.
func TestRLSFailsafeOnOps(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	// Seed data so the test proves the failsafe, not just an empty table.
	hh := suiteCreateHousehold(t, store)
	_, err := store.Push(ctx, []sync.Envelope{{
		ID: "failsafe-op", HouseholdID: hh.HouseholdID,
		DeviceID: hh.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}})
	require.NoError(t, err)

	// Either current_setting throws (error) or returns '' and no rows match.
	// Both prevent data leakage.
	var count int64
	err = store.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM ops").Scan(&count).Error
	})
	if err != nil {
		assert.Contains(t, err.Error(), "app.household_id")
	} else {
		assert.Equal(t, int64(0), count, "WithoutHousehold must not see any ops")
	}
}

// TestRLSFailsafeOnBlobs verifies that querying blobs without a household ID set
// either errors or returns zero rows — no data leaks.
func TestRLSFailsafeOnBlobs(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)
	require.NoError(t, store.PutBlob(ctx, hh.HouseholdID,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		[]byte("failsafe-data"), 0))

	var count int64
	err := store.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM blobs").Scan(&count).Error
	})
	if err != nil {
		assert.Contains(t, err.Error(), "app.household_id")
	} else {
		assert.Equal(t, int64(0), count, "WithoutHousehold must not see any blobs")
	}
}

// TestRLSInsertViaWithoutHouseholdBlocked verifies that INSERT into RLS tables
// via WithoutHousehold fails (WITH CHECK rejects empty/null household scope).
func TestRLSInsertViaWithoutHouseholdBlocked(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	err := store.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Exec(
			"INSERT INTO ops (seq, household_id, id, device_id, nonce, ciphertext, created_at) "+
				"VALUES (99999, $1, 'unscoped-insert', $2, $3, $4, now())",
			hh.HouseholdID, hh.DeviceID, []byte("n"), []byte("c"),
		).Error
	})
	require.Error(t, err, "INSERT via WithoutHousehold should fail on WITH CHECK")
}

// TestRLSNonRLSTablesUnaffected verifies that tables without RLS can be queried
// without setting app.household_id.
func TestRLSNonRLSTablesUnaffected(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	ctx := t.Context()

	// Seed data so the test proves visibility, not just empty-table success.
	hh := suiteCreateHousehold(t, store)                           // populates households + devices
	_, err := store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID) // populates invites
	require.NoError(t, err)
	suiteJoinDevice(t, store, hh.HouseholdID, hh.DeviceID) // populates key_exchanges

	tables := []string{"households", "devices", "invites", "key_exchanges"}
	for _, table := range tables {
		t.Run(table, func(t *testing.T) {
			var count int64
			err := store.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
				return tx.Raw("SELECT count(*) FROM " + table).Scan(&count).Error
			})
			require.NoError(t, err, "table %s should not be protected by RLS", table)
			assert.Positive(
				t,
				count,
				"table %s should have visible rows without household_id",
				table,
			)
		})
	}
}

// TestRLSInsertMismatchedHouseholdID verifies that inserting a row with a
// household_id that doesn't match the session GUC is rejected by the WITH CHECK
// policy. Uses a real second household to avoid FK-related false passes.
func TestRLSInsertMismatchedHouseholdID(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	// Scope RLS to hh1, but insert with hh2's valid household_id.
	// Use uid.New() for the op ID and a unique seq to avoid collisions.
	err := store.rls.Tx(ctx, hh1.HouseholdID, func(tx *gorm.DB) error {
		return tx.Exec(
			"INSERT INTO ops (seq, household_id, id, device_id, nonce, ciphertext, created_at) "+
				"VALUES (2000000000, $1, $2, $3, $4, $5, now())",
			hh2.HouseholdID, uid.New(), hh1.DeviceID, []byte("n"), []byte("c"),
		).Error
	})
	require.Error(t, err, "INSERT with valid but mismatched household_id should fail on WITH CHECK")
	assert.Contains(t, err.Error(), "row-level security")
}

// TestRLSInsertMismatchedBlobHouseholdID mirrors the ops test for the blobs table.
func TestRLSInsertMismatchedBlobHouseholdID(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	err := store.rls.Tx(ctx, hh1.HouseholdID, func(tx *gorm.DB) error {
		return tx.Exec(
			"INSERT INTO blobs (household_id, hash, data, size_bytes) VALUES ($1, $2, $3, $4)",
			hh2.HouseholdID,
			"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			[]byte("data"),
			4,
		).Error
	})
	require.Error(t, err, "INSERT blob with mismatched household_id should fail on WITH CHECK")
	assert.Contains(t, err.Error(), "row-level security")
}

// TestRLSForceAppliesToOwner verifies that FORCE ROW LEVEL SECURITY prevents
// even the table owner from bypassing RLS when app.household_id is not set.
func TestRLSForceAppliesToOwner(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	ctx := t.Context()
	requireNonPrivilegedOwner(t, store, "ops")

	hh := suiteCreateHousehold(t, store)
	_, err := store.Push(ctx, []sync.Envelope{{
		ID:          "op-force-rls",
		HouseholdID: hh.HouseholdID,
		DeviceID:    hh.DeviceID,
		Nonce:       []byte("n"),
		Ciphertext:  []byte("c"),
	}})
	require.NoError(t, err)

	// Owner without GUC: either error or zero rows. Both prevent data leaks.
	var count int64
	err = store.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM ops").Scan(&count).Error
	})
	if err != nil {
		assert.Contains(t, err.Error(), "app.household_id")
	} else {
		assert.Equal(t, int64(0), count, "owner without GUC must not see ops")
	}
}

// TestRLSForceAppliesToOwnerBlobs mirrors the ops test for the blobs table.
func TestRLSForceAppliesToOwnerBlobs(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	ctx := t.Context()
	requireNonPrivilegedOwner(t, store, "blobs")

	hh := suiteCreateHousehold(t, store)
	require.NoError(t, store.PutBlob(ctx, hh.HouseholdID,
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		[]byte("data"), 0))

	var count int64
	err := store.rls.WithoutHousehold(ctx, func(tx *gorm.DB) error {
		return tx.Raw("SELECT count(*) FROM blobs").Scan(&count).Error
	})
	if err != nil {
		assert.Contains(t, err.Error(), "app.household_id")
	} else {
		assert.Equal(t, int64(0), count, "owner without GUC must not see blobs")
	}
}

// TestRLSUpdateCrossHousehold verifies that UPDATE across households affects
// zero rows (USING policy filters them out).
func TestRLSUpdateCrossHousehold(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	_, err := store.Push(ctx, []sync.Envelope{{
		ID: "update-target", HouseholdID: hh1.HouseholdID,
		DeviceID: hh1.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}})
	require.NoError(t, err)

	// Scoped to hh2, try to UPDATE hh1's op — USING policy hides it.
	var affected int64
	require.NoError(t, store.rls.Tx(ctx, hh2.HouseholdID, func(tx *gorm.DB) error {
		result := tx.Exec("UPDATE ops SET ciphertext = 'tampered' WHERE id = 'update-target'")
		affected = result.RowsAffected
		return result.Error
	}))
	assert.Equal(t, int64(0), affected, "UPDATE across households must affect zero rows")

	// Verify the op is unchanged.
	pulled, _, err := store.Pull(ctx, hh1.HouseholdID, "other", 0, 100)
	require.NoError(t, err)
	require.Len(t, pulled, 1)
	assert.Equal(t, []byte("c"), pulled[0].Ciphertext)
}

// TestRLSDeleteCrossHousehold verifies that DELETE across households affects
// zero rows (USING policy filters them out).
func TestRLSDeleteCrossHousehold(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	_, err := store.Push(ctx, []sync.Envelope{{
		ID: "delete-target", HouseholdID: hh1.HouseholdID,
		DeviceID: hh1.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}})
	require.NoError(t, err)

	// Scoped to hh2, try to DELETE hh1's op — USING policy hides it.
	var affected int64
	require.NoError(t, store.rls.Tx(ctx, hh2.HouseholdID, func(tx *gorm.DB) error {
		result := tx.Exec("DELETE FROM ops WHERE id = 'delete-target'")
		affected = result.RowsAffected
		return result.Error
	}))
	assert.Equal(t, int64(0), affected, "DELETE across households must affect zero rows")

	// Verify the op still exists.
	pulled, _, err := store.Pull(ctx, hh1.HouseholdID, "other", 0, 100)
	require.NoError(t, err)
	assert.Len(t, pulled, 1)
}

// TestRLSUpdateSetHouseholdIDBlocked verifies that a household cannot move
// its own rows to another household by UPDATE SET household_id. The WITH CHECK
// policy rejects the new value.
func TestRLSUpdateSetHouseholdIDBlocked(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	_, err := store.Push(ctx, []sync.Envelope{{
		ID: "move-target", HouseholdID: hh1.HouseholdID,
		DeviceID: hh1.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}})
	require.NoError(t, err)

	// Scoped to hh1 (owns the row), try to SET household_id to hh2.
	// WITH CHECK rejects because the new value doesn't match the scope.
	err = store.rls.Tx(ctx, hh1.HouseholdID, func(tx *gorm.DB) error {
		return tx.Exec(
			"UPDATE ops SET household_id = $1 WHERE id = 'move-target'",
			hh2.HouseholdID,
		).Error
	})
	require.Error(t, err, "UPDATE SET household_id to another household must fail")
	assert.Contains(t, err.Error(), "row-level security")
}

// TestRLSUpdateCrossHouseholdBlobs verifies that UPDATE across households
// affects zero rows on the blobs table.
func TestRLSUpdateCrossHouseholdBlobs(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	hash := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	require.NoError(t, store.PutBlob(ctx, hh1.HouseholdID, hash, []byte("original"), 0))

	var affected int64
	require.NoError(t, store.rls.Tx(ctx, hh2.HouseholdID, func(tx *gorm.DB) error {
		result := tx.Exec("UPDATE blobs SET data = 'tampered' WHERE hash = $1", hash)
		affected = result.RowsAffected
		return result.Error
	}))
	assert.Equal(t, int64(0), affected, "UPDATE blobs across households must affect zero rows")
}

// TestRLSDeleteCrossHouseholdBlobs verifies that DELETE across households
// affects zero rows on the blobs table.
func TestRLSDeleteCrossHouseholdBlobs(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	hash := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	require.NoError(t, store.PutBlob(ctx, hh1.HouseholdID, hash, []byte("data"), 0))

	var affected int64
	require.NoError(t, store.rls.Tx(ctx, hh2.HouseholdID, func(tx *gorm.DB) error {
		result := tx.Exec("DELETE FROM blobs WHERE hash = $1", hash)
		affected = result.RowsAffected
		return result.Error
	}))
	assert.Equal(t, int64(0), affected, "DELETE blobs across households must affect zero rows")

	exists, err := store.HasBlob(ctx, hh1.HouseholdID, hash)
	require.NoError(t, err)
	assert.True(t, exists, "blob should still exist after cross-household DELETE")
}

// TestRLSUpdateSetHouseholdIDBlobsBlocked verifies that a household cannot
// move its own blobs to another household.
func TestRLSUpdateSetHouseholdIDBlobsBlocked(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	requireRLSEnforced(t, store)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	hash := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	require.NoError(t, store.PutBlob(ctx, hh1.HouseholdID, hash, []byte("data"), 0))

	err := store.rls.Tx(ctx, hh1.HouseholdID, func(tx *gorm.DB) error {
		return tx.Exec(
			"UPDATE blobs SET household_id = $1 WHERE hash = $2",
			hh2.HouseholdID, hash,
		).Error
	})
	require.Error(t, err, "UPDATE SET household_id on blobs must fail")
	assert.Contains(t, err.Error(), "row-level security")
}
