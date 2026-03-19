// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityRows_Empty(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ctx, err := store.EntityRows()
	require.NoError(t, err)
	assert.Empty(t, ctx.Vendors)
	assert.Empty(t, ctx.Projects)
	assert.Empty(t, ctx.Appliances)
	// Categories and types are seeded by newTestStore -> AutoMigrate + SeedDefaults.
	assert.NotEmpty(t, ctx.MaintenanceCategories)
	assert.NotEmpty(t, ctx.ProjectTypes)
}

func TestEntityRows_WithDemoData(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithDemoData(t, testSeed)
	ctx, err := store.EntityRows()
	require.NoError(t, err)

	assert.NotEmpty(t, ctx.Vendors)
	assert.NotEmpty(t, ctx.Projects)
	assert.NotEmpty(t, ctx.Appliances)
	assert.NotEmpty(t, ctx.MaintenanceCategories)
	assert.NotEmpty(t, ctx.ProjectTypes)

	// Every row should have a nonzero ID and non-empty name.
	for _, r := range ctx.Vendors {
		assert.NotEmpty(t, r.ID)
		assert.NotEmpty(t, r.Name)
	}
	for _, r := range ctx.Projects {
		assert.NotEmpty(t, r.ID)
		assert.NotEmpty(t, r.Name)
	}
	for _, r := range ctx.Appliances {
		assert.NotEmpty(t, r.ID)
		assert.NotEmpty(t, r.Name)
	}
}

func TestEntityRows_ExcludesDeleted(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithDemoData(t, testSeed)

	// Get initial counts.
	before, err := store.EntityRows()
	require.NoError(t, err)
	require.NotEmpty(t, before.Vendors)

	// Pick the first vendor and remove its dependents so we can delete it.
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	target := vendors[0]

	quotes, err := store.ListQuotesByVendor(target.ID, false)
	require.NoError(t, err)
	for _, q := range quotes {
		require.NoError(t, store.DeleteQuote(q.ID))
	}

	incidents, err := store.ListIncidents(false)
	require.NoError(t, err)
	for _, inc := range incidents {
		if inc.VendorID != nil && *inc.VendorID == target.ID {
			require.NoError(t, store.DeleteIncident(inc.ID))
		}
	}

	require.NoError(t, store.DeleteVendor(target.ID))
	deletedID := target.ID

	after, err := store.EntityRows()
	require.NoError(t, err)
	assert.Len(t, after.Vendors, len(before.Vendors)-1)

	// The deleted vendor should not appear.
	for _, r := range after.Vendors {
		assert.NotEqual(t, deletedID, r.ID)
	}
}

func TestEntityRows_SortedByName(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithDemoData(t, testSeed)
	ctx, err := store.EntityRows()
	require.NoError(t, err)

	for i := 1; i < len(ctx.Vendors); i++ {
		assert.LessOrEqual(t, ctx.Vendors[i-1].Name, ctx.Vendors[i].Name)
	}
	for i := 1; i < len(ctx.ProjectTypes); i++ {
		assert.LessOrEqual(t, ctx.ProjectTypes[i-1].Name, ctx.ProjectTypes[i].Name)
	}
}
