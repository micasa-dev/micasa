// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/micasa-dev/micasa/internal/config"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/locale"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func fmtMoney(cents *int64) string {
	if cents == nil {
		return "-"
	}
	return fmtMoneyVal(*cents)
}

func fmtDate(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return fmtDateVal(*t)
}

func fmtInt(n int) string {
	if n == 0 {
		return "-"
	}
	return strconv.Itoa(n)
}

func fmtFloat(f float64) string {
	if f == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", f)
}

// showCol defines a column for tabular text output of an entity.
type showCol[T any] struct {
	header string
	value  func(T) string
}

// showEntity renders a slice of entities as either a text table or JSON array.
func showEntity[T any](
	w io.Writer,
	header string,
	items []T,
	cols []showCol[T],
	toMap func(T) map[string]any,
	asJSON bool,
) error {
	if asJSON {
		return writeJSON(w, items, toMap)
	}
	return writeTable(w, header, items, cols)
}

func writeTable[T any](w io.Writer, header string, items []T, cols []showCol[T]) error {
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "=== %s ===\n", header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	// header row
	hdrs := make([]string, len(cols))
	for i, c := range cols {
		hdrs[i] = c.header
	}
	if _, err := fmt.Fprintln(tw, strings.Join(hdrs, "\t")); err != nil {
		return fmt.Errorf("write column headers: %w", err)
	}

	for _, item := range items {
		vals := make([]string, len(cols))
		for i, c := range cols {
			vals[i] = c.value(item)
		}
		if _, err := fmt.Fprintln(tw, strings.Join(vals, "\t")); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush table: %w", err)
	}
	return nil
}

func writeJSON[T any](w io.Writer, items []T, toMap func(T) map[string]any) error {
	out := make([]map[string]any, len(items))
	for i, item := range items {
		out[i] = toMap(item)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}

func fmtStr(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func fmtDateVal(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(data.DateLayout)
}

// withDeletedCol appends a DELETED column and deleted_at JSON field when includeDeleted is true.
func withDeletedCol[T any](
	cols []showCol[T],
	toMap func(T) map[string]any,
	includeDeleted bool,
	deletedAt func(T) gorm.DeletedAt,
) ([]showCol[T], func(T) map[string]any) {
	if !includeDeleted {
		return cols, toMap
	}
	extended := make([]showCol[T], len(cols)+1)
	copy(extended, cols)
	extended[len(cols)] = showCol[T]{
		header: "DELETED",
		value: func(item T) string {
			da := deletedAt(item)
			if da.Valid {
				return da.Time.Format(data.DateLayout)
			}
			return "-"
		},
	}
	extendedToMap := func(item T) map[string]any {
		m := toMap(item)
		da := deletedAt(item)
		if da.Valid {
			m["deleted_at"] = da.Time
		}
		return m
	}
	return extended, extendedToMap
}

// --- show command ---

func newShowCmd() *cobra.Command {
	var jsonFlag bool
	var deletedFlag bool

	cmd := &cobra.Command{
		Use:   "show <entity>",
		Short: "Display data as text or JSON",
		Long: `Print entity data to stdout. Entities: house, projects, project-types,
quotes, vendors, maintenance, maintenance-categories, service-log,
appliances, incidents, documents, all.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output as JSON")
	cmd.PersistentFlags().BoolVar(&deletedFlag, "deleted", false, "Include soft-deleted rows")

	// Per-entity show subcommands are deprecated in favor of
	// "micasa <entity> list --table". The "all" subcommand is not
	// deprecated since it has no replacement.
	type showDef struct {
		name       string
		short      string
		fn         func(io.Writer, *data.Store, bool, bool) error
		deprecated string // empty = not deprecated
	}
	entityDefs := []showDef{
		{
			entityProjects,
			"Show projects",
			showProjects,
			"use 'micasa project list --table' instead",
		},
		{entityVendors, "Show vendors", showVendors, "use 'micasa vendor list --table' instead"},
		{
			entityAppliances,
			"Show appliances",
			showAppliances,
			"use 'micasa appliance list --table' instead",
		},
		{
			entityIncidents,
			"Show incidents",
			showIncidents,
			"use 'micasa incident list --table' instead",
		},
		{entityQuotes, "Show quotes", showQuotes, "use 'micasa quote list --table' instead"},
		{
			entityMaintenance,
			"Show maintenance items",
			showMaintenance,
			"use 'micasa maintenance list --table' instead",
		},
		{
			entityServiceLog,
			"Show service log entries",
			showServiceLog,
			"use 'micasa service-log list --table' instead",
		},
		{
			entityDocuments,
			"Show documents",
			showDocuments,
			"use 'micasa document list --table' instead",
		},
		{
			"project-types",
			"Show project types",
			showProjectTypes,
			"use 'micasa project-type list' instead",
		},
		{
			"maintenance-categories",
			"Show maintenance categories",
			showMaintenanceCategories,
			"use 'micasa maintenance-category list' instead",
		},
	}
	houseCmd := newShowHouseCmd(&jsonFlag)
	houseCmd.Deprecated = "use 'micasa house get' instead"
	cmd.AddCommand(houseCmd)
	for _, d := range entityDefs {
		sub := newShowEntityCmd(d.name, d.short, &jsonFlag, &deletedFlag, d.fn)
		if d.deprecated != "" {
			sub.Deprecated = d.deprecated
		}
		cmd.AddCommand(sub)
	}
	cmd.AddCommand(newShowEntityCmd("all", "Show all entities", &jsonFlag, &deletedFlag, showAll))

	return cmd
}

func newShowEntityCmd(
	name, short string,
	jsonFlag, deletedFlag *bool,
	showFn func(io.Writer, *data.Store, bool, bool) error,
) *cobra.Command {
	return &cobra.Command{
		Use:           name + " [database-path]",
		Short:         short,
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			return showFn(cmd.OutOrStdout(), store, *jsonFlag, *deletedFlag)
		},
	}
}

func newShowHouseCmd(jsonFlag *bool) *cobra.Command {
	return &cobra.Command{
		Use:           "house [database-path]",
		Short:         "Show house profile",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			return showHouse(cmd.OutOrStdout(), store, *jsonFlag)
		},
	}
}

func showHouse(w io.Writer, store *data.Store, asJSON bool) error {
	h, err := store.HouseProfile()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load house profile: %w", err)
	}

	if asJSON {
		return showHouseJSON(w, h)
	}
	return showHouseText(w, h)
}

func showHouseText(w io.Writer, h data.HouseProfile) error {
	if _, err := fmt.Fprintln(w, "=== HOUSE ==="); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	var writeErr error
	kv := func(label, value string) {
		if writeErr != nil {
			return
		}
		if value != "" && value != "-" {
			_, writeErr = fmt.Fprintf(tw, "%s:\t%s\n", label, value)
		}
	}

	kv("Nickname", h.Nickname)
	kv("Address", formatAddress(h))
	kv("Year Built", fmtInt(h.YearBuilt))
	kv("Square Feet", fmtInt(h.SquareFeet))
	kv("Lot Size", fmtInt(h.LotSquareFeet))
	kv("Bedrooms", fmtInt(h.Bedrooms))
	kv("Bathrooms", fmtFloat(h.Bathrooms))
	kv("Foundation", h.FoundationType)
	kv("Wiring", h.WiringType)
	kv("Roof", h.RoofType)
	kv("Exterior", h.ExteriorType)
	kv("Heating", h.HeatingType)
	kv("Cooling", h.CoolingType)
	kv("Water Source", h.WaterSource)
	kv("Sewer", h.SewerType)
	kv("Parking", h.ParkingType)
	kv("Basement", h.BasementType)
	kv("Insurance Carrier", h.InsuranceCarrier)
	kv("Insurance Policy", h.InsurancePolicy)
	kv("Insurance Renewal", fmtDate(h.InsuranceRenewal))
	kv("Property Tax", fmtMoney(h.PropertyTaxCents))
	kv("HOA", h.HOAName)
	kv("HOA Fee", fmtMoney(h.HOAFeeCents))

	if writeErr != nil {
		return fmt.Errorf("write field: %w", writeErr)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush table: %w", err)
	}
	return nil
}

func formatAddress(h data.HouseProfile) string {
	var parts []string
	if h.AddressLine1 != "" {
		parts = append(parts, h.AddressLine1)
	}
	if h.AddressLine2 != "" {
		parts = append(parts, h.AddressLine2)
	}
	var cityState []string
	if h.City != "" {
		cityState = append(cityState, h.City)
	}
	if h.State != "" {
		cityState = append(cityState, h.State)
	}
	csStr := strings.Join(cityState, ", ")
	if csStr != "" && h.PostalCode != "" {
		csStr += " " + h.PostalCode
	} else if h.PostalCode != "" {
		csStr = h.PostalCode
	}
	if csStr != "" {
		parts = append(parts, csStr)
	}
	return strings.Join(parts, ", ")
}

func showHouseJSON(w io.Writer, h data.HouseProfile) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(h); err != nil {
		return fmt.Errorf("encode house JSON: %w", err)
	}
	return nil
}

// --- projects ---

var projectCols = []showCol[data.Project]{
	{"TITLE", func(p data.Project) string { return fmtStr(p.Title) }},
	{"TYPE", func(p data.Project) string { return fmtStr(p.ProjectType.Name) }},
	{"STATUS", func(p data.Project) string { return fmtStr(p.Status) }},
	{"START", func(p data.Project) string { return fmtDate(p.StartDate) }},
	{"END", func(p data.Project) string { return fmtDate(p.EndDate) }},
	{"BUDGET", func(p data.Project) string { return fmtMoney(p.BudgetCents) }},
	{"ACTUAL", func(p data.Project) string { return fmtMoney(p.ActualCents) }},
	{"DESCRIPTION", func(p data.Project) string { return fmtStr(p.Description) }},
}

func projectToMap(p data.Project) map[string]any {
	return map[string]any{
		data.ColID:            p.ID,
		data.ColTitle:         p.Title,
		data.ColProjectTypeID: p.ProjectTypeID,
		"project_type":        p.ProjectType.Name,
		data.ColStatus:        p.Status,
		data.ColStartDate:     p.StartDate,
		data.ColEndDate:       p.EndDate,
		data.ColBudgetCents:   p.BudgetCents,
		data.ColActualCents:   p.ActualCents,
		data.ColDescription:   p.Description,
	}
}

func showProjects(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListProjects(includeDeleted)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	cols, toMap := withDeletedCol(projectCols, projectToMap, includeDeleted,
		func(p data.Project) gorm.DeletedAt { return p.DeletedAt })
	return showEntity(w, "PROJECTS", items, cols, toMap, asJSON)
}

// --- vendors ---

var vendorCols = []showCol[data.Vendor]{
	{colHdrName, func(v data.Vendor) string { return fmtStr(v.Name) }},
	{"CONTACT", func(v data.Vendor) string { return fmtStr(v.ContactName) }},
	{"EMAIL", func(v data.Vendor) string { return fmtStr(v.Email) }},
	{"PHONE", func(v data.Vendor) string {
		region := strings.ToUpper(config.DetectCountry())
		if v.Locale != "" {
			region = strings.ToUpper(v.Locale)
		}
		return fmtStr(locale.FormatPhoneNumber(v.Phone, region))
	}},
	{"WEBSITE", func(v data.Vendor) string { return fmtStr(v.Website) }},
}

func vendorToMap(v data.Vendor) map[string]any {
	return map[string]any{
		data.ColID:          v.ID,
		data.ColName:        v.Name,
		data.ColContactName: v.ContactName,
		data.ColEmail:       v.Email,
		data.ColPhone:       v.Phone,
		data.ColWebsite:     v.Website,
		data.ColNotes:       v.Notes,
	}
}

func showVendors(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListVendors(includeDeleted)
	if err != nil {
		return fmt.Errorf("list vendors: %w", err)
	}
	cols, toMap := withDeletedCol(vendorCols, vendorToMap, includeDeleted,
		func(v data.Vendor) gorm.DeletedAt { return v.DeletedAt })
	return showEntity(w, "VENDORS", items, cols, toMap, asJSON)
}

// --- appliances ---

var applianceCols = []showCol[data.Appliance]{
	{colHdrName, func(a data.Appliance) string { return fmtStr(a.Name) }},
	{"BRAND", func(a data.Appliance) string { return fmtStr(a.Brand) }},
	{"MODEL", func(a data.Appliance) string { return fmtStr(a.ModelNumber) }},
	{"SERIAL", func(a data.Appliance) string { return fmtStr(a.SerialNumber) }},
	{"LOCATION", func(a data.Appliance) string { return fmtStr(a.Location) }},
	{"PURCHASED", func(a data.Appliance) string { return fmtDate(a.PurchaseDate) }},
	{"WARRANTY", func(a data.Appliance) string { return fmtDate(a.WarrantyExpiry) }},
	{"COST", func(a data.Appliance) string { return fmtMoney(a.CostCents) }},
}

func applianceToMap(a data.Appliance) map[string]any {
	return map[string]any{
		data.ColID:             a.ID,
		data.ColName:           a.Name,
		data.ColBrand:          a.Brand,
		data.ColModelNumber:    a.ModelNumber,
		data.ColSerialNumber:   a.SerialNumber,
		data.ColLocation:       a.Location,
		data.ColPurchaseDate:   a.PurchaseDate,
		data.ColWarrantyExpiry: a.WarrantyExpiry,
		data.ColCostCents:      a.CostCents,
		data.ColNotes:          a.Notes,
	}
}

func showAppliances(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListAppliances(includeDeleted)
	if err != nil {
		return fmt.Errorf("list appliances: %w", err)
	}
	cols, toMap := withDeletedCol(applianceCols, applianceToMap, includeDeleted,
		func(a data.Appliance) gorm.DeletedAt { return a.DeletedAt })
	return showEntity(w, "APPLIANCES", items, cols, toMap, asJSON)
}

// --- incidents ---

var incidentCols = []showCol[data.Incident]{
	{"TITLE", func(i data.Incident) string { return fmtStr(i.Title) }},
	{"STATUS", func(i data.Incident) string { return fmtStr(i.Status) }},
	{"SEVERITY", func(i data.Incident) string { return fmtStr(i.Severity) }},
	{"NOTICED", func(i data.Incident) string { return fmtDateVal(i.DateNoticed) }},
	{"RESOLVED", func(i data.Incident) string { return fmtDate(i.DateResolved) }},
	{"LOCATION", func(i data.Incident) string { return fmtStr(i.Location) }},
	{"COST", func(i data.Incident) string { return fmtMoney(i.CostCents) }},
	{"APPLIANCE", func(i data.Incident) string { return fmtStr(i.Appliance.Name) }},
	{"VENDOR", func(i data.Incident) string { return fmtStr(i.Vendor.Name) }},
}

func incidentToMap(i data.Incident) map[string]any {
	return map[string]any{
		data.ColID:           i.ID,
		data.ColTitle:        i.Title,
		data.ColStatus:       i.Status,
		data.ColSeverity:     i.Severity,
		data.ColDateNoticed:  i.DateNoticed,
		data.ColDateResolved: i.DateResolved,
		data.ColLocation:     i.Location,
		data.ColCostCents:    i.CostCents,
		data.ColApplianceID:  i.ApplianceID,
		entityAppliance:      i.Appliance.Name,
		data.ColVendorID:     i.VendorID,
		entityVendor:         i.Vendor.Name,
		data.ColDescription:  i.Description,
		data.ColNotes:        i.Notes,
	}
}

func showIncidents(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListIncidents(includeDeleted)
	if err != nil {
		return fmt.Errorf("list incidents: %w", err)
	}
	cols, toMap := withDeletedCol(incidentCols, incidentToMap, includeDeleted,
		func(i data.Incident) gorm.DeletedAt { return i.DeletedAt })
	return showEntity(w, "INCIDENTS", items, cols, toMap, asJSON)
}

// --- quotes ---

func fmtMoneyVal(cents int64) string {
	return fmt.Sprintf("$%.2f", float64(cents)/100)
}

var quoteCols = []showCol[data.Quote]{
	{"PROJECT", func(q data.Quote) string { return fmtStr(q.Project.Title) }},
	{"VENDOR", func(q data.Quote) string { return fmtStr(q.Vendor.Name) }},
	{"TOTAL", func(q data.Quote) string { return fmtMoneyVal(q.TotalCents) }},
	{"LABOR", func(q data.Quote) string { return fmtMoney(q.LaborCents) }},
	{"MATERIALS", func(q data.Quote) string { return fmtMoney(q.MaterialsCents) }},
	{"RECEIVED", func(q data.Quote) string { return fmtDate(q.ReceivedDate) }},
	{"NOTES", func(q data.Quote) string { return fmtStr(q.Notes) }},
}

func quoteToMap(q data.Quote) map[string]any {
	return map[string]any{
		data.ColID:             q.ID,
		data.ColProjectID:      q.ProjectID,
		"project":              q.Project.Title,
		data.ColVendorID:       q.VendorID,
		entityVendor:           q.Vendor.Name,
		data.ColTotalCents:     q.TotalCents,
		data.ColLaborCents:     q.LaborCents,
		data.ColMaterialsCents: q.MaterialsCents,
		data.ColReceivedDate:   q.ReceivedDate,
		data.ColNotes:          q.Notes,
	}
}

func showQuotes(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListQuotes(includeDeleted)
	if err != nil {
		return fmt.Errorf("list quotes: %w", err)
	}
	cols, toMap := withDeletedCol(quoteCols, quoteToMap, includeDeleted,
		func(q data.Quote) gorm.DeletedAt { return q.DeletedAt })
	return showEntity(w, "QUOTES", items, cols, toMap, asJSON)
}

// --- maintenance ---

var maintenanceCols = []showCol[data.MaintenanceItem]{
	{colHdrName, func(m data.MaintenanceItem) string { return fmtStr(m.Name) }},
	{"CATEGORY", func(m data.MaintenanceItem) string { return fmtStr(m.Category.Name) }},
	{"APPLIANCE", func(m data.MaintenanceItem) string { return fmtStr(m.Appliance.Name) }},
	{"SEASON", func(m data.MaintenanceItem) string { return fmtStr(m.Season) }},
	{"LAST SERVICED", func(m data.MaintenanceItem) string { return fmtDate(m.LastServicedAt) }},
	{"INTERVAL", func(m data.MaintenanceItem) string { return strconv.Itoa(m.IntervalMonths) }},
	{"DUE", func(m data.MaintenanceItem) string { return fmtDate(m.DueDate) }},
	{"COST", func(m data.MaintenanceItem) string { return fmtMoney(m.CostCents) }},
}

func maintenanceToMap(m data.MaintenanceItem) map[string]any {
	return map[string]any{
		data.ColID:             m.ID,
		data.ColName:           m.Name,
		data.ColCategoryID:     m.CategoryID,
		"category":             m.Category.Name,
		data.ColApplianceID:    m.ApplianceID,
		entityAppliance:        m.Appliance.Name,
		data.ColSeason:         m.Season,
		data.ColLastServicedAt: m.LastServicedAt,
		data.ColIntervalMonths: m.IntervalMonths,
		data.ColDueDate:        m.DueDate,
		data.ColCostCents:      m.CostCents,
		data.ColNotes:          m.Notes,
	}
}

func showMaintenance(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListMaintenance(includeDeleted)
	if err != nil {
		return fmt.Errorf("list maintenance: %w", err)
	}
	cols, toMap := withDeletedCol(maintenanceCols, maintenanceToMap, includeDeleted,
		func(m data.MaintenanceItem) gorm.DeletedAt { return m.DeletedAt })
	return showEntity(w, "MAINTENANCE", items, cols, toMap, asJSON)
}

// --- service-log ---

var serviceLogCols = []showCol[data.ServiceLogEntry]{
	{"ITEM", func(e data.ServiceLogEntry) string { return fmtStr(e.MaintenanceItem.Name) }},
	{"VENDOR", func(e data.ServiceLogEntry) string { return fmtStr(e.Vendor.Name) }},
	{"SERVICED", func(e data.ServiceLogEntry) string { return fmtDateVal(e.ServicedAt) }},
	{"COST", func(e data.ServiceLogEntry) string { return fmtMoney(e.CostCents) }},
	{"NOTES", func(e data.ServiceLogEntry) string { return fmtStr(e.Notes) }},
}

func serviceLogToMap(e data.ServiceLogEntry) map[string]any {
	return map[string]any{
		data.ColID:                e.ID,
		data.ColMaintenanceItemID: e.MaintenanceItemID,
		"maintenance_item":        e.MaintenanceItem.Name,
		data.ColVendorID:          e.VendorID,
		entityVendor:              e.Vendor.Name,
		data.ColServicedAt:        e.ServicedAt,
		data.ColCostCents:         e.CostCents,
		data.ColNotes:             e.Notes,
	}
}

func showServiceLog(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListAllServiceLogEntries(includeDeleted)
	if err != nil {
		return fmt.Errorf("list service log: %w", err)
	}
	cols, toMap := withDeletedCol(serviceLogCols, serviceLogToMap, includeDeleted,
		func(e data.ServiceLogEntry) gorm.DeletedAt { return e.DeletedAt })
	return showEntity(w, "SERVICE LOG", items, cols, toMap, asJSON)
}

// --- documents ---

var documentCols = []showCol[data.Document]{
	{"TITLE", func(d data.Document) string { return fmtStr(d.Title) }},
	{"FILE", func(d data.Document) string { return fmtStr(d.FileName) }},
	{"ENTITY", func(d data.Document) string { return fmtStr(d.EntityKind) }},
	{"MIME", func(d data.Document) string { return fmtStr(d.MIMEType) }},
	{"SIZE", func(d data.Document) string { return strconv.FormatInt(d.SizeBytes, 10) }},
	{"NOTES", func(d data.Document) string { return fmtStr(d.Notes) }},
}

func documentToMap(d data.Document) map[string]any {
	return map[string]any{
		data.ColID:         d.ID,
		data.ColTitle:      d.Title,
		data.ColFileName:   d.FileName,
		data.ColEntityKind: d.EntityKind,
		data.ColEntityID:   d.EntityID,
		data.ColMIMEType:   d.MIMEType,
		data.ColSizeBytes:  d.SizeBytes,
		"sha256":           d.ChecksumSHA256,
		data.ColNotes:      d.Notes,
	}
}

func showDocuments(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListDocuments(includeDeleted)
	if err != nil {
		return fmt.Errorf("list documents: %w", err)
	}
	cols, toMap := withDeletedCol(documentCols, documentToMap, includeDeleted,
		func(d data.Document) gorm.DeletedAt { return d.DeletedAt })
	return showEntity(w, "DOCUMENTS", items, cols, toMap, asJSON)
}

// --- project-types ---

var projectTypeCols = []showCol[data.ProjectType]{
	{colHdrName, func(pt data.ProjectType) string { return fmtStr(pt.Name) }},
}

func projectTypeToMap(pt data.ProjectType) map[string]any {
	return map[string]any{
		data.ColID:   pt.ID,
		data.ColName: pt.Name,
	}
}

func showProjectTypes(w io.Writer, store *data.Store, asJSON, _ bool) error {
	items, err := store.ProjectTypes()
	if err != nil {
		return fmt.Errorf("list project types: %w", err)
	}
	return showEntity(w, "PROJECT TYPES", items, projectTypeCols, projectTypeToMap, asJSON)
}

// --- maintenance-categories ---

var maintenanceCategoryCols = []showCol[data.MaintenanceCategory]{
	{colHdrName, func(mc data.MaintenanceCategory) string { return fmtStr(mc.Name) }},
}

func maintenanceCategoryToMap(mc data.MaintenanceCategory) map[string]any {
	return map[string]any{
		data.ColID:   mc.ID,
		data.ColName: mc.Name,
	}
}

func showMaintenanceCategories(w io.Writer, store *data.Store, asJSON, _ bool) error {
	items, err := store.MaintenanceCategories()
	if err != nil {
		return fmt.Errorf("list maintenance categories: %w", err)
	}
	return showEntity(
		w,
		"MAINTENANCE CATEGORIES",
		items,
		maintenanceCategoryCols,
		maintenanceCategoryToMap,
		asJSON,
	)
}

// --- all ---

// mapSlice converts a slice of T to a slice of map[string]any using toMap.
func mapSlice[T any](items []T, toMap func(T) map[string]any) []map[string]any {
	out := make([]map[string]any, len(items))
	for i, item := range items {
		out[i] = toMap(item)
	}
	return out
}

func showAll(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	if asJSON {
		return showAllJSON(w, store, includeDeleted)
	}
	return showAllText(w, store, includeDeleted)
}

func showAllText(w io.Writer, store *data.Store, includeDeleted bool) error {
	if err := showHouse(w, store, false); err != nil {
		return err
	}
	showFns := []func(io.Writer, *data.Store, bool, bool) error{
		showProjects, showVendors, showAppliances, showIncidents,
		showQuotes, showMaintenance, showServiceLog, showDocuments,
		showProjectTypes, showMaintenanceCategories,
	}
	for _, fn := range showFns {
		if err := fn(w, store, false, includeDeleted); err != nil {
			return err
		}
	}
	return nil
}

func showAllJSON(w io.Writer, store *data.Store, includeDeleted bool) error {
	result := make(map[string]any)

	h, err := store.HouseProfile()
	if err == nil {
		result["house"] = h
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load house profile: %w", err)
	}

	projects, err := store.ListProjects(includeDeleted)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	_, projToMap := withDeletedCol(projectCols, projectToMap, includeDeleted,
		func(p data.Project) gorm.DeletedAt { return p.DeletedAt })
	result[entityProjects] = mapSlice(projects, projToMap)

	ptypes, err := store.ProjectTypes()
	if err != nil {
		return fmt.Errorf("list project types: %w", err)
	}
	result["project_types"] = mapSlice(ptypes, projectTypeToMap)

	vendors, err := store.ListVendors(includeDeleted)
	if err != nil {
		return fmt.Errorf("list vendors: %w", err)
	}
	_, vendToMap := withDeletedCol(vendorCols, vendorToMap, includeDeleted,
		func(v data.Vendor) gorm.DeletedAt { return v.DeletedAt })
	result[entityVendors] = mapSlice(vendors, vendToMap)

	quotes, err := store.ListQuotes(includeDeleted)
	if err != nil {
		return fmt.Errorf("list quotes: %w", err)
	}
	_, quoteMap := withDeletedCol(quoteCols, quoteToMap, includeDeleted,
		func(q data.Quote) gorm.DeletedAt { return q.DeletedAt })
	result[entityQuotes] = mapSlice(quotes, quoteMap)

	maintenance, err := store.ListMaintenance(includeDeleted)
	if err != nil {
		return fmt.Errorf("list maintenance: %w", err)
	}
	_, maintMap := withDeletedCol(maintenanceCols, maintenanceToMap, includeDeleted,
		func(m data.MaintenanceItem) gorm.DeletedAt { return m.DeletedAt })
	result[entityMaintenance] = mapSlice(maintenance, maintMap)

	mcats, err := store.MaintenanceCategories()
	if err != nil {
		return fmt.Errorf("list maintenance categories: %w", err)
	}
	result["maintenance_categories"] = mapSlice(mcats, maintenanceCategoryToMap)

	svcLog, err := store.ListAllServiceLogEntries(includeDeleted)
	if err != nil {
		return fmt.Errorf("list service log: %w", err)
	}
	_, svcMap := withDeletedCol(serviceLogCols, serviceLogToMap, includeDeleted,
		func(e data.ServiceLogEntry) gorm.DeletedAt { return e.DeletedAt })
	result["service_log"] = mapSlice(svcLog, svcMap)

	appliances, err := store.ListAppliances(includeDeleted)
	if err != nil {
		return fmt.Errorf("list appliances: %w", err)
	}
	_, appMap := withDeletedCol(applianceCols, applianceToMap, includeDeleted,
		func(a data.Appliance) gorm.DeletedAt { return a.DeletedAt })
	result[entityAppliances] = mapSlice(appliances, appMap)

	incidents, err := store.ListIncidents(includeDeleted)
	if err != nil {
		return fmt.Errorf("list incidents: %w", err)
	}
	_, incMap := withDeletedCol(incidentCols, incidentToMap, includeDeleted,
		func(i data.Incident) gorm.DeletedAt { return i.DeletedAt })
	result[entityIncidents] = mapSlice(incidents, incMap)

	documents, err := store.ListDocuments(includeDeleted)
	if err != nil {
		return fmt.Errorf("list documents: %w", err)
	}
	_, docMap := withDeletedCol(documentCols, documentToMap, includeDeleted,
		func(d data.Document) gorm.DeletedAt { return d.DeletedAt })
	result[entityDocuments] = mapSlice(documents, docMap)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}
