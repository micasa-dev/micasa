// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/table"
	"github.com/charmbracelet/x/ansi"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenDetailSetsContext(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	require.Nil(t, m.detail())

	require.NoError(t, m.openServiceLogDetail("01JTEST00000000000000042", "Test Item"))
	require.NotNil(t, m.detail())
	assert.Equal(t, "01JTEST00000000000000042", m.detail().ParentRowID)
	assert.Equal(
		t,
		"Maintenance"+breadcrumbSep+"Test Item"+breadcrumbSep+"Service Log",
		m.detail().Breadcrumb,
	)
}

func TestCloseDetailRestoresParent(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JTEST00000000000000042", "Test Item")

	m.closeDetail()
	assert.Nil(t, m.detail())
	assert.Equal(t, tabIndex(tabMaintenance), m.active)
}

func TestEffectiveTabReturnsDetailWhenOpen(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	mainTab := m.effectiveTab()
	require.NotNil(t, mainTab)
	assert.Equal(t, tabMaintenance, mainTab.Kind)

	_ = m.openServiceLogDetail("01JTEST00000000000000001", "Test")
	detailTab := m.effectiveTab()
	require.NotNil(t, detailTab)
	require.NotNil(t, detailTab.Handler)
	assert.Equal(t, formServiceLog, detailTab.Handler.FormKind())
}

func TestEffectiveTabFallsBackToMainTab(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabProjects)
	tab := m.effectiveTab()
	require.NotNil(t, tab)
	assert.Equal(t, tabProjects, tab.Kind)
}

func TestEscInNormalModeClosesDetail(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JTEST00000000000000001", "Test")
	require.NotNil(t, m.detail())
	sendKey(m, "esc")
	assert.Nil(t, m.detail())
}

func TestEscInEditModeDoesNotCloseDetail(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JTEST00000000000000001", "Test")

	sendKey(m, "i") // enter edit mode
	require.Equal(t, modeEdit, m.mode)
	sendKey(m, "esc") // should go to normal, not close detail
	assert.Equal(t, modeNormal, m.mode)
	assert.NotNil(t, m.detail(), "expected detail still open after edit-mode esc")
}

func TestTabSwitchBlockedInDetailView(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JTEST00000000000000001", "Test")

	before := m.active
	sendKey(m, "f")
	assert.Equal(t, before, m.active, "tab switch should be blocked while in detail view")
}

func TestColumnNavWorksInDetailView(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JTEST00000000000000001", "Test")

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	initial := tab.ColCursor
	sendKey(m, "l")
	if len(tab.Specs) > 1 {
		assert.NotEqual(
			t,
			initial,
			tab.ColCursor,
			"expected column cursor to advance in detail view",
		)
	}
}

func TestDetailTabHasServiceLogSpecs(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JTEST00000000000000001", "Test")

	tab := m.effectiveTab()
	require.Len(t, tab.Specs, 6)
	expected := []string{"ID", "Date", "Performed By", "Cost", "Notes", tabDocuments.String()}
	for i, want := range expected {
		assert.Equalf(t, want, tab.Specs[i].Title, "column %d", i)
	}
}

func TestHandlerForFormKindFindsDetailHandler(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JTEST00000000000000001", "Test")

	handler := m.handlerForFormKind(formServiceLog)
	require.NotNil(t, handler)
	assert.Equal(t, formServiceLog, handler.FormKind())
}

func TestServiceLogHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newServiceLogHandler("01JTEST00000000000000005")
	assert.Equal(t, formServiceLog, h.FormKind())
}

func TestMaintenanceColumnsIncludeLogAndDocs(t *testing.T) {
	t.Parallel()
	specs := maintenanceColumnSpecs()
	secondLast := specs[len(specs)-2]
	assert.Equal(t, "Log", secondLast.Title)
	assert.Equal(t, cellDrilldown, secondLast.Kind)
}

func TestApplianceColumnsIncludeMaintAndDocs(t *testing.T) {
	t.Parallel()
	specs := applianceColumnSpecs()
	secondLast := specs[len(specs)-2]
	assert.Equal(t, "Maint", secondLast.Title)
	assert.Equal(t, cellDrilldown, secondLast.Kind)
	last := specs[len(specs)-1]
	assert.Equal(t, tabDocuments.String(), last.Title)
	assert.Equal(t, cellDrilldown, last.Kind)
}

func TestVendorOptions(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	opts := vendorOpts("Self (homeowner)", m.vendors)
	require.NotEmpty(t, opts, "expected at least 1 vendor option (Self)")
	assert.Empty(t, opts[0].Value, "expected first vendor option value=\"\" (Self)")
}

func TestServiceLogColumnSpecs(t *testing.T) {
	t.Parallel()
	specs := serviceLogColumnSpecs()
	require.Len(t, specs, 6)
	// Verify the "Performed By" column is flex and linked to vendors.
	pb := specs[2]
	assert.True(t, pb.Flex)
	require.NotNil(t, pb.Link)
	assert.Equal(t, tabVendors, pb.Link.TargetTab)
}

func TestServiceLogRowsSelfPerformed(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	entries := []data.ServiceLogEntry{
		{
			ID:         "01JTEST00000000000000001",
			ServicedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Notes:      "test note",
		},
	}
	_, meta, cellRows := serviceLogRows(entries, nil, cur)
	require.Len(t, cellRows, 1)
	assert.Equal(t, "Self", cellRows[0][2].Value)
	assert.Equal(t, "01JTEST00000000000000001", meta[0].ID)
}

func TestServiceLogRowsVendorPerformed(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	vendorID := "01JTEST00000000000000005"
	entries := []data.ServiceLogEntry{
		{
			ID:         "01JTEST00000000000000002",
			ServicedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			VendorID:   &vendorID,
			Vendor:     data.Vendor{Name: "Acme Plumbing"},
		},
	}
	_, _, cellRows := serviceLogRows(entries, nil, cur)
	assert.Equal(t, "Acme Plumbing", cellRows[0][2].Value)
	assert.Equal(t, "01JTEST00000000000000005", cellRows[0][2].LinkID)
}

func TestServiceLogRowsSelfHasNoLink(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	entries := []data.ServiceLogEntry{
		{
			ID:         "01JTEST00000000000000001",
			ServicedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	_, _, cellRows := serviceLogRows(entries, nil, cur)
	assert.Empty(t, cellRows[0][2].LinkID)
}

func TestServiceLogRowsDocCount(t *testing.T) {
	t.Parallel()
	cur := locale.DefaultCurrency()
	entries := []data.ServiceLogEntry{
		{ID: "01JTEST00000000000000001", ServicedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "01JTEST00000000000000002", ServicedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
	}
	docCounts := map[string]int{"01JTEST00000000000000001": 3}
	_, _, cellRows := serviceLogRows(entries, docCounts, cur)
	require.Len(t, cellRows, 2)
	assert.Equal(t, "3", cellRows[0][int(serviceLogColDocs)].Value)
	assert.Equal(t, cellDrilldown, cellRows[0][int(serviceLogColDocs)].Kind)
	assert.Equal(t, "0", cellRows[1][int(serviceLogColDocs)].Value)
}

func TestMaintenanceLogColumnReplacedManual(t *testing.T) {
	t.Parallel()
	specs := maintenanceColumnSpecs()
	for _, s := range specs {
		assert.NotEqual(t, "Manual", s.Title, "expected 'Manual' column to be replaced by 'Log'")
	}
}

func TestResizeTablesIncludesDetail(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JTEST00000000000000001", "Test")

	m.resizeTables()
	assert.Positive(t, m.detail().Tab.Table.Height())
}

func TestSortWorksInDetailView(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JTEST00000000000000001", "Test")

	tab := m.effectiveTab()
	tab.ColCursor = 1 // Date column

	sendKey(m, "s")
	assert.NotEmpty(t, tab.Sorts, "expected sort entry after 's' in detail view")
}

func TestSortIndicatorAppearsImmediately(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDetailRows(t)

	tab := m.effectiveTab()
	tab.ColCursor = 1 // Date column

	// Render before sort -- no sort indicator.
	before := ansi.Strip(m.View().Content)
	require.NotContains(t, before, symTriUp,
		"sort indicator should not appear before sorting")

	// Press sort -- the rendered output must show the indicator
	// immediately, without requiring a navigation keypress.
	sendKey(m, "s")
	after := ansi.Strip(m.View().Content)
	assert.Contains(t, after, symTriUp,
		"sort indicator must appear in rendered view immediately after pressing 's'")
}

func TestSortImmediatelyReordersRenderedRows(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDetailRows(t)

	tab := m.effectiveTab()
	tab.ColCursor = 1 // Date column

	// Render before sort -- first date should appear before second.
	// Strip ANSI: lipgloss v2 may split styled text across escape sequences.
	before := ansi.Strip(m.View().Content)
	idx1 := strings.Index(before, "2026-01-15")
	idx2 := strings.Index(before, "2026-02-01")
	require.Greater(t, idx1, -1, "first date should be in initial render")
	require.Greater(t, idx2, -1, "second date should be in initial render")
	require.Less(t, idx1, idx2, "earlier date should render first initially")

	// Sort ascending by date -- already in order, so press twice for desc.
	sendKey(m, "s")
	sendKey(m, "s")

	// The rendered view must show rows in descending date order
	// immediately, without any navigation keypress.
	after := ansi.Strip(m.View().Content)
	idx1 = strings.Index(after, "2026-01-15")
	idx2 = strings.Index(after, "2026-02-01")
	require.Greater(t, idx1, -1, "first date should be in render after sort")
	require.Greater(t, idx2, -1, "second date should be in render after sort")
	assert.Less(t, idx2, idx1,
		"sort desc must put later date before earlier date in rendered view without navigation")
}

// newTestModelWithDetailRows creates a model with detail open and seeded rows.
func newTestModelWithDetailRows(t *testing.T) *Model {
	t.Helper()

	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)
	_ = m.openServiceLogDetail("01JTEST00000000000000001", "Test")

	tab := m.effectiveTab()
	// Seed a couple rows.
	tab.Table.SetRows([]table.Row{
		{"1", "2026-01-15", "Self", "", "first"},
		{"2", "2026-02-01", "Acme", "$150.00", "second"},
	})
	tab.Table.SetCursor(0)
	tab.Rows = []rowMeta{{ID: "01JTEST00000000000000001"}, {ID: "01JTEST00000000000000002"}}
	tab.CellRows = [][]cell{
		{
			{Value: "1", Kind: cellReadonly},
			{Value: "2026-01-15", Kind: cellDate},
			{Value: "Self", Kind: cellText},
			{Value: "", Kind: cellMoney},
			{Value: "first", Kind: cellText},
		},
		{
			{Value: "2", Kind: cellReadonly},
			{Value: "2026-02-01", Kind: cellDate},
			{Value: "Acme", Kind: cellText},
			{Value: "$150.00", Kind: cellMoney},
			{Value: "second", Kind: cellText},
		},
	}
	return m
}

func TestSelectedRowMetaUsesDetailTab(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDetailRows(t)
	meta, ok := m.selectedRowMeta()
	require.True(t, ok)
	assert.Equal(t, "01JTEST00000000000000001", meta.ID)
}

func TestSelectedCellUsesDetailTab(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDetailRows(t)
	c, ok := m.selectedCell(2)
	require.True(t, ok)
	assert.Equal(t, "Self", c.Value)
}

func TestApplianceMaintenanceDetailOpens(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabAppliances)
	require.NoError(t, m.openApplianceMaintenanceDetail("01JTEST00000000000000005", "Dishwasher"))
	require.NotNil(t, m.detail())
	assert.Equal(t, "Appliances"+breadcrumbSep+"Dishwasher", m.detail().Breadcrumb)
	assert.Equal(t, "Maintenance", m.detail().Tab.Name)
	assert.Equal(t, tabAppliances, m.detail().Tab.Kind)
}

func TestApplianceMaintenanceHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newApplianceMaintenanceHandler("01JTEST00000000000000001")
	assert.Equal(t, formMaintenance, h.FormKind())
}

func TestApplianceMaintenanceColumnSpecsNoAppliance(t *testing.T) {
	t.Parallel()
	specs := applianceMaintenanceColumnSpecs()
	for _, s := range specs {
		assert.NotEqual(
			t,
			"Appliance",
			s.Title,
			"appliance maintenance detail should not include Appliance column",
		)
	}
	// Second-to-last should be the Log drilldown, last should be Docs.
	secondLast := specs[len(specs)-2]
	assert.Equal(t, "Log", secondLast.Title)
	assert.Equal(t, cellDrilldown, secondLast.Kind)
	last := specs[len(specs)-1]
	assert.Equal(t, tabDocuments.String(), last.Title)
	assert.Equal(t, cellDrilldown, last.Kind)
}

// ---------------------------------------------------------------------------
// Drilldown stack tests
// ---------------------------------------------------------------------------

func TestDrilldownStackPushPop(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabMaintenance)

	// Push first level.
	require.NoError(t, m.openServiceLogDetail("01JTEST00000000000000010", "HVAC Filter"))
	assert.True(t, m.inDetail())
	assert.Len(t, m.detailStack, 1)
	assert.Equal(t, "Service Log", m.detail().Tab.Name)

	// Pop back.
	m.closeDetail()
	assert.False(t, m.inDetail())
	assert.Equal(t, tabIndex(tabMaintenance), m.active)
}

func TestNestedDrilldownApplianceMaintServiceLog(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabAppliances)

	// Level 1: Appliance → Maintenance
	require.NoError(t, m.openApplianceMaintenanceDetail("01JTEST00000000000000005", "Dishwasher"))
	assert.Len(t, m.detailStack, 1)
	assert.Equal(t, "Maintenance", m.detail().Tab.Name)

	// Level 2: Maintenance → Service Log (nested)
	require.NoError(t, m.openServiceLogDetail("01JTEST00000000000000042", "Filter Replacement"))
	assert.Len(t, m.detailStack, 2)
	assert.Equal(t, "Service Log", m.detail().Tab.Name)

	// Pop back to maintenance detail.
	m.closeDetail()
	assert.Len(t, m.detailStack, 1)
	assert.Equal(t, "Maintenance", m.detail().Tab.Name)

	// Pop back to top-level.
	m.closeDetail()
	assert.False(t, m.inDetail())
	assert.Equal(t, tabIndex(tabAppliances), m.active)
}

func TestCloseAllDetailsCollapsesStack(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabAppliances)

	require.NoError(t, m.openApplianceMaintenanceDetail("01JTEST00000000000000005", "Dishwasher"))
	require.NoError(t, m.openServiceLogDetail("01JTEST00000000000000042", "Filter"))
	assert.Len(t, m.detailStack, 2)

	m.closeAllDetails()
	assert.False(t, m.inDetail())
	assert.Equal(t, tabIndex(tabAppliances), m.active)
}

func TestCloseAllDetailsDeepStackFinalState(t *testing.T) {
	t.Parallel()
	// Push a two-level detail stack (Appliance -> Maintenance -> Service Log)
	// and verify closeAllDetails restores the correct top-level state in a
	// single operation.
	m := newTestModelWithDemoData(t, testSeed)
	m.active = tabIndex(tabAppliances)
	tab := m.activeTab()
	require.NotNil(t, tab)

	appliances, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	require.NotEmpty(t, appliances)

	// Find an appliance with maintenance items so nested drilldown works.
	var applianceID string
	var items []data.MaintenanceItem
	for _, a := range appliances {
		items, err = m.store.ListMaintenanceByAppliance(a.ID, false)
		require.NoError(t, err)
		if len(items) > 0 {
			applianceID = a.ID
			break
		}
	}
	require.NotEmpty(t, items)

	// Level 1: Appliance -> Maintenance
	require.NoError(t, m.openDetailForRow(tab, applianceID, "Maint"))
	require.Len(t, m.detailStack, 1)

	// Reload so rows are populated for the nested drilldown.
	require.NoError(t, m.reloadDetailTab())

	// Level 2: Maintenance -> Service Log
	detailTab := &m.detail().Tab
	require.NoError(t, m.openDetailForRow(detailTab, items[0].ID, "Log"))
	require.Len(t, m.detailStack, 2)

	// Set a status to confirm it gets cleared.
	m.status = statusMsg{Text: "should be cleared", Kind: statusInfo}

	m.closeAllDetails()

	assert.False(t, m.inDetail(), "detail stack should be empty")
	assert.Equal(t, tabIndex(tabAppliances), m.active, "should return to Appliances tab")
	assert.Equal(t, statusMsg{}, m.status, "status should be cleared")

	// The active tab should have loaded rows (reload happened).
	activeTab := m.activeTab()
	require.NotNil(t, activeTab)
	assert.NotEmpty(t, activeTab.Rows, "active tab should have rows after reload")
}

func TestCloseAllDetailsNoopOnEmptyStack(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabProjects)
	m.status = statusMsg{Text: "keep me", Kind: statusInfo}

	m.closeAllDetails()

	// Nothing should change.
	assert.Equal(t, tabIndex(tabProjects), m.active)
	assert.False(t, m.inDetail())
	assert.Equal(t, "keep me", m.status.Text, "status should be preserved when stack is empty")
}

func TestBreadcrumbsMultiLevel(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	m.active = tabIndex(tabAppliances)

	require.NoError(t, m.openApplianceMaintenanceDetail("01JTEST00000000000000005", "Dishwasher"))
	bc1 := m.breadcrumbView()
	assert.Contains(t, bc1, "Appliances")
	assert.Contains(t, bc1, "Dishwasher")

	require.NoError(t, m.openServiceLogDetail("01JTEST00000000000000042", "Filter Replacement"))
	bc2 := m.breadcrumbView()
	assert.Contains(t, bc2, "Appliances")
	assert.Contains(t, bc2, "Dishwasher")
	assert.Contains(t, bc2, "Filter Replacement")
	assert.Contains(t, bc2, "Service Log")
}

func TestEscPopsOneLevel(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabAppliances)

	require.NoError(t, m.openApplianceMaintenanceDetail("01JTEST00000000000000005", "Dishwasher"))
	require.NoError(t, m.openServiceLogDetail("01JTEST00000000000000042", "Filter"))
	assert.Len(t, m.detailStack, 2)

	sendKey(m, "esc")
	assert.Len(t, m.detailStack, 1, "esc should pop one level")

	sendKey(m, "esc")
	assert.False(t, m.inDetail(), "second esc should return to top-level")
}

// ---------------------------------------------------------------------------
// Vendor drilldown tests
// ---------------------------------------------------------------------------

func TestVendorQuoteDrilldown(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabVendors)

	require.NoError(t, m.openVendorQuoteDetail("01JTEST00000000000000003", "Acme Plumbing"))
	require.True(t, m.inDetail())
	assert.Equal(t, "Quotes", m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Vendors")
	assert.Contains(t, m.detail().Breadcrumb, "Acme Plumbing")
	assert.Contains(t, m.detail().Breadcrumb, "Quotes")

	// Verify column specs omit Vendor column.
	specs := m.effectiveTab().Specs
	for _, s := range specs {
		assert.NotEqual(t, "Vendor", s.Title,
			"vendor quote detail should not include Vendor column")
	}
	// Project column should link to Projects tab.
	assert.NotNil(t, specs[1].Link)
	assert.Equal(t, tabProjects, specs[1].Link.TargetTab)
}

func TestVendorJobsDrilldown(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabVendors)

	require.NoError(t, m.openVendorJobsDetail("01JTEST00000000000000003", "Acme Plumbing"))
	require.True(t, m.inDetail())
	assert.Equal(t, "Jobs", m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Vendors")
	assert.Contains(t, m.detail().Breadcrumb, "Acme Plumbing")
	assert.Contains(t, m.detail().Breadcrumb, "Jobs")

	// Verify column specs.
	specs := m.effectiveTab().Specs
	titles := make([]string, len(specs))
	for i, s := range specs {
		titles[i] = s.Title
	}
	assert.Equal(t, []string{"ID", "Item", "Date", "Cost", "Notes"}, titles)
}

func TestVendorQuoteHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newVendorQuoteHandler("01JTEST00000000000000001")
	assert.Equal(t, formQuote, h.FormKind())
}

func TestVendorJobsHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newVendorJobsHandler("01JTEST00000000000000001")
	assert.Equal(t, formServiceLog, h.FormKind())
}

// ---------------------------------------------------------------------------
// Project drilldown tests
// ---------------------------------------------------------------------------

func TestProjectQuoteDrilldown(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabProjects)

	require.NoError(t, m.openProjectQuoteDetail("01JTEST00000000000000007", "Kitchen Remodel"))
	require.True(t, m.inDetail())
	assert.Equal(t, "Quotes", m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Projects")
	assert.Contains(t, m.detail().Breadcrumb, "Kitchen Remodel")
	assert.Contains(t, m.detail().Breadcrumb, "Quotes")

	// Verify column specs omit Project column.
	specs := m.effectiveTab().Specs
	for _, s := range specs {
		assert.NotEqual(t, "Project", s.Title,
			"project quote detail should not include Project column")
	}
	// Vendor column should link to Vendors tab.
	assert.NotNil(t, specs[1].Link)
	assert.Equal(t, tabVendors, specs[1].Link.TargetTab)
}

func TestProjectQuoteHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newProjectQuoteHandler("01JTEST00000000000000001")
	assert.Equal(t, formQuote, h.FormKind())
}

func TestProjectColumnSpecsIncludeQuotesAndDocs(t *testing.T) {
	t.Parallel()
	specs := projectColumnSpecs()
	secondLast := specs[len(specs)-2]
	assert.Equal(t, "Quotes", secondLast.Title)
	assert.Equal(t, cellDrilldown, secondLast.Kind)
	last := specs[len(specs)-1]
	assert.Equal(t, tabDocuments.String(), last.Title)
	assert.Equal(t, cellDrilldown, last.Kind)
}

// ---------------------------------------------------------------------------
// openDetailForRow dispatch tests
// ---------------------------------------------------------------------------

func TestOpenDetailForRow_MaintenanceLog(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabMaintenance)
	tab := m.activeTab()
	require.NotNil(t, tab)

	items, err := m.store.ListMaintenance(false)
	require.NoError(t, err)
	require.NotEmpty(t, items)

	require.NoError(t, m.openDetailForRow(tab, items[0].ID, "Log"))
	require.True(t, m.inDetail())
	assert.Equal(t, "Service Log", m.detail().Tab.Name)
}

func TestOpenDetailForRow_ApplianceMaint(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabAppliances)
	tab := m.activeTab()
	require.NotNil(t, tab)

	appliances, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	require.NotEmpty(t, appliances)

	require.NoError(t, m.openDetailForRow(tab, appliances[0].ID, "Maint"))
	require.True(t, m.inDetail())
	assert.Equal(t, "Maintenance", m.detail().Tab.Name)
}

func TestOpenDetailForRow_VendorQuotes(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabVendors)
	tab := m.activeTab()
	require.NotNil(t, tab)

	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	require.NotEmpty(t, vendors)

	require.NoError(t, m.openDetailForRow(tab, vendors[0].ID, "Quotes"))
	require.True(t, m.inDetail())
	assert.Equal(t, "Quotes", m.detail().Tab.Name)
}

func TestOpenDetailForRow_VendorJobs(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabVendors)
	tab := m.activeTab()
	require.NotNil(t, tab)

	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	require.NotEmpty(t, vendors)

	require.NoError(t, m.openDetailForRow(tab, vendors[0].ID, "Jobs"))
	require.True(t, m.inDetail())
	assert.Equal(t, "Jobs", m.detail().Tab.Name)
}

func TestOpenDetailForRow_ProjectQuotes(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabProjects)
	tab := m.activeTab()
	require.NotNil(t, tab)

	projects, err := m.store.ListProjects(false)
	require.NoError(t, err)
	require.NotEmpty(t, projects)

	require.NoError(t, m.openDetailForRow(tab, projects[0].ID, "Quotes"))
	require.True(t, m.inDetail())
	assert.Equal(t, "Quotes", m.detail().Tab.Name)
}

func TestOpenDetailForRow_NestedApplianceMaintenanceLog(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabAppliances)
	tab := m.activeTab()
	require.NotNil(t, tab)

	appliances, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	require.NotEmpty(t, appliances)

	// Find an appliance that has linked maintenance items.
	var applianceID string
	var items []data.MaintenanceItem
	for _, a := range appliances {
		items, err = m.store.ListMaintenanceByAppliance(a.ID, false)
		require.NoError(t, err)
		if len(items) > 0 {
			applianceID = a.ID
			break
		}
	}
	require.NotEmpty(t, items, "demo data must have at least one appliance with maintenance")

	// Drill into maintenance items for the chosen appliance.
	require.NoError(t, m.openDetailForRow(tab, applianceID, "Maint"))
	require.True(t, m.inDetail())
	assert.Equal(t, "Maintenance", m.detail().Tab.Name)

	// The detail tab's Kind is tabAppliances (inherits from parent).
	detailTab := &m.detail().Tab
	require.Equal(t, tabAppliances, detailTab.Kind)

	// Reload so rows are populated.
	require.NoError(t, m.reloadDetailTab())

	require.NoError(t, m.openDetailForRow(detailTab, items[0].ID, "Log"))
	assert.Equal(t, "Service Log", m.detail().Tab.Name)
	assert.Len(t, m.detailStack, 2, "should be a doubly-nested drilldown")
}

// ---------------------------------------------------------------------------
// Drilldown hint tests
// ---------------------------------------------------------------------------

func TestDrilldownHint(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	tab := &Tab{Kind: tabProjects}
	spec := columnSpec{Title: "Quotes"}
	assert.Equal(t, drilldownArrow+" drill", m.drilldownHint(tab, spec))
}

func TestNavigateToLinkClosesDetailStack(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabVendors)

	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	require.NotEmpty(t, vendors)

	// Drill into vendor quotes.
	require.NoError(t, m.openVendorQuoteDetail(vendors[0].ID, vendors[0].Name))
	require.True(t, m.inDetail())

	// Follow the Project link from the detail view.
	link := &columnLink{TargetTab: tabProjects}
	require.NoError(t, m.navigateToLink(link, "01JTEST00000000000000001"))

	// Detail stack should be fully collapsed and we should be on Projects.
	assert.False(t, m.inDetail(), "detail stack should be closed after navigateToLink")
	assert.Equal(t, tabIndex(tabProjects), m.active)
}

// ---------------------------------------------------------------------------
// Document drilldown tests
// ---------------------------------------------------------------------------

func TestProjectDocumentDrilldown(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabProjects)

	require.NoError(t, m.openProjectDocumentDetail("01JTEST00000000000000007", "Kitchen Remodel"))
	require.True(t, m.inDetail())
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Projects")
	assert.Contains(t, m.detail().Breadcrumb, "Kitchen Remodel")
	assert.Contains(t, m.detail().Breadcrumb, tabDocuments.String())

	// Verify uses entity document column specs (no Entity column).
	specs := m.effectiveTab().Specs
	for _, s := range specs {
		assert.NotEqual(t, "Entity", s.Title,
			"project document detail should not include Entity column")
	}
}

func TestApplianceDocumentDrilldown(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.active = tabIndex(tabAppliances)

	require.NoError(t, m.openApplianceDocumentDetail("01JTEST00000000000000005", "Dishwasher"))
	require.True(t, m.inDetail())
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Appliances")
	assert.Contains(t, m.detail().Breadcrumb, "Dishwasher")
	assert.Contains(t, m.detail().Breadcrumb, tabDocuments.String())
}

func TestProjectDocumentHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newEntityDocumentHandler(data.DocumentEntityProject, "01JTEST00000000000000001")
	assert.Equal(t, formDocument, h.FormKind())
}

func TestApplianceDocumentHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newEntityDocumentHandler(data.DocumentEntityAppliance, "01JTEST00000000000000001")
	assert.Equal(t, formDocument, h.FormKind())
}

func TestDocumentHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newDocumentHandler()
	assert.Equal(t, formDocument, h.FormKind())
}

func TestEntityDocumentColumnSpecsNoEntity(t *testing.T) {
	t.Parallel()
	specs := entityDocumentColumnSpecs()
	for _, s := range specs {
		assert.NotEqual(t, "Entity", s.Title,
			"entity document specs should not include Entity column")
	}
}

func TestDocumentColumnSpecsIncludeEntity(t *testing.T) {
	t.Parallel()
	specs := documentColumnSpecs()
	var found bool
	for _, s := range specs {
		if s.Title == "Entity" {
			found = true
			break
		}
	}
	assert.True(t, found, "top-level document specs should include Entity column")
}

func TestOpenDetailForRow_ProjectDocuments(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabProjects)
	tab := m.activeTab()
	require.NotNil(t, tab)

	projects, err := m.store.ListProjects(false)
	require.NoError(t, err)
	require.NotEmpty(t, projects)

	require.NoError(t, m.openDetailForRow(tab, projects[0].ID, tabDocuments.String()))
	require.True(t, m.inDetail())
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
}

func TestOpenDetailForRow_ApplianceDocuments(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabAppliances)
	tab := m.activeTab()
	require.NotNil(t, tab)

	appliances, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	require.NotEmpty(t, appliances)

	require.NoError(t, m.openDetailForRow(tab, appliances[0].ID, tabDocuments.String()))
	require.True(t, m.inDetail())
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
}

func TestServiceLogDocumentHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newEntityDocumentHandler(data.DocumentEntityServiceLog, "01JTEST00000000000000001")
	assert.Equal(t, formDocument, h.FormKind())
}

func TestOpenDetailForRow_ServiceLogDocuments(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabMaintenance)

	// List maintenance items and drill into one's service log.
	items, err := m.store.ListMaintenance(false)
	require.NoError(t, err)
	require.NotEmpty(t, items)

	// Create a service log entry for the first maintenance item.
	entry := &data.ServiceLogEntry{
		MaintenanceItemID: items[0].ID,
		ServicedAt:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Notes:             "test entry for doc drilldown",
	}
	require.NoError(t, m.store.CreateServiceLog(entry, data.Vendor{}))

	require.NoError(t, m.openServiceLogDetail(items[0].ID, items[0].Name))
	require.True(t, m.inDetail())

	// Drill from the service log detail into documents.
	detailTab := m.effectiveTab()
	require.NotNil(t, detailTab)
	require.NoError(t, m.openDetailForRow(detailTab, entry.ID, tabDocuments.String()))
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, tabDocuments.String())
}

func TestServiceLogDocumentColumnSpecsHasDocsColumn(t *testing.T) {
	t.Parallel()
	specs := serviceLogColumnSpecs()
	last := specs[len(specs)-1]
	assert.Equal(t, tabDocuments.String(), last.Title)
	assert.Equal(t, cellDrilldown, last.Kind)
}

// ---------------------------------------------------------------------------
// Maintenance document drilldown tests
// ---------------------------------------------------------------------------

func TestMaintenanceColumnSpecsIncludeDocs(t *testing.T) {
	t.Parallel()
	specs := maintenanceColumnSpecs()
	last := specs[len(specs)-1]
	assert.Equal(t, tabDocuments.String(), last.Title)
	assert.Equal(t, cellDrilldown, last.Kind)
}

func TestMaintenanceDocumentHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newEntityDocumentHandler(data.DocumentEntityMaintenance, "01JTEST00000000000000001")
	assert.Equal(t, formDocument, h.FormKind())
}

func TestOpenDetailForRow_MaintenanceDocuments(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabMaintenance)
	tab := m.activeTab()
	require.NotNil(t, tab)

	items, err := m.store.ListMaintenance(false)
	require.NoError(t, err)
	require.NotEmpty(t, items)

	require.NoError(t, m.openDetailForRow(tab, items[0].ID, tabDocuments.String()))
	require.True(t, m.inDetail())
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Maintenance")
}

// ---------------------------------------------------------------------------
// Quote document drilldown tests
// ---------------------------------------------------------------------------

func TestQuoteColumnSpecsIncludeDocs(t *testing.T) {
	t.Parallel()
	specs := quoteColumnSpecs()
	last := specs[len(specs)-1]
	assert.Equal(t, tabDocuments.String(), last.Title)
	assert.Equal(t, cellDrilldown, last.Kind)
}

func TestQuoteDocumentHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newEntityDocumentHandler(data.DocumentEntityQuote, "01JTEST00000000000000001")
	assert.Equal(t, formDocument, h.FormKind())
}

func TestOpenDetailForRow_QuoteDocuments(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabQuotes)
	tab := m.activeTab()
	require.NotNil(t, tab)

	quotes, err := m.store.ListQuotes(false)
	require.NoError(t, err)
	require.NotEmpty(t, quotes)

	require.NoError(t, m.openDetailForRow(tab, quotes[0].ID, tabDocuments.String()))
	require.True(t, m.inDetail())
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Quotes")
}

// ---------------------------------------------------------------------------
// Vendor document drilldown tests
// ---------------------------------------------------------------------------

func TestVendorColumnSpecsIncludeDocs(t *testing.T) {
	t.Parallel()
	specs := vendorColumnSpecs()
	last := specs[len(specs)-1]
	assert.Equal(t, tabDocuments.String(), last.Title)
	assert.Equal(t, cellDrilldown, last.Kind)
}

func TestVendorDocumentHandlerFormKind(t *testing.T) {
	t.Parallel()
	h := newEntityDocumentHandler(data.DocumentEntityVendor, "01JTEST00000000000000001")
	assert.Equal(t, formDocument, h.FormKind())
}

func TestOpenDetailForRow_VendorDocuments(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabVendors)
	tab := m.activeTab()
	require.NotNil(t, tab)

	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	require.NotEmpty(t, vendors)

	require.NoError(t, m.openDetailForRow(tab, vendors[0].ID, tabDocuments.String()))
	require.True(t, m.inDetail())
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Vendors")
}

// ---------------------------------------------------------------------------
// Nested document drilldown routing tests
// ---------------------------------------------------------------------------

func TestNestedApplianceMaintenanceDocuments(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabAppliances)

	appliances, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	require.NotEmpty(t, appliances)

	// Find an appliance with maintenance items.
	var applianceID string
	var items []data.MaintenanceItem
	for _, a := range appliances {
		items, err = m.store.ListMaintenanceByAppliance(a.ID, false)
		require.NoError(t, err)
		if len(items) > 0 {
			applianceID = a.ID
			break
		}
	}
	require.NotEmpty(t, items, "demo data must have at least one appliance with maintenance")

	// Level 1: Appliance → Maintenance
	tab := m.activeTab()
	require.NoError(t, m.openDetailForRow(tab, applianceID, "Maint"))
	require.Len(t, m.detailStack, 1)

	// Reload so rows are populated.
	require.NoError(t, m.reloadDetailTab())

	// Level 2: Maintenance item → Documents (should use maintenanceDocumentDef,
	// not applianceDocumentDef, even though tab.Kind == tabAppliances).
	detailTab := &m.detail().Tab
	require.NoError(t, m.openDetailForRow(detailTab, items[0].ID, tabDocuments.String()))
	require.Len(t, m.detailStack, 2)
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Maintenance")
}

func TestNestedVendorQuoteDocuments(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabVendors)

	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	require.NotEmpty(t, vendors)

	projects, err := m.store.ListProjects(false)
	require.NoError(t, err)
	require.NotEmpty(t, projects)

	// Explicitly create a quote so the test doesn't depend on demo data randomness.
	vendor := vendors[0]
	quote := data.Quote{ProjectID: projects[0].ID, TotalCents: 10000}
	require.NoError(t, m.store.CreateQuote(&quote, vendor))

	// Level 1: Vendor → Quotes
	tab := m.activeTab()
	require.NoError(t, m.openDetailForRow(tab, vendor.ID, tabQuotes.String()))
	require.Len(t, m.detailStack, 1)

	require.NoError(t, m.reloadDetailTab())

	// Level 2: Quote → Documents (should use quoteDocumentDef, not vendorDocumentDef).
	detailTab := &m.detail().Tab
	require.NoError(t, m.openDetailForRow(detailTab, quote.ID, tabDocuments.String()))
	require.Len(t, m.detailStack, 2)
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Quotes")
}

func TestNestedProjectQuoteDocuments(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.active = tabIndex(tabProjects)

	projects, err := m.store.ListProjects(false)
	require.NoError(t, err)
	require.NotEmpty(t, projects)

	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	require.NotEmpty(t, vendors)

	// Explicitly create a quote so the test doesn't depend on demo data randomness.
	project := projects[0]
	quote := data.Quote{ProjectID: project.ID, TotalCents: 10000}
	require.NoError(t, m.store.CreateQuote(&quote, vendors[0]))

	// Level 1: Project → Quotes
	tab := m.activeTab()
	require.NoError(t, m.openDetailForRow(tab, project.ID, tabQuotes.String()))
	require.Len(t, m.detailStack, 1)

	require.NoError(t, m.reloadDetailTab())

	// Level 2: Quote → Documents (should use quoteDocumentDef, not projectDocumentDef).
	detailTab := &m.detail().Tab
	require.NoError(t, m.openDetailForRow(detailTab, quote.ID, tabDocuments.String()))
	require.Len(t, m.detailStack, 2)
	assert.Equal(t, tabDocuments.String(), m.detail().Tab.Name)
	assert.Contains(t, m.detail().Breadcrumb, "Quotes")
}
