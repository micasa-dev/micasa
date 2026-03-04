// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/cpcloud/micasa/internal/data"
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
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{
			"name":  "Garcia Plumbing",
			"phone": "555-1234",
		}},
	}
	require.NoError(t, sdb.Stage(ops))

	ids := sdb.CreatedIDs("vendors")
	require.Len(t, ids, 1)
	assert.Equal(t, uint(1), ids[0])
}

func TestShadowDB_StageMultipleVendors(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{"name": "Vendor A"}},
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{"name": "Vendor B"}},
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{"name": "Vendor C"}},
	}
	require.NoError(t, sdb.Stage(ops))

	ids := sdb.CreatedIDs("vendors")
	require.Len(t, ids, 3)
	assert.Equal(t, uint(1), ids[0])
	assert.Equal(t, uint(2), ids[1])
	assert.Equal(t, uint(3), ids[2])
}

func TestShadowDB_StageSkipsUpdates(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionUpdate, Table: "documents", Data: map[string]any{
			"id":    jn("42"),
			"title": "Updated Title",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	assert.Empty(t, sdb.CreatedIDs("documents"))
}

func TestShadowDB_StageRejectsEmptyData(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	ops := []Operation{
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{}},
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
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{
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

	// The LLM creates a vendor then references it by fictional ID 1 in the quote.
	ops := []Operation{
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{
			"name": "Garcia Plumbing",
		}},
		{Action: ActionCreate, Table: "quotes", Data: map[string]any{
			"vendor_id":   jn("1"),
			"project_id":  json.Number(fmt.Sprintf("%d", projectID)),
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
		{Action: ActionCreate, Table: "appliances", Data: map[string]any{
			"name":     "HVAC Unit",
			"brand":    "Carrier",
			"location": "Basement",
		}},
		{Action: ActionCreate, Table: "maintenance_items", Data: map[string]any{
			"name":            "Replace HVAC Filter",
			"appliance_id":    jn("1"),
			"category_id":     json.Number(fmt.Sprintf("%d", catID)),
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
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{"name": "Plumber A"}},
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{"name": "Plumber B"}},
		{Action: ActionCreate, Table: "quotes", Data: map[string]any{
			"vendor_id":   jn("1"),
			"project_id":  json.Number(fmt.Sprintf("%d", projectID)),
			"total_cents": jn("100000"),
		}},
		{Action: ActionCreate, Table: "quotes", Data: map[string]any{
			"vendor_id":   jn("2"),
			"project_id":  json.Number(fmt.Sprintf("%d", projectID)),
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
	vendorIDByName := map[string]uint{}
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
		{Action: ActionUpdate, Table: "documents", Data: map[string]any{
			"id":    json.Number(fmt.Sprintf("%d", doc.ID)),
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
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{
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
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{
			"name": "DocVendor",
		}},
		{Action: ActionCreate, Table: "documents", Data: map[string]any{
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
		{Action: ActionCreate, Table: "quotes", Data: map[string]any{
			"vendor_id":   json.Number(fmt.Sprintf("%d", realVendorID)),
			"project_id":  json.Number(fmt.Sprintf("%d", projectID)),
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
		{Action: ActionCreate, Table: "quotes", Data: map[string]any{
			"vendor_name": "New Plumber",
			"project_id":  json.Number(fmt.Sprintf("%d", projectID)),
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
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{
			"name": "Mixed Batch Vendor",
		}},
		{Action: ActionUpdate, Table: "documents", Data: map[string]any{
			"id":    json.Number(fmt.Sprintf("%d", doc.ID)),
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
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{
			"name":  "Test Vendor",
			"phone": "555-0000",
		}},
	}
	require.NoError(t, sdb.Stage(ops))

	ids := sdb.CreatedIDs("vendors")
	require.Len(t, ids, 1)
	row, err := sdb.readShadowRow("vendors", ids[0])
	require.NoError(t, err)
	assert.NotNil(t, row)
}

func TestShadowDB_AutoIncrementOffset(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Pre-create 3 vendors so max vendor ID = 3.
	for _, name := range []string{"Vendor One", "Vendor Two", "Vendor Three"} {
		require.NoError(t, store.CreateVendor(&data.Vendor{Name: name}))
	}
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 3)

	sdb, err := NewShadowDB(store)
	require.NoError(t, err)

	// Stage a new vendor -- should get shadow ID 4 (max real + 1).
	ops := []Operation{
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{
			"name": "Vendor Four",
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	ids := sdb.CreatedIDs("vendors")
	require.Len(t, ids, 1)
	assert.Equal(t, uint(4), ids[0], "shadow auto-increment should start after max real ID")
}

func TestShadowDB_OffsetCrossReference(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Pre-create vendors so shadow IDs are offset.
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

	// The LLM creates a vendor (shadow ID 4) and references it by ID.
	ops := []Operation{
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{
			"name": "New Plumber",
		}},
		{Action: ActionCreate, Table: "quotes", Data: map[string]any{
			"vendor_id":   jn("4"),
			"project_id":  json.Number(fmt.Sprintf("%d", projectID)),
			"total_cents": jn("99000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	// Verify the quote links to the newly-created vendor, not existing ID 4.
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
	require.NotZero(t, newVendor.ID)

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, newVendor.ID, quotes[0].VendorID)
}

func TestShadowDB_OffsetExistingVendorNotRemapped(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Pre-create 2 vendors (IDs 1 and 2).
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Existing Plumber"}))
	require.NoError(t, store.CreateVendor(&data.Vendor{Name: "Another Vendor"}))
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

	// The LLM references existing vendor ID 1 (below shadow range) -- no remap.
	ops := []Operation{
		{Action: ActionCreate, Table: "quotes", Data: map[string]any{
			"vendor_id":   jn("1"),
			"project_id":  json.Number(fmt.Sprintf("%d", projectID)),
			"total_cents": jn("50000"),
		}},
	}
	require.NoError(t, sdb.Stage(ops))
	require.NoError(t, sdb.Commit(store, ops))

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	// vendor_id=1 is a real ID and should pass through unchanged.
	assert.Equal(t, uint(1), quotes[0].VendorID)
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
		{Action: ActionCreate, Table: "appliances", Data: map[string]any{
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
		{Action: ActionCreate, Table: "maintenance_items", Data: map[string]any{
			"name":            "Replace Filter",
			"category_id":     json.Number(fmt.Sprintf("%d", catID)),
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
		{Action: ActionCreate, Table: "vendors", Data: map[string]any{
			"name": "Should Be Rolled Back",
		}},
		{Action: ActionCreate, Table: "quotes", Data: map[string]any{
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
	idMap := map[string]map[uint]uint{
		"vendors": {1: 42, 2: 43},
	}
	fk := shadowFKRemap{Column: "vendor_id", Table: "vendors"}

	// Remap shadow ID 1 -> real ID 42.
	row := map[string]any{"vendor_id": int64(1)}
	remapFK(row, fk, idMap)
	assert.Equal(t, uint(42), row["vendor_id"])

	// No mapping -> value unchanged.
	row = map[string]any{"vendor_id": int64(99)}
	remapFK(row, fk, idMap)
	assert.Equal(t, int64(99), row["vendor_id"])

	// Nil value -> no panic.
	row = map[string]any{"vendor_id": nil}
	remapFK(row, fk, idMap)
	assert.Nil(t, row["vendor_id"])
}

func TestRemapDocumentEntity(t *testing.T) {
	t.Parallel()
	idMap := map[string]map[uint]uint{
		"vendors": {1: 42},
	}

	// String entity_kind.
	row := map[string]any{
		"entity_kind": "vendor",
		"entity_id":   int64(1),
	}
	remapDocumentEntity(row, idMap)
	assert.Equal(t, uint(42), row["entity_id"])

	// []byte entity_kind (GORM SQLite behavior).
	row = map[string]any{
		"entity_kind": []byte("vendor"),
		"entity_id":   int64(1),
	}
	remapDocumentEntity(row, idMap)
	assert.Equal(t, uint(42), row["entity_id"])

	// Non-creatable entity_kind -> no remap.
	row = map[string]any{
		"entity_kind": "project",
		"entity_id":   int64(5),
	}
	remapDocumentEntity(row, idMap)
	assert.Equal(t, int64(5), row["entity_id"])
}
