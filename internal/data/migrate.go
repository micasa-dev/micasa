// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"sort"
	"strings"

	"github.com/micasa-dev/micasa/internal/uid"

	"gorm.io/gorm"
)

// migrateIntToStringIDs detects databases created before v2.3 (which used
// integer auto-increment primary keys) and converts all IDs and foreign
// key references to ULID strings. This must run before GORM's AutoMigrate
// so that the schema matches the new string-typed model definitions.
//
// Strategy: rename old tables → let AutoMigrate create fresh tables with
// the correct TEXT-based schema → copy data with converted IDs → drop the
// renamed originals. This avoids any DDL parsing issues in the GORM
// migrator.
func migrateIntToStringIDs(db *gorm.DB) (retErr error) {
	needed, err := needsIntToStringMigration(db)
	if err != nil || !needed {
		return err
	}

	if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}
	defer func() {
		if fkErr := db.Exec("PRAGMA foreign_keys = ON").Error; fkErr != nil && retErr == nil {
			retErr = fmt.Errorf("re-enable foreign keys: %w", fkErr)
		}
	}()

	// Phase 1: Build ID mappings for every table.
	idMaps := make(map[string]map[string]string)
	for _, m := range migrationOrder() {
		exists, err := tableExists(db, m.table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		mapping, err := buildIDMapping(db, m.table)
		if err != nil {
			return fmt.Errorf("build ID mapping for %s: %w", m.table, err)
		}
		idMaps[m.table] = mapping
	}

	// Phase 2: Rename old tables so AutoMigrate creates fresh ones.
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, m := range migrationOrder() {
			if _, ok := idMaps[m.table]; !ok {
				continue
			}
			if err := renameTable(tx, m.table, oldName(m.table)); err != nil {
				return err
			}
			// Drop indexes that reference the old table name, since they'd
			// conflict with indexes GORM creates on the new table.
			if err := dropTableIndexes(tx, oldName(m.table)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("rename old tables: %w", err)
	}

	// Phase 3: Let AutoMigrate create fresh tables. If this fails, rename
	// the old tables back so the database isn't left in a broken state.
	if err := db.AutoMigrate(Models()...); err != nil {
		_ = rollbackRenames(db, idMaps)
		return fmt.Errorf("auto-migrate fresh tables: %w", err)
	}

	// Phase 4: Copy data from old tables to new tables with ID conversion.
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, m := range migrationOrder() {
			mapping := idMaps[m.table]
			if mapping == nil {
				continue
			}
			if err := copyWithConvertedIDs(tx, m, idMaps); err != nil {
				return fmt.Errorf("copy %s: %w", m.table, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	// Phase 5: Drop old tables.
	if err := db.Transaction(func(tx *gorm.DB) error {
		order := migrationOrder()
		for i := len(order) - 1; i >= 0; i-- {
			m := order[i]
			if _, ok := idMaps[m.table]; !ok {
				continue
			}
			if err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", oldName(m.table))).Error; err != nil {
				return fmt.Errorf("drop %s: %w", oldName(m.table), err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("drop old tables: %w", err)
	}

	return nil
}

// rollbackRenames restores renamed tables to their original names if
// AutoMigrate fails between Phase 2 and Phase 4.
func rollbackRenames(db *gorm.DB, idMaps map[string]map[string]string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Drop any partially-created new tables, then rename old back.
		for _, m := range migrationOrder() {
			if _, ok := idMaps[m.table]; !ok {
				continue
			}
			_ = tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", m.table)).Error
			if err := renameTable(tx, oldName(m.table), m.table); err != nil {
				return err
			}
		}
		return nil
	})
}

func oldName(table string) string { return "_old_" + table }

// needsIntToStringMigration returns true if the database has the old
// integer-PK schema.
func needsIntToStringMigration(db *gorm.DB) (bool, error) {
	var count int
	if err := db.Raw(
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		TableProjectTypes,
	).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("check table existence: %w", err)
	}
	if count == 0 {
		return false, nil
	}

	return hasIntegerPK(db, TableProjectTypes)
}

// hasIntegerPK returns true if the table's id column is declared as INTEGER.
func hasIntegerPK(db *gorm.DB, table string) (bool, error) {
	type columnInfo struct {
		CID       int
		Name      string
		Type      string
		NotNull   int
		DfltValue *string
		PK        int
	}
	var cols []columnInfo
	if err := db.Raw("PRAGMA table_info(`" + table + "`)").Scan(&cols).Error; err != nil {
		return false, fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	for _, col := range cols {
		if col.Name == "id" {
			return strings.EqualFold(col.Type, "integer"), nil
		}
	}
	return false, fmt.Errorf("id column not found in %s", table)
}

// tableMigration describes how to migrate one table's IDs.
type tableMigration struct {
	table string
	// fkColumns maps FK column name → parent table name.
	fkColumns map[string]string
}

// migrationOrder returns the tables in topological order (parents first).
func migrationOrder() []tableMigration {
	return []tableMigration{
		{table: TableHouseProfiles},
		{table: TableProjectTypes},
		{table: TableMaintenanceCategories},
		{table: TableVendors},
		{table: TableAppliances},
		{table: TableProjects, fkColumns: map[string]string{
			ColProjectTypeID: TableProjectTypes,
		}},
		{table: TableMaintenanceItems, fkColumns: map[string]string{
			ColCategoryID:  TableMaintenanceCategories,
			ColApplianceID: TableAppliances,
		}},
		{table: TableQuotes, fkColumns: map[string]string{
			ColProjectID: TableProjects,
			ColVendorID:  TableVendors,
		}},
		{table: TableIncidents, fkColumns: map[string]string{
			ColApplianceID: TableAppliances,
			ColVendorID:    TableVendors,
		}},
		{table: TableServiceLogEntries, fkColumns: map[string]string{
			ColMaintenanceItemID: TableMaintenanceItems,
			ColVendorID:          TableVendors,
		}},
		{table: TableDocuments, fkColumns: map[string]string{
			// entity_id is polymorphic — handled specially in copyWithConvertedIDs
		}},
		{table: TableDeletionRecords, fkColumns: map[string]string{
			// target_id is polymorphic — handled specially in copyWithConvertedIDs
		}},
	}
}

// copyWithConvertedIDs copies rows from the renamed old table into the
// fresh table, converting integer IDs and FK references to ULIDs.
func copyWithConvertedIDs(
	tx *gorm.DB,
	m tableMigration,
	idMaps map[string]map[string]string,
) error {
	mapping := idMaps[m.table]

	// Get shared columns between old and new tables.
	oldCols, err := getColumnNames(tx, oldName(m.table))
	if err != nil {
		return err
	}
	newCols, err := getColumnNames(tx, m.table)
	if err != nil {
		return err
	}
	// Use only columns present in both.
	cols := intersectColumns(oldCols, newCols)

	selectExprs := make([]string, len(cols))
	for i, col := range cols {
		selectExprs[i] = buildSelectExpr(col, m, mapping, idMaps)
	}

	insertSQL := fmt.Sprintf(
		"INSERT INTO `%s` (%s) SELECT %s FROM `%s`",
		m.table,
		joinQuoted(cols),
		strings.Join(selectExprs, ", "),
		oldName(m.table),
	)
	return tx.Exec(insertSQL).Error
}

// buildSelectExpr builds a SQL expression for a column during the
// INSERT...SELECT. IDs and FK columns get CASE expressions to map old
// integer values to new ULIDs.
func buildSelectExpr(
	col string,
	m tableMigration,
	mapping map[string]string,
	idMaps map[string]map[string]string,
) string {
	if col == "id" && len(mapping) > 0 {
		return buildCaseExpr(col, mapping)
	}

	if parentTable, ok := m.fkColumns[col]; ok {
		parentMap := idMaps[parentTable]
		if len(parentMap) > 0 {
			return buildCaseExpr(col, parentMap)
		}
	}

	// Documents: entity_id is polymorphic — build a nested CASE over
	// entity_kind to pick the right parent mapping.
	if m.table == TableDocuments && col == ColEntityID {
		return buildPolymorphicCaseExpr(col, ColEntityKind, documentKindToTable(), idMaps)
	}

	// DeletionRecords: target_id references any entity.
	if m.table == TableDeletionRecords && col == ColTargetID {
		return buildPolymorphicCaseExpr(col, ColEntity, deletionEntityToTable, idMaps)
	}

	return quoteCol(col)
}

// buildCaseExpr builds a SQL CASE that maps old integer values to new ULIDs.
func buildCaseExpr(col string, mapping map[string]string) string {
	keys := sortedKeys(mapping)
	var b strings.Builder
	fmt.Fprintf(&b, "CASE CAST(`%s` AS TEXT)", col)
	for _, oldID := range keys {
		fmt.Fprintf(&b, " WHEN '%s' THEN '%s'", oldID, mapping[oldID])
	}
	fmt.Fprintf(&b, " ELSE `%s` END", col)
	return b.String()
}

// buildPolymorphicCaseExpr builds a SQL CASE that branches on a type
// discriminator column to pick the right ID mapping.
func buildPolymorphicCaseExpr(
	idCol, kindCol string,
	kindToTable map[string]string,
	idMaps map[string]map[string]string,
) string {
	// For each entity kind, embed a sub-CASE over the ID values.
	var parts []string
	for _, kind := range sortedKeys(kindToTable) {
		table := kindToTable[kind]
		parentMap := idMaps[table]
		if len(parentMap) == 0 {
			continue
		}
		parentKeys := sortedKeys(parentMap)
		var sub strings.Builder
		fmt.Fprintf(&sub, "WHEN `%s` = '%s' THEN (CASE CAST(`%s` AS TEXT)", kindCol, kind, idCol)
		for _, oldID := range parentKeys {
			fmt.Fprintf(&sub, " WHEN '%s' THEN '%s'", oldID, parentMap[oldID])
		}
		fmt.Fprintf(&sub, " ELSE `%s` END)", idCol)
		parts = append(parts, sub.String())
	}

	if len(parts) == 0 {
		return quoteCol(idCol)
	}

	return fmt.Sprintf("CASE %s ELSE `%s` END", strings.Join(parts, " "), idCol)
}

func documentKindToTable() map[string]string {
	return map[string]string{
		DocumentEntityProject:     TableProjects,
		DocumentEntityQuote:       TableQuotes,
		DocumentEntityMaintenance: TableMaintenanceItems,
		DocumentEntityAppliance:   TableAppliances,
		DocumentEntityServiceLog:  TableServiceLogEntries,
		DocumentEntityVendor:      TableVendors,
		DocumentEntityIncident:    TableIncidents,
	}
}

// buildIDMapping reads all existing IDs from a table and creates a
// mapping from old (stringified integer) ID to new ULID.
func buildIDMapping(db *gorm.DB, table string) (map[string]string, error) {
	var ids []string
	if err := db.Raw(
		fmt.Sprintf("SELECT CAST(`id` AS TEXT) FROM `%s`", table),
	).Scan(&ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil //nolint:nilnil // empty result is not an error
	}
	mapping := make(map[string]string, len(ids))
	for _, oldID := range ids {
		if oldID == "" {
			continue
		}
		if uid.IsValid(oldID) {
			continue
		}
		mapping[oldID] = uid.New()
	}
	if len(mapping) == 0 {
		return nil, nil //nolint:nilnil // no duplicates found is not an error
	}
	return mapping, nil
}

func renameTable(db *gorm.DB, from, to string) error {
	return db.Exec(fmt.Sprintf("ALTER TABLE `%s` RENAME TO `%s`", from, to)).Error
}

// dropTableIndexes drops all non-autoindex indexes on a table.
func dropTableIndexes(db *gorm.DB, table string) error {
	var indexes []string
	if err := db.Raw(
		"SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name NOT LIKE 'sqlite_%'",
		table,
	).Scan(&indexes).Error; err != nil {
		return err
	}
	for _, idx := range indexes {
		if err := db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS `%s`", idx)).Error; err != nil {
			return fmt.Errorf("drop index %s: %w", idx, err)
		}
	}
	return nil
}

func getColumnNames(db *gorm.DB, table string) ([]string, error) {
	type columnInfo struct {
		CID       int
		Name      string
		Type      string
		NotNull   int
		DfltValue *string
		PK        int
	}
	var cols []columnInfo
	if err := db.Raw("PRAGMA table_info(`" + table + "`)").Scan(&cols).Error; err != nil {
		return nil, fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	return names, nil
}

func intersectColumns(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, c := range b {
		set[c] = true
	}
	var result []string
	for _, c := range a {
		if set[c] {
			result = append(result, c)
		}
	}
	return result
}

func tableExists(db *gorm.DB, table string) (bool, error) {
	var count int
	if err := db.Raw(
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		table,
	).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func quoteCol(name string) string {
	return "`" + name + "`"
}

func joinQuoted(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteCol(c)
	}
	return strings.Join(quoted, ", ")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
