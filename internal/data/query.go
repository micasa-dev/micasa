// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	maxQueryRows = 200

	// readOnlyQueryTimeout caps how long a user-submitted query can run
	// before being cancelled.
	readOnlyQueryTimeout = 10 * time.Second
)

// PragmaColumn mirrors the output of PRAGMA table_info.
type PragmaColumn struct {
	CID       int     `gorm:"column:cid"`
	Name      string  `gorm:"column:name"`
	Type      string  `gorm:"column:type"`
	NotNull   bool    `gorm:"column:notnull"`
	DfltValue *string `gorm:"column:dflt_value"`
	PK        int     `gorm:"column:pk"`
}

// TableNames returns the names of all non-internal tables in the database.
func (s *Store) TableNames() ([]string, error) {
	var names []string
	err := s.db.Raw(
		"SELECT name FROM sqlite_master WHERE type='table' " +
			"AND name NOT LIKE 'sqlite_%' ORDER BY name",
	).Scan(&names).Error
	return names, err
}

// TableColumns returns column metadata for the named table via PRAGMA.
// The table name is validated to contain only safe characters.
func (s *Store) TableColumns(table string) ([]PragmaColumn, error) {
	if !IsSafeIdentifier(table) {
		return nil, fmt.Errorf("invalid table name: %q", table)
	}
	var cols []PragmaColumn
	//nolint:gosec // table name validated by IsSafeIdentifier above
	err := s.db.Raw(fmt.Sprintf("PRAGMA table_info(%s)", table)).Scan(&cols).Error
	return cols, err
}

// writeOpcodes lists SQLite VDBE opcodes that indicate a write operation.
// If EXPLAIN output contains any of these, the query mutates state.
var writeOpcodes = map[string]bool{
	"OpenWrite":    true,
	"Delete":       true,
	"Insert":       true,
	"InsertInt":    true,
	"NewRowid":     true,
	"RowSetAdd":    true,
	"CreateBtree":  true,
	"SqlExec":      true,
	"Destroy":      true,
	"DropTable":    true,
	"DropIndex":    true,
	"DropTrigger":  true,
	"Clear":        true,
	"SetCookie":    true,
	"RealAffinity": false, // read-only, listed for documentation
}

// disallowedKeywords is the defense-in-depth keyword blocklist. It catches
// obviously dangerous statements before the more expensive EXPLAIN check.
var disallowedKeywords = []string{
	"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE",
	"ATTACH", "DETACH", "PRAGMA", "REINDEX", "VACUUM",
}

// ReadOnlyQuery executes a validated SELECT query and returns the results as
// string slices. Only SELECT/WITH statements are allowed; result rows are
// capped at maxQueryRows.
//
// Validation is layered for defense-in-depth:
//  1. Fast prefix check: query must start with SELECT or WITH (after stripping
//     whitespace and SQL comments).
//  2. Semicolon check: rejects multi-statement payloads.
//  3. Keyword blocklist (containsWord): catches obvious mutation keywords.
//  4. EXPLAIN opcode check: runs EXPLAIN on the query and rejects it if any
//     write-related VDBE opcodes appear. This is the primary structural
//     validation and catches attacks that bypass keyword matching.
//  5. Timeout: the actual query runs under a 10-second context deadline to
//     prevent long-running queries from hanging the app.
func (s *Store) ReadOnlyQuery(query string) (columns []string, rows [][]string, err error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, nil, fmt.Errorf("empty query")
	}

	// --- Layer 1: fast prefix check (comment-aware) ---
	stripped := stripLeadingComments(trimmed)
	upper := strings.ToUpper(stripped)
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return nil, nil, fmt.Errorf(
			"only SELECT queries are allowed: query starts with %q",
			firstWord(stripped),
		)
	}

	// --- Layer 2: reject multi-statement payloads ---
	if strings.Contains(trimmed, ";") {
		return nil, nil, fmt.Errorf("multiple statements are not allowed")
	}

	// --- Layer 3: keyword blocklist (defense-in-depth) ---
	upperFull := strings.ToUpper(trimmed)
	for _, kw := range disallowedKeywords {
		if containsWord(upperFull, kw) {
			return nil, nil, fmt.Errorf("query contains disallowed keyword: %s", kw)
		}
	}

	// --- Layer 4: EXPLAIN opcode validation ---
	if err := s.explainIsReadOnly(trimmed); err != nil {
		return nil, nil, err
	}

	// --- Layer 5: execute with timeout ---
	ctx, cancel := context.WithTimeout(context.Background(), readOnlyQueryTimeout)
	defer cancel()

	sqlRows, err := s.db.WithContext(ctx).Raw(trimmed).Rows()
	if err != nil {
		return nil, nil, fmt.Errorf("execute query: %w", err)
	}
	defer func() {
		_ = sqlRows.Close()
	}()

	columns, err = sqlRows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("get columns: %w", err)
	}

	for sqlRows.Next() {
		if len(rows) >= maxQueryRows {
			break
		}
		values := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := sqlRows.Scan(ptrs...); err != nil {
			return nil, nil, fmt.Errorf("scan row: %w", err)
		}
		row := make([]string, len(columns))
		for i, v := range values {
			if v == nil {
				row[i] = ""
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		rows = append(rows, row)
	}
	return columns, rows, sqlRows.Err()
}

// explainIsReadOnly runs EXPLAIN on the query and inspects the resulting VDBE
// program for write-related opcodes. Returns nil if the query is read-only, or
// an error describing which opcode was found.
func (s *Store) explainIsReadOnly(query string) error {
	explainRows, err := s.db.Raw("EXPLAIN " + query).Rows()
	if err != nil {
		return fmt.Errorf("EXPLAIN validation failed: %w", err)
	}
	defer func() { _ = explainRows.Close() }()

	cols, err := explainRows.Columns()
	if err != nil {
		return fmt.Errorf("EXPLAIN columns: %w", err)
	}

	// Find the "opcode" column index. EXPLAIN output columns are:
	// addr, opcode, p1, p2, p3, p4, p5, comment
	opcodeIdx := -1
	for i, c := range cols {
		if strings.ToLower(c) == "opcode" {
			opcodeIdx = i
			break
		}
	}
	if opcodeIdx < 0 {
		return fmt.Errorf("EXPLAIN output missing opcode column")
	}

	for explainRows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := explainRows.Scan(ptrs...); err != nil {
			return fmt.Errorf("scan EXPLAIN row: %w", err)
		}
		opcode := fmt.Sprintf("%v", values[opcodeIdx])
		if writeOpcodes[opcode] {
			return fmt.Errorf(
				"query rejected: EXPLAIN revealed write operation %q -- only read-only queries are allowed",
				opcode,
			)
		}
	}
	return explainRows.Err()
}

// stripLeadingComments removes leading SQL comments (both -- line comments
// and /* block comments */) and whitespace so the true first keyword can be
// checked. This prevents attackers from hiding a non-SELECT statement behind
// comment syntax.
func stripLeadingComments(s string) string {
	for {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "--") {
			if nl := strings.IndexByte(s, '\n'); nl >= 0 {
				s = s[nl+1:]
			} else {
				return ""
			}
		} else if strings.HasPrefix(s, "/*") {
			if end := strings.Index(s, "*/"); end >= 0 {
				s = s[end+2:]
			} else {
				return ""
			}
		} else {
			return s
		}
	}
}

// firstWord returns the first whitespace-delimited token in s, or s itself
// if there is no whitespace. Used for error messages.
func firstWord(s string) string {
	if i := strings.IndexFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}); i >= 0 {
		return s[:i]
	}
	return s
}

// DataDump exports every row of every user table as readable text, suitable
// for stuffing into an LLM context window. For a home-scale database this
// is small enough to fit comfortably.
//
// Unlike ReadOnlyQuery this bypasses the row cap and keyword filter -- table
// names come from sqlite_master so the queries are fully trusted.
//
// The output is optimized for small LLMs: null/empty values are omitted,
// money columns (ending in "_ct") are formatted as dollars, and internal
// columns (id, created_at, updated_at, deleted_at) are excluded to reduce
// noise.
func (s *Store) DataDump() string {
	names, err := s.TableNames()
	if err != nil {
		return ""
	}

	var b strings.Builder
	for _, name := range names {
		// Skip sync infrastructure tables from LLM context dumps.
		if name == TableSyncOplogEntries || name == TableSyncDevices {
			continue
		}
		rows, cols, err := dumpTable(s, name)
		if err != nil {
			continue
		}

		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s (%d rows)\n\n", name, len(rows))
		for _, row := range rows {
			parts := make([]string, 0, len(cols))
			for i, col := range cols {
				v := row[i]
				if v == "" {
					continue
				}
				if isNoiseColumn(col) {
					continue
				}
				parts = append(parts, formatColumnValue(col, v))
			}
			b.WriteString("- " + strings.Join(parts, ", ") + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// columnHint pairs a human label with a SQL query that returns distinct values.
type columnHint struct {
	Label string
	Query string
}

// columnHints defines the queries for populating known-value hints.
// Each query must return a single text column of distinct non-null values
// from non-deleted rows, ordered alphabetically.
var columnHints = []columnHint{
	{
		"project statuses (stored values)",
		fmt.Sprintf("SELECT DISTINCT %s FROM %s WHERE %s IS NULL ORDER BY %s",
			ColStatus, TableProjects, ColDeletedAt, ColStatus),
	},
	{
		"project types",
		fmt.Sprintf("SELECT DISTINCT %s FROM %s ORDER BY %s",
			ColName, TableProjectTypes, ColName),
	},
	{
		"vendor names",
		fmt.Sprintf("SELECT DISTINCT %s FROM %s WHERE %s IS NULL ORDER BY %s",
			ColName, TableVendors, ColDeletedAt, ColName),
	},
	{
		"appliance names",
		fmt.Sprintf("SELECT DISTINCT %s FROM %s WHERE %s IS NULL ORDER BY %s",
			ColName, TableAppliances, ColDeletedAt, ColName),
	},
	{
		"maintenance categories",
		fmt.Sprintf("SELECT DISTINCT %s FROM %s ORDER BY %s",
			ColName, TableMaintenanceCategories, ColName),
	},
	{
		"maintenance item names",
		fmt.Sprintf("SELECT DISTINCT %s FROM %s WHERE %s IS NULL ORDER BY %s",
			ColName, TableMaintenanceItems, ColDeletedAt, ColName),
	},
}

// ColumnHints queries the database for distinct values in key columns and
// returns them as a formatted string suitable for inclusion in an LLM prompt.
// Returns empty string if no hints are available.
func (s *Store) ColumnHints() string {
	var b strings.Builder
	for _, h := range columnHints {
		var values []string
		if err := s.db.Raw(h.Query).Scan(&values).Error; err != nil || len(values) == 0 {
			continue
		}
		b.WriteString("- " + h.Label + ": " + strings.Join(values, ", ") + "\n")
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String()
}

// isNoiseColumn returns true for internal/bookkeeping columns that add
// clutter without helping the LLM answer user questions.
func isNoiseColumn(col string) bool {
	switch strings.ToLower(col) {
	case ColID, ColCreatedAt, ColUpdatedAt, ColDeletedAt, ColData:
		return true
	}
	return false
}

// formatColumnValue renders a column/value pair for the LLM. Money columns
// (suffix "_cents") are converted from cents to a $X.XX string; the suffix
// is stripped from the display name for clarity.
func formatColumnValue(col, val string) string {
	lower := strings.ToLower(col)
	if strings.HasSuffix(lower, "_cents") {
		if cents, err := strconv.ParseInt(val, 10, 64); err == nil {
			dollars := float64(cents) / 100
			label := strings.TrimSuffix(col, "_cents")
			if label == "" {
				label = col
			}
			return fmt.Sprintf("%s: $%.2f", label, dollars)
		}
	}
	return col + ": " + val
}

// IsSafeIdentifier returns true if s contains only alphanumerics and
// underscores -- safe for interpolation into SQL statements.
func IsSafeIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') &&
			(r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

// containsWord checks if s contains keyword as a standalone word (not part
// of a larger identifier like "deleted_at" matching "DELETE").
func containsWord(s, keyword string) bool {
	for i := 0; ; {
		idx := strings.Index(s[i:], keyword)
		if idx < 0 {
			return false
		}
		pos := i + idx
		end := pos + len(keyword)
		// Check that the match is at a word boundary.
		leftOK := pos == 0 || !isIdentChar(s[pos-1])
		rightOK := end >= len(s) || !isIdentChar(s[end])
		if leftOK && rightOK {
			return true
		}
		i = pos + 1
	}
}

func isIdentChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') ||
		(b >= '0' && b <= '9') || b == '_'
}

// dumpTable queries all non-deleted rows from a single table, returning
// them as string slices along with column names. The sql.Rows lifecycle
// is scoped to this function so defer closes correctly.
func dumpTable(s *Store, name string) ([][]string, []string, error) {
	//nolint:gosec // table name comes from sqlite_master, not user input
	sqlRows, err := s.db.Raw(fmt.Sprintf("SELECT * FROM %s", name)).Rows()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = sqlRows.Close() }()
	cols, err := sqlRows.Columns()
	if err != nil {
		return nil, nil, err
	}
	rows, err := scanTableRows(sqlRows, cols)
	if err != nil {
		return nil, nil, err
	}
	return rows, cols, nil
}

// scanTableRows reads all non-deleted rows from an open sql.Rows into
// string slices. The caller must close sqlRows after this returns.
func scanTableRows(sqlRows *sql.Rows, cols []string) ([][]string, error) {
	deletedAtIdx := -1
	for i, c := range cols {
		if strings.ToLower(c) == ColDeletedAt {
			deletedAtIdx = i
			break
		}
	}

	var rows [][]string
	for sqlRows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := sqlRows.Scan(ptrs...); err != nil {
			continue
		}
		if deletedAtIdx >= 0 && values[deletedAtIdx] != nil {
			continue
		}
		row := make([]string, len(cols))
		for i, v := range values {
			if v == nil {
				row[i] = ""
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		rows = append(rows, row)
	}
	if err := sqlRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}
