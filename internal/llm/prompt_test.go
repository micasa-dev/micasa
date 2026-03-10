// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

import (
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testTables = []TableInfo{
	{
		Name: data.TableProjects,
		Columns: []ColumnInfo{
			{Name: data.ColID, Type: "integer", PK: true},
			{Name: data.ColTitle, Type: "text", NotNull: true},
			{Name: data.ColBudgetCents, Type: "integer"},
			{Name: data.ColStatus, Type: "text"},
		},
	},
	{
		Name: data.TableAppliances,
		Columns: []ColumnInfo{
			{Name: data.ColID, Type: "integer", PK: true},
			{Name: data.ColName, Type: "text", NotNull: true},
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

// --- BuildInsightsPrompt ---

func TestBuildInsightsPromptIncludesData(t *testing.T) {
	t.Parallel()
	prompt := BuildInsightsPrompt(
		"### appliances (2 rows)\n\n- id: 1, name: Water heater\n",
		testNow,
		"",
	)
	assert.Contains(t, prompt, "Water heater")
	assert.Contains(t, prompt, "Current Data")
}

func TestBuildInsightsPromptOmitsDataWhenEmpty(t *testing.T) {
	t.Parallel()
	prompt := BuildInsightsPrompt("", testNow, "")
	assert.NotContains(t, prompt, "Current Data")
}

func TestBuildInsightsPromptIncludesCurrentDate(t *testing.T) {
	t.Parallel()
	prompt := BuildInsightsPrompt("", testNow, "")
	assert.Contains(t, prompt, "Friday, February 13, 2026")
}

func TestBuildInsightsPromptIncludesPreamble(t *testing.T) {
	t.Parallel()
	prompt := BuildInsightsPrompt("", testNow, "")
	assert.Contains(t, prompt, "home maintenance analyst")
	assert.Contains(t, prompt, "non-obvious patterns")
}

func TestBuildInsightsPromptIncludesGuidelines(t *testing.T) {
	t.Parallel()
	prompt := BuildInsightsPrompt("", testNow, "")
	assert.Contains(t, prompt, "Output format")
	assert.Contains(t, prompt, "entity_id")
	assert.Contains(t, prompt, `"category"`)
	assert.Contains(t, prompt, "attention")
	assert.Contains(t, prompt, "stale")
	assert.Contains(t, prompt, "pattern")
}

func TestBuildInsightsPromptIncludesExtraContext(t *testing.T) {
	t.Parallel()
	prompt := BuildInsightsPrompt("", testNow, "House built in 1998.")
	assert.Contains(t, prompt, "Additional context")
	assert.Contains(t, prompt, "House built in 1998")
}

// --- InsightsJSONSchema ---

func TestInsightsJSONSchemaStructure(t *testing.T) {
	t.Parallel()
	schema := InsightsJSONSchema()
	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)

	insights, ok := props["insights"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "array", insights["type"])

	items, ok := insights["items"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", items["type"])

	itemProps, ok := items["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, itemProps, "text")
	assert.Contains(t, itemProps, "tab")
	assert.Contains(t, itemProps, "entity_id")
	assert.Contains(t, itemProps, "category")

	required, ok := items["required"].([]string)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"text", "tab", "entity_id", "category"}, required)
}

func TestInsightsJSONSchemaTabEnum(t *testing.T) {
	t.Parallel()
	schema := InsightsJSONSchema()
	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	insights, ok := props["insights"].(map[string]any)
	require.True(t, ok)
	items, ok := insights["items"].(map[string]any)
	require.True(t, ok)
	itemProps, ok := items["properties"].(map[string]any)
	require.True(t, ok)
	tab, ok := itemProps["tab"].(map[string]any)
	require.True(t, ok)

	tabEnum, ok := tab["enum"].([]string)
	require.True(t, ok)
	assert.Contains(t, tabEnum, "projects")
	assert.Contains(t, tabEnum, "appliances")
	assert.Contains(t, tabEnum, "maintenance")
	assert.Contains(t, tabEnum, "incidents")
	assert.Contains(t, tabEnum, "vendors")
	assert.Contains(t, tabEnum, "documents")
	assert.Contains(t, tabEnum, "quotes")
}
