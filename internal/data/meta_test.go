// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneratedTableNames(t *testing.T) {
	assert.Equal(t, "house_profiles", TableHouseProfiles)
	assert.Equal(t, "project_types", TableProjectTypes)
	assert.Equal(t, "projects", TableProjects)
	assert.Equal(t, "vendors", TableVendors)
	assert.Equal(t, "quotes", TableQuotes)
	assert.Equal(t, "maintenance_categories", TableMaintenanceCategories)
	assert.Equal(t, "appliances", TableAppliances)
	assert.Equal(t, "maintenance_items", TableMaintenanceItems)
	assert.Equal(t, "incidents", TableIncidents)
	assert.Equal(t, "service_log_entries", TableServiceLogEntries)
	assert.Equal(t, "documents", TableDocuments)
	assert.Equal(t, "deletion_records", TableDeletionRecords)
}

func TestGeneratedColumnNames(t *testing.T) {
	assert.Equal(t, "vendor_id", ColVendorID)
	assert.Equal(t, "project_id", ColProjectID)
	assert.Equal(t, "title", ColTitle)
	assert.Equal(t, "id", ColID)
	assert.Equal(t, "created_at", ColCreatedAt)
	assert.Equal(t, "updated_at", ColUpdatedAt)
	assert.Equal(t, "deleted_at", ColDeletedAt)
	assert.Equal(t, "name", ColName)
	assert.Equal(t, "contact_name", ColContactName)

	// Custom column mappings via gorm tags
	assert.Equal(t, "file_name", ColFileName)
	assert.Equal(t, "sha256", ColChecksumSHA256)
	assert.Equal(t, "ocr_data", ColExtractData)
}
