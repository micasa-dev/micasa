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
	"golang.org/x/text/language"
)

// testModelOpts controls how newTestModelWith initialises the test Model.
//
//   - withDemo / seed: seed demo data via fake.New(seed); skips house-profile
//     creation (demo seeder creates its own).
//   - currency / currencyTag: resolve a specific currency; when currency is
//     empty the store default (locale.DefaultCurrency) is used.
type testModelOpts struct {
	// Demo data
	withDemo bool
	seed     uint64

	// Currency (empty = use locale.DefaultCurrency)
	currency    string
	currencyTag language.Tag
}

// newTestModelWith is the single parametric factory for fully-wired test
// Models backed by a real SQLite store. Callers that need a specific variant
// should use one of the thin wrappers below.
func newTestModelWith(t *testing.T, opts testModelOpts) *Model {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, templateBytes, 0o600))
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Currency setup.
	if opts.currency != "" {
		cur := locale.MustResolve(opts.currency, opts.currencyTag)
		store.SetCurrency(cur)
		require.NoError(t, store.PutCurrency(opts.currency))
	} else {
		store.SetCurrency(locale.DefaultCurrency())
		// Currency-only tests don't exercise document uploads, so skip
		// SetMaxDocumentSize to match the old newTestModelWithCurrency behavior.
		require.NoError(t, store.SetMaxDocumentSize(50<<20))
	}

	// Seed data.
	if opts.withDemo {
		h := fake.New(opts.seed)
		require.NoError(t, store.SeedDemoDataFrom(h))
	} else {
		require.NoError(t, store.CreateHouseProfile(data.HouseProfile{
			Nickname: "Test House",
		}))
	}

	m, err := NewModel(store, Options{DBPath: path})
	require.NoError(t, err)
	m.width = 120
	m.height = 40

	// Demo data may trigger the house form; dismiss it for a clean slate.
	if m.mode == modeForm {
		m.exitForm()
	}
	m.showDashboard = false
	return m
}
