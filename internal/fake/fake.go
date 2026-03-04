// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Package fake provides a home-domain data generator built on gofakeit.
// It produces realistic but randomized data for houses, projects, vendors,
// appliances, maintenance items, service logs, and quotes.
//
// All output types are defined within this package (no dependency on
// internal/data) so callers can map to their own model types without
// creating import cycles.
package fake

import (
	"fmt"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v7"
)

// Project statuses (mirrors data.ProjectStatus* constants).
const (
	StatusIdeating   = "ideating"
	StatusPlanned    = "planned"
	StatusQuoted     = "quoted"
	StatusInProgress = "underway"
	StatusDelayed    = "delayed"
	StatusCompleted  = "completed"
	StatusAbandoned  = "abandoned"
)

// Incident statuses (mirrors data.IncidentStatus* constants).
// Duplicated here because data imports fake, so fake cannot import data
// without creating a circular dependency.
const (
	IncidentStatusOpen       = "open"
	IncidentStatusInProgress = "in_progress"
)

// Incident severities (mirrors data.IncidentSeverity* constants).
// Duplicated here because data imports fake, so fake cannot import data
// without creating a circular dependency.
const (
	IncidentSeverityUrgent   = "urgent"
	IncidentSeveritySoon     = "soon"
	IncidentSeverityWhenever = "whenever"
)

var (
	allIncidentStatuses   = []string{IncidentStatusOpen, IncidentStatusInProgress}
	allIncidentSeverities = []string{
		IncidentSeverityUrgent,
		IncidentSeveritySoon,
		IncidentSeverityWhenever,
	}
)

var allStatuses = []string{
	StatusIdeating, StatusPlanned, StatusQuoted, StatusInProgress,
	StatusDelayed, StatusCompleted, StatusAbandoned,
}

// HomeFaker wraps gofakeit with home-domain generators.
type HomeFaker struct {
	f *gofakeit.Faker
}

// New creates a HomeFaker with the given seed. Pass 0 for a
// cryptographically random seed.
func New(seed uint64) *HomeFaker {
	return &HomeFaker{f: gofakeit.New(seed)}
}

// IntN returns a random int in [0, n). Exposed so callers can use
// the faker's RNG for loop counts and other randomized decisions.
func (h *HomeFaker) IntN(n int) int {
	return h.f.IntN(n)
}

// pick returns a random element from a string slice.
func (h *HomeFaker) pick(items []string) string {
	return items[h.f.IntN(len(items))]
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------

// HouseProfile holds generated house data.
type HouseProfile struct {
	Nickname         string
	AddressLine1     string
	City             string
	State            string
	PostalCode       string
	YearBuilt        int
	SquareFeet       int
	LotSquareFeet    int
	Bedrooms         int
	Bathrooms        float64
	FoundationType   string
	WiringType       string
	RoofType         string
	ExteriorType     string
	HeatingType      string
	CoolingType      string
	WaterSource      string
	SewerType        string
	ParkingType      string
	BasementType     string
	InsuranceCarrier string
	InsurancePolicy  string
	InsuranceRenewal *time.Time
	PropertyTaxCents *int64
	HOAName          string
	HOAFeeCents      *int64
}

// Vendor holds generated vendor data.
type Vendor struct {
	Name        string
	ContactName string
	Phone       string
	Email       string
	Website     string
}

// Project holds generated project data.
type Project struct {
	Title       string
	TypeName    string
	Status      string
	Description string
	StartDate   *time.Time
	EndDate     *time.Time
	BudgetCents *int64
	ActualCents *int64
}

// Appliance holds generated appliance data.
type Appliance struct {
	Name           string
	Brand          string
	ModelNumber    string
	SerialNumber   string
	Location       string
	PurchaseDate   *time.Time
	WarrantyExpiry *time.Time
	CostCents      *int64
}

// MaintenanceItem holds generated maintenance item data.
type MaintenanceItem struct {
	Name           string
	CategoryName   string
	IntervalMonths int
	Notes          string
	LastServicedAt *time.Time
	CostCents      *int64
}

// ServiceLogEntry holds generated service log data.
type ServiceLogEntry struct {
	ServicedAt time.Time
	CostCents  *int64
	Notes      string
}

// Incident holds generated incident data.
type Incident struct {
	Title       string
	Description string
	Status      string
	Severity    string
	DateNoticed time.Time
	Location    string
	CostCents   *int64
}

// Quote holds generated quote data.
type Quote struct {
	TotalCents     int64
	LaborCents     *int64
	MaterialsCents *int64
	ReceivedDate   *time.Time
	Notes          string
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

// HouseProfile generates a complete house profile with realistic specs.
func (h *HomeFaker) HouseProfile() HouseProfile {
	addr := h.f.Address()
	yearBuilt := h.f.IntRange(1920, 2024)
	sqft := h.f.IntRange(800, 4500)
	renewal := h.f.FutureDate()
	taxCents := int64(h.f.IntRange(100000, 1200000))
	hoaCents := int64(h.f.IntRange(5000, 50000))

	return HouseProfile{
		Nickname:         addr.Street,
		AddressLine1:     addr.Address,
		City:             addr.City,
		State:            addr.State,
		PostalCode:       addr.Zip,
		YearBuilt:        yearBuilt,
		SquareFeet:       sqft,
		LotSquareFeet:    h.f.IntRange(sqft, sqft*4),
		Bedrooms:         h.f.IntRange(1, 6),
		Bathrooms:        float64(h.f.IntRange(2, 9)) / 2.0,
		FoundationType:   h.pick(foundationTypes),
		WiringType:       h.pick(wiringTypes),
		RoofType:         h.pick(roofTypes),
		ExteriorType:     h.pick(exteriorTypes),
		HeatingType:      h.pick(heatingTypes),
		CoolingType:      h.pick(coolingTypes),
		WaterSource:      h.pick(waterSources),
		SewerType:        h.pick(sewerTypes),
		ParkingType:      h.pick(parkingTypes),
		BasementType:     h.pick(basementTypes),
		InsuranceCarrier: h.pick(insuranceCarriers),
		InsurancePolicy: fmt.Sprintf(
			"HO-%02d-%07d",
			h.f.IntRange(1, 99),
			h.f.IntRange(0, 9999999),
		),
		InsuranceRenewal: &renewal,
		PropertyTaxCents: &taxCents,
		HOAName:          fmt.Sprintf("%s HOA", addr.Street),
		HOAFeeCents:      &hoaCents,
	}
}

// vendorNameForTrade builds a vendor name from a trade like "Plumbing".
func (h *HomeFaker) vendorNameForTrade(trade string) string {
	if h.f.Bool() {
		return fmt.Sprintf("%s %s", h.f.LastName(), trade)
	}
	return fmt.Sprintf("%s %s %s", h.pick(vendorAdjectives), trade, h.pick(vendorSuffixes))
}

// Vendor generates a complete vendor with contact details.
func (h *HomeFaker) Vendor() Vendor {
	trade := h.pick(vendorTrades)
	return Vendor{
		Name:        h.vendorNameForTrade(trade),
		ContactName: h.f.Name(),
		Phone:       h.f.Phone(),
		Email:       h.f.Email(),
		Website:     fmt.Sprintf("https://%s", h.f.DomainName()),
	}
}

// VendorForTrade generates a vendor specializing in the given trade.
func (h *HomeFaker) VendorForTrade(trade string) Vendor {
	return Vendor{
		Name:        h.vendorNameForTrade(trade),
		ContactName: h.f.Name(),
		Phone:       h.f.Phone(),
		Email:       h.f.Email(),
	}
}

// Project generates a project for the given project type name.
func (h *HomeFaker) Project(typeName string) Project {
	titles, ok := projectTitles[typeName]
	if !ok {
		titles = []string{fmt.Sprintf("Fix %s issue", strings.ToLower(typeName))}
	}
	title := h.pick(titles)
	status := h.pick(allStatuses)

	p := Project{
		Title:       title,
		TypeName:    typeName,
		Status:      status,
		Description: h.f.Sentence(h.f.IntRange(8, 20)),
	}

	if status != StatusIdeating && status != StatusAbandoned {
		start := h.f.DateRange(
			time.Now().UTC().UTC().AddDate(-2, 0, 0),
			time.Now().UTC(),
		)
		p.StartDate = &start
		budgetCents := int64(h.f.IntRange(5000, 1500000))
		p.BudgetCents = &budgetCents
	}
	if status == StatusCompleted {
		end := h.f.DateRange(*p.StartDate, time.Now().UTC())
		p.EndDate = &end
		budget := *p.BudgetCents
		variance := int64(float64(budget) * (h.f.Float64Range(-0.2, 0.2)))
		actual := budget + variance
		p.ActualCents = &actual
	}

	return p
}

// Appliance generates an appliance with brand, model, serial, and location.
func (h *HomeFaker) Appliance() Appliance {
	name := h.pick(applianceNames)
	brand := h.pick(applianceBrands)
	prefix := brandPrefix(brand)
	purchDate := h.f.DateRange(
		time.Now().UTC().UTC().AddDate(-10, 0, 0),
		time.Now().UTC().UTC().AddDate(-1, 0, 0),
	)
	costCents := int64(h.f.IntRange(15000, 800000))

	a := Appliance{
		Name:        name,
		Brand:       brand,
		ModelNumber: fmt.Sprintf("%s-%04d", prefix, h.f.IntRange(100, 9999)),
		SerialNumber: fmt.Sprintf(
			"%s-%02d-%06d",
			prefix,
			h.f.IntRange(0, 99),
			h.f.IntRange(0, 999999),
		),
		Location:     h.pick(applianceLocations),
		PurchaseDate: &purchDate,
		CostCents:    &costCents,
	}

	if h.f.IntRange(1, 10) <= 6 {
		years := h.f.IntRange(1, 10)
		expiry := purchDate.AddDate(years, 0, 0)
		a.WarrantyExpiry = &expiry
	}

	return a
}

// MaintenanceItem generates a maintenance item for the given category.
func (h *HomeFaker) MaintenanceItem(categoryName string) MaintenanceItem {
	items, ok := maintenanceItems[categoryName]
	if !ok || len(items) == 0 {
		return MaintenanceItem{
			Name:           fmt.Sprintf("Check %s", strings.ToLower(categoryName)),
			CategoryName:   categoryName,
			IntervalMonths: 12,
		}
	}

	item := items[h.f.IntN(len(items))]

	m := MaintenanceItem{
		Name:           item.Name,
		CategoryName:   categoryName,
		IntervalMonths: item.Interval,
		Notes:          item.Notes,
	}

	if h.f.IntRange(1, 10) <= 7 {
		lastSrv := h.f.DateRange(
			time.Now().UTC().UTC().AddDate(0, -item.Interval*2, 0),
			time.Now().UTC(),
		)
		m.LastServicedAt = &lastSrv
	}

	if h.f.IntRange(1, 10) <= 4 {
		cost := int64(h.f.IntRange(500, 50000))
		m.CostCents = &cost
	}

	return m
}

// ServiceLogEntry generates a service log entry.
func (h *HomeFaker) ServiceLogEntry() ServiceLogEntry {
	servicedAt := h.f.DateRange(
		time.Now().UTC().UTC().AddDate(-2, 0, 0),
		time.Now().UTC(),
	)
	costCents := int64(h.f.IntRange(1000, 60000))

	return ServiceLogEntry{
		ServicedAt: servicedAt,
		CostCents:  &costCents,
		Notes:      h.pick(serviceLogNotes),
	}
}

// Quote generates a quote.
func (h *HomeFaker) Quote() Quote {
	totalCents := int64(h.f.IntRange(10000, 2000000))
	laborPct := h.f.Float64Range(0.4, 0.7)
	laborCents := int64(float64(totalCents) * laborPct)
	materialsCents := totalCents - laborCents

	received := h.f.DateRange(
		time.Now().UTC().UTC().AddDate(-1, 0, 0),
		time.Now().UTC(),
	)

	return Quote{
		TotalCents:     totalCents,
		LaborCents:     &laborCents,
		MaterialsCents: &materialsCents,
		ReceivedDate:   &received,
		Notes:          h.f.Sentence(h.f.IntRange(5, 15)),
	}
}

// Incident generates a random incident.
func (h *HomeFaker) Incident() Incident {
	title := h.pick(incidentTitles)
	severity := h.pick(allIncidentSeverities)
	status := h.pick(allIncidentStatuses)
	noticed := h.f.DateRange(
		time.Now().UTC().UTC().AddDate(-1, 0, 0),
		time.Now().UTC(),
	)

	inc := Incident{
		Title:       title,
		Description: h.f.Sentence(h.f.IntRange(8, 20)),
		Status:      status,
		Severity:    severity,
		DateNoticed: noticed,
		Location:    h.pick(incidentLocations),
	}

	if h.f.IntRange(1, 10) <= 5 {
		cost := int64(h.f.IntRange(2000, 300000))
		inc.CostCents = &cost
	}

	return inc
}

// ServiceLogEntryAt generates a service log entry with the given service date.
func (h *HomeFaker) ServiceLogEntryAt(servicedAt time.Time) ServiceLogEntry {
	costCents := int64(h.f.IntRange(1000, 60000))
	return ServiceLogEntry{
		ServicedAt: servicedAt,
		CostCents:  &costCents,
		Notes:      h.pick(serviceLogNotes),
	}
}

// ---------------------------------------------------------------------------
// Static lookups
// ---------------------------------------------------------------------------

// ProjectTypes returns the list of known project type names.
func ProjectTypes() []string {
	types := make([]string, 0, len(projectTitles))
	for k := range projectTitles {
		types = append(types, k)
	}
	return types
}

// MaintenanceCategories returns the list of known maintenance category names.
func MaintenanceCategories() []string {
	cats := make([]string, 0, len(maintenanceItems))
	for k := range maintenanceItems {
		cats = append(cats, k)
	}
	return cats
}

// VendorTrades returns the list of vendor trade specializations.
func VendorTrades() []string {
	return append([]string{}, vendorTrades...)
}

// brandPrefix returns the first two characters of a brand, uppercased.
func brandPrefix(brand string) string {
	runes := []rune(brand)
	return strings.ToUpper(string(runes[:2]))
}
