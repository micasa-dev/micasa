// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) registerTools() {
	s.mcpSrv.AddTool(
		mcpgo.NewTool("query",
			mcpgo.WithDescription(
				"Execute read-only SQL against the micasa home management database. "+
					"Supports SELECT and WITH statements. Defense-in-depth validation "+
					"enforces read-only access. Max 200 rows returned. Use get_schema "+
					"to discover table structure before writing queries.",
			),
			mcpgo.WithString("sql",
				mcpgo.Description("SQL query (SELECT or WITH only)"),
				mcpgo.Required(),
			),
		),
		s.handleQuery,
	)
}

type queryResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

func (s *Server) handleQuery(
	_ context.Context,
	req mcpgo.CallToolRequest,
) (*mcpgo.CallToolResult, error) {
	sql, err := req.RequireString("sql")
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}

	cols, rows, err := s.store.ReadOnlyQuery(sql)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("query failed: %v", err)), nil
	}

	b, err := json.Marshal(queryResult{Columns: cols, Rows: rows})
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("marshal result: %v", err)), nil
	}
	return mcpgo.NewToolResultText(string(b)), nil
}
