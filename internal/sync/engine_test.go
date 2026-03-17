// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/crypto"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/relay"
	"github.com/cpcloud/micasa/internal/sync"
	"github.com/cpcloud/micasa/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupEngineRelay creates a test relay server with a household and returns
// the server, household ID, and a sync client connected to that household.
func setupEngineRelay(
	t *testing.T,
) (*httptest.Server, *relay.MemStore, string, string, crypto.HouseholdKey) {
	t.Helper()

	ms := relay.NewMemStore()
	handler := relay.NewHandler(ms, slog.Default())
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	key, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	resp, err := ms.CreateHousehold(context.Background(), sync.CreateHouseholdRequest{
		DeviceName: "test-device",
		PublicKey:  make([]byte, 32),
	})
	require.NoError(t, err)

	return srv, ms, resp.HouseholdID, resp.DeviceToken, key
}

// openTestStore opens an in-memory SQLite store pre-wired with a SyncDevice
// record pointing at the given relay URL and household.
func openTestStore(t *testing.T, relayURL, householdID, deviceID string) *data.Store {
	t.Helper()

	store, err := data.Open(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate())
	t.Cleanup(func() { _ = store.Close() })

	// Seed the SyncDevice row directly — UpdateSyncDevice only updates
	// existing rows, so we must create it first before setting fields.
	err = store.GormDB().Create(&data.SyncDevice{
		ID:          deviceID,
		Name:        "test-device",
		HouseholdID: householdID,
		RelayURL:    relayURL,
		LastSeq:     0,
	}).Error
	require.NoError(t, err)

	// Wire the device ID into the oplog cell so hooks write the right ID.
	store.SetDeviceID(deviceID)

	return store
}

// seedRelayOps pushes ops from a second device into the relay so the engine
// under test can pull them.  Returns the number of ops pushed.
func seedRelayOps(
	t *testing.T,
	ms *relay.MemStore,
	srv *httptest.Server,
	householdID string,
	key crypto.HouseholdKey,
	n int,
) int {
	t.Helper()

	// Register a second device to act as the remote sender.
	regResp, err := ms.RegisterDevice(context.Background(), sync.RegisterDeviceRequest{
		HouseholdID: householdID,
		Name:        "remote-device",
		PublicKey:   make([]byte, 32),
	})
	require.NoError(t, err)

	remoteClient := sync.NewClient(srv.URL, regResp.DeviceToken, key)

	ops := make([]sync.OpPayload, n)
	for i := range ops {
		rowID := uid.New()
		payload, err := json.Marshal(map[string]any{
			"id":   rowID,
			"name": "Remote Vendor " + rowID, // unique per ULID
		})
		require.NoError(t, err)
		ops[i] = sync.OpPayload{
			ID:        uid.New(),
			TableName: "vendors",
			RowID:     rowID,
			OpType:    "insert",
			Payload:   string(payload),
			DeviceID:  regResp.DeviceToken,
			CreatedAt: time.Now(),
		}
	}

	pushResp, err := remoteClient.Push(ops)
	require.NoError(t, err)
	return len(pushResp.Confirmed)
}

func TestEngineSyncPullsThenPushes(t *testing.T) {
	t.Parallel()

	srv, ms, householdID, token, key := setupEngineRelay(t)

	// Seed ops from a remote device so pull has something to fetch.
	pulled := seedRelayOps(t, ms, srv, householdID, key, 3)
	require.Equal(t, 3, pulled)

	// Set up local store with a pending op to push.
	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	// Insert a vendor so the oplog gets a local op to push.
	require.NoError(t, store.CreateVendor(&data.Vendor{
		ID:   uid.New(),
		Name: "Local Vendor",
	}))

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(context.Background())
	require.NoError(t, err)

	// Should have pulled the 3 remote ops.
	assert.Equal(t, 3, result.Pulled, "expected 3 pulled ops")

	// Should have pushed the 1 local vendor insert.
	assert.Equal(t, 1, result.Pushed, "expected 1 pushed op")

	// No blob activity for vendor ops.
	assert.Zero(t, result.BlobsUp)
	assert.Zero(t, result.BlobsDown)
	assert.Zero(t, result.BlobErrs)
}

func TestEngineSyncCancelledContext(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := engine.Sync(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestEngineSyncReadsLastSeqFromStore(t *testing.T) {
	t.Parallel()

	srv, ms, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	// Seed 5 remote ops.
	seedRelayOps(t, ms, srv, householdID, key, 5)

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	// First sync: should pull all 5.
	result1, err := engine.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 5, result1.Pulled)

	// Second sync: last_seq was updated, so nothing new to pull.
	result2, err := engine.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, result2.Pulled, "second sync should pull nothing (already seen)")
}

func TestEngineSyncEmptyIsNoop(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(context.Background())
	require.NoError(t, err)

	assert.Zero(t, result.Pulled)
	assert.Zero(t, result.Pushed)
	assert.Zero(t, result.Conflicts)
	assert.Zero(t, result.BlobsUp)
	assert.Zero(t, result.BlobsDown)
	assert.Zero(t, result.BlobErrs)
}
