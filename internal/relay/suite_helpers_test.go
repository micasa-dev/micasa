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
func suiteCreateHousehold(tb testing.TB, store Store) sync.CreateHouseholdResponse {
	tb.Helper()
	resp, err := store.CreateHousehold(tb.Context(), sync.CreateHouseholdRequest{
		DeviceName: "test-device",
		PublicKey:  testPublicKey,
	})
	require.NoError(tb, err)
	return resp
}

// suiteJoinDevice runs the full invite -> join -> complete -> poll flow,
// returning the new device's ID and raw token.
func suiteJoinDevice(
	tb testing.TB, store Store, hhID, inviterDevID string,
) (deviceID, deviceToken string) {
	tb.Helper()
	ctx := tb.Context()

	invite, err := store.CreateInvite(ctx, hhID, inviterDevID)
	require.NoError(tb, err)

	joinResp, err := store.StartJoin(ctx, hhID, invite.Code, sync.JoinRequest{
		DeviceName: "joined-device",
		PublicKey:  testPublicKey,
	})
	require.NoError(tb, err)

	err = store.CompleteKeyExchange(ctx, hhID, joinResp.ExchangeID, []byte("fake-enc-key"))
	require.NoError(tb, err)

	result, err := store.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
	require.NoError(tb, err)
	require.True(tb, result.Ready)

	return result.DeviceID, result.DeviceToken
}

// suiteActivateSubscription sets a household's subscription to "active".
func suiteActivateSubscription(tb testing.TB, store Store, hhID string) {
	tb.Helper()
	err := store.UpdateSubscription(tb.Context(), hhID, "sub_test", sync.SubscriptionActive)
	require.NoError(tb, err)
}

// expireInvite sets an invite's expiry to the past (test-only).
func expireInvite(tb testing.TB, store Store, code string) {
	tb.Helper()
	switch s := store.(type) {
	case *MemStore:
		s.mu.Lock()
		inv, ok := s.invites[code]
		require.True(tb, ok, "invite %q not found in MemStore", code)
		inv.expiresAt = time.Now().Add(-time.Hour)
		s.mu.Unlock()
	case *PgStore:
		require.NoError(tb, s.rls.WithoutHousehold(tb.Context(), func(tx *gorm.DB) error {
			return tx.Exec(
				"UPDATE invites SET expires_at = ? WHERE code = ?",
				time.Now().Add(-time.Hour), code,
			).Error
		}))
	default:
		tb.Fatalf("unsupported store type %T", store)
	}
}

// expireKeyExchange sets an exchange's created_at to >15min ago (test-only).
func expireKeyExchange(tb testing.TB, store Store, exchangeID string) {
	tb.Helper()
	past := time.Now().Add(-20 * time.Minute)
	switch s := store.(type) {
	case *MemStore:
		s.mu.Lock()
		ex, ok := s.exchanges[exchangeID]
		require.True(tb, ok, "exchange %q not found in MemStore", exchangeID)
		ex.createdAt = past
		s.mu.Unlock()
	case *PgStore:
		require.NoError(tb, s.rls.WithoutHousehold(tb.Context(), func(tx *gorm.DB) error {
			return tx.Exec(
				"UPDATE key_exchanges SET created_at = ? WHERE id = ?",
				past, exchangeID,
			).Error
		}))
	default:
		tb.Fatalf("unsupported store type %T", store)
	}
}
