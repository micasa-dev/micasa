// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/data/datatest"
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

	// Demo models start from seeded demo data; others start with only a
	// house profile.
	dbBytes, err := houseTemplateBytes()
	if opts.withDemo {
		dbBytes, err = demoTemplateBytes(opts.seed)
	}
	require.NoError(t, err)
	store := datatest.OpenMemory(t, dbBytes)

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

	m, err := NewModel(store, Options{DBPath: testDBPath})
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
