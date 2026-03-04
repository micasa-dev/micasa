// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var testTables = []TableInfo{
	{
		Name: "projects",
		Columns: []ColumnInfo{
			{Name: "id", Type: "integer", PK: true},
			{Name: "title", Type: "text", NotNull: true},
			{Name: "budget_cents", Type: "integer"},
			{Name: "status", Type: "text"},
		},
	},
	{
		Name: "appliances",
		Columns: []ColumnInfo{
			{Name: "id", Type: "integer", PK: true},
			{Name: "name", Type: "text", NotNull: true},
		},
	},
}

var testNow = time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)

// --- BuildSystemPrompt (fallback) ---

func TestBuildSystemPromptIncludesSchema(t *testing.T) {
	t.Parallel()
	prompt := BuildSystemPrompt(testTables, "", testNow, "")
	assert.Contains(t, prompt, "projects")
	assert.Contains(t, prompt, "id integer PK")
	assert.Contains(t, prompt, "title text NOT NULL")
	assert.Contains(t, prompt, "status text")
	assert.Contains(t, prompt, "home management")
}

func TestBuildSystemPromptIncludesData(t *testing.T) {
	t.Parallel()
	prompt := BuildSystemPrompt(
		nil,
		"### projects (3 rows)\n\n- id: 1, title: Fix roof\n",
		testNow,
		"",
	)
	assert.Contains(t, prompt, "Fix roof")
	assert.Contains(t, prompt, "Current Data")
}

func TestBuildSystemPromptOmitsDataWhenEmpty(t *testing.T) {
	t.Parallel()
	prompt := BuildSystemPrompt(nil, "", testNow, "")
	assert.NotContains(t, prompt, "Current Data")
}

func TestBuildSystemPromptIncludesCurrentDate(t *testing.T) {
	t.Parallel()
	prompt := BuildSystemPrompt(nil, "", testNow, "")
	assert.Contains(t, prompt, "Friday, February 13, 2026")
}

func TestBuildSystemPromptIncludesExtraContext(t *testing.T) {
	t.Parallel()
	prompt := BuildSystemPrompt(nil, "", testNow, "House is a 1920s craftsman.")
	assert.Contains(t, prompt, "Additional context")
	assert.Contains(t, prompt, "1920s craftsman")
}

// --- BuildSQLPrompt ---

func TestBuildSQLPromptIncludesDDL(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "")
	assert.Contains(t, prompt, "CREATE TABLE projects")
	assert.Contains(t, prompt, "id integer PRIMARY KEY")
	assert.Contains(t, prompt, "title text NOT NULL")
	assert.Contains(t, prompt, "budget_cents integer")
	assert.Contains(t, prompt, "CREATE TABLE appliances")
}

func TestBuildSQLPromptIncludesFewShotExamples(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "")
	assert.Contains(t, prompt, "SELECT COUNT(*)")
	assert.Contains(t, prompt, "budget_cents / 100.0")
	assert.Contains(t, prompt, "deleted_at IS NULL")
}

func TestBuildSQLPromptIncludesRules(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "")
	assert.Contains(t, prompt, "single SELECT statement")
	assert.Contains(t, prompt, "never INSERT")
}

func TestBuildSQLPromptIncludesCurrentDate(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "")
	assert.Contains(t, prompt, "Friday, February 13, 2026")
}

func TestBuildSQLPromptIncludesExtraContext(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "Budgets are in CAD.")
	assert.Contains(t, prompt, "Additional context")
	assert.Contains(t, prompt, "Budgets are in CAD")
}

// --- BuildSummaryPrompt ---

func TestBuildSummaryPromptIncludesAllParts(t *testing.T) {
	t.Parallel()
	prompt := BuildSummaryPrompt(
		"How many projects?",
		"SELECT COUNT(*) AS count FROM projects",
		"count\n3\n",
		testNow,
		"",
	)
	assert.Contains(t, prompt, "How many projects?")
	assert.Contains(t, prompt, "SELECT COUNT(*)")
	assert.Contains(t, prompt, "count\n3")
	assert.Contains(t, prompt, "concise")
}

func TestBuildSummaryPromptIncludesCurrentDate(t *testing.T) {
	t.Parallel()
	prompt := BuildSummaryPrompt("test", "SELECT 1", "1\n", testNow, "")
	assert.Contains(t, prompt, "Friday, February 13, 2026")
}

func TestBuildSummaryPromptIncludesExtraContext(t *testing.T) {
	t.Parallel()
	prompt := BuildSummaryPrompt("test", "SELECT 1", "1\n", testNow, "Currency is CAD.")
	assert.Contains(t, prompt, "Additional context")
	assert.Contains(t, prompt, "Currency is CAD")
}

// --- FormatResultsTable ---

func TestFormatResultsTableWithRows(t *testing.T) {
	t.Parallel()
	result := FormatResultsTable(
		[]string{"name", "budget"},
		[][]string{
			{"Kitchen", "$5000"},
			{"Deck", "$3000"},
		},
	)
	assert.Contains(t, result, "name | budget")
	assert.Contains(t, result, "Kitchen | $5000")
	assert.Contains(t, result, "Deck | $3000")
}

func TestFormatResultsTableEmpty(t *testing.T) {
	t.Parallel()
	result := FormatResultsTable([]string{"name"}, nil)
	assert.Equal(t, "(no rows)\n", result)
}

// --- ExtractSQL ---

func TestExtractSQLBare(t *testing.T) {
	t.Parallel()
	sql := ExtractSQL("SELECT * FROM projects")
	assert.Equal(t, "SELECT * FROM projects", sql)
}

func TestExtractSQLWithFences(t *testing.T) {
	t.Parallel()
	raw := "```sql\nSELECT * FROM projects;\n```"
	sql := ExtractSQL(raw)
	assert.Equal(t, "SELECT * FROM projects", sql)
}

func TestExtractSQLWithBareBackticks(t *testing.T) {
	t.Parallel()
	raw := "```\nSELECT COUNT(*) FROM appliances\n```"
	sql := ExtractSQL(raw)
	assert.Equal(t, "SELECT COUNT(*) FROM appliances", sql)
}

func TestExtractSQLStripsTrailingSemicolons(t *testing.T) {
	t.Parallel()
	sql := ExtractSQL("SELECT 1;;;")
	assert.Equal(t, "SELECT 1", sql)
}

func TestExtractSQLTrimsWhitespace(t *testing.T) {
	t.Parallel()
	sql := ExtractSQL("  \n  SELECT 1  \n  ")
	assert.Equal(t, "SELECT 1", sql)
}

func TestBuildSQLPromptIncludesEntityRelationships(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "")
	assert.Contains(t, prompt, "## Entity Relationships")
	assert.Contains(t, prompt, "Foreign key relationships")
	assert.Contains(t, prompt, "projects.project_type_id")
	assert.Contains(t, prompt, "maintenance_items.appliance_id")
	assert.Contains(t, prompt, "incidents.appliance_id")
	assert.Contains(t, prompt, "incidents.vendor_id")
	assert.Contains(t, prompt, "NO direct FK between projects and appliances")
}

func TestBuildSystemPromptIncludesEntityRelationships(t *testing.T) {
	t.Parallel()
	prompt := BuildSystemPrompt(testTables, "", testNow, "")
	assert.Contains(t, prompt, "## Entity Relationships")
	assert.Contains(t, prompt, "Foreign key relationships")
	assert.Contains(t, prompt, "projects.project_type_id")
	assert.Contains(t, prompt, "maintenance_items.appliance_id")
	assert.Contains(t, prompt, "incidents.appliance_id")
	assert.Contains(t, prompt, "incidents.vendor_id")
	assert.Contains(t, prompt, "NO direct FK between projects and appliances")
}

func TestBuildSQLPromptIncludesCaseInsensitiveGuidance(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "")
	assert.Contains(t, prompt, "case-insensitive matching")
	assert.Contains(t, prompt, "LOWER()")
}

func TestBuildSQLPromptIncludesColumnHints(t *testing.T) {
	t.Parallel()
	hints := "- project types: electrical, flooring, plumbing\n"
	prompt := BuildSQLPrompt(testTables, testNow, hints, "")
	assert.Contains(t, prompt, "Known values in the database")
	assert.Contains(t, prompt, "electrical, flooring, plumbing")
}

func TestBuildSQLPromptOmitsColumnHintsWhenEmpty(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "")
	assert.NotContains(t, prompt, "Known values")
}

func TestBuildSQLPromptIncludesGroupByExamples(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "")
	assert.Contains(t, prompt, "GROUP BY")
	assert.Contains(t, prompt, "total spending by project status")
	assert.Contains(t, prompt, "vendors have given me the most quotes")
	assert.Contains(t, prompt, "average quote amount")
}

func TestBuildSQLPromptIncludesIncidentExamples(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "")
	assert.Contains(t, prompt, "What open incidents do I have?")
	assert.Contains(t, prompt, "FROM incidents WHERE status IN ('open', 'in_progress')")
	assert.Contains(t, prompt, "How much have I spent on incidents this year?")
	assert.Contains(t, prompt, "SUM(cost_cents) / 100.0")
}

func TestBuildSQLPromptIncludesIncidentSchemaNotes(t *testing.T) {
	t.Parallel()
	prompt := BuildSQLPrompt(testTables, testNow, "", "")
	assert.Contains(t, prompt, "Incident statuses: open, in_progress")
	assert.Contains(t, prompt, "Incident severities: urgent, soon, whenever")
}

func TestBuildSystemPromptIncludesIncidentFallbackNotes(t *testing.T) {
	t.Parallel()
	prompt := BuildSystemPrompt(testTables, "", testNow, "")
	assert.Contains(t, prompt, "Incident statuses: open, in_progress")
	assert.Contains(t, prompt, "Incident severities: urgent, soon, whenever")
}
