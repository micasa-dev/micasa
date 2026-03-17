// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"testing"
	"time"

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

func TestApplyInsertRejectsMismatchedRowID(t *testing.T) {
	t.Parallel()

	// The payload ID doesn't match op.RowID -- applyInsert should reject it.
	op := OpPayload{
		ID:        "op-1",
		TableName: "vendors",
		RowID:     "vendor-1",
		OpType:    "insert",
		Payload:   `{"id":"vendor-WRONG","name":"Spoofed"}`,
	}
	err := applyInsert(nil, op) // tx is nil; we expect an error before DB access
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payload id")
}

func TestApplyInsertAcceptsMatchingRowID(t *testing.T) {
	t.Parallel()

	// Matching IDs should not trigger the validation error.
	// We pass nil tx so it panics at the DB call (past validation).
	op := OpPayload{
		ID:        "op-1",
		TableName: "vendors",
		RowID:     "vendor-1",
		OpType:    "insert",
		Payload:   `{"id":"vendor-1","name":"Legit"}`,
	}
	assert.Panics(t, func() { _ = applyInsert(nil, op) },
		"should pass ID validation and panic on nil tx")
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
