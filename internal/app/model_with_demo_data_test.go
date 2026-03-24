// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/fake"
	"github.com/micasa-dev/micasa/internal/locale"
	"github.com/stretchr/testify/require"
)

// newTestModelWithDemoData creates a Model backed by a real SQLite store,
// seeded with randomized demo data from the given HomeFaker. This provides
// richer test scenarios than newTestModelWithStore (which has only defaults).
func newTestModelWithDemoData(t *testing.T, seed uint64) *Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, templateBytes, 0o600))
	store, err := data.Open(path)
	require.NoError(t, err)
	require.NoError(t, store.SetMaxDocumentSize(50<<20))
	t.Cleanup(func() { _ = store.Close() })
	store.SetCurrency(locale.DefaultCurrency())
	h := fake.New(seed)
	require.NoError(t, store.SeedDemoDataFrom(h))
	m, err := NewModel(store, Options{DBPath: path})
	require.NoError(t, err)
	m.width = 120
	m.height = 40
	if m.mode == modeForm {
		m.exitForm()
	}
	m.showDashboard = false
	return m
}
