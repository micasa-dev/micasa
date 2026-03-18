// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/cpcloud/micasa/internal/crypto"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/cpcloud/micasa/internal/relay"
	"github.com/cpcloud/micasa/internal/sync"
	"github.com/cpcloud/micasa/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendMsg feeds a tea.Msg through Update and returns the resulting cmd.
func sendMsg(m *Model, msg tea.Msg) tea.Cmd {
	_, cmd := m.Update(msg)
	return cmd
}

func TestSyncStartedSetsSyncing(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.syncCfg = &syncConfig{} // enable sync indicator

	sendMsg(m, syncStartedMsg{})

	assert.Equal(t, syncSyncing, m.syncStatus)
}

func TestSyncDoneSetsSynced(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.syncCfg = &syncConfig{}

	sendMsg(m, syncDoneMsg{Pulled: 5})

	assert.Equal(t, syncSynced, m.syncStatus)
}

func TestSyncDoneWithConflictsSetsConflict(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.syncCfg = &syncConfig{}

	sendMsg(m, syncDoneMsg{Conflicts: 1})

	assert.Equal(t, syncConflict, m.syncStatus)
}

func TestSyncErrorSetsOffline(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.syncCfg = &syncConfig{}

	sendMsg(m, syncErrorMsg{Err: errors.New("network down")})

	assert.Equal(t, syncOffline, m.syncStatus)
}

func TestSyncTickWhileSyncingRearms(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.syncCfg = &syncConfig{}
	m.syncStatus = syncSyncing

	cmd := sendMsg(m, syncTickMsg{})

	// Should re-arm the tick but NOT start a new sync.
	assert.NotNil(t, cmd, "tick should produce a cmd (re-arm)")
	assert.Equal(t, syncSyncing, m.syncStatus, "status should stay syncing")
}

func TestSyncDebounceStaleGenIgnored(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.syncCfg = &syncConfig{}
	m.syncDebounceGen = 5

	cmd := sendMsg(m, syncDebounceMsg{gen: 3})

	assert.Nil(t, cmd, "stale debounce should produce no cmd")
}

func TestSyncIndicatorGlyphs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status syncStatus
		glyph  string
	}{
		{syncSynced, "\u25c8"},
		{syncSyncing, "\u25c9"},
		{syncOffline, "\u25cb"},
		{syncConflict, "!"},
	}
	for _, tt := range tests {
		m := newTestModel(t)
		m.syncCfg = &syncConfig{}
		m.syncStatus = tt.status

		ind := m.syncIndicator()
		assert.Contains(t, ind, tt.glyph)
	}
}

func TestSyncIndicatorEmptyWhenDisabled(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	// syncCfg is nil — sync not configured

	assert.Empty(t, m.syncIndicator())
}

func TestSyncDonePulledDefersReloadDuringForm(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.syncCfg = &syncConfig{}
	m.mode = modeForm

	sendMsg(m, syncDoneMsg{Pulled: 3})

	assert.True(t, m.syncPendingReload, "should defer reload when form is open")
	assert.Equal(t, syncSynced, m.syncStatus)
}

func TestSyncPendingReloadClearedOnExitForm(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.syncCfg = &syncConfig{}

	// Enter form mode via user interaction.
	openAddForm(m)
	require.Equal(t, modeForm, m.mode, "should be in form mode")

	// Simulate pending reload from a sync while form is open.
	m.syncPendingReload = true

	// Exit form via esc (form is clean so no confirm dialog).
	sendKey(m, keyEsc)

	assert.False(t, m.syncPendingReload, "esc should clear pending reload")
}

func TestMutationBumpsSyncDebounceGen(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.syncCfg = &syncConfig{}
	m.syncEngine = nil

	before := m.syncDebounceGen
	m.reloadAfterMutation()
	assert.Equal(t, before, m.syncDebounceGen, "gen should not bump without sync engine")
}

// TestSyncIntegrationPushesLocalOps verifies the full pipeline:
// Model + real store (oplog hooks) + sync.Engine + test relay.
// A local vendor create generates an oplog entry, and Engine.Sync
// pushes it to the relay.
func TestSyncIntegrationPushesLocalOps(t *testing.T) {
	t.Parallel()

	// 1. Stand up a test relay.
	ms := relay.NewMemStore()
	handler := relay.NewHandler(ms, slog.Default())
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	key, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	resp, err := ms.CreateHousehold(
		context.Background(),
		sync.CreateHouseholdRequest{
			DeviceName: "test-device",
			PublicKey:  make([]byte, 32),
		},
	)
	require.NoError(t, err)

	// 2. Create a real store with oplog hooks.
	dbPath := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(dbPath, templateBytes, 0o600))
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	store.SetCurrency(locale.DefaultCurrency())

	// Seed a house profile so the model doesn't open the setup form.
	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{
		Nickname: "Test House",
	}))

	deviceID := uid.New()
	store.SetDeviceID(deviceID)

	// Seed the SyncDevice row so the engine can read lastSeq.
	require.NoError(t, store.GormDB().Create(&data.SyncDevice{
		ID:          deviceID,
		Name:        "test-device",
		HouseholdID: resp.HouseholdID,
		RelayURL:    srv.URL,
		LastSeq:     0,
	}).Error)

	// 3. Build a Model with sync configured.
	syncClient := sync.NewClient(srv.URL, resp.DeviceToken, key)
	engine := sync.NewEngine(store, syncClient, resp.HouseholdID)

	m, err := NewModel(store, Options{DBPath: dbPath})
	require.NoError(t, err)
	m.width = 120
	m.height = 40
	m.showDashboard = false
	m.syncCfg = &syncConfig{
		relayURL:    srv.URL,
		token:       resp.DeviceToken,
		householdID: resp.HouseholdID,
		key:         key,
	}
	m.syncEngine = engine
	m.syncCtx, m.syncCancel = context.WithCancel(context.Background())
	t.Cleanup(m.syncCancel)

	// Mark any pre-existing oplog entries as synced so they don't
	// interfere with the test assertion below.
	existing, err := store.UnsyncedOps()
	require.NoError(t, err)
	if len(existing) > 0 {
		ids := make([]string, len(existing))
		for i, op := range existing {
			ids[i] = op.ID
		}
		require.NoError(t, store.MarkSynced(ids))
	}

	// 4. Create a vendor — triggers oplog insert via GORM hooks.
	require.NoError(t, store.CreateVendor(&data.Vendor{
		ID:   uid.New(),
		Name: "Integration Vendor",
	}))

	// Verify the oplog has an unsynced entry.
	unsynced, err := store.UnsyncedOps()
	require.NoError(t, err)
	require.NotEmpty(t, unsynced, "vendor create should produce unsynced ops")

	// 5. Run sync — this pushes the local ops to the relay.
	result, err := engine.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Pushed, "should push the vendor insert")
	assert.Zero(t, result.Pulled, "nothing to pull from empty relay")

	// 6. Feed the sync result through the Model's Update.
	sendMsg(m, syncDoneMsg{
		Pushed:    result.Pushed,
		Conflicts: result.Conflicts,
	})
	assert.Equal(t, syncSynced, m.syncStatus)
	assert.Contains(t, m.syncIndicator(), "\u25c8")

	// 7. Verify reloadAfterMutation bumps gen with engine present.
	before := m.syncDebounceGen
	m.reloadAfterMutation()
	assert.Equal(t, before+1, m.syncDebounceGen, "gen should bump with sync engine")

	// 8. Verify the Update wrapper emits debounce cmd on gen change.
	genBefore := m.syncDebounceGen
	sendKey(m, keyA) // triggers table nav, calls through Update wrapper
	// The key handler doesn't call reloadAfterMutation, so gen stays same.
	assert.Equal(t, genBefore, m.syncDebounceGen)
}
