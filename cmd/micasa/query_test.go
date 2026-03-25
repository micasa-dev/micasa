// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/micasa-dev/micasa/internal/data"
)

func TestQueryText(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Acme"}))

	var buf bytes.Buffer
	err := runQuery(t.Context(), &buf, store, "SELECT name FROM vendors", false)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "name")
	assert.Contains(t, out, "Acme")
}

func TestQueryJSON(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Acme"}))

	var buf bytes.Buffer
	err := runQuery(t.Context(), &buf, store, "SELECT name FROM vendors", true)
	require.NoError(t, err)

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "Acme", result[0]["name"])
}

func TestQueryRejectsMutation(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	err := runQuery(t.Context(), &buf, store, "DELETE FROM vendors", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only SELECT queries are allowed")
}

func TestQueryEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	var buf bytes.Buffer
	err := runQuery(t.Context(), &buf, store, "SELECT name FROM vendors", false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "name")
}
