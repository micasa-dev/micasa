// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVendorTabExists(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	found := false
	for _, tab := range m.tabs {
		if tab.Kind == tabVendors {
			found = true
			break
		}
	}
	require.True(t, found, "expected Vendors tab to exist")
}

func TestVendorTabIndex(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 5, tabIndex(tabVendors))
}

func TestVendorTabKindString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Vendors", tabVendors.String())
}

func TestVendorColumnSpecs(t *testing.T) {
	t.Parallel()
	specs := vendorColumnSpecs()
	require.Len(t, specs, 9)
	expected := []string{
		"ID",
		"Name",
		"Contact",
		"Email",
		"Phone",
		"Website",
		"Quotes",
		"Jobs",
		"Docs",
	}
	for i, want := range expected {
		assert.Equalf(t, want, specs[i].Title, "column %d", i)
	}
}

func TestVendorRows(t *testing.T) {
	t.Parallel()
	rows, meta, cells := vendorRows(
		sampleVendors(),
		map[string]int{"01JTEST00000000000000001": 3},
		map[string]int{"01JTEST00000000000000002": 5},
		nil,
	)
	require.Len(t, rows, 2)
	assert.Equal(t, "01JTEST00000000000000001", meta[0].ID)
	assert.Equal(t, "01JTEST00000000000000002", meta[1].ID)
	// Vendor 1 has 3 quotes, 0 jobs.
	assert.Equal(t, "3", cells[0][6].Value)
	assert.Equal(t, "0", cells[0][7].Value)
	// Vendor 2 has 0 quotes, 5 jobs.
	assert.Equal(t, "0", cells[1][6].Value)
	assert.Equal(t, "5", cells[1][7].Value)
}

func TestVendorRowsDocCount(t *testing.T) {
	t.Parallel()
	docCounts := map[string]int{"01JTEST00000000000000001": 9}
	_, _, cells := vendorRows(sampleVendors(), nil, nil, docCounts)
	require.Len(t, cells, 2)
	assert.Equal(t, "9", cells[0][int(vendorColDocs)].Value)
	assert.Equal(t, cellDrilldown, cells[0][int(vendorColDocs)].Kind)
	assert.Equal(t, "0", cells[1][int(vendorColDocs)].Value)
}

func TestVendorHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := vendorHandler{}
	assert.Equal(t, formVendor, h.FormKind())
}

func TestVendorHandlerDeleteRestore(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	h := vendorHandler{}
	require.NoError(t, m.store.CreateVendor(&data.Vendor{Name: "Test Co"}))
	vendors, _ := m.store.ListVendors(false)
	id := vendors[0].ID

	require.NoError(t, h.Delete(m.store, id))
	vendors, _ = m.store.ListVendors(false)
	assert.Empty(t, vendors)

	require.NoError(t, h.Restore(m.store, id))
	vendors, _ = m.store.ListVendors(false)
	assert.Len(t, vendors, 1)
}

func TestVendorTabNavigable(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	// Navigate to vendor tab.
	m.active = tabIndex(tabVendors)
	tab := m.activeTab()
	require.NotNil(t, tab)
	assert.Equal(t, tabVendors, tab.Kind)
}

func TestVendorColumnSpecKinds(t *testing.T) {
	t.Parallel()
	specs := vendorColumnSpecs()
	// ID (0) is readonly.
	assert.Equal(t, cellReadonly, specs[0].Kind, "ID column should be readonly")
	// Editable columns: Name, Contact, Email, Phone, Website.
	for _, col := range []int{1, 2, 3, 4, 5} {
		assert.Equalf(t, cellText, specs[col].Kind,
			"col %d (%s): expected cellText", col, specs[col].Title)
	}
	// Quotes (6), Jobs (7), and Docs (8) are drilldown columns.
	for _, col := range []int{6, 7, 8} {
		assert.Equalf(t, cellDrilldown, specs[col].Kind,
			"col %d (%s): expected cellDrilldown", col, specs[col].Title)
	}
}

func TestQuoteVendorColumnLinksToVendorTab(t *testing.T) {
	t.Parallel()
	specs := quoteColumnSpecs()
	vendorSpec := specs[2] // Vendor column
	require.NotNil(t, vendorSpec.Link, "expected Vendor column to have a Link")
	assert.Equal(t, tabVendors, vendorSpec.Link.TargetTab)
}

func TestVendorJobsItemColumnLinksToMaintenanceTab(t *testing.T) {
	t.Parallel()
	specs := vendorJobsColumnSpecs()
	itemSpec := specs[1] // Item column
	require.NotNil(t, itemSpec.Link, "expected Item column to have a Link")
	assert.Equal(t, tabMaintenance, itemSpec.Link.TargetTab)
}

func TestVendorJobsRowsSetsItemLinkID(t *testing.T) {
	t.Parallel()
	entries := []data.ServiceLogEntry{
		{
			ID:                "01JTEST00000000000000001",
			MaintenanceItemID: "01JTEST00000000000000007",
			MaintenanceItem:   data.MaintenanceItem{Name: "HVAC Filter"},
			ServicedAt:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		},
	}
	_, _, cells := vendorJobsRows(entries, locale.DefaultCurrency())
	require.Len(t, cells, 1)
	assert.Equal(t, "HVAC Filter", cells[0][1].Value)
	assert.Equal(t, "01JTEST00000000000000007", cells[0][1].LinkID)
}

func sampleVendors() []data.Vendor {
	return []data.Vendor{
		{
			ID:          "01JTEST00000000000000001",
			Name:        "Acme Plumbing",
			ContactName: "Jo Smith",
			Email:       "jo@example.com",
			Phone:       "555-0142",
		},
		{
			ID:          "01JTEST00000000000000002",
			Name:        "Sparks Electric",
			ContactName: "Tom",
			Phone:       "555-0231",
		},
	}
}
