// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"path/filepath"
	"testing"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/fake"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/require"
)

// newTestModelWithDemoData creates a Model backed by a real SQLite store,
// seeded with randomized demo data from the given HomeFaker. This provides
// richer test scenarios than newTestModelWithStore (which has only defaults).
func newTestModelWithDemoData(t *testing.T, seed uint64) *Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())
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
