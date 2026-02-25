// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/cpcloud/micasa/internal/data"
)

// baseTableKeyMap returns the default table KeyMap with b/f removed from
// page-up/page-down so those keys can be used for tab navigation.
func baseTableKeyMap() table.KeyMap {
	km := table.DefaultKeyMap()
	km.PageDown.SetKeys(keyPgDown)
	km.PageDown.SetHelp(keyPgDown, "page down")
	km.PageUp.SetKeys(keyPgUp)
	km.PageUp.SetHelp(keyPgUp, "page up")
	return km
}

// normalTableKeyMap returns the table KeyMap for normal (nav) mode.
func normalTableKeyMap() table.KeyMap {
	return baseTableKeyMap()
}

// editTableKeyMap returns a table KeyMap with d/u stripped from half-page
// bindings so they can be used for delete/undo without conflicting.
func editTableKeyMap() table.KeyMap {
	km := baseTableKeyMap()
	km.HalfPageDown.SetKeys(keyCtrlD)
	km.HalfPageDown.SetHelp(keyCtrlD, "½ page down")
	km.HalfPageUp.SetKeys(keyCtrlU)
	km.HalfPageUp.SetHelp(keyCtrlU, "½ page up")
	return km
}

// setAllTableKeyMaps applies a KeyMap to every tab's table.
func (m *Model) setAllTableKeyMaps(km table.KeyMap) {
	for i := range m.tabs {
		m.tabs[i].Table.KeyMap = km
	}
	if dc := m.detail(); dc != nil {
		dc.Tab.Table.KeyMap = km
	}
}

func NewTabs() []Tab {
	projectSpecs := projectColumnSpecs()
	quoteSpecs := quoteColumnSpecs()
	maintenanceSpecs := maintenanceColumnSpecs()
	incidentSpecs := incidentColumnSpecs()
	applianceSpecs := applianceColumnSpecs()
	vendorSpecs := vendorColumnSpecs()
	documentSpecs := documentColumnSpecs()
	return []Tab{
		{
			Kind:    tabProjects,
			Name:    "Projects",
			Handler: projectHandler{},
			Specs:   projectSpecs,
			Table:   newTable(specsToColumns(projectSpecs)),
		},
		{
			Kind:    tabQuotes,
			Name:    tabQuotes.String(),
			Handler: quoteHandler{},
			Specs:   quoteSpecs,
			Table:   newTable(specsToColumns(quoteSpecs)),
		},
		{
			Kind:    tabMaintenance,
			Name:    "Maintenance",
			Handler: maintenanceHandler{},
			Specs:   maintenanceSpecs,
			Table:   newTable(specsToColumns(maintenanceSpecs)),
		},
		{
			Kind:        tabIncidents,
			Name:        tabIncidents.String(),
			Handler:     incidentHandler{},
			Specs:       incidentSpecs,
			Table:       newTable(specsToColumns(incidentSpecs)),
			ShowDeleted: true,
		},
		{
			Kind:    tabAppliances,
			Name:    "Appliances",
			Handler: applianceHandler{},
			Specs:   applianceSpecs,
			Table:   newTable(specsToColumns(applianceSpecs)),
		},
		{
			Kind:    tabVendors,
			Name:    "Vendors",
			Handler: vendorHandler{},
			Specs:   vendorSpecs,
			Table:   newTable(specsToColumns(vendorSpecs)),
		},
		{
			Kind:    tabDocuments,
			Name:    tabDocuments.String(),
			Handler: documentHandler{},
			Specs:   documentSpecs,
			Table:   newTable(specsToColumns(documentSpecs)),
		},
	}
}

type projectCol int

const (
	projectColID projectCol = iota
	projectColType
	projectColTitle
	projectColStatus
	projectColBudget
	projectColActual
	projectColStart
	projectColEnd
	projectColQuotes
	projectColDocs
)

func projectColumnSpecs() []columnSpec {
	return []columnSpec{
		{Title: "ID", Min: 4, Max: 6, Align: alignRight, Kind: cellReadonly},
		{Title: "Type", Min: 8, Max: 14, Flex: true},
		{Title: "Title", Min: 14, Max: 32, Flex: true},
		{Title: "Status", Min: 6, Max: 8, Kind: cellStatus},
		{Title: "Budget", Min: 10, Max: 14, Align: alignRight, Kind: cellMoney},
		{Title: "Actual", Min: 10, Max: 14, Align: alignRight, Kind: cellMoney},
		{Title: "Start", Min: 10, Max: 12, Kind: cellDate},
		{Title: "End", Min: 10, Max: 12, Kind: cellDate},
		{Title: tabQuotes.String(), Min: 6, Max: 8, Align: alignRight, Kind: cellDrilldown},
		{Title: tabDocuments.String(), Min: 5, Max: 6, Align: alignRight, Kind: cellDrilldown},
	}
}

type quoteCol int

const (
	quoteColID quoteCol = iota
	quoteColProject
	quoteColVendor
	quoteColTotal
	quoteColLabor
	quoteColMat
	quoteColOther
	quoteColRecv
	quoteColDocs
)

func quoteColumnSpecs() []columnSpec {
	return []columnSpec{
		{Title: "ID", Min: 4, Max: 6, Align: alignRight, Kind: cellReadonly},
		{
			Title: "Project",
			Min:   12,
			Max:   24,
			Flex:  true,
			Link:  &columnLink{TargetTab: tabProjects},
		},
		{
			Title: "Vendor",
			Min:   12,
			Max:   20,
			Flex:  true,
			Link:  &columnLink{TargetTab: tabVendors},
		},
		{Title: "Total", Min: 10, Max: 14, Align: alignRight, Kind: cellMoney},
		{Title: "Labor", Min: 10, Max: 14, Align: alignRight, Kind: cellMoney},
		{Title: "Mat", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
		{Title: "Other", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
		{Title: "Recv", Min: 10, Max: 12, Kind: cellDate},
		{Title: tabDocuments.String(), Min: 5, Max: 6, Align: alignRight, Kind: cellDrilldown},
	}
}

type maintenanceCol int

const (
	maintenanceColID maintenanceCol = iota
	maintenanceColItem
	maintenanceColCategory
	maintenanceColAppliance
	maintenanceColLast
	maintenanceColNext
	maintenanceColEvery
	maintenanceColLog
	maintenanceColDocs
)

func maintenanceColumnSpecs() []columnSpec {
	return []columnSpec{
		{Title: "ID", Min: 4, Max: 6, Align: alignRight, Kind: cellReadonly},
		{Title: "Item", Min: 12, Max: 26, Flex: true},
		{Title: "Category", Min: 10, Max: 14},
		{
			Title: "Appliance",
			Min:   10,
			Max:   18,
			Flex:  true,
			Link:  &columnLink{TargetTab: tabAppliances},
		},
		{Title: "Last", Min: 10, Max: 12, Kind: cellDate},
		{Title: "Next", Min: 10, Max: 12, Kind: cellUrgency},
		{Title: "Every", Min: 6, Max: 10},
		{Title: "Log", Min: 4, Max: 6, Align: alignRight, Kind: cellDrilldown},
		{Title: tabDocuments.String(), Min: 5, Max: 6, Align: alignRight, Kind: cellDrilldown},
	}
}

type incidentCol int

const (
	incidentColID incidentCol = iota
	incidentColTitle
	incidentColStatus
	incidentColSeverity
	incidentColLocation
	incidentColAppliance
	incidentColVendor
	incidentColNoticed
	incidentColResolved
	incidentColCost
	incidentColDocs
)

func incidentColumnSpecs() []columnSpec {
	return []columnSpec{
		{Title: "ID", Min: 4, Max: 6, Align: alignRight, Kind: cellReadonly},
		{Title: "Title", Min: 14, Max: 32, Flex: true},
		{Title: "Status", Min: 6, Max: 12, Kind: cellStatus},
		{Title: "Severity", Min: 6, Max: 10, Kind: cellStatus},
		{Title: "Location", Min: 8, Max: 16, Flex: true},
		{
			Title: "Appliance",
			Min:   10,
			Max:   18,
			Flex:  true,
			Link:  &columnLink{TargetTab: tabAppliances},
		},
		{
			Title: "Vendor",
			Min:   10,
			Max:   18,
			Flex:  true,
			Link:  &columnLink{TargetTab: tabVendors},
		},
		{Title: "Noticed", Min: 10, Max: 12, Kind: cellDate},
		{Title: "Resolved", Min: 10, Max: 12, Kind: cellDate},
		{Title: "Cost", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
		{Title: tabDocuments.String(), Min: 5, Max: 6, Align: alignRight, Kind: cellDrilldown},
	}
}

func incidentRows(
	items []data.Incident,
	docCounts map[uint]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(items, func(inc data.Incident) rowSpec {
		var appCell cell
		if inc.ApplianceID != nil {
			appCell = cell{Value: inc.Appliance.Name, Kind: cellText, LinkID: *inc.ApplianceID}
		} else {
			appCell = cell{Kind: cellText, Null: true}
		}
		var vendorCell cell
		if inc.VendorID != nil {
			vendorCell = cell{Value: inc.Vendor.Name, Kind: cellText, LinkID: *inc.VendorID}
		} else {
			vendorCell = cell{Kind: cellText, Null: true}
		}
		docCount := "0"
		if n := docCounts[inc.ID]; n > 0 {
			docCount = fmt.Sprintf("%d", n)
		}
		return rowSpec{
			ID:      inc.ID,
			Deleted: inc.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", inc.ID), Kind: cellReadonly},
				{Value: inc.Title, Kind: cellText},
				{Value: inc.Status, Kind: cellStatus},
				{Value: inc.Severity, Kind: cellStatus},
				{Value: inc.Location, Kind: cellText},
				appCell,
				vendorCell,
				{Value: inc.DateNoticed.Format(data.DateLayout), Kind: cellDate},
				dateCell(inc.DateResolved, cellDate),
				centsCell(inc.CostCents),
				{Value: docCount, Kind: cellDrilldown},
			},
		}
	})
}

type applianceCol int

const (
	applianceColID applianceCol = iota
	applianceColName
	applianceColBrand
	applianceColModel
	applianceColSerial
	applianceColLocation
	applianceColPurchased
	applianceColAge
	applianceColWarranty
	applianceColCost
	applianceColMaint
	applianceColDocs
)

func applianceColumnSpecs() []columnSpec {
	return []columnSpec{
		{Title: "ID", Min: 4, Max: 6, Align: alignRight, Kind: cellReadonly},
		{Title: "Name", Min: 12, Max: 24, Flex: true},
		{Title: "Brand", Min: 8, Max: 16, Flex: true},
		{Title: "Model", Min: 8, Max: 16},
		{Title: "Serial", Min: 8, Max: 14},
		{Title: "Location", Min: 8, Max: 14},
		{Title: "Purchased", Min: 10, Max: 12, Kind: cellDate},
		{Title: "Age", Min: 5, Max: 8, Kind: cellReadonly},
		{Title: "Warranty", Min: 10, Max: 12, Kind: cellWarranty},
		{Title: "Cost", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
		{Title: "Maint", Min: 5, Max: 6, Align: alignRight, Kind: cellDrilldown},
		{Title: tabDocuments.String(), Min: 5, Max: 6, Align: alignRight, Kind: cellDrilldown},
	}
}

// withoutColumn returns a copy of specs with the named column removed.
func withoutColumn(specs []columnSpec, title string) []columnSpec {
	out := make([]columnSpec, 0, len(specs)-1)
	for _, s := range specs {
		if s.Title != title {
			out = append(out, s)
		}
	}
	return out
}

func applianceMaintenanceColumnSpecs() []columnSpec {
	return withoutColumn(maintenanceColumnSpecs(), "Appliance")
}

func applianceMaintenanceRows(
	items []data.MaintenanceItem,
	logCounts map[uint]int,
	docCounts map[uint]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(items, func(item data.MaintenanceItem) rowSpec {
		interval := maintenanceIntervalText(item)
		logCount := "0"
		if n := logCounts[item.ID]; n > 0 {
			logCount = fmt.Sprintf("%d", n)
		}
		docCount := "0"
		if n := docCounts[item.ID]; n > 0 {
			docCount = fmt.Sprintf("%d", n)
		}
		nextDue := data.ComputeNextDue(item.LastServicedAt, item.IntervalMonths, item.DueDate)
		return rowSpec{
			ID:      item.ID,
			Deleted: item.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", item.ID), Kind: cellReadonly},
				{Value: item.Name, Kind: cellText},
				{Value: item.Category.Name, Kind: cellText},
				dateCell(item.LastServicedAt, cellDate),
				dateCell(nextDue, cellUrgency),
				{Value: interval, Kind: cellText},
				{Value: logCount, Kind: cellDrilldown},
				{Value: docCount, Kind: cellDrilldown},
			},
		}
	})
}

type serviceLogCol int

const (
	serviceLogColID serviceLogCol = iota
	serviceLogColDate
	serviceLogColPerformedBy
	serviceLogColCost
	serviceLogColNotes
	serviceLogColDocs
)

func serviceLogColumnSpecs() []columnSpec {
	return []columnSpec{
		{Title: "ID", Min: 4, Max: 6, Align: alignRight, Kind: cellReadonly},
		{Title: "Date", Min: 10, Max: 12, Kind: cellDate},
		{
			Title: "Performed By",
			Min:   12,
			Max:   22,
			Flex:  true,
			Link:  &columnLink{TargetTab: tabVendors},
		},
		{Title: "Cost", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
		{Title: "Notes", Min: 12, Max: 40, Flex: true, Kind: cellNotes},
		{Title: tabDocuments.String(), Min: 5, Max: 8, Align: alignRight, Kind: cellDrilldown},
	}
}

func serviceLogRows(
	entries []data.ServiceLogEntry,
	docCounts map[uint]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(entries, func(e data.ServiceLogEntry) rowSpec {
		performedBy := "Self"
		var vendorLinkID uint
		if e.VendorID != nil && e.Vendor.Name != "" {
			performedBy = e.Vendor.Name
			vendorLinkID = *e.VendorID
		}
		docCount := "0"
		if n := docCounts[e.ID]; n > 0 {
			docCount = fmt.Sprintf("%d", n)
		}
		return rowSpec{
			ID:      e.ID,
			Deleted: e.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", e.ID), Kind: cellReadonly},
				{Value: e.ServicedAt.Format(data.DateLayout), Kind: cellDate},
				{Value: performedBy, Kind: cellText, LinkID: vendorLinkID},
				centsCell(e.CostCents),
				{Value: e.Notes, Kind: cellNotes},
				{Value: docCount, Kind: cellDrilldown},
			},
		}
	})
}

func applianceRows(
	items []data.Appliance,
	maintCounts map[uint]int,
	docCounts map[uint]int,
	now time.Time,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(items, func(a data.Appliance) rowSpec {
		maintCount := "0"
		if n := maintCounts[a.ID]; n > 0 {
			maintCount = fmt.Sprintf("%d", n)
		}
		docCount := "0"
		if n := docCounts[a.ID]; n > 0 {
			docCount = fmt.Sprintf("%d", n)
		}
		ageCell := cell{Kind: cellReadonly, Null: a.PurchaseDate == nil}
		if a.PurchaseDate != nil {
			ageCell.Value = applianceAge(a.PurchaseDate, now)
		}
		return rowSpec{
			ID:      a.ID,
			Deleted: a.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", a.ID), Kind: cellReadonly},
				{Value: a.Name, Kind: cellText},
				{Value: a.Brand, Kind: cellText},
				{Value: a.ModelNumber, Kind: cellText},
				{Value: a.SerialNumber, Kind: cellText},
				{Value: a.Location, Kind: cellText},
				dateCell(a.PurchaseDate, cellDate),
				ageCell,
				dateCell(a.WarrantyExpiry, cellWarranty),
				centsCell(a.CostCents),
				{Value: maintCount, Kind: cellDrilldown},
				{Value: docCount, Kind: cellDrilldown},
			},
		}
	})
}

// formatInterval returns a compact interval string: "3m", "1y", "2y 6m".
// Returns empty for non-positive values.
// maintenanceIntervalText returns the display value for the "Every" column.
// Items with an explicit due date show "--" (no recurring interval).
func maintenanceIntervalText(item data.MaintenanceItem) string {
	if item.DueDate != nil {
		return "--"
	}
	return formatInterval(item.IntervalMonths)
}

func formatInterval(months int) string {
	if months <= 0 {
		return ""
	}
	y := months / 12
	m := months % 12
	if y == 0 {
		return fmt.Sprintf("%dm", m)
	}
	if m == 0 {
		return fmt.Sprintf("%dy", y)
	}
	return fmt.Sprintf("%dy %dm", y, m)
}

// applianceAge returns a human-readable age string from purchase date to now.
func applianceAge(purchased *time.Time, now time.Time) string {
	if purchased == nil {
		return ""
	}
	years := now.Year() - purchased.Year()
	months := int(now.Month()) - int(purchased.Month())
	if now.Day() < purchased.Day() {
		months--
	}
	if months < 0 {
		years--
		months += 12
	}
	if years < 0 {
		return ""
	}
	if years == 0 {
		if months <= 0 {
			return "<1m"
		}
		return fmt.Sprintf("%dm", months)
	}
	if months == 0 {
		return fmt.Sprintf("%dy", years)
	}
	return fmt.Sprintf("%dy %dm", years, months)
}

type vendorCol int

const (
	vendorColID vendorCol = iota
	vendorColName
	vendorColContact
	vendorColEmail
	vendorColPhone
	vendorColWebsite
	vendorColQuotes
	vendorColJobs
	vendorColDocs
)

func vendorColumnSpecs() []columnSpec {
	return []columnSpec{
		{Title: "ID", Min: 4, Max: 6, Align: alignRight, Kind: cellReadonly},
		{Title: "Name", Min: 14, Max: 24, Flex: true},
		{Title: "Contact", Min: 10, Max: 20, Flex: true},
		{Title: "Email", Min: 12, Max: 24, Flex: true},
		{Title: "Phone", Min: 12, Max: 16},
		{Title: "Website", Min: 12, Max: 28, Flex: true},
		{Title: tabQuotes.String(), Min: 6, Max: 8, Align: alignRight, Kind: cellDrilldown},
		{Title: "Jobs", Min: 5, Max: 8, Align: alignRight, Kind: cellDrilldown},
		{Title: tabDocuments.String(), Min: 5, Max: 6, Align: alignRight, Kind: cellDrilldown},
	}
}

func vendorRows(
	vendors []data.Vendor,
	quoteCounts map[uint]int,
	jobCounts map[uint]int,
	docCounts map[uint]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(vendors, func(v data.Vendor) rowSpec {
		quoteCount := "0"
		if n := quoteCounts[v.ID]; n > 0 {
			quoteCount = fmt.Sprintf("%d", n)
		}
		jobCount := "0"
		if n := jobCounts[v.ID]; n > 0 {
			jobCount = fmt.Sprintf("%d", n)
		}
		docCount := "0"
		if n := docCounts[v.ID]; n > 0 {
			docCount = fmt.Sprintf("%d", n)
		}
		return rowSpec{
			ID:      v.ID,
			Deleted: v.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", v.ID), Kind: cellReadonly},
				{Value: v.Name, Kind: cellText},
				{Value: v.ContactName, Kind: cellText},
				{Value: v.Email, Kind: cellText},
				{Value: v.Phone, Kind: cellText},
				{Value: v.Website, Kind: cellText},
				{Value: quoteCount, Kind: cellDrilldown},
				{Value: jobCount, Kind: cellDrilldown},
				{Value: docCount, Kind: cellDrilldown},
			},
		}
	})
}

func specsToColumns(specs []columnSpec) []table.Column {
	cols := make([]table.Column, 0, len(specs))
	for _, spec := range specs {
		width := spec.Min
		if width <= 0 {
			width = 6
		}
		cols = append(cols, table.Column{Title: spec.Title, Width: width})
	}
	return cols
}

func newTable(columns []table.Column) table.Model {
	tbl := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
	)
	tbl.SetStyles(table.Styles{
		Header:   appStyles.TableHeader,
		Selected: appStyles.TableSelected,
	})
	return tbl
}

func projectRows(
	projects []data.Project,
	quoteCounts map[uint]int,
	docCounts map[uint]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(projects, func(p data.Project) rowSpec {
		quoteCount := "0"
		if n := quoteCounts[p.ID]; n > 0 {
			quoteCount = fmt.Sprintf("%d", n)
		}
		docCount := "0"
		if n := docCounts[p.ID]; n > 0 {
			docCount = fmt.Sprintf("%d", n)
		}
		return rowSpec{
			ID:      p.ID,
			Deleted: p.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", p.ID), Kind: cellReadonly},
				{Value: p.ProjectType.Name, Kind: cellText},
				{Value: p.Title, Kind: cellText},
				{Value: p.Status, Kind: cellStatus},
				centsCell(p.BudgetCents),
				centsCell(p.ActualCents),
				dateCell(p.StartDate, cellDate),
				dateCell(p.EndDate, cellDate),
				{Value: quoteCount, Kind: cellDrilldown},
				{Value: docCount, Kind: cellDrilldown},
			},
		}
	})
}

func quoteRows(
	quotes []data.Quote,
	docCounts map[uint]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(quotes, func(q data.Quote) rowSpec {
		projectName := q.Project.Title
		if projectName == "" {
			projectName = fmt.Sprintf("Project %d", q.ProjectID)
		}
		docCount := "0"
		if n := docCounts[q.ID]; n > 0 {
			docCount = fmt.Sprintf("%d", n)
		}
		return rowSpec{
			ID:      q.ID,
			Deleted: q.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", q.ID), Kind: cellReadonly},
				{Value: projectName, Kind: cellText, LinkID: q.ProjectID},
				{Value: q.Vendor.Name, Kind: cellText, LinkID: q.VendorID},
				{Value: data.FormatCents(q.TotalCents), Kind: cellMoney},
				centsCell(q.LaborCents),
				centsCell(q.MaterialsCents),
				centsCell(q.OtherCents),
				dateCell(q.ReceivedDate, cellDate),
				{Value: docCount, Kind: cellDrilldown},
			},
		}
	})
}

func maintenanceRows(
	items []data.MaintenanceItem,
	logCounts map[uint]int,
	docCounts map[uint]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(items, func(item data.MaintenanceItem) rowSpec {
		interval := maintenanceIntervalText(item)
		var appCell cell
		if item.ApplianceID != nil {
			appCell = cell{Value: item.Appliance.Name, Kind: cellText, LinkID: *item.ApplianceID}
		} else {
			appCell = cell{Kind: cellText, Null: true}
		}
		logCount := "0"
		if n := logCounts[item.ID]; n > 0 {
			logCount = fmt.Sprintf("%d", n)
		}
		docCount := "0"
		if n := docCounts[item.ID]; n > 0 {
			docCount = fmt.Sprintf("%d", n)
		}
		nextDue := data.ComputeNextDue(item.LastServicedAt, item.IntervalMonths, item.DueDate)
		return rowSpec{
			ID:      item.ID,
			Deleted: item.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", item.ID), Kind: cellReadonly},
				{Value: item.Name, Kind: cellText},
				{Value: item.Category.Name, Kind: cellText},
				appCell,
				dateCell(item.LastServicedAt, cellDate),
				dateCell(nextDue, cellUrgency),
				{Value: interval, Kind: cellText},
				{Value: logCount, Kind: cellDrilldown},
				{Value: docCount, Kind: cellDrilldown},
			},
		}
	})
}

func cellsToRow(cells []cell) table.Row {
	row := make(table.Row, len(cells))
	for i, cell := range cells {
		row[i] = cell.Value
	}
	return row
}

// rowSpec describes one table row from an entity.
type rowSpec struct {
	ID      uint
	Deleted bool
	Cells   []cell
}

// entityIDs extracts the ID from each element using the given accessor.
func entityIDs[T any](items []T, id func(T) uint) []uint {
	ids := make([]uint, len(items))
	for i, item := range items {
		ids[i] = id(item)
	}
	return ids
}

// buildRows converts a slice of entities into the three parallel slices that
// the table and sort systems consume. The toRow function maps each entity to
// its ID, deletion status, and cell values.
func buildRows[T any](items []T, toRow func(T) rowSpec) ([]table.Row, []rowMeta, [][]cell) {
	rows := make([]table.Row, 0, len(items))
	meta := make([]rowMeta, 0, len(items))
	cells := make([][]cell, 0, len(items))
	for _, item := range items {
		spec := toRow(item)
		rows = append(rows, cellsToRow(spec.Cells))
		cells = append(cells, spec.Cells)
		meta = append(meta, rowMeta{ID: spec.ID, Deleted: spec.Deleted})
	}
	return rows, meta, cells
}

// vendorQuoteColumnSpecs defines the columns for quotes scoped to a vendor.
// Omits the Vendor column since the parent context provides that.
func vendorQuoteColumnSpecs() []columnSpec {
	return withoutColumn(quoteColumnSpecs(), "Vendor")
}

func vendorQuoteRows(
	quotes []data.Quote,
	docCounts map[uint]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(quotes, func(q data.Quote) rowSpec {
		projectName := q.Project.Title
		if projectName == "" {
			projectName = fmt.Sprintf("Project %d", q.ProjectID)
		}
		docCount := "0"
		if n := docCounts[q.ID]; n > 0 {
			docCount = fmt.Sprintf("%d", n)
		}
		return rowSpec{
			ID:      q.ID,
			Deleted: q.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", q.ID), Kind: cellReadonly},
				{Value: projectName, Kind: cellText, LinkID: q.ProjectID},
				{Value: data.FormatCents(q.TotalCents), Kind: cellMoney},
				centsCell(q.LaborCents),
				centsCell(q.MaterialsCents),
				centsCell(q.OtherCents),
				dateCell(q.ReceivedDate, cellDate),
				{Value: docCount, Kind: cellDrilldown},
			},
		}
	})
}

type vendorJobsCol int

const (
	vendorJobsColID vendorJobsCol = iota
	vendorJobsColItem
	vendorJobsColDate
	vendorJobsColCost
	vendorJobsColNotes
)

// vendorJobsColumnSpecs defines the columns for service log entries scoped to
// a vendor. Omits the Vendor column since the parent context provides that.
func vendorJobsColumnSpecs() []columnSpec {
	return []columnSpec{
		{Title: "ID", Min: 4, Max: 6, Align: alignRight, Kind: cellReadonly},
		{
			Title: "Item",
			Min:   12,
			Max:   24,
			Flex:  true,
			Link:  &columnLink{TargetTab: tabMaintenance},
		},
		{Title: "Date", Min: 10, Max: 12, Kind: cellDate},
		{Title: "Cost", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
		{Title: "Notes", Min: 12, Max: 40, Flex: true, Kind: cellNotes},
	}
}

func vendorJobsRows(
	entries []data.ServiceLogEntry,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(entries, func(e data.ServiceLogEntry) rowSpec {
		itemName := e.MaintenanceItem.Name
		return rowSpec{
			ID:      e.ID,
			Deleted: e.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", e.ID), Kind: cellReadonly},
				{Value: itemName, Kind: cellText, LinkID: e.MaintenanceItemID},
				{Value: e.ServicedAt.Format(data.DateLayout), Kind: cellDate},
				centsCell(e.CostCents),
				{Value: e.Notes, Kind: cellNotes},
			},
		}
	})
}

func projectQuoteColumnSpecs() []columnSpec {
	return withoutColumn(quoteColumnSpecs(), "Project")
}

func projectQuoteRows(
	quotes []data.Quote,
	docCounts map[uint]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(quotes, func(q data.Quote) rowSpec {
		docCount := "0"
		if n := docCounts[q.ID]; n > 0 {
			docCount = fmt.Sprintf("%d", n)
		}
		return rowSpec{
			ID:      q.ID,
			Deleted: q.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", q.ID), Kind: cellReadonly},
				{Value: q.Vendor.Name, Kind: cellText, LinkID: q.VendorID},
				{Value: data.FormatCents(q.TotalCents), Kind: cellMoney},
				centsCell(q.LaborCents),
				centsCell(q.MaterialsCents),
				centsCell(q.OtherCents),
				dateCell(q.ReceivedDate, cellDate),
				{Value: docCount, Kind: cellDrilldown},
			},
		}
	})
}

func centsValue(cents *int64) string {
	if cents == nil {
		return ""
	}
	return data.FormatCents(*cents)
}

func dateValue(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(data.DateLayout)
}

// centsCell returns a cell for an optional money value. NULL pointer produces
// a null cell; non-nil produces a formatted money cell.
func centsCell(cents *int64) cell {
	if cents == nil {
		return cell{Kind: cellMoney, Null: true}
	}
	return cell{Value: data.FormatCents(*cents), Kind: cellMoney}
}

// dateCell returns a cell for an optional date value. NULL pointer produces
// a null cell with the given kind; non-nil produces a formatted date cell.
func dateCell(value *time.Time, kind cellKind) cell {
	if value == nil {
		return cell{Kind: kind, Null: true}
	}
	return cell{Value: value.Format(data.DateLayout), Kind: kind}
}

// nullTextCell returns a null cell when value is empty (representing a NULL FK
// or optional field), otherwise a normal text cell.
func nullTextCell(value string, linkID uint) cell {
	if value == "" && linkID == 0 {
		return cell{Kind: cellText, Null: true}
	}
	return cell{Value: value, Kind: cellText, LinkID: linkID}
}

type documentCol int

const (
	documentColID documentCol = iota
	documentColTitle
	documentColEntity
	documentColType
	documentColSize
	documentColNotes
	documentColUpdated
)

// documentColumnSpecs defines columns for the top-level Documents tab.
func documentColumnSpecs() []columnSpec {
	return []columnSpec{
		{Title: "ID", Min: 4, Max: 6, Align: alignRight, Kind: cellReadonly},
		{Title: "Title", Min: 14, Max: 32, Flex: true},
		{Title: "Entity", Min: 10, Max: 24, Flex: true, Kind: cellEntity},
		{Title: "Type", Min: 8, Max: 16},
		{Title: "Size", Min: 6, Max: 10, Align: alignRight, Kind: cellReadonly},
		{Title: "Notes", Min: 12, Max: 40, Flex: true, Kind: cellNotes},
		{Title: "Updated", Min: 10, Max: 12, Kind: cellReadonly},
	}
}

func entityDocumentColumnSpecs() []columnSpec {
	return withoutColumn(documentColumnSpecs(), "Entity")
}

// entityNameMap maps (kind, id) pairs to display names for document entities.
type entityNameMap map[entityRef]string

// buildEntityNameMap loads names for all entity types so the document table
// can display resolved labels instead of raw "kind #id".
func buildEntityNameMap(store *data.Store) entityNameMap {
	names := make(entityNameMap)

	if appliances, err := store.ListAppliances(false); err == nil {
		for _, a := range appliances {
			names[entityRef{Kind: data.DocumentEntityAppliance, ID: a.ID}] = a.Name
		}
	}
	if incidents, err := store.ListIncidents(false); err == nil {
		for _, inc := range incidents {
			names[entityRef{Kind: data.DocumentEntityIncident, ID: inc.ID}] = inc.Title
		}
	}
	if items, err := store.ListMaintenance(false); err == nil {
		for _, item := range items {
			names[entityRef{Kind: data.DocumentEntityMaintenance, ID: item.ID}] = item.Name
		}
	}
	if projects, err := store.ListProjects(false); err == nil {
		for _, p := range projects {
			names[entityRef{Kind: data.DocumentEntityProject, ID: p.ID}] = p.Title
		}
	}
	if quotes, err := store.ListQuotes(false); err == nil {
		for _, q := range quotes {
			names[entityRef{Kind: data.DocumentEntityQuote, ID: q.ID}] = fmt.Sprintf(
				"%s / %s",
				q.Project.Title,
				q.Vendor.Name,
			)
		}
	}
	if vendors, err := store.ListVendors(false); err == nil {
		for _, v := range vendors {
			names[entityRef{Kind: data.DocumentEntityVendor, ID: v.ID}] = v.Name
		}
	}

	return names
}

func documentRows(docs []data.Document, names entityNameMap) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(docs, func(d data.Document) rowSpec {
		return rowSpec{
			ID:      d.ID,
			Deleted: d.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", d.ID), Kind: cellReadonly},
				{Value: d.Title, Kind: cellText},
				{
					Value:  documentEntityLabel(d.EntityKind, d.EntityID, names),
					Kind:   cellEntity,
					LinkID: d.EntityID,
				},
				{Value: d.MIMEType, Kind: cellText},
				{Value: formatFileSize(docSizeBytes(d)), Kind: cellReadonly},
				{Value: d.Notes, Kind: cellNotes},
				{Value: d.UpdatedAt.Format(data.DateLayout), Kind: cellReadonly},
			},
		}
	})
}

func entityDocumentRows(docs []data.Document) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(docs, func(d data.Document) rowSpec {
		return rowSpec{
			ID:      d.ID,
			Deleted: d.DeletedAt.Valid,
			Cells: []cell{
				{Value: fmt.Sprintf("%d", d.ID), Kind: cellReadonly},
				{Value: d.Title, Kind: cellText},
				{Value: d.MIMEType, Kind: cellText},
				{Value: formatFileSize(docSizeBytes(d)), Kind: cellReadonly},
				{Value: d.Notes, Kind: cellNotes},
				{Value: d.UpdatedAt.Format(data.DateLayout), Kind: cellReadonly},
			},
		}
	})
}

// entityLetterTab maps the single-letter entity prefix to the tab it links to.
var entityLetterTab = map[byte]TabKind{
	'A': tabAppliances,
	'I': tabIncidents,
	'M': tabMaintenance,
	'P': tabProjects,
	'Q': tabQuotes,
	'V': tabVendors,
}

// entityLetterKind maps the single-letter entity prefix to the full kind name.
// Used by cellDisplayValue for kind-based pinning.
var entityLetterKind = map[byte]string{
	'A': data.DocumentEntityAppliance,
	'I': data.DocumentEntityIncident,
	'M': data.DocumentEntityMaintenance,
	'P': data.DocumentEntityProject,
	'Q': data.DocumentEntityQuote,
	'V': data.DocumentEntityVendor,
}

// entityKindLetter maps entity kind strings to a single-letter prefix used in
// the Entity column. Each letter is unique across all entity types.
var entityKindLetter = map[string]string{
	data.DocumentEntityAppliance:   "A",
	data.DocumentEntityIncident:    "I",
	data.DocumentEntityMaintenance: "M",
	data.DocumentEntityProject:     "P",
	data.DocumentEntityQuote:       "Q",
	data.DocumentEntityVendor:      "V",
}

// documentEntityLabel returns a label like "P Kitchen Reno" with a
// single-letter kind prefix, or falls back to "project #3" when
// the name map has no entry.
func documentEntityLabel(kind string, id uint, names entityNameMap) string {
	if kind == "" {
		return ""
	}
	letter, ok := entityKindLetter[kind]
	if !ok {
		return kind + " #" + fmt.Sprintf("%d", id)
	}
	if name, found := names[entityRef{Kind: kind, ID: id}]; found {
		return letter + " " + name
	}
	return letter + " #" + fmt.Sprintf("%d", id)
}

// docSizeBytes returns d.SizeBytes as uint64. The DB column is int64 but
// values are always non-negative since they come from os.FileInfo.Size().
func docSizeBytes(d data.Document) uint64 {
	return uint64(max(d.SizeBytes, 0)) //nolint:gosec // clamped to non-negative
}

// formatFileSize returns a human-readable file size string.
func formatFileSize(bytes uint64) string {
	const (
		kB = 1024
		mB = kB * 1024
		gB = mB * 1024
	)
	switch {
	case bytes >= gB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gB))
	case bytes >= mB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mB))
	case bytes >= kB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
