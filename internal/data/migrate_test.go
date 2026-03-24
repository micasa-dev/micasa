// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createOldSchemaDB creates a SQLite database with the pre-v2.3 integer-PK
// schema and populates it with sample data. Returns the path to the database.
func createOldSchemaDB(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "old.db")

	store, err := Open(path)
	require.NoError(t, err)
	db := store.GormDB()

	// Create tables with INTEGER PRIMARY KEY (the old schema).
	ddls := []string{
		`CREATE TABLE house_profiles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			nickname TEXT DEFAULT '',
			address_line1 TEXT DEFAULT '',
			address_line2 TEXT DEFAULT '',
			city TEXT DEFAULT '',
			state TEXT DEFAULT '',
			postal_code TEXT DEFAULT '',
			year_built INTEGER DEFAULT 0,
			square_feet INTEGER DEFAULT 0,
			lot_square_feet INTEGER DEFAULT 0,
			bedrooms INTEGER DEFAULT 0,
			bathrooms REAL DEFAULT 0,
			foundation_type TEXT DEFAULT '',
			wiring_type TEXT DEFAULT '',
			roof_type TEXT DEFAULT '',
			exterior_type TEXT DEFAULT '',
			heating_type TEXT DEFAULT '',
			cooling_type TEXT DEFAULT '',
			water_source TEXT DEFAULT '',
			sewer_type TEXT DEFAULT '',
			parking_type TEXT DEFAULT '',
			basement_type TEXT DEFAULT '',
			insurance_carrier TEXT DEFAULT '',
			insurance_policy TEXT DEFAULT '',
			insurance_renewal DATETIME,
			property_tax_cents INTEGER,
			hoa_name TEXT DEFAULT '',
			hoa_fee_cents INTEGER,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE project_types (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE maintenance_categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE vendors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE,
			contact_name TEXT DEFAULT '',
			email TEXT DEFAULT '',
			phone TEXT DEFAULT '',
			website TEXT DEFAULT '',
			notes TEXT DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE appliances (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT DEFAULT '',
			brand TEXT DEFAULT '',
			model_number TEXT DEFAULT '',
			serial_number TEXT DEFAULT '',
			purchase_date DATETIME,
			warranty_expiry DATETIME,
			location TEXT DEFAULT '',
			cost_cents INTEGER,
			notes TEXT DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT DEFAULT '',
			project_type_id INTEGER,
			status TEXT DEFAULT 'planned',
			description TEXT DEFAULT '',
			start_date DATETIME,
			end_date DATETIME,
			budget_cents INTEGER,
			actual_cents INTEGER,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			CONSTRAINT fk_project_type FOREIGN KEY (project_type_id) REFERENCES project_types(id) ON DELETE RESTRICT
		)`,
		`CREATE TABLE maintenance_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT DEFAULT '',
			category_id INTEGER,
			appliance_id INTEGER,
			season TEXT DEFAULT '',
			last_serviced_at DATETIME,
			interval_months INTEGER DEFAULT 0,
			due_date DATETIME,
			manual_url TEXT DEFAULT '',
			manual_text TEXT DEFAULT '',
			notes TEXT DEFAULT '',
			cost_cents INTEGER,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			CONSTRAINT fk_category FOREIGN KEY (category_id) REFERENCES maintenance_categories(id) ON DELETE RESTRICT,
			CONSTRAINT fk_appliance FOREIGN KEY (appliance_id) REFERENCES appliances(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE quotes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER,
			vendor_id INTEGER,
			total_cents INTEGER DEFAULT 0,
			labor_cents INTEGER,
			materials_cents INTEGER,
			other_cents INTEGER,
			received_date DATETIME,
			notes TEXT DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			CONSTRAINT fk_project FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT,
			CONSTRAINT fk_vendor FOREIGN KEY (vendor_id) REFERENCES vendors(id) ON DELETE RESTRICT
		)`,
		`CREATE TABLE incidents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT DEFAULT '',
			description TEXT DEFAULT '',
			status TEXT DEFAULT 'open',
			previous_status TEXT DEFAULT '',
			severity TEXT DEFAULT 'soon',
			date_noticed DATETIME DEFAULT CURRENT_TIMESTAMP,
			date_resolved DATETIME,
			location TEXT DEFAULT '',
			cost_cents INTEGER,
			appliance_id INTEGER,
			vendor_id INTEGER,
			notes TEXT DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			CONSTRAINT fk_appliance FOREIGN KEY (appliance_id) REFERENCES appliances(id) ON DELETE SET NULL,
			CONSTRAINT fk_vendor FOREIGN KEY (vendor_id) REFERENCES vendors(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE service_log_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			maintenance_item_id INTEGER,
			serviced_at DATETIME,
			vendor_id INTEGER,
			cost_cents INTEGER,
			notes TEXT DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			CONSTRAINT fk_maintenance FOREIGN KEY (maintenance_item_id) REFERENCES maintenance_items(id) ON DELETE CASCADE,
			CONSTRAINT fk_vendor FOREIGN KEY (vendor_id) REFERENCES vendors(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE documents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT DEFAULT '',
			file_name TEXT DEFAULT '',
			entity_kind TEXT DEFAULT '',
			entity_id INTEGER DEFAULT 0,
			mime_type TEXT DEFAULT '',
			size_bytes INTEGER DEFAULT 0,
			sha256 TEXT DEFAULT '',
			data BLOB,
			extracted_text TEXT DEFAULT '',
			ocr_data BLOB,
			extraction_model TEXT DEFAULT '',
			extraction_ops BLOB,
			notes TEXT DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE deletion_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity TEXT DEFAULT '',
			target_id INTEGER DEFAULT 0,
			deleted_at DATETIME,
			restored_at DATETIME
		)`,
		`CREATE TABLE settings (
			key TEXT PRIMARY KEY,
			value TEXT DEFAULT '',
			updated_at DATETIME
		)`,
		`CREATE TABLE chat_inputs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			input TEXT NOT NULL,
			created_at DATETIME
		)`,
	}
	for _, ddl := range ddls {
		require.NoError(t, db.Exec(ddl).Error, "DDL: %s", ddl)
	}

	now := time.Now().UTC().Truncate(time.Second)

	// Seed data with integer IDs.
	require.NoError(t, db.Exec(
		"INSERT INTO house_profiles (id, nickname, city, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		1,
		"My House",
		"Portland",
		now,
		now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO project_types (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)",
		1, "renovation", now, now,
	).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO project_types (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)",
		2, "repair", now, now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO maintenance_categories (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)",
		1, "HVAC", now, now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO vendors (id, name, contact_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		1,
		"Ace Plumbing",
		"John",
		now,
		now,
	).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO vendors (id, name, contact_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		2,
		"Bob Electric",
		"Bob",
		now,
		now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO appliances (id, name, brand, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		1, "Furnace", "Carrier", now, now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO projects (id, title, project_type_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		1,
		"Kitchen Reno",
		1,
		"planned",
		now,
		now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO maintenance_items (id, name, category_id, appliance_id, season, interval_months, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		1,
		"Filter Change",
		1,
		1,
		"fall",
		3,
		now,
		now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO quotes (id, project_id, vendor_id, total_cents, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		1,
		1,
		1,
		500000,
		now,
		now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO incidents (id, title, status, severity, appliance_id, vendor_id, date_noticed, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		1,
		"Leak",
		"open",
		"urgent",
		1,
		2,
		now,
		now,
		now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO service_log_entries (id, maintenance_item_id, serviced_at, vendor_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		1,
		1,
		now,
		1,
		now,
		now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO documents (id, title, file_name, entity_kind, entity_id, mime_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		1,
		"Quote PDF",
		"quote.pdf",
		"project",
		1,
		"application/pdf",
		now,
		now,
	).Error)

	require.NoError(t, db.Exec(
		"INSERT INTO deletion_records (id, entity, target_id, deleted_at) VALUES (?, ?, ?, ?)",
		1, "vendor", 2, now,
	).Error)

	// Mark vendor 2 as soft-deleted to match the deletion record.
	require.NoError(t, db.Exec(
		"UPDATE vendors SET deleted_at = ? WHERE id = 2", now,
	).Error)

	require.NoError(t, store.Close())
	return path
}

func TestMigrateIntToStringIDs(t *testing.T) {
	t.Parallel()

	path := createOldSchemaDB(t)

	// Open the old DB and run AutoMigrate (which calls migrateIntToStringIDs).
	store, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.AutoMigrate())

	// Verify all IDs are now valid ULIDs.
	t.Run("house_profile_id_is_ulid", func(t *testing.T) {
		house, err := store.HouseProfile()
		require.NoError(t, err)
		assert.True(t, uid.IsValid(house.ID), "house ID %q is not a valid ULID", house.ID)
		assert.Equal(t, "My House", house.Nickname)
		assert.Equal(t, "Portland", house.City)
	})

	t.Run("project_types_migrated", func(t *testing.T) {
		types, err := store.ProjectTypes()
		require.NoError(t, err)
		require.Len(t, types, 2)
		for _, pt := range types {
			assert.True(t, uid.IsValid(pt.ID), "project type ID %q is not a valid ULID", pt.ID)
		}
	})

	t.Run("vendors_migrated", func(t *testing.T) {
		vendors, err := store.ListVendors(true)
		require.NoError(t, err)
		require.Len(t, vendors, 2)
		for _, v := range vendors {
			assert.True(t, uid.IsValid(v.ID), "vendor ID %q is not a valid ULID", v.ID)
		}
	})

	t.Run("project_fk_preserved", func(t *testing.T) {
		projects, err := store.ListProjects(false)
		require.NoError(t, err)
		require.Len(t, projects, 1)
		p := projects[0]
		assert.True(t, uid.IsValid(p.ID), "project ID is not a valid ULID")
		assert.True(t, uid.IsValid(p.ProjectTypeID), "project_type_id is not a valid ULID")
		assert.Equal(t, "Kitchen Reno", p.Title)
		// FK should point to a real project type.
		assert.Equal(t, "renovation", p.ProjectType.Name)
	})

	t.Run("quote_fk_preserved", func(t *testing.T) {
		quotes, err := store.ListQuotes(false)
		require.NoError(t, err)
		require.Len(t, quotes, 1)
		q := quotes[0]
		assert.True(t, uid.IsValid(q.ID))
		assert.True(t, uid.IsValid(q.ProjectID))
		assert.True(t, uid.IsValid(q.VendorID))
		assert.Equal(t, int64(500000), q.TotalCents)
		assert.Equal(t, "Kitchen Reno", q.Project.Title)
		assert.Equal(t, "Ace Plumbing", q.Vendor.Name)
	})

	t.Run("maintenance_item_fk_preserved", func(t *testing.T) {
		items, err := store.ListMaintenance(false)
		require.NoError(t, err)
		require.Len(t, items, 1)
		item := items[0]
		assert.True(t, uid.IsValid(item.ID))
		assert.True(t, uid.IsValid(item.CategoryID))
		require.NotNil(t, item.ApplianceID)
		assert.True(t, uid.IsValid(*item.ApplianceID))
		assert.Equal(t, "HVAC", item.Category.Name)
		assert.Equal(t, "Furnace", item.Appliance.Name)
	})

	t.Run("incident_fk_preserved", func(t *testing.T) {
		incidents, err := store.ListIncidents(false)
		require.NoError(t, err)
		require.Len(t, incidents, 1)
		inc := incidents[0]
		assert.True(t, uid.IsValid(inc.ID))
		require.NotNil(t, inc.ApplianceID)
		assert.True(t, uid.IsValid(*inc.ApplianceID))
		require.NotNil(t, inc.VendorID)
		assert.True(t, uid.IsValid(*inc.VendorID))
		assert.Equal(t, "Furnace", inc.Appliance.Name)
		assert.Equal(t, "Bob Electric", inc.Vendor.Name)
	})

	t.Run("service_log_fk_preserved", func(t *testing.T) {
		// Get the maintenance item to find service logs.
		items, err := store.ListMaintenance(false)
		require.NoError(t, err)
		require.Len(t, items, 1)
		logs, err := store.ListServiceLog(items[0].ID, false)
		require.NoError(t, err)
		require.Len(t, logs, 1)
		log := logs[0]
		assert.True(t, uid.IsValid(log.ID))
		assert.True(t, uid.IsValid(log.MaintenanceItemID))
		require.NotNil(t, log.VendorID)
		assert.True(t, uid.IsValid(*log.VendorID))
	})

	t.Run("document_entity_id_migrated", func(t *testing.T) {
		// The document's entity_id should now reference the project's new ULID.
		projects, err := store.ListProjects(false)
		require.NoError(t, err)
		require.Len(t, projects, 1)

		var doc Document
		require.NoError(t, store.GormDB().
			Where("entity_kind = ? AND entity_id = ?", "project", projects[0].ID).
			First(&doc).Error)
		assert.True(t, uid.IsValid(doc.ID))
		assert.Equal(t, "Quote PDF", doc.Title)
	})

	t.Run("deletion_record_target_id_migrated", func(t *testing.T) {
		// The deletion record's target_id should now reference vendor 2's new ULID.
		vendors, err := store.ListVendors(true)
		require.NoError(t, err)
		var deletedVendor *Vendor
		for i := range vendors {
			if vendors[i].Name == "Bob Electric" {
				deletedVendor = &vendors[i]
				break
			}
		}
		require.NotNil(t, deletedVendor)

		var dr DeletionRecord
		require.NoError(t, store.GormDB().
			Where("entity = ? AND target_id = ?", "vendor", deletedVendor.ID).
			First(&dr).Error)
		assert.True(t, uid.IsValid(dr.ID))
	})

	t.Run("new_inserts_work", func(t *testing.T) {
		// The whole point: inserting new records with ULID IDs should work.
		types, err := store.ProjectTypes()
		require.NoError(t, err)

		require.NoError(t, store.CreateProject(&Project{
			Title:         "New Project",
			ProjectTypeID: types[0].ID,
			Status:        ProjectStatusPlanned,
		}))

		projects, err := store.ListProjects(false)
		require.NoError(t, err)
		require.Len(t, projects, 2)

		var newProject *Project
		for i := range projects {
			if projects[i].Title == "New Project" {
				newProject = &projects[i]
				break
			}
		}
		require.NotNil(t, newProject)
		assert.True(t, uid.IsValid(newProject.ID))
	})
}

func TestMigrateIdempotent(t *testing.T) {
	t.Parallel()

	path := createOldSchemaDB(t)

	store, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Run AutoMigrate twice — second time should be a no-op.
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.AutoMigrate())

	// Data should still be intact.
	house, err := store.HouseProfile()
	require.NoError(t, err)
	assert.Equal(t, "My House", house.Nickname)
	assert.True(t, uid.IsValid(house.ID))
}

func TestMigrateEmptyOldDB(t *testing.T) {
	t.Parallel()

	// Create a database with old schema but no data rows.
	dir := t.TempDir()
	path := filepath.Join(dir, "empty-old.db")

	store, err := Open(path)
	require.NoError(t, err)
	db := store.GormDB()

	require.NoError(t, db.Exec(
		`CREATE TABLE project_types (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE,
			created_at DATETIME,
			updated_at DATETIME
		)`,
	).Error)
	require.NoError(t, db.Exec(
		`CREATE TABLE vendors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE,
			contact_name TEXT DEFAULT '',
			email TEXT DEFAULT '',
			phone TEXT DEFAULT '',
			website TEXT DEFAULT '',
			notes TEXT DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
	).Error)

	require.NoError(t, store.Close())

	// Reopen and migrate.
	store2, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store2.Close() })

	require.NoError(t, store2.AutoMigrate())
	require.NoError(t, store2.SeedDefaults())

	// Should be able to insert new data.
	types, err := store2.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, types)
	for _, pt := range types {
		assert.True(t, uid.IsValid(pt.ID), "seeded project type ID %q should be ULID", pt.ID)
	}
}

func TestMigrateFreshDB(t *testing.T) {
	t.Parallel()

	// A brand-new database should not trigger the migration.
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.db")

	store, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, types)
	for _, pt := range types {
		assert.True(t, uid.IsValid(pt.ID))
	}
}

func TestMigratePreservesRowCount(t *testing.T) {
	t.Parallel()

	path := createOldSchemaDB(t)

	store, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.AutoMigrate())

	tables := []struct {
		name  string
		count int
	}{
		{TableHouseProfiles, 1},
		{TableProjectTypes, 2},
		{TableMaintenanceCategories, 1},
		{TableVendors, 2},
		{TableAppliances, 1},
		{TableProjects, 1},
		{TableMaintenanceItems, 1},
		{TableQuotes, 1},
		{TableIncidents, 1},
		{TableServiceLogEntries, 1},
		{TableDocuments, 1},
		{TableDeletionRecords, 1},
	}
	for _, tc := range tables {
		t.Run(fmt.Sprintf("%s_count", tc.name), func(t *testing.T) {
			var count int64
			require.NoError(t, store.GormDB().Unscoped().Table(tc.name).Count(&count).Error)
			assert.Equal(t, int64(tc.count), count, "table %s", tc.name)
		})
	}
}
