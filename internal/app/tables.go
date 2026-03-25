// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"charm.land/bubbles/v2/table"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/locale"
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

// editTableKeyMap returns a table KeyMap with d stripped from half-page-down
// so it can be used for delete without conflicting.
func editTableKeyMap() table.KeyMap {
	km := baseTableKeyMap()
	km.HalfPageDown.SetKeys(keyCtrlD)
	km.HalfPageDown.SetHelp(keyCtrlD, "½ page down")
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
			Handler: newProjectHandler(),
			Specs:   projectSpecs,
			Table:   newTable(specsToColumns(projectSpecs)),
		},
		{
			Kind:    tabQuotes,
			Name:    tabQuotes.String(),
			Handler: newQuoteHandler(),
			Specs:   quoteSpecs,
			Table:   newTable(specsToColumns(quoteSpecs)),
		},
		{
			Kind:    tabMaintenance,
			Name:    "Maintenance",
			Handler: newMaintenanceHandler(),
			Specs:   maintenanceSpecs,
			Table:   newTable(specsToColumns(maintenanceSpecs)),
		},
		{
			Kind:        tabIncidents,
			Name:        tabIncidents.String(),
			Handler:     newIncidentHandler(),
			Specs:       incidentSpecs,
			Table:       newTable(specsToColumns(incidentSpecs)),
			ShowDeleted: true,
		},
		{
			Kind:    tabAppliances,
			Name:    "Appliances",
			Handler: newApplianceHandler(),
			Specs:   applianceSpecs,
			Table:   newTable(specsToColumns(applianceSpecs)),
		},
		{
			Kind:    tabVendors,
			Name:    "Vendors",
			Handler: newVendorHandler(),
			Specs:   vendorSpecs,
			Table:   newTable(specsToColumns(vendorSpecs)),
		},
		{
			Kind:    tabDocuments,
			Name:    tabDocuments.String(),
			Handler: newDocumentHandler(),
			Specs:   documentSpecs,
			Table:   newTable(specsToColumns(documentSpecs)),
		},
	}
}

func incidentRows(
	items []data.Incident,
	docCounts map[string]int,
	cur locale.Currency,
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
		return rowSpec{
			ID:      inc.ID,
			Deleted: inc.DeletedAt.Valid,
			Cells: []cell{
				{Value: shortID(inc.ID), Kind: cellReadonly},
				{Value: inc.Title, Kind: cellText},
				{Value: inc.Status, Kind: cellStatus},
				{Value: inc.Severity, Kind: cellStatus},
				{Value: inc.Location, Kind: cellText},
				appCell,
				vendorCell,
				{Value: inc.DateNoticed.Format(data.DateLayout), Kind: cellDate},
				dateCell(inc.DateResolved, cellDate),
				centsCell(inc.CostCents, cur),
				{Value: countStr(docCounts, inc.ID), Kind: cellDrilldown},
			},
		}
	})
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
	logCounts map[string]int,
	docCounts map[string]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(items, func(item data.MaintenanceItem) rowSpec {
		intervalCell := maintenanceIntervalCell(item)
		nextDue := data.ComputeNextDue(item.LastServicedAt, item.IntervalMonths, item.DueDate)
		return rowSpec{
			ID:      item.ID,
			Deleted: item.DeletedAt.Valid,
			Cells: []cell{
				{Value: shortID(item.ID), Kind: cellReadonly},
				{Value: item.Name, Kind: cellText},
				{Value: item.Category.Name, Kind: cellText},
				maintenanceSeasonCell(item.Season),
				dateCell(item.LastServicedAt, cellDate),
				dateCell(nextDue, cellUrgency),
				intervalCell,
				{Value: countStr(logCounts, item.ID), Kind: cellDrilldown},
				{Value: countStr(docCounts, item.ID), Kind: cellDrilldown},
			},
		}
	})
}

func serviceLogRows(
	entries []data.ServiceLogEntry,
	docCounts map[string]int,
	cur locale.Currency,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(entries, func(e data.ServiceLogEntry) rowSpec {
		performedBy := "Self"
		var vendorLinkID string
		if e.VendorID != nil && e.Vendor.Name != "" {
			performedBy = e.Vendor.Name
			vendorLinkID = *e.VendorID
		}
		return rowSpec{
			ID:      e.ID,
			Deleted: e.DeletedAt.Valid,
			Cells: []cell{
				{Value: shortID(e.ID), Kind: cellReadonly},
				{Value: e.ServicedAt.Format(data.DateLayout), Kind: cellDate},
				{Value: performedBy, Kind: cellText, LinkID: vendorLinkID},
				centsCell(e.CostCents, cur),
				{Value: e.Notes, Kind: cellNotes},
				{Value: countStr(docCounts, e.ID), Kind: cellDrilldown},
			},
		}
	})
}

func applianceRows(
	items []data.Appliance,
	maintCounts map[string]int,
	docCounts map[string]int,
	now time.Time,
	cur locale.Currency,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(items, func(a data.Appliance) rowSpec {
		ageCell := cell{Kind: cellReadonly, Null: a.PurchaseDate == nil}
		if a.PurchaseDate != nil {
			ageCell.Value = applianceAge(a.PurchaseDate, now)
		}
		return rowSpec{
			ID:      a.ID,
			Deleted: a.DeletedAt.Valid,
			Cells: []cell{
				{Value: shortID(a.ID), Kind: cellReadonly},
				{Value: a.Name, Kind: cellText},
				{Value: a.Brand, Kind: cellText},
				{Value: a.ModelNumber, Kind: cellText},
				{Value: a.SerialNumber, Kind: cellText},
				{Value: a.Location, Kind: cellText},
				dateCell(a.PurchaseDate, cellDate),
				ageCell,
				dateCell(a.WarrantyExpiry, cellWarranty),
				centsCell(a.CostCents, cur),
				{Value: countStr(maintCounts, a.ID), Kind: cellDrilldown},
				{Value: countStr(docCounts, a.ID), Kind: cellDrilldown},
			},
		}
	})
}

// formatInterval returns a compact interval string: "3m", "1y", "2y 6m".
// Returns empty for non-positive values.
// maintenanceIntervalCell returns the cell for the "Every" column.
// Items with no interval return a NULL cell.
func maintenanceIntervalCell(item data.MaintenanceItem) cell {
	v := formatInterval(item.IntervalMonths)
	if v == "" {
		return cell{Kind: cellText, Null: true}
	}
	return cell{Value: v, Kind: cellText}
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

func vendorRows(
	vendors []data.Vendor,
	quoteCounts map[string]int,
	jobCounts map[string]int,
	docCounts map[string]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(vendors, func(v data.Vendor) rowSpec {
		return rowSpec{
			ID:      v.ID,
			Deleted: v.DeletedAt.Valid,
			Cells: []cell{
				{Value: shortID(v.ID), Kind: cellReadonly},
				{Value: v.Name, Kind: cellText},
				{Value: v.ContactName, Kind: cellText},
				{Value: v.Email, Kind: cellText},
				{Value: v.Phone, Kind: cellText},
				{Value: v.Website, Kind: cellText},
				{Value: countStr(quoteCounts, v.ID), Kind: cellDrilldown},
				{Value: countStr(jobCounts, v.ID), Kind: cellDrilldown},
				{Value: countStr(docCounts, v.ID), Kind: cellDrilldown},
			},
		}
	})
}

// countStr formats a count map value as a display string.
// Zero or absent entries render as "0".
func countStr(counts map[string]int, id string) string {
	if n := counts[id]; n > 0 {
		return fmt.Sprintf("%d", n)
	}
	return "0"
}

// shortID returns a truncated ULID for display (last 7 chars).
// The full ID is preserved in rowMeta.ID for lookups.
func shortID(id string) string {
	if len(id) <= 7 {
		return id
	}
	return id[len(id)-7:]
}

// idColumnSpec returns the standard ID column spec shared by all tables.
func idColumnSpec() columnSpec {
	return columnSpec{Title: "ID", Min: 7, Max: 7, Align: alignRight, Kind: cellReadonly}
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
		Header:   appStyles.TableHeader(),
		Selected: appStyles.TableSelected(),
	})
	return tbl
}

func projectRows(
	projects []data.Project,
	quoteCounts map[string]int,
	docCounts map[string]int,
	cur locale.Currency,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(projects, func(p data.Project) rowSpec {
		return rowSpec{
			ID:      p.ID,
			Deleted: p.DeletedAt.Valid,
			Cells: []cell{
				{Value: shortID(p.ID), Kind: cellReadonly},
				{Value: p.ProjectType.Name, Kind: cellText},
				{Value: p.Title, Kind: cellText},
				{Value: p.Status, Kind: cellStatus},
				centsCell(p.BudgetCents, cur),
				centsCell(p.ActualCents, cur),
				dateCell(p.StartDate, cellDate),
				dateCell(p.EndDate, cellDate),
				{Value: countStr(quoteCounts, p.ID), Kind: cellDrilldown},
				{Value: countStr(docCounts, p.ID), Kind: cellDrilldown},
			},
		}
	})
}

// quoteRowSpec builds the common rowSpec for a quote. includeProject/includeVendor
// control whether the context columns are included (top-level has both, detail
// views omit the parent's column).
func quoteRowSpec(
	q data.Quote,
	docCounts map[string]int,
	cur locale.Currency,
	includeProject, includeVendor bool,
) rowSpec {
	cells := make([]cell, 0, 9)
	cells = append(cells, cell{Value: shortID(q.ID), Kind: cellReadonly})
	if includeProject {
		projectName := q.Project.Title
		if projectName == "" {
			projectName = "Project " + q.ProjectID
		}
		cells = append(cells, cell{Value: projectName, Kind: cellText, LinkID: q.ProjectID})
	}
	if includeVendor {
		cells = append(cells, cell{Value: q.Vendor.Name, Kind: cellText, LinkID: q.VendorID})
	}
	cells = append(cells,
		cell{Value: cur.FormatCents(q.TotalCents), Kind: cellMoney},
		centsCell(q.LaborCents, cur),
		centsCell(q.MaterialsCents, cur),
		centsCell(q.OtherCents, cur),
		dateCell(q.ReceivedDate, cellDate),
		cell{Value: countStr(docCounts, q.ID), Kind: cellDrilldown},
	)
	return rowSpec{ID: q.ID, Deleted: q.DeletedAt.Valid, Cells: cells}
}

func quoteRows(
	quotes []data.Quote,
	docCounts map[string]int,
	cur locale.Currency,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(quotes, func(q data.Quote) rowSpec {
		return quoteRowSpec(q, docCounts, cur, true, true)
	})
}

func maintenanceSeasonCell(season string) cell {
	if season == "" {
		return cell{Kind: cellStatus, Null: true}
	}
	return cell{Value: season, Kind: cellStatus}
}

func maintenanceRows(
	items []data.MaintenanceItem,
	logCounts map[string]int,
	docCounts map[string]int,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(items, func(item data.MaintenanceItem) rowSpec {
		intervalCell := maintenanceIntervalCell(item)
		var appCell cell
		if item.ApplianceID != nil {
			appCell = cell{Value: item.Appliance.Name, Kind: cellText, LinkID: *item.ApplianceID}
		} else {
			appCell = cell{Kind: cellText, Null: true}
		}
		nextDue := data.ComputeNextDue(item.LastServicedAt, item.IntervalMonths, item.DueDate)
		return rowSpec{
			ID:      item.ID,
			Deleted: item.DeletedAt.Valid,
			Cells: []cell{
				{Value: shortID(item.ID), Kind: cellReadonly},
				{Value: item.Name, Kind: cellText},
				{Value: item.Category.Name, Kind: cellText},
				maintenanceSeasonCell(item.Season),
				appCell,
				dateCell(item.LastServicedAt, cellDate),
				dateCell(nextDue, cellUrgency),
				intervalCell,
				{Value: countStr(logCounts, item.ID), Kind: cellDrilldown},
				{Value: countStr(docCounts, item.ID), Kind: cellDrilldown},
			},
		}
	})
}

// transformCells returns a shallow copy of the cell grid with each cell
// passed through fn. The original grid is not modified.
func transformCells(rows [][]cell, fn func(cell) cell) [][]cell {
	out := make([][]cell, len(rows))
	for i, row := range rows {
		transformed := make([]cell, len(row))
		for j, c := range row {
			transformed[j] = fn(c)
		}
		out[i] = transformed
	}
	return out
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
	ID      string
	Deleted bool
	Cells   []cell
}

// entityIDs extracts the ID from each element using the given accessor.
func entityIDs[T any](items []T, id func(T) string) []string {
	ids := make([]string, len(items))
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
	docCounts map[string]int,
	cur locale.Currency,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(quotes, func(q data.Quote) rowSpec {
		return quoteRowSpec(q, docCounts, cur, true, false)
	})
}

func vendorJobsRows(
	entries []data.ServiceLogEntry,
	cur locale.Currency,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(entries, func(e data.ServiceLogEntry) rowSpec {
		itemName := e.MaintenanceItem.Name
		return rowSpec{
			ID:      e.ID,
			Deleted: e.DeletedAt.Valid,
			Cells: []cell{
				{Value: shortID(e.ID), Kind: cellReadonly},
				{Value: itemName, Kind: cellText, LinkID: e.MaintenanceItemID},
				{Value: e.ServicedAt.Format(data.DateLayout), Kind: cellDate},
				centsCell(e.CostCents, cur),
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
	docCounts map[string]int,
	cur locale.Currency,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(quotes, func(q data.Quote) rowSpec {
		return quoteRowSpec(q, docCounts, cur, false, true)
	})
}

// centsCell returns a cell for an optional money value. NULL pointer produces
// a null cell; non-nil produces a formatted money cell.
func centsCell(cents *int64, cur locale.Currency) cell {
	if cents == nil {
		return cell{Kind: cellMoney, Null: true}
	}
	return cell{Value: cur.FormatCents(*cents), Kind: cellMoney}
}

// dateCell returns a cell for an optional date value. NULL pointer produces
// a null cell with the given kind; non-nil produces a formatted date cell.
func dateCell(value *time.Time, kind cellKind) cell {
	if value == nil {
		return cell{Kind: kind, Null: true}
	}
	return cell{Value: value.Format(data.DateLayout), Kind: kind}
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

// opsCell returns a cell for the Ops column, showing the count of extraction
// operations stored in the document's ExtractionOps JSON blob.
func opsCell(raw []byte) cell {
	if len(raw) == 0 {
		return cell{Kind: cellOps}
	}
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) == 0 {
		return cell{Kind: cellOps}
	}
	return cell{Value: strconv.Itoa(len(arr)), Kind: cellOps}
}

func documentRows(docs []data.Document, names entityNameMap) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(docs, func(d data.Document) rowSpec {
		return rowSpec{
			ID:      d.ID,
			Deleted: d.DeletedAt.Valid,
			Cells: []cell{
				{Value: shortID(d.ID), Kind: cellReadonly},
				{Value: d.Title, Kind: cellText},
				{
					Value:  documentEntityLabel(d.EntityKind, d.EntityID, names),
					Kind:   cellEntity,
					LinkID: d.EntityID,
				},
				{Value: d.MIMEType, Kind: cellText},
				{Value: formatFileSize(docSizeBytes(d)), Kind: cellReadonly},
				{Value: d.ExtractionModel, Kind: cellReadonly},
				opsCell(d.ExtractionOps),
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
				{Value: shortID(d.ID), Kind: cellReadonly},
				{Value: d.Title, Kind: cellText},
				{Value: d.MIMEType, Kind: cellText},
				{Value: formatFileSize(docSizeBytes(d)), Kind: cellReadonly},
				{Value: d.ExtractionModel, Kind: cellReadonly},
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
func documentEntityLabel(kind string, id string, names entityNameMap) string {
	if kind == "" {
		return ""
	}
	letter, ok := entityKindLetter[kind]
	if !ok {
		return kind + " #" + id
	}
	if name, found := names[entityRef{Kind: kind, ID: id}]; found {
		return letter + " " + name
	}
	return letter + " #" + id
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
