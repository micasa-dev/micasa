// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLWWLocalWinsLaterTimestamp(t *testing.T) {
	t.Parallel()
	local := time.Now()
	remote := local.Add(-time.Minute) // remote is older
	assert.True(t, lwwLocalWins(local, "dev-a", remote, "dev-b"))
}

func TestLWWRemoteWinsLaterTimestamp(t *testing.T) {
	t.Parallel()
	local := time.Now()
	remote := local.Add(time.Minute) // remote is newer
	assert.False(t, lwwLocalWins(local, "dev-a", remote, "dev-b"))
}

func TestLWWTiebreakByDeviceID(t *testing.T) {
	t.Parallel()
	ts := time.Now()

	// Same timestamp, higher device_id wins.
	assert.True(t, lwwLocalWins(ts, "dev-z", ts, "dev-a"))
	assert.False(t, lwwLocalWins(ts, "dev-a", ts, "dev-z"))
}

func TestLWWTiebreakSameDevice(t *testing.T) {
	t.Parallel()
	ts := time.Now()
	// Same timestamp, same device -- local wins (>=).
	assert.True(t, lwwLocalWins(ts, "dev-a", ts, "dev-a"))
}

func TestStripNonColumnKeysRemovesBlobRef(t *testing.T) {
	t.Parallel()

	row := map[string]any{
		"id":        "doc-1",
		"title":     "Invoice",
		"file_name": "invoice.pdf",
		"sha256":    "abc123",
		"blob_ref":  "abc123",
	}
	stripNonColumnKeys("documents", row)
	assert.NotContains(t, row, "blob_ref", "blob_ref should be stripped from documents")
	assert.Contains(t, row, "sha256", "sha256 should be preserved")
	assert.Contains(t, row, "title", "other fields should be preserved")
}

func TestValidateInsertPayloadID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		row     map[string]any
		rowID   string
		wantErr string
	}{
		{
			name:    "matching ID passes",
			row:     map[string]any{"id": "vendor-1", "name": "Legit"},
			rowID:   "vendor-1",
			wantErr: "",
		},
		{
			name:    "mismatched ID rejected",
			row:     map[string]any{"id": "vendor-WRONG", "name": "Spoofed"},
			rowID:   "vendor-1",
			wantErr: "does not match",
		},
		{
			name:    "missing ID rejected",
			row:     map[string]any{"name": "NoID"},
			rowID:   "vendor-1",
			wantErr: "missing string id",
		},
		{
			name:    "non-string ID rejected",
			row:     map[string]any{"id": 42, "name": "NumericID"},
			rowID:   "vendor-1",
			wantErr: "missing string id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateInsertPayloadID(tt.row, tt.rowID)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestApplyOpsSortsBySeqBeforeApplying(t *testing.T) {
	t.Parallel()

	// Open a real SQLite DB so ApplyOps can run GORM transactions.
	dbPath := t.TempDir() + "/test.db"
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	require.NoError(t, store.AutoMigrate())

	db := store.GormDB()
	now := time.Now()
	vendorID := "vendor-sort-test"

	// Build two ops for the same vendor row, deliberately out of seq order.
	// Seq 20 (insert) should apply first, then seq 30 (update).
	// If ApplyOps doesn't sort, the update arrives before the insert and fails.
	ops := []DecryptedOp{
		{
			Envelope: Envelope{Seq: 30},
			Payload: OpPayload{
				ID:        "op-2",
				TableName: data.TableVendors,
				RowID:     vendorID,
				OpType:    "update",
				Payload:   `{"name":"Updated Name"}`,
				DeviceID:  "dev-a",
				CreatedAt: now.Add(time.Second),
			},
		},
		{
			Envelope: Envelope{Seq: 20},
			Payload: OpPayload{
				ID:        "op-1",
				TableName: data.TableVendors,
				RowID:     vendorID,
				OpType:    "insert",
				Payload:   `{"id":"` + vendorID + `","name":"Original Name"}`,
				DeviceID:  "dev-a",
				CreatedAt: now,
			},
		},
	}

	result := ApplyOps(db, ops)
	require.Empty(t, result.Errors, "no errors expected")
	assert.Equal(t, 2, result.Applied)

	// The final state should reflect the update (seq 30), not the insert.
	var vendor data.Vendor
	require.NoError(t, db.Where("id = ?", vendorID).First(&vendor).Error)
	assert.Equal(t, "Updated Name", vendor.Name)
}

func TestApplyOneRemoteWinsLWWClearsLocalAppliedAt(t *testing.T) {
	t.Parallel()

	// Verifies the remote-wins LWW path: when a newer remote op conflicts
	// with an older local unsynced op, the remote op is applied and the
	// local op's applied_at is cleared (all within one transaction).
	dbPath := t.TempDir() + "/test.db"
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	require.NoError(t, store.AutoMigrate())

	db := store.GormDB()
	db = db.WithContext(data.WithSyncApplying(db.Statement.Context))
	now := time.Now()
	vendorID := "vendor-toctou"

	// Insert a vendor row so the remote update has something to target.
	require.NoError(t, db.Table(data.TableVendors).Create(map[string]any{
		"id":   vendorID,
		"name": "Original",
	}).Error)

	// Create a local unsynced op for the same row.
	require.NoError(t, db.Table("sync_oplog_entries").Create(map[string]any{
		"id":         "local-op-1",
		"table_name": data.TableVendors,
		"row_id":     vendorID,
		"op_type":    "update",
		"payload":    `{"name":"Local Update"}`,
		"device_id":  "dev-local",
		"created_at": now.Add(-time.Minute), // older than remote
		"applied_at": now,
		"synced_at":  nil,
	}).Error)

	// Remote op is newer, so it should win the LWW check.
	remoteOp := DecryptedOp{
		Envelope: Envelope{Seq: 1},
		Payload: OpPayload{
			ID:        "remote-op-1",
			TableName: data.TableVendors,
			RowID:     vendorID,
			OpType:    "update",
			Payload:   `{"name":"Remote Update"}`,
			DeviceID:  "dev-remote",
			CreatedAt: now,
		},
	}

	// applyOne should succeed: remote wins, local op's applied_at is cleared.
	err = applyOne(db, remoteOp)
	require.NoError(t, err)

	// Verify the remote update was applied.
	var vendor struct{ Name string }
	require.NoError(t, db.Table(data.TableVendors).
		Where("id = ?", vendorID).Scan(&vendor).Error)
	assert.Equal(t, "Remote Update", vendor.Name)

	// Verify the local op's applied_at was cleared (inside the same tx).
	var localOp struct{ AppliedAt *time.Time }
	require.NoError(t, db.Table("sync_oplog_entries").
		Select("applied_at").
		Where("id = ?", "local-op-1").
		Scan(&localOp).Error)
	assert.Nil(t, localOp.AppliedAt,
		"local op's applied_at should be cleared when remote wins")
}

func TestApplyOneRemoteWinsLWWClearsAllLocalOps(t *testing.T) {
	t.Parallel()

	// Verifies the multi-op remote-wins LWW path: when a newer remote op
	// conflicts with multiple older local unsynced ops for the same row,
	// ALL local ops' applied_at values are cleared — not just the latest.
	// This covers the code comment at apply.go:116-119.
	dbPath := t.TempDir() + "/test.db"
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	require.NoError(t, store.AutoMigrate())

	db := store.GormDB()
	db = db.WithContext(data.WithSyncApplying(db.Statement.Context))
	now := time.Now()
	vendorID := "vendor-multi-local-ops"

	// Insert a vendor row directly so the remote delete has something to target.
	require.NoError(t, db.Table(data.TableVendors).Create(map[string]any{
		"id":   vendorID,
		"name": "Original",
	}).Error)

	// Create TWO local unsynced ops for the same row.
	// INSERT op: older, applied, not synced.
	require.NoError(t, db.Table("sync_oplog_entries").Create(map[string]any{
		"id":         "local-insert-op",
		"table_name": data.TableVendors,
		"row_id":     vendorID,
		"op_type":    "insert",
		"payload":    `{"id":"` + vendorID + `","name":"Original"}`,
		"device_id":  "dev-local",
		"created_at": now.Add(-2 * time.Minute), // oldest
		"applied_at": now,
		"synced_at":  nil,
	}).Error)

	// UPDATE op: newer than insert, still older than remote, applied, not synced.
	require.NoError(t, db.Table("sync_oplog_entries").Create(map[string]any{
		"id":         "local-update-op",
		"table_name": data.TableVendors,
		"row_id":     vendorID,
		"op_type":    "update",
		"payload":    `{"name":"Local Update"}`,
		"device_id":  "dev-local",
		"created_at": now.Add(-time.Minute), // newer than insert, older than remote
		"applied_at": now,
		"synced_at":  nil,
	}).Error)

	// Remote DELETE op is the newest, so it wins the LWW check.
	remoteOp := DecryptedOp{
		Envelope: Envelope{Seq: 1},
		Payload: OpPayload{
			ID:        "remote-delete-op",
			TableName: data.TableVendors,
			RowID:     vendorID,
			OpType:    "delete",
			Payload:   `{}`,
			DeviceID:  "dev-remote",
			CreatedAt: now, // newest of the three
		},
	}

	err = applyOne(db, remoteOp)
	require.NoError(t, err)

	// The vendor row should be soft-deleted.
	var vendor struct {
		DeletedAt *time.Time
	}
	require.NoError(t, db.Table(data.TableVendors).
		Unscoped().
		Where("id = ?", vendorID).
		Scan(&vendor).Error)
	assert.NotNil(t, vendor.DeletedAt, "vendor should be soft-deleted after remote delete wins")

	// BOTH local ops must have applied_at cleared.
	var insertOp struct{ AppliedAt *time.Time }
	require.NoError(t, db.Table("sync_oplog_entries").
		Select("applied_at").
		Where("id = ?", "local-insert-op").
		Scan(&insertOp).Error)
	assert.Nil(t, insertOp.AppliedAt,
		"local insert op's applied_at should be cleared when remote wins")

	var updateOp struct{ AppliedAt *time.Time }
	require.NoError(t, db.Table("sync_oplog_entries").
		Select("applied_at").
		Where("id = ?", "local-update-op").
		Scan(&updateOp).Error)
	assert.Nil(t, updateOp.AppliedAt,
		"local update op's applied_at should be cleared when remote wins")

	// The remote delete op should be recorded with applied_at and synced_at set.
	var remoteRecord struct {
		AppliedAt *time.Time
		SyncedAt  *time.Time
	}
	require.NoError(t, db.Table("sync_oplog_entries").
		Select("applied_at, synced_at").
		Where("id = ?", "remote-delete-op").
		Scan(&remoteRecord).Error)
	assert.NotNil(t, remoteRecord.AppliedAt, "remote op should have applied_at set")
	assert.NotNil(t, remoteRecord.SyncedAt, "remote op should have synced_at set")
}

func TestRecordUnappliedOpReturnsDBError(t *testing.T) {
	t.Parallel()

	// Open a real SQLite DB so we can operate on sync_oplog_entries.
	dbPath := t.TempDir() + "/test.db"
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	require.NoError(t, store.AutoMigrate())

	db := store.GormDB()
	now := time.Now()

	// Insert a valid op first, so we can trigger a duplicate ID error.
	op := OpPayload{
		ID:        "dup-op-id",
		TableName: data.TableVendors,
		RowID:     "vendor-1",
		OpType:    "insert",
		Payload:   `{"id":"vendor-1","name":"Test"}`,
		DeviceID:  "dev-a",
		CreatedAt: now,
	}

	// First call succeeds (returns errConflictLoss).
	err = recordUnappliedOp(db, op)
	require.Error(t, err)
	assert.True(t, isConflictLoss(err), "first call should return errConflictLoss")

	// Second call with same ID should fail with a real DB error (duplicate PK),
	// not errConflictLoss.
	err = recordUnappliedOp(db, op)
	require.Error(t, err)
	assert.False(t, isConflictLoss(err),
		"DB error from duplicate insert should not be masked as errConflictLoss")
}

func TestApplyDeleteMissingRowReturnsError(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir() + "/test.db"
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	require.NoError(t, store.AutoMigrate())

	db := store.GormDB()
	db = db.WithContext(data.WithSyncApplying(db.Statement.Context))

	op := OpPayload{
		TableName: data.TableVendors,
		RowID:     "nonexistent-vendor",
		OpType:    "delete",
	}
	err = applyDelete(db, op)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "row not found")
}

func TestApplyRestoreMissingRowReturnsError(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir() + "/test.db"
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	require.NoError(t, store.AutoMigrate())

	db := store.GormDB()
	db = db.WithContext(data.WithSyncApplying(db.Statement.Context))

	op := OpPayload{
		TableName: data.TableVendors,
		RowID:     "nonexistent-vendor",
		OpType:    "restore",
	}
	err = applyRestore(db, op)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "row not found")
}

func TestApplyUpdateStripsCreatedAt(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir() + "/test.db"
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	require.NoError(t, store.AutoMigrate())

	db := store.GormDB()
	db = db.WithContext(data.WithSyncApplying(db.Statement.Context))

	vendorID := "vendor-created-at"
	originalTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Insert a vendor with a known created_at.
	require.NoError(t, db.Table(data.TableVendors).Create(map[string]any{
		"id":         vendorID,
		"name":       "Original",
		"created_at": originalTime,
	}).Error)

	// Remote update tries to overwrite created_at.
	op := OpPayload{
		TableName: data.TableVendors,
		RowID:     vendorID,
		OpType:    "update",
		Payload:   `{"name":"Updated","created_at":"2099-12-31T00:00:00Z"}`,
	}
	require.NoError(t, applyUpdate(db, op))

	// Verify name was updated but created_at was preserved.
	var vendor struct {
		Name      string
		CreatedAt time.Time
	}
	require.NoError(t, db.Table(data.TableVendors).
		Where("id = ?", vendorID).Scan(&vendor).Error)
	assert.Equal(t, "Updated", vendor.Name)
	assert.True(t, vendor.CreatedAt.Equal(originalTime),
		"created_at should not be overwritten by remote update")
}

func TestApplyUpdateMissingRowReturnsError(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir() + "/test.db"
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	require.NoError(t, store.AutoMigrate())

	db := store.GormDB()
	db = db.WithContext(data.WithSyncApplying(db.Statement.Context))

	op := OpPayload{
		TableName: data.TableVendors,
		RowID:     "nonexistent-vendor",
		OpType:    "update",
		Payload:   `{"name":"Ghost"}`,
	}
	err = applyUpdate(db, op)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "row not found")
}

func TestApplyOpsDeleteBeforeInsertSortedBySeq(t *testing.T) {
	t.Parallel()

	// Open a real SQLite DB so ApplyOps can run GORM transactions.
	dbPath := t.TempDir() + "/test.db"
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = store.Close() }()
	require.NoError(t, store.AutoMigrate())

	db := store.GormDB()
	now := time.Now()
	vendorID := "vendor-del-seq"
	deleteTime := now.Add(time.Second)

	// Build two ops for the same vendor row, deliberately in reverse seq order.
	// Seq 20 (insert) must apply first; seq 30 (delete) must apply second.
	// If ApplyOps doesn't sort, the delete arrives before the insert and fails
	// because the row doesn't exist yet.
	ops := []DecryptedOp{
		{
			Envelope: Envelope{Seq: 30},
			Payload: OpPayload{
				ID:        "op-del",
				TableName: data.TableVendors,
				RowID:     vendorID,
				OpType:    "delete",
				DeviceID:  "dev-a",
				CreatedAt: deleteTime,
			},
		},
		{
			Envelope: Envelope{Seq: 20},
			Payload: OpPayload{
				ID:        "op-ins",
				TableName: data.TableVendors,
				RowID:     vendorID,
				OpType:    "insert",
				Payload:   `{"id":"` + vendorID + `","name":"Test Vendor"}`,
				DeviceID:  "dev-a",
				CreatedAt: now,
			},
		},
	}

	result := ApplyOps(db, ops)
	require.Empty(t, result.Errors, "no errors expected")
	assert.Equal(t, 2, result.Applied)

	// The row should be soft-deleted with deleted_at equal to the delete op's
	// CreatedAt (deterministic convergence: same op => same deleted_at).
	var vendor data.Vendor
	require.NoError(t, db.Unscoped().Where("id = ?", vendorID).First(&vendor).Error)
	require.True(t, vendor.DeletedAt.Valid, "vendor should be soft-deleted")
	assert.True(t, vendor.DeletedAt.Time.Equal(deleteTime),
		"deleted_at should equal the delete op's CreatedAt for deterministic convergence")
}

func TestStripNonColumnKeysIgnoresNonDocuments(t *testing.T) {
	t.Parallel()

	// For non-document tables, blob_ref should NOT be stripped
	// (it wouldn't exist, but verify the function is a no-op).
	row := map[string]any{
		"id":   "v-1",
		"name": "Acme",
	}
	stripNonColumnKeys("vendors", row)
	assert.Contains(t, row, "name")
}

func TestStripNonColumnKeysAlwaysStripsDeletedAt(t *testing.T) {
	t.Parallel()

	t.Run("non-document table strips deleted_at", func(t *testing.T) {
		t.Parallel()

		row := map[string]any{
			"id":         "v-1",
			"name":       "Acme",
			"deleted_at": "2026-01-01T00:00:00Z",
		}
		stripNonColumnKeys(data.TableVendors, row)
		assert.NotContains(t, row, "deleted_at", "deleted_at should always be stripped")
		assert.Contains(t, row, "id", "id should be preserved")
		assert.Contains(t, row, "name", "name should be preserved")
	})

	t.Run("documents table strips both blob_ref and deleted_at", func(t *testing.T) {
		t.Parallel()

		row := map[string]any{
			"id":         "doc-1",
			"title":      "Invoice",
			"file_name":  "invoice.pdf",
			"sha256":     "abc123",
			"blob_ref":   "abc123",
			"deleted_at": "2026-01-01T00:00:00Z",
		}
		stripNonColumnKeys(data.TableDocuments, row)
		assert.NotContains(t, row, "deleted_at", "deleted_at should always be stripped")
		assert.NotContains(t, row, "blob_ref", "blob_ref should be stripped from documents")
		assert.Contains(t, row, "id", "id should be preserved")
		assert.Contains(t, row, "title", "title should be preserved")
		assert.Contains(t, row, "sha256", "sha256 should be preserved")
	})
}
