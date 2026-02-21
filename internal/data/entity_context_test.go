// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityNames_Empty(t *testing.T) {
	store := newTestStore(t)
	vendors, projects, appliances, err := store.EntityNames()
	require.NoError(t, err)
	assert.Empty(t, vendors)
	assert.Empty(t, projects)
	assert.Empty(t, appliances)
}

func TestEntityNames_ReturnsActiveOnly(t *testing.T) {
	store := newTestStore(t)

	// Seed some data.
	require.NoError(t, store.SeedDemoData())

	vendors, projects, appliances, err := store.EntityNames()
	require.NoError(t, err)
	assert.NotEmpty(t, vendors)
	assert.NotEmpty(t, projects)
	assert.NotEmpty(t, appliances)

	// All should be sorted alphabetically.
	for i := 1; i < len(vendors); i++ {
		assert.LessOrEqual(t, vendors[i-1], vendors[i], "vendors should be sorted")
	}
}
