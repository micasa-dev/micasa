// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/extract"
	"github.com/cpcloud/micasa/internal/locale"
)

func (*houseFormData) formKind() FormKind       { return formHouse }
func (*projectFormData) formKind() FormKind     { return formProject }
func (*quoteFormData) formKind() FormKind       { return formQuote }
func (*maintenanceFormData) formKind() FormKind { return formMaintenance }
func (*serviceLogFormData) formKind() FormKind  { return formServiceLog }
func (*vendorFormData) formKind() FormKind      { return formVendor }
func (*documentFormData) formKind() FormKind    { return formDocument }
func (*incidentFormData) formKind() FormKind    { return formIncident }
func (*applianceFormData) formKind() FormKind   { return formAppliance }

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
	Status        string `default:"planned"`
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

type scheduleType int

const (
	schedNone scheduleType = iota
	schedInterval
	schedDueDate
)

type maintenanceFormData struct {
	Name           string
	CategoryID     uint
	ApplianceID    uint // 0 means none
	Season         string
	ScheduleType   scheduleType
	LastServiced   string
	IntervalMonths string
	DueDate        string
	ManualURL      string
	ManualText     string
	Cost           string
	Notes          string
}

type serviceLogFormData struct {
	MaintenanceItemID uint
	ServicedAt        string `default:"today"`
	VendorID          uint   // 0 = self
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
	Title       string
	FilePath    string // local file path; read on submit for new documents
	EntityRef   entityRef
	Notes       string
	DeferCreate bool // true for magic-add: hold document in memory until accept
}

// documentParseResult holds the parsed document and any non-fatal extraction
// error. LLM hints are extracted asynchronously after save (see
// extractDocumentHintsCmd).
type documentParseResult struct {
	Doc        data.Document
	ExtractErr error
}

type incidentFormData struct {
	Title        string
	Description  string
	Status       string `default:"open"`
	Severity     string `default:"soon"`
	DateNoticed  string `default:"today"`
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
		values = m.houseFormValues(m.house)
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
				Title(data.AreaFormTitle(m.unitSystem)).
				Placeholder(data.AreaPlaceholder(m.unitSystem)).
				Value(&values.SquareFeet).
				Validate(optionalInt(data.AreaFormTitle(m.unitSystem))),
			huh.NewInput().
				Title(data.LotAreaFormTitle(m.unitSystem)).
				Placeholder(data.LotAreaPlaceholder(m.unitSystem)).
				Value(&values.LotSquareFeet).
				Validate(optionalInt(data.LotAreaFormTitle(m.unitSystem))),
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
				Validate(optionalMoney("property tax", m.cur)),
			huh.NewInput().Title("HOA name").Value(&values.HOAName),
			huh.NewInput().
				Title("HOA fee (monthly)").
				Placeholder("250.00").
				Value(&values.HOAFee).
				Validate(optionalMoney("HOA fee", m.cur)),
		).Title("Financial"),
	)
	formWidth := 60
	if m.width > 0 && m.width < formWidth+10 {
		formWidth = m.width - 10
	}
	form.WithWidth(formWidth)
	m.activateForm(form, values)
}

func (m *Model) startProjectForm() {
	values := &projectFormData{}
	data.ApplyDefaults(values)
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
	m.activateForm(form, values)
}

func (m *Model) startEditProjectForm(id uint) error {
	project, err := m.store.GetProject(id)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	values := projectFormValues(project, m.cur)
	options := projectTypeOptions(m.projectTypes)
	m.fs.editID = &id
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
				Validate(optionalMoney("budget", m.cur)),
			huh.NewInput().
				Title("Actual cost").
				Placeholder("1400.00").
				Value(&values.Actual).
				Validate(optionalMoney("actual cost", m.cur)),
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
	m.activateForm(form, values)
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
				Validate(requiredMoney("total", m.cur)),
		),
	)
	m.activateForm(form, values)
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
	values := quoteFormValues(quote, m.cur)
	options := projectOptions(projects)
	m.fs.editID = &id
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
				Validate(requiredMoney("total", m.cur)),
			huh.NewInput().
				Title("Labor").
				Placeholder("2000.00").
				Value(&values.Labor).
				Validate(optionalMoney("labor", m.cur)),
			huh.NewInput().
				Title("Materials").
				Placeholder("1000.00").
				Value(&values.Materials).
				Validate(optionalMoney("materials", m.cur)),
			huh.NewInput().
				Title("Other").
				Placeholder("250.00").
				Value(&values.Other).
				Validate(optionalMoney("other costs", m.cur)),
			huh.NewInput().
				Title("Received date (YYYY-MM-DD)").
				Value(&values.ReceivedDate).
				Validate(optionalDate("received date")),
			huh.NewText().Title("Notes").Value(&values.Notes),
		).Title("Quote"),
	)
	m.activateForm(form, values)
}

func scheduleTypeOptions() []huh.Option[scheduleType] {
	return []huh.Option[scheduleType]{
		huh.NewOption("None", schedNone),
		huh.NewOption("Recurring interval", schedInterval),
		huh.NewOption("Fixed due date", schedDueDate),
	}
}

func (m *Model) startMaintenanceForm() error {
	values := &maintenanceFormData{ScheduleType: schedNone}
	catOptions := maintenanceOptions(m.maintenanceCategories)
	if len(catOptions) > 0 {
		values.CategoryID = catOptions[0].Value
	}
	appliances, err := m.store.ListAppliances(false)
	if err != nil {
		return fmt.Errorf("list appliances: %w", err)
	}
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
			huh.NewSelect[string]().
				Title("Season").
				Options(seasonOptions()...).
				Value(&values.Season),
			huh.NewSelect[uint]().
				Title("Appliance").
				Options(appOpts...).
				Value(&values.ApplianceID),
			huh.NewSelect[scheduleType]().
				Title("Schedule").
				Options(scheduleTypeOptions()...).
				Value(&values.ScheduleType),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Interval").
				Placeholder("6m").
				Value(&values.IntervalMonths).
				Validate(optionalInterval("interval")),
		).WithHideFunc(func() bool { return values.ScheduleType != schedInterval }),
		huh.NewGroup(
			huh.NewInput().
				Title("Due date (YYYY-MM-DD)").
				Value(&values.DueDate).
				Validate(optionalDate("due date")),
		).WithHideFunc(func() bool { return values.ScheduleType != schedDueDate }),
	)
	m.activateForm(form, values)
	return nil
}

func (m *Model) startEditMaintenanceForm(id uint) error {
	item, err := m.store.GetMaintenance(id)
	if err != nil {
		return fmt.Errorf("load maintenance item: %w", err)
	}
	values := maintenanceFormValues(item, m.cur)
	options := maintenanceOptions(m.maintenanceCategories)
	appliances, err := m.store.ListAppliances(false)
	if err != nil {
		return fmt.Errorf("list appliances: %w", err)
	}
	appOpts := applianceOptions(appliances)
	m.fs.editID = &id
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
			huh.NewSelect[string]().
				Title("Season").
				Options(seasonOptions()...).
				Value(&values.Season),
			huh.NewSelect[uint]().
				Title("Appliance").
				Options(appOptions...).
				Value(&values.ApplianceID),
			huh.NewInput().
				Title("Last serviced (YYYY-MM-DD)").
				Value(&values.LastServiced).
				Validate(optionalDate("last serviced")),
			huh.NewSelect[scheduleType]().
				Title("Schedule").
				Options(scheduleTypeOptions()...).
				Value(&values.ScheduleType),
		).Title("Schedule"),
		huh.NewGroup(
			huh.NewInput().
				Title("Interval").
				Placeholder("6m").
				Value(&values.IntervalMonths).
				Validate(optionalInterval("interval")),
		).WithHideFunc(func() bool { return values.ScheduleType != schedInterval }),
		huh.NewGroup(
			huh.NewInput().
				Title("Due date (YYYY-MM-DD)").
				Value(&values.DueDate).
				Validate(optionalDate("due date")),
		).WithHideFunc(func() bool { return values.ScheduleType != schedDueDate }),
		huh.NewGroup(
			huh.NewInput().Title("Manual URL").Value(&values.ManualURL),
			huh.NewText().Title("Manual notes").Value(&values.ManualText),
			huh.NewInput().
				Title("Cost").
				Placeholder("125.00").
				Value(&values.Cost).
				Validate(optionalMoney("cost", m.cur)),
			huh.NewText().Title("Notes").Value(&values.Notes),
		).Title("Details"),
	)
	m.activateForm(form, values)
}

func (m *Model) startIncidentForm() error {
	values := &incidentFormData{}
	data.ApplyDefaults(values)
	appliances, err := m.store.ListAppliances(false)
	if err != nil {
		return fmt.Errorf("list appliances: %w", err)
	}
	appOpts := applianceOptions(appliances)
	vendorOpts := vendorOpts("(none)", m.vendors)
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
	m.activateForm(form, values)
	return nil
}

func (m *Model) startEditIncidentForm(id uint) error {
	item, err := m.store.GetIncident(id)
	if err != nil {
		return fmt.Errorf("load incident: %w", err)
	}
	values := incidentFormValues(item, m.cur)
	appliances, err := m.store.ListAppliances(false)
	if err != nil {
		return fmt.Errorf("list appliances: %w", err)
	}
	appOpts := applianceOptions(appliances)
	vendorOpts := vendorOpts("(none)", m.vendors)
	m.fs.editID = &id
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
				Validate(optionalMoney("cost", m.cur)),
			huh.NewText().Title("Description").Value(&values.Description),
			huh.NewText().Title("Notes").Value(&values.Notes),
		).Title("Context"),
	)
	m.activateForm(form, values)
}

func (m *Model) submitIncidentForm() error {
	item, err := m.parseIncidentFormData()
	if err != nil {
		return err
	}
	if err := m.createOrUpdate(&item.ID,
		func() error { return m.store.CreateIncident(&item) },
		func() error { return m.store.UpdateIncident(item) },
	); err != nil {
		return err
	}
	// Setting status to resolved via the picker should also soft-delete.
	if m.fs.editID != nil && item.Status == data.IncidentStatusResolved {
		return m.store.DeleteIncident(item.ID)
	}
	return nil
}

func (m *Model) parseIncidentFormData() (data.Incident, error) {
	values, err := formDataAs[incidentFormData](m)
	if err != nil {
		return data.Incident{}, err
	}
	noticed, err := data.ParseRequiredDate(values.DateNoticed)
	if err != nil {
		return data.Incident{}, data.FieldError("Date Noticed", err)
	}
	resolved, err := data.ParseOptionalDate(values.DateResolved)
	if err != nil {
		return data.Incident{}, data.FieldError("Date Resolved", err)
	}
	cost, err := m.cur.ParseOptionalCents(values.Cost)
	if err != nil {
		return data.Incident{}, data.FieldError("Cost", err)
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
	values := incidentFormValues(item, m.cur)
	switch col {
	case incidentColTitle:
		m.openInlineInput(
			id,
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
		m.openInlineEdit(id, field, values)
	case incidentColSeverity:
		field := huh.NewSelect[string]().Title("Severity").
			Options(incidentSeverityOptions()...).
			Value(&values.Severity)
		m.openInlineEdit(id, field, values)
	case incidentColLocation:
		m.openInlineInput(id, "Location", "Kitchen", &values.Location, nil, values)
	case incidentColAppliance:
		appliances, loadErr := m.store.ListAppliances(false)
		if loadErr != nil {
			return loadErr
		}
		appOpts := applianceOptions(appliances)
		field := huh.NewSelect[uint]().Title("Appliance").
			Options(appOpts...).
			Value(&values.ApplianceID)
		m.openInlineEdit(id, field, values)
	case incidentColVendor:
		vendorOpts := vendorOpts("(none)", m.vendors)
		field := huh.NewSelect[uint]().Title("Vendor").
			Options(vendorOpts...).
			Value(&values.VendorID)
		m.openInlineEdit(id, field, values)
	case incidentColNoticed:
		m.openDatePicker(id, &values.DateNoticed, values)
	case incidentColResolved:
		m.openDatePicker(id, &values.DateResolved, values)
	case incidentColCost:
		m.openInlineInput(
			id,
			"Cost",
			"250.00",
			&values.Cost,
			optionalMoney("cost", m.cur),
			values,
		)
	case incidentColID, incidentColDocs:
		return m.startEditIncidentForm(id)
	}
	return nil
}

func incidentFormValues(item data.Incident, cur locale.Currency) *incidentFormData {
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
		Cost:         cur.FormatOptionalCents(item.CostCents),
		ApplianceID:  appID,
		VendorID:     vendorID,
		Notes:        item.Notes,
	}
}

func incidentStatusOptions() []huh.Option[string] {
	type entry struct {
		value string
		color adaptiveColor
	}
	statuses := []entry{
		{data.IncidentStatusOpen, accentPair},
		{data.IncidentStatusInProgress, successPair},
		{data.IncidentStatusResolved, textDimPair},
	}
	opts := make([]huh.Option[string], len(statuses))
	for i, s := range statuses {
		label := statusLabel(s.value)
		colored := lipgloss.NewStyle().Foreground(s.color.resolve(appIsDark)).Render(label)
		opts[i] = huh.NewOption(colored, s.value)
	}
	return withOrdinals(opts)
}

func incidentSeverityOptions() []huh.Option[string] {
	type entry struct {
		value string
		color adaptiveColor
	}
	severities := []entry{
		{data.IncidentSeverityUrgent, dangerPair},
		{data.IncidentSeveritySoon, warningPair},
		{data.IncidentSeverityWhenever, textDimPair},
	}
	opts := make([]huh.Option[string], len(severities))
	for i, s := range severities {
		label := statusLabel(s.value)
		colored := lipgloss.NewStyle().Foreground(s.color.resolve(appIsDark)).Render(label)
		opts[i] = huh.NewOption(colored, s.value)
	}
	return withOrdinals(opts)
}

func seasonOptions() []huh.Option[string] {
	type entry struct {
		value string
		color adaptiveColor
	}
	seasons := []entry{
		{"", textDimPair},
		{data.SeasonSpring, successPair},
		{data.SeasonSummer, warningPair},
		{data.SeasonFall, secondaryPair},
		{data.SeasonWinter, accentPair},
	}
	opts := make([]huh.Option[string], len(seasons))
	for i, s := range seasons {
		label := "(none)"
		if s.value != "" {
			label = statusLabel(s.value)
		}
		colored := lipgloss.NewStyle().Foreground(s.color.resolve(appIsDark)).Render(label)
		opts[i] = huh.NewOption(colored, s.value)
	}
	return withOrdinals(opts)
}

// optionalVendorOptions is like vendorOptions but with "(none)" instead of "Self".
// labelWithDetail returns "name (detail)" when detail is non-empty,
// otherwise just "name".
func labelWithDetail(name, detail string) string {
	if detail != "" {
		return fmt.Sprintf("%s (%s)", name, detail)
	}
	return name
}

// vendorOpts builds a vendor option list with noneLabel as the leading
// zero-value entry.
func vendorOpts(noneLabel string, vendors []data.Vendor) []huh.Option[uint] {
	options := make([]huh.Option[uint], 0, len(vendors)+1)
	options = append(options, huh.NewOption(noneLabel, uint(0)))
	for _, v := range vendors {
		options = append(options, huh.NewOption(labelWithDetail(v.Name, v.ContactName), v.ID))
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
	m.activateForm(form, values)
}

func (m *Model) startEditApplianceForm(id uint) error {
	item, err := m.store.GetAppliance(id)
	if err != nil {
		return fmt.Errorf("load appliance: %w", err)
	}
	values := applianceFormValues(item, m.cur)
	m.fs.editID = &id
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
				Validate(optionalMoney("cost", m.cur)),
			huh.NewText().Title("Notes").Value(&values.Notes),
		).Title("Details"),
	)
	m.activateForm(form, values)
}

func (m *Model) submitApplianceForm() error {
	item, err := m.parseApplianceFormData()
	if err != nil {
		return err
	}
	return m.createOrUpdate(&item.ID,
		func() error { return m.store.CreateAppliance(&item) },
		func() error { return m.store.UpdateAppliance(item) },
	)
}

func (m *Model) parseApplianceFormData() (data.Appliance, error) {
	values, err := formDataAs[applianceFormData](m)
	if err != nil {
		return data.Appliance{}, err
	}
	purchaseDate, err := data.ParseOptionalDate(values.PurchaseDate)
	if err != nil {
		return data.Appliance{}, data.FieldError("Purchase Date", err)
	}
	warrantyExpiry, err := data.ParseOptionalDate(values.WarrantyExpiry)
	if err != nil {
		return data.Appliance{}, data.FieldError("Warranty Expiry", err)
	}
	cost, err := m.cur.ParseOptionalCents(values.Cost)
	if err != nil {
		return data.Appliance{}, data.FieldError("Cost", err)
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
	m.activateForm(form, values)
}

func (m *Model) startEditVendorForm(id uint) error {
	vendor, err := m.store.GetVendor(id)
	if err != nil {
		return fmt.Errorf("load vendor: %w", err)
	}
	values := vendorFormValues(vendor)
	m.fs.editID = &id
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
	m.activateForm(form, values)
}

func (m *Model) submitVendorForm() error {
	vendor, err := m.parseVendorFormData()
	if err != nil {
		return err
	}
	return m.createOrUpdate(&vendor.ID,
		func() error { return m.store.CreateVendor(&vendor) },
		func() error { return m.store.UpdateVendor(vendor) },
	)
}

func (m *Model) parseVendorFormData() (data.Vendor, error) {
	values, err := formDataAs[vendorFormData](m)
	if err != nil {
		return data.Vendor{}, err
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
		m.openInlineInput(id, "Name", "", &values.Name, requiredText("name"), values)
	case vendorColContact:
		m.openInlineInput(id, "Contact name", "", &values.ContactName, nil, values)
	case vendorColEmail:
		m.openInlineInput(id, "Email", "", &values.Email, nil, values)
	case vendorColPhone:
		m.openInlineInput(id, "Phone", "", &values.Phone, nil, values)
	case vendorColWebsite:
		m.openInlineInput(id, "Website", "", &values.Website, nil, values)
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
	values := projectFormValues(project, m.cur)
	switch col {
	case projectColType:
		options := projectTypeOptions(m.projectTypes)
		field := huh.NewSelect[uint]().Title("Project type").
			Options(options...).
			Value(&values.ProjectTypeID)
		m.openInlineEdit(id, field, values)
	case projectColTitle:
		m.openInlineInput(
			id,
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
		m.openInlineEdit(id, field, values)
	case projectColBudget:
		m.openInlineInput(
			id,
			"Budget",
			"1250.00",
			&values.Budget,
			optionalMoney("budget", m.cur),
			values,
		)
	case projectColActual:
		m.openInlineInput(
			id,
			"Actual cost",
			"1400.00",
			&values.Actual,
			optionalMoney("actual cost", m.cur),
			values,
		)
	case projectColStart:
		m.openDatePicker(id, &values.StartDate, values)
	case projectColEnd:
		m.openDatePicker(id, &values.EndDate, values)
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
	values := quoteFormValues(quote, m.cur)
	switch col {
	case quoteColProject:
		projectOpts := projectOptions(projects)
		field := huh.NewSelect[uint]().Title("Project").
			Options(projectOpts...).
			Value(&values.ProjectID)
		m.openInlineEdit(id, field, values)
	case quoteColVendor:
		m.openInlineInput(
			id,
			"Vendor name",
			"",
			&values.VendorName,
			requiredText("vendor name"),
			values,
		)
	case quoteColTotal:
		m.openInlineInput(
			id,
			"Total",
			"3250.00",
			&values.Total,
			requiredMoney("total", m.cur),
			values,
		)
	case quoteColLabor:
		m.openInlineInput(
			id,
			"Labor",
			"2000.00",
			&values.Labor,
			optionalMoney("labor", m.cur),
			values,
		)
	case quoteColMat:
		m.openInlineInput(
			id,
			"Materials",
			"1000.00",
			&values.Materials,
			optionalMoney("materials", m.cur),
			values,
		)
	case quoteColOther:
		m.openInlineInput(
			id,
			"Other",
			"250.00",
			&values.Other,
			optionalMoney("other costs", m.cur),
			values,
		)
	case quoteColRecv:
		m.openDatePicker(id, &values.ReceivedDate, values)
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
	values := maintenanceFormValues(item, m.cur)
	switch col {
	case maintenanceColItem:
		m.openInlineInput(
			id,
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
		m.openInlineEdit(id, field, values)
	case maintenanceColSeason:
		field := huh.NewSelect[string]().Title("Season").
			Options(seasonOptions()...).
			Value(&values.Season)
		m.openInlineEdit(id, field, values)
	case maintenanceColAppliance:
		appliances, loadErr := m.store.ListAppliances(false)
		if loadErr != nil {
			return loadErr
		}
		appOpts := applianceOptions(appliances)
		field := huh.NewSelect[uint]().Title("Appliance").
			Options(appOpts...).
			Value(&values.ApplianceID)
		m.openInlineEdit(id, field, values)
	case maintenanceColLast:
		m.openDatePicker(id, &values.LastServiced, values)
	case maintenanceColEvery:
		values.ScheduleType = schedInterval
		values.DueDate = ""
		m.openInlineInput(
			id,
			"Interval",
			"6m",
			&values.IntervalMonths,
			optionalInterval("interval"),
			values,
		)
	case maintenanceColNext:
		values.ScheduleType = schedDueDate
		values.IntervalMonths = ""
		m.openDatePicker(id, &values.DueDate, values)
	case maintenanceColID, maintenanceColLog, maintenanceColDocs:
		return m.startEditMaintenanceForm(id)
	}
	return nil
}

func (m *Model) inlineEditAppliance(id uint, col applianceCol) error {
	item, err := m.store.GetAppliance(id)
	if err != nil {
		return fmt.Errorf("load appliance: %w", err)
	}
	values := applianceFormValues(item, m.cur)
	switch col {
	case applianceColName:
		m.openInlineInput(id, "Name", "", &values.Name, requiredText("name"), values)
	case applianceColBrand:
		m.openInlineInput(id, "Brand", "", &values.Brand, nil, values)
	case applianceColModel:
		m.openInlineInput(id, "Model number", "", &values.ModelNumber, nil, values)
	case applianceColSerial:
		m.openInlineInput(id, "Serial number", "", &values.SerialNumber, nil, values)
	case applianceColLocation:
		m.openInlineInput(id, "Location", "Kitchen", &values.Location, nil, values)
	case applianceColPurchased:
		m.openDatePicker(id, &values.PurchaseDate, values)
	case applianceColWarranty:
		m.openDatePicker(id, &values.WarrantyExpiry, values)
	case applianceColCost:
		m.openInlineInput(
			id,
			"Cost",
			"899.00",
			&values.Cost,
			optionalMoney("cost", m.cur),
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
	}
	data.ApplyDefaults(values)
	vendorOpts := vendorOpts("Self (homeowner)", m.vendors)
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
	m.activateForm(form, values)
	return nil
}

func (m *Model) startEditServiceLogForm(id uint) error {
	entry, err := m.store.GetServiceLog(id)
	if err != nil {
		return fmt.Errorf("load service log: %w", err)
	}
	values := serviceLogFormValues(entry, m.cur)
	vendorOpts := vendorOpts("Self (homeowner)", m.vendors)
	m.fs.editID = &id
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
				Validate(optionalMoney("cost", m.cur)),
			huh.NewText().Title("Notes").Value(&values.Notes),
		),
	)
	m.activateForm(form, values)
}

func (m *Model) submitServiceLogForm() error {
	entry, vendor, err := m.parseServiceLogFormData()
	if err != nil {
		return err
	}
	return m.createOrUpdate(&entry.ID,
		func() error { return m.store.CreateServiceLog(&entry, vendor) },
		func() error { return m.store.UpdateServiceLog(entry, vendor) },
	)
}

func (m *Model) parseServiceLogFormData() (data.ServiceLogEntry, data.Vendor, error) {
	values, err := formDataAs[serviceLogFormData](m)
	if err != nil {
		return data.ServiceLogEntry{}, data.Vendor{}, err
	}
	servicedAt, err := data.ParseRequiredDate(values.ServicedAt)
	if err != nil {
		return data.ServiceLogEntry{}, data.Vendor{}, data.FieldError("Serviced At", err)
	}
	cost, err := m.cur.ParseOptionalCents(values.Cost)
	if err != nil {
		return data.ServiceLogEntry{}, data.Vendor{}, data.FieldError("Cost", err)
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
	values := serviceLogFormValues(entry, m.cur)
	switch col {
	case serviceLogColDate:
		m.openDatePicker(id, &values.ServicedAt, values)
	case serviceLogColPerformedBy:
		vendorOpts := vendorOpts("Self (homeowner)", m.vendors)
		field := huh.NewSelect[uint]().
			Title("Performed by").
			Options(vendorOpts...).
			Value(&values.VendorID)
		m.openInlineEdit(id, field, values)
	case serviceLogColCost:
		m.openInlineInput(
			id,
			"Cost",
			"125.00",
			&values.Cost,
			optionalMoney("cost", m.cur),
			values,
		)
	case serviceLogColNotes:
		m.openNotesEdit(id, &values.Notes, values)
	case serviceLogColID, serviceLogColDocs:
		return m.startEditServiceLogForm(id)
	}
	return nil
}

func serviceLogFormValues(entry data.ServiceLogEntry, cur locale.Currency) *serviceLogFormData {
	var vendorID uint
	if entry.VendorID != nil {
		vendorID = *entry.VendorID
	}
	return &serviceLogFormData{
		MaintenanceItemID: entry.MaintenanceItemID,
		ServicedAt:        entry.ServicedAt.Format(data.DateLayout),
		VendorID:          vendorID,
		Cost:              cur.FormatOptionalCents(entry.CostCents),
		Notes:             entry.Notes,
	}
}

func requiredDate(label string) func(string) error {
	return func(input string) error {
		if strings.TrimSpace(input) == "" {
			return fmt.Errorf("%s is required", label)
		}
		if _, err := data.ParseRequiredDate(input); err != nil {
			return data.FieldError(label, err)
		}
		return nil
	}
}

func applianceOptions(appliances []data.Appliance) []huh.Option[uint] {
	opts := buildOptions(appliances, func(a data.Appliance) (string, uint) {
		return labelWithDetail(a.Name, a.Brand), a.ID
	})
	return append([]huh.Option[uint]{huh.NewOption("(none)", uint(0))}, opts...)
}

// entityOptionLabel colors the entire label using the kind's color from the
// Entity column palette.
func entityOptionLabel(kind, label string) string {
	letter, ok := entityKindLetter[kind]
	if !ok {
		return label
	}
	if s, ok := appStyles.EntityKindStyle(letter[0]); ok {
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
			label = fmt.Sprintf("%s (%s)", label, a.Brand)
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
			label = fmt.Sprintf("%s (%s)", label, v.ContactName)
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
	dateField *string,
	values formData,
) {
	m.fs.editID = &id
	m.fs.formData = values
	savedKind := values.formKind()
	m.openCalendar(dateField, func() {
		if err := m.handleFormSubmit(); err != nil {
			m.setStatusError(err.Error())
		} else {
			m.setStatusSaved()
			m.reloadAfterFormSave(savedKind)
		}
		m.fs.formData = nil
		m.fs.editID = nil
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
func (m *Model) activateForm(form *huh.Form, values formData) {
	applyFormDefaults(form)
	// Set form width before Init so groups render at the correct terminal
	// width immediately. Without this, groups start at the default 80
	// columns and jump when the deferred WindowSizeMsg arrives.
	// The house form sets its own narrower width before calling
	// activateForm, so skip the override for that case.
	if m.width > 0 && values.formKind() != formHouse {
		form.WithWidth(m.width)
	}
	m.prevMode = m.mode
	m.mode = modeForm
	m.fs.form = form
	m.fs.formData = values
	m.fs.formHasRequired = true
	m.fs.pendingFormInit = form.Init()
	m.snapshotForm()
}

// openInlineEdit sets up a single-field inline edit form (overlay).
// Used for Select fields where a list picker is needed.
func (m *Model) openInlineEdit(id uint, field huh.Field, values formData) {
	m.fs.editID = &id
	m.activateForm(huh.NewForm(huh.NewGroup(field)), values)
	m.fs.formHasRequired = false
}

// openNotesEdit opens a standalone textarea overlay for editing a notes field.
// On submit the form data is saved via the handler, just like openInlineEdit
// for select fields. The textarea supports ctrl+e to escalate to $EDITOR.
func (m *Model) openNotesEdit(id uint, fieldPtr *string, values formData) {
	m.fs.editID = &id
	m.fs.formData = values
	m.openNotesTextarea(fieldPtr, values)
}

// openNotesTextarea creates and activates a textarea form for notes editing.
// Separated from openNotesEdit so it can be reused when reopening after an
// external editor session.
func (m *Model) openNotesTextarea(fieldPtr *string, values formData) {
	field := huh.NewText().Title("Notes").Value(fieldPtr)
	form := huh.NewForm(huh.NewGroup(field))
	m.activateForm(form, values)
	m.fs.formHasRequired = false
	m.fs.notesEditMode = true
	m.fs.notesFieldPtr = fieldPtr
}

// openInlineInput sets up a single-field text edit rendered in the status bar,
// keeping the table visible. Used for simple text and number fields.
func (m *Model) openInlineInput(
	id uint,
	title, placeholder string,
	fieldPtr *string,
	validate func(string) error,
	values formData,
) {
	ti := textinput.New()
	ti.SetValue(*fieldPtr)
	ti.Placeholder = placeholder
	ti.Focus()
	ti.Prompt = ""
	ti.CharLimit = 256
	m.fs.editID = &id
	m.fs.formData = values
	m.inlineInput = &inlineInputState{
		Input:    ti,
		Title:    title,
		EditID:   id,
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
// It returns a huh.ThemeFunc so the form can re-resolve colors when the
// terminal's dark/light status changes.
func formTheme() huh.ThemeFunc {
	return func(isDark bool) *huh.Styles {
		t := huh.ThemeBase(isDark)

		accent := accentPair.resolve(isDark)
		secondary := secondaryPair.resolve(isDark)
		success := successPair.resolve(isDark)
		textBright := textBrightPair.resolve(isDark)
		textMid := textMidPair.resolve(isDark)
		textDim := textDimPair.resolve(isDark)
		surface := surfacePair.resolve(isDark)
		onAccent := onAccentPair.resolve(isDark)
		border := borderPair.resolve(isDark)

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
		t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(success).Bold(true)
		t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(success).SetString("[•] ")
		t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(textMid).SetString("[ ] ")
		t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(textBright)
		t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(onAccent).Background(accent)
		t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(textMid).Background(surface)

		t.Focused.Directory = lipgloss.NewStyle().Foreground(accent).Bold(true)
		t.Focused.File = lipgloss.NewStyle().Foreground(textBright)

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
}

func formKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Quit.SetKeys(keyEsc)
	keymap.Quit.SetHelp(keyEsc, "cancel")
	return keymap
}

func (m *Model) handleFormSubmit() error {
	kind := m.fs.formKind()
	if kind == formHouse {
		return m.submitHouseForm()
	}
	handler := m.handlerForFormKind(kind)
	if handler == nil {
		return fmt.Errorf("no handler for form kind %v", kind)
	}
	return handler.SubmitForm(m)
}

func (m *Model) submitHouseForm() error {
	values, err := formDataAs[houseFormData](m)
	if err != nil {
		return err
	}
	yearBuilt, err := data.ParseOptionalInt(values.YearBuilt)
	if err != nil {
		return data.FieldError("Year Built", err)
	}
	sqftDisplay, err := data.ParseOptionalInt(values.SquareFeet)
	if err != nil {
		return data.FieldError(data.AreaFormTitle(m.unitSystem), err)
	}
	sqft := data.DisplayIntToSqFt(sqftDisplay, m.unitSystem)
	lotDisplay, err := data.ParseOptionalInt(values.LotSquareFeet)
	if err != nil {
		return data.FieldError(data.LotAreaFormTitle(m.unitSystem), err)
	}
	lotSqft := data.DisplayIntToSqFt(lotDisplay, m.unitSystem)
	bedrooms, err := data.ParseOptionalInt(values.Bedrooms)
	if err != nil {
		return data.FieldError("Bedrooms", err)
	}
	bathrooms, err := data.ParseOptionalFloat(values.Bathrooms)
	if err != nil {
		return data.FieldError("Bathrooms", err)
	}
	insuranceRenewal, err := data.ParseOptionalDate(values.InsuranceRenewal)
	if err != nil {
		return data.FieldError("Insurance Renewal", err)
	}
	propertyTax, err := m.cur.ParseOptionalCents(values.PropertyTax)
	if err != nil {
		return data.FieldError("Property Tax", err)
	}
	hoaFee, err := m.cur.ParseOptionalCents(values.HOAFee)
	if err != nil {
		return data.FieldError("HOA Fee", err)
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
	return m.createOrUpdate(&project.ID,
		func() error { return m.store.CreateProject(&project) },
		func() error { return m.store.UpdateProject(project) },
	)
}

func (m *Model) parseProjectFormData() (data.Project, error) {
	values, err := formDataAs[projectFormData](m)
	if err != nil {
		return data.Project{}, err
	}
	budget, err := m.cur.ParseOptionalCents(values.Budget)
	if err != nil {
		return data.Project{}, data.FieldError("Budget", err)
	}
	actual, err := m.cur.ParseOptionalCents(values.Actual)
	if err != nil {
		return data.Project{}, data.FieldError("Actual", err)
	}
	startDate, err := data.ParseOptionalDate(values.StartDate)
	if err != nil {
		return data.Project{}, data.FieldError("Start Date", err)
	}
	endDate, err := data.ParseOptionalDate(values.EndDate)
	if err != nil {
		return data.Project{}, data.FieldError("End Date", err)
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
	return m.createOrUpdate(&quote.ID,
		func() error { return m.store.CreateQuote(&quote, vendor) },
		func() error { return m.store.UpdateQuote(quote, vendor) },
	)
}

func (m *Model) parseQuoteFormData() (data.Quote, data.Vendor, error) {
	values, err := formDataAs[quoteFormData](m)
	if err != nil {
		return data.Quote{}, data.Vendor{}, err
	}
	total, err := m.cur.ParseRequiredCents(values.Total)
	if err != nil {
		return data.Quote{}, data.Vendor{}, data.FieldError("Total", err)
	}
	labor, err := m.cur.ParseOptionalCents(values.Labor)
	if err != nil {
		return data.Quote{}, data.Vendor{}, data.FieldError("Labor", err)
	}
	materials, err := m.cur.ParseOptionalCents(values.Materials)
	if err != nil {
		return data.Quote{}, data.Vendor{}, data.FieldError("Materials", err)
	}
	other, err := m.cur.ParseOptionalCents(values.Other)
	if err != nil {
		return data.Quote{}, data.Vendor{}, data.FieldError("Other", err)
	}
	received, err := data.ParseOptionalDate(values.ReceivedDate)
	if err != nil {
		return data.Quote{}, data.Vendor{}, data.FieldError("Received Date", err)
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
	return m.createOrUpdate(&item.ID,
		func() error { return m.store.CreateMaintenance(&item) },
		func() error { return m.store.UpdateMaintenance(item) },
	)
}

func (m *Model) parseMaintenanceFormData() (data.MaintenanceItem, error) {
	values, err := formDataAs[maintenanceFormData](m)
	if err != nil {
		return data.MaintenanceItem{}, err
	}
	lastServiced, err := data.ParseOptionalDate(values.LastServiced)
	if err != nil {
		return data.MaintenanceItem{}, data.FieldError("Last Serviced", err)
	}

	// The schedule type selector enforces mutual exclusion at the UI level:
	// only the field matching the selected type is parsed.
	var interval int
	var dueDate *time.Time

	switch values.ScheduleType {
	case schedNone:
	case schedInterval:
		interval, err = data.ParseIntervalMonths(values.IntervalMonths)
		if err != nil {
			return data.MaintenanceItem{}, data.FieldError("Interval", err)
		}
	case schedDueDate:
		dueDate, err = data.ParseOptionalDate(values.DueDate)
		if err != nil {
			return data.MaintenanceItem{}, data.FieldError("Due Date", err)
		}
	}

	cost, err := m.cur.ParseOptionalCents(values.Cost)
	if err != nil {
		return data.MaintenanceItem{}, data.FieldError("Cost", err)
	}
	var appID *uint
	if values.ApplianceID > 0 {
		appID = &values.ApplianceID
	}
	return data.MaintenanceItem{
		Name:           strings.TrimSpace(values.Name),
		CategoryID:     values.CategoryID,
		ApplianceID:    appID,
		Season:         values.Season,
		LastServicedAt: lastServiced,
		IntervalMonths: interval,
		DueDate:        dueDate,
		ManualURL:      strings.TrimSpace(values.ManualURL),
		ManualText:     strings.TrimSpace(values.ManualText),
		CostCents:      cost,
		Notes:          strings.TrimSpace(values.Notes),
	}, nil
}

func buildOptions[T any](items []T, entry func(T) (string, uint)) []huh.Option[uint] {
	opts := make([]huh.Option[uint], 0, len(items))
	for _, item := range items {
		label, id := entry(item)
		opts = append(opts, huh.NewOption(label, id))
	}
	return withOrdinals(opts)
}

func projectTypeOptions(types []data.ProjectType) []huh.Option[uint] {
	return buildOptions(types, func(t data.ProjectType) (string, uint) { return t.Name, t.ID })
}

func maintenanceOptions(categories []data.MaintenanceCategory) []huh.Option[uint] {
	return buildOptions(
		categories,
		func(c data.MaintenanceCategory) (string, uint) { return c.Name, c.ID },
	)
}

func projectOptions(projects []data.Project) []huh.Option[uint] {
	return buildOptions(projects, func(p data.Project) (string, uint) {
		label := p.Title
		if label == "" {
			label = fmt.Sprintf("Project %d", p.ID)
		}
		return label, p.ID
	})
}

func statusOptions() []huh.Option[string] {
	type entry struct {
		value string
		color adaptiveColor
	}
	statuses := []entry{
		{data.ProjectStatusIdeating, mutedPair},
		{data.ProjectStatusPlanned, accentPair},
		{data.ProjectStatusQuoted, secondaryPair},
		{data.ProjectStatusInProgress, successPair},
		{data.ProjectStatusDelayed, warningPair},
		{data.ProjectStatusCompleted, textDimPair},
		{data.ProjectStatusAbandoned, dangerPair},
	}
	opts := make([]huh.Option[string], len(statuses))
	for i, s := range statuses {
		label := statusLabel(s.value)
		colored := lipgloss.NewStyle().Foreground(s.color.resolve(appIsDark)).Render(label)
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

// validateWith builds a validator from a parse function. The parse result is
// discarded; only the error matters. Eliminates repetitive validator factories.
func validateWith[T any](label string, parse func(string) (T, error)) func(string) error {
	return func(input string) error {
		if _, err := parse(input); err != nil {
			return data.FieldError(label, err)
		}
		return nil
	}
}

func optionalInt(label string) func(string) error {
	return validateWith(label, data.ParseOptionalInt)
}

func optionalInterval(label string) func(string) error {
	return validateWith(label, data.ParseIntervalMonths)
}

func optionalFloat(label string) func(string) error {
	return validateWith(label, data.ParseOptionalFloat)
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
		s, err := data.ParseOptionalDate(start)
		if err != nil || s == nil {
			return nil //nolint:nilerr // start date validated by its own field
		}
		e, err := data.ParseOptionalDate(end)
		if err != nil || e == nil {
			return nil //nolint:nilerr // end date format already checked by optionalDate above
		}
		if e.Before(*s) {
			return fmt.Errorf("end date must not be before start date")
		}
		return nil
	}
}

func optionalDate(label string) func(string) error {
	return validateWith(label, data.ParseOptionalDate)
}

func optionalMoney(label string, cur locale.Currency) func(string) error {
	return validateWith(label, cur.ParseOptionalCents)
}

func requiredMoney(label string, cur locale.Currency) func(string) error {
	return validateWith(label, cur.ParseRequiredCents)
}

func projectFormValues(project data.Project, cur locale.Currency) *projectFormData {
	return &projectFormData{
		Title:         project.Title,
		ProjectTypeID: project.ProjectTypeID,
		Status:        project.Status,
		Budget:        cur.FormatOptionalCents(project.BudgetCents),
		Actual:        cur.FormatOptionalCents(project.ActualCents),
		StartDate:     data.FormatDate(project.StartDate),
		EndDate:       data.FormatDate(project.EndDate),
		Description:   project.Description,
	}
}

func quoteFormValues(quote data.Quote, cur locale.Currency) *quoteFormData {
	return &quoteFormData{
		ProjectID:    quote.ProjectID,
		VendorName:   quote.Vendor.Name,
		ContactName:  quote.Vendor.ContactName,
		Email:        quote.Vendor.Email,
		Phone:        quote.Vendor.Phone,
		Website:      quote.Vendor.Website,
		VendorNotes:  quote.Vendor.Notes,
		Total:        cur.FormatCents(quote.TotalCents),
		Labor:        cur.FormatOptionalCents(quote.LaborCents),
		Materials:    cur.FormatOptionalCents(quote.MaterialsCents),
		Other:        cur.FormatOptionalCents(quote.OtherCents),
		ReceivedDate: data.FormatDate(quote.ReceivedDate),
		Notes:        quote.Notes,
	}
}

func maintenanceFormValues(item data.MaintenanceItem, cur locale.Currency) *maintenanceFormData {
	var appID uint
	if item.ApplianceID != nil {
		appID = *item.ApplianceID
	}
	sched := schedNone
	switch {
	case item.IntervalMonths > 0:
		sched = schedInterval
	case item.DueDate != nil:
		sched = schedDueDate
	}
	return &maintenanceFormData{
		Name:           item.Name,
		CategoryID:     item.CategoryID,
		ApplianceID:    appID,
		Season:         item.Season,
		ScheduleType:   sched,
		LastServiced:   data.FormatDate(item.LastServicedAt),
		IntervalMonths: formatInterval(item.IntervalMonths),
		DueDate:        data.FormatDate(item.DueDate),
		ManualURL:      item.ManualURL,
		ManualText:     item.ManualText,
		Cost:           cur.FormatOptionalCents(item.CostCents),
		Notes:          item.Notes,
	}
}

func applianceFormValues(item data.Appliance, cur locale.Currency) *applianceFormData {
	return &applianceFormData{
		Name:           item.Name,
		Brand:          item.Brand,
		ModelNumber:    item.ModelNumber,
		SerialNumber:   item.SerialNumber,
		PurchaseDate:   data.FormatDate(item.PurchaseDate),
		WarrantyExpiry: data.FormatDate(item.WarrantyExpiry),
		Location:       item.Location,
		Cost:           cur.FormatOptionalCents(item.CostCents),
		Notes:          item.Notes,
	}
}

func (m *Model) houseFormValues(profile data.HouseProfile) *houseFormData {
	return &houseFormData{
		Nickname:         profile.Nickname,
		AddressLine1:     profile.AddressLine1,
		AddressLine2:     profile.AddressLine2,
		City:             profile.City,
		State:            profile.State,
		PostalCode:       profile.PostalCode,
		YearBuilt:        intToString(profile.YearBuilt),
		SquareFeet:       intToString(data.SqFtToDisplayInt(profile.SquareFeet, m.unitSystem)),
		LotSquareFeet:    intToString(data.SqFtToDisplayInt(profile.LotSquareFeet, m.unitSystem)),
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
		PropertyTax:      m.cur.FormatOptionalCents(profile.PropertyTaxCents),
		HOAName:          profile.HOAName,
		HOAFee:           m.cur.FormatOptionalCents(profile.HOAFeeCents),
	}
}

func (m *Model) createOrUpdate(
	idPtr *uint,
	create func() error,
	update func() error,
) error {
	if m.fs.editID != nil {
		*idPtr = *m.fs.editID
		return update()
	}
	if err := create(); err != nil {
		return err
	}
	id := *idPtr
	m.fs.editID = &id
	return nil
}

// formDataAs asserts m.fs.formData to the given pointer type, returning a
// typed error on mismatch. Eliminates the repeated type-assertion boilerplate
// in every parse* function.
func formDataAs[T any](m *Model) (*T, error) {
	v, ok := any(m.fs.formData).(*T)
	if !ok {
		var zero T
		return nil, fmt.Errorf("unexpected form data: want *%T, got %T", zero, m.fs.formData)
	}
	return v, nil
}

// requiredTitle appends a colored ∗ (U+2217) to a form field label.
func requiredTitle(label string) string {
	return label + appStyles.SecondaryText().Render(" ∗")
}

// requiredLegend returns the "∗ required" legend line for forms that have
// required fields. Returns empty string when the form has none.
func (m *Model) requiredLegend() string {
	if !m.fs.formHasRequired {
		return ""
	}
	return appStyles.SecondaryText().Render("∗") + appStyles.TextDim().Render(" required")
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
		m.newDocumentFilePicker("File to attach").
			Value(&values.FilePath),
		huh.NewText().Title("Notes").Value(&values.Notes),
	)

	form := huh.NewForm(huh.NewGroup(fields...))
	m.activateForm(form, values)
	return nil
}

// startQuickDocumentForm opens a minimal document form that only asks for a
// file path. Title and notes are auto-filled by the extraction pipeline on
// submit, making this the fast path for ingesting files.
func (m *Model) startQuickDocumentForm() error {
	values := &documentFormData{DeferCreate: true}
	form := huh.NewForm(
		huh.NewGroup(
			m.newDocumentFilePicker("File to attach").
				Value(&values.FilePath),
		),
	)
	m.activateForm(form, values)
	return nil
}

func (m *Model) startEditDocumentForm(id uint) error {
	doc, err := m.store.GetDocumentMetadata(id)
	if err != nil {
		return fmt.Errorf("load document: %w", err)
	}
	values := documentFormValues(doc)
	m.fs.editID = &id

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
		m.newDocumentFilePicker("Replacement file").
			Value(&values.FilePath),
		huh.NewText().Title("Notes").Value(&values.Notes),
	)

	form := huh.NewForm(huh.NewGroup(fields...))
	m.activateForm(form, values)
	return nil
}

// newDocumentFilePicker creates a file picker pre-configured for document
// uploads: files only, hidden files shown, height scaled to the terminal.
func (m *Model) newDocumentFilePicker(title string) *huh.FilePicker {
	h := m.height / 3
	if h < 5 {
		h = 5
	}
	// Use the configured starting directory (defaults to ~/Downloads).
	// Fall back to cwd if the configured dir is empty.
	dir := m.filePickerDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			dir = "."
		}
	}
	short := "\x1b[22m" + dimPath.Render("in "+shortenHome(dir))
	return huh.NewFilePicker().
		Key(title).
		Title(title + " " + short).
		Description(filePickerDesc(false)).
		Cursor(symTriRightSm).
		CurrentDirectory(dir).
		Picking(true).
		FileAllowed(true).
		DirAllowed(false).
		ShowHidden(false).
		ShowPermissions(false).
		Height(h)
}

func (m *Model) submitDocumentForm() error {
	result, err := m.parseDocumentFormData()
	if err != nil {
		return err
	}
	doc := result.Doc
	if err := m.createOrUpdate(&doc.ID,
		func() error { return m.store.CreateDocument(&doc) },
		func() error { return m.store.UpdateDocument(doc) },
	); err != nil {
		return err
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
	if err := m.createOrUpdate(&doc.ID,
		func() error { return m.store.CreateDocument(&doc) },
		func() error { return m.store.UpdateDocument(doc) },
	); err != nil {
		return err
	}
	if result.ExtractErr != nil {
		m.setStatusInfo(fmt.Sprintf("extraction incomplete: %s", result.ExtractErr))
	}
	return nil
}

func (m *Model) parseDocumentFormData() (documentParseResult, error) {
	values, err := formDataAs[documentFormData](m)
	if err != nil {
		return documentParseResult{}, err
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

		// Run text extraction synchronously (instant, pure Go). Async
		// extraction and LLM run in the extraction overlay after save.
		var extractErr error
		text, err := extract.ExtractText(
			fileData,
			doc.MIMEType,
			extract.ExtractorTimeout(m.ex.extractors),
		)
		if err != nil {
			extractErr = err
		}
		doc.ExtractedText = text

		// Show one-time tesseract hint if extraction tools aren't available.
		if extract.IsScanned(doc.ExtractedText) && !extract.OCRAvailable() {
			if extract.IsImageMIME(doc.MIMEType) || doc.MIMEType == "application/pdf" {
				m.showTesseractHint()
			}
		}

		// Title defaults to filename; LLM may improve it asynchronously.
		if doc.Title == "" {
			doc.Title = data.TitleFromFilename(doc.FileName)
		}

		return documentParseResult{
			Doc:        doc,
			ExtractErr: extractErr,
		}, nil
	}
	return documentParseResult{Doc: doc}, nil
}

// showTesseractHint displays a one-time status bar hint suggesting the
// user install tesseract for better document extraction. The hint is
// persisted in the DB so it's never shown again.
func (m *Model) showTesseractHint() {
	if m.store.TesseractHintSeen() {
		return
	}
	m.setStatusInfo("install tesseract for text extraction from scanned docs")
	// Best-effort: hint reappears next session if persist fails.
	_ = m.store.MarkTesseractHintSeen()
}

func (m *Model) inlineEditDocument(id uint, col documentCol) error {
	doc, err := m.store.GetDocumentMetadata(id)
	if err != nil {
		return fmt.Errorf("load document: %w", err)
	}
	values := documentFormValues(doc)
	switch col {
	case documentColTitle:
		m.openInlineInput(
			id,
			"Title",
			"",
			&values.Title,
			requiredText("title"),
			values,
		)
	case documentColNotes:
		m.openNotesEdit(id, &values.Notes, values)
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
		m.openInlineEdit(id, field, values)
	case documentColID,
		documentColType,
		documentColSize,
		documentColModel,
		documentColOps,
		documentColUpdated:
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
