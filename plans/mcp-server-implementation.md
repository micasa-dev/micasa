<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# MCP Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `micasa mcp` subcommand exposing home data to MCP-compatible LLM clients over stdio.

**Architecture:** Thin wrapper in `internal/mcp/` over the existing `internal/data.Store`. Five read-only tools: `query`, `get_schema`, `search_documents`, `get_maintenance_schedule`, `get_house_profile`. Cobra subcommand opens DB with `PRAGMA query_only = ON` and runs mcp-go stdio transport.

**Tech Stack:** Go, mcp-go (`github.com/mark3labs/mcp-go`), existing `internal/data` Store, cobra

**Spec:** `plans/mcp-server.md`

---

### Task 1: Add mcp-go Dependency and Store.SetQueryOnly

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `internal/data/store.go` (add `SetQueryOnly` method near line 320)

- [ ] **Step 1: Audit mcp-go source**

Skim the mcp-go repo for security issues, license compatibility (MIT), and
transitive dependencies. Check for anything that conflicts with Apache-2.0.

- [ ] **Step 2: Add the dependency**

Run: `go get github.com/mark3labs/mcp-go@latest`

- [ ] **Step 3: Add SetQueryOnly method to Store**

In `internal/data/store.go`, after `IsMicasaDB()`:

```go
func (s *Store) SetQueryOnly() error {
	return s.db.Exec("PRAGMA query_only = ON").Error
}
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./...`

- [ ] **Step 5: Commit**

`feat(data): add SetQueryOnly method for read-only MCP access`

---

### Task 2: Server Skeleton with query Tool (TDD)

**Files:**
- Create: `internal/mcp/server.go` (Server struct, NewServer, ServeStdio, MCPServer)
- Create: `internal/mcp/tools.go` (registerTools, tool handlers)
- Create: `internal/mcp/server_test.go`

- [ ] **Step 1: Write integration test for query tool**

Create `internal/mcp/server_test.go`:

```go
package mcp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
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
	t.Cleanup(func() { store.Close() })

	err = store.AutoMigrate()
	require.NoError(t, err)

	srv := mcp.NewServer(store)
	return srv, store
}

func callTool(t *testing.T, srv *mcp.Server, name string, args map[string]any) *mcpgo.CallToolResult {
	t.Helper()
	mcpSrv := srv.MCPServer()
	ctx := context.Background()

	result := mcpSrv.HandleMessage(ctx, mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": args,
		},
	}))

	// Parse the JSON-RPC response to extract the tool result
	raw, err := json.Marshal(result)
	require.NoError(t, err)

	var resp struct {
		Result mcpgo.CallToolResult `json:"result"`
	}
	err = json.Unmarshal(raw, &resp)
	require.NoError(t, err)
	return &resp.Result
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/...`
Expected: FAIL (package doesn't exist yet)

- [ ] **Step 3: Create server.go and tools.go with query tool**

Create `internal/mcp/server.go` (Server struct, constructor, stdio runner):

```go
package mcp

import (
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/micasa-dev/micasa/internal/data"
)

type Server struct {
	store  *data.Store
	mcpSrv *mcpserver.MCPServer
}

func NewServer(store *data.Store) *Server {
	s := &Server{store: store}
	s.mcpSrv = mcpserver.NewMCPServer(
		"micasa",
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
	)
	s.registerTools()
	return s
}

func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcpSrv
}

func (s *Server) ServeStdio() error {
	return mcpserver.ServeStdio(s.mcpSrv)
}
```

Create `internal/mcp/tools.go` (tool registration and handlers):

```go
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

func (s *Server) handleQuery(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/mcp/...`
Expected: PASS

- [ ] **Step 5: Commit**

`feat(mcp): add server skeleton with query tool`

---

### Task 3: get_schema Tool (TDD)

**Files:**
- Modify: `internal/mcp/tools.go` (add tool registration + handler)
- Modify: `internal/mcp/server_test.go` (add tests)

- [ ] **Step 1: Write test for get_schema**

Append to `internal/mcp/server_test.go`:

```go
func TestGetSchemaTool(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "get_schema", map[string]any{})
	assert.False(t, result.IsError)

	raw, err := json.Marshal(result.Content)
	require.NoError(t, err)
	// Should contain known table names
	assert.Contains(t, string(raw), "vendors")
	assert.Contains(t, string(raw), "projects")
}

func TestGetSchemaToolFiltered(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "get_schema", map[string]any{
		"tables": []string{"vendors"},
	})
	assert.False(t, result.IsError)

	raw, err := json.Marshal(result.Content)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "vendors")
	assert.NotContains(t, string(raw), "projects")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/... -run TestGetSchema`
Expected: FAIL

- [ ] **Step 3: Add get_schema tool registration and handler**

In `tools.go`, add to `registerTools()`:

```go
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
```

Add handler:

```go
type schemaTable struct {
	Name    string         `json:"name"`
	Columns []data.PragmaColumn `json:"columns"`
	DDL     string         `json:"ddl"`
}

type schemaResult struct {
	Tables []schemaTable `json:"tables"`
}

func (s *Server) handleGetSchema(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	// Extract optional tables filter
	var tableFilter []string
	if raw, ok := req.Params.Arguments["tables"]; ok {
		if arr, ok := raw.([]any); ok {
			for _, v := range arr {
				if name, ok := v.(string); ok {
					tableFilter = append(tableFilter, name)
				}
			}
		}
	}

	// Get table names
	var tables []string
	if len(tableFilter) > 0 {
		tables = tableFilter
	} else {
		var err error
		tables, err = s.store.TableNames()
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("list tables: %v", err)), nil
		}
	}

	// Get DDL for all requested tables
	ddlMap, err := s.store.TableDDL(tables...)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("get ddl: %v", err)), nil
	}

	// Build result
	result := schemaResult{Tables: make([]schemaTable, 0, len(tables))}
	for _, name := range tables {
		cols, err := s.store.TableColumns(name)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("columns for %s: %v", name, err)), nil
		}
		result.Tables = append(result.Tables, schemaTable{
			Name:    name,
			Columns: cols,
			DDL:     ddlMap[name],
		})
	}

	b, err := json.Marshal(result)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
	}
	return mcpgo.NewToolResultText(string(b)), nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/mcp/... -run TestGetSchema`
Expected: PASS

- [ ] **Step 5: Commit**

`feat(mcp): add get_schema tool`

---

### Task 4: search_documents Tool (TDD)

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/server_test.go`

- [ ] **Step 1: Write test for search_documents**

Append to `server_test.go`:

```go
func TestSearchDocumentsTool(t *testing.T) {
	srv, store := newTestServer(t)

	// Seed a document
	doc := data.Document{
		Title:         "HVAC Manual",
		FileName:      "hvac.pdf",
		ExtractedText: "This is the furnace maintenance guide for heating systems.",
	}
	require.NoError(t, store.CreateDocument(&doc))

	result := callTool(t, srv, "search_documents", map[string]any{
		"query": "furnace",
	})
	assert.False(t, result.IsError)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/... -run TestSearchDocuments`
Expected: FAIL

- [ ] **Step 3: Add search_documents tool registration and handler**

In `tools.go`, add to `registerTools()`:

```go
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
```

Add handler:

```go
func (s *Server) handleSearchDocuments(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}

	results, err := s.store.SearchDocuments(query)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	if results == nil {
		results = []data.DocumentSearchResult{}
	}

	b, err := json.Marshal(results)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
	}
	return mcpgo.NewToolResultText(string(b)), nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/mcp/... -run TestSearchDocuments`
Expected: PASS

- [ ] **Step 5: Commit**

`feat(mcp): add search_documents tool`

---

### Task 5: get_maintenance_schedule Tool (TDD)

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/server_test.go`

- [ ] **Step 1: Write test for get_maintenance_schedule**

Append to `server_test.go`:

```go
func TestGetMaintenanceScheduleTool(t *testing.T) {
	srv, store := newTestServer(t)

	// Seed data: categories are seeded by AutoMigrate, retrieve them
	categories, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, categories, "AutoMigrate should seed maintenance categories")

	// Find the HVAC category (seeded by default)
	var hvacID string
	for _, cat := range categories {
		if cat.Name == "HVAC" {
			hvacID = cat.ID
			break
		}
	}
	require.NotEmpty(t, hvacID, "HVAC category should exist")

	past := time.Now().AddDate(0, -6, 0)
	item := data.MaintenanceItem{
		Name:           "Replace furnace filter",
		CategoryID:     hvacID,
		IntervalMonths: 3,
		LastServicedAt: &past,
	}
	require.NoError(t, store.CreateMaintenance(&item))

	result := callTool(t, srv, "get_maintenance_schedule", map[string]any{})
	assert.False(t, result.IsError)

	raw, err := json.Marshal(result.Content)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "Replace furnace filter")
	assert.Contains(t, string(raw), `"overdue":true`)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/... -run TestGetMaintenanceSchedule`
Expected: FAIL

- [ ] **Step 3: Add get_maintenance_schedule tool registration and handler**

In `tools.go`, add to `registerTools()`:

```go
s.mcpSrv.AddTool(
	mcpgo.NewTool("get_maintenance_schedule",
		mcpgo.WithDescription(
			"Get overdue and upcoming home maintenance items. Shows scheduled "+
				"maintenance with due dates, intervals, and overdue status. "+
				"Includes linked appliance names when available.",
		),
	),
	s.handleGetMaintenanceSchedule,
)
```

Add handler:

```go
type maintenanceScheduleItem struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Category       string     `json:"category"`
	Season         string     `json:"season"`
	LastServicedAt *time.Time `json:"last_serviced_at"`
	IntervalMonths int        `json:"interval_months"`
	DueDate        *time.Time `json:"due_date"`
	Overdue        bool       `json:"overdue"`
	ApplianceName  string     `json:"appliance_name,omitempty"`
}

func (s *Server) handleGetMaintenanceSchedule(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	items, err := s.store.ListMaintenanceWithSchedule()
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("list maintenance: %v", err)), nil
	}

	now := time.Now()
	results := make([]maintenanceScheduleItem, 0, len(items))
	for _, item := range items {
		due := item.DueDate
		if due == nil && item.LastServicedAt != nil && item.IntervalMonths > 0 {
			d := item.LastServicedAt.AddDate(0, item.IntervalMonths, 0)
			due = &d
		}

		overdue := due != nil && due.Before(now)

		var applianceName string
		if item.ApplianceID != nil {
			applianceName = item.Appliance.Name
		}

		results = append(results, maintenanceScheduleItem{
			ID:             item.ID,
			Name:           item.Name,
			Category:       item.Category.Name,
			Season:         item.Season,
			LastServicedAt: item.LastServicedAt,
			IntervalMonths: item.IntervalMonths,
			DueDate:        due,
			Overdue:        overdue,
			ApplianceName:  applianceName,
		})
	}

	b, err := json.Marshal(results)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
	}
	return mcpgo.NewToolResultText(string(b)), nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/mcp/... -run TestGetMaintenanceSchedule`
Expected: PASS

- [ ] **Step 5: Commit**

`feat(mcp): add get_maintenance_schedule tool`

---

### Task 6: get_house_profile Tool (TDD)

**Files:**
- Modify: `internal/mcp/tools.go`
- Modify: `internal/mcp/server_test.go`

- [ ] **Step 1: Write test for get_house_profile**

Append to `server_test.go`:

```go
func TestGetHouseProfileTool(t *testing.T) {
	srv, store := newTestServer(t)

	profile := data.HouseProfile{
		Nickname:     "Casa Cloud",
		AddressLine1: "123 Main St",
		City:         "Portland",
		State:        "OR",
		PostalCode:   "97201",
		YearBuilt:    1925,
		SquareFeet:   2400,
		Bedrooms:     3,
		Bathrooms:    2,
	}
	require.NoError(t, store.CreateHouseProfile(profile))

	result := callTool(t, srv, "get_house_profile", map[string]any{})
	assert.False(t, result.IsError)

	raw, err := json.Marshal(result.Content)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "Casa Cloud")
	assert.Contains(t, string(raw), "97201")
}

func TestGetHouseProfileToolEmpty(t *testing.T) {
	srv, _ := newTestServer(t)

	result := callTool(t, srv, "get_house_profile", map[string]any{})
	assert.True(t, result.IsError)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/... -run TestGetHouseProfile`
Expected: FAIL

- [ ] **Step 3: Add get_house_profile tool registration and handler**

In `tools.go`, add to `registerTools()`:

```go
s.mcpSrv.AddTool(
	mcpgo.NewTool("get_house_profile",
		mcpgo.WithDescription(
			"Get the house profile including address, property characteristics "+
				"(year built, square footage, bedrooms, bathrooms), construction "+
				"details (foundation, roof, wiring), and insurance/HOA information.",
		),
	),
	s.handleGetHouseProfile,
)
```

Add handler:

```go
func (s *Server) handleGetHouseProfile(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	profile, err := s.store.HouseProfile()
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("get house profile: %v", err)), nil
	}

	b, err := json.Marshal(profile)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
	}
	return mcpgo.NewToolResultText(string(b)), nil
}
```


- [ ] **Step 4: Run tests**

Run: `go test ./internal/mcp/... -run TestGetHouseProfile`
Expected: PASS

- [ ] **Step 5: Commit**

`feat(mcp): add get_house_profile tool`

---

### Task 7: Cobra Command and Wiring

**Files:**
- Create: `cmd/micasa/mcp.go`
- Modify: `cmd/micasa/main.go` (add to `root.AddCommand`, line ~75)

- [ ] **Step 1: Create cmd/micasa/mcp.go**

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "mcp [database-path]",
		Short:         "Run MCP server for LLM client access",
		Long:          "Start a Model Context Protocol server over stdio, exposing micasa data to LLM clients like Claude Desktop and Claude Code.",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, err := resolveMCPDBPath(args)
			if err != nil {
				return err
			}
			return runMCP(dbPath)
		},
	}
	return cmd
}

func resolveMCPDBPath(args []string) (string, error) {
	if len(args) > 0 {
		return data.ExpandHome(args[0]), nil
	}
	if envPath := os.Getenv("MICASA_DB_PATH"); envPath != "" {
		return data.ExpandHome(envPath), nil
	}
	return data.DefaultDBPath()
}

func runMCP(dbPath string) error {
	store, err := data.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	if err := store.SetQueryOnly(); err != nil {
		return fmt.Errorf("set query_only: %w", err)
	}

	ok, err := store.IsMicasaDB()
	if err != nil {
		return fmt.Errorf("validate database: %w", err)
	}
	if !ok {
		return fmt.Errorf(
			"not a micasa database: %s\n\nRun 'micasa' first to create and migrate the database, then point 'micasa mcp' at it.",
			dbPath,
		)
	}

	srv := mcp.NewServer(store)
	return srv.ServeStdio()
}
```

- [ ] **Step 2: Wire into root command**

In `cmd/micasa/main.go`, add `newMCPCmd()` to the `root.AddCommand(...)` call
(around line 75):

```go
root.AddCommand(
	newDemoCmd(),
	newBackupCmd(),
	newConfigCmd(),
	newCompletionCmd(root),
	newProCmd(),
	newMCPCmd(),
)
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./cmd/micasa/`

- [ ] **Step 4: Smoke test**

Run: `echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | go run ./cmd/micasa mcp :memory:`

This should fail with "not a micasa database" since `:memory:` is empty, which
confirms the validation works. Test with a real DB path if available.

- [ ] **Step 5: Commit**

`feat(mcp): add micasa mcp subcommand`

---

### Task 8: Full Integration Test

**Files:**
- Modify: `internal/mcp/server_test.go`

- [ ] **Step 1: Add round-trip integration test**

This test exercises the full MCP protocol flow through mcp-go's HandleMessage,
covering tool listing and calling all five tools:

```go
func TestListTools(t *testing.T) {
	srv, _ := newTestServer(t)
	mcpSrv := srv.MCPServer()
	ctx := context.Background()

	result := mcpSrv.HandleMessage(ctx, mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]any{},
	}))

	raw, err := json.Marshal(result)
	require.NoError(t, err)
	s := string(raw)

	assert.Contains(t, s, "query")
	assert.Contains(t, s, "get_schema")
	assert.Contains(t, s, "search_documents")
	assert.Contains(t, s, "get_maintenance_schedule")
	assert.Contains(t, s, "get_house_profile")
}
```

- [ ] **Step 2: Run full test suite**

Run: `go test ./internal/mcp/...`
Expected: ALL PASS

- [ ] **Step 3: Run full project test suite**

Run: `go test -shuffle=on ./...`
Expected: ALL PASS

- [ ] **Step 4: Run linter**

Run: `golangci-lint run ./...`
Expected: No warnings

- [ ] **Step 5: Commit**

`test(mcp): add tool listing integration test`

---

### Task 9: Update Vendor Hash and Final Checks

**Files:**
- Modify: `flake.nix` (vendor hash update after new dependency)

- [ ] **Step 1: Update vendor hash**

Use `/update-vendor-hash` skill to update the Nix vendor hash after adding
mcp-go to go.mod.

- [ ] **Step 2: Run pre-commit checks**

Use `/pre-commit-check` skill to verify everything passes.

- [ ] **Step 3: Final commit if needed**

Commit any remaining changes from vendor hash / formatting fixes.

---

## Next Steps (not part of this plan)

- Add more tools as LLM usage patterns emerge
- Write MCP client configuration docs for Claude Desktop / Claude Code
- Consider write tools behind confirmation flows
- Record a demo with `/record-demo`
