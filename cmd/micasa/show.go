// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func fmtMoney(cents *int64) string {
	if cents == nil {
		return "-"
	}
	return fmt.Sprintf("$%.2f", float64(*cents)/100)
}

func fmtDate(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

func fmtInt(n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

func fmtIntAlways(n int) string {
	return fmt.Sprintf("%d", n)
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
	return t.Format("2006-01-02")
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

	cmd.AddCommand(
		newShowHouseCmd(&jsonFlag, &deletedFlag),
		newShowEntityCmd("projects", "Show projects", &jsonFlag, &deletedFlag, showProjects),
		newShowEntityCmd("vendors", "Show vendors", &jsonFlag, &deletedFlag, showVendors),
		newShowEntityCmd("appliances", "Show appliances", &jsonFlag, &deletedFlag, showAppliances),
		newShowEntityCmd("incidents", "Show incidents", &jsonFlag, &deletedFlag, showIncidents),
		newShowEntityCmd("quotes", "Show quotes", &jsonFlag, &deletedFlag, showQuotes),
		newShowEntityCmd(
			"maintenance",
			"Show maintenance items",
			&jsonFlag,
			&deletedFlag,
			showMaintenance,
		),
		newShowEntityCmd(
			"service-log",
			"Show service log entries",
			&jsonFlag,
			&deletedFlag,
			showServiceLog,
		),
		newShowEntityCmd("documents", "Show documents", &jsonFlag, &deletedFlag, showDocuments),
		newShowEntityCmd(
			"project-types",
			"Show project types",
			&jsonFlag,
			&deletedFlag,
			showProjectTypes,
		),
		newShowEntityCmd(
			"maintenance-categories",
			"Show maintenance categories",
			&jsonFlag,
			&deletedFlag,
			showMaintenanceCategories,
		),
	)

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

func newShowHouseCmd(jsonFlag, deletedFlag *bool) *cobra.Command {
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
			return runShow(cmd.OutOrStdout(), store, "house", *jsonFlag, *deletedFlag)
		},
	}
}

// validEntities lists entity names for error messages.
var validEntities = []string{
	"house", "projects", "project-types", "quotes", "vendors",
	"maintenance", "maintenance-categories", "service-log",
	"appliances", "incidents", "documents", "all",
}

func runShow(w io.Writer, store *data.Store, entity string, asJSON, includeDeleted bool) error {
	switch entity {
	case "house":
		return showHouse(w, store, asJSON)
	case "projects":
		return showProjects(w, store, asJSON, includeDeleted)
	case "vendors":
		return showVendors(w, store, asJSON, includeDeleted)
	case "appliances":
		return showAppliances(w, store, asJSON, includeDeleted)
	case "incidents":
		return showIncidents(w, store, asJSON, includeDeleted)
	case "quotes":
		return showQuotes(w, store, asJSON, includeDeleted)
	case "maintenance":
		return showMaintenance(w, store, asJSON, includeDeleted)
	case "service-log":
		return showServiceLog(w, store, asJSON, includeDeleted)
	case "documents":
		return showDocuments(w, store, asJSON, includeDeleted)
	case "project-types":
		return showProjectTypes(w, store, asJSON, includeDeleted)
	case "maintenance-categories":
		return showMaintenanceCategories(w, store, asJSON, includeDeleted)
	default:
		return fmt.Errorf("unknown entity %q; valid entities: %s",
			entity, strings.Join(validEntities, ", "))
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
	var lines []string
	if h.AddressLine1 != "" {
		lines = append(lines, h.AddressLine1)
	}
	if h.AddressLine2 != "" {
		lines = append(lines, h.AddressLine2)
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
		lines = append(lines, csStr)
	}
	return strings.Join(lines, "\n                   ")
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
		"id":           p.ID,
		"title":        p.Title,
		"project_type": p.ProjectType.Name,
		"status":       p.Status,
		"start_date":   p.StartDate,
		"end_date":     p.EndDate,
		"budget_cents": p.BudgetCents,
		"actual_cents": p.ActualCents,
		"description":  p.Description,
	}
}

func showProjects(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListProjects(includeDeleted)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	return showEntity(w, "PROJECTS", items, projectCols, projectToMap, asJSON)
}

// --- vendors ---

var vendorCols = []showCol[data.Vendor]{
	{"NAME", func(v data.Vendor) string { return fmtStr(v.Name) }},
	{"CONTACT", func(v data.Vendor) string { return fmtStr(v.ContactName) }},
	{"EMAIL", func(v data.Vendor) string { return fmtStr(v.Email) }},
	{"PHONE", func(v data.Vendor) string { return fmtStr(v.Phone) }},
	{"WEBSITE", func(v data.Vendor) string { return fmtStr(v.Website) }},
}

func vendorToMap(v data.Vendor) map[string]any {
	return map[string]any{
		"id":           v.ID,
		"name":         v.Name,
		"contact_name": v.ContactName,
		"email":        v.Email,
		"phone":        v.Phone,
		"website":      v.Website,
		"notes":        v.Notes,
	}
}

func showVendors(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListVendors(includeDeleted)
	if err != nil {
		return fmt.Errorf("list vendors: %w", err)
	}
	return showEntity(w, "VENDORS", items, vendorCols, vendorToMap, asJSON)
}

// --- appliances ---

var applianceCols = []showCol[data.Appliance]{
	{"NAME", func(a data.Appliance) string { return fmtStr(a.Name) }},
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
		"id":              a.ID,
		"name":            a.Name,
		"brand":           a.Brand,
		"model_number":    a.ModelNumber,
		"serial_number":   a.SerialNumber,
		"location":        a.Location,
		"purchase_date":   a.PurchaseDate,
		"warranty_expiry": a.WarrantyExpiry,
		"cost_cents":      a.CostCents,
		"notes":           a.Notes,
	}
}

func showAppliances(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListAppliances(includeDeleted)
	if err != nil {
		return fmt.Errorf("list appliances: %w", err)
	}
	return showEntity(w, "APPLIANCES", items, applianceCols, applianceToMap, asJSON)
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
		"id":            i.ID,
		"title":         i.Title,
		"status":        i.Status,
		"severity":      i.Severity,
		"date_noticed":  i.DateNoticed,
		"date_resolved": i.DateResolved,
		"location":      i.Location,
		"cost_cents":    i.CostCents,
		"appliance":     i.Appliance.Name,
		"vendor":        i.Vendor.Name,
		"description":   i.Description,
		"notes":         i.Notes,
	}
}

func showIncidents(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListIncidents(includeDeleted)
	if err != nil {
		return fmt.Errorf("list incidents: %w", err)
	}
	return showEntity(w, "INCIDENTS", items, incidentCols, incidentToMap, asJSON)
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
		"id":              q.ID,
		"project":         q.Project.Title,
		"vendor":          q.Vendor.Name,
		"total_cents":     q.TotalCents,
		"labor_cents":     q.LaborCents,
		"materials_cents": q.MaterialsCents,
		"received_date":   q.ReceivedDate,
		"notes":           q.Notes,
	}
}

func showQuotes(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListQuotes(includeDeleted)
	if err != nil {
		return fmt.Errorf("list quotes: %w", err)
	}
	return showEntity(w, "QUOTES", items, quoteCols, quoteToMap, asJSON)
}

// --- maintenance ---

var maintenanceCols = []showCol[data.MaintenanceItem]{
	{"NAME", func(m data.MaintenanceItem) string { return fmtStr(m.Name) }},
	{"CATEGORY", func(m data.MaintenanceItem) string { return fmtStr(m.Category.Name) }},
	{"APPLIANCE", func(m data.MaintenanceItem) string { return fmtStr(m.Appliance.Name) }},
	{"SEASON", func(m data.MaintenanceItem) string { return fmtStr(m.Season) }},
	{"LAST SERVICED", func(m data.MaintenanceItem) string { return fmtDate(m.LastServicedAt) }},
	{"INTERVAL", func(m data.MaintenanceItem) string { return fmtIntAlways(m.IntervalMonths) }},
	{"DUE", func(m data.MaintenanceItem) string { return fmtDate(m.DueDate) }},
	{"COST", func(m data.MaintenanceItem) string { return fmtMoney(m.CostCents) }},
}

func maintenanceToMap(m data.MaintenanceItem) map[string]any {
	return map[string]any{
		"id":               m.ID,
		"name":             m.Name,
		"category":         m.Category.Name,
		"appliance":        m.Appliance.Name,
		"season":           m.Season,
		"last_serviced_at": m.LastServicedAt,
		"interval_months":  m.IntervalMonths,
		"due_date":         m.DueDate,
		"cost_cents":       m.CostCents,
		"notes":            m.Notes,
	}
}

func showMaintenance(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListMaintenance(includeDeleted)
	if err != nil {
		return fmt.Errorf("list maintenance: %w", err)
	}
	return showEntity(w, "MAINTENANCE", items, maintenanceCols, maintenanceToMap, asJSON)
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
		"id":               e.ID,
		"maintenance_item": e.MaintenanceItem.Name,
		"vendor":           e.Vendor.Name,
		"serviced_at":      e.ServicedAt,
		"cost_cents":       e.CostCents,
		"notes":            e.Notes,
	}
}

func showServiceLog(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListAllServiceLogEntries(includeDeleted)
	if err != nil {
		return fmt.Errorf("list service log: %w", err)
	}
	return showEntity(w, "SERVICE LOG", items, serviceLogCols, serviceLogToMap, asJSON)
}

// --- documents ---

func fmtSize(n int64) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

var documentCols = []showCol[data.Document]{
	{"TITLE", func(d data.Document) string { return fmtStr(d.Title) }},
	{"FILE", func(d data.Document) string { return fmtStr(d.FileName) }},
	{"ENTITY", func(d data.Document) string { return fmtStr(d.EntityKind) }},
	{"MIME", func(d data.Document) string { return fmtStr(d.MIMEType) }},
	{"SIZE", func(d data.Document) string { return fmtSize(d.SizeBytes) }},
	{"NOTES", func(d data.Document) string { return fmtStr(d.Notes) }},
}

func documentToMap(d data.Document) map[string]any {
	return map[string]any{
		"id":          d.ID,
		"title":       d.Title,
		"file_name":   d.FileName,
		"entity_kind": d.EntityKind,
		"entity_id":   d.EntityID,
		"mime_type":   d.MIMEType,
		"size_bytes":  d.SizeBytes,
		"sha256":      d.ChecksumSHA256,
		"notes":       d.Notes,
	}
}

func showDocuments(w io.Writer, store *data.Store, asJSON, includeDeleted bool) error {
	items, err := store.ListDocuments(includeDeleted)
	if err != nil {
		return fmt.Errorf("list documents: %w", err)
	}
	return showEntity(w, "DOCUMENTS", items, documentCols, documentToMap, asJSON)
}

// --- project-types ---

var projectTypeCols = []showCol[data.ProjectType]{
	{"NAME", func(pt data.ProjectType) string { return fmtStr(pt.Name) }},
}

func projectTypeToMap(pt data.ProjectType) map[string]any {
	return map[string]any{
		"id":   pt.ID,
		"name": pt.Name,
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
	{"NAME", func(mc data.MaintenanceCategory) string { return fmtStr(mc.Name) }},
}

func maintenanceCategoryToMap(mc data.MaintenanceCategory) map[string]any {
	return map[string]any{
		"id":   mc.ID,
		"name": mc.Name,
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
