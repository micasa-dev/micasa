// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDbPathFromEnvOrArg(t *testing.T) {
	t.Run("positional arg takes precedence", func(t *testing.T) {
		t.Setenv("MICASA_DB_PATH", "/env/path.db")
		got := dbPathFromEnvOrArg([]string{"/arg/path.db"})
		assert.Equal(t, "/arg/path.db", got)
	})

	t.Run("falls back to env var", func(t *testing.T) {
		t.Setenv("MICASA_DB_PATH", "/env/path.db")
		got := dbPathFromEnvOrArg(nil)
		assert.Equal(t, "/env/path.db", got)
	})

	t.Run("returns empty when neither set", func(t *testing.T) {
		t.Setenv("MICASA_DB_PATH", "")
		got := dbPathFromEnvOrArg(nil)
		assert.Empty(t, got)
	})
}

func TestResolveDBPathArg(t *testing.T) {
	t.Parallel()

	t.Run("explicit path returned as-is", func(t *testing.T) {
		t.Parallel()
		got, err := resolveDBPathArg("/tmp/test.db")
		require.NoError(t, err)
		assert.Equal(t, "/tmp/test.db", got)
	})

	t.Run("tilde expanded", func(t *testing.T) {
		t.Parallel()
		got, err := resolveDBPathArg("~/test.db")
		require.NoError(t, err)
		home, _ := os.UserHomeDir()
		assert.Equal(t, home+"/test.db", got)
	})

	t.Run("empty falls back to default", func(t *testing.T) {
		t.Parallel()
		got, err := resolveDBPathArg("")
		require.NoError(t, err)
		assert.NotEmpty(t, got)
	})
}

func TestRunProJoinInvalidCodeFormat(t *testing.T) {
	t.Parallel()

	t.Run("no dot separator", func(t *testing.T) {
		t.Parallel()
		err := runProJoin("NODOTSHERE", ":memory:", defaultRelayURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid invite code format")
	})

	t.Run("empty household ID", func(t *testing.T) {
		t.Parallel()
		err := runProJoin(".CODE", ":memory:", defaultRelayURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both household ID and code must be non-empty")
	})

	t.Run("empty code", func(t *testing.T) {
		t.Parallel()
		err := runProJoin("HOUSEHOLD.", ":memory:", defaultRelayURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both household ID and code must be non-empty")
	})
}

func TestRunProInitAlreadyInitialized(t *testing.T) {
	t.Parallel()

	// Use a file-backed DB so both openAndMigrate calls see the same state.
	dbPath := t.TempDir() + "/test.db"
	store, err := openAndMigrate(dbPath)
	require.NoError(t, err)

	// Trigger lazy SyncDevice creation, then set a household ID.
	_ = store.DeviceID()
	require.NoError(t, store.UpdateSyncDevice(map[string]any{
		"household_id": "01HOUSEHOLD",
	}))
	// Close setup store before runProInit opens its own connection.
	require.NoError(t, store.Close())

	// runProInit should reject re-initialization.
	err = runProInit(dbPath, "http://localhost:0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already initialized")
}

func TestRunProJoinAlreadyInHousehold(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir() + "/test.db"
	store, err := openAndMigrate(dbPath)
	require.NoError(t, err)

	_ = store.DeviceID()
	require.NoError(t, store.UpdateSyncDevice(map[string]any{
		"household_id": "01EXISTING",
	}))
	// Close setup store before runProJoin opens its own connection.
	require.NoError(t, store.Close())

	err = runProJoin("01OTHER.invitecode", dbPath, defaultRelayURL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already in household")
}

func TestResolveRelayURL(t *testing.T) {
	t.Run("flag takes precedence", func(t *testing.T) {
		t.Setenv("MICASA_RELAY_URL", "https://env.example.com")
		got := resolveRelayURL("https://flag.example.com", true)
		assert.Equal(t, "https://flag.example.com", got)
	})

	t.Run("env var when flag unchanged", func(t *testing.T) {
		t.Setenv("MICASA_RELAY_URL", "https://env.example.com")
		got := resolveRelayURL(defaultRelayURL, false)
		assert.Equal(t, "https://env.example.com", got)
	})

	t.Run("default when neither set", func(t *testing.T) {
		t.Setenv("MICASA_RELAY_URL", "")
		got := resolveRelayURL(defaultRelayURL, false)
		assert.Equal(t, defaultRelayURL, got)
	})
}

func TestProCommandTree(t *testing.T) {
	t.Parallel()

	root := newProCmd()

	// Verify all expected subcommands exist.
	subNames := make([]string, 0, len(root.Commands()))
	for _, c := range root.Commands() {
		subNames = append(subNames, c.Name())
	}

	assert.Contains(t, subNames, "init")
	assert.Contains(t, subNames, "status")
	assert.Contains(t, subNames, "storage")
	assert.Contains(t, subNames, "sync")
	assert.Contains(t, subNames, "invite")
	assert.Contains(t, subNames, "join")
	assert.Contains(t, subNames, "devices")
	assert.Contains(t, subNames, "conflicts")

	// Verify devices has a revoke subcommand.
	var devicesCmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "devices" {
			devicesCmd = c
			break
		}
	}
	require.NotNil(t, devicesCmd)

	var revokeFound bool
	for _, c := range devicesCmd.Commands() {
		if c.Name() == "revoke" {
			revokeFound = true
			break
		}
	}
	assert.True(t, revokeFound, "devices should have a revoke subcommand")
}

func TestProHelpText(t *testing.T) {
	t.Parallel()

	cmd := newProCmd()
	assert.Contains(t, cmd.Long, "Encrypted multi-device sync")
	assert.Contains(t, cmd.Long, "micasa pro init")
	assert.NotEmpty(t, cmd.Example)
}

func TestRunProConflictsEmpty(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir() + "/test.db"
	store, err := openAndMigrate(dbPath)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	var buf bytes.Buffer
	err = runProConflicts(&buf, dbPath)
	require.NoError(t, err)
	assert.Empty(t, buf.String(), "no conflicts should produce no output")
}

func TestRunProConflictsWithLosers(t *testing.T) {
	t.Parallel()

	dbPath := t.TempDir() + "/test.db"
	store, err := openAndMigrate(dbPath)
	require.NoError(t, err)

	// Insert a conflict loser directly: synced but not applied.
	now := time.Now()
	db := store.GormDB()
	require.NoError(t, db.Table("sync_oplog_entries").Create(map[string]any{
		"id":         "conflict-1",
		"table_name": "vendors",
		"row_id":     "vendor-abc",
		"op_type":    "update",
		"payload":    `{"name":"Remote Vendor"}`,
		"device_id":  "dev-remote-1",
		"created_at": now,
		"applied_at": nil,
		"synced_at":  now,
	}).Error)

	require.NoError(t, store.Close())

	var buf bytes.Buffer
	err = runProConflicts(&buf, dbPath)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "conflict-1")
	assert.Contains(t, out, "vendors")
	assert.Contains(t, out, "vendor-abc")
	assert.Contains(t, out, "update")
	assert.Contains(t, out, "dev-remote-1")
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "Zero", bytes: 0, want: "0 B"},
		{name: "SmallBytes", bytes: 512, want: "512 B"},
		{name: "OneKiB", bytes: 1024, want: "1.0 KiB"},
		{name: "OneMiB", bytes: 1024 * 1024, want: "1.0 MiB"},
		{name: "OneGiB", bytes: 1024 * 1024 * 1024, want: "1.0 GiB"},
		{name: "FractionalMiB", bytes: 54 * 1024 * 1024, want: "54 MiB"},
		{name: "LargeGiB", bytes: 10 * 1024 * 1024 * 1024, want: "10 GiB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, formatBytes(tt.bytes))
		})
	}
}

func TestFormatStorageUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		used  int64
		quota int64
		want  string
	}{
		{
			name:  "ZeroUsed",
			used:  0,
			quota: 1024 * 1024 * 1024,
			want:  "0 B / 1.0 GiB (0.0%)",
		},
		{
			name:  "HalfUsed",
			used:  512 * 1024 * 1024,
			quota: 1024 * 1024 * 1024,
			want:  "512 MiB / 1.0 GiB (50.0%)",
		},
		{
			name:  "FullUsed",
			used:  1024 * 1024 * 1024,
			quota: 1024 * 1024 * 1024,
			want:  "1.0 GiB / 1.0 GiB (100.0%)",
		},
		{
			name:  "SmallFraction",
			used:  54 * 1024 * 1024,
			quota: 1024 * 1024 * 1024,
			want:  "54 MiB / 1.0 GiB (5.3%)",
		},
		{
			name:  "ZeroQuota",
			used:  0,
			quota: 0,
			want:  "0 B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, formatStorageUsage(tt.used, tt.quota))
		})
	}
}

func TestFormatStorageUsageUnlimited(t *testing.T) {
	t.Parallel()
	result := formatStorageUsage(52428800, 0) // 50 MiB, unlimited
	assert.NotContains(t, result, "/")
	assert.NotContains(t, result, "%")
	assert.Contains(t, result, "50 MiB")
}

func TestFormatStorageUsageNegativeQuota(t *testing.T) {
	t.Parallel()
	result := formatStorageUsage(1024, -1)
	assert.NotContains(t, result, "/")
	assert.NotContains(t, result, "%")
	assert.Equal(t, "1.0 KiB", result)
}

func TestProStorageCmdWiring(t *testing.T) {
	t.Parallel()

	root := newRootCmd()
	proCmd, _, err := root.Find([]string{"pro"})
	assert.NoError(t, err)
	assert.NotNil(t, proCmd)

	storageCmd, _, err := root.Find([]string{"pro", "storage"})
	assert.NoError(t, err)
	assert.NotNil(t, storageCmd)
	assert.Equal(t, "storage [database-path]", storageCmd.Use)
}
