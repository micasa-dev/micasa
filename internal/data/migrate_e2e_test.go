// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

// TestMigrateE2E_RealisticV22Database creates a database that matches what
// v2.2's GORM AutoMigrate would produce (including exact DDL formatting,
// indexes, and FK constraints), populates it with realistic user data, then
// verifies the full upgrade path: migration, all FK relationships intact,
// and new CRUD operations work.
//
// This is the end-to-end regression test for issue #794.
func TestMigrateE2E_RealisticV22Database(t *testing.T) {
	t.Parallel()

	path := createRealisticV22DB(t)

	// Open with current code — triggers migration.
	store, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.AutoMigrate())

	// --- Verify all existing data survived with valid ULIDs ---

	t.Run("house_profile", func(t *testing.T) {
		house, err := store.HouseProfile()
		require.NoError(t, err)
		assert.True(t, uid.IsValid(house.ID))
		assert.Equal(t, "Our Home", house.Nickname)
		assert.Equal(t, "Portland", house.City)
		assert.Equal(t, 1985, house.YearBuilt)
	})

	t.Run("project_types_preserved", func(t *testing.T) {
		types, err := store.ProjectTypes()
		require.NoError(t, err)
		assert.Len(t, types, 10)
		names := make(map[string]bool)
		for _, pt := range types {
			assert.True(t, uid.IsValid(pt.ID))
			names[pt.Name] = true
		}
		assert.True(t, names["renovation"])
		assert.True(t, names["plumbing"])
	})

	t.Run("maintenance_categories_preserved", func(t *testing.T) {
		cats, err := store.MaintenanceCategories()
		require.NoError(t, err)
		assert.Len(t, cats, 9)
	})

	t.Run("vendors_with_data", func(t *testing.T) {
		vendors, err := store.ListVendors(false)
		require.NoError(t, err)
		assert.Len(t, vendors, 2)
		for _, v := range vendors {
			assert.True(t, uid.IsValid(v.ID))
			assert.NotEmpty(t, v.Name)
		}
	})

	t.Run("projects_with_fk_resolution", func(t *testing.T) {
		projects, err := store.ListProjects(false)
		require.NoError(t, err)
		assert.Len(t, projects, 2)
		for _, p := range projects {
			assert.True(t, uid.IsValid(p.ID))
			assert.True(t, uid.IsValid(p.ProjectTypeID))
			assert.NotEmpty(t, p.ProjectType.Name, "FK to project_type should resolve")
		}
	})

	t.Run("quotes_with_multi_fk", func(t *testing.T) {
		quotes, err := store.ListQuotes(false)
		require.NoError(t, err)
		assert.Len(t, quotes, 1)
		q := quotes[0]
		assert.True(t, uid.IsValid(q.ID))
		assert.Equal(t, "Kitchen Renovation", q.Project.Title)
		assert.Equal(t, "ABC Plumbing", q.Vendor.Name)
		assert.Equal(t, int64(4500000), q.TotalCents)
	})

	t.Run("maintenance_items_with_nullable_fk", func(t *testing.T) {
		items, err := store.ListMaintenance(false)
		require.NoError(t, err)
		assert.Len(t, items, 2)
		for _, m := range items {
			assert.True(t, uid.IsValid(m.ID))
			assert.True(t, uid.IsValid(m.CategoryID))
			assert.NotEmpty(t, m.Category.Name)
			if m.ApplianceID != nil {
				assert.True(t, uid.IsValid(*m.ApplianceID))
				assert.NotEmpty(t, m.Appliance.Name)
			}
		}
	})

	t.Run("incidents_with_nullable_fks", func(t *testing.T) {
		incidents, err := store.ListIncidents(false)
		require.NoError(t, err)
		assert.Len(t, incidents, 1)
		inc := incidents[0]
		assert.True(t, uid.IsValid(inc.ID))
		require.NotNil(t, inc.ApplianceID)
		assert.Equal(t, "Water Heater", inc.Appliance.Name)
		require.NotNil(t, inc.VendorID)
		assert.Equal(t, "ABC Plumbing", inc.Vendor.Name)
	})

	t.Run("service_logs_with_cascade_fk", func(t *testing.T) {
		items, err := store.ListMaintenance(false)
		require.NoError(t, err)
		var hvacItem *MaintenanceItem
		for i := range items {
			if items[i].Name == "Replace HVAC Filter" {
				hvacItem = &items[i]
				break
			}
		}
		require.NotNil(t, hvacItem)
		logs, err := store.ListServiceLog(hvacItem.ID, false)
		require.NoError(t, err)
		assert.Len(t, logs, 1)
		assert.True(t, uid.IsValid(logs[0].ID))
		assert.True(t, uid.IsValid(logs[0].MaintenanceItemID))
	})

	t.Run("documents_polymorphic_fk", func(t *testing.T) {
		projects, err := store.ListProjects(false)
		require.NoError(t, err)
		var kitchenProject *Project
		for i := range projects {
			if projects[i].Title == "Kitchen Renovation" {
				kitchenProject = &projects[i]
				break
			}
		}
		require.NotNil(t, kitchenProject)

		var doc Document
		require.NoError(t, store.GormDB().
			Where("entity_kind = ? AND entity_id = ?", "project", kitchenProject.ID).
			First(&doc).Error)
		assert.True(t, uid.IsValid(doc.ID))
		assert.Equal(t, "Kitchen Quote", doc.Title)
	})

	t.Run("settings_preserved", func(t *testing.T) {
		// Settings table uses text PK ("key"), not integer — migration
		// doesn't touch it. Verify data survived.
		val, err := store.GetSetting("currency")
		require.NoError(t, err)
		assert.Equal(t, "USD", val)
	})

	// --- THE CRITICAL TEST: New CRUD operations work ---
	// This is what issue #794 reports as broken.

	t.Run("insert_new_project", func(t *testing.T) {
		types, err := store.ProjectTypes()
		require.NoError(t, err)
		require.NoError(t, store.CreateProject(&Project{
			Title:         "Bathroom Remodel",
			ProjectTypeID: types[0].ID,
			Status:        ProjectStatusPlanned,
		}))
		projects, err := store.ListProjects(false)
		require.NoError(t, err)
		assert.Len(t, projects, 3)
	})

	t.Run("insert_new_vendor", func(t *testing.T) {
		v := Vendor{Name: "New Contractor LLC", ContactName: "Bob Builder"}
		require.NoError(t, store.GormDB().Create(&v).Error)
		assert.True(t, uid.IsValid(v.ID))
	})

	t.Run("insert_new_quote_with_fk", func(t *testing.T) {
		projects, err := store.ListProjects(false)
		require.NoError(t, err)
		require.NoError(t, store.CreateQuote(
			&Quote{ProjectID: projects[0].ID, TotalCents: 3500000},
			Vendor{Name: "New Contractor LLC"},
		))
		quotes, err := store.ListQuotes(false)
		require.NoError(t, err)
		assert.Len(t, quotes, 2)
	})

	t.Run("update_house_profile", func(t *testing.T) {
		house, err := store.HouseProfile()
		require.NoError(t, err)
		house.Nickname = "The Cloud House"
		require.NoError(t, store.UpdateHouseProfile(house))
		updated, err := store.HouseProfile()
		require.NoError(t, err)
		assert.Equal(t, "The Cloud House", updated.Nickname)
	})

	t.Run("seed_defaults_after_migration", func(t *testing.T) {
		// SeedDefaults uses FirstOrCreate — the v2.3 seed names differ
		// from the v2.2 test data, so both sets coexist. What matters
		// is that SeedDefaults doesn't error on a migrated database.
		require.NoError(t, store.SeedDefaults())
		types, err := store.ProjectTypes()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(types), 10)
	})
}

// createRealisticV22DB creates a SQLite database using the exact DDL that
// GORM v2.2 AutoMigrate would produce for uint-PK models. This uses the
// raw sqlite driver (not GORM) to ensure the DDL matches exactly.
func createRealisticV22DB(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "micasa.db")

	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	// DDL exactly as GORM AutoMigrate produces for uint PKs (double-quoted
	// identifiers, inline constraints, no AUTOINCREMENT).
	ddls := []string{
		`CREATE TABLE "house_profiles" ("id" integer,"nickname" text,"address_line1" text,"address_line2" text,"city" text,"state" text,"postal_code" text,"year_built" integer,"square_feet" integer,"lot_square_feet" integer,"bedrooms" integer,"bathrooms" real,"foundation_type" text,"wiring_type" text,"roof_type" text,"exterior_type" text,"heating_type" text,"cooling_type" text,"water_source" text,"sewer_type" text,"parking_type" text,"basement_type" text,"insurance_carrier" text,"insurance_policy" text,"insurance_renewal" datetime,"property_tax_cents" integer,"hoa_name" text,"hoa_fee_cents" integer,"created_at" datetime,"updated_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "project_types" ("id" integer,"name" text UNIQUE,"created_at" datetime,"updated_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "maintenance_categories" ("id" integer,"name" text UNIQUE,"created_at" datetime,"updated_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "vendors" ("id" integer,"name" text UNIQUE,"contact_name" text,"email" text,"phone" text,"website" text,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE INDEX "idx_vendors_deleted_at" ON "vendors"("deleted_at")`,
		`CREATE TABLE "appliances" ("id" integer,"name" text,"brand" text,"model_number" text,"serial_number" text,"purchase_date" datetime,"warranty_expiry" datetime,"location" text,"cost_cents" integer,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE INDEX "idx_appliances_deleted_at" ON "appliances"("deleted_at")`,
		`CREATE INDEX "idx_appliances_warranty_expiry" ON "appliances"("warranty_expiry")`,
		`CREATE TABLE "projects" ("id" integer,"title" text,"project_type_id" integer,"status" text DEFAULT 'planned',"description" text,"start_date" datetime,"end_date" datetime,"budget_cents" integer,"actual_cents" integer,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"),CONSTRAINT "fk_projects_project_type" FOREIGN KEY ("project_type_id") REFERENCES "project_types"("id") ON DELETE RESTRICT)`,
		`CREATE INDEX "idx_projects_deleted_at" ON "projects"("deleted_at")`,
		`CREATE TABLE "quotes" ("id" integer,"project_id" integer,"vendor_id" integer,"total_cents" integer,"labor_cents" integer,"materials_cents" integer,"other_cents" integer,"received_date" datetime,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"),CONSTRAINT "fk_quotes_project" FOREIGN KEY ("project_id") REFERENCES "projects"("id") ON DELETE RESTRICT,CONSTRAINT "fk_quotes_vendor" FOREIGN KEY ("vendor_id") REFERENCES "vendors"("id") ON DELETE RESTRICT)`,
		`CREATE INDEX "idx_quotes_deleted_at" ON "quotes"("deleted_at")`,
		`CREATE INDEX "idx_quotes_project_id" ON "quotes"("project_id")`,
		`CREATE INDEX "idx_quotes_vendor_id" ON "quotes"("vendor_id")`,
		`CREATE TABLE "maintenance_items" ("id" integer,"name" text,"category_id" integer,"appliance_id" integer,"season" text,"last_serviced_at" datetime,"interval_months" integer,"due_date" datetime,"manual_url" text,"manual_text" text,"notes" text,"cost_cents" integer,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"),CONSTRAINT "fk_maintenance_items_category" FOREIGN KEY ("category_id") REFERENCES "maintenance_categories"("id") ON DELETE RESTRICT,CONSTRAINT "fk_maintenance_items_appliance" FOREIGN KEY ("appliance_id") REFERENCES "appliances"("id") ON DELETE SET NULL)`,
		`CREATE INDEX "idx_maintenance_items_deleted_at" ON "maintenance_items"("deleted_at")`,
		`CREATE INDEX "idx_maintenance_items_category_id" ON "maintenance_items"("category_id")`,
		`CREATE INDEX "idx_maintenance_items_appliance_id" ON "maintenance_items"("appliance_id")`,
		`CREATE TABLE "incidents" ("id" integer,"title" text,"description" text,"status" text DEFAULT 'open',"previous_status" text,"severity" text DEFAULT 'soon',"date_noticed" datetime DEFAULT CURRENT_TIMESTAMP,"date_resolved" datetime,"location" text,"cost_cents" integer,"appliance_id" integer,"vendor_id" integer,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"),CONSTRAINT "fk_incidents_appliance" FOREIGN KEY ("appliance_id") REFERENCES "appliances"("id") ON DELETE SET NULL,CONSTRAINT "fk_incidents_vendor" FOREIGN KEY ("vendor_id") REFERENCES "vendors"("id") ON DELETE SET NULL)`,
		`CREATE INDEX "idx_incidents_deleted_at" ON "incidents"("deleted_at")`,
		`CREATE INDEX "idx_incidents_appliance_id" ON "incidents"("appliance_id")`,
		`CREATE INDEX "idx_incidents_vendor_id" ON "incidents"("vendor_id")`,
		`CREATE TABLE "service_log_entries" ("id" integer,"maintenance_item_id" integer,"serviced_at" datetime,"vendor_id" integer,"cost_cents" integer,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"),CONSTRAINT "fk_service_log_entries_maintenance_item" FOREIGN KEY ("maintenance_item_id") REFERENCES "maintenance_items"("id") ON DELETE CASCADE,CONSTRAINT "fk_service_log_entries_vendor" FOREIGN KEY ("vendor_id") REFERENCES "vendors"("id") ON DELETE SET NULL)`,
		`CREATE INDEX "idx_service_log_entries_deleted_at" ON "service_log_entries"("deleted_at")`,
		`CREATE INDEX "idx_service_log_entries_maintenance_item_id" ON "service_log_entries"("maintenance_item_id")`,
		`CREATE INDEX "idx_service_log_entries_vendor_id" ON "service_log_entries"("vendor_id")`,
		`CREATE TABLE "documents" ("id" integer,"title" text,"file_name" text,"entity_kind" text,"entity_id" integer,"mime_type" text,"size_bytes" integer,"sha256" text,"data" blob,"extracted_text" text,"ocr_data" blob,"extraction_model" text,"extraction_ops" blob,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE INDEX "idx_documents_deleted_at" ON "documents"("deleted_at")`,
		`CREATE INDEX "idx_doc_entity" ON "documents"("entity_kind","entity_id")`,
		`CREATE TABLE "deletion_records" ("id" integer,"entity" text,"target_id" integer,"deleted_at" datetime,"restored_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE INDEX "idx_deletion_records_target_id" ON "deletion_records"("target_id")`,
		`CREATE INDEX "idx_deletion_records_deleted_at" ON "deletion_records"("deleted_at")`,
		`CREATE INDEX "idx_entity_restored" ON "deletion_records"("entity","restored_at")`,
		`CREATE TABLE "settings" ("key" text,"value" text,"updated_at" datetime,PRIMARY KEY ("key"))`,
		`CREATE TABLE "chat_inputs" ("id" integer,"input" text NOT NULL,"created_at" datetime,PRIMARY KEY ("id"))`,
	}
	for _, ddl := range ddls {
		_, err := db.ExecContext(t.Context(), ddl)
		require.NoError(t, err, "DDL: %s", ddl)
	}

	now := time.Now().UTC().Truncate(time.Second)

	// Seed project types (10, matching SeedDefaults)
	for i, name := range []string{
		"renovation", "repair", "upgrade", "addition", "landscaping",
		"emergency", "cosmetic", "structural", "electrical", "plumbing",
	} {
		_, err := db.ExecContext(t.Context(),
			`INSERT INTO project_types (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			i+1, name, now, now,
		)
		require.NoError(t, err)
	}

	// Seed maintenance categories (9, matching SeedDefaults)
	for i, name := range []string{
		"HVAC", "plumbing", "electrical", "appliance", "exterior",
		"interior", "safety", "pest control", "seasonal",
	} {
		_, err := db.ExecContext(
			t.Context(),
			`INSERT INTO maintenance_categories (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			i+1,
			name,
			now,
			now,
		)
		require.NoError(t, err)
	}

	// Realistic user data
	inserts := []struct {
		sql  string
		args []any
	}{
		{
			`INSERT INTO house_profiles (id, nickname, address_line1, city, state, postal_code, year_built, square_feet, bedrooms, bathrooms, created_at, updated_at) VALUES (1, 'Our Home', '123 Main St', 'Portland', 'OR', '97201', 1985, 2200, 3, 2.5, ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO vendors (id, name, contact_name, phone, email, created_at, updated_at) VALUES (1, 'ABC Plumbing', 'Mike Smith', '555-0100', 'mike@abc.com', ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO vendors (id, name, contact_name, phone, created_at, updated_at) VALUES (2, 'XYZ Electric', 'Jane Doe', '555-0200', ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO appliances (id, name, brand, model_number, location, created_at, updated_at) VALUES (1, 'Water Heater', 'Rheem', 'RH-4050', 'Basement', ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO appliances (id, name, brand, model_number, location, created_at, updated_at) VALUES (2, 'Furnace', 'Carrier', 'C-9000', 'Basement', ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO projects (id, title, project_type_id, status, description, budget_cents, created_at, updated_at) VALUES (1, 'Kitchen Renovation', 1, 'planned', 'Complete kitchen remodel', 5000000, ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO projects (id, title, project_type_id, status, description, budget_cents, created_at, updated_at) VALUES (2, 'Fix Leak', 10, 'completed', 'Fix bathroom pipe leak', 50000, ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO quotes (id, project_id, vendor_id, total_cents, labor_cents, materials_cents, notes, created_at, updated_at) VALUES (1, 1, 1, 4500000, 2000000, 2500000, 'Includes cabinets and countertops', ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO maintenance_items (id, name, category_id, appliance_id, season, interval_months, created_at, updated_at) VALUES (1, 'Replace HVAC Filter', 1, 2, 'fall', 3, ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO maintenance_items (id, name, category_id, appliance_id, season, interval_months, created_at, updated_at) VALUES (2, 'Flush Water Heater', 4, 1, 'spring', 12, ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO service_log_entries (id, maintenance_item_id, serviced_at, vendor_id, cost_cents, notes, created_at, updated_at) VALUES (1, 1, ?, 2, 15000, 'Replaced filter', ?, ?)`,
			[]any{now, now, now},
		},
		{
			`INSERT INTO incidents (id, title, description, status, severity, date_noticed, appliance_id, vendor_id, cost_cents, created_at, updated_at) VALUES (1, 'Water Heater Leak', 'Small drip from pressure valve', 'resolved', 'urgent', ?, 1, 1, 25000, ?, ?)`,
			[]any{now, now, now},
		},
		{
			`INSERT INTO documents (id, title, file_name, entity_kind, entity_id, mime_type, size_bytes, sha256, created_at, updated_at) VALUES (1, 'Kitchen Quote', 'kitchen_quote.pdf', 'project', 1, 'application/pdf', 102400, 'abc123', ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO documents (id, title, file_name, entity_kind, entity_id, mime_type, size_bytes, sha256, created_at, updated_at) VALUES (2, 'WH Manual', 'wh_manual.pdf', 'appliance', 1, 'application/pdf', 204800, 'def456', ?, ?)`,
			[]any{now, now},
		},
		{
			`INSERT INTO deletion_records (id, entity, target_id, deleted_at, restored_at) VALUES (1, 'vendor', 2, ?, ?)`,
			[]any{now, now},
		},
		{`INSERT INTO settings (key, value, updated_at) VALUES ('currency', 'USD', ?)`, []any{now}},
		{
			`INSERT INTO chat_inputs (id, input, created_at) VALUES (1, 'how much did I spend on plumbing?', ?)`,
			[]any{now},
		},
	}
	for _, ins := range inserts {
		_, err := db.ExecContext(t.Context(), ins.sql, ins.args...)
		require.NoError(t, err, "INSERT: %s", ins.sql)
	}

	return path
}

// TestMigrateE2E_V22EmptyAfterSeedDefaults simulates a user who ran v2.2,
// created the house profile via the setup wizard, but never added any other
// data. This is the "empty database" case from the issue.
func TestMigrateE2E_V22EmptyAfterSeedDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "micasa.db")

	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	require.NoError(t, err)

	// Only create the tables that SeedDefaults populates + house_profiles
	ddls := []string{
		`CREATE TABLE "house_profiles" ("id" integer,"nickname" text,"address_line1" text,"city" text,"state" text,"postal_code" text,"year_built" integer,"square_feet" integer,"lot_square_feet" integer,"bedrooms" integer,"bathrooms" real,"foundation_type" text,"wiring_type" text,"roof_type" text,"exterior_type" text,"heating_type" text,"cooling_type" text,"water_source" text,"sewer_type" text,"parking_type" text,"basement_type" text,"insurance_carrier" text,"insurance_policy" text,"insurance_renewal" datetime,"property_tax_cents" integer,"hoa_name" text,"hoa_fee_cents" integer,"address_line2" text,"created_at" datetime,"updated_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "project_types" ("id" integer,"name" text UNIQUE,"created_at" datetime,"updated_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "maintenance_categories" ("id" integer,"name" text UNIQUE,"created_at" datetime,"updated_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "vendors" ("id" integer,"name" text UNIQUE,"contact_name" text,"email" text,"phone" text,"website" text,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "appliances" ("id" integer,"name" text,"brand" text,"model_number" text,"serial_number" text,"purchase_date" datetime,"warranty_expiry" datetime,"location" text,"cost_cents" integer,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "projects" ("id" integer,"title" text,"project_type_id" integer,"status" text DEFAULT 'planned',"description" text,"start_date" datetime,"end_date" datetime,"budget_cents" integer,"actual_cents" integer,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "quotes" ("id" integer,"project_id" integer,"vendor_id" integer,"total_cents" integer,"labor_cents" integer,"materials_cents" integer,"other_cents" integer,"received_date" datetime,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "maintenance_items" ("id" integer,"name" text,"category_id" integer,"appliance_id" integer,"season" text,"last_serviced_at" datetime,"interval_months" integer,"due_date" datetime,"manual_url" text,"manual_text" text,"notes" text,"cost_cents" integer,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "incidents" ("id" integer,"title" text,"description" text,"status" text DEFAULT 'open',"previous_status" text,"severity" text DEFAULT 'soon',"date_noticed" datetime,"date_resolved" datetime,"location" text,"cost_cents" integer,"appliance_id" integer,"vendor_id" integer,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "service_log_entries" ("id" integer,"maintenance_item_id" integer,"serviced_at" datetime,"vendor_id" integer,"cost_cents" integer,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "documents" ("id" integer,"title" text,"file_name" text,"entity_kind" text,"entity_id" integer,"mime_type" text,"size_bytes" integer,"sha256" text,"data" blob,"extracted_text" text,"ocr_data" blob,"extraction_model" text,"extraction_ops" blob,"notes" text,"created_at" datetime,"updated_at" datetime,"deleted_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "deletion_records" ("id" integer,"entity" text,"target_id" integer,"deleted_at" datetime,"restored_at" datetime,PRIMARY KEY ("id"))`,
		`CREATE TABLE "settings" ("key" text,"value" text,"updated_at" datetime,PRIMARY KEY ("key"))`,
		`CREATE TABLE "chat_inputs" ("id" integer,"input" text NOT NULL,"created_at" datetime,PRIMARY KEY ("id"))`,
	}
	for _, ddl := range ddls {
		_, err := db.ExecContext(t.Context(), ddl)
		require.NoError(t, err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	// Only project types and categories (from SeedDefaults)
	for i, name := range []string{"renovation", "repair", "upgrade", "addition", "landscaping", "emergency", "cosmetic", "structural", "electrical", "plumbing"} {
		_, err := db.ExecContext(t.Context(),
			`INSERT INTO project_types (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			i+1,
			name,
			now,
			now,
		)
		require.NoError(t, err)
	}
	for i, name := range []string{"HVAC", "plumbing", "electrical", "appliance", "exterior", "interior", "safety", "pest control", "seasonal"} {
		_, err := db.ExecContext(
			t.Context(),
			`INSERT INTO maintenance_categories (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			i+1,
			name,
			now,
			now,
		)
		require.NoError(t, err)
	}

	require.NoError(t, db.Close())

	// Open with current code
	store, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.AutoMigrate())

	// The migrated seed data (old integer IDs) should now have ULIDs.
	// SeedDefaults also adds the v2.3 types on top.
	require.NoError(t, store.SeedDefaults())
	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(types), 10)
	for _, pt := range types {
		assert.True(t, uid.IsValid(pt.ID))
	}

	// The critical test: create a house profile and project
	require.NoError(t, store.GormDB().Create(&HouseProfile{
		Nickname: "Test House",
		City:     "Seattle",
	}).Error)
	house, err := store.HouseProfile()
	require.NoError(t, err)
	assert.True(t, uid.IsValid(house.ID))

	require.NoError(t, store.CreateProject(&Project{
		Title:         "First Project",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}))
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.True(t, uid.IsValid(projects[0].ID))
	assert.NotEmpty(t, projects[0].ProjectType.Name)
}

// TestMigrateE2E_V22GORMExactDDL verifies migration works with the exact
// DDL format GORM produces (double-quoted identifiers, PRIMARY KEY as
// separate clause) rather than the hand-written DDL in the simpler tests.
// This catches DDL-parsing issues that only surface with GORM's format.
func TestMigrateE2E_V22GORMExactDDL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "gorm-exact.db")

	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)

	// This is the exact DDL GORM produces: PRIMARY KEY as separate clause,
	// double-quoted identifiers, no AUTOINCREMENT.
	_, err = db.ExecContext(
		t.Context(),
		`CREATE TABLE "project_types" ("id" integer,"name" text UNIQUE,"created_at" datetime,"updated_at" datetime,PRIMARY KEY ("id"))`,
	)
	require.NoError(t, err)

	now := time.Now().UTC()
	_, err = db.ExecContext(t.Context(),
		`INSERT INTO "project_types" ("id","name","created_at","updated_at") VALUES (1,'test',?,?)`,
		now,
		now,
	)
	require.NoError(t, err)

	require.NoError(t, db.Close())

	store, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.AutoMigrate())

	types, err := store.ProjectTypes()
	require.NoError(t, err)
	require.Len(t, types, 1)
	assert.True(t, uid.IsValid(types[0].ID))
	assert.Equal(t, "test", types[0].Name)

	// New insert must work
	require.NoError(t, store.SeedDefaults())
	types2, err := store.ProjectTypes()
	require.NoError(t, err)
	assert.Greater(t, len(types2), 1)
}
