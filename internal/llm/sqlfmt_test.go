// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatSQLSimpleSelect(t *testing.T) {
	t.Parallel()
	got := FormatSQL("SELECT name, age FROM users WHERE age > 21", 0)
	expected := "SELECT name,\n  age\nFROM users\nWHERE age > 21"
	assert.Equal(t, expected, got)
}

func TestFormatSQLSingleColumn(t *testing.T) {
	t.Parallel()
	got := FormatSQL("SELECT COUNT(*) FROM projects WHERE deleted_at IS NULL", 0)
	expected := "SELECT COUNT(*)\nFROM projects\nWHERE deleted_at IS NULL"
	assert.Equal(t, expected, got)
}

func TestFormatSQLMultipleClauses(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"SELECT name, budget_cents / 100.0 AS budget FROM projects "+
			"WHERE status = 'underway' AND deleted_at IS NULL "+
			"ORDER BY budget_cents DESC LIMIT 5",
		0,
	)
	expected := "SELECT name,\n" +
		"  budget_cents / 100.0 AS budget\n" +
		"FROM projects\n" +
		"WHERE status = 'underway'\n" +
		"  AND deleted_at IS NULL\n" +
		"ORDER BY budget_cents DESC\n" +
		"LIMIT 5"
	assert.Equal(t, expected, got)
}

func TestFormatSQLJoin(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"SELECT m.name, a.name FROM maintenance_items m "+
			"LEFT JOIN appliances a ON m.appliance_id = a.id "+
			"WHERE m.deleted_at IS NULL",
		0,
	)
	expected := "SELECT m.name,\n" +
		"  a.name\n" +
		"FROM maintenance_items m\n" +
		"LEFT JOIN appliances a\nON m.appliance_id = a.id\n" +
		"WHERE m.deleted_at IS NULL"
	assert.Equal(t, expected, got)
}

func TestFormatSQLSubquery(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"SELECT name FROM projects WHERE id IN (SELECT project_id FROM quotes WHERE total_cents > 10000)",
		0,
	)
	assert.Contains(t, got, "SELECT name")
	assert.Contains(t, got, "FROM projects")
	assert.Contains(t, got, "WHERE id IN (SELECT project_id FROM quotes WHERE total_cents > 10000)")
}

func TestFormatSQLNestedSubquery(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"SELECT name, (SELECT COUNT(*) FROM quotes WHERE project_id = projects.id) AS quote_count FROM projects WHERE status = 'active'",
		0,
	)
	// Verify main query columns are on separate lines
	assert.Contains(t, got, "SELECT name,")
	assert.Contains(t, got, "AS quote_count")
	assert.Contains(t, got, "FROM projects")
	assert.Contains(t, got, "WHERE status = 'active'")
	// Verify each column is on its own line
	lines := strings.Split(got, "\n")
	assert.Len(t, lines, 4, "should have 4 lines: SELECT, column with subquery, FROM, WHERE")
	// Second line should be indented and contain the subquery
	assert.True(t, strings.HasPrefix(lines[1], "  "), "second column should be indented")
	assert.Contains(t, lines[1], "SELECT COUNT(*)", "should contain nested SELECT on column line")
}

func TestFormatSQLGroupBy(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"SELECT status, COUNT(*) AS cnt FROM projects "+
			"WHERE deleted_at IS NULL "+
			"GROUP BY status "+
			"HAVING cnt > 1 "+
			"ORDER BY cnt DESC",
		0,
	)
	expected := "SELECT status,\n" +
		"  COUNT(*) AS cnt\n" +
		"FROM projects\n" +
		"WHERE deleted_at IS NULL\n" +
		"GROUP BY status\n" +
		"HAVING cnt > 1\n" +
		"ORDER BY cnt DESC"
	assert.Equal(t, expected, got)
}

func TestFormatSQLKeywordsUppercased(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"select name from projects where status = 'underway' and deleted_at is null limit 1",
		0,
	)
	assert.Contains(t, got, "SELECT")
	assert.Contains(t, got, "FROM")
	assert.Contains(t, got, "WHERE")
	assert.Contains(t, got, "AND")
	assert.Contains(t, got, "IS NULL")
	assert.Contains(t, got, "LIMIT")
	assert.Contains(t, got, "name")
	assert.Contains(t, got, "projects")
	assert.Contains(t, got, "status")
	assert.Contains(t, got, "deleted_at")
}

func TestFormatSQLPreservesStrings(t *testing.T) {
	t.Parallel()
	got := FormatSQL("SELECT * FROM projects WHERE name = 'Kitchen Remodel'", 0)
	assert.Contains(t, got, "'Kitchen Remodel'")
}

func TestFormatSQLDateFunctions(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"SELECT name, date(last_serviced_at, '+' || interval_months || ' months') AS next_due "+
			"FROM maintenance_items WHERE deleted_at IS NULL ORDER BY next_due",
		0,
	)
	assert.Contains(t, got, "SELECT name")
	assert.Contains(
		t,
		got,
		"date(last_serviced_at, '+' || interval_months || ' months') AS next_due",
	)
	assert.Contains(t, got, "FROM maintenance_items")
	assert.Contains(t, got, "ORDER BY next_due")
}

func TestFormatSQLEmpty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, FormatSQL("", 0))
}

func TestFormatSQLAlreadyFormatted(t *testing.T) {
	t.Parallel()
	input := "SELECT name\nFROM projects\nWHERE id = 1"
	got := FormatSQL(input, 0)
	assert.Contains(t, got, "SELECT name")
	assert.Contains(t, got, "FROM projects")
	assert.Contains(t, got, "WHERE id = 1")
}

func TestFormatSQLBetween(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"SELECT name FROM appliances WHERE warranty_expiry BETWEEN date('now') AND date('now', '+90 days')",
		0,
	)
	assert.Contains(t, got, "BETWEEN")
	assert.Contains(t, got, "date('now')")
}

func TestFormatSQLAggregateWithJoin(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"SELECT SUM(q.total_cents) / 100.0 AS total FROM quotes q "+
			"JOIN projects p ON q.project_id = p.id "+
			"WHERE p.deleted_at IS NULL AND q.deleted_at IS NULL",
		0,
	)
	expected := "SELECT SUM(q.total_cents) / 100.0 AS total\n" +
		"FROM quotes q\n" +
		"JOIN projects p\nON q.project_id = p.id\n" +
		"WHERE p.deleted_at IS NULL\n" +
		"  AND q.deleted_at IS NULL"
	assert.Equal(t, expected, got)
}

func TestFormatSQLWrapsLongLines(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"SELECT name, date(last_serviced_at, '+' || interval_months || ' months') AS next_due "+
			"FROM maintenance_items WHERE deleted_at IS NULL ORDER BY next_due",
		40,
	)
	for _, line := range strings.Split(got, "\n") {
		assert.LessOrEqual(t, len(line), 50,
			"line too long (allowing 10 char grace for unbreakable tokens): %q", line)
	}
	// Verify content is still present.
	assert.Contains(t, got, "SELECT name")
	assert.Contains(t, got, "next_due")
	assert.Contains(t, got, "FROM maintenance_items")
}

func TestFormatSQLWrapPreservesIndent(t *testing.T) {
	t.Parallel()
	got := FormatSQL(
		"SELECT a_very_long_column_name, another_really_long_column_name, yet_another_one "+
			"FROM some_table",
		30,
	)
	lines := strings.Split(got, "\n")
	// Continuation lines from SELECT columns should be indented.
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "FROM") {
			assert.True(t, strings.HasPrefix(line, "  "),
				"continuation line should be indented: %q", line)
		}
	}
}

// --- tokenizer tests ---

func TestTokenizeSQLBasic(t *testing.T) {
	t.Parallel()
	tokens := tokenizeSQL("SELECT name FROM users")
	words := filterKind(tokens, tokWord)
	assert.Equal(t, []string{"SELECT", "name", "FROM", "users"}, words)
}

func TestTokenizeSQLString(t *testing.T) {
	t.Parallel()
	tokens := tokenizeSQL("WHERE name = 'O''Brien'")
	strs := filterKind(tokens, tokString)
	assert.Equal(t, []string{"'O''Brien'"}, strs)
}

func TestTokenizeSQLNumbers(t *testing.T) {
	t.Parallel()
	tokens := tokenizeSQL("LIMIT 10 OFFSET 3.5")
	nums := filterKind(tokens, tokNumber)
	assert.Equal(t, []string{"10", "3.5"}, nums)
}

func TestTokenizeSQLOperators(t *testing.T) {
	t.Parallel()
	tokens := tokenizeSQL("a >= 1 AND b <> 2")
	syms := filterKind(tokens, tokSymbol)
	assert.Contains(t, syms, ">=")
	assert.Contains(t, syms, "<>")
}

func filterKind(tokens []sqlToken, kind sqlTokenKind) []string {
	var result []string
	for _, t := range tokens {
		if t.Kind == kind {
			result = append(result, t.Text)
		}
	}
	return result
}
