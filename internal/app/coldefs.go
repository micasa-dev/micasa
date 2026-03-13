// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// coldefs.go is the single source of truth for column ordering, metadata, and
// iota constant names. Each entity defines a []columnDef slice; the
// gencolumns tool reads these to produce typed iota constants in
// columns_generated.go. To add or reorder columns, edit the slice here, then
// run: go generate ./internal/app/

package app

//go:generate go run ./cmd/gencolumns/

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
	{"Type", columnSpec{Title: "Type", Min: 8, Max: 14, Flex: true}},
	{"Title", columnSpec{Title: "Title", Min: 14, Max: 32, Flex: true}},
	{"Status", columnSpec{Title: "Status", Min: 6, Max: 8, Kind: cellStatus}},
	{"Budget", columnSpec{Title: "Budget", Min: 10, Max: 14, Align: alignRight, Kind: cellMoney}},
	{"Actual", columnSpec{Title: "Actual", Min: 10, Max: 14, Align: alignRight, Kind: cellMoney}},
	{"Start", columnSpec{Title: "Start", Min: 10, Max: 12, Kind: cellDate}},
	{"End", columnSpec{Title: "End", Min: 10, Max: 12, Kind: cellDate}},
	{
		"Quotes",
		columnSpec{
			Title: tabQuotes.String(),
			Min:   6,
			Max:   8,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
	{
		"Docs",
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
	{"Project", columnSpec{
		Title: "Project",
		Min:   12,
		Max:   24,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabProjects},
	}},
	{"Vendor", columnSpec{
		Title: "Vendor",
		Min:   12,
		Max:   20,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabVendors},
	}},
	{"Total", columnSpec{Title: "Total", Min: 10, Max: 14, Align: alignRight, Kind: cellMoney}},
	{"Labor", columnSpec{Title: "Labor", Min: 10, Max: 14, Align: alignRight, Kind: cellMoney}},
	{"Mat", columnSpec{Title: "Mat", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney}},
	{"Other", columnSpec{Title: "Other", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney}},
	{"Recv", columnSpec{Title: "Recv", Min: 10, Max: 12, Kind: cellDate}},
	{
		"Docs",
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
	{"Item", columnSpec{Title: "Item", Min: 12, Max: 26, Flex: true}},
	{"Category", columnSpec{Title: "Category", Min: 10, Max: 14}},
	{"Season", columnSpec{Title: "Season", Min: 6, Max: 8, Kind: cellStatus}},
	{"Appliance", columnSpec{
		Title: "Appliance",
		Min:   10,
		Max:   18,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabAppliances},
	}},
	{"Last", columnSpec{Title: "Last", Min: 10, Max: 12, Kind: cellDate}},
	{"Next", columnSpec{Title: "Next", Min: 10, Max: 12, Kind: cellUrgency}},
	{"Every", columnSpec{Title: "Every", Min: 6, Max: 10}},
	{"Log", columnSpec{Title: "Log", Min: 4, Max: 6, Align: alignRight, Kind: cellDrilldown}},
	{
		"Docs",
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
	{"Title", columnSpec{Title: "Title", Min: 14, Max: 32, Flex: true}},
	{"Status", columnSpec{Title: "Status", Min: 6, Max: 12, Kind: cellStatus}},
	{"Severity", columnSpec{Title: "Severity", Min: 6, Max: 10, Kind: cellStatus}},
	{"Location", columnSpec{Title: "Location", Min: 8, Max: 16, Flex: true}},
	{"Appliance", columnSpec{
		Title: "Appliance",
		Min:   10,
		Max:   18,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabAppliances},
	}},
	{"Vendor", columnSpec{
		Title: "Vendor",
		Min:   10,
		Max:   18,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabVendors},
	}},
	{"Noticed", columnSpec{Title: "Noticed", Min: 10, Max: 12, Kind: cellDate}},
	{"Resolved", columnSpec{Title: "Resolved", Min: 10, Max: 12, Kind: cellDate}},
	{"Cost", columnSpec{Title: "Cost", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney}},
	{
		"Docs",
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
	{"Name", columnSpec{Title: "Name", Min: 12, Max: 24, Flex: true}},
	{"Brand", columnSpec{Title: "Brand", Min: 8, Max: 16, Flex: true}},
	{"Model", columnSpec{Title: "Model", Min: 8, Max: 16}},
	{"Serial", columnSpec{Title: "Serial", Min: 8, Max: 14}},
	{"Location", columnSpec{Title: "Location", Min: 8, Max: 14}},
	{"Purchased", columnSpec{Title: "Purchased", Min: 10, Max: 12, Kind: cellDate}},
	{"Age", columnSpec{Title: "Age", Min: 5, Max: 8, Kind: cellReadonly}},
	{"Warranty", columnSpec{Title: "Warranty", Min: 10, Max: 12, Kind: cellWarranty}},
	{"Cost", columnSpec{Title: "Cost", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney}},
	{"Maint", columnSpec{Title: "Maint", Min: 5, Max: 6, Align: alignRight, Kind: cellDrilldown}},
	{
		"Docs",
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
	{"Name", columnSpec{Title: "Name", Min: 14, Max: 24, Flex: true}},
	{"Contact", columnSpec{Title: "Contact", Min: 10, Max: 20, Flex: true}},
	{"Email", columnSpec{Title: "Email", Min: 12, Max: 24, Flex: true}},
	{"Phone", columnSpec{Title: "Phone", Min: 12, Max: 16}},
	{"Website", columnSpec{Title: "Website", Min: 12, Max: 28, Flex: true}},
	{
		"Quotes",
		columnSpec{
			Title: tabQuotes.String(),
			Min:   6,
			Max:   8,
			Align: alignRight,
			Kind:  cellDrilldown,
		},
	},
	{"Jobs", columnSpec{Title: "Jobs", Min: 5, Max: 8, Align: alignRight, Kind: cellDrilldown}},
	{
		"Docs",
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
	{"Date", columnSpec{Title: "Date", Min: 10, Max: 12, Kind: cellDate}},
	{"PerformedBy", columnSpec{
		Title: "Performed By",
		Min:   12,
		Max:   22,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabVendors},
	}},
	{"Cost", columnSpec{Title: "Cost", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney}},
	{"Notes", columnSpec{Title: "Notes", Min: 12, Max: 40, Flex: true, Kind: cellNotes}},
	{
		"Docs",
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
	{"Item", columnSpec{
		Title: "Item",
		Min:   12,
		Max:   24,
		Flex:  true,
		Link:  &columnLink{TargetTab: tabMaintenance},
	}},
	{"Date", columnSpec{Title: "Date", Min: 10, Max: 12, Kind: cellDate}},
	{"Cost", columnSpec{Title: "Cost", Min: 8, Max: 12, Align: alignRight, Kind: cellMoney}},
	{"Notes", columnSpec{Title: "Notes", Min: 12, Max: 40, Flex: true, Kind: cellNotes}},
}

func vendorJobsColumnSpecs() []columnSpec { return defsToSpecs(vendorJobsColumnDefs) }

// ---------------------------------------------------------------------------
// Document columns
// ---------------------------------------------------------------------------

var documentColumnDefs = []columnDef{
	{"ID", idColumnSpec()},
	{"Title", columnSpec{Title: "Title", Min: 14, Max: 32, Flex: true}},
	{"Entity", columnSpec{Title: "Entity", Min: 10, Max: 24, Flex: true, Kind: cellEntity}},
	{"Type", columnSpec{Title: "Type", Min: 8, Max: 16}},
	{"Size", columnSpec{Title: "Size", Min: 6, Max: 10, Align: alignRight, Kind: cellReadonly}},
	{"Model", columnSpec{Title: "Model", Min: 8, Max: 20, Kind: cellReadonly}},
	{"Notes", columnSpec{Title: "Notes", Min: 12, Max: 40, Flex: true, Kind: cellNotes}},
	{"Updated", columnSpec{Title: "Updated", Min: 10, Max: 12, Kind: cellReadonly}},
}

func documentColumnSpecs() []columnSpec { return defsToSpecs(documentColumnDefs) }
