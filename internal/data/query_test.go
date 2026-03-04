// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableNames(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	names, err := store.TableNames()
	require.NoError(t, err)

	// Should include our core tables.
	assert.Contains(t, names, TableHouseProfiles)
	assert.Contains(t, names, TableProjects)
	assert.Contains(t, names, TableVendors)
	assert.Contains(t, names, TableMaintenanceItems)
	assert.Contains(t, names, TableAppliances)

	// Should not include sqlite internals.
	for _, name := range names {
		assert.NotContains(t, name, "sqlite_")
	}
}

func TestTableColumns(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cols, err := store.TableColumns(TableProjects)
	require.NoError(t, err)
	assert.NotEmpty(t, cols)

	// Check that the id column is a PK.
	var foundID bool
	for _, col := range cols {
		if col.Name == ColID {
			foundID = true
			assert.Positive(t, col.PK)
		}
	}
	assert.True(t, foundID, "expected to find id column")
}

func TestTableColumnsInvalidName(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	_, err := store.TableColumns("'; DROP TABLE projects; --")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid table name")
}

func TestReadOnlyQuerySelect(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cols, rows, err := store.ReadOnlyQuery("SELECT name FROM project_types ORDER BY name LIMIT 3")
	require.NoError(t, err)
	assert.Equal(t, []string{"name"}, cols)
	assert.Len(t, rows, 3)
}

func TestReadOnlyQueryRejectsInsert(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, _, err := store.ReadOnlyQuery("INSERT INTO projects (title) VALUES ('hack')")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only SELECT")
}

func TestReadOnlyQueryRejectsDelete(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, _, err := store.ReadOnlyQuery("DELETE FROM projects WHERE id = 1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only SELECT")
}

func TestReadOnlyQueryRejectsMultiStatement(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, _, err := store.ReadOnlyQuery("SELECT * FROM projects; DROP TABLE projects")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple statements")
}

func TestReadOnlyQueryRejectsAttach(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, _, err := store.ReadOnlyQuery("SELECT * FROM (SELECT 1) ATTACH DATABASE '/tmp/x' AS x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disallowed keyword: ATTACH")
}

func TestReadOnlyQueryRejectsPragma(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, _, err := store.ReadOnlyQuery(
		"SELECT * FROM pragma_table_info('projects') WHERE 1=1 PRAGMA journal_mode",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disallowed keyword: PRAGMA")
}

func TestReadOnlyQueryEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, _, err := store.ReadOnlyQuery("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestReadOnlyQueryAllowsDeletedAtColumn(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	// "deleted_at" contains "DELETE" as a substring but should be allowed.
	cols, _, err := store.ReadOnlyQuery(
		"SELECT id FROM projects WHERE deleted_at IS NULL LIMIT 1",
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"id"}, cols)
}

func TestContainsWord(t *testing.T) {
	t.Parallel()
	assert.True(t, containsWord("SELECT * DELETE FROM", "DELETE"))
	assert.False(t, containsWord("WHERE DELETED_AT IS NULL", "DELETE"))
	assert.True(t, containsWord("DROP TABLE x", "DROP"))
	assert.False(t, containsWord("BACKDROP", "DROP"))
}

func TestIsSafeIdentifier(t *testing.T) {
	t.Parallel()
	assert.True(t, isSafeIdentifier(TableProjects))
	assert.True(t, isSafeIdentifier(TableHouseProfiles))
	assert.True(t, isSafeIdentifier("table123"))
	assert.False(t, isSafeIdentifier(""))
	assert.False(t, isSafeIdentifier("table; DROP"))
	assert.False(t, isSafeIdentifier("table'name"))
}

func TestDataDump(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithDemoData(t, testSeed)

	dump := store.DataDump()
	assert.NotEmpty(t, dump)
	// Should contain table headers with row counts.
	assert.Contains(t, dump, "rows)")
	// Should include actual data rows as bullet points.
	assert.Contains(t, dump, "- ")
}

func TestDataDumpExcludesSoftDeletedRecords(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create a vendor, then soft-delete it. The LLM dump should NOT mention it.
	v := Vendor{Name: "DeletedVendorXYZ"}
	require.NoError(t, store.db.Create(&v).Error)
	require.NoError(t, store.db.Delete(&v).Error) // soft delete

	// Create a non-deleted vendor to verify the dump still works.
	require.NoError(t, store.db.Create(&Vendor{Name: "ActiveVendorABC"}).Error)

	dump := store.DataDump()
	assert.NotContains(t, dump, "DeletedVendorXYZ",
		"soft-deleted vendor should not appear in DataDump")
	assert.Contains(t, dump, "ActiveVendorABC",
		"active vendor should appear in DataDump")
}

func TestColumnHints(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithDemoData(t, testSeed)

	hints := store.ColumnHints()
	assert.NotEmpty(t, hints)
	// Should include project statuses (seeded by demo data).
	assert.Contains(t, hints, "project statuses")
	// Should include project types from SeedDefaults.
	assert.Contains(t, hints, "project types")
	// Each line is a bullet.
	assert.Contains(t, hints, "- ")
}

func TestColumnHintsEmptyDB(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	// Empty DB (no SeedDefaults) should still not panic.
	hints := store.ColumnHints()
	// May be empty or have only categories from migration.
	assert.NotContains(t, hints, "vendor names")
}
