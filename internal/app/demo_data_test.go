// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelWithDemoDataLoadsAllTabs(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, testSeed)

	for i, tab := range m.tabs {
		assert.NotEmptyf(
			t,
			tab.Table.Rows(),
			"tab %d (%s) has no rows after demo data seed",
			i,
			tab.Name,
		)
	}
}

func TestModelWithDemoDataDashboard(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, testSeed)
	m.showDashboard = true
	require.NoError(t, m.loadDashboardAt(time.Now()))

	assert.NotZero(t, m.dashNavCount(), "expected dashboard nav entries after demo data seed")
}

func TestModelWithDemoDataVariedSeeds(t *testing.T) {
	t.Parallel()
	for i := range uint64(5) {
		seed := testSeed + i
		m := newTestModelWithDemoData(t, seed)
		require.NotNilf(t, m, "seed %d: nil model", seed)
		totalRows := 0
		for _, tab := range m.tabs {
			totalRows += len(tab.Table.Rows())
		}
		assert.NotZerof(t, totalRows, "seed %d: no rows in any tab", seed)
	}
}
