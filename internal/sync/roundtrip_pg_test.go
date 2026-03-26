// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync_test

import (
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/micasa-dev/micasa/internal/crypto"
	"github.com/micasa-dev/micasa/internal/relay"
	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/stretchr/testify/require"
)

// TestSyncRoundTripPgStore runs the same round-trip sync test as
// TestSyncRoundTrip but with a real PostgreSQL-backed relay store.
// Requires RELAY_POSTGRES_DSN to be set (e.g., from Docker Compose).
func TestSyncRoundTripPgStore(t *testing.T) {
	dsn := os.Getenv("RELAY_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("RELAY_POSTGRES_DSN not set, skipping Postgres integration test")
	}
	t.Parallel()
	ctx := t.Context()

	// --- 1. Start in-process relay with PgStore ---
	pg, err := relay.OpenPgStore(dsn)
	require.NoError(t, err)
	require.NoError(t, pg.AutoMigrate())
	pg.SetEncryptionKey([]byte("test-encryption-key-exactly-32b!"))
	t.Cleanup(func() { _ = pg.Close() })

	handler := relay.NewHandler(pg, slog.Default(), relay.WithSelfHosted())
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Generate a shared household key.
	hhKey, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	// --- 2. Create household (device A) ---
	kpA, err := crypto.GenerateDeviceKeyPair()
	require.NoError(t, err)

	hhResp, err := pg.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "device-a",
		PublicKey:  kpA.PublicKey[:],
	})
	require.NoError(t, err)

	householdID := hhResp.HouseholdID
	tokenA := hhResp.DeviceToken
	deviceIDA := hhResp.DeviceID

	// --- 3. Open local SQLite A, migrate, seed defaults + demo data ---
	storeA := openRoundTripStore(t, srv.URL, householdID, deviceIDA)
	require.NoError(t, storeA.SeedDefaults())
	require.NoError(t, storeA.SeedDemoData())

	// --- 4. Create sync engine A, push all data ---
	clientA := sync.NewClient(srv.URL, tokenA, hhKey)
	engineA := sync.NewEngine(storeA, clientA, householdID)

	resultA, err := engineA.Sync(ctx)
	require.NoError(t, err)
	require.Greater(t, resultA.Pushed, 0, "device A should push demo data ops")

	// --- 5. Register device B on the relay ---
	kpB, err := crypto.GenerateDeviceKeyPair()
	require.NoError(t, err)

	regResp, err := pg.RegisterDevice(ctx, sync.RegisterDeviceRequest{
		HouseholdID: householdID,
		Name:        "device-b",
		PublicKey:   kpB.PublicKey[:],
	})
	require.NoError(t, err)

	tokenB := regResp.DeviceToken
	deviceIDB := regResp.DeviceID

	// --- 6. Open local SQLite B, migrate ---
	storeB := openRoundTripStore(t, srv.URL, householdID, deviceIDB)

	// --- 7. Create sync engine B, pull all data ---
	clientB := sync.NewClient(srv.URL, tokenB, hhKey)
	engineB := sync.NewEngine(storeB, clientB, householdID)

	resultB, err := engineB.Sync(ctx)
	require.NoError(t, err)
	require.Greater(t, resultB.Pulled, 0, "device B should pull ops from device A")

	// --- 8. Compare every entity table between A and B ---
	compareHouseProfiles(t, storeA, storeB)
	compareProjects(t, storeA, storeB)
	compareProjectTypes(t, storeA, storeB)
	compareMaintenanceCategories(t, storeA, storeB)
	compareMaintenanceItems(t, storeA, storeB)
	compareServiceLogEntries(t, storeA, storeB)
	compareAppliances(t, storeA, storeB)
	compareIncidents(t, storeA, storeB)
	compareVendors(t, storeA, storeB)
	compareQuotes(t, storeA, storeB)
	compareDocuments(t, storeA, storeB)
}
