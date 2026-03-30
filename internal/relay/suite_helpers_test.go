// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var testPublicKey = []byte("fake-public-key-32-bytes-paddin!")

// suiteCreateHousehold creates a household + device via the Store interface.
func suiteCreateHousehold(t testing.TB, store Store) sync.CreateHouseholdResponse {
	t.Helper()
	resp, err := store.CreateHousehold(t.Context(), sync.CreateHouseholdRequest{
		DeviceName: "test-device",
		PublicKey:  testPublicKey,
	})
	require.NoError(t, err)
	return resp
}

// suiteJoinDevice runs the full invite -> join -> complete -> poll flow,
// returning the new device's ID and raw token.
func suiteJoinDevice(
	t testing.TB, store Store, hhID, inviterDevID string,
) (deviceID, deviceToken string) {
	t.Helper()
	ctx := t.Context()

	invite, err := store.CreateInvite(ctx, hhID, inviterDevID)
	require.NoError(t, err)

	joinResp, err := store.StartJoin(ctx, hhID, invite.Code, sync.JoinRequest{
		DeviceName: "joined-device",
		PublicKey:  testPublicKey,
	})
	require.NoError(t, err)

	err = store.CompleteKeyExchange(ctx, hhID, joinResp.ExchangeID, []byte("fake-enc-key"))
	require.NoError(t, err)

	result, err := store.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
	require.NoError(t, err)
	require.True(t, result.Ready)

	return result.DeviceID, result.DeviceToken
}

// suiteActivateSubscription sets a household's subscription to "active".
func suiteActivateSubscription(t testing.TB, store Store, hhID string) {
	t.Helper()
	err := store.UpdateSubscription(t.Context(), hhID, "sub_test", sync.SubscriptionActive)
	require.NoError(t, err)
}

// expireInvite sets an invite's expiry to the past (test-only).
func expireInvite(t testing.TB, store Store, code string) {
	t.Helper()
	switch s := store.(type) {
	case *MemStore:
		s.mu.Lock()
		inv, ok := s.invites[code]
		require.True(t, ok, "invite %q not found in MemStore", code)
		inv.expiresAt = time.Now().Add(-time.Hour)
		s.mu.Unlock()
	case *PgStore:
		require.NoError(t, s.rls.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
			return tx.Exec(
				"UPDATE invites SET expires_at = ? WHERE code = ?",
				time.Now().Add(-time.Hour), code,
			).Error
		}))
	default:
		t.Fatalf("unsupported store type %T", store)
	}
}

// expireKeyExchange sets an exchange's created_at to >15min ago (test-only).
func expireKeyExchange(t testing.TB, store Store, exchangeID string) {
	t.Helper()
	past := time.Now().Add(-20 * time.Minute)
	switch s := store.(type) {
	case *MemStore:
		s.mu.Lock()
		ex, ok := s.exchanges[exchangeID]
		require.True(t, ok, "exchange %q not found in MemStore", exchangeID)
		ex.createdAt = past
		s.mu.Unlock()
	case *PgStore:
		require.NoError(t, s.rls.WithoutHousehold(t.Context(), func(tx *gorm.DB) error {
			return tx.Exec(
				"UPDATE key_exchanges SET created_at = ? WHERE id = ?",
				past, exchangeID,
			).Error
		}))
	default:
		t.Fatalf("unsupported store type %T", store)
	}
}
