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

	s.mcpSrv.AddTool(
		mcpgo.NewTool("search_documents",
			mcpgo.WithDescription(
				"Full-text search over documents stored in micasa. Searches title, "+
					"notes, and extracted text using FTS5. Returns ranked results with "+
					"contextual snippets. Simple queries get automatic prefix matching. "+
					"Max 50 results.",
			),
			mcpgo.WithString("query",
				mcpgo.Description("Search query text"),
				mcpgo.Required(),
			),
		),
		s.handleSearchDocuments,
	)

	s.mcpSrv.AddTool(
		mcpgo.NewTool("get_schema",
			mcpgo.WithDescription(
				"Get database schema: table names, column definitions, and DDL. "+
					"Use this to understand the database structure before writing SQL "+
					"queries with the query tool.",
			),
			mcpgo.WithArray("tables",
				mcpgo.Description("Filter to specific table names. Returns all tables if empty."),
			),
		),
		s.handleGetSchema,
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

type schemaColumn struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	NotNull   bool    `json:"not_null"`
	DfltValue *string `json:"default_value,omitempty"`
	PK        bool    `json:"primary_key"`
}

type tableSchema struct {
	Name    string         `json:"name"`
	DDL     string         `json:"ddl"`
	Columns []schemaColumn `json:"columns"`
}

func (s *Server) handleGetSchema(
	_ context.Context,
	req mcpgo.CallToolRequest,
) (*mcpgo.CallToolResult, error) {
	tables := req.GetStringSlice("tables", nil)

	if len(tables) == 0 {
		var err error
		tables, err = s.store.TableNames()
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("list tables: %v", err)), nil
		}
	}

	ddlMap, err := s.store.TableDDL(tables...)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("get DDL: %v", err)), nil
	}

	result := make([]tableSchema, 0, len(tables))
	for _, name := range tables {
		cols, err := s.store.TableColumns(name)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("columns for %s: %v", name, err)), nil
		}
		sc := make([]schemaColumn, 0, len(cols))
		for _, c := range cols {
			sc = append(sc, schemaColumn{
				Name:      c.Name,
				Type:      c.Type,
				NotNull:   c.NotNull,
				DfltValue: c.DfltValue,
				PK:        c.PK > 0,
			})
		}
		result = append(result, tableSchema{
			Name:    name,
			DDL:     ddlMap[name],
			Columns: sc,
		})
	}

	b, err := json.Marshal(result)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("marshal schema: %v", err)), nil
	}
	return mcpgo.NewToolResultText(string(b)), nil
}

type documentResult struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	FileName   string `json:"file_name"`
	EntityKind string `json:"entity_kind"`
	EntityID   string `json:"entity_id"`
	Snippet    string `json:"snippet"`
	UpdatedAt  string `json:"updated_at"`
}

func (s *Server) handleSearchDocuments(
	_ context.Context,
	req mcpgo.CallToolRequest,
) (*mcpgo.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}

	results, err := s.store.SearchDocuments(query)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	out := make([]documentResult, 0, len(results))
	for _, r := range results {
		out = append(out, documentResult{
			ID:         r.ID,
			Title:      r.Title,
			FileName:   r.FileName,
			EntityKind: r.EntityKind,
			EntityID:   r.EntityID,
			Snippet:    r.Snippet,
			UpdatedAt:  r.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	b, err := json.Marshal(out)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("marshal results: %v", err)), nil
	}
	return mcpgo.NewToolResultText(string(b)), nil
}
