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
| `-h`, `--help` | - | help for micasa |
| `--print-path` | - | Print the resolved database path and exit |
| `-v`, `--version` | - | version for micasa |

### Subcommands

- [`micasa appliance`](#micasa-appliance) -- Manage appliances
- [`micasa backup`](#micasa-backup) -- Back up the database to a file
- [`micasa config`](#micasa-config) -- Manage application configuration
- [`micasa demo`](#micasa-demo) -- Launch with sample data in an in-memory database
- [`micasa document`](#micasa-document) -- Manage documents
- [`micasa house`](#micasa-house) -- Manage house profile
- [`micasa incident`](#micasa-incident) -- Manage incidents
- [`micasa maintenance`](#micasa-maintenance) -- Manage maintenance items
- [`micasa maintenance-category`](#micasa-maintenance-category) -- Manage maintenance categorys
- [`micasa mcp`](#micasa-mcp) -- Run MCP server for LLM client access
- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync
- [`micasa project`](#micasa-project) -- Manage projects
- [`micasa project-type`](#micasa-project-type) -- Manage project types
- [`micasa query`](#micasa-query) -- Run a read-only SQL query
- [`micasa quote`](#micasa-quote) -- Manage quotes
- [`micasa service-log`](#micasa-service-log) -- Manage service log entrys
- [`micasa show`](#micasa-show) -- Display data as text or JSON
- [`micasa vendor`](#micasa-vendor) -- Manage vendors

## micasa appliance

Manage appliances.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for appliance |

### Subcommands

- [`micasa appliance add`](#micasa-appliance-add) -- Add a appliance
- [`micasa appliance delete`](#micasa-appliance-delete) -- Delete a appliance
- [`micasa appliance edit`](#micasa-appliance-edit) -- Edit a appliance
- [`micasa appliance get`](#micasa-appliance-get) -- Get a appliance by ID
- [`micasa appliance list`](#micasa-appliance-list) -- List appliances
- [`micasa appliance restore`](#micasa-appliance-restore) -- Restore a deleted appliance

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa appliance add

Add a appliance.

### Usage

```
micasa appliance add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa appliance`](#micasa-appliance) -- Manage appliances

## micasa appliance delete

Delete a appliance.

### Usage

```
micasa appliance delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa appliance`](#micasa-appliance) -- Manage appliances

## micasa appliance edit

Edit a appliance.

### Usage

```
micasa appliance edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa appliance`](#micasa-appliance) -- Manage appliances

## micasa appliance get

Get a appliance by ID.

### Usage

```
micasa appliance get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa appliance`](#micasa-appliance) -- Manage appliances

## micasa appliance list

List appliances.

### Usage

```
micasa appliance list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa appliance`](#micasa-appliance) -- Manage appliances

## micasa appliance restore

Restore a deleted appliance.

### Usage

```
micasa appliance restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa appliance`](#micasa-appliance) -- Manage appliances

## micasa backup

Back up the database to a file.

### Usage

```
micasa backup [destination] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for backup |
| `--source` | - | Source database path (default: standard location, honors MICASA_DB_PATH) |

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa config

Manage application configuration.

### Usage

```
micasa config [filter] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for config |

### Subcommands

- [`micasa config edit`](#micasa-config-edit) -- Open the config file in an editor
- [`micasa config get`](#micasa-config-get) -- Query config values with a jq filter (default: identity)

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa config edit

Open the config file in an editor.

### Usage

```
micasa config edit [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa config`](#micasa-config) -- Manage application configuration

## micasa config get

Query config values with a jq filter (default: identity).

### Usage

```
micasa config get [filter] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |

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
| `-h`, `--help` | - | help for demo |
| `--seed-only` | - | Seed data and exit without launching the TUI |
| `--years` | `0` | Generate N years of simulated home ownership data |

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa document

Manage documents.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for document |

### Subcommands

- [`micasa document add`](#micasa-document-add) -- Add a document
- [`micasa document delete`](#micasa-document-delete) -- Delete a document
- [`micasa document edit`](#micasa-document-edit) -- Edit a document
- [`micasa document get`](#micasa-document-get) -- Get a document by ID
- [`micasa document list`](#micasa-document-list) -- List documents
- [`micasa document restore`](#micasa-document-restore) -- Restore a deleted document

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa document add

Add a document.

### Usage

```
micasa document add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `--file` | - | Path to file to upload |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa document`](#micasa-document) -- Manage documents

## micasa document delete

Delete a document.

### Usage

```
micasa document delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa document`](#micasa-document) -- Manage documents

## micasa document edit

Edit a document.

### Usage

```
micasa document edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa document`](#micasa-document) -- Manage documents

## micasa document get

Get a document by ID.

### Usage

```
micasa document get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa document`](#micasa-document) -- Manage documents

## micasa document list

List documents.

### Usage

```
micasa document list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa document`](#micasa-document) -- Manage documents

## micasa document restore

Restore a deleted document.

### Usage

```
micasa document restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa document`](#micasa-document) -- Manage documents

## micasa house

Manage house profile.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for house |

### Subcommands

- [`micasa house add`](#micasa-house-add) -- Add house profile
- [`micasa house edit`](#micasa-house-edit) -- Edit house profile
- [`micasa house get`](#micasa-house-get) -- Get house profile

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa house add

Add house profile.

### Usage

```
micasa house add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa house`](#micasa-house) -- Manage house profile

## micasa house edit

Edit house profile.

### Usage

```
micasa house edit [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa house`](#micasa-house) -- Manage house profile

## micasa house get

Get house profile.

### Usage

```
micasa house get [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |

### See also

- [`micasa house`](#micasa-house) -- Manage house profile

## micasa incident

Manage incidents.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for incident |

### Subcommands

- [`micasa incident add`](#micasa-incident-add) -- Add a incident
- [`micasa incident delete`](#micasa-incident-delete) -- Delete a incident
- [`micasa incident edit`](#micasa-incident-edit) -- Edit a incident
- [`micasa incident get`](#micasa-incident-get) -- Get a incident by ID
- [`micasa incident list`](#micasa-incident-list) -- List incidents
- [`micasa incident restore`](#micasa-incident-restore) -- Restore a deleted incident

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa incident add

Add a incident.

### Usage

```
micasa incident add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa incident`](#micasa-incident) -- Manage incidents

## micasa incident delete

Delete a incident.

### Usage

```
micasa incident delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa incident`](#micasa-incident) -- Manage incidents

## micasa incident edit

Edit a incident.

### Usage

```
micasa incident edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa incident`](#micasa-incident) -- Manage incidents

## micasa incident get

Get a incident by ID.

### Usage

```
micasa incident get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa incident`](#micasa-incident) -- Manage incidents

## micasa incident list

List incidents.

### Usage

```
micasa incident list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa incident`](#micasa-incident) -- Manage incidents

## micasa incident restore

Restore a deleted incident.

### Usage

```
micasa incident restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa incident`](#micasa-incident) -- Manage incidents

## micasa maintenance

Manage maintenance items.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for maintenance |

### Subcommands

- [`micasa maintenance add`](#micasa-maintenance-add) -- Add a maintenance item
- [`micasa maintenance delete`](#micasa-maintenance-delete) -- Delete a maintenance item
- [`micasa maintenance edit`](#micasa-maintenance-edit) -- Edit a maintenance item
- [`micasa maintenance get`](#micasa-maintenance-get) -- Get a maintenance item by ID
- [`micasa maintenance list`](#micasa-maintenance-list) -- List maintenance items
- [`micasa maintenance restore`](#micasa-maintenance-restore) -- Restore a deleted maintenance item

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa maintenance add

Add a maintenance item.

### Usage

```
micasa maintenance add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa maintenance`](#micasa-maintenance) -- Manage maintenance items

## micasa maintenance delete

Delete a maintenance item.

### Usage

```
micasa maintenance delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa maintenance`](#micasa-maintenance) -- Manage maintenance items

## micasa maintenance edit

Edit a maintenance item.

### Usage

```
micasa maintenance edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa maintenance`](#micasa-maintenance) -- Manage maintenance items

## micasa maintenance get

Get a maintenance item by ID.

### Usage

```
micasa maintenance get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa maintenance`](#micasa-maintenance) -- Manage maintenance items

## micasa maintenance list

List maintenance items.

### Usage

```
micasa maintenance list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa maintenance`](#micasa-maintenance) -- Manage maintenance items

## micasa maintenance restore

Restore a deleted maintenance item.

### Usage

```
micasa maintenance restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa maintenance`](#micasa-maintenance) -- Manage maintenance items

## micasa maintenance-category

Manage maintenance categorys.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for maintenance-category |

### Subcommands

- [`micasa maintenance-category list`](#micasa-maintenance-category-list) -- List maintenance categorys

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa maintenance-category list

List maintenance categorys.

### Usage

```
micasa maintenance-category list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa maintenance-category`](#micasa-maintenance-category) -- Manage maintenance categorys

## micasa mcp

Start a Model Context Protocol server over stdio, exposing micasa data to LLM clients like Claude Desktop and Claude Code.

### Usage

```
micasa mcp [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for mcp |

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

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for pro |

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
micasa pro conflicts [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for conflicts |

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro devices

List devices.

### Usage

```
micasa pro devices [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for devices |

### Subcommands

- [`micasa pro devices revoke`](#micasa-pro-devices-revoke) -- Revoke a device

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro devices revoke

Revoke a device.

### Usage

```
micasa pro devices revoke <device-id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for revoke |

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
| `-h`, `--help` | - | help for init |
| `--relay-url` | `https://relay.micasa.dev` | Relay server URL (honors MICASA_RELAY_URL) |

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro invite

Generate invite code, wait for joiner handshake.

### Usage

```
micasa pro invite [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for invite |

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
| `-h`, `--help` | - | help for join |
| `--relay-url` | `https://relay.micasa.dev` | Relay server URL (honors MICASA_RELAY_URL) |

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro status

Show sync status.

### Usage

```
micasa pro status [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for status |

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro storage

Show blob storage usage.

### Usage

```
micasa pro storage [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for storage |

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa pro sync

Force immediate push+pull cycle.

### Usage

```
micasa pro sync [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for sync |

### See also

- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync

## micasa project

Manage projects.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for project |

### Subcommands

- [`micasa project add`](#micasa-project-add) -- Add a project
- [`micasa project delete`](#micasa-project-delete) -- Delete a project
- [`micasa project edit`](#micasa-project-edit) -- Edit a project
- [`micasa project get`](#micasa-project-get) -- Get a project by ID
- [`micasa project list`](#micasa-project-list) -- List projects
- [`micasa project restore`](#micasa-project-restore) -- Restore a deleted project

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa project add

Add a project.

### Usage

```
micasa project add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa project`](#micasa-project) -- Manage projects

## micasa project delete

Delete a project.

### Usage

```
micasa project delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa project`](#micasa-project) -- Manage projects

## micasa project edit

Edit a project.

### Usage

```
micasa project edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa project`](#micasa-project) -- Manage projects

## micasa project get

Get a project by ID.

### Usage

```
micasa project get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa project`](#micasa-project) -- Manage projects

## micasa project list

List projects.

### Usage

```
micasa project list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa project`](#micasa-project) -- Manage projects

## micasa project restore

Restore a deleted project.

### Usage

```
micasa project restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa project`](#micasa-project) -- Manage projects

## micasa project-type

Manage project types.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for project-type |

### Subcommands

- [`micasa project-type list`](#micasa-project-type-list) -- List project types

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa project-type list

List project types.

### Usage

```
micasa project-type list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa project-type`](#micasa-project-type) -- Manage project types

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
| `-h`, `--help` | - | help for query |
| `--json` | - | Output as JSON |

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa quote

Manage quotes.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for quote |

### Subcommands

- [`micasa quote add`](#micasa-quote-add) -- Add a quote
- [`micasa quote delete`](#micasa-quote-delete) -- Delete a quote
- [`micasa quote edit`](#micasa-quote-edit) -- Edit a quote
- [`micasa quote get`](#micasa-quote-get) -- Get a quote by ID
- [`micasa quote list`](#micasa-quote-list) -- List quotes
- [`micasa quote restore`](#micasa-quote-restore) -- Restore a deleted quote

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa quote add

Add a quote.

### Usage

```
micasa quote add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa quote`](#micasa-quote) -- Manage quotes

## micasa quote delete

Delete a quote.

### Usage

```
micasa quote delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa quote`](#micasa-quote) -- Manage quotes

## micasa quote edit

Edit a quote.

### Usage

```
micasa quote edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa quote`](#micasa-quote) -- Manage quotes

## micasa quote get

Get a quote by ID.

### Usage

```
micasa quote get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa quote`](#micasa-quote) -- Manage quotes

## micasa quote list

List quotes.

### Usage

```
micasa quote list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa quote`](#micasa-quote) -- Manage quotes

## micasa quote restore

Restore a deleted quote.

### Usage

```
micasa quote restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa quote`](#micasa-quote) -- Manage quotes

## micasa service-log

Manage service log entrys.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for service-log |

### Subcommands

- [`micasa service-log add`](#micasa-service-log-add) -- Add a service log entry
- [`micasa service-log delete`](#micasa-service-log-delete) -- Delete a service log entry
- [`micasa service-log edit`](#micasa-service-log-edit) -- Edit a service log entry
- [`micasa service-log get`](#micasa-service-log-get) -- Get a service log entry by ID
- [`micasa service-log list`](#micasa-service-log-list) -- List service log entrys
- [`micasa service-log restore`](#micasa-service-log-restore) -- Restore a deleted service log entry

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa service-log add

Add a service log entry.

### Usage

```
micasa service-log add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa service-log`](#micasa-service-log) -- Manage service log entrys

## micasa service-log delete

Delete a service log entry.

### Usage

```
micasa service-log delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa service-log`](#micasa-service-log) -- Manage service log entrys

## micasa service-log edit

Edit a service log entry.

### Usage

```
micasa service-log edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa service-log`](#micasa-service-log) -- Manage service log entrys

## micasa service-log get

Get a service log entry by ID.

### Usage

```
micasa service-log get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa service-log`](#micasa-service-log) -- Manage service log entrys

## micasa service-log list

List service log entrys.

### Usage

```
micasa service-log list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa service-log`](#micasa-service-log) -- Manage service log entrys

## micasa service-log restore

Restore a deleted service log entry.

### Usage

```
micasa service-log restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa service-log`](#micasa-service-log) -- Manage service log entrys

## micasa show

Print entity data to stdout. Entities: house, projects, project-types,
quotes, vendors, maintenance, maintenance-categories, service-log,
appliances, incidents, documents, all.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for show |
| `--json` | - | Output as JSON |

### Subcommands

- [`micasa show all`](#micasa-show-all) -- Show all entities

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa show all

Show all entities.

### Usage

```
micasa show all [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for all |

### Inherited flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `--json` | - | Output as JSON |

### See also

- [`micasa show`](#micasa-show) -- Display data as text or JSON

## micasa vendor

Manage vendors.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for vendor |

### Subcommands

- [`micasa vendor add`](#micasa-vendor-add) -- Add a vendor
- [`micasa vendor delete`](#micasa-vendor-delete) -- Delete a vendor
- [`micasa vendor edit`](#micasa-vendor-edit) -- Edit a vendor
- [`micasa vendor get`](#micasa-vendor-get) -- Get a vendor by ID
- [`micasa vendor list`](#micasa-vendor-list) -- List vendors
- [`micasa vendor restore`](#micasa-vendor-restore) -- Restore a deleted vendor

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa vendor add

Add a vendor.

### Usage

```
micasa vendor add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa vendor`](#micasa-vendor) -- Manage vendors

## micasa vendor delete

Delete a vendor.

### Usage

```
micasa vendor delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa vendor`](#micasa-vendor) -- Manage vendors

## micasa vendor edit

Edit a vendor.

### Usage

```
micasa vendor edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa vendor`](#micasa-vendor) -- Manage vendors

## micasa vendor get

Get a vendor by ID.

### Usage

```
micasa vendor get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa vendor`](#micasa-vendor) -- Manage vendors

## micasa vendor list

List vendors.

### Usage

```
micasa vendor list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa vendor`](#micasa-vendor) -- Manage vendors

## micasa vendor restore

Restore a deleted vendor.

### Usage

```
micasa vendor restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa vendor`](#micasa-vendor) -- Manage vendors

