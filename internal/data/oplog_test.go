// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func lastOplogEntry(t *testing.T, store *Store, table, rowID string) SyncOplogEntry {
	t.Helper()
	ops, err := store.OplogEntries(table, rowID)
	require.NoError(t, err)
	require.NotEmpty(t, ops, "expected oplog entries for %s/%s", table, rowID)
	return ops[len(ops)-1]
}

func oplogCount(t *testing.T, store *Store, table, rowID string) int {
	t.Helper()
	ops, err := store.OplogEntries(table, rowID)
	require.NoError(t, err)
	return len(ops)
}

// --- Insert oplog entries (AfterCreate hooks) ---

func TestOplogInsertVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v := &Vendor{Name: "Acme Plumbing"}
	require.NoError(t, store.CreateVendor(v))

	op := lastOplogEntry(t, store, TableVendors, v.ID)
	assert.Equal(t, OpInsert, op.OpType)
	assert.Equal(t, TableVendors, op.TableName)
	assert.Equal(t, v.ID, op.RowID)
	assert.NotEmpty(t, op.DeviceID)
	assert.NotNil(t, op.AppliedAt)
	// Model structs use PascalCase JSON keys (no json tags).
	assert.Contains(t, op.Payload, `"Name":"Acme Plumbing"`)
}

func TestOplogInsertProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	types, _ := store.ProjectTypes()
	p := &Project{Title: "Kitchen Reno", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned}
	require.NoError(t, store.CreateProject(p))

	op := lastOplogEntry(t, store, TableProjects, p.ID)
	assert.Equal(t, OpInsert, op.OpType)
	assert.Contains(t, op.Payload, `"Title":"Kitchen Reno"`)
}

func TestOplogInsertAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	a := &Appliance{Name: "Dishwasher", Brand: "Bosch"}
	require.NoError(t, store.CreateAppliance(a))

	op := lastOplogEntry(t, store, TableAppliances, a.ID)
	assert.Equal(t, OpInsert, op.OpType)
	assert.Contains(t, op.Payload, `"Name":"Dishwasher"`)
}

func TestOplogInsertIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	inc := &Incident{Title: "Roof Leak", Status: IncidentStatusOpen}
	require.NoError(t, store.CreateIncident(inc))

	op := lastOplogEntry(t, store, TableIncidents, inc.ID)
	assert.Equal(t, OpInsert, op.OpType)
	assert.Contains(t, op.Payload, `"Title":"Roof Leak"`)
}

func TestOplogInsertQuote(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	types, _ := store.ProjectTypes()
	p := &Project{Title: "Bath Remodel", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned}
	require.NoError(t, store.CreateProject(p))

	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: p.ID, TotalCents: 5000},
		Vendor{Name: "V1"},
	))

	quotes, _ := store.ListQuotes(false)
	require.NotEmpty(t, quotes)
	op := lastOplogEntry(t, store, TableQuotes, quotes[0].ID)
	assert.Equal(t, OpInsert, op.OpType)
}

func TestOplogInsertHouseProfile(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	hp := HouseProfile{Nickname: "Home"}
	require.NoError(t, store.CreateHouseProfile(hp))

	profile, err := store.HouseProfile()
	require.NoError(t, err)

	op := lastOplogEntry(t, store, TableHouseProfiles, profile.ID)
	assert.Equal(t, OpInsert, op.OpType)
	assert.Contains(t, op.Payload, `"Nickname":"Home"`)
}

// --- Update oplog entries (explicit in Store methods) ---

func TestOplogUpdateVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v := &Vendor{Name: "Old Name"}
	require.NoError(t, store.CreateVendor(v))

	v.Name = "New Name"
	require.NoError(t, store.UpdateVendor(*v))

	ops, err := store.OplogEntries(TableVendors, v.ID)
	require.NoError(t, err)
	require.Len(t, ops, 2) // insert + update

	assert.Equal(t, OpInsert, ops[0].OpType)
	assert.Equal(t, OpUpdate, ops[1].OpType)
	// updateByID passes the values map, which uses column name keys.
	assert.Contains(t, ops[1].Payload, `New Name`)
}

func TestOplogUpdateProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	types, _ := store.ProjectTypes()
	p := &Project{Title: "Before", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned}
	require.NoError(t, store.CreateProject(p))

	p.Title = "After"
	require.NoError(t, store.UpdateProject(*p))

	ops, err := store.OplogEntries(TableProjects, p.ID)
	require.NoError(t, err)
	require.Len(t, ops, 2)
	assert.Equal(t, OpUpdate, ops[1].OpType)
}

func TestOplogUpdateIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	inc := &Incident{Title: "Leak", Status: IncidentStatusOpen}
	require.NoError(t, store.CreateIncident(inc))

	inc.Title = "Fixed Leak"
	require.NoError(t, store.UpdateIncident(*inc))

	ops, err := store.OplogEntries(TableIncidents, inc.ID)
	require.NoError(t, err)
	require.Len(t, ops, 2)
	assert.Equal(t, OpUpdate, ops[1].OpType)
}

// --- Delete oplog entries (softDelete) ---

func TestOplogDeleteVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v := &Vendor{Name: "To Delete"}
	require.NoError(t, store.CreateVendor(v))

	require.NoError(t, store.DeleteVendor(v.ID))

	ops, err := store.OplogEntries(TableVendors, v.ID)
	require.NoError(t, err)
	require.Len(t, ops, 2) // insert + delete
	assert.Equal(t, OpDelete, ops[1].OpType)
}

func TestOplogDeleteProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	types, _ := store.ProjectTypes()
	p := &Project{Title: "Doomed", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned}
	require.NoError(t, store.CreateProject(p))

	require.NoError(t, store.DeleteProject(p.ID))

	ops, err := store.OplogEntries(TableProjects, p.ID)
	require.NoError(t, err)
	require.Len(t, ops, 2)
	assert.Equal(t, OpDelete, ops[1].OpType)
}

func TestOplogDeleteIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	inc := &Incident{Title: "Temporary", Status: IncidentStatusOpen}
	require.NoError(t, store.CreateIncident(inc))

	require.NoError(t, store.DeleteIncident(inc.ID))

	ops, err := store.OplogEntries(TableIncidents, inc.ID)
	require.NoError(t, err)
	require.Len(t, ops, 2)
	assert.Equal(t, OpDelete, ops[1].OpType)
}

// --- Restore oplog entries ---

func TestOplogRestoreVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v := &Vendor{Name: "Restorable"}
	require.NoError(t, store.CreateVendor(v))
	require.NoError(t, store.DeleteVendor(v.ID))
	require.NoError(t, store.RestoreVendor(v.ID))

	ops, err := store.OplogEntries(TableVendors, v.ID)
	require.NoError(t, err)
	require.Len(t, ops, 3) // insert + delete + restore
	assert.Equal(t, OpInsert, ops[0].OpType)
	assert.Equal(t, OpDelete, ops[1].OpType)
	assert.Equal(t, OpRestore, ops[2].OpType)
}

func TestOplogRestoreProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	types, _ := store.ProjectTypes()
	p := &Project{Title: "Phoenix", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned}
	require.NoError(t, store.CreateProject(p))
	require.NoError(t, store.DeleteProject(p.ID))
	require.NoError(t, store.RestoreProject(p.ID))

	ops, err := store.OplogEntries(TableProjects, p.ID)
	require.NoError(t, err)
	require.Len(t, ops, 3)
	assert.Equal(t, OpRestore, ops[2].OpType)
}

// --- Document oplog excludes BLOB ---

func TestOplogDocumentExcludesBLOB(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	doc := &Document{
		Title:    "Invoice",
		FileName: "invoice.pdf",
		MIMEType: "application/pdf",
		Data:     []byte("fake PDF content"),
	}
	require.NoError(t, store.CreateDocument(doc))

	op := lastOplogEntry(t, store, TableDocuments, doc.ID)
	assert.Equal(t, OpInsert, op.OpType)

	// Payload should NOT contain the raw data.
	assert.NotContains(t, op.Payload, "fake PDF content")

	// documentOplogPayload uses explicit json tags (lowercase keys).
	var payload documentOplogPayload
	require.NoError(t, json.Unmarshal([]byte(op.Payload), &payload))
	assert.Equal(t, "Invoice", payload.Title)
	assert.Equal(t, "invoice.pdf", payload.FileName)
	assert.Equal(t, "application/pdf", payload.MIMEType)
}

// --- Context flag suppresses oplog writes ---

func TestOplogSyncApplyingSuppressesWrites(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	ctx := WithSyncApplying(context.Background())
	v := &Vendor{Name: "Remote Vendor"}
	require.NoError(t, store.db.WithContext(ctx).Create(v).Error)

	ops, err := store.OplogEntries(TableVendors, v.ID)
	require.NoError(t, err)
	assert.Empty(t, ops, "no oplog entry should be written when sync-applying")
}

func TestOplogSyncApplyingSuppressesSoftDelete(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v := &Vendor{Name: "Local Then Remote Delete"}
	require.NoError(t, store.CreateVendor(v))

	before := oplogCount(t, store, TableVendors, v.ID)

	ctx := WithSyncApplying(context.Background())
	db := store.db.WithContext(ctx)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return softDeleteWith(tx, &Vendor{}, DeletionEntityVendor, v.ID)
	}))

	after := oplogCount(t, store, TableVendors, v.ID)
	assert.Equal(t, before, after, "sync-applying soft delete should not write oplog")
}

func TestOplogSyncApplyingSuppressesUpdate(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v := &Vendor{Name: "Original"}
	require.NoError(t, store.CreateVendor(v))

	before := oplogCount(t, store, TableVendors, v.ID)

	ctx := WithSyncApplying(context.Background())
	require.NoError(
		t,
		updateByIDWith(store.db.WithContext(ctx), TableVendors, &Vendor{}, v.ID, map[string]any{
			ColName: "Updated Remotely",
		}),
	)

	after := oplogCount(t, store, TableVendors, v.ID)
	assert.Equal(t, before, after, "sync-applying update should not write oplog")
}

// --- Device ID ---

func TestDeviceIDLazyInit(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	id := store.DeviceID()
	assert.Len(t, id, 26, "device ID should be a 26-char ULID")

	// Subsequent calls return the same ID (cached).
	assert.Equal(t, id, store.DeviceID())
}

func TestDeviceIDPersistedAcrossStoreInstances(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	id1 := store.DeviceID()
	ResetCachedDeviceID()
	id2 := store.DeviceID()

	assert.Equal(t, id1, id2, "device ID should persist in the DB")
}

// --- UnsyncedOps / MarkSynced ---

func TestUnsyncedOpsAndMarkSynced(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v := &Vendor{Name: "Sync Me"}
	require.NoError(t, store.CreateVendor(v))

	unsynced, err := store.UnsyncedOps()
	require.NoError(t, err)
	found := false
	var opIDs []string
	for _, op := range unsynced {
		if op.TableName == TableVendors && op.RowID == v.ID {
			found = true
			opIDs = append(opIDs, op.ID)
		}
	}
	require.True(t, found, "vendor insert should appear in unsynced ops")

	require.NoError(t, store.MarkSynced(opIDs))

	unsynced2, err := store.UnsyncedOps()
	require.NoError(t, err)
	for _, op := range unsynced2 {
		if op.TableName == TableVendors && op.RowID == v.ID {
			t.Fatal("vendor ops should be marked as synced")
		}
	}
}

func TestMarkSyncedEmptySlice(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.MarkSynced(nil))
	require.NoError(t, store.MarkSynced([]string{}))
}

// --- Non-syncable tables produce no oplog ---

func TestOplogExcludesNonSyncableTables(t *testing.T) {
	t.Parallel()

	for _, table := range []string{
		TableDeletionRecords,
		TableSettings,
		TableChatInputs,
		TableSyncOplogEntries,
		TableSyncDevices,
	} {
		assert.False(t, syncableTable(table), "%s should not be syncable", table)
	}

	for _, table := range []string{
		TableVendors,
		TableProjects,
		TableQuotes,
		TableAppliances,
		TableMaintenanceItems,
		TableMaintenanceCategories,
		TableProjectTypes,
		TableIncidents,
		TableServiceLogEntries,
		TableDocuments,
		TableHouseProfiles,
	} {
		assert.True(t, syncableTable(table), "%s should be syncable", table)
	}
}

// --- findOrCreate produces correct oplog entries ---

func TestOplogFindOrCreateVendorNew(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v, err := store.FindOrCreateVendor(Vendor{Name: "Brand New Vendor"})
	require.NoError(t, err)

	op := lastOplogEntry(t, store, TableVendors, v.ID)
	assert.Equal(t, OpInsert, op.OpType)
}

func TestOplogFindOrCreateVendorExisting(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v := &Vendor{Name: "Existing Co"}
	require.NoError(t, store.CreateVendor(v))

	before := oplogCount(t, store, TableVendors, v.ID)
	_, err := store.FindOrCreateVendor(Vendor{Name: "Existing Co"})
	require.NoError(t, err)

	// findOrCreate of existing vendor does a contact field sync via
	// tx.Model().Updates() which does NOT go through updateByIDWith,
	// so no additional oplog entry is expected.
	after := oplogCount(t, store, TableVendors, v.ID)
	assert.Equal(t, before, after, "findOrCreate existing should not write extra oplog")
}

func TestOplogFindOrCreateVendorRestore(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v := &Vendor{Name: "Deleted Then Found"}
	require.NoError(t, store.CreateVendor(v))
	require.NoError(t, store.DeleteVendor(v.ID))

	before := oplogCount(t, store, TableVendors, v.ID)
	_, err := store.FindOrCreateVendor(Vendor{Name: "Deleted Then Found"})
	require.NoError(t, err)

	ops, err := store.OplogEntries(TableVendors, v.ID)
	require.NoError(t, err)
	assert.Greater(
		t,
		len(ops),
		before,
		"findOrCreate of deleted vendor should produce restore oplog",
	)

	hasRestore := false
	for _, op := range ops {
		if op.OpType == OpRestore {
			hasRestore = true
		}
	}
	assert.True(t, hasRestore, "should have a restore oplog entry")
}

// --- AllOplogEntries ---

func TestAllOplogEntries(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	v := &Vendor{Name: "V1"}
	require.NoError(t, store.CreateVendor(v))

	a := &Appliance{Name: "A1"}
	require.NoError(t, store.CreateAppliance(a))

	all, err := store.AllOplogEntries()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(all), 2)
}

// --- Document update oplog ---

func TestOplogDocumentUpdate(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	doc := &Document{
		Title:    "Original Title",
		FileName: "doc.pdf",
		MIMEType: "application/pdf",
		Data:     []byte("content"),
	}
	require.NoError(t, store.CreateDocument(doc))

	doc.Title = "Updated Title"
	require.NoError(t, store.UpdateDocument(*doc))

	ops, err := store.OplogEntries(TableDocuments, doc.ID)
	require.NoError(t, err)
	require.Len(t, ops, 2) // insert + update
	assert.Equal(t, OpUpdate, ops[1].OpType)

	var payload documentOplogPayload
	require.NoError(t, json.Unmarshal([]byte(ops[1].Payload), &payload))
	assert.Equal(t, "Updated Title", payload.Title)
	// Update payload should also exclude BLOB.
	assert.NotContains(t, ops[1].Payload, "content")
}

// --- Service log entry oplog ---

func TestOplogInsertServiceLogEntry(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	cats, _ := store.MaintenanceCategories()
	mi := &MaintenanceItem{Name: "Filter Change", CategoryID: cats[0].ID}
	require.NoError(t, store.CreateMaintenance(mi))

	sle := &ServiceLogEntry{
		MaintenanceItemID: mi.ID,
		Notes:             "Changed filter",
	}
	require.NoError(t, store.CreateServiceLog(sle, Vendor{}))

	op := lastOplogEntry(t, store, TableServiceLogEntries, sle.ID)
	assert.Equal(t, OpInsert, op.OpType)
}

// --- Maintenance category oplog ---

func TestOplogInsertMaintenanceCategory(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	cats, _ := store.MaintenanceCategories()
	require.NotEmpty(t, cats)

	op := lastOplogEntry(t, store, TableMaintenanceCategories, cats[0].ID)
	assert.Equal(t, OpInsert, op.OpType)
}

// --- Project type oplog ---

func TestOplogInsertProjectType(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	ResetCachedDeviceID()

	types, _ := store.ProjectTypes()
	require.NotEmpty(t, types)

	op := lastOplogEntry(t, store, TableProjectTypes, types[0].ID)
	assert.Equal(t, OpInsert, op.OpType)
}
