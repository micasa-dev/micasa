// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/table"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProjectRows(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	budget := int64(100000)
	start := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	projects := []data.Project{
		{
			ID:            1,
			Title:         "Kitchen",
			ProjectType:   data.ProjectType{Name: "Renovation"},
			ProjectTypeID: 1,
			Status:        data.ProjectStatusPlanned,
			BudgetCents:   &budget,
			StartDate:     &start,
		},
	}
	rows, meta, cells := projectRows(projects, nil, nil, cur)
	require.Len(t, rows, 1)
	assert.Equal(t, uint(1), meta[0].ID)
	assert.False(t, meta[0].Deleted)
	assert.Equal(t, "Kitchen", cells[0][2].Value)
	assert.Equal(t, "$1,000.00", cells[0][4].Value)
	assert.Equal(t, "2025-03-01", cells[0][6].Value)
	assert.Equal(t, "Kitchen", rows[0][2])
}

func TestProjectRowsDeleted(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	projects := []data.Project{
		{
			ID:        1,
			Title:     "Old Project",
			DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true},
		},
	}
	_, meta, _ := projectRows(projects, nil, nil, cur)
	assert.True(t, meta[0].Deleted)
}

func TestQuoteRows(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	labor := int64(20000)
	quotes := []data.Quote{
		{
			ID:         1,
			ProjectID:  1,
			Project:    data.Project{Title: "Kitchen"},
			Vendor:     data.Vendor{Name: "ContractorCo"},
			VendorID:   1,
			TotalCents: 50000,
			LaborCents: &labor,
		},
	}
	rows, meta, cells := quoteRows(quotes, nil, cur)
	require.Len(t, rows, 1)
	assert.Equal(t, uint(1), meta[0].ID)
	assert.Equal(t, "Kitchen", cells[0][1].Value)
	assert.Equal(t, uint(1), cells[0][1].LinkID)
	assert.Equal(t, "ContractorCo", cells[0][2].Value)
	assert.Equal(t, "$500.00", cells[0][3].Value)
}

func TestQuoteRowsFallbackProjectName(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	quotes := []data.Quote{
		{
			ID:         1,
			ProjectID:  42,
			TotalCents: 100,
		},
	}
	_, _, cells := quoteRows(quotes, nil, cur)
	assert.Equal(t, "Project 42", cells[0][1].Value)
}

func TestQuoteRowsDocCount(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	quotes := []data.Quote{
		{ID: 1, ProjectID: 1, Project: data.Project{Title: "Kitchen"}, TotalCents: 100},
		{ID: 2, ProjectID: 1, Project: data.Project{Title: "Kitchen"}, TotalCents: 200},
	}
	docCounts := map[uint]int{2: 5}
	_, _, cells := quoteRows(quotes, docCounts, cur)
	require.Len(t, cells, 2)
	assert.Equal(t, "0", cells[0][int(quoteColDocs)].Value)
	assert.Equal(t, cellDrilldown, cells[0][int(quoteColDocs)].Kind)
	assert.Equal(t, "5", cells[1][int(quoteColDocs)].Value)
}

func TestMaintenanceRows(t *testing.T) {
	t.Parallel()
	appID := uint(5)
	lastServiced := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []data.MaintenanceItem{
		{
			ID:             1,
			Name:           "HVAC Filter",
			Category:       data.MaintenanceCategory{Name: "HVAC"},
			ApplianceID:    &appID,
			Appliance:      data.Appliance{Name: "AC Unit"},
			LastServicedAt: &lastServiced,
			IntervalMonths: 3,
		},
	}
	logCounts := map[uint]int{1: 4}
	rows, meta, cells := maintenanceRows(items, logCounts, nil)
	require.Len(t, rows, 1)
	assert.Equal(t, uint(1), meta[0].ID)
	assert.Equal(t, "HVAC Filter", cells[0][int(maintenanceColItem)].Value)
	assert.Equal(t, "HVAC", cells[0][int(maintenanceColCategory)].Value)
	assert.Equal(t, "AC Unit", cells[0][int(maintenanceColAppliance)].Value)
	assert.Equal(t, uint(5), cells[0][int(maintenanceColAppliance)].LinkID)
	assert.Equal(t, "3m", cells[0][int(maintenanceColEvery)].Value)
	assert.Equal(t, "4", cells[0][int(maintenanceColLog)].Value)
}

func TestMaintenanceRowsDocCount(t *testing.T) {
	t.Parallel()
	items := []data.MaintenanceItem{
		{ID: 1, Name: "HVAC Filter", IntervalMonths: 3},
		{ID: 2, Name: "Gutters", IntervalMonths: 6},
	}
	docCounts := map[uint]int{1: 7}
	_, _, cells := maintenanceRows(items, nil, docCounts)
	require.Len(t, cells, 2)
	assert.Equal(t, "7", cells[0][int(maintenanceColDocs)].Value)
	assert.Equal(t, cellDrilldown, cells[0][int(maintenanceColDocs)].Kind)
	assert.Equal(t, "0", cells[1][int(maintenanceColDocs)].Value)
}

func TestMaintenanceRowsNoAppliance(t *testing.T) {
	t.Parallel()
	items := []data.MaintenanceItem{
		{ID: 1, Name: "Gutters", Category: data.MaintenanceCategory{Name: "Exterior"}},
	}
	_, _, cells := maintenanceRows(items, nil, nil)
	appCol := int(maintenanceColAppliance)
	assert.Empty(t, cells[0][appCol].Value)
	assert.True(t, cells[0][appCol].Null, "nil appliance should produce a null cell")
	assert.Zero(t, cells[0][appCol].LinkID)
}

// Step 12: Due-date items use the same urgency cell kind as interval items,
// ensuring identical overdue/upcoming coloring.
func TestMaintenanceRowsDueDateUrgencyCell(t *testing.T) {
	t.Parallel()
	due := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
	items := []data.MaintenanceItem{
		{
			ID:       1,
			Name:     "Inspect Roof",
			DueDate:  &due,
			Category: data.MaintenanceCategory{Name: "Exterior"},
		},
	}
	_, _, cells := maintenanceRows(items, nil, nil)
	nextCell := cells[0][int(maintenanceColNext)]
	// "Next" column shows the due date with cellUrgency kind (same as interval items).
	assert.Equal(t, "2025-11-01", nextCell.Value)
	assert.Equal(t, cellUrgency, nextCell.Kind,
		"due-date items must use cellUrgency for consistent overdue/upcoming coloring")
	// "Every" column is NULL (non-recurring).
	everyCell := cells[0][int(maintenanceColEvery)]
	assert.Empty(t, everyCell.Value)
	assert.True(t, everyCell.Null, "non-recurring items should have NULL interval")
}

func TestMaintenanceRowsSeasonCell(t *testing.T) {
	t.Parallel()
	items := []data.MaintenanceItem{
		{
			ID:       1,
			Name:     "Clean Gutters",
			Season:   data.SeasonSpring,
			Category: data.MaintenanceCategory{Name: "Exterior"},
		},
	}
	_, _, cells := maintenanceRows(items, nil, nil)
	seasonCell := cells[0][int(maintenanceColSeason)]
	assert.Equal(t, data.SeasonSpring, seasonCell.Value)
	assert.Equal(t, cellStatus, seasonCell.Kind,
		"season should render as cellStatus for badge styling and pin-filtering")
}

func TestMaintenanceRowsNoSeason(t *testing.T) {
	t.Parallel()
	items := []data.MaintenanceItem{
		{
			ID:       1,
			Name:     "Oil Furnace",
			Category: data.MaintenanceCategory{Name: "HVAC"},
		},
	}
	_, _, cells := maintenanceRows(items, nil, nil)
	seasonCell := cells[0][int(maintenanceColSeason)]
	assert.Empty(t, seasonCell.Value)
	assert.True(t, seasonCell.Null, "empty season should produce a null cell")
}

func TestApplianceRows(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	cost := int64(89900)
	purchase := time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)
	now := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	items := []data.Appliance{
		{
			ID:           1,
			Name:         "Fridge",
			Brand:        "Samsung",
			ModelNumber:  "RF28R",
			PurchaseDate: &purchase,
			CostCents:    &cost,
		},
	}
	maintCounts := map[uint]int{1: 2}
	rows, meta, cells := applianceRows(items, maintCounts, nil, now, cur)
	require.Len(t, rows, 1)
	assert.Equal(t, uint(1), meta[0].ID)
	assert.Equal(t, "Fridge", cells[0][1].Value)
	assert.Equal(t, "Samsung", cells[0][2].Value)
	assert.Equal(t, "2023-06-15", cells[0][6].Value)
	assert.Equal(t, "2y", cells[0][7].Value)
	assert.Equal(t, "$899.00", cells[0][9].Value)
	assert.Equal(t, "2", cells[0][10].Value)
}

func TestApplianceRowsNoOptionalFields(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	now := time.Now()
	items := []data.Appliance{
		{ID: 1, Name: "Lamp"},
	}
	_, _, cells := applianceRows(items, nil, nil, now, cur)
	assert.Empty(t, cells[0][6].Value, "expected empty purchase date")
	assert.True(t, cells[0][6].Null, "nil purchase date should be null")
	assert.Empty(t, cells[0][7].Value, "expected empty age")
	assert.True(t, cells[0][7].Null, "age without purchase date should be null")
	assert.Empty(t, cells[0][9].Value, "expected empty cost")
	assert.True(t, cells[0][9].Null, "nil cost should be null")
	assert.Equal(t, "0", cells[0][10].Value, "zero maint count should be explicit")
}

func TestBuildRowsEmpty(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	rows, meta, cells := projectRows(nil, nil, nil, cur)
	assert.Empty(t, rows)
	assert.Empty(t, meta)
	assert.Empty(t, cells)
}

func TestDocumentRows(t *testing.T) {
	t.Parallel()
	docs := []data.Document{
		{
			ID:         1,
			Title:      "Invoice",
			EntityKind: data.DocumentEntityProject,
			EntityID:   42,
			MIMEType:   "application/pdf",
			SizeBytes:  2048,
			UpdatedAt:  time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	names := entityNameMap{
		{Kind: data.DocumentEntityProject, ID: 42}: "Kitchen Reno",
	}
	rows, meta, cells := documentRows(docs, names)
	require.Len(t, rows, 1)
	assert.Equal(t, uint(1), meta[0].ID)
	assert.Equal(t, "Invoice", cells[0][1].Value)
	assert.Equal(t, "P Kitchen Reno", cells[0][2].Value)
	assert.Equal(t, "application/pdf", cells[0][3].Value)
	assert.Equal(t, "2.0 KB", cells[0][4].Value)
}

func TestEntityDocumentRows(t *testing.T) {
	t.Parallel()
	docs := []data.Document{
		{
			ID:        1,
			Title:     "Manual",
			MIMEType:  "application/pdf",
			SizeBytes: 1048576, // 1 MB
			UpdatedAt: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	rows, meta, cells := entityDocumentRows(docs)
	require.Len(t, rows, 1)
	assert.Equal(t, uint(1), meta[0].ID)
	assert.Equal(t, "Manual", cells[0][1].Value)
	assert.Equal(t, "application/pdf", cells[0][2].Value)
	assert.Equal(t, "1.0 MB", cells[0][3].Value)
}

func TestFormatFileSize(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "0 B", formatFileSize(0))
	assert.Equal(t, "512 B", formatFileSize(512))
	assert.Equal(t, "1.0 KB", formatFileSize(1024))
	assert.Equal(t, "1.5 KB", formatFileSize(1536))
	assert.Equal(t, "1.0 MB", formatFileSize(1024*1024))
	assert.Equal(t, "1.0 GB", formatFileSize(1024*1024*1024))
}

func TestDocSizeBytesClamsNegative(t *testing.T) {
	t.Parallel()
	d := data.Document{SizeBytes: -1}
	assert.Equal(t, uint64(0), docSizeBytes(d))
}

func TestDocSizeBytesPassesPositive(t *testing.T) {
	t.Parallel()
	d := data.Document{SizeBytes: 4096}
	assert.Equal(t, uint64(4096), docSizeBytes(d))
}

func TestDocSizeBytesZero(t *testing.T) {
	t.Parallel()
	d := data.Document{SizeBytes: 0}
	assert.Equal(t, uint64(0), docSizeBytes(d))
}

func TestDocumentEntityLabel(t *testing.T) {
	t.Parallel()
	names := entityNameMap{
		{Kind: "project", ID: 5}:    "Kitchen Reno",
		{Kind: "appliance", ID: 12}: "Dishwasher",
	}
	assert.Empty(t, documentEntityLabel("", 0, names))
	assert.Equal(t, "P Kitchen Reno", documentEntityLabel("project", 5, names))
	assert.Equal(t, "A Dishwasher", documentEntityLabel("appliance", 12, names))
	assert.Equal(
		t,
		"V #99",
		documentEntityLabel("vendor", 99, names),
		"fallback for missing name",
	)
}

func TestCentsCellNil(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	c := centsCell(nil, cur)
	assert.True(t, c.Null, "nil cents should produce a null cell")
	assert.Empty(t, c.Value)
	assert.Equal(t, cellMoney, c.Kind)
}

func TestCentsCellPresent(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	v := int64(123456)
	c := centsCell(&v, cur)
	assert.False(t, c.Null)
	assert.Equal(t, "$1,234.56", c.Value)
}

func TestDateCellNil(t *testing.T) {
	t.Parallel()
	c := dateCell(nil, cellDate)
	assert.True(t, c.Null, "nil time should produce a null cell")
	assert.Empty(t, c.Value)
	assert.Equal(t, cellDate, c.Kind)
}

func TestDateCellPresent(t *testing.T) {
	t.Parallel()
	d := time.Date(2025, 6, 11, 0, 0, 0, 0, time.UTC)
	c := dateCell(&d, cellDate)
	assert.False(t, c.Null)
	assert.Equal(t, "2025-06-11", c.Value)
}

func TestProjectRowsNullOptionalFields(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	projects := []data.Project{
		{ID: 1, Title: "Minimal", Status: data.ProjectStatusPlanned},
	}
	_, _, cells := projectRows(projects, nil, nil, cur)
	require.Len(t, cells, 1)
	// Budget (col 4), Actual (col 5), Start (col 6), End (col 7) are all nil.
	assert.True(t, cells[0][4].Null, "nil budget should be null")
	assert.True(t, cells[0][5].Null, "nil actual should be null")
	assert.True(t, cells[0][6].Null, "nil start date should be null")
	assert.True(t, cells[0][7].Null, "nil end date should be null")
}

func TestCellsToRow(t *testing.T) {
	t.Parallel()
	cells := []cell{
		{Value: "1"},
		{Value: "hello"},
		{Value: "$100.00"},
	}
	row := cellsToRow(cells)
	assert.Equal(t, table.Row{"1", "hello", "$100.00"}, row)
}
