// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/mcp"
)

func newTestServer(t *testing.T) (*mcp.Server, *data.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	err = store.AutoMigrate()
	require.NoError(t, err)

	srv := mcp.NewServer(store)
	return srv, store
}

func callTool(
	t *testing.T,
	srv *mcp.Server,
	name string,
	args map[string]any,
) *mcpgo.CallToolResult {
	t.Helper()

	testSrv := mcptest.NewUnstartedServer(t)
	for _, tool := range srv.Tools() {
		testSrv.AddTools(tool)
	}
	err := testSrv.Start(context.Background())
	require.NoError(t, err)
	t.Cleanup(testSrv.Close)

	client := testSrv.Client()
	var req mcpgo.CallToolRequest
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := client.CallTool(context.Background(), req)
	require.NoError(t, err)
	return result
}

func TestListTools(t *testing.T) {
	srv, _ := newTestServer(t)

	testSrv := mcptest.NewUnstartedServer(t)
	for _, tool := range srv.Tools() {
		testSrv.AddTools(tool)
	}
	err := testSrv.Start(context.Background())
	require.NoError(t, err)
	t.Cleanup(testSrv.Close)

	client := testSrv.Client()
	tools, err := client.ListTools(context.Background(), mcpgo.ListToolsRequest{})
	require.NoError(t, err)

	toolNames := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		toolNames = append(toolNames, tool.Name)
	}

	assert.Contains(t, toolNames, "query")
	assert.Contains(t, toolNames, "get_schema")
	assert.Contains(t, toolNames, "search_documents")
	assert.Contains(t, toolNames, "get_maintenance_schedule")
	assert.Contains(t, toolNames, "get_house_profile")
}

func TestQueryTool(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "query", map[string]any{
		"sql": "SELECT COUNT(*) AS cnt FROM vendors",
	})
	assert.False(t, result.IsError)

	raw, err := json.Marshal(result.Content)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "cnt")
}

func TestQueryToolInvalidSQL(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "query", map[string]any{
		"sql": "DROP TABLE vendors",
	})
	assert.True(t, result.IsError)
}

func toolResultText(t *testing.T, result *mcpgo.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	return tc.Text
}

func TestGetSchemaTool(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "get_schema", map[string]any{})
	require.False(t, result.IsError)

	var schema struct {
		Tables []struct {
			Name string `json:"name"`
		} `json:"tables"`
	}
	require.NoError(t, json.Unmarshal([]byte(toolResultText(t, result)), &schema))
	require.NotEmpty(t, schema.Tables)

	names := make([]string, 0, len(schema.Tables))
	for _, tbl := range schema.Tables {
		names = append(names, tbl.Name)
	}
	assert.Contains(t, names, "vendors")
	assert.Contains(t, names, "projects")
}

func TestGetSchemaToolFiltered(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "get_schema", map[string]any{
		"tables": []any{"vendors"},
	})
	require.False(t, result.IsError)

	var schema struct {
		Tables []struct {
			Name string `json:"name"`
		} `json:"tables"`
	}
	require.NoError(t, json.Unmarshal([]byte(toolResultText(t, result)), &schema))
	require.Len(t, schema.Tables, 1)
	assert.Equal(t, "vendors", schema.Tables[0].Name)
}

func TestSearchDocumentsTool(t *testing.T) {
	srv, store := newTestServer(t)

	require.NoError(t, store.SetMaxDocumentSize(1<<20))

	doc := &data.Document{
		Title:         "HVAC Manual",
		FileName:      "hvac.pdf",
		MIMEType:      "application/pdf",
		SizeBytes:     100,
		Data:          []byte("dummy"),
		ExtractedText: "furnace maintenance guide",
	}
	require.NoError(t, store.CreateDocument(doc))

	result := callTool(t, srv, "search_documents", map[string]any{
		"query": "furnace",
	})
	require.False(t, result.IsError)

	raw, err := json.Marshal(result.Content)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "HVAC Manual")
}

func TestSearchDocumentsToolEmpty(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "search_documents", map[string]any{
		"query": "nonexistent",
	})
	assert.False(t, result.IsError)
}

func TestGetMaintenanceScheduleTool(t *testing.T) {
	srv, store := newTestServer(t)
	require.NoError(t, store.SeedDefaults())

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)

	var hvacCat data.MaintenanceCategory
	for _, c := range cats {
		if c.Name == "HVAC" {
			hvacCat = c
			break
		}
	}
	require.NotEmpty(t, hvacCat.ID, "HVAC category not found in seeded data")

	sixMonthsAgo := time.Now().AddDate(0, -6, 0)
	item := &data.MaintenanceItem{
		Name:           "Replace furnace filter",
		CategoryID:     hvacCat.ID,
		IntervalMonths: 3,
		LastServicedAt: &sixMonthsAgo,
	}
	require.NoError(t, store.CreateMaintenance(item))

	result := callTool(t, srv, "get_maintenance_schedule", map[string]any{})
	require.False(t, result.IsError)

	var items []struct {
		Name    string     `json:"name"`
		Overdue bool       `json:"overdue"`
		DueDate *time.Time `json:"due_date"`
	}
	require.NoError(t, json.Unmarshal([]byte(toolResultText(t, result)), &items))
	require.NotEmpty(t, items)

	var found bool
	for _, it := range items {
		if it.Name == "Replace furnace filter" {
			found = true
			assert.True(t, it.Overdue)
			require.NotNil(t, it.DueDate, "computed due_date should be populated")
			break
		}
	}
	require.True(t, found, "expected maintenance item not found")
}

func TestGetHouseProfileTool(t *testing.T) {
	srv, store := newTestServer(t)

	profile := data.HouseProfile{
		Nickname:   "Lake House",
		PostalCode: "90210",
	}
	require.NoError(t, store.CreateHouseProfile(profile))

	result := callTool(t, srv, "get_house_profile", map[string]any{})
	require.False(t, result.IsError)

	raw, err := json.Marshal(result.Content)
	require.NoError(t, err)
	output := string(raw)
	assert.Contains(t, output, "Lake House")
	assert.Contains(t, output, "90210")
}

func TestGetHouseProfileToolEmpty(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "get_house_profile", map[string]any{})
	assert.True(t, result.IsError)
}
