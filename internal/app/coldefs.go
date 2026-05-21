// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// coldefs.go is the single source of truth for column ordering, metadata, and
// iota constant names. Each entity defines a []columnDef slice; the
// gencolumns tool reads these to produce typed iota constants in
// columns_generated.go. To add or reorder columns, edit the slice here, then
// run: go generate ./internal/app/

package app

//go:generate go run ./cmd/gencolumns/

// Column-header labels. Each is the display title used in the columnDef slices
// below -- both as the iota-suffix (first field) and the columnSpec Title --
// and is echoed by form-field titles, detail panels, and TabKind.String(). A
// header label and a same-valued tab name share one constant; they are the
// same word. gencolumns resolves these identifiers when reading coldefs.go, so
// they must be declared in this file.
const (
	colHdrName        = "Name"
	colHdrTitle       = "Title"
	colHdrStatus      = "Status"
	colHdrSeverity    = "Severity"
	colHdrLocation    = "Location"
	colHdrCost        = "Cost"
	colHdrBudget      = "Budget"
	colHdrBrand       = "Brand"
	colHdrEmail       = "Email"
	colHdrPhone       = "Phone"
	colHdrDate        = "Date"
	colHdrNotes       = "Notes"
	colHdrItem        = "Item"
	colHdrCategory    = "Category"
	colHdrAppliance   = "Appliance"
	colHdrVendor      = "Vendor"
	colHdrProject     = "Project"
	colHdrLabor       = "Labor"
	colHdrOther       = "Other"
	colHdrTotal       = "Total"
	colHdrLog         = "Log"
	colHdrJobs        = "Jobs"
	colHdrSeason      = "Season"
	colHdrEntity      = "Entity"
	colHdrType        = "Type"
	colHdrWebsite     = "Website"
	colHdrModel       = "Model"
	colHdrInterval    = "Interval"
	colHdrDetails     = "Details"
	colHdrQuotes      = "Quotes"
	colHdrDocs        = "Docs"
	colHdrMaintenance = "Maintenance"
)

// columnDef pairs a const-name suffix with its columnSpec, forming the single
// source of truth for column ordering and metadata. The gencolumns tool reads
// these slices to produce typed iota constants in columns_generated.go.
type columnDef struct {
	name string // iota suffix, e.g. "ID" -> projectColID
	spec columnSpec
}

// defsToSpecs extracts the specs from a columnDef slice.
func defsToSpecs(defs []columnDef) []columnSpec {
	specs := make([]columnSpec, len(defs))
	for i := range defs {
		specs[i] = defs[i].spec
	}
	return specs
}

// ---------------------------------------------------------------------------
// Project columns
// ---------------------------------------------------------------------------

var projectColumnDefs = []columnDef{
	{"ID", idColumnSpec()},
	{colHdrType, columnSpec{Title: colHdrType, Min: 8, Max: 14, Flex: true}},
	{colHdrTitle, columnSpec{Title: colHdrTitle, Min: 14, Max: 32, Flex: true}},
	{colHdrStatus, columnSpec{Title: colHdrStatus, Min: 6, Max: 8, Kind: cellStatus}},
	{
		colHdrBudget,
		columnSpec{Title: colHdrBudget, Min: 10, Max: 14, Align: alignRight, Kind: cellMoney},
	},
	{"Actual", columnSpec{Title: "Actual", Min: 10, Max: 14, Align: alignRight, Kind: cellMoney}},
	{"Start", columnSpec{Title: "Start", Min: 10, Max: 12, Kind: cellDate}},
	{"End", columnSpec{Title: "End", Min: 10, Max: 12, Kind: cellDate}},
	{
		colHdrQuotes,
		columnSpec{
			Title: tabQuotes.String(),
			Min:   6,
			Max:   8,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
	{
		colHdrDocs,
		columnSpec{
			Title: tabDocuments.String(),
			Min:   5,
			Max:   6,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
}

func projectColumnSpecs() []columnSpec { return defsToSpecs(projectColumnDefs) }

// ---------------------------------------------------------------------------
// Quote columns
// ---------------------------------------------------------------------------

var quoteColumnDefs = []columnDef{
	{"ID", idColumnSpec()},
	{colHdrProject, columnSpec{
		Title: colHdrProject,
		Min:   12,
		Max:   24,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabProjects},
	}},
	{colHdrVendor, columnSpec{
		Title: colHdrVendor,
		Min:   12,
		Max:   20,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabVendors},
	}},
	{
		colHdrTotal,
		columnSpec{Title: colHdrTotal, Min: 10, Max: 14, Align: alignRight, Kind: cellMoney},
	},
	{
		colHdrLabor,
		columnSpec{Title: colHdrLabor, Min: 10, Max: 14, Align: alignRight, Kind: cellMoney},
	},
	{"Mat", columnSpec{Title: "Mat", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney}},
	{
		colHdrOther,
		columnSpec{Title: colHdrOther, Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
	},
	{"Recv", columnSpec{Title: "Recv", Min: 10, Max: 12, Kind: cellDate}},
	{
		colHdrDocs,
		columnSpec{
			Title: tabDocuments.String(),
			Min:   5,
			Max:   6,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
}

func quoteColumnSpecs() []columnSpec { return defsToSpecs(quoteColumnDefs) }

// ---------------------------------------------------------------------------
// Maintenance columns
// ---------------------------------------------------------------------------

var maintenanceColumnDefs = []columnDef{
	{"ID", idColumnSpec()},
	{colHdrItem, columnSpec{Title: colHdrItem, Min: 12, Max: 26, Flex: true}},
	{colHdrCategory, columnSpec{Title: colHdrCategory, Min: 10, Max: 14}},
	{colHdrSeason, columnSpec{Title: colHdrSeason, Min: 6, Max: 8, Kind: cellStatus}},
	{colHdrAppliance, columnSpec{
		Title: colHdrAppliance,
		Min:   10,
		Max:   18,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabAppliances},
	}},
	{"Last", columnSpec{Title: "Last", Min: 10, Max: 12, Kind: cellDate}},
	{"Next", columnSpec{Title: "Next", Min: 10, Max: 12, Kind: cellUrgency}},
	{"Every", columnSpec{Title: "Every", Min: 6, Max: 10}},
	{
		colHdrLog,
		columnSpec{Title: colHdrLog, Min: 4, Max: 6, Align: alignRight, Kind: cellDrilldown},
	},
	{
		colHdrDocs,
		columnSpec{
			Title: tabDocuments.String(),
			Min:   5,
			Max:   6,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
}

func maintenanceColumnSpecs() []columnSpec { return defsToSpecs(maintenanceColumnDefs) }

// ---------------------------------------------------------------------------
// Incident columns
// ---------------------------------------------------------------------------

var incidentColumnDefs = []columnDef{
	{"ID", idColumnSpec()},
	{colHdrTitle, columnSpec{Title: colHdrTitle, Min: 14, Max: 32, Flex: true}},
	{colHdrStatus, columnSpec{Title: colHdrStatus, Min: 6, Max: 12, Kind: cellStatus}},
	{colHdrSeverity, columnSpec{Title: colHdrSeverity, Min: 6, Max: 10, Kind: cellStatus}},
	{colHdrLocation, columnSpec{Title: colHdrLocation, Min: 8, Max: 16, Flex: true}},
	{colHdrAppliance, columnSpec{
		Title: colHdrAppliance,
		Min:   10,
		Max:   18,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabAppliances},
	}},
	{colHdrVendor, columnSpec{
		Title: colHdrVendor,
		Min:   10,
		Max:   18,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabVendors},
	}},
	{"Noticed", columnSpec{Title: "Noticed", Min: 10, Max: 12, Kind: cellDate}},
	{"Resolved", columnSpec{Title: "Resolved", Min: 10, Max: 12, Kind: cellDate}},
	{
		colHdrCost,
		columnSpec{Title: colHdrCost, Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
	},
	{
		colHdrDocs,
		columnSpec{
			Title: tabDocuments.String(),
			Min:   5,
			Max:   6,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
}

func incidentColumnSpecs() []columnSpec { return defsToSpecs(incidentColumnDefs) }

// ---------------------------------------------------------------------------
// Appliance columns
// ---------------------------------------------------------------------------

var applianceColumnDefs = []columnDef{
	{"ID", idColumnSpec()},
	{colHdrName, columnSpec{Title: colHdrName, Min: 12, Max: 24, Flex: true}},
	{colHdrBrand, columnSpec{Title: colHdrBrand, Min: 8, Max: 16, Flex: true}},
	{colHdrModel, columnSpec{Title: colHdrModel, Min: 8, Max: 16}},
	{"Serial", columnSpec{Title: "Serial", Min: 8, Max: 14}},
	{colHdrLocation, columnSpec{Title: colHdrLocation, Min: 8, Max: 14}},
	{"Purchased", columnSpec{Title: "Purchased", Min: 10, Max: 12, Kind: cellDate}},
	{"Age", columnSpec{Title: "Age", Min: 5, Max: 8, Kind: cellReadonly}},
	{"Warranty", columnSpec{Title: "Warranty", Min: 10, Max: 12, Kind: cellWarranty}},
	{
		colHdrCost,
		columnSpec{Title: colHdrCost, Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
	},
	{"Maint", columnSpec{Title: "Maint", Min: 5, Max: 6, Align: alignRight, Kind: cellDrilldown}},
	{
		colHdrDocs,
		columnSpec{
			Title: tabDocuments.String(),
			Min:   5,
			Max:   6,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
}

func applianceColumnSpecs() []columnSpec { return defsToSpecs(applianceColumnDefs) }

// ---------------------------------------------------------------------------
// Vendor columns
// ---------------------------------------------------------------------------

var vendorColumnDefs = []columnDef{
	{"ID", idColumnSpec()},
	{colHdrName, columnSpec{Title: colHdrName, Min: 14, Max: 24, Flex: true}},
	{"Contact", columnSpec{Title: "Contact", Min: 10, Max: 20, Flex: true}},
	{colHdrEmail, columnSpec{Title: colHdrEmail, Min: 12, Max: 24, Flex: true}},
	{colHdrPhone, columnSpec{Title: colHdrPhone, Min: 12, Max: 20, Kind: cellTelephoneNumber}},
	{colHdrWebsite, columnSpec{Title: colHdrWebsite, Min: 12, Max: 28, Flex: true}},
	{
		colHdrQuotes,
		columnSpec{
			Title: tabQuotes.String(),
			Min:   6,
			Max:   8,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
	{
		colHdrJobs,
		columnSpec{Title: colHdrJobs, Min: 5, Max: 8, Align: alignRight, Kind: cellDrilldown},
	},
	{
		colHdrDocs,
		columnSpec{
			Title: tabDocuments.String(),
			Min:   5,
			Max:   6,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
}

func vendorColumnSpecs() []columnSpec { return defsToSpecs(vendorColumnDefs) }

// ---------------------------------------------------------------------------
// Service log columns
// ---------------------------------------------------------------------------

var serviceLogColumnDefs = []columnDef{
	{"ID", idColumnSpec()},
	{colHdrDate, columnSpec{Title: colHdrDate, Min: 10, Max: 12, Kind: cellDate}},
	{"PerformedBy", columnSpec{
		Title: "Performed By",
		Min:   12,
		Max:   22,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabVendors},
	}},
	{
		colHdrCost,
		columnSpec{Title: colHdrCost, Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
	},
	{colHdrNotes, columnSpec{Title: colHdrNotes, Min: 12, Max: 40, Flex: true, Kind: cellNotes}},
	{
		colHdrDocs,
		columnSpec{
			Title: tabDocuments.String(),
			Min:   5,
			Max:   8,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
}

func serviceLogColumnSpecs() []columnSpec { return defsToSpecs(serviceLogColumnDefs) }

// ---------------------------------------------------------------------------
// Vendor jobs columns (service logs scoped to a vendor)
// ---------------------------------------------------------------------------

var vendorJobsColumnDefs = []columnDef{
	{"ID", idColumnSpec()},
	{colHdrItem, columnSpec{
		Title: colHdrItem,
		Min:   12,
		Max:   24,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabMaintenance},
	}},
	{colHdrDate, columnSpec{Title: colHdrDate, Min: 10, Max: 12, Kind: cellDate}},
	{
		colHdrCost,
		columnSpec{Title: colHdrCost, Min: 8, Max: 12, Align: alignRight, Kind: cellMoney},
	},
	{colHdrNotes, columnSpec{Title: colHdrNotes, Min: 12, Max: 40, Flex: true, Kind: cellNotes}},
}

func vendorJobsColumnSpecs() []columnSpec { return defsToSpecs(vendorJobsColumnDefs) }

// ---------------------------------------------------------------------------
// Document columns
// ---------------------------------------------------------------------------

var documentColumnDefs = []columnDef{
	{"ID", idColumnSpec()},
	{colHdrTitle, columnSpec{Title: colHdrTitle, Min: 14, Max: 32, Flex: true}},
	{colHdrEntity, columnSpec{Title: colHdrEntity, Min: 10, Max: 24, Flex: true, Kind: cellEntity}},
	{colHdrType, columnSpec{Title: colHdrType, Min: 8, Max: 16}},
	{"Size", columnSpec{Title: "Size", Min: 6, Max: 10, Align: alignRight, Kind: cellReadonly}},
	{colHdrModel, columnSpec{Title: colHdrModel, Min: 8, Max: 20, Kind: cellReadonly}},
	{"Ops", columnSpec{Title: "Ops", Min: 4, Max: 6, Align: alignRight, Kind: cellOps}},
	{colHdrNotes, columnSpec{Title: colHdrNotes, Min: 12, Max: 40, Flex: true, Kind: cellNotes}},
	{"Updated", columnSpec{Title: "Updated", Min: 10, Max: 12, Kind: cellReadonly}},
}

func documentColumnSpecs() []columnSpec { return defsToSpecs(documentColumnDefs) }
