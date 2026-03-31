// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/data/sqlite"
	"github.com/micasa-dev/micasa/internal/safeconv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ShadowDB stages LLM extraction operations in an in-memory SQLite database
// so that cross-references between batch-created entities (e.g. a quote
// referencing a just-created vendor) resolve correctly via ordinal tracking.
//
// The shadow DB has FK constraints OFF -- it is a staging area, not a
// validator. Validation happens during commit against the real DB.
// Each staged entity receives a sequential ordinal string ID (matching what
// the LLM outputs for batch cross-references) so cross-references within a
// batch can be remapped to the real ULIDs assigned during commit.
type ShadowDB struct {
	db *gorm.DB
	// created tracks shadow entries per table in insertion order.
	created map[string][]shadowEntry
	// nextOrdinal tracks the next sequential ID per table for shadow inserts.
	nextOrdinal map[string]int
}

// shadowEntry pairs a shadow ordinal string ID with the original operation
// data so synthetic fields (e.g. vendor_name) survive the staging round-trip.
type shadowEntry struct {
	shadowID string
	opData   map[string]any
}

// shadowFKRemap describes a foreign key column and the table it references,
// used during commit to rewrite shadow IDs to real IDs.
type shadowFKRemap struct {
	Column string // FK column name (e.g. "vendor_id")
	Table  string // referenced table (e.g. "vendors")
}

// NewShadowDB creates an in-memory SQLite database and migrates the
// extraction-relevant tables. Ordinal counters are seeded from the real
// DB's row counts so shadow ordinals occupy a disjoint range from
// existing real IDs, matching the sequential numbering the LLM uses for
// batch cross-references. FK constraints are OFF -- the shadow DB is a
// staging area; validation happens during commit.
func NewShadowDB(store *data.Store) (*ShadowDB, error) {
	db, err := gorm.Open(
		sqlite.Open(":memory:"),
		&gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("open shadow db: %w", err)
	}

	if err := db.AutoMigrate(data.Models()...); err != nil {
		return nil, fmt.Errorf("shadow db migrate: %w", err)
	}

	// Seed ordinal counters from the real DB's row counts so shadow
	// ordinals start after the existing entities. This ensures the LLM's
	// sequential cross-references (max_count+1, max_count+2, ...) map
	// unambiguously to batch-created entities.
	nextOrd := make(map[string]int)
	counts, err := store.RowCounts(creatableFKs.order...)
	if err != nil {
		return nil, fmt.Errorf("query row counts: %w", err)
	}
	for table, count := range counts {
		nextOrd[table] = count + 1
	}

	return &ShadowDB{
		db:          db,
		created:     make(map[string][]shadowEntry),
		nextOrdinal: nextOrd,
	}, nil
}

// Close closes the underlying in-memory database connection.
func (s *ShadowDB) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

// Stage inserts all operations into the shadow database in order.
// Create operations are inserted into the appropriate shadow table;
// update operations are recorded but not applied to the shadow DB
// (they target real DB rows and are handled during commit).
func (s *ShadowDB) Stage(ops []Operation) error {
	for i, op := range ops {
		if err := s.stageOne(op); err != nil {
			return fmt.Errorf("operation %d (%s %s): %w", i, op.Action, op.Table, err)
		}
	}
	return nil
}

// stageOne dispatches a single operation to the appropriate handler.
func (s *ShadowDB) stageOne(op Operation) error {
	switch op.Action {
	case ActionCreate:
		return s.stageCreate(op)
	case ActionUpdate:
		return nil
	default:
		return fmt.Errorf("unsupported action %q", op.Action)
	}
}

// stageCreate inserts a create operation into the shadow table with a
// sequential ordinal string ID for cross-reference tracking.
func (s *ShadowDB) stageCreate(op Operation) error {
	if err := validateTable(op.Table); err != nil {
		return err
	}

	ordinal := s.nextOrdinal[op.Table]
	if ordinal == 0 {
		ordinal = 1
	}
	s.nextOrdinal[op.Table] = ordinal + 1
	shadowID := strconv.Itoa(ordinal)

	cols, vals, placeholders, err := buildInsert(op.Table, op.Data)
	if err != nil {
		return err
	}

	if len(cols) == 0 {
		return fmt.Errorf("no columns to insert")
	}

	// Prepend the ordinal string as the id column.
	cols = append([]string{quoteIdent(data.ColID)}, cols...)
	vals = append([]any{shadowID}, vals...)
	placeholders = append([]string{"?"}, placeholders...)

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		quoteIdent(op.Table),
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	result := s.db.Exec(sql, vals...)
	if result.Error != nil {
		return result.Error
	}

	s.created[op.Table] = append(s.created[op.Table], shadowEntry{
		shadowID: shadowID,
		opData:   op.Data,
	})
	return nil
}

// buildInsert extracts column names, values, and placeholders from operation
// data for a raw SQL INSERT. Skips "id" (shadow DB auto-assigns) and
// "vendor_name" (synthetic field, not a real column). Each column name is
// validated against the allowed schema and double-quoted for defense-in-depth.
func buildInsert(
	table string,
	opData map[string]any,
) (cols []string, vals []any, placeholders []string, err error) {
	skip := map[string]bool{data.ColID: true, "vendor_name": true}

	for _, k := range sortedKeys(opData) {
		if skip[k] {
			continue
		}
		if err := validateColumn(table, k); err != nil {
			return nil, nil, nil, err
		}
		cols = append(cols, quoteIdent(k))
		vals = append(vals, normalizeValue(opData[k]))
		placeholders = append(placeholders, "?")
	}
	return cols, vals, placeholders, nil
}

// CreatedIDs returns the shadow ordinal string IDs for a given table in
// insertion order.
func (s *ShadowDB) CreatedIDs(table string) []string {
	entries := s.created[table]
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.shadowID
	}
	return ids
}

// Commit copies staged shadow rows to the real database inside a single
// transaction, remapping shadow auto-increment IDs to real IDs. If any
// operation fails the entire batch is rolled back. Tables are processed
// in dependency order; updates are applied after all creates.
func (s *ShadowDB) Commit(store *data.Store, ops []Operation) error {
	return store.Transaction(func(tx *data.Store) error {
		return s.commitInner(tx, ops)
	})
}

func (s *ShadowDB) commitInner(store *data.Store, ops []Operation) error {
	idMap := make(map[string]map[string]string) // table -> shadowID -> realID

	for _, table := range creatableFKs.order {
		entries := s.created[table]
		if len(entries) == 0 {
			continue
		}

		if idMap[table] == nil {
			idMap[table] = make(map[string]string)
		}

		for _, entry := range entries {
			row, err := s.readShadowRow(table, entry.shadowID)
			if err != nil {
				return fmt.Errorf("read shadow %s %s: %w", table, entry.shadowID, err)
			}

			// Remap FK columns from shadow IDs to real IDs.
			for _, fk := range creatableFKs.remaps[table] {
				remapFK(row, fk, idMap)
			}

			// Remap document entity_id if entity_kind maps to a creatable.
			if table == data.TableDocuments {
				remapDocumentEntity(row, idMap)
			}

			realID, err := commitRow(store, table, row, entry.opData)
			if err != nil {
				return fmt.Errorf("commit %s (shadow %s): %w", table, entry.shadowID, err)
			}

			idMap[table][entry.shadowID] = realID
		}
	}

	// Process updates (these target real DB rows directly).
	for _, op := range ops {
		if op.Action != ActionUpdate {
			continue
		}
		if err := commitUpdate(store, op); err != nil {
			return fmt.Errorf("update %s: %w", op.Table, err)
		}
	}

	return nil
}

// readShadowRow reads a single row from a shadow table by ID.
func (s *ShadowDB) readShadowRow(table string, id string) (map[string]any, error) {
	var result map[string]any
	if err := s.db.Table(table).Where("id = ?", id).Take(&result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

// remapFK rewrites a foreign key column value from a shadow ID to the
// corresponding real ID, if a mapping exists. Values without a mapping
// are left unchanged (they reference existing real DB entities).
func remapFK(row map[string]any, fk shadowFKRemap, idMap map[string]map[string]string) {
	v, ok := row[fk.Column]
	if !ok || v == nil {
		return
	}
	shadowID := ParseStringID(v)
	if shadowID == "" {
		return
	}
	if tableMap, ok := idMap[fk.Table]; ok {
		if realID, ok := tableMap[shadowID]; ok {
			row[fk.Column] = realID
		}
	}
}

// remapDocumentEntity rewrites entity_id on a document row if entity_kind
// maps to a creatable table whose IDs were remapped.
func remapDocumentEntity(row map[string]any, idMap map[string]map[string]string) {
	kind, _ := row[data.ColEntityKind].(string)
	if kind == "" {
		// GORM may return []byte for text columns from raw queries.
		if b, ok := row[data.ColEntityKind].([]byte); ok {
			kind = string(b)
		}
	}
	table, ok := creatableFKs.entityKindToTable[kind]
	if !ok {
		return
	}
	entityID := ParseStringID(row[data.ColEntityID])
	if entityID == "" {
		return
	}
	if tableMap, ok := idMap[table]; ok {
		if realID, ok := tableMap[entityID]; ok {
			row[data.ColEntityID] = realID
		}
	}
}

// commitRow creates a single entity in the real DB from shadow row data.
// opData carries the original operation data for synthetic fields (e.g.
// vendor_name) that aren't stored in the shadow table.
// Returns the real ULID assigned by the database.
func commitRow(
	store *data.Store,
	table string,
	row map[string]any,
	opData map[string]any,
) (string, error) {
	switch table {
	case data.TableVendors:
		return commitVendor(store, row)
	case data.TableAppliances:
		return commitAppliance(store, row)
	case data.TableProjects:
		return commitProject(store, row)
	case data.TableQuotes:
		return commitQuote(store, row, opData)
	case data.TableMaintenanceItems:
		return commitMaintenance(store, row)
	case data.TableIncidents:
		return commitIncident(store, row, opData)
	case data.TableServiceLogEntries:
		return commitServiceLog(store, row, opData)
	case data.TableDocuments:
		return commitDocument(store, row)
	default:
		return "", fmt.Errorf("unsupported table %q", table)
	}
}

func commitVendor(store *data.Store, row map[string]any) (string, error) {
	v := data.Vendor{}
	stringField(row, data.ColName, &v.Name)
	stringField(row, data.ColContactName, &v.ContactName)
	stringField(row, data.ColEmail, &v.Email)
	stringField(row, data.ColPhone, &v.Phone)
	stringField(row, data.ColWebsite, &v.Website)
	stringField(row, data.ColNotes, &v.Notes)
	if strings.TrimSpace(v.Name) == "" {
		return "", fmt.Errorf("vendor name is required")
	}
	found, err := store.FindOrCreateVendor(v)
	if err != nil {
		return "", err
	}
	return found.ID, nil
}

func commitAppliance(store *data.Store, row map[string]any) (string, error) {
	a := data.Appliance{}
	stringField(row, data.ColName, &a.Name)
	stringField(row, data.ColNotes, &a.Notes)
	stringField(row, data.ColLocation, &a.Location)
	stringField(row, data.ColBrand, &a.Brand)
	stringField(row, data.ColModelNumber, &a.ModelNumber)
	stringField(row, data.ColSerialNumber, &a.SerialNumber)
	if v := toInt64Ptr(row[data.ColCostCents]); v != nil {
		a.CostCents = v
	}
	found, err := store.FindOrCreateAppliance(a)
	if err != nil {
		return "", err
	}
	return found.ID, nil
}

func commitProject(store *data.Store, row map[string]any) (string, error) {
	p := data.Project{}
	stringField(row, data.ColTitle, &p.Title)
	stringField(row, data.ColDescription, &p.Description)
	stringField(row, data.ColStatus, &p.Status)
	p.ProjectTypeID = ParseStringID(row[data.ColProjectTypeID])
	if v := toInt64Ptr(row[data.ColBudgetCents]); v != nil {
		p.BudgetCents = v
	}
	if strings.TrimSpace(p.Title) == "" {
		return "", fmt.Errorf("project title is required")
	}
	if p.Status == "" {
		p.Status = data.ProjectStatusIdeating
	}
	if err := store.CreateProject(&p); err != nil {
		return "", err
	}
	return p.ID, nil
}

func commitQuote(store *data.Store, row map[string]any, opData map[string]any) (string, error) {
	q := data.Quote{}
	q.ProjectID = ParseStringID(row[data.ColProjectID])
	if q.ProjectID == "" {
		return "", fmt.Errorf("quote requires a project_id referencing an existing project")
	}
	q.TotalCents = ParseInt64(row[data.ColTotalCents])
	stringField(row, data.ColNotes, &q.Notes)

	if v := toInt64Ptr(row[data.ColLaborCents]); v != nil {
		q.LaborCents = v
	}
	if v := toInt64Ptr(row[data.ColMaterialsCents]); v != nil {
		q.MaterialsCents = v
	}

	// Resolve vendor. After remapping, vendor_id is either:
	// - A real ID (batch-created vendor already committed, or existing vendor)
	// - "" / absent (no vendor reference)
	var vendor data.Vendor
	vendorID := ParseStringID(row[data.ColVendorID])
	if vendorID != "" {
		got, err := store.GetVendor(vendorID)
		if err == nil {
			vendor = got
		}
	}
	// Fall back to vendor_name from original operation data (synthetic field
	// not stored in the shadow table).
	if vendor.ID == "" {
		stringField(opData, "vendor_name", &vendor.Name)
	}

	if err := store.CreateQuote(&q, vendor); err != nil {
		return "", err
	}
	return q.ID, nil
}

func commitMaintenance(store *data.Store, row map[string]any) (string, error) {
	m := data.MaintenanceItem{}
	stringField(row, data.ColName, &m.Name)
	stringField(row, data.ColNotes, &m.Notes)
	m.CategoryID = ParseStringID(row[data.ColCategoryID])
	if v := ParseStringID(row[data.ColApplianceID]); v != "" {
		m.ApplianceID = &v
	}
	if v := ParseInt64(row[data.ColIntervalMonths]); v != 0 {
		n, err := safeconv.Int(v)
		if err != nil {
			return "", fmt.Errorf("interval_months: %w", err)
		}
		m.IntervalMonths = n
	}
	if v := toInt64Ptr(row[data.ColCostCents]); v != nil {
		m.CostCents = v
	}
	found, err := store.FindOrCreateMaintenance(m)
	if err != nil {
		return "", err
	}
	return found.ID, nil
}

func commitIncident(store *data.Store, row map[string]any, opData map[string]any) (string, error) {
	inc := data.Incident{}
	stringField(row, data.ColTitle, &inc.Title)
	stringField(row, data.ColDescription, &inc.Description)
	stringField(row, data.ColStatus, &inc.Status)
	stringField(row, data.ColSeverity, &inc.Severity)
	stringField(row, data.ColLocation, &inc.Location)
	stringField(row, data.ColNotes, &inc.Notes)
	if v := toInt64Ptr(row[data.ColCostCents]); v != nil {
		inc.CostCents = v
	}
	if v := ParseStringID(row[data.ColApplianceID]); v != "" {
		inc.ApplianceID = &v
	}
	if v := ParseStringID(row[data.ColVendorID]); v != "" {
		inc.VendorID = &v
	}
	if inc.VendorID == nil {
		var vendorName string
		stringField(opData, "vendor_name", &vendorName)
		if strings.TrimSpace(vendorName) != "" {
			v := data.Vendor{Name: vendorName}
			found, err := store.FindOrCreateVendor(v)
			if err != nil {
				return "", fmt.Errorf("find-or-create vendor for incident: %w", err)
			}
			inc.VendorID = &found.ID
		}
	}
	if inc.Status == "" {
		inc.Status = data.IncidentStatusOpen
	}
	if inc.Severity == "" {
		inc.Severity = data.IncidentSeverityWhenever
	}
	inc.DateNoticed = parseDateOrNow(opData, data.ColDateNoticed)
	if strings.TrimSpace(inc.Title) == "" {
		return "", fmt.Errorf("incident title is required")
	}
	if err := store.CreateIncident(&inc); err != nil {
		return "", err
	}
	return inc.ID, nil
}

func commitServiceLog(
	store *data.Store,
	row map[string]any,
	opData map[string]any,
) (string, error) {
	entry := data.ServiceLogEntry{}
	entry.MaintenanceItemID = ParseStringID(row[data.ColMaintenanceItemID])
	stringField(row, data.ColNotes, &entry.Notes)
	if v := toInt64Ptr(row[data.ColCostCents]); v != nil {
		entry.CostCents = v
	}
	entry.ServicedAt = parseDateOrNow(opData, data.ColServicedAt)

	var vendor data.Vendor
	vendorID := ParseStringID(row[data.ColVendorID])
	if vendorID != "" {
		got, err := store.GetVendor(vendorID)
		if err == nil {
			vendor = got
		}
	}
	if vendor.ID == "" {
		stringField(opData, "vendor_name", &vendor.Name)
	}

	if err := store.CreateServiceLog(&entry, vendor); err != nil {
		return "", err
	}
	return entry.ID, nil
}

func commitDocument(store *data.Store, row map[string]any) (string, error) {
	doc := data.Document{}
	stringField(row, data.ColTitle, &doc.Title)
	stringField(row, data.ColFileName, &doc.FileName)
	stringField(row, data.ColNotes, &doc.Notes)
	stringField(row, data.ColEntityKind, &doc.EntityKind)
	doc.EntityID = ParseStringID(row[data.ColEntityID])
	if err := store.CreateDocument(&doc); err != nil {
		return "", err
	}
	return doc.ID, nil
}

// commitUpdate applies an update operation directly to the real DB.
func commitUpdate(store *data.Store, op Operation) error {
	switch op.Table {
	case data.TableVendors:
		return commitUpdateVendor(store, op)
	case data.TableAppliances:
		return commitUpdateAppliance(store, op)
	case data.TableQuotes:
		return commitUpdateQuote(store, op)
	case data.TableMaintenanceItems:
		return commitUpdateMaintenance(store, op)
	case data.TableDocuments:
		return commitUpdateDocument(store, op)
	default:
		return fmt.Errorf("update not supported on %q", op.Table)
	}
}

func commitUpdateDocument(store *data.Store, op Operation) error {
	rowID := ParseStringID(op.Data[data.ColID])
	if rowID == "" {
		return fmt.Errorf("update documents requires id in data")
	}
	doc, err := store.GetDocumentMetadata(rowID)
	if err != nil {
		return fmt.Errorf("get document %s: %w", rowID, err)
	}
	stringField(op.Data, data.ColTitle, &doc.Title)
	stringField(op.Data, data.ColNotes, &doc.Notes)
	stringField(op.Data, data.ColEntityKind, &doc.EntityKind)
	if v, ok := op.Data[data.ColEntityID]; ok {
		if n := ParseStringID(v); n != "" {
			doc.EntityID = n
		}
	}
	return store.UpdateDocument(doc)
}

func commitUpdateMaintenance(store *data.Store, op Operation) error {
	rowID := ParseStringID(op.Data[data.ColID])
	if rowID == "" {
		return fmt.Errorf("update maintenance_items requires id in data")
	}
	item, err := store.GetMaintenance(rowID)
	if err != nil {
		return fmt.Errorf("get maintenance_item %s: %w", rowID, err)
	}
	stringField(op.Data, data.ColName, &item.Name)
	stringField(op.Data, data.ColNotes, &item.Notes)
	if v, ok := op.Data[data.ColCategoryID]; ok {
		if n := ParseStringID(v); n != "" {
			item.CategoryID = n
		}
	}
	if v, ok := op.Data[data.ColApplianceID]; ok {
		if n := ParseStringID(v); n != "" {
			item.ApplianceID = &n
		}
	}
	if v, ok := op.Data[data.ColIntervalMonths]; ok {
		if raw := ParseInt64(v); raw > 0 {
			n, err := safeconv.Int(raw)
			if err != nil {
				return fmt.Errorf("interval_months: %w", err)
			}
			item.IntervalMonths = n
		}
	}
	if v, ok := op.Data[data.ColCostCents]; ok {
		n := ParseInt64(v)
		item.CostCents = &n
	}
	return store.UpdateMaintenance(item)
}

func commitUpdateVendor(store *data.Store, op Operation) error {
	rowID := ParseStringID(op.Data[data.ColID])
	if rowID == "" {
		return fmt.Errorf("update vendors requires id in data")
	}
	v, err := store.GetVendor(rowID)
	if err != nil {
		return fmt.Errorf("get vendor %s: %w", rowID, err)
	}
	stringField(op.Data, data.ColName, &v.Name)
	stringField(op.Data, data.ColContactName, &v.ContactName)
	stringField(op.Data, data.ColEmail, &v.Email)
	stringField(op.Data, data.ColPhone, &v.Phone)
	stringField(op.Data, data.ColWebsite, &v.Website)
	stringField(op.Data, data.ColNotes, &v.Notes)
	return store.UpdateVendor(v)
}

func commitUpdateAppliance(store *data.Store, op Operation) error {
	rowID := ParseStringID(op.Data[data.ColID])
	if rowID == "" {
		return fmt.Errorf("update appliances requires id in data")
	}
	a, err := store.GetAppliance(rowID)
	if err != nil {
		return fmt.Errorf("get appliance %s: %w", rowID, err)
	}
	stringField(op.Data, data.ColName, &a.Name)
	stringField(op.Data, data.ColBrand, &a.Brand)
	stringField(op.Data, data.ColModelNumber, &a.ModelNumber)
	stringField(op.Data, data.ColSerialNumber, &a.SerialNumber)
	stringField(op.Data, data.ColLocation, &a.Location)
	stringField(op.Data, data.ColNotes, &a.Notes)
	if v, ok := op.Data[data.ColCostCents]; ok {
		n := ParseInt64(v)
		a.CostCents = &n
	}
	return store.UpdateAppliance(a)
}

func commitUpdateQuote(store *data.Store, op Operation) error {
	rowID := ParseStringID(op.Data[data.ColID])
	if rowID == "" {
		return fmt.Errorf("update quotes requires id in data")
	}
	q, err := store.GetQuote(rowID)
	if err != nil {
		return fmt.Errorf("get quote %s: %w", rowID, err)
	}
	stringField(op.Data, data.ColNotes, &q.Notes)
	if v, ok := op.Data[data.ColTotalCents]; ok {
		q.TotalCents = ParseInt64(v)
	}
	if v, ok := op.Data[data.ColLaborCents]; ok {
		n := ParseInt64(v)
		q.LaborCents = &n
	}
	if v, ok := op.Data[data.ColMaterialsCents]; ok {
		n := ParseInt64(v)
		q.MaterialsCents = &n
	}
	if v, ok := op.Data[data.ColProjectID]; ok {
		if n := ParseStringID(v); n != "" {
			q.ProjectID = n
		}
	}

	var vendor data.Vendor
	if v, ok := op.Data[data.ColVendorID]; ok {
		if n := ParseStringID(v); n != "" {
			got, getErr := store.GetVendor(n)
			if getErr == nil {
				vendor = got
			}
		}
	}
	if vendor.ID == "" {
		stringField(op.Data, "vendor_name", &vendor.Name)
	}
	if vendor.ID == "" && vendor.Name == "" {
		vendor = q.Vendor
	}
	return store.UpdateQuote(q, vendor)
}

// --- identifier validation ---

// allowedColumns maps each allowed table to its set of valid column names,
// derived from ExtractionOps at init time.
var allowedColumns = func() map[string]map[string]bool {
	m := make(map[string]map[string]bool)
	for _, op := range ExtractionOps {
		if m[op.Table] == nil {
			m[op.Table] = make(map[string]bool)
		}
		for _, col := range op.Columns {
			m[op.Table][col.Name] = true
		}
	}
	return m
}()

// validateTable checks that the table name is in the allowed set and
// contains only safe characters.
func validateTable(table string) error {
	if !data.IsSafeIdentifier(table) {
		return fmt.Errorf("invalid table name: %q", table)
	}
	if _, ok := ExtractionAllowedOps[table]; !ok {
		return fmt.Errorf("table %q is not in the allowed set", table)
	}
	return nil
}

// validateColumn checks that the column name contains only safe characters
// and exists in the target table's schema.
func validateColumn(table, col string) error {
	if !data.IsSafeIdentifier(col) {
		return fmt.Errorf("invalid column name: %q", col)
	}
	allowed, ok := allowedColumns[table]
	if !ok {
		return fmt.Errorf("no column schema for table %q", table)
	}
	if !allowed[col] {
		return fmt.Errorf("column %q not allowed on table %q", col, table)
	}
	return nil
}

// quoteIdent wraps a SQL identifier in double-quotes (SQLite's standard
// identifier quoting mechanism) as defense-in-depth after validation.
func quoteIdent(name string) string {
	return `"` + name + `"`
}

// --- helpers ---

// stringField sets *dst to the string value at row[key] if present.
// Handles both string and []byte (GORM returns []byte for text columns
// from raw map queries in SQLite).
func stringField(row map[string]any, key string, dst *string) {
	v, ok := row[key]
	if !ok || v == nil {
		return
	}
	switch s := v.(type) {
	case string:
		*dst = s
	case []byte:
		*dst = string(s)
	}
}

// parseDateOrNow extracts a date from row[key] and returns it. The value
// may be a time.Time (from GORM datetime columns), a string, or []byte.
// Returns time.Now() truncated to midnight if missing, empty, or unparsable.
func parseDateOrNow(row map[string]any, key string) time.Time {
	v, ok := row[key]
	if !ok || v == nil {
		return time.Now().Truncate(24 * time.Hour)
	}
	if t, ok := v.(time.Time); ok {
		return t
	}
	var s string
	switch val := v.(type) {
	case string:
		s = val
	case []byte:
		s = string(val)
	}
	if t, err := data.ParseOptionalDate(s); err == nil && t != nil {
		return *t
	}
	return time.Now().Truncate(24 * time.Hour)
}

// ParseStringID extracts a string ID from an arbitrary value. Handles string,
// []byte (from GORM/SQLite map queries), json.Number, and numeric types.
// Returns "" for nil or empty values.
func ParseStringID(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case []byte:
		return strings.TrimSpace(string(val))
	case float64:
		if val != math.Trunc(val) || math.IsInf(val, 0) {
			return ""
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(val, 10)
	case int:
		return strconv.Itoa(val)
	case uint:
		return strconv.FormatUint(uint64(val), 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	default:
		if s, ok := v.(interface{ String() string }); ok {
			return strings.TrimSpace(s.String())
		}
	}
	return ""
}

// ParseInt64 extracts an int64 from an arbitrary value. Handles concrete
// numeric types (from GORM/SQLite map queries), json.Number, and string
// representations. Returns 0 for nil or unparsable values.
func ParseInt64(v any) int64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case uint:
		if val > math.MaxInt64 {
			return 0
		}
		return int64(val)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		return n
	default:
		if s, ok := v.(interface{ String() string }); ok {
			n, _ := strconv.ParseInt(strings.TrimSpace(s.String()), 10, 64)
			return n
		}
	}
	return 0
}

// toInt64Ptr returns a pointer to the int64 value, or nil if the value is
// nil or zero. Used for optional *int64 model fields.
func toInt64Ptr(v any) *int64 {
	if v == nil {
		return nil
	}
	n := ParseInt64(v)
	if n == 0 {
		return nil
	}
	return &n
}

// normalizeValue converts json.Number values to concrete Go types so SQLite
// receives typed values rather than opaque strings.
func normalizeValue(v any) any {
	n, ok := v.(interface{ String() string })
	if !ok {
		return v
	}
	s := n.String()
	if strings.ContainsRune(s, '.') {
		return s
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	return s
}
