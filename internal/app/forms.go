// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/extract"
	"github.com/cpcloud/micasa/internal/llm"
)

type houseFormData struct {
	Nickname         string
	AddressLine1     string
	AddressLine2     string
	City             string
	State            string
	PostalCode       string
	YearBuilt        string
	SquareFeet       string
	LotSquareFeet    string
	Bedrooms         string
	Bathrooms        string
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
	InsuranceRenewal string
	PropertyTax      string
	HOAName          string
	HOAFee           string
}

type projectFormData struct {
	Title         string
	ProjectTypeID uint
	Status        string
	Budget        string
	Actual        string
	StartDate     string
	EndDate       string
	Description   string
}

type quoteFormData struct {
	ProjectID    uint
	VendorName   string
	ContactName  string
	Email        string
	Phone        string
	Website      string
	VendorNotes  string // not shown in UI; carried through to preserve existing value
	Total        string
	Labor        string
	Materials    string
	Other        string
	ReceivedDate string
	Notes        string
}

type maintenanceFormData struct {
	Name           string
	CategoryID     uint
	ApplianceID    uint // 0 means none
	LastServiced   string
	IntervalMonths string
	ManualURL      string
	ManualText     string
	Cost           string
	Notes          string
}

type serviceLogFormData struct {
	MaintenanceItemID uint
	ServicedAt        string
	VendorID          uint // 0 = self
	Cost              string
	Notes             string
}

type vendorFormData struct {
	Name        string
	ContactName string
	Email       string
	Phone       string
	Website     string
	Notes       string
}

// entityRef identifies a polymorphic document parent (kind + ID).
// The zero value represents "no entity".
type entityRef struct {
	Kind string
	ID   uint
}

type documentFormData struct {
	Title     string
	FilePath  string // local file path; read on submit for new documents
	EntityRef entityRef
	Notes     string
}

// documentParseResult holds the parsed document, extraction hints from the
// LLM (if available), and any non-fatal extraction error. The document is
// always valid when err is nil; extractErr captures extraction failures
// that should not block the upload.
type documentParseResult struct {
	Doc        data.Document
	Hints      *extract.ExtractionHints // nil when LLM unavailable or failed
	ExtractErr error
}

type incidentFormData struct {
	Title        string
	Description  string
	Status       string
	Severity     string
	DateNoticed  string
	DateResolved string
	Location     string
	Cost         string
	ApplianceID  uint // 0 means none
	VendorID     uint // 0 means none (self)
	Notes        string
}

type applianceFormData struct {
	Name           string
	Brand          string
	ModelNumber    string
	SerialNumber   string
	PurchaseDate   string
	WarrantyExpiry string
	Location       string
	Cost           string
	Notes          string
}

func (m *Model) startHouseForm() {
	values := &houseFormData{}
	if m.hasHouse {
		values = houseFormValues(m.house)
	}

	basicsGroup := huh.NewGroup(
		huh.NewInput().
			Title(requiredTitle("Nickname")).
			Description("Ex: Primary Residence").
			Value(&values.Nickname).
			Validate(requiredText("nickname")),
		huh.NewInput().Title("Address line 1").Value(&values.AddressLine1),
		huh.NewInput().Title("Address line 2").Value(&values.AddressLine2),
		huh.NewInput().Title("City").Value(&values.City),
		huh.NewInput().Title("State").Value(&values.State),
		huh.NewInput().Title("Postal code").Value(&values.PostalCode),
	).Title("Basics")
	if !m.hasHouse {
		basicsGroup.Description(
			"Only nickname is required -- edit the rest anytime with p (edit mode)")
	}

	form := huh.NewForm(
		basicsGroup,
		huh.NewGroup(
			huh.NewInput().
				Title("Year built").
				Placeholder("1998").
				Value(&values.YearBuilt).
				Validate(optionalInt("year built")),
			huh.NewInput().
				Title("Square feet").
				Placeholder("1820").
				Value(&values.SquareFeet).
				Validate(optionalInt("square feet")),
			huh.NewInput().
				Title("Lot square feet").
				Placeholder("7000").
				Value(&values.LotSquareFeet).
				Validate(optionalInt("lot square feet")),
			huh.NewInput().
				Title("Bedrooms").
				Placeholder("3").
				Value(&values.Bedrooms).
				Validate(optionalInt("bedrooms")),
			huh.NewInput().
				Title("Bathrooms").
				Placeholder("2.5").
				Value(&values.Bathrooms).
				Validate(optionalFloat("bathrooms")),
			huh.NewInput().Title("Foundation type").Value(&values.FoundationType),
			huh.NewInput().Title("Wiring type").Value(&values.WiringType),
			huh.NewInput().Title("Roof type").Value(&values.RoofType),
			huh.NewInput().Title("Exterior type").Value(&values.ExteriorType),
			huh.NewInput().Title("Basement type").Value(&values.BasementType),
		).Title("Structure"),
		huh.NewGroup(
			huh.NewInput().Title("Heating type").Value(&values.HeatingType),
			huh.NewInput().Title("Cooling type").Value(&values.CoolingType),
			huh.NewInput().Title("Water source").Value(&values.WaterSource),
			huh.NewInput().Title("Sewer type").Value(&values.SewerType),
			huh.NewInput().Title("Parking type").Value(&values.ParkingType),
		).Title("Utilities"),
		huh.NewGroup(
			huh.NewInput().Title("Insurance carrier").Value(&values.InsuranceCarrier),
			huh.NewInput().Title("Insurance policy").Value(&values.InsurancePolicy),
			huh.NewInput().
				Title("Insurance renewal (YYYY-MM-DD)").
				Value(&values.InsuranceRenewal).
				Validate(optionalDate("insurance renewal")),
			huh.NewInput().
				Title("Property tax (annual)").
				Placeholder("4200.00").
				Value(&values.PropertyTax).
				Validate(optionalMoney("property tax")),
			huh.NewInput().Title("HOA name").Value(&values.HOAName),
			huh.NewInput().
				Title("HOA fee (monthly)").
				Placeholder("250.00").
				Value(&values.HOAFee).
				Validate(optionalMoney("HOA fee")),
		).Title("Financial"),
	)
	formWidth := 60
	if m.width > 0 && m.width < formWidth+10 {
		formWidth = m.width - 10
	}
	form.WithWidth(formWidth)
	m.activateForm(formHouse, form, values)
}

func (m *Model) startProjectForm() {
	values := &projectFormData{
		Status: data.ProjectStatusPlanned,
	}
	options := projectTypeOptions(m.projectTypes)
	if len(options) > 0 {
		values.ProjectTypeID = options[0].Value
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Title")).
				Value(&values.Title).
				Validate(requiredText("title")),
			huh.NewSelect[uint]().
				Title("Project type").
				Options(options...).
				Value(&values.ProjectTypeID),
			huh.NewSelect[string]().
				Title("Status").
				Options(statusOptions()...).
				Value(&values.Status),
		),
	)
	m.activateForm(formProject, form, values)
}

func (m *Model) startEditProjectForm(id uint) error {
	project, err := m.store.GetProject(id)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	values := projectFormValues(project)
	options := projectTypeOptions(m.projectTypes)
	m.editID = &id
	m.openProjectForm(values, options)
	return nil
}

func (m *Model) openProjectForm(values *projectFormData, options []huh.Option[uint]) {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Title")).
				Value(&values.Title).
				Validate(requiredText("title")),
			huh.NewSelect[uint]().
				Title("Project type").
				Options(options...).
				Value(&values.ProjectTypeID),
			huh.NewSelect[string]().
				Title("Status").
				Options(statusOptions()...).
				Value(&values.Status),
			huh.NewInput().
				Title("Budget").
				Placeholder("1250.00").
				Value(&values.Budget).
				Validate(optionalMoney("budget")),
			huh.NewInput().
				Title("Actual cost").
				Placeholder("1400.00").
				Value(&values.Actual).
				Validate(optionalMoney("actual cost")),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Start date (YYYY-MM-DD)").
				Value(&values.StartDate).
				Validate(optionalDate("start date")),
			huh.NewInput().
				Title("End date (YYYY-MM-DD)").
				Value(&values.EndDate).
				Validate(endDateAfterStart(&values.StartDate, &values.EndDate)),
			huh.NewText().
				Title("Description").
				Value(&values.Description),
		).Title("Timeline"),
	)
	m.activateForm(formProject, form, values)
}

func (m *Model) startQuoteForm() error {
	projects, err := m.store.ListProjects(false)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return fmt.Errorf("add a project before adding quotes")
	}
	values := &quoteFormData{}
	options := projectOptions(projects)
	values.ProjectID = options[0].Value
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[uint]().
				Title("Project").
				Options(options...).
				Value(&values.ProjectID),
			huh.NewInput().
				Title(requiredTitle("Vendor name")).
				Value(&values.VendorName).
				Validate(requiredText("vendor name")),
			huh.NewInput().
				Title(requiredTitle("Total")).
				Placeholder("3250.00").
				Value(&values.Total).
				Validate(requiredMoney("total")),
		),
	)
	m.activateForm(formQuote, form, values)
	return nil
}

func (m *Model) startEditQuoteForm(id uint) error {
	quote, err := m.store.GetQuote(id)
	if err != nil {
		return fmt.Errorf("load quote: %w", err)
	}
	projects, err := m.store.ListProjects(false)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		return fmt.Errorf("no projects available")
	}
	values := quoteFormValues(quote)
	options := projectOptions(projects)
	m.editID = &id
	m.openQuoteForm(values, options)
	return nil
}

func (m *Model) openQuoteForm(values *quoteFormData, projectOpts []huh.Option[uint]) {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[uint]().
				Title("Project").
				Options(projectOpts...).
				Value(&values.ProjectID),
			huh.NewInput().
				Title(requiredTitle("Vendor name")).
				Value(&values.VendorName).
				Validate(requiredText("vendor name")),
			huh.NewInput().Title("Contact name").Value(&values.ContactName),
			huh.NewInput().Title("Email").Value(&values.Email),
			huh.NewInput().Title("Phone").Value(&values.Phone),
			huh.NewInput().Title("Website").Value(&values.Website),
		).Title("Vendor"),
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Total")).
				Placeholder("3250.00").
				Value(&values.Total).
				Validate(requiredMoney("total")),
			huh.NewInput().
				Title("Labor").
				Placeholder("2000.00").
				Value(&values.Labor).
				Validate(optionalMoney("labor")),
			huh.NewInput().
				Title("Materials").
				Placeholder("1000.00").
				Value(&values.Materials).
				Validate(optionalMoney("materials")),
			huh.NewInput().
				Title("Other").
				Placeholder("250.00").
				Value(&values.Other).
				Validate(optionalMoney("other costs")),
			huh.NewInput().
				Title("Received date (YYYY-MM-DD)").
				Value(&values.ReceivedDate).
				Validate(optionalDate("received date")),
			huh.NewText().Title("Notes").Value(&values.Notes),
		).Title("Quote"),
	)
	m.activateForm(formQuote, form, values)
}

func (m *Model) startMaintenanceForm() {
	values := &maintenanceFormData{}
	catOptions := maintenanceOptions(m.maintenanceCategories)
	if len(catOptions) > 0 {
		values.CategoryID = catOptions[0].Value
	}
	appliances, _ := m.store.ListAppliances(false)
	appOpts := applianceOptions(appliances)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Item")).
				Value(&values.Name).
				Validate(requiredText("item")),
			huh.NewSelect[uint]().
				Title("Category").
				Options(catOptions...).
				Value(&values.CategoryID),
			huh.NewSelect[uint]().
				Title("Appliance").
				Options(appOpts...).
				Value(&values.ApplianceID),
			huh.NewInput().
				Title("Interval").
				Description("e.g. 6 (months), 6m, 1y, 2y 6m; blank for one-time").
				Placeholder("6m").
				Value(&values.IntervalMonths).
				Validate(optionalInterval("interval")),
		),
	)
	m.activateForm(formMaintenance, form, values)
}

func (m *Model) startEditMaintenanceForm(id uint) error {
	item, err := m.store.GetMaintenance(id)
	if err != nil {
		return fmt.Errorf("load maintenance item: %w", err)
	}
	values := maintenanceFormValues(item)
	options := maintenanceOptions(m.maintenanceCategories)
	appliances, _ := m.store.ListAppliances(false)
	appOpts := applianceOptions(appliances)
	m.editID = &id
	m.openMaintenanceForm(values, options, appOpts)
	return nil
}

func (m *Model) openMaintenanceForm(
	values *maintenanceFormData,
	catOptions []huh.Option[uint],
	appOptions []huh.Option[uint],
) {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Item")).
				Value(&values.Name).
				Validate(requiredText("item")),
			huh.NewSelect[uint]().
				Title("Category").
				Options(catOptions...).
				Value(&values.CategoryID),
			huh.NewSelect[uint]().
				Title("Appliance").
				Options(appOptions...).
				Value(&values.ApplianceID),
			huh.NewInput().
				Title("Last serviced (YYYY-MM-DD)").
				Value(&values.LastServiced).
				Validate(optionalDate("last serviced")),
			huh.NewInput().
				Title("Interval").
				Description("e.g. 6 (months), 6m, 1y, 2y 6m; blank for one-time").
				Placeholder("6m").
				Value(&values.IntervalMonths).
				Validate(optionalInterval("interval")),
		).Title("Schedule"),
		huh.NewGroup(
			huh.NewInput().Title("Manual URL").Value(&values.ManualURL),
			huh.NewText().Title("Manual notes").Value(&values.ManualText),
			huh.NewInput().
				Title("Cost").
				Placeholder("125.00").
				Value(&values.Cost).
				Validate(optionalMoney("cost")),
			huh.NewText().Title("Notes").Value(&values.Notes),
		).Title("Details"),
	)
	m.activateForm(formMaintenance, form, values)
}

func (m *Model) startIncidentForm() {
	values := &incidentFormData{
		Status:      data.IncidentStatusOpen,
		Severity:    data.IncidentSeveritySoon,
		DateNoticed: time.Now().Format(data.DateLayout),
	}
	appliances, _ := m.store.ListAppliances(false)
	appOpts := applianceOptions(appliances)
	vendorOpts := optionalVendorOptions(m.vendors)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Title")).
				Value(&values.Title).
				Validate(requiredText("title")),
			huh.NewSelect[string]().
				Title("Severity").
				Options(incidentSeverityOptions()...).
				Value(&values.Severity),
			huh.NewInput().
				Title(requiredTitle("Date noticed")+" (YYYY-MM-DD)").
				Value(&values.DateNoticed).
				Validate(requiredDate("date noticed")),
			huh.NewInput().
				Title("Location").
				Placeholder("Kitchen").
				Value(&values.Location),
		).Title("Details"),
		huh.NewGroup(
			huh.NewSelect[uint]().
				Title("Appliance").
				Options(appOpts...).
				Value(&values.ApplianceID),
			huh.NewSelect[uint]().
				Title("Vendor").
				Options(vendorOpts...).
				Value(&values.VendorID),
		).Title("Links"),
	)
	m.activateForm(formIncident, form, values)
}

func (m *Model) startEditIncidentForm(id uint) error {
	item, err := m.store.GetIncident(id)
	if err != nil {
		return fmt.Errorf("load incident: %w", err)
	}
	values := incidentFormValues(item)
	appliances, _ := m.store.ListAppliances(false)
	appOpts := applianceOptions(appliances)
	vendorOpts := optionalVendorOptions(m.vendors)
	m.editID = &id
	m.openIncidentForm(values, appOpts, vendorOpts)
	return nil
}

func (m *Model) openIncidentForm(
	values *incidentFormData,
	appOptions []huh.Option[uint],
	vendorOptions []huh.Option[uint],
) {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Title")).
				Value(&values.Title).
				Validate(requiredText("title")),
			huh.NewSelect[string]().
				Title("Status").
				Options(incidentStatusOptions()...).
				Value(&values.Status),
			huh.NewSelect[string]().
				Title("Severity").
				Options(incidentSeverityOptions()...).
				Value(&values.Severity),
			huh.NewInput().
				Title(requiredTitle("Date noticed")+" (YYYY-MM-DD)").
				Value(&values.DateNoticed).
				Validate(requiredDate("date noticed")),
			huh.NewInput().
				Title("Date resolved (YYYY-MM-DD)").
				Value(&values.DateResolved).
				Validate(optionalDate("date resolved")),
			huh.NewInput().
				Title("Location").
				Placeholder("Kitchen").
				Value(&values.Location),
		).Title("Details"),
		huh.NewGroup(
			huh.NewSelect[uint]().
				Title("Appliance").
				Options(appOptions...).
				Value(&values.ApplianceID),
			huh.NewSelect[uint]().
				Title("Vendor").
				Options(vendorOptions...).
				Value(&values.VendorID),
			huh.NewInput().
				Title("Cost").
				Placeholder("250.00").
				Value(&values.Cost).
				Validate(optionalMoney("cost")),
			huh.NewText().Title("Description").Value(&values.Description),
			huh.NewText().Title("Notes").Value(&values.Notes),
		).Title("Context"),
	)
	m.activateForm(formIncident, form, values)
}

func (m *Model) submitIncidentForm() error {
	item, err := m.parseIncidentFormData()
	if err != nil {
		return err
	}
	if m.editID != nil {
		item.ID = *m.editID
		return m.store.UpdateIncident(item)
	}
	return m.store.CreateIncident(&item)
}

func (m *Model) parseIncidentFormData() (data.Incident, error) {
	values, ok := m.formData.(*incidentFormData)
	if !ok {
		return data.Incident{}, fmt.Errorf("unexpected incident form data")
	}
	noticed, err := data.ParseRequiredDate(values.DateNoticed)
	if err != nil {
		return data.Incident{}, err
	}
	resolved, err := data.ParseOptionalDate(values.DateResolved)
	if err != nil {
		return data.Incident{}, err
	}
	cost, err := data.ParseOptionalCents(values.Cost)
	if err != nil {
		return data.Incident{}, err
	}
	var appID *uint
	if values.ApplianceID > 0 {
		appID = &values.ApplianceID
	}
	var vendorID *uint
	if values.VendorID > 0 {
		vendorID = &values.VendorID
	}
	return data.Incident{
		Title:        strings.TrimSpace(values.Title),
		Description:  strings.TrimSpace(values.Description),
		Status:       values.Status,
		Severity:     values.Severity,
		DateNoticed:  noticed,
		DateResolved: resolved,
		Location:     strings.TrimSpace(values.Location),
		CostCents:    cost,
		ApplianceID:  appID,
		VendorID:     vendorID,
		Notes:        strings.TrimSpace(values.Notes),
	}, nil
}

func (m *Model) inlineEditIncident(id uint, col incidentCol) error {
	item, err := m.store.GetIncident(id)
	if err != nil {
		return fmt.Errorf("load incident: %w", err)
	}
	values := incidentFormValues(item)
	switch col {
	case incidentColTitle:
		m.openInlineInput(
			id,
			formIncident,
			"Title",
			"",
			&values.Title,
			requiredText("title"),
			values,
		)
	case incidentColStatus:
		field := huh.NewSelect[string]().Title("Status").
			Options(incidentStatusOptions()...).
			Value(&values.Status)
		m.openInlineEdit(id, formIncident, field, values)
	case incidentColSeverity:
		field := huh.NewSelect[string]().Title("Severity").
			Options(incidentSeverityOptions()...).
			Value(&values.Severity)
		m.openInlineEdit(id, formIncident, field, values)
	case incidentColLocation:
		m.openInlineInput(id, formIncident, "Location", "Kitchen", &values.Location, nil, values)
	case incidentColAppliance:
		appliances, loadErr := m.store.ListAppliances(false)
		if loadErr != nil {
			return loadErr
		}
		appOpts := applianceOptions(appliances)
		field := huh.NewSelect[uint]().Title("Appliance").
			Options(appOpts...).
			Value(&values.ApplianceID)
		m.openInlineEdit(id, formIncident, field, values)
	case incidentColVendor:
		vendorOpts := optionalVendorOptions(m.vendors)
		field := huh.NewSelect[uint]().Title("Vendor").
			Options(vendorOpts...).
			Value(&values.VendorID)
		m.openInlineEdit(id, formIncident, field, values)
	case incidentColNoticed:
		m.openDatePicker(id, formIncident, &values.DateNoticed, values)
	case incidentColResolved:
		m.openDatePicker(id, formIncident, &values.DateResolved, values)
	case incidentColCost:
		m.openInlineInput(
			id,
			formIncident,
			"Cost",
			"250.00",
			&values.Cost,
			optionalMoney("cost"),
			values,
		)
	case incidentColID, incidentColDocs:
		return m.startEditIncidentForm(id)
	}
	return nil
}

func incidentFormValues(item data.Incident) *incidentFormData {
	var appID uint
	if item.ApplianceID != nil {
		appID = *item.ApplianceID
	}
	var vendorID uint
	if item.VendorID != nil {
		vendorID = *item.VendorID
	}
	return &incidentFormData{
		Title:        item.Title,
		Description:  item.Description,
		Status:       item.Status,
		Severity:     item.Severity,
		DateNoticed:  item.DateNoticed.Format(data.DateLayout),
		DateResolved: data.FormatDate(item.DateResolved),
		Location:     item.Location,
		Cost:         data.FormatOptionalCents(item.CostCents),
		ApplianceID:  appID,
		VendorID:     vendorID,
		Notes:        item.Notes,
	}
}

func incidentStatusOptions() []huh.Option[string] {
	type entry struct {
		value string
		color lipgloss.AdaptiveColor
	}
	statuses := []entry{
		{data.IncidentStatusOpen, accent},
		{data.IncidentStatusInProgress, success},
	}
	opts := make([]huh.Option[string], len(statuses))
	for i, s := range statuses {
		label := statusLabel(s.value)
		colored := lipgloss.NewStyle().Foreground(s.color).Render(label)
		opts[i] = huh.NewOption(colored, s.value)
	}
	return withOrdinals(opts)
}

func incidentSeverityOptions() []huh.Option[string] {
	type entry struct {
		value string
		color lipgloss.AdaptiveColor
	}
	severities := []entry{
		{data.IncidentSeverityUrgent, danger},
		{data.IncidentSeveritySoon, warning},
		{data.IncidentSeverityWhenever, textDim},
	}
	opts := make([]huh.Option[string], len(severities))
	for i, s := range severities {
		label := statusLabel(s.value)
		colored := lipgloss.NewStyle().Foreground(s.color).Render(label)
		opts[i] = huh.NewOption(colored, s.value)
	}
	return withOrdinals(opts)
}

// optionalVendorOptions is like vendorOptions but with "(none)" instead of "Self".
func optionalVendorOptions(vendors []data.Vendor) []huh.Option[uint] {
	options := make([]huh.Option[uint], 0, len(vendors)+1)
	options = append(options, huh.NewOption("(none)", uint(0)))
	for _, v := range vendors {
		label := v.Name
		if v.ContactName != "" {
			label = fmt.Sprintf("%s (%s)", v.Name, v.ContactName)
		}
		options = append(options, huh.NewOption(label, v.ID))
	}
	return withOrdinals(options)
}

func (m *Model) startApplianceForm() {
	values := &applianceFormData{}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Name")).
				Placeholder("Kitchen Refrigerator").
				Value(&values.Name).
				Validate(requiredText("name")),
		),
	)
	m.activateForm(formAppliance, form, values)
}

func (m *Model) startEditApplianceForm(id uint) error {
	item, err := m.store.GetAppliance(id)
	if err != nil {
		return fmt.Errorf("load appliance: %w", err)
	}
	values := applianceFormValues(item)
	m.editID = &id
	m.openApplianceForm(values)
	return nil
}

func (m *Model) openApplianceForm(values *applianceFormData) {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Name")).
				Placeholder("Kitchen Refrigerator").
				Value(&values.Name).
				Validate(requiredText("name")),
			huh.NewInput().Title("Brand").Value(&values.Brand),
			huh.NewInput().Title("Model number").Value(&values.ModelNumber),
			huh.NewInput().Title("Serial number").Value(&values.SerialNumber),
			huh.NewInput().Title("Location").Placeholder("Kitchen").Value(&values.Location),
		).Title("Identity"),
		huh.NewGroup(
			huh.NewInput().
				Title("Purchase date (YYYY-MM-DD)").
				Value(&values.PurchaseDate).
				Validate(optionalDate("purchase date")),
			huh.NewInput().
				Title("Warranty expiry (YYYY-MM-DD)").
				Value(&values.WarrantyExpiry).
				Validate(optionalDate("warranty expiry")),
			huh.NewInput().
				Title("Cost").
				Placeholder("899.00").
				Value(&values.Cost).
				Validate(optionalMoney("cost")),
			huh.NewText().Title("Notes").Value(&values.Notes),
		).Title("Details"),
	)
	m.activateForm(formAppliance, form, values)
}

func (m *Model) submitApplianceForm() error {
	item, err := m.parseApplianceFormData()
	if err != nil {
		return err
	}
	if m.editID != nil {
		item.ID = *m.editID
		return m.store.UpdateAppliance(item)
	}
	if err := m.store.CreateAppliance(&item); err != nil {
		return err
	}
	id := item.ID
	m.editID = &id
	return nil
}

func (m *Model) parseApplianceFormData() (data.Appliance, error) {
	values, ok := m.formData.(*applianceFormData)
	if !ok {
		return data.Appliance{}, fmt.Errorf("unexpected appliance form data")
	}
	purchaseDate, err := data.ParseOptionalDate(values.PurchaseDate)
	if err != nil {
		return data.Appliance{}, err
	}
	warrantyExpiry, err := data.ParseOptionalDate(values.WarrantyExpiry)
	if err != nil {
		return data.Appliance{}, err
	}
	cost, err := data.ParseOptionalCents(values.Cost)
	if err != nil {
		return data.Appliance{}, err
	}
	return data.Appliance{
		Name:           strings.TrimSpace(values.Name),
		Brand:          strings.TrimSpace(values.Brand),
		ModelNumber:    strings.TrimSpace(values.ModelNumber),
		SerialNumber:   strings.TrimSpace(values.SerialNumber),
		PurchaseDate:   purchaseDate,
		WarrantyExpiry: warrantyExpiry,
		Location:       strings.TrimSpace(values.Location),
		CostCents:      cost,
		Notes:          strings.TrimSpace(values.Notes),
	}, nil
}

func (m *Model) startVendorForm() {
	values := &vendorFormData{}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Name")).
				Placeholder("Acme Plumbing").
				Value(&values.Name).
				Validate(requiredText("name")),
		),
	)
	m.activateForm(formVendor, form, values)
}

func (m *Model) startEditVendorForm(id uint) error {
	vendor, err := m.store.GetVendor(id)
	if err != nil {
		return fmt.Errorf("load vendor: %w", err)
	}
	values := vendorFormValues(vendor)
	m.editID = &id
	m.openVendorForm(values)
	return nil
}

func (m *Model) openVendorForm(values *vendorFormData) {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Name")).
				Placeholder("Acme Plumbing").
				Value(&values.Name).
				Validate(requiredText("name")),
			huh.NewInput().Title("Contact name").Value(&values.ContactName),
			huh.NewInput().Title("Email").Value(&values.Email),
			huh.NewInput().Title("Phone").Value(&values.Phone),
			huh.NewInput().Title("Website").Value(&values.Website),
			huh.NewText().Title("Notes").Value(&values.Notes),
		),
	)
	m.activateForm(formVendor, form, values)
}

func (m *Model) submitVendorForm() error {
	vendor, err := m.parseVendorFormData()
	if err != nil {
		return err
	}
	if m.editID != nil {
		vendor.ID = *m.editID
		return m.store.UpdateVendor(vendor)
	}
	if err := m.store.CreateVendor(&vendor); err != nil {
		return err
	}
	id := vendor.ID
	m.editID = &id
	return nil
}

func (m *Model) parseVendorFormData() (data.Vendor, error) {
	values, ok := m.formData.(*vendorFormData)
	if !ok {
		return data.Vendor{}, fmt.Errorf("unexpected vendor form data")
	}
	return data.Vendor{
		Name:        strings.TrimSpace(values.Name),
		ContactName: strings.TrimSpace(values.ContactName),
		Email:       strings.TrimSpace(values.Email),
		Phone:       strings.TrimSpace(values.Phone),
		Website:     strings.TrimSpace(values.Website),
		Notes:       strings.TrimSpace(values.Notes),
	}, nil
}

func (m *Model) inlineEditVendor(id uint, col vendorCol) error {
	vendor, err := m.store.GetVendor(id)
	if err != nil {
		return fmt.Errorf("load vendor: %w", err)
	}
	values := vendorFormValues(vendor)
	switch col {
	case vendorColName:
		m.openInlineInput(id, formVendor, "Name", "", &values.Name, requiredText("name"), values)
	case vendorColContact:
		m.openInlineInput(id, formVendor, "Contact name", "", &values.ContactName, nil, values)
	case vendorColEmail:
		m.openInlineInput(id, formVendor, "Email", "", &values.Email, nil, values)
	case vendorColPhone:
		m.openInlineInput(id, formVendor, "Phone", "", &values.Phone, nil, values)
	case vendorColWebsite:
		m.openInlineInput(id, formVendor, "Website", "", &values.Website, nil, values)
	case vendorColID, vendorColQuotes, vendorColJobs, vendorColDocs:
		return m.startEditVendorForm(id)
	}
	return nil
}

func vendorFormValues(vendor data.Vendor) *vendorFormData {
	return &vendorFormData{
		Name:        vendor.Name,
		ContactName: vendor.ContactName,
		Email:       vendor.Email,
		Phone:       vendor.Phone,
		Website:     vendor.Website,
		Notes:       vendor.Notes,
	}
}

func (m *Model) inlineEditProject(id uint, col projectCol) error {
	project, err := m.store.GetProject(id)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	values := projectFormValues(project)
	switch col {
	case projectColType:
		options := projectTypeOptions(m.projectTypes)
		field := huh.NewSelect[uint]().Title("Project type").
			Options(options...).
			Value(&values.ProjectTypeID)
		m.openInlineEdit(id, formProject, field, values)
	case projectColTitle:
		m.openInlineInput(
			id,
			formProject,
			"Title",
			"",
			&values.Title,
			requiredText("title"),
			values,
		)
	case projectColStatus:
		field := huh.NewSelect[string]().Title("Status").
			Options(statusOptions()...).
			Value(&values.Status)
		m.openInlineEdit(id, formProject, field, values)
	case projectColBudget:
		m.openInlineInput(
			id,
			formProject,
			"Budget",
			"1250.00",
			&values.Budget,
			optionalMoney("budget"),
			values,
		)
	case projectColActual:
		m.openInlineInput(
			id,
			formProject,
			"Actual cost",
			"1400.00",
			&values.Actual,
			optionalMoney("actual cost"),
			values,
		)
	case projectColStart:
		m.openDatePicker(id, formProject, &values.StartDate, values)
	case projectColEnd:
		m.openDatePicker(id, formProject, &values.EndDate, values)
	case projectColID, projectColQuotes, projectColDocs:
		return m.startEditProjectForm(id)
	}
	return nil
}

func (m *Model) inlineEditQuote(id uint, col quoteCol) error {
	quote, err := m.store.GetQuote(id)
	if err != nil {
		return fmt.Errorf("load quote: %w", err)
	}
	projects, err := m.store.ListProjects(false)
	if err != nil {
		return err
	}
	values := quoteFormValues(quote)
	switch col {
	case quoteColProject:
		projectOpts := projectOptions(projects)
		field := huh.NewSelect[uint]().Title("Project").
			Options(projectOpts...).
			Value(&values.ProjectID)
		m.openInlineEdit(id, formQuote, field, values)
	case quoteColVendor:
		m.openInlineInput(
			id,
			formQuote,
			"Vendor name",
			"",
			&values.VendorName,
			requiredText("vendor name"),
			values,
		)
	case quoteColTotal:
		m.openInlineInput(
			id,
			formQuote,
			"Total",
			"3250.00",
			&values.Total,
			requiredMoney("total"),
			values,
		)
	case quoteColLabor:
		m.openInlineInput(
			id,
			formQuote,
			"Labor",
			"2000.00",
			&values.Labor,
			optionalMoney("labor"),
			values,
		)
	case quoteColMat:
		m.openInlineInput(
			id,
			formQuote,
			"Materials",
			"1000.00",
			&values.Materials,
			optionalMoney("materials"),
			values,
		)
	case quoteColOther:
		m.openInlineInput(
			id,
			formQuote,
			"Other",
			"250.00",
			&values.Other,
			optionalMoney("other costs"),
			values,
		)
	case quoteColRecv:
		m.openDatePicker(id, formQuote, &values.ReceivedDate, values)
	case quoteColID, quoteColDocs:
		return m.startEditQuoteForm(id)
	}
	return nil
}

func (m *Model) inlineEditMaintenance(id uint, col maintenanceCol) error {
	item, err := m.store.GetMaintenance(id)
	if err != nil {
		return fmt.Errorf("load maintenance item: %w", err)
	}
	values := maintenanceFormValues(item)
	switch col {
	case maintenanceColItem:
		m.openInlineInput(
			id,
			formMaintenance,
			"Item",
			"",
			&values.Name,
			requiredText("item"),
			values,
		)
	case maintenanceColCategory:
		catOptions := maintenanceOptions(m.maintenanceCategories)
		field := huh.NewSelect[uint]().Title("Category").
			Options(catOptions...).
			Value(&values.CategoryID)
		m.openInlineEdit(id, formMaintenance, field, values)
	case maintenanceColAppliance:
		appliances, loadErr := m.store.ListAppliances(false)
		if loadErr != nil {
			return loadErr
		}
		appOpts := applianceOptions(appliances)
		field := huh.NewSelect[uint]().Title("Appliance").
			Options(appOpts...).
			Value(&values.ApplianceID)
		m.openInlineEdit(id, formMaintenance, field, values)
	case maintenanceColLast:
		m.openDatePicker(id, formMaintenance, &values.LastServiced, values)
	case maintenanceColEvery:
		m.openInlineInput(
			id,
			formMaintenance,
			"Interval",
			"6m",
			&values.IntervalMonths,
			optionalInterval("interval"),
			values,
		)
	case maintenanceColID, maintenanceColNext, maintenanceColLog, maintenanceColDocs:
		return m.startEditMaintenanceForm(id)
	}
	return nil
}

func (m *Model) inlineEditAppliance(id uint, col applianceCol) error {
	item, err := m.store.GetAppliance(id)
	if err != nil {
		return fmt.Errorf("load appliance: %w", err)
	}
	values := applianceFormValues(item)
	switch col {
	case applianceColName:
		m.openInlineInput(id, formAppliance, "Name", "", &values.Name, requiredText("name"), values)
	case applianceColBrand:
		m.openInlineInput(id, formAppliance, "Brand", "", &values.Brand, nil, values)
	case applianceColModel:
		m.openInlineInput(id, formAppliance, "Model number", "", &values.ModelNumber, nil, values)
	case applianceColSerial:
		m.openInlineInput(id, formAppliance, "Serial number", "", &values.SerialNumber, nil, values)
	case applianceColLocation:
		m.openInlineInput(id, formAppliance, "Location", "Kitchen", &values.Location, nil, values)
	case applianceColPurchased:
		m.openDatePicker(id, formAppliance, &values.PurchaseDate, values)
	case applianceColWarranty:
		m.openDatePicker(id, formAppliance, &values.WarrantyExpiry, values)
	case applianceColCost:
		m.openInlineInput(
			id,
			formAppliance,
			"Cost",
			"899.00",
			&values.Cost,
			optionalMoney("cost"),
			values,
		)
	case applianceColID, applianceColAge, applianceColMaint, applianceColDocs:
		return m.startEditApplianceForm(id)
	}
	return nil
}

func (m *Model) startServiceLogForm(maintenanceItemID uint) error {
	values := &serviceLogFormData{
		MaintenanceItemID: maintenanceItemID,
		ServicedAt:        time.Now().Format(data.DateLayout),
	}
	vendorOpts := vendorOptions(m.vendors)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Date serviced")+" (YYYY-MM-DD)").
				Value(&values.ServicedAt).
				Validate(requiredDate("date serviced")),
			huh.NewSelect[uint]().
				Title("Performed by").
				Options(vendorOpts...).
				Value(&values.VendorID),
		),
	)
	m.activateForm(formServiceLog, form, values)
	return nil
}

func (m *Model) startEditServiceLogForm(id uint) error {
	entry, err := m.store.GetServiceLog(id)
	if err != nil {
		return fmt.Errorf("load service log: %w", err)
	}
	values := serviceLogFormValues(entry)
	vendorOpts := vendorOptions(m.vendors)
	m.editID = &id
	m.openServiceLogForm(values, vendorOpts)
	return nil
}

func (m *Model) openServiceLogForm(
	values *serviceLogFormData,
	vendorOpts []huh.Option[uint],
) {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(requiredTitle("Date serviced")+" (YYYY-MM-DD)").
				Value(&values.ServicedAt).
				Validate(requiredDate("date serviced")),
			huh.NewSelect[uint]().
				Title("Performed by").
				Options(vendorOpts...).
				Value(&values.VendorID),
			huh.NewInput().
				Title("Cost").
				Placeholder("125.00").
				Value(&values.Cost).
				Validate(optionalMoney("cost")),
			huh.NewText().Title("Notes").Value(&values.Notes),
		),
	)
	m.activateForm(formServiceLog, form, values)
}

func (m *Model) submitServiceLogForm() error {
	entry, vendor, err := m.parseServiceLogFormData()
	if err != nil {
		return err
	}
	if m.editID != nil {
		entry.ID = *m.editID
		return m.store.UpdateServiceLog(entry, vendor)
	}
	if err := m.store.CreateServiceLog(&entry, vendor); err != nil {
		return err
	}
	id := entry.ID
	m.editID = &id
	return nil
}

func (m *Model) parseServiceLogFormData() (data.ServiceLogEntry, data.Vendor, error) {
	values, ok := m.formData.(*serviceLogFormData)
	if !ok {
		return data.ServiceLogEntry{}, data.Vendor{}, fmt.Errorf("unexpected service log form data")
	}
	servicedAt, err := data.ParseRequiredDate(values.ServicedAt)
	if err != nil {
		return data.ServiceLogEntry{}, data.Vendor{}, err
	}
	cost, err := data.ParseOptionalCents(values.Cost)
	if err != nil {
		return data.ServiceLogEntry{}, data.Vendor{}, err
	}
	entry := data.ServiceLogEntry{
		MaintenanceItemID: values.MaintenanceItemID,
		ServicedAt:        servicedAt,
		CostCents:         cost,
		Notes:             strings.TrimSpace(values.Notes),
	}
	var vendor data.Vendor
	if values.VendorID > 0 {
		// Look up the vendor to pass to CreateServiceLog/UpdateServiceLog.
		for _, v := range m.vendors {
			if v.ID == values.VendorID {
				vendor = v
				break
			}
		}
	}
	return entry, vendor, nil
}

func (m *Model) inlineEditServiceLog(id uint, col serviceLogCol) error {
	entry, err := m.store.GetServiceLog(id)
	if err != nil {
		return fmt.Errorf("load service log: %w", err)
	}
	values := serviceLogFormValues(entry)
	switch col {
	case serviceLogColDate:
		m.openDatePicker(id, formServiceLog, &values.ServicedAt, values)
	case serviceLogColPerformedBy:
		vendorOpts := vendorOptions(m.vendors)
		field := huh.NewSelect[uint]().
			Title("Performed by").
			Options(vendorOpts...).
			Value(&values.VendorID)
		m.openInlineEdit(id, formServiceLog, field, values)
	case serviceLogColCost:
		m.openInlineInput(
			id,
			formServiceLog,
			"Cost",
			"125.00",
			&values.Cost,
			optionalMoney("cost"),
			values,
		)
	case serviceLogColNotes:
		m.openNotesEdit(id, formServiceLog, &values.Notes, values)
	case serviceLogColID, serviceLogColDocs:
		return m.startEditServiceLogForm(id)
	}
	return nil
}

func serviceLogFormValues(entry data.ServiceLogEntry) *serviceLogFormData {
	var vendorID uint
	if entry.VendorID != nil {
		vendorID = *entry.VendorID
	}
	return &serviceLogFormData{
		MaintenanceItemID: entry.MaintenanceItemID,
		ServicedAt:        entry.ServicedAt.Format(data.DateLayout),
		VendorID:          vendorID,
		Cost:              data.FormatOptionalCents(entry.CostCents),
		Notes:             entry.Notes,
	}
}

func vendorOptions(vendors []data.Vendor) []huh.Option[uint] {
	options := make([]huh.Option[uint], 0, len(vendors)+1)
	options = append(options, huh.NewOption("Self (homeowner)", uint(0)))
	for _, v := range vendors {
		label := v.Name
		if v.ContactName != "" {
			label = fmt.Sprintf("%s (%s)", v.Name, v.ContactName)
		}
		options = append(options, huh.NewOption(label, v.ID))
	}
	return withOrdinals(options)
}

func requiredDate(label string) func(string) error {
	return func(input string) error {
		if strings.TrimSpace(input) == "" {
			return fmt.Errorf("%s is required", label)
		}
		if _, err := data.ParseRequiredDate(input); err != nil {
			return fmt.Errorf("%s should be YYYY-MM-DD", label)
		}
		return nil
	}
}

func applianceOptions(appliances []data.Appliance) []huh.Option[uint] {
	options := make([]huh.Option[uint], 0, len(appliances)+1)
	options = append(options, huh.NewOption("(none)", uint(0)))
	for _, appliance := range appliances {
		label := appliance.Name
		if appliance.Brand != "" {
			label = fmt.Sprintf("%s (%s)", appliance.Name, appliance.Brand)
		}
		options = append(options, huh.NewOption(label, appliance.ID))
	}
	return withOrdinals(options)
}

// entityOptionLabel colors the entire label using the kind's color from the
// Entity column palette.
func entityOptionLabel(kind, label string) string {
	letter, ok := entityKindLetter[kind]
	if !ok {
		return label
	}
	if s, ok := entityKindLetterStyles[letter[0]]; ok {
		return s.Render(label)
	}
	return label
}

// documentEntityOptions builds a flat option list of all active entities that
// a document can be linked to. Options are grouped by kind with descriptive
// labels. The first option is always "(none)" for unlinked documents.
func (m *Model) documentEntityOptions() ([]huh.Option[entityRef], error) {
	none := entityRef{}
	opts := []huh.Option[entityRef]{huh.NewOption("(none)", none)}

	// Appliances
	appliances, err := m.store.ListAppliances(false)
	if err != nil {
		return nil, fmt.Errorf("list appliances: %w", err)
	}
	for _, a := range appliances {
		label := a.Name
		if a.Brand != "" {
			label += " (" + a.Brand + ")"
		}
		opts = append(opts, huh.NewOption(
			entityOptionLabel(data.DocumentEntityAppliance, label),
			entityRef{Kind: data.DocumentEntityAppliance, ID: a.ID},
		))
	}

	// Incidents
	incidents, err := m.store.ListIncidents(false)
	if err != nil {
		return nil, fmt.Errorf("list incidents: %w", err)
	}
	for _, inc := range incidents {
		opts = append(opts, huh.NewOption(
			entityOptionLabel(data.DocumentEntityIncident, inc.Title),
			entityRef{Kind: data.DocumentEntityIncident, ID: inc.ID},
		))
	}

	// Maintenance items
	items, err := m.store.ListMaintenance(false)
	if err != nil {
		return nil, fmt.Errorf("list maintenance: %w", err)
	}
	for _, item := range items {
		opts = append(opts, huh.NewOption(
			entityOptionLabel(data.DocumentEntityMaintenance, item.Name),
			entityRef{Kind: data.DocumentEntityMaintenance, ID: item.ID},
		))
	}

	// Projects
	projects, err := m.store.ListProjects(false)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	for _, p := range projects {
		opts = append(opts, huh.NewOption(
			entityOptionLabel(data.DocumentEntityProject, p.Title),
			entityRef{Kind: data.DocumentEntityProject, ID: p.ID},
		))
	}

	// Quotes
	quotes, err := m.store.ListQuotes(false)
	if err != nil {
		return nil, fmt.Errorf("list quotes: %w", err)
	}
	for _, q := range quotes {
		label := fmt.Sprintf("%s / %s", q.Project.Title, q.Vendor.Name)
		opts = append(opts, huh.NewOption(
			entityOptionLabel(data.DocumentEntityQuote, label),
			entityRef{Kind: data.DocumentEntityQuote, ID: q.ID},
		))
	}

	// Vendors
	for _, v := range m.vendors {
		label := v.Name
		if v.ContactName != "" {
			label += " (" + v.ContactName + ")"
		}
		opts = append(opts, huh.NewOption(
			entityOptionLabel(data.DocumentEntityVendor, label),
			entityRef{Kind: data.DocumentEntityVendor, ID: v.ID},
		))
	}

	return withOrdinals(opts), nil
}

// openDatePicker opens the calendar picker for an inline date edit.
// When the user picks a date, the form data is saved via the handler.
func (m *Model) openDatePicker(
	id uint,
	kind FormKind,
	dateField *string,
	values any,
) {
	m.editID = &id
	m.formKind = kind
	m.formData = values
	savedKind := kind
	m.openCalendar(dateField, func() {
		m.snapshotForUndo()
		if err := m.handleFormSubmit(); err != nil {
			m.setStatusError(err.Error())
		} else {
			m.setStatusSaved(true) // calendar inline edits are always edits
			m.reloadAfterFormSave(savedKind)
		}
		m.formKind = formNone
		m.formData = nil
		m.editID = nil
	})
}

// activateForm applies defaults and switches the model into form mode.
// All form-opening paths should call this instead of duplicating the epilogue.
//
// The form width is set to the terminal width before Init so that group
// layouts match the actual terminal from the very first frame. huh's
// Form.Init defers a tea.WindowSize() that would recalculate widths and
// equalize group heights one frame late, causing a visible jump; updateForm
// blocks that deferred message so neither width nor height changes after the
// initial render.
func (m *Model) activateForm(kind FormKind, form *huh.Form, values any) {
	applyFormDefaults(form)
	// Set form width before Init so groups render at the correct terminal
	// width immediately. Without this, groups start at the default 80
	// columns and jump when the deferred WindowSizeMsg arrives.
	// The house form sets its own narrower width before calling
	// activateForm, so skip the override for that case.
	if m.width > 0 && kind != formHouse {
		form.WithWidth(m.width)
	}
	m.prevMode = m.mode
	m.mode = modeForm
	m.formKind = kind
	m.form = form
	m.formData = values
	m.formHasRequired = true
	m.pendingFormInit = form.Init()
	m.snapshotForm()
}

// openInlineEdit sets up a single-field inline edit form (overlay).
// Used for Select fields where a list picker is needed.
func (m *Model) openInlineEdit(id uint, kind FormKind, field huh.Field, values any) {
	m.editID = &id
	m.activateForm(kind, huh.NewForm(huh.NewGroup(field)), values)
	m.formHasRequired = false
}

// openNotesEdit opens a standalone textarea overlay for editing a notes field.
// On submit the form data is saved via the handler, just like openInlineEdit
// for select fields. The textarea supports ctrl+e to escalate to $EDITOR.
func (m *Model) openNotesEdit(id uint, kind FormKind, fieldPtr *string, values any) {
	m.editID = &id
	m.formData = values
	m.openNotesTextarea(kind, fieldPtr, values)
}

// openNotesTextarea creates and activates a textarea form for notes editing.
// Separated from openNotesEdit so it can be reused when reopening after an
// external editor session.
func (m *Model) openNotesTextarea(kind FormKind, fieldPtr *string, values any) {
	field := huh.NewText().Title("Notes").Value(fieldPtr)
	form := huh.NewForm(huh.NewGroup(field))
	m.activateForm(kind, form, values)
	m.formHasRequired = false
	m.notesEditMode = true
	m.notesFieldPtr = fieldPtr
}

// openInlineInput sets up a single-field text edit rendered in the status bar,
// keeping the table visible. Used for simple text and number fields.
func (m *Model) openInlineInput(
	id uint,
	kind FormKind,
	title, placeholder string,
	fieldPtr *string,
	validate func(string) error,
	values any,
) {
	ti := textinput.New()
	ti.SetValue(*fieldPtr)
	ti.Placeholder = placeholder
	ti.Focus()
	ti.Prompt = ""
	ti.CharLimit = 256
	m.editID = &id
	m.formKind = kind
	m.formData = values
	m.inlineInput = &inlineInputState{
		Input:    ti,
		Title:    title,
		EditID:   id,
		FormKind: kind,
		FormData: values,
		FieldPtr: fieldPtr,
		Validate: validate,
	}
}

func applyFormDefaults(form *huh.Form) {
	form.WithShowErrors(true)
	form.WithKeyMap(formKeyMap())

	form.WithTheme(formTheme())
}

// formTheme builds a huh form theme using the app's Wong palette.
func formTheme() *huh.Theme {
	t := huh.ThemeBase()

	marker := lipgloss.NewStyle().
		SetString(" ∗").
		Foreground(secondary)

	// Focused field styles.
	t.Focused.Base = t.Focused.Base.BorderForeground(border)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(accent).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(accent).Bold(true).MarginBottom(1)
	t.Focused.Description = t.Focused.Description.Foreground(textDim)
	t.Focused.ErrorIndicator = marker
	t.Focused.ErrorMessage = marker
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(accent)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(accent)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(accent)
	t.Focused.Option = t.Focused.Option.Foreground(textBright)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(accent)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(success)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(success).SetString("[•] ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(textMid).SetString("[ ] ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(textBright)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(onAccent).Background(accent)
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(textMid).Background(surface)

	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(accent)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(textDim)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(accent)

	// Blurred inherits focused, then dims.
	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.Title = t.Blurred.Title.Foreground(textMid).Bold(false)
	t.Blurred.NoteTitle = t.Blurred.NoteTitle.Foreground(textMid).Bold(false)
	t.Blurred.TextInput.Prompt = t.Blurred.TextInput.Prompt.Foreground(textDim)
	t.Blurred.TextInput.Text = t.Blurred.TextInput.Text.Foreground(textMid)
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description

	return t
}

func formKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Quit.SetKeys("esc")
	keymap.Quit.SetHelp("esc", "cancel")
	return keymap
}

func (m *Model) handleFormSubmit() error {
	if m.formKind == formHouse {
		return m.submitHouseForm()
	}
	handler := m.handlerForFormKind(m.formKind)
	if handler == nil {
		return fmt.Errorf("no handler for form kind %v", m.formKind)
	}
	return handler.SubmitForm(m)
}

func (m *Model) submitHouseForm() error {
	values, ok := m.formData.(*houseFormData)
	if !ok {
		return fmt.Errorf("unexpected house form data")
	}
	yearBuilt, err := data.ParseOptionalInt(values.YearBuilt)
	if err != nil {
		return err
	}
	sqft, err := data.ParseOptionalInt(values.SquareFeet)
	if err != nil {
		return err
	}
	lotSqft, err := data.ParseOptionalInt(values.LotSquareFeet)
	if err != nil {
		return err
	}
	bedrooms, err := data.ParseOptionalInt(values.Bedrooms)
	if err != nil {
		return err
	}
	bathrooms, err := data.ParseOptionalFloat(values.Bathrooms)
	if err != nil {
		return err
	}
	insuranceRenewal, err := data.ParseOptionalDate(values.InsuranceRenewal)
	if err != nil {
		return err
	}
	propertyTax, err := data.ParseOptionalCents(values.PropertyTax)
	if err != nil {
		return err
	}
	hoaFee, err := data.ParseOptionalCents(values.HOAFee)
	if err != nil {
		return err
	}
	profile := data.HouseProfile{
		Nickname:         strings.TrimSpace(values.Nickname),
		AddressLine1:     strings.TrimSpace(values.AddressLine1),
		AddressLine2:     strings.TrimSpace(values.AddressLine2),
		City:             strings.TrimSpace(values.City),
		State:            strings.TrimSpace(values.State),
		PostalCode:       strings.TrimSpace(values.PostalCode),
		YearBuilt:        yearBuilt,
		SquareFeet:       sqft,
		LotSquareFeet:    lotSqft,
		Bedrooms:         bedrooms,
		Bathrooms:        bathrooms,
		FoundationType:   strings.TrimSpace(values.FoundationType),
		WiringType:       strings.TrimSpace(values.WiringType),
		RoofType:         strings.TrimSpace(values.RoofType),
		ExteriorType:     strings.TrimSpace(values.ExteriorType),
		HeatingType:      strings.TrimSpace(values.HeatingType),
		CoolingType:      strings.TrimSpace(values.CoolingType),
		WaterSource:      strings.TrimSpace(values.WaterSource),
		SewerType:        strings.TrimSpace(values.SewerType),
		ParkingType:      strings.TrimSpace(values.ParkingType),
		BasementType:     strings.TrimSpace(values.BasementType),
		InsuranceCarrier: strings.TrimSpace(values.InsuranceCarrier),
		InsurancePolicy:  strings.TrimSpace(values.InsurancePolicy),
		InsuranceRenewal: insuranceRenewal,
		PropertyTaxCents: propertyTax,
		HOAName:          strings.TrimSpace(values.HOAName),
		HOAFeeCents:      hoaFee,
	}
	if m.hasHouse {
		if err := m.store.UpdateHouseProfile(profile); err != nil {
			return err
		}
	} else {
		if err := m.store.CreateHouseProfile(profile); err != nil {
			return err
		}
	}
	m.house = profile
	m.hasHouse = true
	return nil
}

func (m *Model) submitProjectForm() error {
	project, err := m.parseProjectFormData()
	if err != nil {
		return err
	}
	if m.editID != nil {
		project.ID = *m.editID
		return m.store.UpdateProject(project)
	}
	if err := m.store.CreateProject(&project); err != nil {
		return err
	}
	id := project.ID
	m.editID = &id
	return nil
}

func (m *Model) parseProjectFormData() (data.Project, error) {
	values, ok := m.formData.(*projectFormData)
	if !ok {
		return data.Project{}, fmt.Errorf("unexpected project form data")
	}
	budget, err := data.ParseOptionalCents(values.Budget)
	if err != nil {
		return data.Project{}, err
	}
	actual, err := data.ParseOptionalCents(values.Actual)
	if err != nil {
		return data.Project{}, err
	}
	startDate, err := data.ParseOptionalDate(values.StartDate)
	if err != nil {
		return data.Project{}, err
	}
	endDate, err := data.ParseOptionalDate(values.EndDate)
	if err != nil {
		return data.Project{}, err
	}
	return data.Project{
		Title:         strings.TrimSpace(values.Title),
		ProjectTypeID: values.ProjectTypeID,
		Status:        values.Status,
		Description:   strings.TrimSpace(values.Description),
		StartDate:     startDate,
		EndDate:       endDate,
		BudgetCents:   budget,
		ActualCents:   actual,
	}, nil
}

func (m *Model) submitQuoteForm() error {
	quote, vendor, err := m.parseQuoteFormData()
	if err != nil {
		return err
	}
	if m.editID != nil {
		quote.ID = *m.editID
		return m.store.UpdateQuote(quote, vendor)
	}
	if err := m.store.CreateQuote(&quote, vendor); err != nil {
		return err
	}
	id := quote.ID
	m.editID = &id
	return nil
}

func (m *Model) parseQuoteFormData() (data.Quote, data.Vendor, error) {
	values, ok := m.formData.(*quoteFormData)
	if !ok {
		return data.Quote{}, data.Vendor{}, fmt.Errorf("unexpected quote form data")
	}
	total, err := data.ParseRequiredCents(values.Total)
	if err != nil {
		return data.Quote{}, data.Vendor{}, err
	}
	labor, err := data.ParseOptionalCents(values.Labor)
	if err != nil {
		return data.Quote{}, data.Vendor{}, err
	}
	materials, err := data.ParseOptionalCents(values.Materials)
	if err != nil {
		return data.Quote{}, data.Vendor{}, err
	}
	other, err := data.ParseOptionalCents(values.Other)
	if err != nil {
		return data.Quote{}, data.Vendor{}, err
	}
	received, err := data.ParseOptionalDate(values.ReceivedDate)
	if err != nil {
		return data.Quote{}, data.Vendor{}, err
	}
	quote := data.Quote{
		ProjectID:      values.ProjectID,
		TotalCents:     total,
		LaborCents:     labor,
		MaterialsCents: materials,
		OtherCents:     other,
		ReceivedDate:   received,
		Notes:          strings.TrimSpace(values.Notes),
	}
	vendor := data.Vendor{
		Name:        strings.TrimSpace(values.VendorName),
		ContactName: strings.TrimSpace(values.ContactName),
		Email:       strings.TrimSpace(values.Email),
		Phone:       strings.TrimSpace(values.Phone),
		Website:     strings.TrimSpace(values.Website),
		Notes:       values.VendorNotes,
	}
	return quote, vendor, nil
}

func (m *Model) submitMaintenanceForm() error {
	item, err := m.parseMaintenanceFormData()
	if err != nil {
		return err
	}
	if m.editID != nil {
		item.ID = *m.editID
		return m.store.UpdateMaintenance(item)
	}
	if err := m.store.CreateMaintenance(&item); err != nil {
		return err
	}
	id := item.ID
	m.editID = &id
	return nil
}

func (m *Model) parseMaintenanceFormData() (data.MaintenanceItem, error) {
	values, ok := m.formData.(*maintenanceFormData)
	if !ok {
		return data.MaintenanceItem{}, fmt.Errorf("unexpected maintenance form data")
	}
	lastServiced, err := data.ParseOptionalDate(values.LastServiced)
	if err != nil {
		return data.MaintenanceItem{}, err
	}
	interval, err := data.ParseIntervalMonths(values.IntervalMonths)
	if err != nil {
		return data.MaintenanceItem{}, err
	}
	cost, err := data.ParseOptionalCents(values.Cost)
	if err != nil {
		return data.MaintenanceItem{}, err
	}
	var appID *uint
	if values.ApplianceID > 0 {
		appID = &values.ApplianceID
	}
	return data.MaintenanceItem{
		Name:           strings.TrimSpace(values.Name),
		CategoryID:     values.CategoryID,
		ApplianceID:    appID,
		LastServicedAt: lastServiced,
		IntervalMonths: interval,
		ManualURL:      strings.TrimSpace(values.ManualURL),
		ManualText:     strings.TrimSpace(values.ManualText),
		CostCents:      cost,
		Notes:          strings.TrimSpace(values.Notes),
	}, nil
}

func projectTypeOptions(types []data.ProjectType) []huh.Option[uint] {
	options := make([]huh.Option[uint], 0, len(types))
	for _, projectType := range types {
		options = append(options, huh.NewOption(projectType.Name, projectType.ID))
	}
	return withOrdinals(options)
}

func maintenanceOptions(
	categories []data.MaintenanceCategory,
) []huh.Option[uint] {
	options := make([]huh.Option[uint], 0, len(categories))
	for _, category := range categories {
		options = append(options, huh.NewOption(category.Name, category.ID))
	}
	return withOrdinals(options)
}

func projectOptions(projects []data.Project) []huh.Option[uint] {
	options := make([]huh.Option[uint], 0, len(projects))
	for _, project := range projects {
		label := project.Title
		if label == "" {
			label = fmt.Sprintf("Project %d", project.ID)
		}
		options = append(options, huh.NewOption(label, project.ID))
	}
	return withOrdinals(options)
}

func statusOptions() []huh.Option[string] {
	type entry struct {
		value string
		color lipgloss.AdaptiveColor
	}
	statuses := []entry{
		{data.ProjectStatusIdeating, muted},
		{data.ProjectStatusPlanned, accent},
		{data.ProjectStatusQuoted, secondary},
		{data.ProjectStatusInProgress, success},
		{data.ProjectStatusDelayed, warning},
		{data.ProjectStatusCompleted, textDim},
		{data.ProjectStatusAbandoned, danger},
	}
	opts := make([]huh.Option[string], len(statuses))
	for i, s := range statuses {
		label := statusLabel(s.value)
		colored := lipgloss.NewStyle().Foreground(s.color).Render(label)
		opts[i] = huh.NewOption(colored, s.value)
	}
	return withOrdinals(opts)
}

// withOrdinals prefixes each option label with its 1-based position so users
// can see which number key jumps to which option.
func withOrdinals[T comparable](opts []huh.Option[T]) []huh.Option[T] {
	for i := range opts {
		opts[i].Key = fmt.Sprintf("%d. %s", i+1, opts[i].Key)
	}
	return opts
}

func requiredText(label string) func(string) error {
	return func(input string) error {
		if strings.TrimSpace(input) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
}

func optionalInt(label string) func(string) error {
	return func(input string) error {
		if _, err := data.ParseOptionalInt(input); err != nil {
			return fmt.Errorf("%s should be a whole number", label)
		}
		return nil
	}
}

func optionalInterval(label string) func(string) error {
	return func(input string) error {
		if _, err := data.ParseIntervalMonths(input); err != nil {
			return fmt.Errorf(
				"%s should be months (6), or a duration like 6m, 1y, 2y 6m",
				label,
			)
		}
		return nil
	}
}

func optionalFloat(label string) func(string) error {
	return func(input string) error {
		if _, err := data.ParseOptionalFloat(input); err != nil {
			return fmt.Errorf("%s should be a number like 2.5", label)
		}
		return nil
	}
}

// endDateAfterStart validates that end date is a valid optional date and,
// when both dates are provided, that end date is not before start date.
func endDateAfterStart(startDate, endDate *string) func(string) error {
	return func(_ string) error {
		end := strings.TrimSpace(*endDate)
		if err := optionalDate("end date")(end); err != nil {
			return err
		}
		start := strings.TrimSpace(*startDate)
		if end == "" || start == "" {
			return nil
		}
		s, err := time.Parse(data.DateLayout, start)
		if err != nil {
			// start date will be caught by its own validator
			return nil //nolint:nilerr // start date validated by its own field
		}
		e, err := time.Parse(data.DateLayout, end)
		if err != nil {
			return nil //nolint:nilerr // end date format already checked by optionalDate above
		}
		if e.Before(s) {
			return fmt.Errorf("end date must not be before start date")
		}
		return nil
	}
}

func optionalDate(label string) func(string) error {
	return func(input string) error {
		if _, err := data.ParseOptionalDate(input); err != nil {
			return fmt.Errorf("%s should be YYYY-MM-DD", label)
		}
		return nil
	}
}

func moneyError(label string, err error) error {
	if errors.Is(err, data.ErrNegativeMoney) {
		return fmt.Errorf("%s must be a positive amount", label)
	}
	return fmt.Errorf("%s should look like 1250.00", label)
}

func optionalMoney(label string) func(string) error {
	return func(input string) error {
		if _, err := data.ParseOptionalCents(input); err != nil {
			return moneyError(label, err)
		}
		return nil
	}
}

func requiredMoney(label string) func(string) error {
	return func(input string) error {
		if _, err := data.ParseRequiredCents(input); err != nil {
			return moneyError(label, err)
		}
		return nil
	}
}

func projectFormValues(project data.Project) *projectFormData {
	return &projectFormData{
		Title:         project.Title,
		ProjectTypeID: project.ProjectTypeID,
		Status:        project.Status,
		Budget:        data.FormatOptionalCents(project.BudgetCents),
		Actual:        data.FormatOptionalCents(project.ActualCents),
		StartDate:     data.FormatDate(project.StartDate),
		EndDate:       data.FormatDate(project.EndDate),
		Description:   project.Description,
	}
}

func quoteFormValues(quote data.Quote) *quoteFormData {
	return &quoteFormData{
		ProjectID:    quote.ProjectID,
		VendorName:   quote.Vendor.Name,
		ContactName:  quote.Vendor.ContactName,
		Email:        quote.Vendor.Email,
		Phone:        quote.Vendor.Phone,
		Website:      quote.Vendor.Website,
		VendorNotes:  quote.Vendor.Notes,
		Total:        data.FormatCents(quote.TotalCents),
		Labor:        data.FormatOptionalCents(quote.LaborCents),
		Materials:    data.FormatOptionalCents(quote.MaterialsCents),
		Other:        data.FormatOptionalCents(quote.OtherCents),
		ReceivedDate: data.FormatDate(quote.ReceivedDate),
		Notes:        quote.Notes,
	}
}

func maintenanceFormValues(item data.MaintenanceItem) *maintenanceFormData {
	var appID uint
	if item.ApplianceID != nil {
		appID = *item.ApplianceID
	}
	return &maintenanceFormData{
		Name:           item.Name,
		CategoryID:     item.CategoryID,
		ApplianceID:    appID,
		LastServiced:   data.FormatDate(item.LastServicedAt),
		IntervalMonths: formatInterval(item.IntervalMonths),
		ManualURL:      item.ManualURL,
		ManualText:     item.ManualText,
		Cost:           data.FormatOptionalCents(item.CostCents),
		Notes:          item.Notes,
	}
}

func applianceFormValues(item data.Appliance) *applianceFormData {
	return &applianceFormData{
		Name:           item.Name,
		Brand:          item.Brand,
		ModelNumber:    item.ModelNumber,
		SerialNumber:   item.SerialNumber,
		PurchaseDate:   data.FormatDate(item.PurchaseDate),
		WarrantyExpiry: data.FormatDate(item.WarrantyExpiry),
		Location:       item.Location,
		Cost:           data.FormatOptionalCents(item.CostCents),
		Notes:          item.Notes,
	}
}

func houseFormValues(profile data.HouseProfile) *houseFormData {
	return &houseFormData{
		Nickname:         profile.Nickname,
		AddressLine1:     profile.AddressLine1,
		AddressLine2:     profile.AddressLine2,
		City:             profile.City,
		State:            profile.State,
		PostalCode:       profile.PostalCode,
		YearBuilt:        intToString(profile.YearBuilt),
		SquareFeet:       intToString(profile.SquareFeet),
		LotSquareFeet:    intToString(profile.LotSquareFeet),
		Bedrooms:         intToString(profile.Bedrooms),
		Bathrooms:        formatFloat(profile.Bathrooms),
		FoundationType:   profile.FoundationType,
		WiringType:       profile.WiringType,
		RoofType:         profile.RoofType,
		ExteriorType:     profile.ExteriorType,
		HeatingType:      profile.HeatingType,
		CoolingType:      profile.CoolingType,
		WaterSource:      profile.WaterSource,
		SewerType:        profile.SewerType,
		ParkingType:      profile.ParkingType,
		BasementType:     profile.BasementType,
		InsuranceCarrier: profile.InsuranceCarrier,
		InsurancePolicy:  profile.InsurancePolicy,
		InsuranceRenewal: data.FormatDate(profile.InsuranceRenewal),
		PropertyTax:      data.FormatOptionalCents(profile.PropertyTaxCents),
		HOAName:          profile.HOAName,
		HOAFee:           data.FormatOptionalCents(profile.HOAFeeCents),
	}
}

// requiredTitle appends a colored ∗ (U+2217) to a form field label.
func requiredTitle(label string) string {
	marker := lipgloss.NewStyle().Foreground(secondary).Render(" ∗")
	return label + marker
}

// requiredLegend returns the "∗ required" legend line for forms that have
// required fields. Returns empty string when the form has none.
func (m *Model) requiredLegend() string {
	if !m.formHasRequired {
		return ""
	}
	marker := lipgloss.NewStyle().Foreground(secondary).Render("∗")
	label := lipgloss.NewStyle().Foreground(textDim).Render(" required")
	return marker + label
}

func intToString(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

// ---------------------------------------------------------------------------
// Document forms
// ---------------------------------------------------------------------------

// startDocumentForm opens a new-document form. entityKind is set by scoped
// handlers (e.g. "project") or empty for the top-level Documents tab.
func (m *Model) startDocumentForm(entityKind string) error {
	values := &documentFormData{}
	scoped := entityKind != ""

	fields := []huh.Field{
		huh.NewInput().
			Title(requiredTitle("Title")).
			Value(&values.Title).
			Validate(requiredText("title")),
	}

	if !scoped {
		entityOpts, err := m.documentEntityOptions()
		if err != nil {
			return err
		}
		fields = append(fields,
			huh.NewSelect[entityRef]().
				Title("Entity").
				Height(10).
				Options(entityOpts...).
				Value(&values.EntityRef),
		)
	}

	fields = append(fields,
		huh.NewInput().
			Title("File path").
			Description("Local path to the file to attach").
			Value(&values.FilePath).
			Validate(optionalFilePath()),
		huh.NewText().Title("Notes").Value(&values.Notes),
	)

	form := huh.NewForm(huh.NewGroup(fields...))
	m.activateForm(formDocument, form, values)
	return nil
}

func (m *Model) startEditDocumentForm(id uint) error {
	doc, err := m.store.GetDocument(id)
	if err != nil {
		return fmt.Errorf("load document: %w", err)
	}
	values := documentFormValues(doc)
	m.editID = &id

	scoped := len(m.detailStack) > 0
	return m.openEditDocumentForm(values, scoped)
}

func (m *Model) openEditDocumentForm(values *documentFormData, scoped bool) error {
	fields := []huh.Field{
		huh.NewInput().
			Title(requiredTitle("Title")).
			Value(&values.Title).
			Validate(requiredText("title")),
	}

	if !scoped {
		entityOpts, err := m.documentEntityOptions()
		if err != nil {
			return err
		}
		fields = append(fields,
			huh.NewSelect[entityRef]().
				Title("Entity").
				Height(10).
				Options(entityOpts...).
				Value(&values.EntityRef),
		)
	}

	fields = append(fields,
		huh.NewInput().
			Title("File path").
			Description("Local path to a replacement file").
			Value(&values.FilePath).
			Validate(optionalFilePath()),
		huh.NewText().Title("Notes").Value(&values.Notes),
	)

	form := huh.NewForm(huh.NewGroup(fields...))
	m.activateForm(formDocument, form, values)
	return nil
}

func (m *Model) submitDocumentForm() error {
	result, err := m.parseDocumentFormData()
	if err != nil {
		return err
	}
	doc := result.Doc
	if m.editID != nil {
		doc.ID = *m.editID
		if err := m.store.UpdateDocument(doc); err != nil {
			return err
		}
	} else {
		if err := m.store.CreateDocument(&doc); err != nil {
			return err
		}
		id := doc.ID
		m.editID = &id
	}
	if result.ExtractErr != nil {
		m.setStatusInfo(fmt.Sprintf("extraction incomplete: %s", result.ExtractErr))
	}
	return nil
}

// submitScopedDocumentForm creates a document with the given entity scope.
func (m *Model) submitScopedDocumentForm(entityKind string, entityID uint) error {
	result, err := m.parseDocumentFormData()
	if err != nil {
		return err
	}
	doc := result.Doc
	doc.EntityKind = entityKind
	doc.EntityID = entityID
	if m.editID != nil {
		doc.ID = *m.editID
		if err := m.store.UpdateDocument(doc); err != nil {
			return err
		}
	} else {
		if err := m.store.CreateDocument(&doc); err != nil {
			return err
		}
		id := doc.ID
		m.editID = &id
	}
	if result.ExtractErr != nil {
		m.setStatusInfo(fmt.Sprintf("extraction incomplete: %s", result.ExtractErr))
	}
	return nil
}

func (m *Model) parseDocumentFormData() (documentParseResult, error) {
	values, ok := m.formData.(*documentFormData)
	if !ok {
		return documentParseResult{}, fmt.Errorf("unexpected document form data")
	}
	doc := data.Document{
		Title:      strings.TrimSpace(values.Title),
		EntityKind: values.EntityRef.Kind,
		EntityID:   values.EntityRef.ID,
		Notes:      strings.TrimSpace(values.Notes),
	}
	// Read file from path if provided (new document or file replacement).
	path := filepath.Clean(data.ExpandHome(strings.TrimSpace(values.FilePath)))
	if path != "" && path != "." {
		info, err := os.Stat(path)
		if err != nil {
			return documentParseResult{}, fmt.Errorf("stat file: %w", err)
		}
		maxSize := m.store.MaxDocumentSize()
		fileSize := info.Size()
		if fileSize < 0 {
			return documentParseResult{}, fmt.Errorf("file has invalid size %d", fileSize)
		}
		if uint64(fileSize) > maxSize { //nolint:gosec // negative ruled out above
			return documentParseResult{}, fmt.Errorf(
				"file is too large (%s) -- maximum allowed is %s",
				formatFileSize(
					uint64(fileSize),
				),
				formatFileSize(maxSize), //nolint:gosec // negative ruled out above
			)
		}
		fileData, err := os.ReadFile(path)
		if err != nil {
			return documentParseResult{}, fmt.Errorf("read file: %w", err)
		}
		doc.FileName = filepath.Base(path)
		doc.Data = fileData
		doc.SizeBytes = int64(len(fileData))
		doc.MIMEType = detectMIMEType(path, fileData)
		doc.ChecksumSHA256 = fmt.Sprintf("%x", sha256.Sum256(fileData))

		// Run the extraction pipeline.
		pipeline := m.buildExtractionPipeline()
		result := pipeline.Run(context.Background(), fileData, doc.FileName, doc.MIMEType)
		doc.ExtractedText = result.ExtractedText
		doc.OCRData = result.OCRData

		// Show one-time tesseract hint if OCR was needed but unavailable.
		if extract.IsScanned(doc.ExtractedText) && !result.OCRUsed {
			if extract.IsImageMIME(doc.MIMEType) || doc.MIMEType == "application/pdf" {
				m.showTesseractHint()
			}
		}

		// Auto-fill title: prefer LLM suggestion, fall back to filename.
		if doc.Title == "" {
			if result.Hints != nil && result.Hints.TitleSugg != "" {
				doc.Title = result.Hints.TitleSugg
			} else {
				doc.Title = data.TitleFromFilename(doc.FileName)
			}
		}

		// Pre-fill notes from LLM summary if user left notes blank.
		if doc.Notes == "" && result.Hints != nil && result.Hints.Summary != "" {
			doc.Notes = result.Hints.Summary
		}

		return documentParseResult{
			Doc:        doc,
			Hints:      result.Hints,
			ExtractErr: result.Err,
		}, nil
	}
	return documentParseResult{Doc: doc}, nil
}

// buildExtractionPipeline creates an extraction pipeline configured from
// the current app state.
func (m *Model) buildExtractionPipeline() *extract.Pipeline {
	p := &extract.Pipeline{
		MaxOCRPages: m.maxOCRPages,
	}

	// Only wire LLM if extraction is enabled and a client exists.
	if m.extractionEnabled && m.llmClient != nil {
		// Use the extraction-specific model if configured, otherwise the
		// chat client is used as-is.
		if m.extractionModel != "" && m.extractionModel != m.llmClient.Model() {
			p.LLMClient = llm.NewClient(
				m.llmClient.BaseURL(),
				m.extractionModel,
				m.llmClient.Timeout(),
			)
		} else {
			p.LLMClient = m.llmClient
		}
	}

	// Load entity context for LLM matching (best-effort).
	vendors, projects, appliances, err := m.store.EntityNames()
	if err == nil {
		p.EntityContext = extract.EntityContext{
			Vendors:    vendors,
			Projects:   projects,
			Appliances: appliances,
		}
	}

	return p
}

// showTesseractHint displays a one-time status bar hint suggesting the
// user install tesseract for better document extraction. The hint is
// persisted in the DB so it's never shown again.
func (m *Model) showTesseractHint() {
	if m.store.TesseractHintSeen() {
		return
	}
	m.setStatusInfo("install tesseract for text extraction from scanned docs")
	_ = m.store.MarkTesseractHintSeen()
}

func (m *Model) inlineEditDocument(id uint, col documentCol) error {
	doc, err := m.store.GetDocument(id)
	if err != nil {
		return fmt.Errorf("load document: %w", err)
	}
	values := documentFormValues(doc)
	switch col {
	case documentColTitle:
		m.openInlineInput(
			id,
			formDocument,
			"Title",
			"",
			&values.Title,
			requiredText("title"),
			values,
		)
	case documentColNotes:
		m.openNotesEdit(id, formDocument, &values.Notes, values)
	case documentColEntity:
		entityOpts, err := m.documentEntityOptions()
		if err != nil {
			return err
		}
		field := huh.NewSelect[entityRef]().
			Title("Entity").
			Height(10).
			Options(entityOpts...).
			Value(&values.EntityRef)
		m.openInlineEdit(id, formDocument, field, values)
	case documentColID, documentColType, documentColSize, documentColUpdated:
		return m.startEditDocumentForm(id)
	}
	return nil
}

func documentFormValues(doc data.Document) *documentFormData {
	return &documentFormData{
		Title:     doc.Title,
		EntityRef: entityRef{Kind: doc.EntityKind, ID: doc.EntityID},
		Notes:     doc.Notes,
	}
}

// detectMIMEType uses http.DetectContentType with a file extension fallback.
func detectMIMEType(path string, fileData []byte) string {
	mime := http.DetectContentType(fileData)
	// DetectContentType returns application/octet-stream for unknown types;
	// try extension-based detection as a fallback.
	if mime == "application/octet-stream" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".pdf":
			return "application/pdf"
		case ".txt":
			return "text/plain"
		case ".csv":
			return "text/csv"
		case ".json":
			return "application/json"
		case ".md":
			return "text/markdown"
		}
	}
	return mime
}

func optionalFilePath() func(string) error {
	return func(input string) error {
		path := strings.TrimSpace(input)
		if path == "" {
			return nil
		}
		path = data.ExpandHome(path)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("file not found: %s", path)
		}
		if info.IsDir() {
			return fmt.Errorf("path is a directory, not a file")
		}
		return nil
	}
}
