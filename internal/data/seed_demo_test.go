// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedDemoDataPopulatesAllEntities(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithDemoData(t, testSeed)

	house, err := store.HouseProfile()
	require.NoError(t, err)
	assert.NotEmpty(t, house.Nickname)
	assert.NotZero(t, house.YearBuilt)

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	assert.NotEmpty(t, vendors)

	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	assert.NotEmpty(t, projects)

	appliances, err := store.ListAppliances(false)
	require.NoError(t, err)
	assert.NotEmpty(t, appliances)

	maint, err := store.ListMaintenance(false)
	require.NoError(t, err)
	assert.NotEmpty(t, maint)
}

func TestSeedDemoDataDeterministic(t *testing.T) {
	t.Parallel()
	store1 := newTestStoreWithDemoData(t, testSeed)
	store2 := newTestStoreWithDemoData(t, testSeed)

	h1, _ := store1.HouseProfile()
	h2, _ := store2.HouseProfile()

	assert.Equal(t, h1.Nickname, h2.Nickname, "same seed should produce identical house names")
}

func TestSeedDemoDataVariety(t *testing.T) {
	t.Parallel()
	names := make(map[string]bool)
	for i := range uint64(5) {
		store := newTestStoreWithDemoData(t, testSeed+i)
		h, _ := store.HouseProfile()
		names[h.Nickname] = true
	}
	assert.GreaterOrEqual(t, len(names), 3, "expected variety across seeds")
}

func TestSeedDemoDataSkipsIfDataExists(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithDemoData(t, testSeed)

	vendors1, _ := store.ListVendors(false)
	count1 := len(vendors1)

	require.NoError(t, store.SeedDemoData())

	vendors2, _ := store.ListVendors(false)
	assert.Len(t, vendors2, count1)
}
