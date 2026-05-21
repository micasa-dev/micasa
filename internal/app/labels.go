// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

// Plural entity nouns returned by TabKind.String() / .plural() and used as
// breadcrumb labels.
const (
	lblProjects   = "Projects"
	lblVendors    = "Vendors"
	lblAppliances = "Appliances"
	lblIncidents  = "Incidents"
)

// Lowercase singular entity nouns returned by TabKind.singular() for
// empty-state messages.
const tabSingularAppliance = "appliance"

// Chat notice and status strings.
const (
	noticeInterrupted      = "Interrupted"
	noticeGeneratingQuery  = "generating query"
	statusSwitchedToPrefix = "Switched to "
)

// Form field labels and placeholders.
const (
	fieldContactName   = "Contact name"
	fieldCost          = "cost"
	placeholderKitchen = "Kitchen"
	valueSelf          = "Self"
)
