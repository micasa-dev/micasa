// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package ftseval

import (
	"errors"
	"fmt"
	"time"

	"github.com/micasa-dev/micasa/internal/data"
)

// SeededFixture is the concrete set of entity IDs created by SeedFixture.
// Tests and the question set use these IDs to assert FTS surfacing.
type SeededFixture struct {
	ProjectKitchenID   string
	ProjectBasementID  string
	ProjectRoofID      string
	VendorKitchenSupID string
	VendorPacificID    string
	VendorAcmeID       string
	ApplianceMaytagID  string
	ApplianceRheemID   string
	ApplianceCarrierID string
	MaintHVACID        string
	MaintGutterID      string
	MaintWaterHtrID    string
	IncidentLeakID     string
	ServiceLogHVACID   string
	QuoteKitchenID     string
}

// SeedFixture populates the given store with the fixture described in
// plans/707-fts-eval-and-hardening.md. The store must be empty; on any
// insert failure, the partial state is left behind and the error is
// returned (the caller is expected to discard the store).
func SeedFixture(store *data.Store) (SeededFixture, error) {
	var f SeededFixture

	types, err := store.ProjectTypes()
	if err != nil {
		return f, fmt.Errorf("fixture: read project types: %w", err)
	}
	if len(types) == 0 {
		return f, errors.New("fixture: store has no project types seeded")
	}
	defaultType := types[0].ID

	cats, err := store.MaintenanceCategories()
	if err != nil {
		return f, fmt.Errorf("fixture: read maintenance categories: %w", err)
	}
	if len(cats) == 0 {
		return f, errors.New("fixture: store has no maintenance categories seeded")
	}
	defaultCat := cats[0].ID

	// Projects.
	budgetKitchen := int64(1_200_000) // $12,000.00
	actualKitchen := int64(300_000)
	kitchen := &data.Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: defaultType,
		Status:        data.ProjectStatusInProgress,
		Description:   "Full gut and refresh; plumbing + tile + cabinets",
		BudgetCents:   &budgetKitchen,
		ActualCents:   &actualKitchen,
	}
	if err := store.CreateProject(kitchen); err != nil {
		return f, fmt.Errorf("fixture: create kitchen project: %w", err)
	}
	f.ProjectKitchenID = kitchen.ID

	basement := &data.Project{
		Title:         "Basement Refinish",
		ProjectTypeID: defaultType,
		Status:        data.ProjectStatusPlanned,
		Description:   "Drywall, flooring, egress window",
	}
	if err := store.CreateProject(basement); err != nil {
		return f, fmt.Errorf("fixture: create basement project: %w", err)
	}
	f.ProjectBasementID = basement.ID

	roof := &data.Project{
		Title:         "Roof Replacement",
		ProjectTypeID: defaultType,
		Status:        data.ProjectStatusCompleted,
		Description:   "Tear-off and new asphalt shingles",
	}
	if err := store.CreateProject(roof); err != nil {
		return f, fmt.Errorf("fixture: create roof project: %w", err)
	}
	f.ProjectRoofID = roof.ID

	// Vendors.
	kitchenSup := &data.Vendor{Name: "Kitchen Supplies Co"}
	if err := store.CreateVendor(kitchenSup); err != nil {
		return f, fmt.Errorf("fixture: create kitchen-supplies vendor: %w", err)
	}
	f.VendorKitchenSupID = kitchenSup.ID

	pacific := &data.Vendor{
		Name:  "Pacific Plumbing",
		Notes: "quoted in February; mentioned permit delays for the basement job",
	}
	if err := store.CreateVendor(pacific); err != nil {
		return f, fmt.Errorf("fixture: create pacific vendor: %w", err)
	}
	f.VendorPacificID = pacific.ID

	acme := &data.Vendor{Name: "Acme HVAC"}
	if err := store.CreateVendor(acme); err != nil {
		return f, fmt.Errorf("fixture: create acme vendor: %w", err)
	}
	f.VendorAcmeID = acme.ID

	// Appliances.
	maytag := &data.Appliance{
		Name:     "Maytag Dishwasher",
		Brand:    "Maytag",
		Location: "kitchen",
	}
	if err := store.CreateAppliance(maytag); err != nil {
		return f, fmt.Errorf("fixture: create maytag appliance: %w", err)
	}
	f.ApplianceMaytagID = maytag.ID

	rheem := &data.Appliance{
		Name:     "Rheem Water Heater",
		Brand:    "Rheem",
		Location: "basement",
	}
	if err := store.CreateAppliance(rheem); err != nil {
		return f, fmt.Errorf("fixture: create rheem appliance: %w", err)
	}
	f.ApplianceRheemID = rheem.ID

	carrier := &data.Appliance{
		Name:     "Carrier Furnace",
		Brand:    "Carrier",
		Location: "basement",
	}
	if err := store.CreateAppliance(carrier); err != nil {
		return f, fmt.Errorf("fixture: create carrier appliance: %w", err)
	}
	f.ApplianceCarrierID = carrier.ID

	// Maintenance items.
	hvac := &data.MaintenanceItem{
		Name:           "HVAC Filter Change",
		CategoryID:     defaultCat,
		IntervalMonths: 3,
		Season:         "fall",
	}
	if err := store.CreateMaintenance(hvac); err != nil {
		return f, fmt.Errorf("fixture: create hvac maintenance: %w", err)
	}
	f.MaintHVACID = hvac.ID

	gutter := &data.MaintenanceItem{
		Name:           "Gutter Cleaning",
		CategoryID:     defaultCat,
		IntervalMonths: 12,
		Season:         "fall",
	}
	if err := store.CreateMaintenance(gutter); err != nil {
		return f, fmt.Errorf("fixture: create gutter maintenance: %w", err)
	}
	f.MaintGutterID = gutter.ID

	waterHtr := &data.MaintenanceItem{
		Name:           "Water Heater Flush",
		CategoryID:     defaultCat,
		IntervalMonths: 12,
		Season:         "spring",
	}
	if err := store.CreateMaintenance(waterHtr); err != nil {
		return f, fmt.Errorf("fixture: create water-heater maintenance: %w", err)
	}
	f.MaintWaterHtrID = waterHtr.ID

	// Incidents.
	leak := &data.Incident{
		Title:       "Basement leak after heavy rain",
		Status:      "open",
		Severity:    "high",
		Location:    "basement",
		Description: "Water intrusion near the foundation wall",
	}
	if err := store.CreateIncident(leak); err != nil {
		return f, fmt.Errorf("fixture: create leak incident: %w", err)
	}
	f.IncidentLeakID = leak.ID

	// Service log (HVAC serviced a month ago).
	sleCost := int64(12_000) // $120.00
	sle := &data.ServiceLogEntry{
		MaintenanceItemID: hvac.ID,
		ServicedAt:        time.Now().AddDate(0, -1, 0),
		CostCents:         &sleCost,
		Notes:             "filter swapped; blower cleaned",
	}
	if err := store.CreateServiceLog(sle, data.Vendor{}); err != nil {
		return f, fmt.Errorf("fixture: create service log: %w", err)
	}
	f.ServiceLogHVACID = sle.ID

	// Quote (Pacific plumbing for the kitchen project).
	quote := &data.Quote{
		ProjectID:  kitchen.ID,
		VendorID:   pacific.ID,
		TotalCents: 450_000, // $4,500.00
		Notes:      "labor + materials for rough plumbing",
	}
	if err := store.CreateQuote(quote, *pacific); err != nil {
		return f, fmt.Errorf("fixture: create kitchen quote: %w", err)
	}
	f.QuoteKitchenID = quote.ID

	return f, nil
}
