// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

// Entity slugs used as CLI command names, "show" subcommand names, and
// denormalized JSON keys for embedded related records.
const (
	entityAppliance   = "appliance"
	entityVendor      = "vendor"
	entityMaintenance = "maintenance"
	entityServiceLog  = "service-log"
)

// Plural entity slugs used as "show" subcommand names and JSON collection keys.
const (
	entityProjects   = "projects"
	entityVendors    = "vendors"
	entityAppliances = "appliances"
	entityIncidents  = "incidents"
	entityQuotes     = "quotes"
	entityDocuments  = "documents"
)

// Uppercase column headers shared across "show" text tables.
const colHdrName = "NAME"
