// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, templateBytes, 0o600))
	store, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store.db
}

func TestFindOrCreateVendorNewVendor(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	v, err := findOrCreateVendor(db, Vendor{Name: "New Plumber"})
	require.NoError(t, err)
	assert.NotZero(t, v.ID)
	assert.Equal(t, "New Plumber", v.Name)
}

func TestFindOrCreateVendorExistingClearsFields(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	require.NoError(t, db.Create(&Vendor{Name: "Existing Co", Phone: "555-0000"}).Error)

	// Passing empty contact fields clears them on the existing vendor.
	v, err := findOrCreateVendor(db, Vendor{Name: "Existing Co"})
	require.NoError(t, err)

	var reloaded Vendor
	require.NoError(t, db.First(&reloaded, v.ID).Error)
	assert.Empty(t, reloaded.Phone, "empty phone should clear existing value")
}

func TestFindOrCreateVendorExistingPreservesWhenPassedThrough(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	require.NoError(t, db.Create(&Vendor{
		Name: "Preserve Co", Phone: "555-0000", Notes: "keep me",
	}).Error)

	// Passing the existing values back preserves them.
	v, err := findOrCreateVendor(db, Vendor{
		Name:  "Preserve Co",
		Phone: "555-0000",
		Notes: "keep me",
	})
	require.NoError(t, err)

	var reloaded Vendor
	require.NoError(t, db.First(&reloaded, v.ID).Error)
	assert.Equal(t, "555-0000", reloaded.Phone)
	assert.Equal(t, "keep me", reloaded.Notes)
}

func TestFindOrCreateVendorExistingWithUpdates(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	require.NoError(t, db.Create(&Vendor{Name: "Update Co"}).Error)

	v, err := findOrCreateVendor(db, Vendor{
		Name:        "Update Co",
		ContactName: "Alice",
		Email:       "alice@update.co",
		Phone:       "555-1111",
		Website:     "https://update.co",
		Notes:       "great vendor",
	})
	require.NoError(t, err)

	var reloaded Vendor
	require.NoError(t, db.First(&reloaded, v.ID).Error)
	assert.Equal(t, "Alice", reloaded.ContactName)
	assert.Equal(t, "alice@update.co", reloaded.Email)
	assert.Equal(t, "555-1111", reloaded.Phone)
	assert.Equal(t, "https://update.co", reloaded.Website)
	assert.Equal(t, "great vendor", reloaded.Notes)
}

func TestFindOrCreateVendorReturnedValueReflectsUpdates(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	require.NoError(t, db.Create(&Vendor{
		Name:  "Stale Co",
		Phone: "111",
		Email: "old@stale.co",
	}).Error)

	// The returned vendor must carry the new contact fields, not the
	// pre-update values from the initial lookup.
	v, err := findOrCreateVendor(db, Vendor{
		Name:        "Stale Co",
		Phone:       "222",
		Email:       "new@stale.co",
		ContactName: "Bob",
		Website:     "https://stale.co",
		Notes:       "updated notes",
	})
	require.NoError(t, err)

	assert.Equal(t, "222", v.Phone, "returned vendor should have updated phone")
	assert.Equal(t, "new@stale.co", v.Email, "returned vendor should have updated email")
	assert.Equal(t, "Bob", v.ContactName, "returned vendor should have updated contact name")
	assert.Equal(t, "https://stale.co", v.Website, "returned vendor should have updated website")
	assert.Equal(t, "updated notes", v.Notes, "returned vendor should have updated notes")
}

func TestFindOrCreateVendorEmptyNameReturnsError(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	_, err := findOrCreateVendor(db, Vendor{Name: ""})
	assert.Error(t, err)
}

func TestFindOrCreateVendorWhitespaceNameReturnsError(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	_, err := findOrCreateVendor(db, Vendor{Name: "   "})
	assert.Error(t, err)
}
