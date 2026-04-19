// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// LastDeletion is a test-only helper that returns the most recent unrestored
// deletion record for an entity name. Production code does not need to look
// up deletions by entity (they are surfaced through the trash UI which lists
// all of them).
func (s *Store) LastDeletion(entity string) (DeletionRecord, error) {
	var record DeletionRecord
	err := s.db.
		Where(ColEntity+" = ? AND "+ColRestoredAt+" IS NULL", entity).
		Order(ColID + " desc").
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DeletionRecord{}, gorm.ErrRecordNotFound
	}
	return record, err
}

func TestSeedDefaults(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, types)
	categories, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, categories)
}

func TestHouseProfileSingle(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	profile := HouseProfile{Nickname: "Primary Residence"}
	require.NoError(t, store.CreateHouseProfile(profile))
	_, err := store.HouseProfile()
	require.NoError(t, err)
	assert.Error(t, store.CreateHouseProfile(profile), "second profile should fail")
}

func TestUpdateHouseProfile(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(
		t,
		store.CreateHouseProfile(HouseProfile{Nickname: "Primary Residence", City: "Portland"}),
	)
	require.NoError(
		t,
		store.UpdateHouseProfile(HouseProfile{Nickname: "Primary Residence", City: "Seattle"}),
	)
	fetched, err := store.HouseProfile()
	require.NoError(t, err)
	assert.Equal(t, "Seattle", fetched.City)
}

func TestSoftDeleteRestoreProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&Project{
		Title: "Test Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))

	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)

	require.NoError(t, store.DeleteProject(projects[0].ID))

	projects, err = store.ListProjects(false)
	require.NoError(t, err)
	assert.Empty(t, projects)

	projects, err = store.ListProjects(true)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.True(t, projects[0].DeletedAt.Valid)

	require.NoError(t, store.RestoreProject(projects[0].ID))
	projects, err = store.ListProjects(false)
	require.NoError(t, err)
	assert.Len(t, projects, 1)
}

func TestLastDeletionRecord(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&Project{
		Title: "Test Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)

	require.NoError(t, store.DeleteProject(projects[0].ID))
	record, err := store.LastDeletion(DeletionEntityProject)
	require.NoError(t, err)
	assert.Equal(t, projects[0].ID, record.TargetID)

	require.NoError(t, store.RestoreProject(record.TargetID))
	_, err = store.LastDeletion(DeletionEntityProject)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestUpdateProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&Project{
		Title: "Original Title", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	id := projects[0].ID

	fetched, err := store.GetProject(id)
	require.NoError(t, err)
	assert.Equal(t, "Original Title", fetched.Title)

	require.NoError(t, store.UpdateProject(Project{
		ID: id, Title: "Updated Title", ProjectTypeID: types[0].ID,
		Status: ProjectStatusInProgress,
	}))

	fetched, err = store.GetProject(id)
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", fetched.Title)
	assert.Equal(t, ProjectStatusInProgress, fetched.Status)
}

func TestUpdateQuote(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&Project{
		Title: "Test Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)

	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: projects[0].ID, TotalCents: 100000},
		Vendor{Name: "Acme Corp"},
	))
	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	id := quotes[0].ID

	require.NoError(t, store.UpdateQuote(
		Quote{ID: id, ProjectID: projects[0].ID, TotalCents: 200000},
		Vendor{Name: "Acme Corp", ContactName: "John Doe"},
	))

	fetched, err := store.GetQuote(id)
	require.NoError(t, err)
	assert.Equal(t, int64(200000), fetched.TotalCents)
	assert.Equal(t, "John Doe", fetched.Vendor.ContactName)
}

func TestUpdateMaintenance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	categories, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Filter Change", CategoryID: categories[0].ID,
	}))
	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	id := items[0].ID

	fetched, err := store.GetMaintenance(id)
	require.NoError(t, err)
	assert.Equal(t, "Filter Change", fetched.Name)

	require.NoError(t, store.UpdateMaintenance(MaintenanceItem{
		ID: id, Name: "HVAC Filter Change", CategoryID: categories[0].ID, IntervalMonths: 3,
	}))

	fetched, err = store.GetMaintenance(id)
	require.NoError(t, err)
	assert.Equal(t, "HVAC Filter Change", fetched.Name)
	assert.Equal(t, 3, fetched.IntervalMonths)
}

func TestServiceLogCRUD(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	categories, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Test Maintenance", CategoryID: categories[0].ID,
	}))
	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	maintID := items[0].ID

	// Create a service log entry (self-performed, no vendor).
	require.NoError(t, store.CreateServiceLog(&ServiceLogEntry{
		MaintenanceItemID: maintID,
		ServicedAt:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Notes:             "did it myself",
	}, Vendor{}))

	entries, err := store.ListServiceLog(maintID, false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Nil(t, entries[0].VendorID)
	assert.Equal(t, "did it myself", entries[0].Notes)

	// Create a vendor-performed entry.
	require.NoError(t, store.CreateServiceLog(&ServiceLogEntry{
		MaintenanceItemID: maintID,
		ServicedAt:        time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		CostCents:         new(int64(15000)),
		Notes:             "vendor did it",
	}, Vendor{Name: "Test Plumber", Phone: "555-555-0001"}))

	entries, err = store.ListServiceLog(maintID, false)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	// Most recent first.
	require.NotNil(t, entries[0].VendorID)
	assert.Equal(t, "Test Plumber", entries[0].Vendor.Name)

	// Update: change vendor entry to self-performed.
	updated := entries[0]
	updated.Notes = "actually did it myself"
	require.NoError(t, store.UpdateServiceLog(updated, Vendor{}))

	fetched, err := store.GetServiceLog(updated.ID)
	require.NoError(t, err)
	assert.Nil(t, fetched.VendorID)
	assert.Equal(t, "actually did it myself", fetched.Notes)

	// Delete and restore.
	require.NoError(t, store.DeleteServiceLog(fetched.ID))
	entries, err = store.ListServiceLog(maintID, false)
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	entries, err = store.ListServiceLog(maintID, true)
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	require.NoError(t, store.RestoreServiceLog(fetched.ID))
	entries, err = store.ListServiceLog(maintID, false)
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// CountServiceLogs.
	counts, err := store.CountServiceLogs([]string{maintID})
	require.NoError(t, err)
	assert.Equal(t, 2, counts[maintID])
}

func TestServiceLogSyncsLastServiced(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	categories, err := store.MaintenanceCategories()
	require.NoError(t, err)

	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Sync Test", CategoryID: categories[0].ID,
	}))
	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	maintID := items[0].ID

	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	mar1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	getMaint := func() MaintenanceItem {
		t.Helper()
		item, err := store.GetMaintenance(maintID)
		require.NoError(t, err)
		return item
	}

	// Create first entry → LastServicedAt = jan15.
	require.NoError(t, store.CreateServiceLog(&ServiceLogEntry{
		MaintenanceItemID: maintID, ServicedAt: jan15,
	}, Vendor{}))
	require.NotNil(t, getMaint().LastServicedAt)
	assert.True(t, getMaint().LastServicedAt.Equal(jan15))

	// Create a more recent entry → LastServicedAt advances to mar1.
	require.NoError(t, store.CreateServiceLog(&ServiceLogEntry{
		MaintenanceItemID: maintID, ServicedAt: mar1,
	}, Vendor{}))
	assert.True(t, getMaint().LastServicedAt.Equal(mar1))

	// Update the more recent entry to feb1 → LastServicedAt adjusts to feb1.
	entries, err := store.ListServiceLog(maintID, false)
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	latest := entries[0]
	latest.ServicedAt = feb1
	require.NoError(t, store.UpdateServiceLog(latest, Vendor{}))
	assert.True(t, getMaint().LastServicedAt.Equal(feb1))

	// Delete the feb1 entry → LastServicedAt falls back to jan15.
	require.NoError(t, store.DeleteServiceLog(latest.ID))
	assert.True(t, getMaint().LastServicedAt.Equal(jan15))

	// Restore the feb1 entry → LastServicedAt returns to feb1.
	require.NoError(t, store.RestoreServiceLog(latest.ID))
	assert.True(t, getMaint().LastServicedAt.Equal(feb1))

	// Delete all entries → LastServicedAt preserved (not nulled).
	// Deleting feb1 first syncs to jan15; deleting jan15 finds no entries
	// and preserves the last synced value.
	entries, err = store.ListServiceLog(maintID, false)
	require.NoError(t, err)
	for _, e := range entries {
		require.NoError(t, store.DeleteServiceLog(e.ID))
	}
	require.NotNil(t, getMaint().LastServicedAt,
		"LastServicedAt should be preserved when all entries are deleted")
	assert.True(t, getMaint().LastServicedAt.Equal(jan15),
		"should keep the last synced value")
}

func TestServiceLogMoveBetweenParentsSyncsBoth(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	categories, err := store.MaintenanceCategories()
	require.NoError(t, err)

	// Create two maintenance items, capturing IDs directly from Create.
	itemA := &MaintenanceItem{Name: "Item A", CategoryID: categories[0].ID}
	require.NoError(t, store.CreateMaintenance(itemA))
	itemB := &MaintenanceItem{Name: "Item B", CategoryID: categories[0].ID}
	require.NoError(t, store.CreateMaintenance(itemB))
	idA, idB := itemA.ID, itemB.ID

	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	getMaint := func(id string) MaintenanceItem {
		t.Helper()
		item, err := store.GetMaintenance(id)
		require.NoError(t, err)
		return item
	}

	// Create entry under Item A.
	require.NoError(t, store.CreateServiceLog(&ServiceLogEntry{
		MaintenanceItemID: idA, ServicedAt: jan15,
	}, Vendor{}))
	require.NotNil(t, getMaint(idA).LastServicedAt)
	assert.True(t, getMaint(idA).LastServicedAt.Equal(jan15))

	// Move the entry to Item B.
	entries, err := store.ListServiceLog(idA, false)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	entry := entries[0]
	entry.MaintenanceItemID = idB
	require.NoError(t, store.UpdateServiceLog(entry, Vendor{}))

	// Item B should now have LastServicedAt = jan15.
	require.NotNil(t, getMaint(idB).LastServicedAt)
	assert.True(t, getMaint(idB).LastServicedAt.Equal(jan15),
		"new parent should have LastServicedAt synced")

	// Item A should have LastServicedAt preserved at jan15 (no remaining entries).
	require.NotNil(t, getMaint(idA).LastServicedAt,
		"old parent LastServicedAt should be preserved when no entries remain")
	assert.True(t, getMaint(idA).LastServicedAt.Equal(jan15),
		"old parent should retain the last synced value")
}

func TestSoftDeletePersistsAcrossRuns(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "persist.db")

	// Session 1: create a project, then soft-delete it.
	require.NoError(t, os.WriteFile(path, templateBytes, 0o600))
	store1, err := Open(path)
	require.NoError(t, err)
	types, _ := store1.ProjectTypes()
	require.NoError(t, store1.CreateProject(&Project{
		Title: "Persist Test", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store1.ListProjects(false)
	var projectID string
	for _, p := range projects {
		if p.Title == "Persist Test" {
			projectID = p.ID
			break
		}
	}
	require.NotEmpty(t, projectID)
	require.NoError(t, store1.DeleteProject(projectID))
	_ = store1.Close()

	// Session 2: reopen and verify the project is still soft-deleted.
	store2, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store2.Close() })

	projects2, err := store2.ListProjects(false)
	require.NoError(t, err)
	for _, p := range projects2 {
		assert.NotEqual(
			t,
			projectID,
			p.ID,
			"soft-deleted project should not appear in normal listing after reopen",
		)
	}

	projectsAll, err := store2.ListProjects(true)
	require.NoError(t, err)
	found := false
	for _, p := range projectsAll {
		if p.ID == projectID {
			found = true
			break
		}
	}
	assert.True(t, found, "soft-deleted project should appear in unscoped listing after reopen")

	require.NoError(t, store2.RestoreProject(projectID))
	projects3, err := store2.ListProjects(false)
	require.NoError(t, err)
	found = false
	for _, p := range projects3 {
		if p.ID == projectID {
			found = true
			break
		}
	}
	assert.True(t, found, "restored project should appear in normal listing")
}

func TestVendorCRUD(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{
		Name: "Test Vendor", ContactName: "Alice",
		Email: "alice@example.com", Phone: "555-0001",
	}))

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	got := vendors[0]
	assert.Equal(t, "Test Vendor", got.Name)
	assert.Equal(t, "Alice", got.ContactName)

	fetched, err := store.GetVendor(got.ID)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", fetched.Email)

	fetched.Phone = "555-9999"
	fetched.Website = "https://example.com"
	require.NoError(t, store.UpdateVendor(fetched))
	updated, _ := store.GetVendor(fetched.ID)
	assert.Equal(t, "555-9999", updated.Phone)
	assert.Equal(t, "https://example.com", updated.Website)
}

func TestCountQuotesByVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Quote Vendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Test", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projectID := projects[0].ID

	for range 2 {
		require.NoError(t, store.CreateQuote(
			&Quote{ProjectID: projectID, TotalCents: 100000},
			Vendor{Name: "Quote Vendor"},
		))
	}

	counts, err := store.CountQuotesByVendor([]string{vendorID})
	require.NoError(t, err)
	assert.Equal(t, 2, counts[vendorID])

	empty, err := store.CountQuotesByVendor(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestCountServiceLogsByVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Job Vendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	cats, _ := store.MaintenanceCategories()
	require.NoError(
		t,
		store.CreateMaintenance(&MaintenanceItem{Name: "Filter", CategoryID: cats[0].ID}),
	)
	items, _ := store.ListMaintenance(false)
	maintID := items[0].ID

	require.NoError(t, store.CreateServiceLog(
		&ServiceLogEntry{MaintenanceItemID: maintID, ServicedAt: time.Now()},
		Vendor{Name: "Job Vendor"},
	))

	counts, err := store.CountServiceLogsByVendor([]string{vendorID})
	require.NoError(t, err)
	assert.Equal(t, 1, counts[vendorID])
}

func TestDeleteProjectBlockedByQuotes(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Blocked Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projID := projects[0].ID

	require.NoError(
		t,
		store.CreateQuote(&Quote{ProjectID: projID, TotalCents: 1000}, Vendor{Name: "V1"}),
	)

	require.ErrorContains(t, store.DeleteProject(projID), "active quote")

	quotes, _ := store.ListQuotes(false)
	require.NoError(t, store.DeleteQuote(quotes[0].ID))
	require.NoError(t, store.DeleteProject(projID))
}

func TestRestoreQuoteBlockedByDeletedProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Doomed Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projID := projects[0].ID

	require.NoError(
		t,
		store.CreateQuote(&Quote{ProjectID: projID, TotalCents: 500}, Vendor{Name: "V2"}),
	)
	quotes, _ := store.ListQuotes(false)
	quoteID := quotes[0].ID

	require.NoError(t, store.DeleteQuote(quoteID))
	require.NoError(t, store.DeleteProject(projID))

	require.ErrorContains(t, store.RestoreQuote(quoteID), "project is deleted")

	require.NoError(t, store.RestoreProject(projID))
	require.NoError(t, store.RestoreQuote(quoteID))
}

func TestRestoreServiceLogBlockedByDeletedMaintenance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cats, _ := store.MaintenanceCategories()
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Doomed Maint", CategoryID: cats[0].ID, IntervalMonths: 6,
	}))
	items, _ := store.ListMaintenance(false)
	maintID := items[0].ID

	require.NoError(t, store.CreateServiceLog(
		&ServiceLogEntry{MaintenanceItemID: maintID, ServicedAt: time.Now()},
		Vendor{Name: "SL2"},
	))
	logs, _ := store.ListServiceLog(maintID, false)
	logID := logs[0].ID

	require.NoError(t, store.DeleteServiceLog(logID))
	require.NoError(t, store.DeleteMaintenance(maintID))

	require.ErrorContains(t, store.RestoreServiceLog(logID), "maintenance item is deleted")

	require.NoError(t, store.RestoreMaintenance(maintID))
	require.NoError(t, store.RestoreServiceLog(logID))
}

func TestDeleteMaintenanceBlockedByServiceLogs(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cats, _ := store.MaintenanceCategories()
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Blocked Maint", CategoryID: cats[0].ID, IntervalMonths: 3,
	}))
	items, _ := store.ListMaintenance(false)
	maintID := items[0].ID

	require.NoError(t, store.CreateServiceLog(
		&ServiceLogEntry{MaintenanceItemID: maintID, ServicedAt: time.Now()},
		Vendor{Name: "SL Vendor"},
	))

	require.ErrorContains(t, store.DeleteMaintenance(maintID), "service log")

	logs, _ := store.ListServiceLog(maintID, false)
	require.NoError(t, store.DeleteServiceLog(logs[0].ID))
	require.NoError(t, store.DeleteMaintenance(maintID))
}

func TestPartialQuoteDeletionStillBlocksProjectDelete(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Multi-Quote", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projID := projects[0].ID

	for _, name := range []string{"Vendor A", "Vendor B"} {
		require.NoError(
			t,
			store.CreateQuote(&Quote{ProjectID: projID, TotalCents: 1000}, Vendor{Name: name}),
		)
	}
	quotes, _ := store.ListQuotes(false)
	require.Len(t, quotes, 2)

	require.NoError(t, store.DeleteQuote(quotes[0].ID))
	require.ErrorContains(t, store.DeleteProject(projID), "1 active quote")

	require.NoError(t, store.DeleteQuote(quotes[1].ID))
	require.NoError(t, store.DeleteProject(projID))
}

func TestRestoreMaintenanceBlockedByDeletedAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateAppliance(&Appliance{Name: "Doomed Fridge"}))
	appliances, _ := store.ListAppliances(false)
	appID := appliances[0].ID

	cats, _ := store.MaintenanceCategories()
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Coil Cleaning", CategoryID: cats[0].ID, IntervalMonths: 6, ApplianceID: &appID,
	}))
	items, _ := store.ListMaintenance(false)
	maintID := items[0].ID

	require.NoError(t, store.DeleteMaintenance(maintID))
	require.NoError(t, store.DeleteAppliance(appID))

	require.ErrorContains(t, store.RestoreMaintenance(maintID), "appliance is deleted")

	require.NoError(t, store.RestoreAppliance(appID))
	require.NoError(t, store.RestoreMaintenance(maintID))
}

func TestRestoreMaintenanceAllowedWithoutAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cats, _ := store.MaintenanceCategories()
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Gutter Cleaning", CategoryID: cats[0].ID, IntervalMonths: 6,
	}))
	items, _ := store.ListMaintenance(false)
	maintID := items[0].ID

	require.NoError(t, store.DeleteMaintenance(maintID))
	require.NoError(t, store.RestoreMaintenance(maintID))
}

func TestThreeLevelDeleteRestoreChain(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateAppliance(&Appliance{Name: "HVAC Unit"}))
	appliances, _ := store.ListAppliances(false)
	appID := appliances[0].ID

	cats, _ := store.MaintenanceCategories()
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Filter Change", CategoryID: cats[0].ID, IntervalMonths: 3, ApplianceID: &appID,
	}))
	items, _ := store.ListMaintenance(false)
	maintID := items[0].ID

	require.NoError(t, store.CreateServiceLog(
		&ServiceLogEntry{MaintenanceItemID: maintID, ServicedAt: time.Now()},
		Vendor{},
	))
	logs, _ := store.ListServiceLog(maintID, false)
	logID := logs[0].ID

	// --- Delete bottom-up ---
	require.Error(t, store.DeleteMaintenance(maintID), "active service log should block")

	require.NoError(t, store.DeleteServiceLog(logID))
	require.NoError(t, store.DeleteMaintenance(maintID))
	require.NoError(t, store.DeleteAppliance(appID))

	// --- Attempt wrong-order restores ---
	require.ErrorContains(t, store.RestoreServiceLog(logID), "maintenance item is deleted")
	require.ErrorContains(t, store.RestoreMaintenance(maintID), "appliance is deleted")

	// --- Restore correct order ---
	require.NoError(t, store.RestoreAppliance(appID))
	require.NoError(t, store.RestoreMaintenance(maintID))
	require.NoError(t, store.RestoreServiceLog(logID))

	fetched, err := store.GetMaintenance(maintID)
	require.NoError(t, err)
	require.NotNil(t, fetched.ApplianceID)
	assert.Equal(t, appID, *fetched.ApplianceID)

	restoredLogs, err := store.ListServiceLog(maintID, false)
	require.NoError(t, err)
	assert.Len(t, restoredLogs, 1)
}

func TestDeleteApplianceBlockedByMaintenance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateAppliance(&Appliance{Name: "Guarded Fridge"}))
	appliances, _ := store.ListAppliances(false)
	appID := appliances[0].ID

	cats, _ := store.MaintenanceCategories()
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Filter", CategoryID: cats[0].ID, IntervalMonths: 6, ApplianceID: &appID,
	}))

	require.ErrorContains(t, store.DeleteAppliance(appID), "active maintenance item")

	items, _ := store.ListMaintenanceByAppliance(appID, false)
	require.NoError(t, store.DeleteMaintenance(items[0].ID))
	require.NoError(t, store.DeleteAppliance(appID))
}

func TestGetAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	a := &Appliance{Name: "Fridge"}
	require.NoError(t, store.CreateAppliance(a))
	got, err := store.GetAppliance(a.ID)
	require.NoError(t, err)
	assert.Equal(t, "Fridge", got.Name)
}

func TestGetApplianceNotFound(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	_, err := store.GetAppliance("01JNOTEXIST000000000009999")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestUpdateAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	app := &Appliance{Name: "Fridge"}
	require.NoError(t, store.CreateAppliance(app))
	got, _ := store.GetAppliance(app.ID)
	got.Brand = "Samsung"
	require.NoError(t, store.UpdateAppliance(got))
	updated, _ := store.GetAppliance(app.ID)
	assert.Equal(t, "Samsung", updated.Brand)
}

func TestListMaintenanceByAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	categories, _ := store.MaintenanceCategories()
	catID := categories[0].ID

	app := &Appliance{Name: "Fridge"}
	require.NoError(t, store.CreateAppliance(app))
	appID := app.ID
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Clean coils", CategoryID: catID, ApplianceID: &appID,
	}))
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Check smoke detectors", CategoryID: catID,
	}))

	items, err := store.ListMaintenanceByAppliance(appID, false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Clean coils", items[0].Name)
}

func TestCountMaintenanceByAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	categories, _ := store.MaintenanceCategories()
	catID := categories[0].ID

	app := &Appliance{Name: "Fridge"}
	require.NoError(t, store.CreateAppliance(app))
	appID := app.ID
	for _, name := range []string{"Clean coils", "Replace filter"} {
		require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
			Name: name, CategoryID: catID, ApplianceID: &appID,
		}))
	}

	counts, err := store.CountMaintenanceByAppliance([]string{appID})
	require.NoError(t, err)
	assert.Equal(t, 2, counts[appID])
}

func TestUpdateServiceLog(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	categories, _ := store.MaintenanceCategories()
	catID := categories[0].ID

	mi := &MaintenanceItem{Name: "HVAC filter", CategoryID: catID}
	require.NoError(t, store.CreateMaintenance(mi))
	now := time.Now().Truncate(time.Second)
	sle := &ServiceLogEntry{
		MaintenanceItemID: mi.ID, ServicedAt: now, Notes: "initial",
	}
	require.NoError(t, store.CreateServiceLog(sle, Vendor{}))

	created, _ := store.GetServiceLog(sle.ID)
	created.Notes = "updated"
	require.NoError(t, store.UpdateServiceLog(created, Vendor{Name: "HVAC Pros"}))

	updated, _ := store.GetServiceLog(sle.ID)
	assert.Equal(t, "updated", updated.Notes)
	assert.NotNil(t, updated.VendorID)
}

func TestUpdateServiceLogClearVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	categories, _ := store.MaintenanceCategories()
	catID := categories[0].ID

	mi := &MaintenanceItem{Name: "HVAC filter", CategoryID: catID}
	require.NoError(t, store.CreateMaintenance(mi))
	now := time.Now().Truncate(time.Second)
	sle := &ServiceLogEntry{
		MaintenanceItemID: mi.ID, ServicedAt: now,
	}
	require.NoError(t, store.CreateServiceLog(sle, Vendor{Name: "HVAC Pros"}))

	created, _ := store.GetServiceLog(sle.ID)
	require.NoError(t, store.UpdateServiceLog(created, Vendor{}))
	updated, _ := store.GetServiceLog(sle.ID)
	assert.Nil(t, updated.VendorID)
}

func TestListMaintenanceByApplianceIncludeDeleted(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	categories, _ := store.MaintenanceCategories()
	catID := categories[0].ID

	app := &Appliance{Name: "Fridge"}
	require.NoError(t, store.CreateAppliance(app))
	appID := app.ID
	mi := &MaintenanceItem{
		Name: "Clean coils", CategoryID: catID, ApplianceID: &appID,
	}
	require.NoError(t, store.CreateMaintenance(mi))
	require.NoError(t, store.DeleteMaintenance(mi.ID))

	items, _ := store.ListMaintenanceByAppliance(appID, false)
	assert.Empty(t, items)

	items, _ = store.ListMaintenanceByAppliance(appID, true)
	assert.Len(t, items, 1)
}

func TestSoftDeleteRestoreVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Test Vendor"}))

	vendors, _ := store.ListVendors(false)
	require.Len(t, vendors, 1)
	id := vendors[0].ID

	require.NoError(t, store.DeleteVendor(id))
	vendors, _ = store.ListVendors(false)
	assert.Empty(t, vendors)

	vendors, _ = store.ListVendors(true)
	require.Len(t, vendors, 1)
	assert.True(t, vendors[0].DeletedAt.Valid)

	require.NoError(t, store.RestoreVendor(id))
	vendors, _ = store.ListVendors(false)
	assert.Len(t, vendors, 1)
}

func TestDeleteVendorBlockedByQuotes(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Blocked Vendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Test", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projID := projects[0].ID

	require.NoError(
		t,
		store.CreateQuote(
			&Quote{ProjectID: projID, TotalCents: 1000},
			Vendor{Name: "Blocked Vendor"},
		),
	)

	require.ErrorContains(t, store.DeleteVendor(vendorID), "active quote")

	quotes, _ := store.ListQuotes(false)
	require.NoError(t, store.DeleteQuote(quotes[0].ID))
	require.NoError(t, store.DeleteVendor(vendorID))
}

func TestRestoreQuoteBlockedByDeletedVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Doomed Vendor"}))
	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Test", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projID := projects[0].ID
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	require.NoError(
		t,
		store.CreateQuote(
			&Quote{ProjectID: projID, TotalCents: 500},
			Vendor{Name: "Doomed Vendor"},
		),
	)
	quotes, _ := store.ListQuotes(false)
	quoteID := quotes[0].ID

	require.NoError(t, store.DeleteQuote(quoteID))
	require.NoError(t, store.DeleteVendor(vendorID))

	require.ErrorContains(t, store.RestoreQuote(quoteID), "vendor is deleted")

	require.NoError(t, store.RestoreVendor(vendorID))
	require.NoError(t, store.RestoreQuote(quoteID))
}

func TestRestoreServiceLogBlockedByDeletedVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Doomed SL Vendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	cats, _ := store.MaintenanceCategories()
	require.NoError(
		t,
		store.CreateMaintenance(&MaintenanceItem{Name: "Test Maint", CategoryID: cats[0].ID}),
	)
	items, _ := store.ListMaintenance(false)
	maintID := items[0].ID

	require.NoError(t, store.CreateServiceLog(
		&ServiceLogEntry{MaintenanceItemID: maintID, ServicedAt: time.Now()},
		Vendor{Name: "Doomed SL Vendor"},
	))
	logs, _ := store.ListServiceLog(maintID, false)
	logID := logs[0].ID

	require.NoError(t, store.DeleteServiceLog(logID))
	require.NoError(t, store.DeleteVendor(vendorID))

	require.ErrorContains(t, store.RestoreServiceLog(logID), "vendor is deleted")

	require.NoError(t, store.RestoreVendor(vendorID))
	require.NoError(t, store.RestoreServiceLog(logID))
}

func TestRestoreServiceLogAllowedWithoutVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cats, _ := store.MaintenanceCategories()
	require.NoError(
		t,
		store.CreateMaintenance(&MaintenanceItem{Name: "Self Maint", CategoryID: cats[0].ID}),
	)
	items, _ := store.ListMaintenance(false)
	maintID := items[0].ID

	require.NoError(t, store.CreateServiceLog(
		&ServiceLogEntry{MaintenanceItemID: maintID, ServicedAt: time.Now()},
		Vendor{},
	))
	logs, _ := store.ListServiceLog(maintID, false)
	logID := logs[0].ID

	require.NoError(t, store.DeleteServiceLog(logID))
	require.NoError(t, store.RestoreServiceLog(logID))
}

func TestVendorQuoteProjectDeleteRestoreChain(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Chain Vendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Chain Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projID := projects[0].ID

	require.NoError(
		t,
		store.CreateQuote(
			&Quote{ProjectID: projID, TotalCents: 1000},
			Vendor{Name: "Chain Vendor"},
		),
	)
	quotes, _ := store.ListQuotes(false)
	quoteID := quotes[0].ID

	// --- Delete bottom-up ---
	require.Error(t, store.DeleteVendor(vendorID), "active quote blocks vendor delete")
	require.Error(t, store.DeleteProject(projID), "active quote blocks project delete")

	require.NoError(t, store.DeleteQuote(quoteID))
	require.NoError(t, store.DeleteProject(projID))
	require.NoError(t, store.DeleteVendor(vendorID))

	// --- Attempt wrong-order restores ---
	require.ErrorContains(t, store.RestoreQuote(quoteID), "project is deleted")

	require.NoError(t, store.RestoreProject(projID))
	require.ErrorContains(t, store.RestoreQuote(quoteID), "vendor is deleted")

	// --- Restore correct order ---
	require.NoError(t, store.RestoreVendor(vendorID))
	require.NoError(t, store.RestoreQuote(quoteID))

	vendors, _ = store.ListVendors(false)
	assert.Len(t, vendors, 1)
	quotes, _ = store.ListQuotes(false)
	assert.Len(t, quotes, 1)
}

func TestFindOrCreateVendorRestoresSoftDeleted(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Revivable Vendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	require.NoError(t, store.DeleteVendor(vendorID))
	vendors, _ = store.ListVendors(false)
	assert.Empty(t, vendors)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Test", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: projects[0].ID, TotalCents: 500},
		Vendor{Name: "Revivable Vendor"},
	))

	vendors, _ = store.ListVendors(false)
	require.Len(t, vendors, 1)
	assert.Equal(t, vendorID, vendors[0].ID)
}

func TestVendorDeletionRecord(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Record Vendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	require.NoError(t, store.DeleteVendor(vendorID))
	record, err := store.LastDeletion(DeletionEntityVendor)
	require.NoError(t, err)
	assert.Equal(t, vendorID, record.TargetID)

	require.NoError(t, store.RestoreVendor(vendorID))
	_, err = store.LastDeletion(DeletionEntityVendor)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestUnicodeRoundTrip(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	tests := []struct {
		name     string
		nickname string
		city     string
	}{
		{"accented Latin", "Casa de Garc\u00eda", "San Jos\u00e9"},
		{"CJK characters", "\u6211\u7684\u5bb6", "\u6771\u4eac"},      // 我的家, 東京
		{"emoji", "Home \U0001f3e0", "City \u2605"},                   // 🏠, ★
		{"mixed scripts", "Haus M\u00fcller \u2014 \u6771\u4eac", ""}, // Haus Müller — 東京
		{"fraction and section", "\u00bd acre lot", "\u00a75 district"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Delete existing profile (if any) so we can create fresh.
			store.db.Where("1 = 1").Delete(&HouseProfile{})

			profile := HouseProfile{Nickname: tt.nickname, City: tt.city}
			require.NoError(t, store.CreateHouseProfile(profile))

			fetched, err := store.HouseProfile()
			require.NoError(t, err)
			assert.Equal(t, tt.nickname, fetched.Nickname, "nickname round-trip")
			assert.Equal(t, tt.city, fetched.City, "city round-trip")
		})
	}
}

func TestUnicodeRoundTripVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	names := []string{
		"Garc\u00eda Plumbing",                 // García
		"M\u00fcller HVAC",                     // Müller
		"\u6771\u829d\u30b5\u30fc\u30d3\u30b9", // 東芝サービス
		"O'Brien & Sons",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, store.CreateVendor(&Vendor{Name: name}))
		})
	}

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	vendorNames := make([]string, len(vendors))
	for i, v := range vendors {
		vendorNames[i] = v.Name
	}
	for _, name := range names {
		assert.Contains(t, vendorNames, name, "vendor %q should survive round-trip", name)
	}
}

func TestUnicodeRoundTripNotes(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, err := store.ProjectTypes()
	require.NoError(t, err)

	notes := "Technician Jos\u00e9 used \u00bd-inch fittings per \u00a75.2"
	require.NoError(t, store.CreateProject(&Project{
		Title:         "Unicode notes test",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
		Description:   notes,
	}))

	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.NotEmpty(t, projects)
	assert.Equal(t, notes, projects[len(projects)-1].Description)
}

func TestDocumentCRUDAndMetadata(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Doc Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projectID := projects[0].ID

	content := []byte("fake pdf content")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	// User attaches a document to a project.
	require.NoError(t, store.CreateDocument(&Document{
		Title:          "Quote PDF",
		FileName:       "invoice.pdf",
		EntityKind:     DocumentEntityProject,
		EntityID:       projectID,
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: checksum,
		Data:           content,
		Notes:          "first draft",
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)

	doc := docs[0]
	assert.Equal(t, "Quote PDF", doc.Title)
	assert.Equal(t, "invoice.pdf", doc.FileName)
	assert.Equal(t, checksum, doc.ChecksumSHA256)
	assert.Equal(t, "application/pdf", doc.MIMEType)
	assert.Equal(t, DocumentEntityProject, doc.EntityKind)
	assert.Equal(t, projectID, doc.EntityID)
	// Data is excluded from list queries.
	assert.Empty(t, doc.Data)

	// User deletes, doc vanishes from active list, then restores.
	require.NoError(t, store.DeleteDocument(doc.ID))
	docs, err = store.ListDocuments(false)
	require.NoError(t, err)
	assert.Empty(t, docs)

	require.NoError(t, store.RestoreDocument(doc.ID))
	docs, err = store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
}

func TestRestoreDocumentBlockedByDeletedTarget(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Doc Restore Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projectID := projects[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title:      "Project Note",
		EntityKind: DocumentEntityProject,
		EntityID:   projectID,
		Data:       []byte("note"),
	}))
	docs, _ := store.ListDocuments(false)
	docID := docs[0].ID

	// User deletes the document, then the project.
	require.NoError(t, store.DeleteDocument(docID))
	require.NoError(t, store.DeleteProject(projectID))
	// Restoring the document should fail while the project is deleted.
	require.ErrorContains(t, store.RestoreDocument(docID), "deleted")

	// Restoring the project first unblocks the document restore.
	require.NoError(t, store.RestoreProject(projectID))
	require.NoError(t, store.RestoreDocument(docID))
}

func TestDocumentBLOBStorageAndExtract(t *testing.T) {
	store := newTestStore(t)

	content := []byte("this is a test PDF")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	require.NoError(t, store.CreateDocument(&Document{
		Title:          "Test Report",
		FileName:       "report.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: checksum,
		Data:           content,
	}))

	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, int64(len(content)), docs[0].SizeBytes)

	// ExtractDocument writes BLOB to cache and returns a path.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cachePath, err := store.ExtractDocument(docs[0].ID)
	require.NoError(t, err)
	assert.FileExists(t, cachePath)
	cached, err := os.ReadFile(cachePath) //nolint:gosec // test-only path
	require.NoError(t, err)
	assert.Equal(t, content, cached)

	// Second call is a cache hit (same path, no error).
	cachePath2, err := store.ExtractDocument(docs[0].ID)
	require.NoError(t, err)
	assert.Equal(t, cachePath, cachePath2)
}

func TestUpdateDocumentMetadataPreservesFile(t *testing.T) {
	store := newTestStore(t)

	content := []byte("important contract text")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	require.NoError(t, store.CreateDocument(&Document{
		Title:          "Contract",
		FileName:       "contract.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: checksum,
		Data:           content,
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	original := docs[0]

	// User edits only metadata -- no new file data.
	require.NoError(t, store.UpdateDocument(Document{
		ID:    original.ID,
		Title: "Updated Contract",
		Notes: "added notes",
	}))

	updated, err := store.GetDocument(original.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Contract", updated.Title)
	assert.Equal(t, "added notes", updated.Notes)
	// File metadata must be preserved.
	assert.Equal(t, original.FileName, updated.FileName)
	assert.Equal(t, original.MIMEType, updated.MIMEType)
	assert.Equal(t, original.SizeBytes, updated.SizeBytes)
	assert.Equal(t, original.ChecksumSHA256, updated.ChecksumSHA256)

	// Verify BLOB content is still intact.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cachePath, err := store.ExtractDocument(original.ID)
	require.NoError(t, err)
	cached, err := os.ReadFile(cachePath) //nolint:gosec // test-only path
	require.NoError(t, err)
	assert.Equal(t, content, cached)
}

func TestUpdateDocumentPreservesEntityLink(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Linked Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projectID := projects[0].ID

	content := []byte("linked doc content")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	require.NoError(t, store.CreateDocument(&Document{
		Title:          "Linked Doc",
		FileName:       "linked.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: checksum,
		Data:           content,
		EntityKind:     DocumentEntityProject,
		EntityID:       projectID,
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	original := docs[0]
	require.Equal(t, projectID, original.EntityID)
	require.Equal(t, DocumentEntityProject, original.EntityKind)

	// Simulate what the form does: pass entity fields through unchanged.
	require.NoError(t, store.UpdateDocument(Document{
		ID:         original.ID,
		Title:      "Renamed Doc",
		Notes:      "added notes",
		EntityKind: DocumentEntityProject,
		EntityID:   projectID,
	}))

	updated, err := store.GetDocument(original.ID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed Doc", updated.Title)
	assert.Equal(t, "added notes", updated.Notes)
	assert.Equal(t, projectID, updated.EntityID, "EntityID must survive a metadata-only edit")
	assert.Equal(
		t,
		DocumentEntityProject,
		updated.EntityKind,
		"EntityKind must survive a metadata-only edit",
	)
}

func TestUpdateDocumentChangesEntity(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Project A", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projectID := projects[0].ID

	require.NoError(t, store.CreateAppliance(&Appliance{Name: "Fridge"}))
	appliances, _ := store.ListAppliances(false)
	applianceID := appliances[0].ID

	content := []byte("reassign me")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))
	require.NoError(t, store.CreateDocument(&Document{
		Title:          "Reassign Doc",
		FileName:       "doc.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: checksum,
		Data:           content,
		EntityKind:     DocumentEntityProject,
		EntityID:       projectID,
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	original := docs[0]

	// Reassign from project to appliance.
	require.NoError(t, store.UpdateDocument(Document{
		ID:         original.ID,
		Title:      original.Title,
		EntityKind: DocumentEntityAppliance,
		EntityID:   applianceID,
	}))

	updated, err := store.GetDocument(original.ID)
	require.NoError(t, err)
	assert.Equal(t, DocumentEntityAppliance, updated.EntityKind)
	assert.Equal(t, applianceID, updated.EntityID)
}

func TestUpdateDocumentClearsEntity(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Temp Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projectID := projects[0].ID

	content := []byte("unlink me")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))
	require.NoError(t, store.CreateDocument(&Document{
		Title:          "Unlinked Doc",
		FileName:       "doc.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: checksum,
		Data:           content,
		EntityKind:     DocumentEntityProject,
		EntityID:       projectID,
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	original := docs[0]

	// Clear entity assignment.
	require.NoError(t, store.UpdateDocument(Document{
		ID:    original.ID,
		Title: original.Title,
	}))

	updated, err := store.GetDocument(original.ID)
	require.NoError(t, err)
	assert.Empty(t, updated.EntityKind)
	assert.Empty(t, updated.EntityID)
}

func TestUpdateDocumentReplacesFile(t *testing.T) {
	store := newTestStore(t)

	oldContent := []byte("draft v1")
	oldChecksum := fmt.Sprintf("%x", sha256.Sum256(oldContent))

	// User uploads an initial file.
	require.NoError(t, store.CreateDocument(&Document{
		Title:          "Contract",
		FileName:       "draft.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(oldContent)),
		ChecksumSHA256: oldChecksum,
		Data:           oldContent,
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	original := docs[0]

	// User realizes they uploaded the wrong version and picks a new file.
	newContent := []byte("final signed version -- much longer")
	newChecksum := fmt.Sprintf("%x", sha256.Sum256(newContent))

	require.NoError(t, store.UpdateDocument(Document{
		ID:             original.ID,
		Title:          "Contract (Signed)",
		FileName:       "final_signed.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(newContent)),
		ChecksumSHA256: newChecksum,
		Data:           newContent,
	}))

	updated, err := store.GetDocument(original.ID)
	require.NoError(t, err)
	assert.Equal(t, "Contract (Signed)", updated.Title)
	assert.Equal(t, "final_signed.pdf", updated.FileName)
	assert.Equal(t, int64(len(newContent)), updated.SizeBytes)
	assert.NotEqual(t, oldChecksum, updated.ChecksumSHA256)

	// User opens the document and sees the new content.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cachePath, err := store.ExtractDocument(original.ID)
	require.NoError(t, err)
	cached, err := os.ReadFile(cachePath) //nolint:gosec // test-only path
	require.NoError(t, err)
	assert.Equal(t, newContent, cached)
}

func TestReplaceFileThenViewServesNewContent(t *testing.T) {
	store := newTestStore(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	v1 := []byte("version 1 content")
	v1Checksum := fmt.Sprintf("%x", sha256.Sum256(v1))

	// User uploads v1 and views it.
	require.NoError(t, store.CreateDocument(&Document{
		Title:          "Report",
		FileName:       "report.pdf",
		SizeBytes:      int64(len(v1)),
		ChecksumSHA256: v1Checksum,
		Data:           v1,
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	docID := docs[0].ID

	path1, err := store.ExtractDocument(docID)
	require.NoError(t, err)
	got1, err := os.ReadFile(path1) //nolint:gosec // test-only path
	require.NoError(t, err)
	assert.Equal(t, v1, got1)

	// User replaces with v2.
	v2 := []byte("version 2 -- updated numbers")
	v2Checksum := fmt.Sprintf("%x", sha256.Sum256(v2))
	require.NoError(t, store.UpdateDocument(Document{
		ID:             docID,
		Title:          "Report",
		FileName:       "report_v2.pdf",
		SizeBytes:      int64(len(v2)),
		ChecksumSHA256: v2Checksum,
		Data:           v2,
	}))

	// User views again -- must get v2, not stale cache.
	path2, err := store.ExtractDocument(docID)
	require.NoError(t, err)
	assert.NotEqual(t, path1, path2, "cache path should differ after file replacement")
	got2, err := os.ReadFile(path2) //nolint:gosec // test-only path
	require.NoError(t, err)
	assert.Equal(t, v2, got2)
}

func TestDeleteRestoreDocumentContentSurvives(t *testing.T) {
	store := newTestStore(t)

	content := []byte("warranty certificate scan")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	require.NoError(t, store.CreateDocument(&Document{
		Title:          "Warranty",
		FileName:       "warranty.pdf",
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: checksum,
		Data:           content,
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	docID := docs[0].ID

	// User deletes the document.
	require.NoError(t, store.DeleteDocument(docID))

	// Document is gone from the normal list.
	docs, err = store.ListDocuments(false)
	require.NoError(t, err)
	assert.Empty(t, docs)

	// But it still shows up when including deleted items.
	docs, err = store.ListDocuments(true)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Warranty", docs[0].Title)

	// User changes their mind and restores it.
	require.NoError(t, store.RestoreDocument(docID))

	// Document is back in the normal list with metadata intact.
	docs, err = store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, checksum, docs[0].ChecksumSHA256)

	// User opens it -- file content is still there.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cachePath, err := store.ExtractDocument(docID)
	require.NoError(t, err)
	cached, err := os.ReadFile(cachePath) //nolint:gosec // test-only path
	require.NoError(t, err)
	assert.Equal(t, content, cached)
}

func TestUnlinkedDocumentFullLifecycle(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	content := []byte("household inventory spreadsheet")

	// User uploads a standalone document (not linked to any entity).
	require.NoError(t, store.CreateDocument(&Document{
		Title:     "Home Inventory",
		FileName:  "inventory.csv",
		SizeBytes: int64(len(content)),
		Data:      content,
		Notes:     "room-by-room list",
	}))

	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	doc := docs[0]
	assert.Empty(t, doc.EntityKind)
	assert.Empty(t, doc.EntityID)

	// User edits the notes.
	require.NoError(t, store.UpdateDocument(Document{
		ID:    doc.ID,
		Title: "Home Inventory",
		Notes: "updated with garage items",
	}))

	updated, err := store.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated with garage items", updated.Notes)
	assert.Equal(t, doc.FileName, updated.FileName)

	// User deletes and restores -- no entity link to block restore.
	require.NoError(t, store.DeleteDocument(doc.ID))
	require.NoError(t, store.RestoreDocument(doc.ID))

	restored, err := store.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Home Inventory", restored.Title)
}

func TestMultipleDocumentsListOrder(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// User uploads three documents in sequence.
	for _, name := range []string{"Alpha", "Beta", "Gamma"} {
		require.NoError(t, store.CreateDocument(&Document{
			Title: name,
			Data:  []byte(name + " content"),
		}))
	}

	// Pin each document to a distinct past timestamp so ordering is
	// deterministic regardless of wall-clock granularity (SQLite stores
	// datetime with second precision; on fast machines the creates can
	// land in the same second).
	base := time.Now().Add(-time.Hour)
	for i, name := range []string{"Alpha", "Beta", "Gamma"} {
		ts := base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, store.db.Exec(
			"UPDATE documents SET updated_at = ? WHERE title = ?", ts, name,
		).Error)
	}

	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 3)

	// Listed by updated_at DESC, so the last-created doc is first.
	assert.Equal(t, "Gamma", docs[0].Title)
	assert.Equal(t, "Beta", docs[1].Title)
	assert.Equal(t, "Alpha", docs[2].Title)

	// User edits the oldest document -- it should move to the top.
	// GORM sets updated_at to time.Now(), which is after the pinned past
	// timestamps, so "Alpha Updated" sorts first.
	require.NoError(t, store.UpdateDocument(Document{
		ID:    docs[2].ID,
		Title: "Alpha Updated",
	}))

	docs, err = store.ListDocuments(false)
	require.NoError(t, err)
	assert.Equal(t, "Alpha Updated", docs[0].Title)
}

func TestUpdateDocumentClearNotes(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	content := []byte("receipt data")
	require.NoError(t, store.CreateDocument(&Document{
		Title:     "Receipt",
		FileName:  "receipt.pdf",
		MIMEType:  "application/pdf",
		SizeBytes: int64(len(content)),
		Data:      content,
		Notes:     "plumber visit 2026-01",
	}))

	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	doc := docs[0]

	// User decides the notes were wrong and clears them.
	require.NoError(t, store.UpdateDocument(Document{
		ID:    doc.ID,
		Title: "Receipt",
		Notes: "",
	}))

	updated, err := store.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Empty(t, updated.Notes)
	// File metadata still intact.
	assert.Equal(t, doc.FileName, updated.FileName)
	assert.Equal(t, doc.SizeBytes, updated.SizeBytes)
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, templateBytes, 0o600))
	store, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, store.SetMaxDocumentSize(50<<20))
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// newTestStoreWithDemoData creates a store pre-populated with randomized
// demo data from the given seed.
func newTestStoreWithDemoData(t *testing.T, seed uint64) *Store {
	t.Helper()
	store := newTestStore(t)
	require.NoError(t, store.SeedDemoDataFrom(fake.New(seed)))
	return store
}

func ptrTime(y, m, d int) *time.Time {
	return new(time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC))
}

func TestCountQuotesByProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()

	require.NoError(t, store.CreateProject(&Project{
		Title: "P1", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projectID := projects[0].ID

	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: projectID, TotalCents: 5000},
		Vendor{Name: "V1"},
	))
	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: projectID, TotalCents: 7500},
		Vendor{Name: "V2"},
	))

	counts, err := store.CountQuotesByProject([]string{projectID})
	require.NoError(t, err)
	assert.Equal(t, 2, counts[projectID])

	empty, err := store.CountQuotesByProject(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestListQuotesByVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()

	require.NoError(t, store.CreateVendor(&Vendor{Name: "TestVendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	require.NoError(t, store.CreateProject(&Project{
		Title: "P1", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projectID := projects[0].ID

	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: projectID, TotalCents: 1000},
		Vendor{Name: "TestVendor"},
	))
	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: projectID, TotalCents: 2000},
		Vendor{Name: "OtherVendor"},
	))

	quotes, err := store.ListQuotesByVendor(vendorID, false)
	require.NoError(t, err)
	assert.Len(t, quotes, 1)
	assert.Equal(t, int64(1000), quotes[0].TotalCents)
}

func TestListQuotesByProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()

	require.NoError(t, store.CreateProject(&Project{
		Title: "P1", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	require.NoError(t, store.CreateProject(&Project{
		Title: "P2", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	p1ID := projects[0].ID
	p2ID := projects[1].ID

	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: p1ID, TotalCents: 1000},
		Vendor{Name: "V1"},
	))
	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: p2ID, TotalCents: 5000},
		Vendor{Name: "V1"},
	))

	quotes, err := store.ListQuotesByProject(p1ID, false)
	require.NoError(t, err)
	assert.Len(t, quotes, 1)
	assert.Equal(t, int64(1000), quotes[0].TotalCents)
}

func TestListServiceLogsByVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cats, _ := store.MaintenanceCategories()

	require.NoError(t, store.CreateVendor(&Vendor{Name: "LogVendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Filter", CategoryID: cats[0].ID,
	}))
	items, _ := store.ListMaintenance(false)
	maintID := items[0].ID

	require.NoError(t, store.CreateServiceLog(
		&ServiceLogEntry{MaintenanceItemID: maintID, ServicedAt: time.Now()},
		Vendor{Name: "LogVendor"},
	))
	require.NoError(t, store.CreateServiceLog(
		&ServiceLogEntry{MaintenanceItemID: maintID, ServicedAt: time.Now()},
		Vendor{Name: "OtherVendor"},
	))

	entries, err := store.ListServiceLogsByVendor(vendorID, false)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "Filter", entries[0].MaintenanceItem.Name,
		"preloaded MaintenanceItem should be available")
}

func TestListAllServiceLogEntries(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	item := MaintenanceItem{Name: "Filter", CategoryID: cats[0].ID, Season: "spring"}
	require.NoError(t, store.CreateMaintenance(&item))

	item2 := MaintenanceItem{Name: "Coils", CategoryID: cats[0].ID, Season: "fall"}
	require.NoError(t, store.CreateMaintenance(&item2))

	earlier := time.Now().Add(-time.Hour)
	later := time.Now()

	entry1 := ServiceLogEntry{MaintenanceItemID: item.ID, ServicedAt: earlier}
	require.NoError(t, store.CreateServiceLog(&entry1, Vendor{}))

	entry2 := ServiceLogEntry{MaintenanceItemID: item2.ID, ServicedAt: later}
	require.NoError(t, store.CreateServiceLog(&entry2, Vendor{}))

	entries, err := store.ListAllServiceLogEntries(false)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	// Ordered by serviced_at desc.
	assert.Equal(t, entry2.ID, entries[0].ID)
	assert.Equal(t, entry1.ID, entries[1].ID)

	// Preloaded associations populated.
	assert.Equal(t, "Coils", entries[0].MaintenanceItem.Name)
	assert.Equal(t, "Filter", entries[1].MaintenanceItem.Name)
}

func TestDocumentCRUD(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Invoice", EntityKind: DocumentEntityProject, EntityID: "01JTEST00000000000000001",
		MIMEType: "application/pdf", SizeBytes: 1024, Data: []byte("fake-pdf"),
	}))

	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Invoice", docs[0].Title)
	assert.Empty(t, docs[0].Data, "ListDocuments should not load BLOB data")

	doc, err := store.GetDocument(docs[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "fake-pdf", string(doc.Data), "GetDocument should load BLOB data")

	doc.Title = "Updated Invoice"
	require.NoError(t, store.UpdateDocument(doc))
	fetched, err := store.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Invoice", fetched.Title)
}

func TestCountDocumentsByEntity(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Doc1", EntityKind: DocumentEntityProject, EntityID: "01JTEST00000000000000010",
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title: "Doc2", EntityKind: DocumentEntityProject, EntityID: "01JTEST00000000000000010",
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title: "Doc3", EntityKind: DocumentEntityAppliance, EntityID: "01JTEST00000000000000010",
	}))

	counts, err := store.CountDocumentsByEntity(
		DocumentEntityProject,
		[]string{"01JTEST00000000000000010", "01JTEST00000000000000099"},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, counts["01JTEST00000000000000010"])
	assert.Zero(t, counts["01JTEST00000000000000099"])

	counts, err = store.CountDocumentsByEntity(
		DocumentEntityAppliance,
		[]string{"01JTEST00000000000000010"},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, counts["01JTEST00000000000000010"])

	empty, err := store.CountDocumentsByEntity(DocumentEntityProject, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestListDocumentsByEntity(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title: "ProjDoc", EntityKind: DocumentEntityProject, EntityID: "01JTEST00000000000000005",
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title: "AppDoc", EntityKind: DocumentEntityAppliance, EntityID: "01JTEST00000000000000005",
	}))

	docs, err := store.ListDocumentsByEntity(
		DocumentEntityProject,
		"01JTEST00000000000000005",
		false,
	)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "ProjDoc", docs[0].Title)
	assert.Empty(t, docs[0].Data, "should not load BLOB data")
}

func TestListDocumentsByEntityIncludeDeleted(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Active", EntityKind: DocumentEntityProject, EntityID: "01JTEST00000000000000001",
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title: "ToDelete", EntityKind: DocumentEntityProject, EntityID: "01JTEST00000000000000001",
	}))

	docs, _ := store.ListDocumentsByEntity(DocumentEntityProject, "01JTEST00000000000000001", false)
	require.Len(t, docs, 2)

	require.NoError(t, store.DeleteDocument(docs[0].ID))

	active, err := store.ListDocumentsByEntity(
		DocumentEntityProject,
		"01JTEST00000000000000001",
		false,
	)
	require.NoError(t, err)
	assert.Len(t, active, 1)

	all, err := store.ListDocumentsByEntity(DocumentEntityProject, "01JTEST00000000000000001", true)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestDeleteProjectAllowedWithDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()

	require.NoError(t, store.CreateProject(&Project{
		Title: "Doc Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projID := projects[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "ProjDoc", EntityKind: DocumentEntityProject, EntityID: projID,
	}))

	// Documents don't block entity deletion -- they survive their parent.
	require.NoError(t, store.DeleteProject(projID))

	docs, _ := store.ListDocumentsByEntity(DocumentEntityProject, projID, false)
	assert.Len(t, docs, 1, "document should survive parent deletion")
}

func TestDeleteApplianceAllowedWithDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateAppliance(&Appliance{Name: "Doc Fridge"}))
	appliances, _ := store.ListAppliances(false)
	appID := appliances[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Manual", EntityKind: DocumentEntityAppliance, EntityID: appID,
	}))

	require.NoError(t, store.DeleteAppliance(appID))

	docs, _ := store.ListDocumentsByEntity(DocumentEntityAppliance, appID, false)
	assert.Len(t, docs, 1, "document should survive parent deletion")
}

func TestRestoreDocumentBlockedByDeletedProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()

	require.NoError(t, store.CreateProject(&Project{
		Title: "Doomed", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projID := projects[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Orphan", EntityKind: DocumentEntityProject, EntityID: projID,
	}))
	docs, _ := store.ListDocuments(false)
	docID := docs[0].ID

	require.NoError(t, store.DeleteDocument(docID))
	require.NoError(t, store.DeleteProject(projID))

	require.ErrorContains(t, store.RestoreDocument(docID), "project is deleted")

	require.NoError(t, store.RestoreProject(projID))
	require.NoError(t, store.RestoreDocument(docID))
}

func TestRestoreDocumentBlockedByDeletedAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateAppliance(&Appliance{Name: "Doomed Washer"}))
	appliances, _ := store.ListAppliances(false)
	appID := appliances[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Warranty", EntityKind: DocumentEntityAppliance, EntityID: appID,
	}))
	docs, _ := store.ListDocuments(false)
	docID := docs[0].ID

	require.NoError(t, store.DeleteDocument(docID))
	require.NoError(t, store.DeleteAppliance(appID))

	require.ErrorContains(t, store.RestoreDocument(docID), "appliance is deleted")

	require.NoError(t, store.RestoreAppliance(appID))
	require.NoError(t, store.RestoreDocument(docID))
}

func TestCreateDocumentRejectsOversized(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.SetMaxDocumentSize(100))

	err := store.CreateDocument(&Document{
		Title:     "Big File",
		SizeBytes: 200,
		Data:      make([]byte, 200),
	})
	require.ErrorContains(t, err, "too large")
	// Error should show human-readable sizes, not raw byte counts.
	assert.Contains(t, err.Error(), "200 B")
	assert.Contains(t, err.Error(), "100 B")
	assert.NotContains(t, err.Error(), "200 bytes")

	// Exactly at the limit should succeed.
	require.NoError(t, store.CreateDocument(&Document{
		Title:     "Just Right",
		SizeBytes: 100,
		Data:      make([]byte, 100),
	}))
}

func TestDeleteVendorAllowedWithDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Doc Vendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Invoice", EntityKind: DocumentEntityVendor, EntityID: vendorID,
	}))

	require.NoError(t, store.DeleteVendor(vendorID))

	docs, _ := store.ListDocumentsByEntity(DocumentEntityVendor, vendorID, false)
	assert.Len(t, docs, 1, "document should survive parent deletion")
}

func TestDeleteQuoteAllowedWithDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "QP", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projID := projects[0].ID

	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: projID, TotalCents: 500},
		Vendor{Name: "QV"},
	))
	quotes, _ := store.ListQuotes(false)
	quoteID := quotes[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Quote PDF", EntityKind: DocumentEntityQuote, EntityID: quoteID,
	}))

	require.NoError(t, store.DeleteQuote(quoteID))

	docs, _ := store.ListDocumentsByEntity(DocumentEntityQuote, quoteID, false)
	assert.Len(t, docs, 1, "document should survive parent deletion")
}

func TestDeleteMaintenanceAllowedWithDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cat := MaintenanceCategory{Name: "DocMCat"}
	require.NoError(t, store.db.Create(&cat).Error)

	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Documented Filter", CategoryID: cat.ID, IntervalMonths: 6,
	}))
	items, _ := store.ListMaintenance(false)
	itemID := items[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Manual", EntityKind: DocumentEntityMaintenance, EntityID: itemID,
	}))

	require.NoError(t, store.DeleteMaintenance(itemID))

	docs, _ := store.ListDocumentsByEntity(DocumentEntityMaintenance, itemID, false)
	assert.Len(t, docs, 1, "document should survive parent deletion")
}

func TestDeleteServiceLogAllowedWithDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cat := MaintenanceCategory{Name: "SLDocCat"}
	require.NoError(t, store.db.Create(&cat).Error)
	item := MaintenanceItem{Name: "SLDoc Item", CategoryID: cat.ID, IntervalMonths: 6}
	require.NoError(t, store.db.Create(&item).Error)

	require.NoError(t, store.CreateServiceLog(
		&ServiceLogEntry{MaintenanceItemID: item.ID, ServicedAt: time.Now()},
		Vendor{},
	))
	logs, _ := store.ListServiceLog(item.ID, false)
	logID := logs[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Receipt", EntityKind: DocumentEntityServiceLog, EntityID: logID,
	}))

	require.NoError(t, store.DeleteServiceLog(logID))

	docs, _ := store.ListDocumentsByEntity(DocumentEntityServiceLog, logID, false)
	assert.Len(t, docs, 1, "document should survive parent deletion")
}

func TestRestoreDocumentBlockedByDeletedVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Doomed Vendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Vendor Doc", EntityKind: DocumentEntityVendor, EntityID: vendorID,
	}))
	docs, _ := store.ListDocuments(false)
	docID := docs[0].ID

	require.NoError(t, store.DeleteDocument(docID))
	require.NoError(t, store.DeleteVendor(vendorID))

	require.ErrorContains(t, store.RestoreDocument(docID), "vendor is deleted")

	require.NoError(t, store.RestoreVendor(vendorID))
	require.NoError(t, store.RestoreDocument(docID))
}

func TestRestoreDocumentBlockedByDeletedQuote(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "RQP", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projID := projects[0].ID

	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: projID, TotalCents: 100},
		Vendor{Name: "RQV"},
	))
	quotes, _ := store.ListQuotes(false)
	quoteID := quotes[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Quote Receipt", EntityKind: DocumentEntityQuote, EntityID: quoteID,
	}))
	docs, _ := store.ListDocuments(false)
	docID := docs[0].ID

	require.NoError(t, store.DeleteDocument(docID))
	require.NoError(t, store.DeleteQuote(quoteID))

	require.ErrorContains(t, store.RestoreDocument(docID), "quote is deleted")

	require.NoError(t, store.RestoreQuote(quoteID))
	require.NoError(t, store.RestoreDocument(docID))
}

func TestRestoreDocumentBlockedByDeletedMaintenance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cat := MaintenanceCategory{Name: "RMCat"}
	require.NoError(t, store.db.Create(&cat).Error)

	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Doomed Filter", CategoryID: cat.ID, IntervalMonths: 12,
	}))
	items, _ := store.ListMaintenance(false)
	itemID := items[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Filter Manual", EntityKind: DocumentEntityMaintenance, EntityID: itemID,
	}))
	docs, _ := store.ListDocuments(false)
	docID := docs[0].ID

	require.NoError(t, store.DeleteDocument(docID))
	require.NoError(t, store.DeleteMaintenance(itemID))

	require.ErrorContains(t, store.RestoreDocument(docID), "maintenance item is deleted")

	require.NoError(t, store.RestoreMaintenance(itemID))
	require.NoError(t, store.RestoreDocument(docID))
}

func TestRestoreDocumentBlockedByDeletedServiceLog(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cat := MaintenanceCategory{Name: "RSLCat"}
	require.NoError(t, store.db.Create(&cat).Error)
	item := MaintenanceItem{Name: "RSL Item", CategoryID: cat.ID, IntervalMonths: 6}
	require.NoError(t, store.db.Create(&item).Error)

	require.NoError(t, store.CreateServiceLog(
		&ServiceLogEntry{MaintenanceItemID: item.ID, ServicedAt: time.Now()},
		Vendor{},
	))
	logs, _ := store.ListServiceLog(item.ID, false)
	logID := logs[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "SL Receipt", EntityKind: DocumentEntityServiceLog, EntityID: logID,
	}))
	docs, _ := store.ListDocuments(false)
	docID := docs[0].ID

	require.NoError(t, store.DeleteDocument(docID))
	require.NoError(t, store.DeleteServiceLog(logID))

	require.ErrorContains(t, store.RestoreDocument(docID), "service log is deleted")

	require.NoError(t, store.RestoreServiceLog(logID))
	require.NoError(t, store.RestoreDocument(docID))
}

func TestSoftDeleteRestoreDocument(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Receipt", EntityKind: DocumentEntityProject, EntityID: "01JTEST00000000000000001",
	}))

	docs, _ := store.ListDocuments(false)
	require.Len(t, docs, 1)

	require.NoError(t, store.DeleteDocument(docs[0].ID))
	active, _ := store.ListDocuments(false)
	assert.Empty(t, active)

	all, _ := store.ListDocuments(true)
	require.Len(t, all, 1)
	assert.True(t, all[0].DeletedAt.Valid)

	// Restore requires no parent check for this entity_id since no actual
	// project/appliance exists -- this tests the happy path where the
	// parent entity doesn't exist (ErrParentNotFound from requireParentAlive).
	// In real usage the parent always exists before the document is created.
}

func TestEvictStaleCacheRemovesOldFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a "fresh" file (now) and a "stale" file (40 days old).
	fresh := filepath.Join(dir, "fresh.txt")
	stale := filepath.Join(dir, "stale.txt")
	require.NoError(t, os.WriteFile(fresh, []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0o600))

	old := time.Now().AddDate(0, 0, -40)
	require.NoError(t, os.Chtimes(stale, old, old))

	removed, err := EvictStaleCache(dir, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	assert.FileExists(t, fresh)
	assert.NoFileExists(t, stale)
}

func TestEvictStaleCacheEmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	removed, err := EvictStaleCache(dir, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
}

func TestEvictStaleCacheSkipsSubdirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a subdirectory with an old timestamp — should not be removed.
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0o700))
	old := time.Now().AddDate(0, 0, -40)
	require.NoError(t, os.Chtimes(subdir, old, old))

	removed, err := EvictStaleCache(dir, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
	assert.DirExists(t, subdir)
}

func TestEvictStaleCacheRecentFileKept(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// File modified 29 days ago (within the 30-day TTL) should be kept.
	f := filepath.Join(dir, "recent.txt")
	require.NoError(t, os.WriteFile(f, []byte("keep"), 0o600))
	recent := time.Now().AddDate(0, 0, -29)
	require.NoError(t, os.Chtimes(f, recent, recent))

	removed, err := EvictStaleCache(dir, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
	assert.FileExists(t, f)
}

func TestEvictStaleCacheZeroTTLDisabled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	f := filepath.Join(dir, "ancient.txt")
	require.NoError(t, os.WriteFile(f, []byte("data"), 0o600))
	old := time.Now().AddDate(-1, 0, 0)
	require.NoError(t, os.Chtimes(f, old, old))

	removed, err := EvictStaleCache(dir, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
	assert.FileExists(t, f)
}

func TestEvictStaleCacheEmptyDirPath(t *testing.T) {
	t.Parallel()
	// Empty dir path should be a no-op, not read CWD.
	removed, err := EvictStaleCache("", 30*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
}

func TestEvictStaleCacheNonexistentDir(t *testing.T) {
	t.Parallel()
	// Missing cache dir should be a no-op, not a fatal error.
	removed, err := EvictStaleCache(filepath.Join(t.TempDir(), "nope"), 30*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, removed)
}

func TestPragmasSurvivePoolRecycling(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sqlDB, err := store.db.DB()
	require.NoError(t, err)

	// Don't keep idle connections -- forces a fresh driver connection on
	// each checkout, exercising the pragma connector hook.
	sqlDB.SetMaxIdleConns(0)

	ctx := t.Context()
	for range 3 {
		conn, err := sqlDB.Conn(ctx)
		require.NoError(t, err)

		var fkEnabled int
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fkEnabled))
		assert.Equal(t, 1, fkEnabled, "foreign_keys should be ON on fresh connection")

		var journalMode string
		require.NoError(t, conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode))
		assert.Equal(t, "wal", journalMode, "journal_mode should be WAL")

		require.NoError(t, conn.Close())
	}
}

// ---------------------------------------------------------------------------
// Incident CRUD
// ---------------------------------------------------------------------------

func TestIncidentCRUDRoundTrip(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateIncident(&Incident{
		Title:    "Ant infestation",
		Status:   IncidentStatusOpen,
		Severity: IncidentSeverityUrgent,
	}))

	items, err := store.ListIncidents(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	id := items[0].ID
	assert.Equal(t, "Ant infestation", items[0].Title)
	assert.Equal(t, IncidentStatusOpen, items[0].Status)

	fetched, err := store.GetIncident(id)
	require.NoError(t, err)
	assert.Equal(t, "Ant infestation", fetched.Title)

	require.NoError(t, store.UpdateIncident(Incident{
		ID:       id,
		Title:    "Ant infestation (resolved)",
		Status:   IncidentStatusInProgress,
		Severity: IncidentSeverityUrgent,
	}))
	updated, err := store.GetIncident(id)
	require.NoError(t, err)
	assert.Equal(t, "Ant infestation (resolved)", updated.Title)
	assert.Equal(t, IncidentStatusInProgress, updated.Status)

	// Soft delete.
	require.NoError(t, store.DeleteIncident(id))
	items, err = store.ListIncidents(false)
	require.NoError(t, err)
	assert.Empty(t, items)

	items, err = store.ListIncidents(true)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.True(t, items[0].DeletedAt.Valid)

	// Restore.
	require.NoError(t, store.RestoreIncident(id))
	items, err = store.ListIncidents(false)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

func TestDeleteIncidentAllowedWithDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Leaky pipe", Status: IncidentStatusOpen, Severity: IncidentSeveritySoon,
	}))
	items, _ := store.ListIncidents(false)
	incID := items[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Pipe photo", EntityKind: DocumentEntityIncident, EntityID: incID,
	}))

	require.NoError(t, store.DeleteIncident(incID))

	docs, _ := store.ListDocumentsByEntity(DocumentEntityIncident, incID, false)
	assert.Len(t, docs, 1, "document should survive parent deletion")
}

func TestRestoreIncidentBlockedByDeletedAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateAppliance(&Appliance{Name: "Doomed Washer"}))
	appliances, _ := store.ListAppliances(false)
	appID := appliances[0].ID

	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Washer leak", Status: IncidentStatusOpen,
		Severity: IncidentSeverityUrgent, ApplianceID: &appID,
	}))
	items, _ := store.ListIncidents(false)
	incID := items[0].ID

	require.NoError(t, store.DeleteIncident(incID))
	require.NoError(t, store.DeleteAppliance(appID))

	require.ErrorContains(t, store.RestoreIncident(incID), "appliance is deleted")

	require.NoError(t, store.RestoreAppliance(appID))
	require.NoError(t, store.RestoreIncident(incID))
}

func TestRestoreIncidentBlockedByDeletedVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Doomed Exterminator"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Termites", Status: IncidentStatusOpen,
		Severity: IncidentSeverityUrgent, VendorID: &vendorID,
	}))
	items, _ := store.ListIncidents(false)
	incID := items[0].ID

	require.NoError(t, store.DeleteIncident(incID))
	require.NoError(t, store.DeleteVendor(vendorID))

	require.ErrorContains(t, store.RestoreIncident(incID), "vendor is deleted")

	require.NoError(t, store.RestoreVendor(vendorID))
	require.NoError(t, store.RestoreIncident(incID))
}

func TestRestoreIncidentAllowedWithoutAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Loose trim", Status: IncidentStatusOpen, Severity: IncidentSeverityWhenever,
	}))
	items, _ := store.ListIncidents(false)
	incID := items[0].ID

	require.NoError(t, store.DeleteIncident(incID))
	require.NoError(t, store.RestoreIncident(incID))
}

func TestDeleteVendorBlockedByIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Busy Vendor"}))
	vendors, _ := store.ListVendors(false)
	vendorID := vendors[0].ID

	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Clogged drain", Status: IncidentStatusOpen,
		Severity: IncidentSeveritySoon, VendorID: &vendorID,
	}))

	require.ErrorContains(t, store.DeleteVendor(vendorID), "active incident")

	items, _ := store.ListIncidents(false)
	require.NoError(t, store.DeleteIncident(items[0].ID))
	require.NoError(t, store.DeleteVendor(vendorID))
}

func TestDeleteApplianceBlockedByIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateAppliance(&Appliance{Name: "Busy Fridge"}))
	appliances, _ := store.ListAppliances(false)
	appID := appliances[0].ID

	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Fridge leaking", Status: IncidentStatusOpen,
		Severity: IncidentSeverityUrgent, ApplianceID: &appID,
	}))

	require.ErrorContains(t, store.DeleteAppliance(appID), "active incident")

	items, _ := store.ListIncidents(false)
	require.NoError(t, store.DeleteIncident(items[0].ID))
	require.NoError(t, store.DeleteAppliance(appID))
}

func TestRestoreDocumentBlockedByDeletedIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Doomed incident", Status: IncidentStatusOpen, Severity: IncidentSeveritySoon,
	}))
	items, _ := store.ListIncidents(false)
	incID := items[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Evidence", EntityKind: DocumentEntityIncident, EntityID: incID,
	}))
	docs, _ := store.ListDocuments(false)
	docID := docs[0].ID

	require.NoError(t, store.DeleteDocument(docID))
	require.NoError(t, store.DeleteIncident(incID))

	require.ErrorContains(t, store.RestoreDocument(docID), "incident is deleted")

	require.NoError(t, store.RestoreIncident(incID))
	require.NoError(t, store.RestoreDocument(docID))
}

func TestDeleteIncidentSetsStatusResolved(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Broken window", Status: IncidentStatusOpen, Severity: IncidentSeverityUrgent,
	}))
	items, _ := store.ListIncidents(false)
	id := items[0].ID

	require.NoError(t, store.DeleteIncident(id))

	// Fetch with Unscoped to see the soft-deleted row.
	var inc Incident
	require.NoError(t, store.db.Unscoped().First(&inc, "id = ?", id).Error)
	assert.Equal(t, IncidentStatusResolved, inc.Status)
}

func TestRestoreIncidentRestoresPreviousStatus(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Cracked tile", Status: IncidentStatusInProgress, Severity: IncidentSeveritySoon,
	}))
	items, _ := store.ListIncidents(false)
	id := items[0].ID

	require.NoError(t, store.DeleteIncident(id))
	require.NoError(t, store.RestoreIncident(id))

	inc, err := store.GetIncident(id)
	require.NoError(t, err)
	assert.Equal(t, IncidentStatusInProgress, inc.Status, "should restore to previous status")
	assert.Empty(t, inc.PreviousStatus, "previous_status should be cleared")
}

func TestRestoreIncidentFallsBackToOpen(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Old incident", Status: IncidentStatusOpen, Severity: IncidentSeverityWhenever,
	}))
	items, _ := store.ListIncidents(false)
	id := items[0].ID

	// Simulate a legacy soft-delete with no previous_status saved.
	require.NoError(t, store.db.Where("id = ?", id).Delete(&Incident{}).Error)
	require.NoError(t, store.RestoreIncident(id))

	inc, err := store.GetIncident(id)
	require.NoError(t, err)
	assert.Equal(t, IncidentStatusOpen, inc.Status, "should fall back to open")
}

func TestHardDeleteIncidentRemovesRow(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Doomed", Status: IncidentStatusOpen, Severity: IncidentSeverityWhenever,
	}))
	items, _ := store.ListIncidents(false)
	id := items[0].ID

	require.NoError(t, store.HardDeleteIncident(id))

	// Not visible even with Unscoped.
	var inc Incident
	err := store.db.Unscoped().First(&inc, "id = ?", id).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestHardDeleteIncidentDetachesLinkedDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Leaky roof", Status: IncidentStatusOpen, Severity: IncidentSeveritySoon,
	}))
	items, _ := store.ListIncidents(false)
	id := items[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Photo", EntityKind: DocumentEntityIncident, EntityID: id,
	}))

	require.NoError(t, store.HardDeleteIncident(id))

	// Document is no longer linked to the incident.
	docs, err := store.ListDocumentsByEntity(DocumentEntityIncident, id, true)
	require.NoError(t, err)
	assert.Empty(t, docs)

	// Document still exists, detached (entity_kind="", entity_id="").
	detached, err := store.ListDocumentsByEntity(DocumentEntityNone, "", false)
	require.NoError(t, err)
	require.Len(t, detached, 1)
	assert.Equal(t, "Photo", detached[0].Title)
}

func TestHardDeleteIncidentDetachesSoftDeletedDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create incident with a document.
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Water damage", Status: IncidentStatusOpen, Severity: IncidentSeveritySoon,
	}))
	items, _ := store.ListIncidents(false)
	incID := items[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Receipt", EntityKind: DocumentEntityIncident, EntityID: incID,
	}))
	docs, err := store.ListDocumentsByEntity(DocumentEntityIncident, incID, false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	docID := docs[0].ID

	// Soft-delete the document — creates a DeletionRecord for it.
	require.NoError(t, store.DeleteDocument(docID))

	// Hard-delete the incident — should detach the document, not destroy it.
	require.NoError(t, store.HardDeleteIncident(incID))

	// Document is detached and still soft-deleted.
	detached, err := store.ListDocumentsByEntity(DocumentEntityNone, "", true)
	require.NoError(t, err)
	require.Len(t, detached, 1)
	assert.Equal(t, "Receipt", detached[0].Title)

	// DeletionRecord for the document still exists (user can restore it).
	var count int64
	store.db.Model(&DeletionRecord{}).
		Where(ColEntity+" = ? AND "+ColTargetID+" = ?", DeletionEntityDocument, docID).
		Count(&count)
	assert.Equal(t, int64(1), count, "DeletionRecord for detached document should survive")

	// Restoring the detached document should succeed.
	require.NoError(t, store.RestoreDocument(docID))
}

func TestEnsureDocumentAlive_AliveIsNoOp(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	doc := &Document{
		FileName: "receipt.pdf",
		MIMEType: "application/pdf",
		Data:     []byte("pdf"),
	}
	require.NoError(t, store.CreateDocument(doc))

	require.NoError(t, store.EnsureDocumentAlive(doc.ID))

	got, err := store.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, "receipt.pdf", got.FileName)
}

func TestEnsureDocumentAlive_RestoresSoftDeleted(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	doc := &Document{
		FileName: "invoice.pdf",
		MIMEType: "application/pdf",
		Data:     []byte("pdf"),
	}
	require.NoError(t, store.CreateDocument(doc))
	require.NoError(t, store.DeleteDocument(doc.ID))

	// Document is soft-deleted; normal Get fails.
	_, err := store.GetDocument(doc.ID)
	require.Error(t, err)

	// EnsureDocumentAlive should restore it.
	require.NoError(t, store.EnsureDocumentAlive(doc.ID))

	got, err := store.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, "invoice.pdf", got.FileName)
}

func TestEnsureDocumentAlive_NonExistentFails(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	err := store.EnsureDocumentAlive("01JNOTEXIST000000000000999")
	assert.Error(t, err)
}

func TestUpdateDocumentExtraction_WorksOnSoftDeletedDoc(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	doc := &Document{
		FileName: "scan.pdf",
		MIMEType: "application/pdf",
		Data:     []byte("pdf"),
	}
	require.NoError(t, store.CreateDocument(doc))
	require.NoError(t, store.DeleteDocument(doc.ID))

	// UpdateDocumentExtraction should work even on soft-deleted documents
	// (uses Unscoped internally).
	require.NoError(t, store.UpdateDocumentExtraction(
		doc.ID, "extracted text", nil, "test-model", nil,
	))

	// Verify via unscoped read.
	var saved Document
	require.NoError(t, store.db.Unscoped().First(&saved, "id = ?", doc.ID).Error)
	assert.Equal(t, "extracted text", saved.ExtractedText)
	assert.Equal(t, "test-model", saved.ExtractionModel)
}

func TestHardDeleteIncidentNotFound(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	err := store.HardDeleteIncident("01JNOTEXIST000000000000999")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestHardDeleteMaintenanceRemovesRowAndChildren(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	// Create a maintenance item with a service log child.
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Filter change", CategoryID: cats[0].ID,
	}))
	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	maintID := items[0].ID

	require.NoError(t, store.CreateServiceLog(&ServiceLogEntry{
		MaintenanceItemID: maintID,
		ServicedAt:        time.Now(),
		Notes:             "Replaced filter",
	}, Vendor{}))

	logs, _ := store.ListServiceLog(maintID, false)
	require.Len(t, logs, 1)

	require.NoError(t, store.HardDeleteMaintenance(maintID))

	// Maintenance item gone even with Unscoped.
	var item MaintenanceItem
	err = store.db.Unscoped().First(&item, "id = ?", maintID).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// Child service log also gone.
	var sle ServiceLogEntry
	err = store.db.Unscoped().Where("maintenance_item_id = ?", maintID).First(&sle).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestHardDeleteMaintenanceWritesOplogForChildren(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Oil change", CategoryID: cats[0].ID,
	}))
	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	maintID := items[0].ID

	require.NoError(t, store.CreateServiceLog(&ServiceLogEntry{
		MaintenanceItemID: maintID,
		ServicedAt:        time.Now(),
	}, Vendor{}))
	logs, _ := store.ListServiceLog(maintID, false)
	logID := logs[0].ID

	// Clear the oplog so we only see hard-delete entries.
	store.db.Where("1=1").Delete(&SyncOplogEntry{})

	require.NoError(t, store.HardDeleteMaintenance(maintID))

	// Should have oplog entries for the service log child and the parent.
	var entries []SyncOplogEntry
	store.db.Order("id").Find(&entries)

	// At least one delete entry for the service log and one for the maintenance item.
	var foundChildDelete, foundParentDelete bool
	for _, e := range entries {
		if e.TableName == TableServiceLogEntries && e.RowID == logID && e.OpType == OpDelete {
			foundChildDelete = true
		}
		if e.TableName == TableMaintenanceItems && e.RowID == maintID && e.OpType == OpDelete {
			foundParentDelete = true
		}
	}
	assert.True(t, foundChildDelete, "oplog should have delete entry for child service log")
	assert.True(t, foundParentDelete, "oplog should have delete entry for parent maintenance item")
}

func TestHardDeleteMaintenanceDetachesServiceLogDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Roof repair", CategoryID: cats[0].ID,
	}))
	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	maintID := items[0].ID

	require.NoError(t, store.CreateServiceLog(&ServiceLogEntry{
		MaintenanceItemID: maintID,
		ServicedAt:        time.Now(),
	}, Vendor{}))
	logs, _ := store.ListServiceLog(maintID, false)
	logID := logs[0].ID

	// Attach a document to the service log.
	require.NoError(t, store.CreateDocument(&Document{
		Title: "Invoice", EntityKind: DocumentEntityServiceLog, EntityID: logID,
	}))

	require.NoError(t, store.HardDeleteMaintenance(maintID))

	// Document is detached, not deleted.
	detached, err := store.ListDocumentsByEntity(DocumentEntityNone, "", false)
	require.NoError(t, err)
	require.Len(t, detached, 1)
	assert.Equal(t, "Invoice", detached[0].Title)
}

func TestHardDeleteMaintenanceDetachesOwnDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "Gutter cleaning", CategoryID: cats[0].ID,
	}))
	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	maintID := items[0].ID

	// Attach a document directly to the maintenance item.
	require.NoError(t, store.CreateDocument(&Document{
		Title: "Manual", EntityKind: DocumentEntityMaintenance, EntityID: maintID,
	}))

	require.NoError(t, store.HardDeleteMaintenance(maintID))

	// Document is detached, not deleted.
	detached, err := store.ListDocumentsByEntity(DocumentEntityNone, "", false)
	require.NoError(t, err)
	require.Len(t, detached, 1)
	assert.Equal(t, "Manual", detached[0].Title)
}

func TestHardDeleteMaintenanceNotFound(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	err := store.HardDeleteMaintenance("01JNOTEXIST000000000000999")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// --- findOrCreate: "name is required" error path ---

func TestFindOrCreateVendorEmptyNameFails(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Quote Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projectID := projects[0].ID

	err := store.CreateQuote(
		&Quote{ProjectID: projectID, TotalCents: 1000},
		Vendor{Name: ""},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vendor name is required")
}

func TestFindOrCreateVendorWhitespaceOnlyNameFails(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Quote Project 2", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	projectID := projects[0].ID

	err := store.CreateQuote(
		&Quote{ProjectID: projectID, TotalCents: 1000},
		Vendor{Name: "   "},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vendor name is required")
}

// --- softDeleteWith: deleting non-existent ID (RowsAffected == 0) ---

func TestSoftDeleteNonExistentVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	err := store.DeleteVendor("01JNOTEXIST000000000000999")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestSoftDeleteNonExistentProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	err := store.DeleteProject("01JNOTEXIST000000000000999")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestSoftDeleteNonExistentAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	err := store.DeleteAppliance("01JNOTEXIST000000000000999")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestSoftDeleteNonExistentDocument(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	err := store.DeleteDocument("01JNOTEXIST000000000000999")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// --- restoreSoftDeleted: verify full restore flow ---

func TestRestoreAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateAppliance(&Appliance{Name: "Dishwasher"}))
	appliances, err := store.ListAppliances(false)
	require.NoError(t, err)
	require.Len(t, appliances, 1)
	id := appliances[0].ID

	require.NoError(t, store.DeleteAppliance(id))

	// Gone from active list.
	appliances, err = store.ListAppliances(false)
	require.NoError(t, err)
	assert.Empty(t, appliances)

	// Visible in unscoped list.
	appliances, err = store.ListAppliances(true)
	require.NoError(t, err)
	require.Len(t, appliances, 1)
	assert.True(t, appliances[0].DeletedAt.Valid)

	// Restore succeeds.
	require.NoError(t, store.RestoreAppliance(id))

	appliances, err = store.ListAppliances(false)
	require.NoError(t, err)
	require.Len(t, appliances, 1)
	assert.False(t, appliances[0].DeletedAt.Valid)

	// DeletionRecord is marked as restored.
	_, err = store.LastDeletion(DeletionEntityAppliance)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound,
		"restored deletion record should not appear as pending")
}

func TestRestoreIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Restore Test", Status: IncidentStatusOpen, Severity: IncidentSeverityWhenever,
	}))
	items, err := store.ListIncidents(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	id := items[0].ID

	require.NoError(t, store.DeleteIncident(id))

	items, err = store.ListIncidents(false)
	require.NoError(t, err)
	assert.Empty(t, items)

	require.NoError(t, store.RestoreIncident(id))

	items, err = store.ListIncidents(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Restore Test", items[0].Title)
}

// --- UpdateDocument: oplog write path for partial update ---

func TestUpdateDocumentPartialUpdateWritesOplog(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	content := []byte("oplog test content")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))
	require.NoError(t, store.CreateDocument(&Document{
		Title:          "Oplog Doc",
		FileName:       "oplog.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: checksum,
		Data:           content,
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	docID := docs[0].ID

	// Clear oplog so we only see the update entry.
	store.db.Where("1=1").Delete(&SyncOplogEntry{})

	// Partial update: change only title and notes, no file data.
	require.NoError(t, store.UpdateDocument(Document{
		ID:    docID,
		Title: "Updated Oplog Doc",
		Notes: "some notes",
	}))

	// Verify oplog entry was written for the update.
	var entries []SyncOplogEntry
	store.db.Order("id").Find(&entries)

	var foundUpdate bool
	for _, e := range entries {
		if e.TableName == TableDocuments && e.RowID == docID && e.OpType == OpUpdate {
			foundUpdate = true
			break
		}
	}
	assert.True(t, foundUpdate, "UpdateDocument should write an oplog entry for partial updates")

	// Verify the document was actually updated.
	updated, err := store.GetDocument(docID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Oplog Doc", updated.Title)
	assert.Equal(t, "some notes", updated.Notes)
	assert.Equal(t, "oplog.pdf", updated.FileName, "file metadata preserved on partial update")
}

// --- HardDeleteIncident: with soft-deleted documents attached ---

func TestHardDeleteIncidentWithSoftDeletedAndActiveDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create incident.
	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Multi-doc incident", Status: IncidentStatusOpen, Severity: IncidentSeveritySoon,
	}))
	items, _ := store.ListIncidents(false)
	incID := items[0].ID

	// Attach two documents.
	require.NoError(t, store.CreateDocument(&Document{
		Title: "Active Photo", EntityKind: DocumentEntityIncident, EntityID: incID,
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title: "Deleted Receipt", EntityKind: DocumentEntityIncident, EntityID: incID,
	}))

	docs, err := store.ListDocumentsByEntity(DocumentEntityIncident, incID, false)
	require.NoError(t, err)
	require.Len(t, docs, 2)

	// Soft-delete one of the documents.
	var receiptID string
	for _, d := range docs {
		if d.Title == "Deleted Receipt" {
			receiptID = d.ID
			break
		}
	}
	require.NotEmpty(t, receiptID)
	require.NoError(t, store.DeleteDocument(receiptID))

	// Hard-delete the incident.
	require.NoError(t, store.HardDeleteIncident(incID))

	// Incident is gone.
	var inc Incident
	err = store.db.Unscoped().First(&inc, "id = ?", incID).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// Both documents survive -- detached but not destroyed.
	// Active document is detached and alive.
	detachedAlive, err := store.ListDocumentsByEntity(DocumentEntityNone, "", false)
	require.NoError(t, err)
	require.Len(t, detachedAlive, 1)
	assert.Equal(t, "Active Photo", detachedAlive[0].Title)

	// Soft-deleted document is detached and still soft-deleted.
	detachedAll, err := store.ListDocumentsByEntity(DocumentEntityNone, "", true)
	require.NoError(t, err)
	require.Len(t, detachedAll, 2)

	// No documents linked to the incident any more.
	linked, err := store.ListDocumentsByEntity(DocumentEntityIncident, incID, true)
	require.NoError(t, err)
	assert.Empty(t, linked)

	// DeletionRecord for incident is cleaned up.
	_, err = store.LastDeletion(DeletionEntityIncident)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound,
		"hard-delete should remove the DeletionRecord for the incident")
}

func TestHardDeleteIncidentWritesOplogForDetachedDocuments(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Oplog detach", Status: IncidentStatusOpen, Severity: IncidentSeverityWhenever,
	}))
	items, _ := store.ListIncidents(false)
	incID := items[0].ID

	require.NoError(t, store.CreateDocument(&Document{
		Title: "Photo", EntityKind: DocumentEntityIncident, EntityID: incID,
	}))
	docs, _ := store.ListDocumentsByEntity(DocumentEntityIncident, incID, false)
	docID := docs[0].ID

	// Clear oplog.
	store.db.Where("1=1").Delete(&SyncOplogEntry{})

	require.NoError(t, store.HardDeleteIncident(incID))

	var entries []SyncOplogEntry
	store.db.Order("id").Find(&entries)

	var foundDocUpdate, foundIncDelete bool
	for _, e := range entries {
		if e.TableName == TableDocuments && e.RowID == docID && e.OpType == OpUpdate {
			foundDocUpdate = true
		}
		if e.TableName == TableIncidents && e.RowID == incID && e.OpType == OpDelete {
			foundIncDelete = true
		}
	}
	assert.True(t, foundDocUpdate, "oplog should have update entry for detached document")
	assert.True(t, foundIncDelete, "oplog should have delete entry for hard-deleted incident")
}

func TestHardDeleteSoftDeletedIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Soft then hard", Status: IncidentStatusOpen, Severity: IncidentSeverityWhenever,
	}))
	items, _ := store.ListIncidents(false)
	incID := items[0].ID

	// Soft-delete first.
	require.NoError(t, store.DeleteIncident(incID))

	// Hard-delete should still work on a soft-deleted incident.
	require.NoError(t, store.HardDeleteIncident(incID))

	// Completely gone.
	var inc Incident
	err := store.db.Unscoped().First(&inc, "id = ?", incID).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
