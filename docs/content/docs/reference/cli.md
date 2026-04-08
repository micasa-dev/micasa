+++
title = "CLI Reference"
linkTitle = "CLI"
weight = 3
description = "Auto-generated reference for every micasa command, flag, and subcommand."
+++

<!-- AUTO-GENERATED. Do not edit by hand. -->
<!-- Source: cobra command tree in cmd/micasa. -->
<!-- Regenerate: `go generate ./cmd/micasa/` -->

A terminal UI for tracking everything about your home.

## micasa

A terminal UI for tracking everything about your home.

### Usage

```
micasa [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--print-path` | - | Print the resolved database path and exit |

### Subcommands

- [`micasa backup`](#micasa-backup) -- Back up the database to a file
- [`micasa config`](#micasa-config) -- Manage application configuration
- [`micasa demo`](#micasa-demo) -- Launch with sample data in an in-memory database
- [`micasa mcp`](#micasa-mcp) -- Run MCP server for LLM client access
- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync
- [`micasa query`](#micasa-query) -- Run a read-only SQL query
- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa backup

Back up the database to a file.

### Usage

```
micasa backup [destination] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--source` | - | Source database path (default: standard location, honors MICASA_DB_PATH) |

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa config

Manage application configuration.

### Usage

```
micasa config [filter]
```

### Subcommands

- [`micasa config edit`](#micasa-config-edit) -- Open the config file in an editor
- [`micasa config get`](#micasa-config-get) -- Query config values with a jq filter (default: identity)

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa config edit

Open the config file in an editor.

### Usage

```
micasa config edit
```

### See also

- [`micasa config`](#micasa-config) -- Manage application configuration

## micasa config get

Query config values with a jq filter (default: identity).

### Usage

```
micasa config get [filter]
```

### See also

- [`micasa config`](#micasa-config) -- Manage application configuration

## micasa demo

Launch with fictitious sample data. Without a path argument, uses an in-memory database.

### Usage

```
micasa demo [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--seed-only` | - | Seed data and exit without launching the TUI |
| `--years` | `0` | Generate N years of simulated home ownership data |

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa mcp

Start a Model Context Protocol server over stdio, exposing micasa data to LLM clients like Claude Desktop and Claude Code.

### Usage

```
micasa mcp [database-path]
```

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa pro

Encrypted multi-device sync for your household data.

Typical workflow:
  1. First device:  micasa pro init
  2. First device:  micasa pro invite    (prints a one-time code)
  3. Second device: micasa pro join &lt;code&gt;
  4. Either device: micasa pro sync      (push and pull changes)

### Examples

```
  micasa pro init
  micasa pro invite
  micasa pro join 01JQ7X2K.abc123
  micasa pro sync
  micasa pro status
```

### Subcommands

- [`micasa pro conflicts`](#micasa-pro-conflicts) -- List sync ops that lost LWW conflict resolution
- [`micasa pro devices`](#micasa-pro-devices) -- List devices
- [`micasa pro init`](#micasa-pro-init) -- Bootstrap: create household, generate keys, register device
- [`micasa pro invite`](#micasa-pro-invite) -- Generate invite code, wait for joiner handshake
- [`micasa pro join`](#micasa-pro-join) -- Join household with invite code
- [`micasa pro status`](#micasa-pro-status) -- Show sync status
- [`micasa pro storage`](#micasa-pro-storage) -- Show blob storage usage
- [`micasa pro sync`](#micasa-pro-sync) -- Force immediate push+pull cycle

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa pro conflicts

List sync ops that lost LWW conflict resolution.

### Usage

```
micasa pro conflicts [database-path]
```

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro devices

List devices.

### Usage

```
micasa pro devices [database-path]
```

### Subcommands

- [`micasa pro devices revoke`](#micasa-pro-devices-revoke) -- Revoke a device

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro devices revoke

Revoke a device.

### Usage

```
micasa pro devices revoke <device-id> [database-path]
```

### See also

- [`micasa pro devices`](#micasa-pro-devices) -- List devices

## micasa pro init

Bootstrap: create household, generate keys, register device.

### Usage

```
micasa pro init [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--relay-url` | `https://relay.micasa.dev` | Relay server URL (honors MICASA_RELAY_URL) |

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro invite

Generate invite code, wait for joiner handshake.

### Usage

```
micasa pro invite [database-path]
```

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro join

Join household with invite code.

### Usage

```
micasa pro join <code> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--relay-url` | `https://relay.micasa.dev` | Relay server URL (honors MICASA_RELAY_URL) |

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro status

Show sync status.

### Usage

```
micasa pro status [database-path]
```

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro storage

Show blob storage usage.

### Usage

```
micasa pro storage [database-path]
```

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro sync

Force immediate push+pull cycle.

### Usage

```
micasa pro sync [database-path]
```

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa query

Execute a validated SELECT query against the database.
Only SELECT/WITH statements are allowed. Results are capped at 200 rows
with a 10-second timeout.

### Usage

```
micasa query <sql> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | - | Output as JSON |

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa show

Print entity data to stdout. Entities: house, projects, project-types,
quotes, vendors, maintenance, maintenance-categories, service-log,
appliances, incidents, documents, all.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### Subcommands

- [`micasa show all`](#micasa-show-all) -- Show all entities
- [`micasa show appliances`](#micasa-show-appliances) -- Show appliances
- [`micasa show documents`](#micasa-show-documents) -- Show documents
- [`micasa show house`](#micasa-show-house) -- Show house profile
- [`micasa show incidents`](#micasa-show-incidents) -- Show incidents
- [`micasa show maintenance`](#micasa-show-maintenance) -- Show maintenance items
- [`micasa show maintenance-categories`](#micasa-show-maintenance-categories) -- Show maintenance categories
- [`micasa show project-types`](#micasa-show-project-types) -- Show project types
- [`micasa show projects`](#micasa-show-projects) -- Show projects
- [`micasa show quotes`](#micasa-show-quotes) -- Show quotes
- [`micasa show service-log`](#micasa-show-service-log) -- Show service log entries
- [`micasa show vendors`](#micasa-show-vendors) -- Show vendors

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa show all

Show all entities.

### Usage

```
micasa show all [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show appliances

Show appliances.

### Usage

```
micasa show appliances [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show documents

Show documents.

### Usage

```
micasa show documents [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show house

Show house profile.

### Usage

```
micasa show house [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show incidents

Show incidents.

### Usage

```
micasa show incidents [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show maintenance

Show maintenance items.

### Usage

```
micasa show maintenance [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show maintenance-categories

Show maintenance categories.

### Usage

```
micasa show maintenance-categories [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show project-types

Show project types.

### Usage

```
micasa show project-types [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show projects

Show projects.

### Usage

```
micasa show projects [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show quotes

Show quotes.

### Usage

```
micasa show quotes [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show service-log

Show service log entries.

### Usage

```
micasa show service-log [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa show vendors

Show vendors.

### Usage

```
micasa show vendors [database-path]
```

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

