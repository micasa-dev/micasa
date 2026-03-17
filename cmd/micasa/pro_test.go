// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"os"
	"testing"

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
	assert.Contains(t, subNames, "sync")
	assert.Contains(t, subNames, "invite")
	assert.Contains(t, subNames, "join")
	assert.Contains(t, subNames, "devices")

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
