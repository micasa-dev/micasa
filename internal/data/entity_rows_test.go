// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityRows_Empty(t *testing.T) {
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
		assert.NotZero(t, r.ID)
		assert.NotEmpty(t, r.Name)
	}
	for _, r := range ctx.Projects {
		assert.NotZero(t, r.ID)
		assert.NotEmpty(t, r.Name)
	}
	for _, r := range ctx.Appliances {
		assert.NotZero(t, r.ID)
		assert.NotEmpty(t, r.Name)
	}
}

func TestEntityRows_ExcludesDeleted(t *testing.T) {
	store := newTestStoreWithDemoData(t, testSeed)

	// Get initial counts.
	before, err := store.EntityRows()
	require.NoError(t, err)
	require.NotEmpty(t, before.Vendors)

	// Soft-delete a vendor (pick one without dependents).
	// Find a vendor with no quotes or incidents.
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	var deletedID uint
	for _, v := range vendors {
		if err := store.DeleteVendor(v.ID); err == nil {
			deletedID = v.ID
			break
		}
	}
	require.NotZero(t, deletedID, "should have deleted at least one vendor")

	after, err := store.EntityRows()
	require.NoError(t, err)
	assert.Len(t, after.Vendors, len(before.Vendors)-1)

	// The deleted vendor should not appear.
	for _, r := range after.Vendors {
		assert.NotEqual(t, deletedID, r.ID)
	}
}

func TestEntityRows_SortedByName(t *testing.T) {
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
