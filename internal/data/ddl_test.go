// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableDDL_KnownTables(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ddl, err := store.TableDDL("vendors", "documents")
	require.NoError(t, err)
	require.Len(t, ddl, 2)
	assert.Contains(t, ddl["vendors"], "CREATE TABLE")
	assert.Contains(t, ddl["vendors"], "name")
	assert.Contains(t, ddl["documents"], "CREATE TABLE")
	assert.Contains(t, ddl["documents"], "entity_kind")
}

func TestTableDDL_UnknownTable(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ddl, err := store.TableDDL("nonexistent_table")
	require.NoError(t, err)
	assert.Empty(t, ddl)
}

func TestTableDDL_MixedKnownAndUnknown(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ddl, err := store.TableDDL("vendors", "nonexistent")
	require.NoError(t, err)
	require.Len(t, ddl, 1)
	assert.Contains(t, ddl, "vendors")
}

func TestTableDDL_Empty(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ddl, err := store.TableDDL()
	require.NoError(t, err)
	assert.Empty(t, ddl)
}
