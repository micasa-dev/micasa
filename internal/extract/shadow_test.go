// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *data.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())
	return store
}

func jn(s string) json.Number { return json.Number(s) }

func TestShadowDB_NewAndClose(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)
	assert.NotNil(t, sdb)
	assert.Empty(t, sdb.created)
}

func TestShadowDB_StageCreateVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name":  "Garcia Plumbing",
			"phone": "555-1234",
		}},
	}
	require.NoError(t, sdb.Stage(ops))

	ids := sdb.CreatedIDs(data.TableVendors)
	require.Len(t, ids, 1)
	assert.Equal(t, "1", ids[0])
}

func TestShadowDB_StageMultipleVendors(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{"name": "Vendor A"}},
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{"name": "Vendor B"}},
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{"name": "Vendor C"}},
	}
	require.NoError(t, sdb.Stage(ops))

	ids := sdb.CreatedIDs(data.TableVendors)
	require.Len(t, ids, 3)
	assert.Equal(t, "1", ids[0])
	assert.Equal(t, "2", ids[1])
	assert.Equal(t, "3", ids[2])
}

func TestShadowDB_StageSkipsUpdates(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionUpdate, Table: data.TableDocuments, Data: map[string]any{
			"id":    jn("42"),
			"title": "Updated Title",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	assert.Empty(t, sdb.CreatedIDs(data.TableDocuments))
}

func TestShadowDB_StageRejectsEmptyData(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{}},
	}
	err = sdb.Stage(ops)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no columns to insert")
}

func TestShadowDB_CommitVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name":  "Garcia Plumbing",
			"phone": "555-1234",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	assert.Equal(t, "Garcia Plumbing", vendors[0].Name)
	assert.Equal(t, "555-1234", vendors[0].Phone)
}

func TestShadowDB_CommitVendorThenQuote_CrossReference(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create a project for the quote to reference.
	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, types)
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	projectID := projects[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// The LLM creates a vendor then references it by ordinal ID "1" in the quote.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "Garcia Plumbing",
		}},
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   "1",
			"project_id":  projectID,
			"total_cents": jn("150000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	// Verify the quote was created with the correct real vendor ID.
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	realVendorID := vendors[0].ID

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, realVendorID, quotes[0].VendorID)
	assert.Equal(t, projectID, quotes[0].ProjectID)
	assert.Equal(t, int64(150000), quotes[0].TotalCents)
}

func TestShadowDB_CommitApplianceThenMaintenance_CrossReference(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Get a maintenance category for the item.
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)
	catID := cats[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableAppliances, Data: map[string]any{
			"name":     "HVAC Unit",
			"brand":    "Carrier",
			"location": "Basement",
		}},
		{Action: ActionCreate, Table: data.TableMaintenanceItems, Data: map[string]any{
			"name":            "Replace HVAC Filter",
			"appliance_id":    "1",
			"category_id":     catID,
			"interval_months": jn("3"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	appliances, err := store.ListAppliances(false)
	require.NoError(t, err)
	require.Len(t, appliances, 1)
	realAppID := appliances[0].ID
	assert.Equal(t, "HVAC Unit", appliances[0].Name)
	assert.Equal(t, "Carrier", appliances[0].Brand)

	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Replace HVAC Filter", items[0].Name)
	require.NotNil(t, items[0].ApplianceID)
	assert.Equal(t, realAppID, *items[0].ApplianceID)
	assert.Equal(t, catID, items[0].CategoryID)
	assert.Equal(t, 3, items[0].IntervalMonths)
}

func TestShadowDB_CommitMultipleVendorsAndQuotes(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Bathroom Renovation",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	projectID := projects[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{"name": "Plumber A"}},
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{"name": "Plumber B"}},
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   "1",
			"project_id":  projectID,
			"total_cents": jn("100000"),
		}},
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   "2",
			"project_id":  projectID,
			"total_cents": jn("200000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 2)

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 2)

	// Each quote should reference the correct vendor.
	vendorIDByName := map[string]string{}
	for _, v := range vendors {
		vendorIDByName[v.Name] = v.ID
	}
	for _, q := range quotes {
		if q.TotalCents == 100000 {
			assert.Equal(t, vendorIDByName["Plumber A"], q.VendorID)
		} else {
			assert.Equal(t, vendorIDByName["Plumber B"], q.VendorID)
		}
	}
}

func TestShadowDB_CommitDocumentUpdate(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create a document to update.
	doc := &data.Document{Title: "Original Title", Notes: "original"}
	require.NoError(t, store.CreateDocument(doc))

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionUpdate, Table: data.TableDocuments, Data: map[string]any{
			"id":    doc.ID,
			"title": "Updated Title",
			"notes": "updated notes",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	updated, err := store.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", updated.Title)
	assert.Equal(t, "updated notes", updated.Notes)
}

func TestShadowDB_CommitEmptyBatch(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	require.NoError(t, sdb.Stage(nil))
	require.NoError(t, sdb.Commit(store, nil))
}

func TestShadowDB_CommitDuplicateVendorUsesExisting(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Pre-create a vendor.
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Existing Co"}))

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// LLM creates a vendor with the same name.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "Existing Co",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	// Should still be just one vendor (findOrCreateVendor deduplicates).
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	assert.Len(t, vendors, 1)
}

func TestShadowDB_CommitDocumentWithEntityRemap(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "DocVendor",
		}},
		{Action: ActionCreate, Table: data.TableDocuments, Data: map[string]any{
			"title":       "Vendor Invoice",
			"entity_kind": "vendor",
			"entity_id":   jn("1"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	realVendorID := vendors[0].ID

	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "vendor", docs[0].EntityKind)
	assert.Equal(t, realVendorID, docs[0].EntityID)
}

func TestShadowDB_CommitQuoteWithExistingVendorByID(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create a vendor and project.
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Pre-existing Vendor"}))
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	realVendorID := vendors[0].ID

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Test Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	projectID := projects[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// No vendor create -- the LLM references an existing vendor by real ID.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   realVendorID,
			"project_id":  projectID,
			"total_cents": jn("50000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, realVendorID, quotes[0].VendorID)
}

func TestShadowDB_CommitQuoteWithVendorName(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Test Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	projectID := projects[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// No vendor create, no vendor_id -- just vendor_name.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_name": "New Plumber",
			"project_id":  projectID,
			"total_cents": jn("75000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	// The vendor should have been created by findOrCreateVendor.
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	assert.Equal(t, "New Plumber", vendors[0].Name)

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, vendors[0].ID, quotes[0].VendorID)
}

func TestShadowDB_CommitMixedCreatesAndUpdates(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create a document to update.
	doc := &data.Document{Title: "Original", Notes: "orig"}
	require.NoError(t, store.CreateDocument(doc))

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "Mixed Batch Vendor",
		}},
		{Action: ActionUpdate, Table: data.TableDocuments, Data: map[string]any{
			"id":    doc.ID,
			"title": "Updated via mixed batch",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	// Vendor was created.
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	assert.Equal(t, "Mixed Batch Vendor", vendors[0].Name)

	// Document was updated.
	updated, err := store.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated via mixed batch", updated.Title)
}

func TestShadowDB_ReadShadowRow(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name":  "Test Vendor",
			"phone": "555-0000",
		}},
	}
	require.NoError(t, sdb.Stage(ops))

	ids := sdb.CreatedIDs(data.TableVendors)
	require.Len(t, ids, 1)
	row, err := sdb.readShadowRow(data.TableVendors, ids[0])
	require.NoError(t, err)
	assert.NotNil(t, row)
}

func TestShadowDB_OrdinalOffset(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Pre-create 3 vendors so ordinal starts at 4.
	for _, name := range []string{"Vendor One", "Vendor Two", "Vendor Three"} {
		require.NoError(t, store.CreateVendor(&data.Vendor{Name: name}))
	}
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 3)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// Stage a new vendor -- should get ordinal "4" (3 existing + 1).
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "Vendor Four",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	ids := sdb.CreatedIDs(data.TableVendors)
	require.Len(t, ids, 1)
	assert.Equal(t, "4", ids[0], "shadow ordinal should start after existing row count")
}

func TestShadowDB_OffsetCrossReference(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Pre-create vendors so shadow ordinals are offset.
	for _, name := range []string{"Existing A", "Existing B", "Existing C"} {
		require.NoError(t, store.CreateVendor(&data.Vendor{Name: name}))
	}
	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Test Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	projectID := projects[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// The LLM creates a vendor (ordinal "4") and references it by ordinal.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "New Plumber",
		}},
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   "4",
			"project_id":  projectID,
			"total_cents": jn("99000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	// Verify the quote links to the newly-created vendor.
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 4)
	var newVendor data.Vendor
	for _, v := range vendors {
		if v.Name == "New Plumber" {
			newVendor = v
			break
		}
	}
	require.NotEmpty(t, newVendor.ID)

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, newVendor.ID, quotes[0].VendorID)
}

func TestShadowDB_OffsetExistingVendorNotRemapped(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Pre-create 2 vendors.
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Existing Plumber"}))
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Another Vendor"}))
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 2)
	existingVendorID := vendors[0].ID

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Test Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	projectID := projects[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// The LLM references an existing vendor by real ULID -- no remap.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   existingVendorID,
			"project_id":  projectID,
			"total_cents": jn("50000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	// Real vendor ID should pass through unchanged.
	assert.Equal(t, existingVendorID, quotes[0].VendorID)
}

func TestShadowDB_CommitDuplicateApplianceUsesExisting(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateAppliance(&data.Appliance{
		Name:  "HVAC Unit",
		Brand: "Carrier",
	}))

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableAppliances, Data: map[string]any{
			"name":  "HVAC Unit",
			"brand": "Trane",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	appliances, err := store.ListAppliances(false)
	require.NoError(t, err)
	assert.Len(t, appliances, 1)
	// Original brand preserved (existing entity returned, not overwritten).
	assert.Equal(t, "Carrier", appliances[0].Brand)
}

func TestShadowDB_CommitDuplicateMaintenanceUsesExisting(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)
	catID := cats[0].ID

	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:           "Replace Filter",
		CategoryID:     catID,
		IntervalMonths: 3,
	}))

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableMaintenanceItems, Data: map[string]any{
			"name":            "Replace Filter",
			"category_id":     catID,
			"interval_months": jn("6"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	// Original interval preserved.
	assert.Equal(t, 3, items[0].IntervalMonths)
}

func TestShadowDB_CommitRollsBackOnFailure(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// Stage a valid vendor followed by a quote with an empty vendor name
	// (the vendor_name fallback will produce a vendor with no name, which
	// FindOrCreateVendor rejects). This forces the second operation to fail.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "Should Be Rolled Back",
		}},
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"total_cents": jn("50000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	err = sdb.Commit(store, ops)
	require.Error(t, err)

	// The vendor should NOT exist because the transaction rolled back.
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	assert.Empty(t, vendors, "vendor should be rolled back on commit failure")
}

func TestShadowDB_NormalizeValueHandlesJSONNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    any
		expected any
	}{
		{jn("42"), int64(42)},
		{jn("-7"), int64(-7)},
		{jn("3.14"), "3.14"},
		{"hello", "hello"},
		{nil, nil},
		{true, true},
	}
	for _, tt := range tests {
		result := normalizeValue(tt.input)
		assert.Equal(t, tt.expected, result, "input: %v", tt.input)
	}
}

func TestRemapFK(t *testing.T) {
	t.Parallel()
	idMap := map[string]map[string]string{
		data.TableVendors: {"1": "01ARZ3NDEKTSV4RRFFQ69G5FAV", "2": "01ARZ3NDEKTSV4RRFFQ69G5FAW"},
	}
	fk := shadowFKRemap{Column: data.ColVendorID, Table: data.TableVendors}

	// Remap shadow ordinal "1" -> real ULID.
	row := map[string]any{"vendor_id": "1"}
	remapFK(row, fk, idMap)
	assert.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV", row["vendor_id"])

	// No mapping -> value unchanged.
	row = map[string]any{"vendor_id": "99"}
	remapFK(row, fk, idMap)
	assert.Equal(t, "99", row["vendor_id"])

	// Nil value -> no panic.
	row = map[string]any{"vendor_id": nil}
	remapFK(row, fk, idMap)
	assert.Nil(t, row["vendor_id"])
}

// --- fkGraph tests ---

func TestFKGraph_AllCreatablesPresent(t *testing.T) {
	t.Parallel()
	g := creatableFKs

	expected := map[string]bool{
		data.TableVendors:           true,
		data.TableAppliances:        true,
		data.TableProjects:          true,
		data.TableQuotes:            true,
		data.TableMaintenanceItems:  true,
		data.TableIncidents:         true,
		data.TableServiceLogEntries: true,
		data.TableDocuments:         true,
	}

	actual := make(map[string]bool, len(g.order))
	for _, table := range g.order {
		actual[table] = true
	}
	assert.Equal(t, expected, actual)
}

func TestFKGraph_OrderRespectsRelationships(t *testing.T) {
	t.Parallel()
	g := creatableFKs

	pos := make(map[string]int, len(g.order))
	for i, table := range g.order {
		pos[table] = i
	}

	for table, remaps := range g.remaps {
		for _, fk := range remaps {
			assert.Less(t, pos[fk.Table], pos[table],
				"%s (pos %d) must come before %s (pos %d) due to FK %s",
				fk.Table, pos[fk.Table], table, pos[table], fk.Column)
		}
	}
}

func TestFKGraph_KnownRemaps(t *testing.T) {
	t.Parallel()
	g := creatableFKs

	quoteRemaps := g.remaps[data.TableQuotes]
	var quoteFKs []string
	for _, r := range quoteRemaps {
		quoteFKs = append(quoteFKs, r.Column)
	}
	assert.Contains(t, quoteFKs, data.ColVendorID)

	maintRemaps := g.remaps[data.TableMaintenanceItems]
	var maintFKs []string
	for _, r := range maintRemaps {
		maintFKs = append(maintFKs, r.Column)
	}
	assert.Contains(t, maintFKs, data.ColApplianceID)
}

func TestFKGraph_DocumentsLast(t *testing.T) {
	t.Parallel()
	g := creatableFKs
	assert.Equal(t, data.TableDocuments, g.order[len(g.order)-1])
}

func TestFKGraph_EntityKindToTableMatchesDataPackage(t *testing.T) {
	t.Parallel()
	assert.Equal(t, data.EntityKindToTable, creatableFKs.entityKindToTable)
}

func TestBuildFKGraph_IgnoresNonPolymorphicHasMany(t *testing.T) {
	t.Parallel()

	type child struct {
		ID       string `gorm:"primaryKey;size:26"`
		ParentID string
	}
	type parent struct {
		ID       string  `gorm:"primaryKey;size:26"`
		Children []child // non-polymorphic HasMany
	}

	allowed := map[string]AllowedOps{
		"parents":  {Insert: true},
		"children": {Insert: true},
	}
	g, err := buildFKGraph([]any{&parent{}, &child{}}, allowed)
	require.NoError(t, err)
	assert.Empty(t, g.entityKindToTable)
}

func TestBuildFKGraph_IgnoresPolymorphicToNonDocuments(t *testing.T) {
	t.Parallel()

	type comment struct {
		ID         string `gorm:"primaryKey;size:26"`
		EntityKind string
		EntityID   string
	}
	type owner struct {
		ID       string    `gorm:"primaryKey;size:26"`
		Comments []comment `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:own"`
	}

	allowed := map[string]AllowedOps{
		"owners":   {Insert: true},
		"comments": {Insert: true},
	}
	g, err := buildFKGraph([]any{&owner{}, &comment{}}, allowed)
	require.NoError(t, err)
	assert.Empty(t, g.entityKindToTable)
}

func TestShadowDB_CommitReversedOrder_QuoteBeforeVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	projectID := projects[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// Ops deliberately in REVERSED order: quote first, then vendor.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   "1",
			"project_id":  projectID,
			"total_cents": jn("150000"),
		}},
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "Garcia Plumbing",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, vendors[0].ID, quotes[0].VendorID)
	assert.Equal(t, projectID, quotes[0].ProjectID)
}

func TestShadowDB_CommitReversedOrder_MaintenanceBeforeAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)
	catID := cats[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// Reversed: maintenance_item first, then appliance.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableMaintenanceItems, Data: map[string]any{
			"name":            "Replace HVAC Filter",
			"appliance_id":    "1",
			"category_id":     catID,
			"interval_months": jn("3"),
		}},
		{Action: ActionCreate, Table: data.TableAppliances, Data: map[string]any{
			"name":     "HVAC Unit",
			"brand":    "Carrier",
			"location": "Basement",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	appliances, err := store.ListAppliances(false)
	require.NoError(t, err)
	require.Len(t, appliances, 1)

	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].ApplianceID)
	assert.Equal(t, appliances[0].ID, *items[0].ApplianceID)
}

func TestShadowDB_CommitReversedOrder_DocumentBeforeVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// Reversed: document first, then vendor.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableDocuments, Data: map[string]any{
			"title":       "Vendor Invoice",
			"entity_kind": "vendor",
			"entity_id":   "1",
		}},
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "DocVendor",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)

	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "vendor", docs[0].EntityKind)
	assert.Equal(t, vendors[0].ID, docs[0].EntityID)
}

func TestShadowDB_CommitReversedOrder_FullChain(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Full Chain Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	projectID := projects[0].ID

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	catID := cats[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// Everything reversed: documents, quotes, maintenance, appliances, vendors.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableDocuments, Data: map[string]any{
			"title":       "Quote Doc",
			"entity_kind": "quote",
			"entity_id":   "1",
		}},
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   "1",
			"project_id":  projectID,
			"total_cents": jn("250000"),
		}},
		{Action: ActionCreate, Table: data.TableMaintenanceItems, Data: map[string]any{
			"name":            "Filter Change",
			"appliance_id":    "1",
			"category_id":     catID,
			"interval_months": jn("6"),
		}},
		{Action: ActionCreate, Table: data.TableAppliances, Data: map[string]any{
			"name":  "Furnace",
			"brand": "Lennox",
		}},
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "Full Chain Vendor",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, vendors[0].ID, quotes[0].VendorID)

	appliances, err := store.ListAppliances(false)
	require.NoError(t, err)
	require.Len(t, appliances, 1)

	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].ApplianceID)
	assert.Equal(t, appliances[0].ID, *items[0].ApplianceID)

	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "quote", docs[0].EntityKind)
}

func TestShadowDB_CommitQuoteWithoutProjectID_Fails(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// LLM produces a vendor + quote but omits project_id entirely.
	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "Sierra Structures",
		}},
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   jn("1"),
			"vendor_name": "Sierra Structures",
			"total_cents": jn("485400"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	err = sdb.Commit(store, ops)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project_id")

	// Transaction should have rolled back -- no vendor either.
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	assert.Empty(t, vendors)
}

func TestRemapDocumentEntity(t *testing.T) {
	t.Parallel()
	idMap := map[string]map[string]string{
		data.TableVendors: {"1": "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
	}

	// String entity_kind.
	row := map[string]any{
		"entity_kind": "vendor",
		"entity_id":   "1",
	}
	remapDocumentEntity(row, idMap)
	assert.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV", row["entity_id"])

	// []byte entity_kind (GORM SQLite behavior).
	row = map[string]any{
		"entity_kind": []byte("vendor"),
		"entity_id":   "1",
	}
	remapDocumentEntity(row, idMap)
	assert.Equal(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV", row["entity_id"])

	// entity_kind with no mapping in idMap -> no remap.
	row = map[string]any{
		"entity_kind": "project",
		"entity_id":   "5",
	}
	remapDocumentEntity(row, idMap)
	assert.Equal(t, "5", row["entity_id"])
}

func TestShadowDB_CommitProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, types)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableProjects, Data: map[string]any{
			"title":           "Fence Installation",
			"project_type_id": types[0].ID,
			"status":          data.ProjectStatusPlanned,
			"description":     "Install a cedar fence",
			"budget_cents":    jn("500000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "Fence Installation", projects[0].Title)
	assert.Equal(t, data.ProjectStatusPlanned, projects[0].Status)
	assert.Equal(t, "Install a cedar fence", projects[0].Description)
	require.NotNil(t, projects[0].BudgetCents)
	assert.Equal(t, int64(500000), *projects[0].BudgetCents)
}

func TestShadowDB_CommitProjectDefaultStatus(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, err := store.ProjectTypes()
	require.NoError(t, err)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableProjects, Data: map[string]any{
			"title":           "Roof Repair",
			"project_type_id": types[0].ID,
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, data.ProjectStatusIdeating, projects[0].Status)
}

func TestShadowDB_CommitProjectThenQuote_CrossReference(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, types)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableVendors, Data: map[string]any{
			"name": "Cedar Fencing Co",
		}},
		{Action: ActionCreate, Table: data.TableProjects, Data: map[string]any{
			"title":           "Fence Installation",
			"project_type_id": types[0].ID,
		}},
		{Action: ActionCreate, Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   "1",
			"project_id":  "1",
			"total_cents": jn("350000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, projects[0].ID, quotes[0].ProjectID)
	assert.Equal(t, int64(350000), quotes[0].TotalCents)
}

func TestShadowDB_CommitIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableIncidents, Data: map[string]any{
			"title":        "Pipe burst in basement",
			"description":  "Water damage to finished basement",
			"status":       data.IncidentStatusOpen,
			"severity":     data.IncidentSeverityUrgent,
			"location":     "Basement",
			"cost_cents":   jn("250000"),
			"date_noticed": "2026-01-15",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	incidents, err := store.ListIncidents(false)
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	assert.Equal(t, "Pipe burst in basement", incidents[0].Title)
	assert.Equal(t, data.IncidentStatusOpen, incidents[0].Status)
	assert.Equal(t, data.IncidentSeverityUrgent, incidents[0].Severity)
	assert.Equal(t, "Basement", incidents[0].Location)
	require.NotNil(t, incidents[0].CostCents)
	assert.Equal(t, int64(250000), *incidents[0].CostCents)
	assert.Equal(t, "2026-01-15", incidents[0].DateNoticed.Format(data.DateLayout))
}

func TestShadowDB_CommitIncidentWithVendorName(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableIncidents, Data: map[string]any{
			"title":       "AC failure",
			"vendor_name": "Cool Air HVAC",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	incidents, err := store.ListIncidents(false)
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	require.NotNil(t, incidents[0].VendorID)

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	assert.Equal(t, "Cool Air HVAC", vendors[0].Name)
	assert.Equal(t, vendors[0].ID, *incidents[0].VendorID)
}

func TestShadowDB_CommitIncidentDefaults(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableIncidents, Data: map[string]any{
			"title": "Minor drywall crack",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	incidents, err := store.ListIncidents(false)
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	assert.Equal(t, data.IncidentStatusOpen, incidents[0].Status)
	assert.Equal(t, data.IncidentSeverityWhenever, incidents[0].Severity)
	assert.False(t, incidents[0].DateNoticed.IsZero())
}

func TestShadowDB_CommitServiceLog(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)
	require.NoError(t, store.CreateMaintenance(&data.MaintenanceItem{
		Name:       "HVAC Filter",
		CategoryID: cats[0].ID,
	}))
	items, err := store.ListMaintenance(false)
	require.NoError(t, err)
	require.NotEmpty(t, items)
	itemID := items[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: data.TableServiceLogEntries, Data: map[string]any{
			"maintenance_item_id": itemID,
			"serviced_at":         "2026-02-20",
			"cost_cents":          jn("15000"),
			"notes":               "Replaced filter",
			"vendor_name":         "HVAC Pro",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	logs, err := store.ListServiceLog(itemID, false)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "2026-02-20", logs[0].ServicedAt.Format(data.DateLayout))
	assert.Equal(t, "Replaced filter", logs[0].Notes)
	require.NotNil(t, logs[0].CostCents)
	assert.Equal(t, int64(15000), *logs[0].CostCents)
	require.NotNil(t, logs[0].VendorID)

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	assert.Equal(t, "HVAC Pro", vendors[0].Name)
}

func TestShadowDB_CommitUpdateVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&data.Vendor{
		Name:  "Old Plumbing Co",
		Phone: "555-0000",
	}))
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	vendorID := vendors[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionUpdate, Table: data.TableVendors, Data: map[string]any{
			"id":    vendorID,
			"phone": "555-9999",
			"email": "new@plumbing.com",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	updated, err := store.GetVendor(vendorID)
	require.NoError(t, err)
	assert.Equal(t, "Old Plumbing Co", updated.Name)
	assert.Equal(t, "555-9999", updated.Phone)
	assert.Equal(t, "new@plumbing.com", updated.Email)
}

func TestShadowDB_CommitUpdateAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateAppliance(&data.Appliance{
		Name:  "Dishwasher",
		Brand: "Bosch",
	}))
	appliances, err := store.ListAppliances(false)
	require.NoError(t, err)
	require.Len(t, appliances, 1)
	applianceID := appliances[0].ID

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionUpdate, Table: data.TableAppliances, Data: map[string]any{
			"id":            applianceID,
			"serial_number": "BSH-12345",
			"model_number":  "SHP878ZD5N",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	updated, err := store.GetAppliance(applianceID)
	require.NoError(t, err)
	assert.Equal(t, "Dishwasher", updated.Name)
	assert.Equal(t, "Bosch", updated.Brand)
	assert.Equal(t, "BSH-12345", updated.SerialNumber)
	assert.Equal(t, "SHP878ZD5N", updated.ModelNumber)
}

func TestShadowDB_CommitUpdateQuote(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, types)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	projectID := projects[0].ID

	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Counter Tops Inc"}))
	vendor := data.Vendor{Name: "Counter Tops Inc"}
	q := &data.Quote{
		ProjectID:  projectID,
		TotalCents: 100000,
	}
	require.NoError(t, store.CreateQuote(q, vendor))

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionUpdate, Table: data.TableQuotes, Data: map[string]any{
			"id":          q.ID,
			"total_cents": jn("125000"),
			"notes":       "Revised estimate after site visit",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	updated, err := store.GetQuote(q.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(125000), updated.TotalCents)
	assert.Equal(t, "Revised estimate after site visit", updated.Notes)
	assert.Equal(t, projectID, updated.ProjectID)
}
