// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStoreWithMigration(t *testing.T) *data.Store {
	t.Helper()
	store, err := data.Open(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestShowHouseText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{
		Nickname:     "Test House",
		AddressLine1: "123 Main St",
		City:         "Springfield",
		State:        "IL",
		PostalCode:   "62701",
		YearBuilt:    1985,
		SquareFeet:   2400,
		Bedrooms:     3,
		Bathrooms:    2.5,
	}))

	var buf bytes.Buffer
	err := runShow(&buf, store, "house", false, false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "=== HOUSE ===")
	assert.Contains(t, out, "Nickname:")
	assert.Contains(t, out, "Test House")
	assert.Contains(t, out, "123 Main St")
	assert.Contains(t, out, "Springfield, IL 62701")
}

func TestShowHouseJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{
		Nickname: "Test House",
	}))

	var buf bytes.Buffer
	err := runShow(&buf, store, "house", true, false)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "Test House", result["nickname"])
}

func TestShowHouseEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	err := runShow(&buf, store, "house", false, false)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestShowUnknownEntity(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	err := runShow(&buf, store, "bogus", false, false)
	require.Error(t, err)
	require.ErrorContains(t, err, "unknown entity")
	require.ErrorContains(t, err, "bogus")
}
