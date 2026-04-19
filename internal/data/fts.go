// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// FTS5 virtual table and trigger names.
const (
	tableFTS         = "documents_fts"
	triggerFTSInsert = "documents_fts_ai"
	triggerFTSDelete = "documents_fts_ad"
	triggerFTSUpdate = "documents_fts_au"

	tableEntitiesFTS = "entities_fts"
)

// DocumentSearchResult holds a single FTS5 match with metadata for display.
type DocumentSearchResult struct {
	ID         string
	Title      string
	FileName   string
	EntityKind string
	EntityID   string
	Snippet    string
	UpdatedAt  time.Time
}

// setupFTS creates the FTS5 virtual table and sync triggers if they do not
// already exist, then rebuilds the index to catch any documents that were
// created before FTS was added.
func (s *Store) setupFTS() error {
	// Create the external-content FTS5 virtual table. Porter stemmer
	// enables "plumbing" matching "plumber"; unicode61 handles case
	// folding and diacritics.
	createTable := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(
			title,
			notes,
			extracted_text,
			content=%s,
			content_rowid=rowid,
			tokenize='porter unicode61'
		)`, tableFTS, TableDocuments)
	if err := s.db.Exec(createTable).Error; err != nil {
		return fmt.Errorf("create FTS table: %w", err)
	}

	// Install triggers to keep the FTS index in sync with the documents
	// table. Triggers are dropped and recreated on every Open so that
	// definition changes (e.g., soft-delete awareness) apply to existing DBs.
	triggers := []struct {
		name string
		sql  string
	}{
		{
			name: triggerFTSInsert,
			sql: fmt.Sprintf(`
				CREATE TRIGGER %s AFTER INSERT ON %s BEGIN
					INSERT INTO %s(rowid, title, notes, extracted_text)
					SELECT new.rowid, new.title, new.notes, new.extracted_text
					WHERE new.deleted_at IS NULL;
				END`, triggerFTSInsert, TableDocuments, tableFTS),
		},
		{
			name: triggerFTSDelete,
			sql: fmt.Sprintf(`
				CREATE TRIGGER %s AFTER DELETE ON %s BEGIN
					INSERT INTO %s(%s, rowid, title, notes, extracted_text)
					VALUES ('delete', old.rowid, old.title, old.notes, old.extracted_text);
				END`, triggerFTSDelete, TableDocuments, tableFTS, tableFTS),
		},
		{
			name: triggerFTSUpdate,
			sql: fmt.Sprintf(`
				CREATE TRIGGER %s AFTER UPDATE ON %s BEGIN
					-- Remove old FTS entry only when it was previously indexed.
					INSERT INTO %s(%s, rowid, title, notes, extracted_text)
					SELECT 'delete', old.rowid, old.title, old.notes, old.extracted_text
					WHERE old.deleted_at IS NULL;
					-- Re-index only when the row is alive (not soft-deleted).
					INSERT INTO %s(rowid, title, notes, extracted_text)
					SELECT new.rowid, new.title, new.notes, new.extracted_text
					WHERE new.deleted_at IS NULL;
				END`, triggerFTSUpdate, TableDocuments, tableFTS, tableFTS, tableFTS),
		},
	}
	for _, t := range triggers {
		// Drop first so updated trigger definitions take effect on existing DBs.
		drop := "DROP TRIGGER IF EXISTS " + t.name
		if err := s.db.Exec(drop).Error; err != nil {
			return fmt.Errorf("drop trigger %s: %w", t.name, err)
		}
		if err := s.db.Exec(t.sql).Error; err != nil {
			return fmt.Errorf("create trigger %s: %w", t.name, err)
		}
	}

	// Rebuild to index any documents created before FTS was set up.
	rebuild := fmt.Sprintf(
		`INSERT INTO %s(%s) VALUES('rebuild')`, tableFTS, tableFTS,
	)
	if err := s.db.Exec(rebuild).Error; err != nil {
		return fmt.Errorf("rebuild FTS index: %w", err)
	}

	return s.setupEntitiesFTS()
}

// SearchDocuments performs a full-text search across document titles, notes,
// and extracted text. Returns results ranked by BM25 relevance with text
// snippets showing matched context. Only non-deleted documents are returned.
func (s *Store) SearchDocuments(query string) ([]DocumentSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	var results []DocumentSearchResult
	err := s.db.Raw(fmt.Sprintf(`
		SELECT
			d.id,
			d.title,
			d.file_name,
			d.entity_kind,
			d.entity_id,
			snippet(%s, -1, '>>>', '<<<', '...', 32) AS snippet,
			d.updated_at
		FROM %s
		JOIN %s d ON d.rowid = %s.rowid
		WHERE %s MATCH ?
			AND d.deleted_at IS NULL
		ORDER BY rank
		LIMIT 50
	`, tableFTS, tableFTS, TableDocuments, tableFTS, tableFTS), prepareFTSQuery(query)).
		Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("search documents: %w", err)
	}
	return results, nil
}

// prepareFTSQuery transforms a user query into a syntactically valid
// FTS5 MATCH expression using the canonical phrase-wrap escape from
// the FTS5 author: each whitespace-separated token becomes a quoted
// phrase (with internal " doubled) suffixed with * for prefix matching,
// and the phrases are implicitly ANDed.
//
// FTS5 operators in user input (AND/OR/NOT/parens) are treated as
// literal text, not operators -- the search box is type-as-you-go and
// partial operator syntax mid-keystroke would otherwise error.
//
// See https://sqlite.org/forum/info/82344cab7c5806980b287ce008975c6585d510e95ac7199de398ff9051ae0907
func prepareFTSQuery(query string) string {
	fields := strings.Fields(query)
	out := make([]string, len(fields))
	for i, w := range fields {
		out[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"*`
	}
	return strings.Join(out, " ")
}

// RebuildFTSIndex forces a full rebuild of the FTS5 index. Useful after
// bulk imports or data recovery.
func (s *Store) RebuildFTSIndex() error {
	rebuild := fmt.Sprintf(
		`INSERT INTO %s(%s) VALUES('rebuild')`, tableFTS, tableFTS,
	)
	return s.db.Exec(rebuild).Error
}

// hasFTSTable checks whether the FTS virtual table exists.
func (s *Store) hasFTSTable() bool {
	var count int64
	s.db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		tableFTS,
	).Scan(&count)
	return count > 0
}

// setupEntitiesFTS drops and recreates the entities_fts table, then populates
// it from all entity source tables. Called from setupFTS on every app open.
func (s *Store) setupEntitiesFTS() error {
	if err := s.db.Exec("DROP TABLE IF EXISTS " + tableEntitiesFTS).Error; err != nil {
		return fmt.Errorf("drop entities FTS table: %w", err)
	}

	createTable := fmt.Sprintf(`
		CREATE VIRTUAL TABLE %s USING fts5(
			entity_type UNINDEXED,
			entity_id UNINDEXED,
			entity_name,
			entity_text,
			tokenize='porter unicode61'
		)`, tableEntitiesFTS)
	if err := s.db.Exec(createTable).Error; err != nil {
		return fmt.Errorf("create entities FTS table: %w", err)
	}

	return s.populateEntitiesFTS()
}

// populateEntitiesFTS inserts rows from each entity source table into the
// entities_fts index. Only non-deleted rows are indexed.
func (s *Store) populateEntitiesFTS() error {
	inserts := []struct {
		label string
		sql   string
	}{
		{
			"projects",
			fmt.Sprintf(`INSERT INTO %s (entity_type, entity_id, entity_name, entity_text)
				SELECT '%s', %s, %s,
					%s || ' ' || COALESCE(%s, '') || ' ' || COALESCE(%s, '')
				FROM %s WHERE %s IS NULL`,
				tableEntitiesFTS,
				DeletionEntityProject, ColID, ColTitle,
				ColTitle, ColDescription, ColStatus,
				TableProjects, ColDeletedAt),
		},
		{
			"vendors",
			fmt.Sprintf(`INSERT INTO %s (entity_type, entity_id, entity_name, entity_text)
				SELECT '%s', %s, %s,
					%s || ' ' || COALESCE(%s, '') || ' ' || COALESCE(%s, '')
				FROM %s WHERE %s IS NULL`,
				tableEntitiesFTS,
				DeletionEntityVendor, ColID, ColName,
				ColName, ColContactName, ColNotes,
				TableVendors, ColDeletedAt),
		},
		{
			"appliances",
			fmt.Sprintf(`INSERT INTO %s (entity_type, entity_id, entity_name, entity_text)
				SELECT '%s', %s, %s,
					%s || ' ' || COALESCE(%s, '') || ' ' || COALESCE(%s, '') || ' ' || COALESCE(%s, '') || ' ' || COALESCE(%s, '')
				FROM %s WHERE %s IS NULL`,
				tableEntitiesFTS,
				DeletionEntityAppliance, ColID, ColName,
				ColName, ColBrand, ColModelNumber, ColLocation, ColNotes,
				TableAppliances, ColDeletedAt),
		},
		{
			"maintenance_items",
			fmt.Sprintf(`INSERT INTO %s (entity_type, entity_id, entity_name, entity_text)
				SELECT '%s', %s, %s,
					%s || ' ' || COALESCE(%s, '') || ' ' || COALESCE(%s, '')
				FROM %s WHERE %s IS NULL`,
				tableEntitiesFTS,
				DeletionEntityMaintenance, ColID, ColName,
				ColName, ColNotes, ColSeason,
				TableMaintenanceItems, ColDeletedAt),
		},
		{
			"incidents",
			fmt.Sprintf(`INSERT INTO %s (entity_type, entity_id, entity_name, entity_text)
				SELECT '%s', %s, %s,
					%s || ' ' || COALESCE(%s, '') || ' ' || COALESCE(%s, '') || ' ' || COALESCE(%s, '') || ' ' || COALESCE(%s, '')
				FROM %s WHERE %s IS NULL`,
				tableEntitiesFTS,
				DeletionEntityIncident, ColID, ColTitle,
				ColTitle, ColDescription, ColLocation, ColNotes, ColSeverity,
				TableIncidents, ColDeletedAt),
		},
		{
			"service_log_entries",
			fmt.Sprintf(`INSERT INTO %s (entity_type, entity_id, entity_name, entity_text)
				SELECT '%s', s.%s, COALESCE(m.%s, ''), COALESCE(s.%s, '')
				FROM %s s
				LEFT JOIN %s m ON s.%s = m.%s
				WHERE s.%s IS NULL`,
				tableEntitiesFTS,
				DeletionEntityServiceLog, ColID, ColName, ColNotes,
				TableServiceLogEntries,
				TableMaintenanceItems, ColMaintenanceItemID, ColID,
				ColDeletedAt),
		},
		{
			"quotes",
			fmt.Sprintf(`INSERT INTO %s (entity_type, entity_id, entity_name, entity_text)
				SELECT '%s', q.%s,
					COALESCE(p.%s, '') || ' - ' || COALESCE(v.%s, ''),
					COALESCE(q.%s, '')
				FROM %s q
				LEFT JOIN %s p ON q.%s = p.%s
				LEFT JOIN %s v ON q.%s = v.%s
				WHERE q.%s IS NULL`,
				tableEntitiesFTS,
				DeletionEntityQuote, ColID,
				ColTitle, ColName,
				ColNotes,
				TableQuotes,
				TableProjects, ColProjectID, ColID,
				TableVendors, ColVendorID, ColID,
				ColDeletedAt),
		},
	}

	for _, ins := range inserts {
		if err := s.db.Exec(ins.sql).Error; err != nil {
			return fmt.Errorf("populate entities FTS (%s): %w", ins.label, err)
		}
	}
	return nil
}

// hasEntitiesFTSTable checks whether the entities FTS virtual table exists.
func (s *Store) hasEntitiesFTSTable() bool {
	var count int64
	s.db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		tableEntitiesFTS,
	).Scan(&count)
	return count > 0
}

// EntitySearchResult holds a single FTS5 match from the entities index.
type EntitySearchResult struct {
	EntityType string  `gorm:"column:entity_type"`
	EntityID   string  `gorm:"column:entity_id"`
	EntityName string  `gorm:"column:entity_name"`
	Rank       float64 `gorm:"column:rank"`
}

// SearchEntities performs a full-text search across all indexed entity types.
// Returns up to 20 results ranked by BM25 relevance. Empty query strings and
// a missing entities_fts table both short-circuit to nil, nil; any other
// query-time error is wrapped and returned to the caller.
func (s *Store) SearchEntities(query string) ([]EntitySearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	if !s.hasEntitiesFTSTable() {
		return nil, nil
	}

	safeQuery := prepareFTSQuery(query)

	var results []EntitySearchResult
	err := s.db.Raw(fmt.Sprintf(`
		SELECT entity_type, entity_id, entity_name, rank
		FROM %s
		WHERE %s MATCH ?
		ORDER BY rank, entity_type, entity_id
		LIMIT 20
	`, tableEntitiesFTS, tableEntitiesFTS), safeQuery).
		Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("search entities: %w", err)
	}
	return results, nil
}

// EntitySummary fetches a live one-line summary for an entity, revalidating
// against the source table. Returns (summary, found, error). When the entity
// has been deleted since the FTS index was built, found is false.
func (s *Store) EntitySummary(entityType, entityID string) (string, bool, error) {
	switch entityType {
	case DeletionEntityProject:
		return s.projectSummary(entityID)
	case DeletionEntityVendor:
		return s.vendorSummary(entityID)
	case DeletionEntityAppliance:
		return s.applianceSummary(entityID)
	case DeletionEntityMaintenance:
		return s.maintenanceSummary(entityID)
	case DeletionEntityIncident:
		return s.incidentSummary(entityID)
	case DeletionEntityServiceLog:
		return s.serviceLogSummary(entityID)
	case DeletionEntityQuote:
		return s.quoteSummary(entityID)
	default:
		return "", false, fmt.Errorf("unknown entity type: %s", entityType)
	}
}

func (s *Store) projectSummary(id string) (string, bool, error) {
	var p Project
	err := s.db.Preload("ProjectType").First(&p, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("project summary: %w", err)
	}

	var details []string
	details = append(details, "status="+p.Status)
	if p.ProjectType.Name != "" {
		details = append(details, "type="+p.ProjectType.Name)
	}
	if p.BudgetCents != nil {
		details = append(details, fmt.Sprintf("budget=$%.2f", float64(*p.BudgetCents)/100))
	}
	if p.ActualCents != nil {
		details = append(details, fmt.Sprintf("actual=$%.2f", float64(*p.ActualCents)/100))
	}
	if p.Description != "" {
		details = append(details, "description="+truncateField(p.Description))
	}

	return fmt.Sprintf(
		"Project %q (id: %s): %s",
		p.Title,
		p.ID,
		strings.Join(details, ", "),
	), true, nil
}

func (s *Store) vendorSummary(id string) (string, bool, error) {
	var v Vendor
	err := s.db.First(&v, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("vendor summary: %w", err)
	}

	var details []string
	if v.ContactName != "" {
		details = append(details, "contact="+v.ContactName)
	}
	if v.Phone != "" {
		details = append(details, "phone="+v.Phone)
	}
	if v.Email != "" {
		details = append(details, "email="+v.Email)
	}
	if v.Notes != "" {
		details = append(details, "notes="+truncateField(v.Notes))
	}

	summary := fmt.Sprintf("Vendor %q (id: %s)", v.Name, v.ID)
	if len(details) > 0 {
		summary += ": " + strings.Join(details, ", ")
	}
	return summary, true, nil
}

func (s *Store) applianceSummary(id string) (string, bool, error) {
	var a Appliance
	err := s.db.First(&a, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("appliance summary: %w", err)
	}

	var details []string
	if a.Brand != "" {
		details = append(details, "brand="+a.Brand)
	}
	if a.ModelNumber != "" {
		details = append(details, "model="+a.ModelNumber)
	}
	if a.Location != "" {
		details = append(details, "location="+a.Location)
	}
	if a.Notes != "" {
		details = append(details, "notes="+truncateField(a.Notes))
	}

	summary := fmt.Sprintf("Appliance %q (id: %s)", a.Name, a.ID)
	if len(details) > 0 {
		summary += ": " + strings.Join(details, ", ")
	}
	return summary, true, nil
}

func (s *Store) maintenanceSummary(id string) (string, bool, error) {
	var m MaintenanceItem
	err := s.db.First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("maintenance summary: %w", err)
	}

	var details []string
	if m.Season != "" {
		details = append(details, "season="+m.Season)
	}
	if m.IntervalMonths > 0 {
		details = append(details, fmt.Sprintf("interval=%d months", m.IntervalMonths))
	}
	if m.LastServicedAt != nil {
		details = append(details, "last_serviced="+m.LastServicedAt.Format("2006-01-02"))
	}
	if m.Notes != "" {
		details = append(details, "notes="+truncateField(m.Notes))
	}

	summary := fmt.Sprintf("Maintenance %q (id: %s)", m.Name, m.ID)
	if len(details) > 0 {
		summary += ": " + strings.Join(details, ", ")
	}
	return summary, true, nil
}

func (s *Store) incidentSummary(id string) (string, bool, error) {
	var inc Incident
	err := s.db.First(&inc, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("incident summary: %w", err)
	}

	details := []string{
		"status=" + inc.Status,
		"severity=" + inc.Severity,
	}
	if inc.Location != "" {
		details = append(details, "location="+inc.Location)
	}
	if inc.Description != "" {
		details = append(details, "description="+truncateField(inc.Description))
	}

	return fmt.Sprintf(
		"Incident %q (id: %s): %s",
		inc.Title,
		inc.ID,
		strings.Join(details, ", "),
	), true, nil
}

func (s *Store) serviceLogSummary(id string) (string, bool, error) {
	var sle ServiceLogEntry
	err := s.db.Preload("MaintenanceItem").First(&sle, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("service log summary: %w", err)
	}

	name := sle.MaintenanceItem.Name
	if name == "" {
		name = "service log"
	}

	var details []string
	details = append(details, "serviced="+sle.ServicedAt.Format("2006-01-02"))
	if sle.CostCents != nil {
		details = append(details, fmt.Sprintf("cost=$%.2f", float64(*sle.CostCents)/100))
	}
	if sle.Notes != "" {
		details = append(details, "notes="+truncateField(sle.Notes))
	}

	return fmt.Sprintf(
		"ServiceLog %q (id: %s): %s",
		name,
		sle.ID,
		strings.Join(details, ", "),
	), true, nil
}

func (s *Store) quoteSummary(id string) (string, bool, error) {
	var q Quote
	err := s.db.Preload("Project").Preload("Vendor").First(&q, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("quote summary: %w", err)
	}

	name := q.Project.Title + " - " + q.Vendor.Name
	var details []string
	details = append(details, fmt.Sprintf("total=$%.2f", float64(q.TotalCents)/100))
	if q.Notes != "" {
		details = append(details, "notes="+truncateField(q.Notes))
	}

	return fmt.Sprintf("Quote %q (id: %s): %s", name, q.ID, strings.Join(details, ", ")), true, nil
}

// truncateField truncates a text field to maxFieldLen runes to limit
// prompt surface area. Uses rune count to avoid splitting multibyte characters.
func truncateField(s string) string {
	const maxFieldLen = 200
	runes := []rune(s)
	if len(runes) <= maxFieldLen {
		return s
	}
	return string(runes[:maxFieldLen]) + "..."
}
