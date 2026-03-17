// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

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

	v := &Vendor{Name: "Acme Plumbing"}
	require.NoError(t, store.CreateVendor(v))

	op := lastOplogEntry(t, store, TableVendors, v.ID)
	assert.Equal(t, OpInsert, op.OpType)
	assert.Equal(t, TableVendors, op.TableName)
	assert.Equal(t, v.ID, op.RowID)
	assert.NotEmpty(t, op.DeviceID)
	assert.NotNil(t, op.AppliedAt)
	assert.Contains(t, op.Payload, `"name":"Acme Plumbing"`)
}

func TestOplogInsertProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{Title: "Kitchen Reno", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned}
	require.NoError(t, store.CreateProject(p))

	op := lastOplogEntry(t, store, TableProjects, p.ID)
	assert.Equal(t, OpInsert, op.OpType)
	assert.Contains(t, op.Payload, `"title":"Kitchen Reno"`)
}

func TestOplogInsertAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	a := &Appliance{Name: "Dishwasher", Brand: "Bosch"}
	require.NoError(t, store.CreateAppliance(a))

	op := lastOplogEntry(t, store, TableAppliances, a.ID)
	assert.Equal(t, OpInsert, op.OpType)
	assert.Contains(t, op.Payload, `"name":"Dishwasher"`)
}

func TestOplogInsertIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	inc := &Incident{Title: "Roof Leak", Status: IncidentStatusOpen}
	require.NoError(t, store.CreateIncident(inc))

	op := lastOplogEntry(t, store, TableIncidents, inc.ID)
	assert.Equal(t, OpInsert, op.OpType)
	assert.Contains(t, op.Payload, `"title":"Roof Leak"`)
}

func TestOplogInsertQuote(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

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

	hp := HouseProfile{Nickname: "Home"}
	require.NoError(t, store.CreateHouseProfile(hp))

	profile, err := store.HouseProfile()
	require.NoError(t, err)

	op := lastOplogEntry(t, store, TableHouseProfiles, profile.ID)
	assert.Equal(t, OpInsert, op.OpType)
	assert.Contains(t, op.Payload, `"nickname":"Home"`)
}

// --- Update oplog entries (explicit in Store methods) ---

func TestOplogUpdateVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

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

	// documentOplogPayload mirrors Document's json tags (snake_case keys).
	var payload documentOplogPayload
	require.NoError(t, json.Unmarshal([]byte(op.Payload), &payload))
	assert.Equal(t, "Invoice", payload.Title)
	assert.Equal(t, "invoice.pdf", payload.FileName)
	assert.Equal(t, "application/pdf", payload.MIMEType)
}

// --- Missing device ID cell returns error ---

func TestOplogFailsWithoutDeviceIDCell(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create a raw GORM session without the device ID cell in context.
	// This simulates code that bypasses Store and uses gorm.DB directly.
	rawDB := store.db.WithContext(context.Background())
	v := &Vendor{Name: "No Cell Vendor"}
	err := rawDB.Create(v).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "device ID cell not in context")
}

// --- Context flag suppresses oplog writes ---

func TestOplogSyncApplyingSuppressesWrites(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

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

	id := store.DeviceID()
	assert.Len(t, id, 26, "device ID should be a 26-char ULID")

	// Subsequent calls return the same ID (cached).
	assert.Equal(t, id, store.DeviceID())
}

func TestDeviceIDPersistedAcrossStoreInstances(t *testing.T) {
	t.Parallel()
	dbPath := t.TempDir() + "/persist.db"

	store1, err := Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, store1.AutoMigrate())
	id1 := store1.DeviceID()
	require.NoError(t, store1.Close())

	store2, err := Open(dbPath)
	require.NoError(t, err)
	id2 := store2.DeviceID()
	require.NoError(t, store2.Close())

	assert.Equal(t, id1, id2, "device ID should persist in the DB")
}

func TestDeviceIDIsolationBetweenStores(t *testing.T) {
	t.Parallel()

	// Use fresh DBs (not template copies) so each gets a unique device row.
	open := func() *Store {
		t.Helper()
		path := t.TempDir() + "/test.db"
		s, err := Open(path)
		require.NoError(t, err)
		require.NoError(t, s.AutoMigrate())
		t.Cleanup(func() { _ = s.Close() })
		return s
	}
	store1 := open()
	store2 := open()

	id1 := store1.DeviceID()
	id2 := store2.DeviceID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "separate stores should have independent device IDs")
}

func TestOplogUsesStoreDeviceID(t *testing.T) {
	t.Parallel()

	// Use fresh DBs to ensure distinct device IDs.
	open := func() *Store {
		t.Helper()
		path := t.TempDir() + "/test.db"
		s, err := Open(path)
		require.NoError(t, err)
		require.NoError(t, s.AutoMigrate())
		require.NoError(t, s.SeedDefaults())
		require.NoError(t, s.SetMaxDocumentSize(50<<20))
		t.Cleanup(func() { _ = s.Close() })
		return s
	}
	store1 := open()
	store2 := open()

	v1 := &Vendor{Name: "Store1 Vendor"}
	require.NoError(t, store1.CreateVendor(v1))

	v2 := &Vendor{Name: "Store2 Vendor"}
	require.NoError(t, store2.CreateVendor(v2))

	op1 := lastOplogEntry(t, store1, TableVendors, v1.ID)
	op2 := lastOplogEntry(t, store2, TableVendors, v2.ID)

	assert.Equal(t, store1.DeviceID(), op1.DeviceID, "oplog should use store1's device ID")
	assert.Equal(t, store2.DeviceID(), op2.DeviceID, "oplog should use store2's device ID")
	assert.NotEqual(
		t,
		op1.DeviceID,
		op2.DeviceID,
		"different stores should produce different device IDs",
	)
}

func TestSetDeviceID(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	original := store.DeviceID()
	assert.NotEmpty(t, original)

	store.SetDeviceID("new-device-id")
	assert.Equal(t, "new-device-id", store.DeviceID())

	// Oplog entries written after SetDeviceID should use the new ID.
	v := &Vendor{Name: "After SetDeviceID"}
	require.NoError(t, store.CreateVendor(v))

	op := lastOplogEntry(t, store, TableVendors, v.ID)
	assert.Equal(t, "new-device-id", op.DeviceID)
}

// --- resolve returns error on failure ---

func TestResolveDeviceIDReturnsErrorOnDBFailure(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Close the store to make DB queries fail.
	require.NoError(t, store.Close())

	// resolveDeviceID should propagate the error, not silently return "".
	_, err := resolveDeviceID(store.db)
	require.Error(t, err, "resolveDeviceID should return error when DB is closed")
}

// --- UnsyncedOps / MarkSynced ---

func TestUnsyncedOpsAndMarkSynced(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

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

	v, err := store.FindOrCreateVendor(Vendor{Name: "Brand New Vendor"})
	require.NoError(t, err)

	op := lastOplogEntry(t, store, TableVendors, v.ID)
	assert.Equal(t, OpInsert, op.OpType)
}

func TestOplogFindOrCreateVendorExisting(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

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

	cats, _ := store.MaintenanceCategories()
	require.NotEmpty(t, cats)

	op := lastOplogEntry(t, store, TableMaintenanceCategories, cats[0].ID)
	assert.Equal(t, OpInsert, op.OpType)
}

// --- Project type oplog ---

func TestOplogInsertProjectType(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NotEmpty(t, types)

	op := lastOplogEntry(t, store, TableProjectTypes, types[0].ID)
	assert.Equal(t, OpInsert, op.OpType)
}

// --- ConflictLosers ---

func TestConflictLosersEmpty(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	losers, err := store.ConflictLosers()
	require.NoError(t, err)
	assert.Empty(t, losers)
}

func TestConflictLosersReturnsUnappliedSyncedOps(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	db := store.GormDB()

	// Insert a conflict loser: synced (synced_at set) but not applied (applied_at NULL).
	now := time.Now()
	require.NoError(t, db.Table("sync_oplog_entries").Create(map[string]any{
		"id":         "conflict-loser-1",
		"table_name": TableVendors,
		"row_id":     "vendor-1",
		"op_type":    OpUpdate,
		"payload":    `{"name":"Remote Vendor"}`,
		"device_id":  "dev-remote",
		"created_at": now,
		"applied_at": nil,
		"synced_at":  now,
	}).Error)

	// Insert a normal applied+synced op (should NOT appear).
	require.NoError(t, db.Table("sync_oplog_entries").Create(map[string]any{
		"id":         "applied-op-1",
		"table_name": TableVendors,
		"row_id":     "vendor-2",
		"op_type":    OpInsert,
		"payload":    `{"id":"vendor-2","name":"Applied Vendor"}`,
		"device_id":  "dev-local",
		"created_at": now.Add(-time.Minute),
		"applied_at": now,
		"synced_at":  now,
	}).Error)

	// Insert a local unsynced op (should NOT appear).
	require.NoError(t, db.Table("sync_oplog_entries").Create(map[string]any{
		"id":         "unsynced-op-1",
		"table_name": TableVendors,
		"row_id":     "vendor-3",
		"op_type":    OpInsert,
		"payload":    `{"id":"vendor-3","name":"Unsynced Vendor"}`,
		"device_id":  "dev-local",
		"created_at": now.Add(-2 * time.Minute),
		"applied_at": now,
		"synced_at":  nil,
	}).Error)

	losers, err := store.ConflictLosers()
	require.NoError(t, err)
	require.Len(t, losers, 1)
	assert.Equal(t, "conflict-loser-1", losers[0].ID)
	assert.Equal(t, TableVendors, losers[0].TableName)
	assert.Equal(t, "vendor-1", losers[0].RowID)
	assert.Equal(t, OpUpdate, losers[0].OpType)
	assert.Equal(t, "dev-remote", losers[0].DeviceID)
	assert.Nil(t, losers[0].AppliedAt)
	assert.NotNil(t, losers[0].SyncedAt)
}

func TestConflictLosersOrderedByCreatedAtDescThenIDDesc(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	db := store.GormDB()
	now := time.Now()

	// Insert two conflict losers with different created_at.
	require.NoError(t, db.Table("sync_oplog_entries").Create(map[string]any{
		"id":         "older-loser",
		"table_name": TableVendors,
		"row_id":     "vendor-1",
		"op_type":    OpUpdate,
		"payload":    `{"name":"Older"}`,
		"device_id":  "dev-remote",
		"created_at": now.Add(-time.Hour),
		"applied_at": nil,
		"synced_at":  now,
	}).Error)

	require.NoError(t, db.Table("sync_oplog_entries").Create(map[string]any{
		"id":         "newer-loser",
		"table_name": TableVendors,
		"row_id":     "vendor-2",
		"op_type":    OpUpdate,
		"payload":    `{"name":"Newer"}`,
		"device_id":  "dev-remote",
		"created_at": now,
		"applied_at": nil,
		"synced_at":  now,
	}).Error)

	losers, err := store.ConflictLosers()
	require.NoError(t, err)
	require.Len(t, losers, 2)
	// Newest first (DESC).
	assert.Equal(t, "newer-loser", losers[0].ID)
	assert.Equal(t, "older-loser", losers[1].ID)
}

// --- JSON tag enforcement ---

func TestAllSyncableModelsHaveJSONTags(t *testing.T) {
	t.Parallel()

	models := []any{
		HouseProfile{},
		ProjectType{},
		Vendor{},
		Project{},
		Quote{},
		MaintenanceCategory{},
		Appliance{},
		MaintenanceItem{},
		Incident{},
		ServiceLogEntry{},
		Document{},
	}

	for _, m := range models {
		rt := reflect.TypeOf(m)
		for i := range rt.NumField() {
			f := rt.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := f.Tag.Get("json")
			assert.NotEmpty(t, tag,
				"%s.%s is missing a json struct tag", rt.Name(), f.Name)
		}
	}
}
