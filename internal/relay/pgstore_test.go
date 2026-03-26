// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"fmt"
	"os"
	"testing"

	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// openTestPgStore opens a PgStore for testing, skipping if RELAY_POSTGRES_DSN
// is not set. Each test gets a fresh schema via AutoMigrate + table truncation.
func openTestPgStore(t *testing.T) *PgStore {
	t.Helper()
	dsn := os.Getenv("RELAY_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("RELAY_POSTGRES_DSN not set, skipping Postgres integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	store := NewPgStore(db)
	require.NoError(t, store.AutoMigrate())

	// Derive table names from pgModels and truncate with CASCADE.
	for _, m := range pgModels {
		tabler, ok := m.(schema.Tabler)
		require.True(t, ok, "model %T must implement schema.Tabler", m)
		require.NoError(t, db.Exec("TRUNCATE "+tabler.TableName()+" CASCADE").Error)
	}

	store.SetEncryptionKey(defaultTestEncryptionKey)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestPgStoreCreateHouseholdAndAuth(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	resp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Test Desktop",
		PublicKey:  []byte("test-public-key"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.HouseholdID)
	assert.NotEmpty(t, resp.DeviceID)
	assert.NotEmpty(t, resp.DeviceToken)

	// Authenticate with the returned token.
	dev, err := store.AuthenticateDevice(ctx, resp.DeviceToken)
	require.NoError(t, err)
	assert.Equal(t, resp.DeviceID, dev.ID)
	assert.Equal(t, resp.HouseholdID, dev.HouseholdID)
	assert.Equal(t, "Test Desktop", dev.Name)
	assert.Equal(t, []byte("test-public-key"), dev.PublicKey)
}

func TestPgStoreInvalidTokenFails(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	_, err := store.AuthenticateDevice(ctx, "not-a-valid-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid token")
}

func TestPgStorePushPull(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	resp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Desktop",
	})
	require.NoError(t, err)

	ops := []sync.Envelope{
		{
			ID:          "op-1",
			HouseholdID: resp.HouseholdID,
			DeviceID:    resp.DeviceID,
			Nonce:       []byte("nonce1"),
			Ciphertext:  []byte("cipher1"),
		},
		{
			ID:          "op-2",
			HouseholdID: resp.HouseholdID,
			DeviceID:    resp.DeviceID,
			Nonce:       []byte("nonce2"),
			Ciphertext:  []byte("cipher2"),
		},
	}
	confirmed, err := store.Push(ctx, ops)
	require.NoError(t, err)
	require.Len(t, confirmed, 2)
	assert.Equal(t, int64(1), confirmed[0].Seq)
	assert.Equal(t, int64(2), confirmed[1].Seq)

	// Pull from a different device should see both ops.
	pulled, hasMore, err := store.Pull(ctx, resp.HouseholdID, "other-device", 0, 100)
	require.NoError(t, err)
	assert.False(t, hasMore)
	assert.Len(t, pulled, 2)
	assert.Equal(t, "op-1", pulled[0].ID)
	assert.Equal(t, "op-2", pulled[1].ID)

	// Pull from the same device should see nothing (self-exclusion).
	pulled, _, err = store.Pull(ctx, resp.HouseholdID, resp.DeviceID, 0, 100)
	require.NoError(t, err)
	assert.Empty(t, pulled)

	// Pull with afterSeq should skip earlier ops.
	pulled, _, err = store.Pull(ctx, resp.HouseholdID, "other-device", 1, 100)
	require.NoError(t, err)
	assert.Len(t, pulled, 1)
	assert.Equal(t, "op-2", pulled[0].ID)
}

func TestPgStorePullHasMorePagination(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	resp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Desktop",
	})
	require.NoError(t, err)

	ops := make([]sync.Envelope, 5)
	for i := range ops {
		ops[i] = sync.Envelope{
			ID:          fmt.Sprintf("op-%d", i),
			HouseholdID: resp.HouseholdID,
			DeviceID:    resp.DeviceID,
			Nonce:       []byte("n"),
			Ciphertext:  []byte("c"),
		}
	}
	_, err = store.Push(ctx, ops)
	require.NoError(t, err)

	// Pull with limit=3 should return 3 ops and hasMore=true.
	pulled, hasMore, err := store.Pull(ctx, resp.HouseholdID, "other", 0, 3)
	require.NoError(t, err)
	assert.True(t, hasMore)
	assert.Len(t, pulled, 3)
}

func TestPgStoreInviteJoinFlow(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	hhResp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Desktop",
		PublicKey:  []byte("inviter-pk"),
	})
	require.NoError(t, err)

	// Create invite.
	invite, err := store.CreateInvite(ctx, hhResp.HouseholdID, hhResp.DeviceID)
	require.NoError(t, err)
	assert.NotEmpty(t, invite.Code)

	// Start join.
	joinResp, err := store.StartJoin(ctx, hhResp.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "Laptop",
		PublicKey:  []byte("joiner-pk"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, joinResp.ExchangeID)
	assert.Equal(t, hhResp.HouseholdID, joinResp.HouseholdID)
	assert.Equal(t, []byte("inviter-pk"), joinResp.InviterPublicKey)

	// Check pending exchanges.
	pending, err := store.GetPendingExchanges(ctx, hhResp.HouseholdID)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "Laptop", pending[0].JoinerName)

	// Exchange not ready yet.
	result, err := store.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
	require.NoError(t, err)
	assert.False(t, result.Ready)

	// Complete key exchange.
	err = store.CompleteKeyExchange(
		ctx, hhResp.HouseholdID, joinResp.ExchangeID, []byte("encrypted-key"),
	)
	require.NoError(t, err)

	// Now ready.
	result, err = store.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
	require.NoError(t, err)
	assert.True(t, result.Ready)
	assert.Equal(t, []byte("encrypted-key"), result.EncryptedHouseholdKey)
	assert.NotEmpty(t, result.DeviceID)
	assert.NotEmpty(t, result.DeviceToken)

	// Second retrieval fails (credentials are single-use).
	_, err = store.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already consumed")

	// Joiner can authenticate.
	dev, err := store.AuthenticateDevice(ctx, result.DeviceToken)
	require.NoError(t, err)
	assert.Equal(t, result.DeviceID, dev.ID)
	assert.Equal(t, "Laptop", dev.Name)
}

func TestPgStoreRevokeDevice(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	resp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Desktop",
	})
	require.NoError(t, err)

	// Register a second device.
	dev2, err := store.RegisterDevice(ctx, sync.RegisterDeviceRequest{
		HouseholdID: resp.HouseholdID,
		Name:        "Laptop",
	})
	require.NoError(t, err)

	// Revoke the second device.
	err = store.RevokeDevice(ctx, resp.HouseholdID, dev2.DeviceID)
	require.NoError(t, err)

	// Revoked device can't authenticate.
	_, err = store.AuthenticateDevice(ctx, dev2.DeviceToken)
	require.Error(t, err)

	// Revoked device not in list.
	devices, err := store.ListDevices(ctx, resp.HouseholdID)
	require.NoError(t, err)
	assert.Len(t, devices, 1)
	assert.Equal(t, resp.DeviceID, devices[0].ID)
}

func TestPgStoreBlobOperations(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	resp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Desktop",
	})
	require.NoError(t, err)

	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	data := []byte("encrypted-blob-data")

	// Put blob.
	err = store.PutBlob(ctx, resp.HouseholdID, hash, data, DefaultBlobQuota)
	require.NoError(t, err)

	// Has blob.
	exists, err := store.HasBlob(ctx, resp.HouseholdID, hash)
	require.NoError(t, err)
	assert.True(t, exists)

	// Get blob.
	got, err := store.GetBlob(ctx, resp.HouseholdID, hash)
	require.NoError(t, err)
	assert.Equal(t, data, got)

	// Dedup: same hash returns errBlobExists.
	err = store.PutBlob(ctx, resp.HouseholdID, hash, data, DefaultBlobQuota)
	require.ErrorIs(t, err, errBlobExists)

	// Usage.
	usage, err := store.BlobUsage(ctx, resp.HouseholdID)
	require.NoError(t, err)
	assert.Equal(t, int64(len(data)), usage)

	// Non-existent blob.
	_, err = store.GetBlob(
		ctx,
		resp.HouseholdID,
		"0000000000000000000000000000000000000000000000000000000000000000",
	)
	require.ErrorIs(t, err, errBlobNotFound)

	exists, err = store.HasBlob(
		ctx,
		resp.HouseholdID,
		"0000000000000000000000000000000000000000000000000000000000000000",
	)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestPgStoreSubscriptionFlow(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	resp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Desktop",
	})
	require.NoError(t, err)

	// Update subscription.
	err = store.UpdateSubscription(ctx, resp.HouseholdID, "sub_123", "active")
	require.NoError(t, err)

	// Get household shows subscription.
	hh, err := store.GetHousehold(ctx, resp.HouseholdID)
	require.NoError(t, err)
	require.NotNil(t, hh.StripeSubscriptionID)
	assert.Equal(t, "sub_123", *hh.StripeSubscriptionID)
	require.NotNil(t, hh.StripeStatus)
	assert.Equal(t, "active", *hh.StripeStatus)

	// Find by subscription.
	hh2, err := store.HouseholdBySubscription(ctx, "sub_123")
	require.NoError(t, err)
	assert.Equal(t, resp.HouseholdID, hh2.ID)
}

func TestPgStoreOpsCount(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	resp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Desktop",
	})
	require.NoError(t, err)

	count, err := store.OpsCount(ctx, resp.HouseholdID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	_, err = store.Push(ctx, []sync.Envelope{{
		ID:          "op-1",
		HouseholdID: resp.HouseholdID,
		DeviceID:    resp.DeviceID,
		Nonce:       []byte("n"),
		Ciphertext:  []byte("c"),
	}})
	require.NoError(t, err)

	count, err = store.OpsCount(ctx, resp.HouseholdID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestPgStoreMaxActiveInvites(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	resp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Desktop",
	})
	require.NoError(t, err)

	// Create 3 invites (the max).
	for i := range 3 {
		_, err := store.CreateInvite(ctx, resp.HouseholdID, resp.DeviceID)
		require.NoError(t, err, "invite %d should succeed", i)
	}

	// Fourth should fail.
	_, err = store.CreateInvite(ctx, resp.HouseholdID, resp.DeviceID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max active invites")
}

func TestPgStoreWrongHouseholdInvite(t *testing.T) {
	store := openTestPgStore(t)
	ctx := t.Context()

	resp1, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Desktop",
	})
	require.NoError(t, err)

	resp2, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "Other Desktop",
	})
	require.NoError(t, err)

	invite, err := store.CreateInvite(ctx, resp1.HouseholdID, resp1.DeviceID)
	require.NoError(t, err)

	// Try to join with wrong household.
	_, err = store.StartJoin(ctx, resp2.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "Laptop",
		PublicKey:  []byte("pk"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
