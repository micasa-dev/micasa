// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

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

func TestGetSchemaTool(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "get_schema", map[string]any{})
	require.False(t, result.IsError)

	raw, err := json.Marshal(result.Content)
	require.NoError(t, err)
	output := string(raw)
	assert.Contains(t, output, "vendors")
	assert.Contains(t, output, "projects")
}

func TestGetSchemaToolFiltered(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "get_schema", map[string]any{
		"tables": []any{"vendors"},
	})
	require.False(t, result.IsError)

	raw, err := json.Marshal(result.Content)
	require.NoError(t, err)
	output := string(raw)
	assert.Contains(t, output, "vendors")
	assert.NotContains(t, output, "projects")
}
