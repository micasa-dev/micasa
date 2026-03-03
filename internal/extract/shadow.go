// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/data/sqlite"
	"github.com/cpcloud/micasa/internal/safeconv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ShadowDB stages LLM extraction operations in an in-memory SQLite database
// so that cross-references between batch-created entities (e.g. a quote
// referencing a just-created vendor) resolve correctly via auto-increment IDs.
//
// The shadow DB has FK constraints OFF -- it is a staging area, not a
// validator. Validation happens during commit against the real DB.
// Auto-increment IDs are seeded from the real DB's max IDs so shadow IDs
// occupy a disjoint range (max_real_id+1, ...), eliminating ambiguity
// between references to existing entities and batch-created ones.
type ShadowDB struct {
	db *gorm.DB
	// created tracks shadow entries per table in insertion order.
	created map[string][]shadowEntry
}

// shadowEntry pairs a shadow auto-increment ID with the original operation
// data so synthetic fields (e.g. vendor_name) survive the staging round-trip.
type shadowEntry struct {
	shadowID uint
	opData   map[string]any
}

// shadowFKRemap describes a foreign key column and the table it references,
// used during commit to rewrite shadow IDs to real IDs.
type shadowFKRemap struct {
	Column string // FK column name (e.g. "vendor_id")
	Table  string // referenced table (e.g. "vendors")
}

// commitOrder defines the dependency-safe order for committing creatables.
// Tables with no FK dependencies on other creatables come first.
var commitOrder = []string{
	"vendors",
	"appliances",
	"quotes",
	"maintenance_items",
	"documents",
}

// fkRemaps maps each creatable table to its FK columns that reference other
// creatables. Only these need shadow->real remapping; FKs to reference-only
// tables (projects, categories) pass through unchanged.
var fkRemaps = map[string][]shadowFKRemap{
	"vendors":    {},
	"appliances": {},
	"quotes": {
		{Column: data.ColVendorID, Table: "vendors"},
	},
	"maintenance_items": {
		{Column: data.ColApplianceID, Table: "appliances"},
	},
	"documents": {},
}

// NewShadowDB creates an in-memory SQLite database and migrates the
// extraction-relevant tables. Auto-increment IDs are seeded from the real
// DB so shadow IDs occupy a disjoint range from existing real IDs, making
// cross-references unambiguous. FK constraints are OFF -- the shadow DB
// is a staging area; validation happens during commit.
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

	if err := db.AutoMigrate(
		&data.ProjectType{},
		&data.MaintenanceCategory{},
		&data.Project{},
		&data.Vendor{},
		&data.Appliance{},
		&data.Quote{},
		&data.MaintenanceItem{},
		&data.Document{},
	); err != nil {
		return nil, fmt.Errorf("shadow db migrate: %w", err)
	}

	// Seed sqlite_sequence so auto-increment starts after the real DB's
	// max IDs. This ensures shadow IDs never collide with real IDs.
	maxIDs, err := store.MaxIDs(commitOrder...)
	if err != nil {
		return nil, fmt.Errorf("query max ids: %w", err)
	}
	for table, maxID := range maxIDs {
		if err := db.Exec(
			"INSERT INTO sqlite_sequence (name, seq) VALUES (?, ?)",
			table, maxID,
		).Error; err != nil {
			return nil, fmt.Errorf("seed sqlite_sequence for %s: %w", table, err)
		}
	}

	return &ShadowDB{
		db:      db,
		created: make(map[string][]shadowEntry),
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

// stageCreate inserts a create operation into the shadow table and records
// the auto-increment ID assigned by the shadow DB.
func (s *ShadowDB) stageCreate(op Operation) error {
	cols, vals, placeholders := buildInsert(op.Data)
	if len(cols) == 0 {
		return fmt.Errorf("no columns to insert")
	}

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		op.Table,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	result := s.db.Exec(sql, vals...)
	if result.Error != nil {
		return result.Error
	}

	var lastID uint
	if err := s.db.Raw("SELECT last_insert_rowid()").Scan(&lastID).Error; err != nil {
		return fmt.Errorf("last_insert_rowid: %w", err)
	}

	s.created[op.Table] = append(s.created[op.Table], shadowEntry{
		shadowID: lastID,
		opData:   op.Data,
	})
	return nil
}

// buildInsert extracts column names, values, and placeholders from operation
// data for a raw SQL INSERT. Skips "id" (shadow DB auto-assigns) and
// "vendor_name" (synthetic field, not a real column).
func buildInsert(opData map[string]any) (cols []string, vals []any, placeholders []string) {
	skip := map[string]bool{"id": true, "vendor_name": true}

	for _, k := range sortedKeys(opData) {
		if skip[k] {
			continue
		}
		cols = append(cols, k)
		vals = append(vals, normalizeValue(opData[k]))
		placeholders = append(placeholders, "?")
	}
	return cols, vals, placeholders
}

// CreatedIDs returns the shadow auto-increment IDs for a given table
// in insertion order.
func (s *ShadowDB) CreatedIDs(table string) []uint {
	entries := s.created[table]
	ids := make([]uint, len(entries))
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
	idMap := make(map[string]map[uint]uint) // table -> shadowID -> realID

	for _, table := range commitOrder {
		entries := s.created[table]
		if len(entries) == 0 {
			continue
		}

		if idMap[table] == nil {
			idMap[table] = make(map[uint]uint)
		}

		for _, entry := range entries {
			row, err := s.readShadowRow(table, entry.shadowID)
			if err != nil {
				return fmt.Errorf("read shadow %s %d: %w", table, entry.shadowID, err)
			}

			// Remap FK columns from shadow IDs to real IDs.
			for _, fk := range fkRemaps[table] {
				remapFK(row, fk, idMap)
			}

			// Remap document entity_id if entity_kind maps to a creatable.
			if table == "documents" {
				remapDocumentEntity(row, idMap)
			}

			realID, err := commitRow(store, table, row, entry.opData)
			if err != nil {
				return fmt.Errorf("commit %s (shadow %d): %w", table, entry.shadowID, err)
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
func (s *ShadowDB) readShadowRow(table string, id uint) (map[string]any, error) {
	var result map[string]any
	if err := s.db.Table(table).Where("id = ?", id).Take(&result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

// remapFK rewrites a foreign key column value from a shadow ID to the
// corresponding real ID, if a mapping exists. Values without a mapping
// are left unchanged (they reference existing real DB entities).
func remapFK(row map[string]any, fk shadowFKRemap, idMap map[string]map[uint]uint) {
	v, ok := row[fk.Column]
	if !ok || v == nil {
		return
	}
	shadowID := toUint(v)
	if shadowID == 0 {
		return
	}
	if tableMap, ok := idMap[fk.Table]; ok {
		if realID, ok := tableMap[shadowID]; ok {
			row[fk.Column] = realID
		}
	}
}

// entityKindToTable maps document entity_kind values to their corresponding
// creatable table names for ID remapping.
var entityKindToTable = map[string]string{
	data.DocumentEntityVendor:      "vendors",
	data.DocumentEntityQuote:       "quotes",
	data.DocumentEntityMaintenance: "maintenance_items",
	data.DocumentEntityAppliance:   "appliances",
}

// remapDocumentEntity rewrites entity_id on a document row if entity_kind
// maps to a creatable table whose IDs were remapped.
func remapDocumentEntity(row map[string]any, idMap map[string]map[uint]uint) {
	kind, _ := row[data.ColEntityKind].(string)
	if kind == "" {
		// GORM may return []byte for text columns from raw queries.
		if b, ok := row[data.ColEntityKind].([]byte); ok {
			kind = string(b)
		}
	}
	table, ok := entityKindToTable[kind]
	if !ok {
		return
	}
	entityID := toUint(row[data.ColEntityID])
	if entityID == 0 {
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
// Returns the real auto-increment ID.
func commitRow(
	store *data.Store,
	table string,
	row map[string]any,
	opData map[string]any,
) (uint, error) {
	switch table {
	case "vendors":
		return commitVendor(store, row)
	case "appliances":
		return commitAppliance(store, row)
	case "quotes":
		return commitQuote(store, row, opData)
	case "maintenance_items":
		return commitMaintenance(store, row)
	case "documents":
		return commitDocument(store, row)
	default:
		return 0, fmt.Errorf("unsupported table %q", table)
	}
}

func commitVendor(store *data.Store, row map[string]any) (uint, error) {
	v := data.Vendor{}
	stringField(row, data.ColName, &v.Name)
	stringField(row, data.ColContactName, &v.ContactName)
	stringField(row, data.ColEmail, &v.Email)
	stringField(row, data.ColPhone, &v.Phone)
	stringField(row, data.ColWebsite, &v.Website)
	stringField(row, data.ColNotes, &v.Notes)
	if strings.TrimSpace(v.Name) == "" {
		return 0, fmt.Errorf("vendor name is required")
	}
	found, err := store.FindOrCreateVendor(v)
	if err != nil {
		return 0, err
	}
	return found.ID, nil
}

func commitAppliance(store *data.Store, row map[string]any) (uint, error) {
	a := data.Appliance{}
	stringField(row, data.ColName, &a.Name)
	stringField(row, data.ColNotes, &a.Notes)
	stringField(row, data.ColLocation, &a.Location)
	stringField(row, "brand", &a.Brand)
	stringField(row, "model_number", &a.ModelNumber)
	stringField(row, "serial_number", &a.SerialNumber)
	if v := toInt64Ptr(row[data.ColCostCents]); v != nil {
		a.CostCents = v
	}
	found, err := store.FindOrCreateAppliance(a)
	if err != nil {
		return 0, err
	}
	return found.ID, nil
}

func commitQuote(store *data.Store, row map[string]any, opData map[string]any) (uint, error) {
	q := data.Quote{}
	q.ProjectID = toUint(row[data.ColProjectID])
	q.TotalCents = toInt64(row[data.ColTotalCents])
	stringField(row, data.ColNotes, &q.Notes)

	if v := toInt64Ptr(row["labor_cents"]); v != nil {
		q.LaborCents = v
	}
	if v := toInt64Ptr(row["materials_cents"]); v != nil {
		q.MaterialsCents = v
	}

	// Resolve vendor. After remapping, vendor_id is either:
	// - A real ID (batch-created vendor already committed, or existing vendor)
	// - 0 / absent (no vendor reference)
	var vendor data.Vendor
	vendorID := toUint(row[data.ColVendorID])
	if vendorID > 0 {
		got, err := store.GetVendor(vendorID)
		if err == nil {
			vendor = got
		}
	}
	// Fall back to vendor_name from original operation data (synthetic field
	// not stored in the shadow table).
	if vendor.ID == 0 {
		applyString(opData, "vendor_name", &vendor.Name)
	}

	if err := store.CreateQuote(&q, vendor); err != nil {
		return 0, err
	}
	return q.ID, nil
}

func commitMaintenance(store *data.Store, row map[string]any) (uint, error) {
	m := data.MaintenanceItem{}
	stringField(row, data.ColName, &m.Name)
	stringField(row, data.ColNotes, &m.Notes)
	m.CategoryID = toUint(row["category_id"])
	if v := toUint(row[data.ColApplianceID]); v != 0 {
		m.ApplianceID = &v
	}
	if v := toInt64(row[data.ColIntervalMonths]); v != 0 {
		n, err := safeconv.Int(v)
		if err != nil {
			return 0, fmt.Errorf("interval_months: %w", err)
		}
		m.IntervalMonths = n
	}
	if v := toInt64Ptr(row[data.ColCostCents]); v != nil {
		m.CostCents = v
	}
	found, err := store.FindOrCreateMaintenance(m)
	if err != nil {
		return 0, err
	}
	return found.ID, nil
}

func commitDocument(store *data.Store, row map[string]any) (uint, error) {
	doc := data.Document{}
	stringField(row, data.ColTitle, &doc.Title)
	stringField(row, data.ColFileName, &doc.FileName)
	stringField(row, data.ColNotes, &doc.Notes)
	stringField(row, data.ColEntityKind, &doc.EntityKind)
	doc.EntityID = toUint(row[data.ColEntityID])
	if err := store.CreateDocument(&doc); err != nil {
		return 0, err
	}
	return doc.ID, nil
}

// commitUpdate applies an update operation directly to the real DB.
func commitUpdate(store *data.Store, op Operation) error {
	switch op.Table {
	case "documents":
		return commitUpdateDocument(store, op)
	case "maintenance_items":
		return commitUpdateMaintenance(store, op)
	default:
		return fmt.Errorf("update not supported on %q", op.Table)
	}
}

func commitUpdateDocument(store *data.Store, op Operation) error {
	rowID := ParseUint(op.Data["id"])
	if rowID == 0 {
		return fmt.Errorf("update documents requires id in data")
	}
	doc, err := store.GetDocument(rowID)
	if err != nil {
		return fmt.Errorf("get document %d: %w", rowID, err)
	}
	applyString(op.Data, "title", &doc.Title)
	applyString(op.Data, "notes", &doc.Notes)
	applyString(op.Data, "entity_kind", &doc.EntityKind)
	if v, ok := op.Data["entity_id"]; ok {
		if n := ParseUint(v); n > 0 {
			doc.EntityID = n
		}
	}
	return store.UpdateDocument(doc)
}

func commitUpdateMaintenance(store *data.Store, op Operation) error {
	rowID := ParseUint(op.Data["id"])
	if rowID == 0 {
		return fmt.Errorf("update maintenance_items requires id in data")
	}
	item, err := store.GetMaintenance(rowID)
	if err != nil {
		return fmt.Errorf("get maintenance_item %d: %w", rowID, err)
	}
	applyString(op.Data, "name", &item.Name)
	applyString(op.Data, "notes", &item.Notes)
	if v, ok := op.Data["category_id"]; ok {
		if n := ParseUint(v); n > 0 {
			item.CategoryID = n
		}
	}
	if v, ok := op.Data["appliance_id"]; ok {
		if n := ParseUint(v); n > 0 {
			item.ApplianceID = &n
		}
	}
	if v, ok := op.Data["interval_months"]; ok {
		if raw := ParseInt64(v); raw > 0 {
			n, err := safeconv.Int(raw)
			if err != nil {
				return fmt.Errorf("interval_months: %w", err)
			}
			item.IntervalMonths = n
		}
	}
	if v, ok := op.Data["cost_cents"]; ok {
		n := ParseInt64(v)
		item.CostCents = &n
	}
	return store.UpdateMaintenance(item)
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

// applyString sets *dst from a JSON operation data map value.
func applyString(d map[string]any, key string, dst *string) {
	if v, ok := d[key]; ok {
		if s, ok := v.(string); ok {
			*dst = s
		}
	}
}

// toUint extracts a uint from types returned by GORM/SQLite map queries.
func toUint(v any) uint {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case uint:
		return val
	case int64:
		if val > 0 {
			return uint(val)
		}
	case float64:
		if val > 0 {
			return uint(val)
		}
	case int:
		if val > 0 {
			return uint(val)
		}
	case uint64:
		return uint(val)
	default:
		return ParseUint(v)
	}
	return 0
}

// toInt64 extracts an int64 from types returned by GORM/SQLite map queries.
func toInt64(v any) int64 {
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
	default:
		return ParseInt64(v)
	}
}

// toInt64Ptr returns a pointer to the int64 value, or nil if the value is
// nil or zero. Used for optional *int64 model fields.
func toInt64Ptr(v any) *int64 {
	if v == nil {
		return nil
	}
	n := toInt64(v)
	if n == 0 {
		return nil
	}
	return &n
}

// ParseInt64 extracts an int64 from a JSON value (json.Number, float64,
// string). Returns 0 for nil or unparsable values.
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
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		return n
	}
	if s, ok := v.(interface{ String() string }); ok {
		n, _ := strconv.ParseInt(strings.TrimSpace(s.String()), 10, 64)
		return n
	}
	return 0
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
