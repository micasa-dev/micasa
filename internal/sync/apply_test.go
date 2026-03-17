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
