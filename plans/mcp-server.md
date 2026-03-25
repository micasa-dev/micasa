<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# MCP Server Design

<!-- verified: 2026-03-25 -->

GitHub issue: [#814](https://github.com/micasa-dev/micasa/issues/814)

## Summary

Add a `micasa mcp` subcommand that exposes micasa's data to MCP-compatible LLM
clients (Claude Desktop, Claude Code, Cursor) over stdio transport. Read-only,
tools-only, minimal tool set.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Integration | Subcommand (`micasa mcp`) | Reuses all existing code, single binary |
| Library | `github.com/mark3labs/mcp-go` | Most popular Go MCP lib, clean API, stdio built-in. Audit before adding. |
| Scope | Read-only | Safer starting point; write ops can be added later |
| Surface | Tools only, no resources | Simpler; not all clients handle resources well |
| DB path | Same resolution as main app | CLI arg > `MICASA_DB_PATH` env > XDG default |
| Architecture | Thin wrapper over Store | Store methods are already well-typed; no service layer needed |
| Tool set | Minimal (5 tools) | Start small, promote patterns to dedicated tools when observed |

## Command

```
micasa mcp [database-path]
```

Opens an existing, migrated micasa database. Sets `PRAGMA query_only = ON`
after opening to enforce read-only access at the SQLite level. Validates the
database with `store.IsMicasaDB()` at startup; exits with an actionable error
if the schema is missing or invalid. Speaks MCP JSON-RPC over stdin/stdout. No
TUI, no config loading beyond the DB path.

Client configuration (e.g. Claude Desktop):

```json
{
  "mcpServers": {
    "micasa": {
      "command": "micasa",
      "args": ["mcp"]
    }
  }
}
```

## Tools

### `query`

Execute read-only SQL against the micasa database. Uses the existing
`ReadOnlyQuery` with its defense-in-depth: SELECT/WITH prefix check, no
semicolons, keyword blocklist, EXPLAIN opcode validation, 10-second timeout,
200-row cap.

**Parameters:** `sql` (string, required)
**Returns:** `{ columns: []string, rows: [][]string }`

### `get_schema`

Returns database schema: table names, column definitions (name, type, nullable,
PK), and DDL statements. Uses existing `TableNames()`, `TableColumns()`,
`TableDDL()` Store methods. Gives LLMs enough context to write SQL for the
`query` tool.

**Parameters:** `tables` ([]string, optional -- filter to specific tables; returns all when empty)
**Returns:** `{ tables: [{ name, columns: [{ name, type, not_null, pk }], ddl }] }`

The handler loops over requested (or all) tables, calling `TableColumns` for
each.

### `search_documents`

Full-text search over documents using FTS5 (Porter stemmer over unicode61
tokenizer). Searches title, notes, and extracted text fields. Simple queries
get automatic prefix matching (`*` appended by `prepareFTSQuery`); FTS
operators pass through unmodified. Returns BM25-ranked results with contextual
snippets. Max 50 results.

**Parameters:** `query` (string, required)
**Returns:** `[{ id, title, file_name, entity_kind, entity_id, snippet, updated_at }]`

### `get_maintenance_schedule`

Returns overdue and upcoming maintenance items with schedule information. Uses
`ListMaintenanceWithSchedule()`. The tool handler computes `overdue` by
comparing `DueDate` (or `LastServicedAt + IntervalMonths` when `DueDate` is
unset) against the current time, and extracts `appliance_name` from the
preloaded `Appliance.Name` relation.

**Parameters:** none
**Returns:** `[{ id, name, category, season, last_serviced_at, interval_months, due_date, overdue, appliance_name }]`

### `get_house_profile`

Returns the house profile with address, property details, and characteristics.

**Parameters:** none
**Returns:** `{ nickname, address_line1, address_line2, city, state, postal_code, year_built, sqft, beds, baths, ... }`

## Package Structure

```
internal/mcp/
  server.go      -- Server struct, tool registration, stdio runner
  tools.go       -- Tool handler methods (one per tool)
  tools_test.go  -- Tests for each tool handler

cmd/micasa/
  mcp.go         -- Cobra command wiring
```

### Server struct

```go
type Server struct {
    store *data.Store
}
```

Each tool handler is a method on Server that unpacks arguments, calls the
Store, and returns a result struct. mcp-go handles JSON marshaling and the
stdio transport.

## Initialization Flow

1. Cobra command resolves DB path (arg > env > XDG default)
2. `data.Open(dbPath)` opens the Store
3. Set `PRAGMA query_only = ON` to enforce read-only at the SQLite level
4. `store.IsMicasaDB()` validates schema; exit with actionable error if invalid
5. `mcp.NewServer(store)` creates the MCP server, registers all 5 tools
6. `server.ServeStdio()` starts the JSON-RPC stdio transport
7. On context cancellation or stdin EOF, clean shutdown and `store.Close()`

## Error Handling

Store errors propagate as MCP tool errors. The LLM sees the error message and
can retry or adjust its query. No silent failures.

## Testing

Integration tests exercise full MCP round-trips: send a JSON-RPC tool call
through mcp-go's in-process transport (or stdio with a pipe) and assert on the
JSON response. This is the primary test surface -- it validates tool
registration, argument parsing, and response serialization end-to-end.

Supplementary unit tests call tool handler methods directly with a seeded temp
SQLite DB for faster iteration on edge cases (empty results, invalid SQL,
schema filtering).

## Dependency

`github.com/mark3labs/mcp-go` — audit source before adding. Pin to latest
stable release. This is the only new dependency.

## Cobra Wiring

`newMCPCmd()` in `cmd/micasa/mcp.go` must be added to the `root.AddCommand()`
call in `newRootCmd()` in `cmd/micasa/main.go`.

## Non-Goals

- Write operations (create/update/delete entities)
- MCP resources (browsable data)
- Vector search / embeddings (deferred, see #722)
- Authentication / multi-user access control
- HTTP/SSE transport (stdio only)

## Future Expansion

When LLM usage patterns emerge, promote frequently-used SQL patterns to
dedicated tools (e.g. `list_vendors`, `get_spending_summary`). Add write tools
behind confirmation flows when there's demand.
