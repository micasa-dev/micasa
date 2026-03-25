// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/micasa-dev/micasa/internal/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupCreatesValidCopy(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithDemoData(t, testSeed)

	destPath := filepath.Join(t.TempDir(), "backup.db")
	require.NoError(t, store.Backup(t.Context(), destPath))

	// Open the backup and verify row counts match the source.
	backup, err := Open(destPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = backup.Close() })

	srcVendors, err := store.ListVendors(false)
	require.NoError(t, err)
	dstVendors, err := backup.ListVendors(false)
	require.NoError(t, err)
	assert.Len(t, dstVendors, len(srcVendors), "vendor count mismatch")

	srcProjects, err := store.ListProjects(false)
	require.NoError(t, err)
	dstProjects, err := backup.ListProjects(false)
	require.NoError(t, err)
	assert.Len(t, dstProjects, len(srcProjects), "project count mismatch")

	srcAppliances, err := store.ListAppliances(false)
	require.NoError(t, err)
	dstAppliances, err := backup.ListAppliances(false)
	require.NoError(t, err)
	assert.Len(t, dstAppliances, len(srcAppliances), "appliance count mismatch")

	srcMaint, err := store.ListMaintenance(false)
	require.NoError(t, err)
	dstMaint, err := backup.ListMaintenance(false)
	require.NoError(t, err)
	assert.Len(t, dstMaint, len(srcMaint), "maintenance count mismatch")

	srcIncidents, err := store.ListIncidents(false)
	require.NoError(t, err)
	dstIncidents, err := backup.ListIncidents(false)
	require.NoError(t, err)
	assert.Len(t, dstIncidents, len(srcIncidents), "incident count mismatch")
}

func TestBackupDestAlreadyExists(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	destPath := filepath.Join(t.TempDir(), "existing.db")
	require.NoError(t, os.WriteFile(destPath, []byte("placeholder"), 0o600))

	err := store.Backup(t.Context(), destPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup")
}

func TestBackupMemoryDB(t *testing.T) {
	t.Parallel()
	// Open a template-backed store, seed demo data, then back it up.
	srcPath := filepath.Join(t.TempDir(), "src.db")
	require.NoError(t, os.WriteFile(srcPath, templateBytes, 0o600))
	store, err := Open(srcPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.SeedDemoDataFrom(fake.New(testSeed)))

	destPath := filepath.Join(t.TempDir(), "mem-backup.db")
	require.NoError(t, store.Backup(t.Context(), destPath))

	// Verify the backup is a valid database with the expected tables.
	backup, err := Open(destPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = backup.Close() })

	vendors, err := backup.ListVendors(false)
	require.NoError(t, err)
	assert.NotEmpty(t, vendors, "backup of in-memory DB should contain vendors")

	projects, err := backup.ListProjects(false)
	require.NoError(t, err)
	assert.NotEmpty(t, projects, "backup of in-memory DB should contain projects")
}

func TestIsMicasaDB_True(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ok, err := store.IsMicasaDB()
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIsMicasaDB_False(t *testing.T) {
	t.Parallel()
	// A freshly opened database with no migrations has no micasa tables.
	store, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	ok, err := store.IsMicasaDB()
	require.NoError(t, err)
	assert.False(t, ok)
}
