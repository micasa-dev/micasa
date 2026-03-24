// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/locale"
	"github.com/stretchr/testify/require"
)

const testProjectTitle = "Test Project"

// newTestModelWithStore creates a Model backed by a real SQLite store with
// seeded defaults (project types, maintenance categories). The model is sized
// to 120x40 and starts in normal mode (dashboard and house form dismissed).
func newTestModelWithStore(t *testing.T) *Model {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, templateBytes, 0o600))
	store, err := data.Open(path)
	require.NoError(t, err)
	require.NoError(t, store.SetMaxDocumentSize(50<<20))
	t.Cleanup(func() { _ = store.Close() })

	store.SetCurrency(locale.DefaultCurrency())

	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{
		Nickname: "Test House",
	}))

	m, err := NewModel(store, Options{DBPath: path})
	require.NoError(t, err)
	m.width = 120
	m.height = 40
	m.showDashboard = false
	return m
}
