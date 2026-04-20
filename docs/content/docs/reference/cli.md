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

- [`micasa backup`](#micasa-backup) -- Back up the database to a file
- [`micasa config`](#micasa-config) -- Manage application configuration
- [`micasa db`](#micasa-db) -- Read and write entity data
- [`micasa demo`](#micasa-demo) -- Launch with sample data in an in-memory database
- [`micasa eval`](#micasa-eval) -- Run chat-quality benchmarks against a fixture or user DB
- [`micasa mcp`](#micasa-mcp) -- Run MCP server for LLM client access
- [`micasa pro`](#micasa-pro) -- Manage micasa Pro sync
- [`micasa query`](#micasa-query) -- Run a read-only SQL query
- [`micasa show`](#micasa-show) -- Display data as text or JSON
- [`micasa status`](#micasa-status) -- Show overdue items, open incidents, and active projects

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

## micasa db

Read and write entity data.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for db |

### Subcommands

- [`micasa db appliance`](#micasa-db-appliance) -- Manage appliances
- [`micasa db chat`](#micasa-db-chat) -- View and manage chat history
- [`micasa db deletion`](#micasa-db-deletion) -- View deletion audit records
- [`micasa db document`](#micasa-db-document) -- Manage documents
- [`micasa db house`](#micasa-db-house) -- Manage house profile
- [`micasa db incident`](#micasa-db-incident) -- Manage incidents
- [`micasa db maintenance`](#micasa-db-maintenance) -- Manage maintenance items
- [`micasa db maintenance-category`](#micasa-db-maintenance-category) -- Manage maintenance categorys
- [`micasa db project`](#micasa-db-project) -- Manage projects
- [`micasa db project-type`](#micasa-db-project-type) -- Manage project types
- [`micasa db quote`](#micasa-db-quote) -- Manage quotes
- [`micasa db service-log`](#micasa-db-service-log) -- Manage service log entrys
- [`micasa db vendor`](#micasa-db-vendor) -- Manage vendors

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa db appliance

Manage appliances.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for appliance |

### Subcommands

- [`micasa db appliance add`](#micasa-db-appliance-add) -- Add a appliance
- [`micasa db appliance delete`](#micasa-db-appliance-delete) -- Delete a appliance
- [`micasa db appliance edit`](#micasa-db-appliance-edit) -- Edit a appliance
- [`micasa db appliance get`](#micasa-db-appliance-get) -- Get a appliance by ID
- [`micasa db appliance list`](#micasa-db-appliance-list) -- List appliances
- [`micasa db appliance restore`](#micasa-db-appliance-restore) -- Restore a deleted appliance

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db appliance add

Add a appliance.

### Usage

```
micasa db appliance add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa db appliance`](#micasa-db-appliance) -- Manage appliances

## micasa db appliance delete

Delete a appliance.

### Usage

```
micasa db appliance delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa db appliance`](#micasa-db-appliance) -- Manage appliances

## micasa db appliance edit

Edit a appliance.

### Usage

```
micasa db appliance edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa db appliance`](#micasa-db-appliance) -- Manage appliances

## micasa db appliance get

Get a appliance by ID.

### Usage

```
micasa db appliance get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa db appliance`](#micasa-db-appliance) -- Manage appliances

## micasa db appliance list

List appliances.

### Usage

```
micasa db appliance list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db appliance`](#micasa-db-appliance) -- Manage appliances

## micasa db appliance restore

Restore a deleted appliance.

### Usage

```
micasa db appliance restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa db appliance`](#micasa-db-appliance) -- Manage appliances

## micasa db chat

View and manage chat history.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for chat |

### Subcommands

- [`micasa db chat delete`](#micasa-db-chat-delete) -- Delete a chat history entry
- [`micasa db chat list`](#micasa-db-chat-list) -- List chat history

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db chat delete

Delete a chat history entry.

### Usage

```
micasa db chat delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa db chat`](#micasa-db-chat) -- View and manage chat history

## micasa db chat list

List chat history.

### Usage

```
micasa db chat list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db chat`](#micasa-db-chat) -- View and manage chat history

## micasa db deletion

View deletion audit records.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for deletion |

### Subcommands

- [`micasa db deletion list`](#micasa-db-deletion-list) -- List deletion records

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db deletion list

List deletion records.

### Usage

```
micasa db deletion list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db deletion`](#micasa-db-deletion) -- View deletion audit records

## micasa db document

Manage documents.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for document |

### Subcommands

- [`micasa db document add`](#micasa-db-document-add) -- Add a document
- [`micasa db document delete`](#micasa-db-document-delete) -- Delete a document
- [`micasa db document edit`](#micasa-db-document-edit) -- Edit a document
- [`micasa db document get`](#micasa-db-document-get) -- Get a document by ID
- [`micasa db document list`](#micasa-db-document-list) -- List documents
- [`micasa db document restore`](#micasa-db-document-restore) -- Restore a deleted document

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db document add

Add a document.

### Usage

```
micasa db document add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `--file` | - | Path to file to upload |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa db document`](#micasa-db-document) -- Manage documents

## micasa db document delete

Delete a document.

### Usage

```
micasa db document delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa db document`](#micasa-db-document) -- Manage documents

## micasa db document edit

Edit a document.

### Usage

```
micasa db document edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa db document`](#micasa-db-document) -- Manage documents

## micasa db document get

Get a document by ID.

### Usage

```
micasa db document get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa db document`](#micasa-db-document) -- Manage documents

## micasa db document list

List documents.

### Usage

```
micasa db document list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db document`](#micasa-db-document) -- Manage documents

## micasa db document restore

Restore a deleted document.

### Usage

```
micasa db document restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa db document`](#micasa-db-document) -- Manage documents

## micasa db house

Manage house profile.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for house |

### Subcommands

- [`micasa db house add`](#micasa-db-house-add) -- Add house profile
- [`micasa db house edit`](#micasa-db-house-edit) -- Edit house profile
- [`micasa db house get`](#micasa-db-house-get) -- Get house profile

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db house add

Add house profile.

### Usage

```
micasa db house add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa db house`](#micasa-db-house) -- Manage house profile

## micasa db house edit

Edit house profile.

### Usage

```
micasa db house edit [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa db house`](#micasa-db-house) -- Manage house profile

## micasa db house get

Get house profile.

### Usage

```
micasa db house get [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |

### See also

- [`micasa db house`](#micasa-db-house) -- Manage house profile

## micasa db incident

Manage incidents.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for incident |

### Subcommands

- [`micasa db incident add`](#micasa-db-incident-add) -- Add a incident
- [`micasa db incident delete`](#micasa-db-incident-delete) -- Delete a incident
- [`micasa db incident edit`](#micasa-db-incident-edit) -- Edit a incident
- [`micasa db incident get`](#micasa-db-incident-get) -- Get a incident by ID
- [`micasa db incident list`](#micasa-db-incident-list) -- List incidents
- [`micasa db incident restore`](#micasa-db-incident-restore) -- Restore a deleted incident

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db incident add

Add a incident.

### Usage

```
micasa db incident add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa db incident`](#micasa-db-incident) -- Manage incidents

## micasa db incident delete

Delete a incident.

### Usage

```
micasa db incident delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa db incident`](#micasa-db-incident) -- Manage incidents

## micasa db incident edit

Edit a incident.

### Usage

```
micasa db incident edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa db incident`](#micasa-db-incident) -- Manage incidents

## micasa db incident get

Get a incident by ID.

### Usage

```
micasa db incident get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa db incident`](#micasa-db-incident) -- Manage incidents

## micasa db incident list

List incidents.

### Usage

```
micasa db incident list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db incident`](#micasa-db-incident) -- Manage incidents

## micasa db incident restore

Restore a deleted incident.

### Usage

```
micasa db incident restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa db incident`](#micasa-db-incident) -- Manage incidents

## micasa db maintenance

Manage maintenance items.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for maintenance |

### Subcommands

- [`micasa db maintenance add`](#micasa-db-maintenance-add) -- Add a maintenance item
- [`micasa db maintenance delete`](#micasa-db-maintenance-delete) -- Delete a maintenance item
- [`micasa db maintenance edit`](#micasa-db-maintenance-edit) -- Edit a maintenance item
- [`micasa db maintenance get`](#micasa-db-maintenance-get) -- Get a maintenance item by ID
- [`micasa db maintenance list`](#micasa-db-maintenance-list) -- List maintenance items
- [`micasa db maintenance restore`](#micasa-db-maintenance-restore) -- Restore a deleted maintenance item

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db maintenance add

Add a maintenance item.

### Usage

```
micasa db maintenance add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa db maintenance`](#micasa-db-maintenance) -- Manage maintenance items

## micasa db maintenance delete

Delete a maintenance item.

### Usage

```
micasa db maintenance delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa db maintenance`](#micasa-db-maintenance) -- Manage maintenance items

## micasa db maintenance edit

Edit a maintenance item.

### Usage

```
micasa db maintenance edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa db maintenance`](#micasa-db-maintenance) -- Manage maintenance items

## micasa db maintenance get

Get a maintenance item by ID.

### Usage

```
micasa db maintenance get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa db maintenance`](#micasa-db-maintenance) -- Manage maintenance items

## micasa db maintenance list

List maintenance items.

### Usage

```
micasa db maintenance list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db maintenance`](#micasa-db-maintenance) -- Manage maintenance items

## micasa db maintenance restore

Restore a deleted maintenance item.

### Usage

```
micasa db maintenance restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa db maintenance`](#micasa-db-maintenance) -- Manage maintenance items

## micasa db maintenance-category

Manage maintenance categorys.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for maintenance-category |

### Subcommands

- [`micasa db maintenance-category list`](#micasa-db-maintenance-category-list) -- List maintenance categorys

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db maintenance-category list

List maintenance categorys.

### Usage

```
micasa db maintenance-category list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db maintenance-category`](#micasa-db-maintenance-category) -- Manage maintenance categorys

## micasa db project

Manage projects.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for project |

### Subcommands

- [`micasa db project add`](#micasa-db-project-add) -- Add a project
- [`micasa db project delete`](#micasa-db-project-delete) -- Delete a project
- [`micasa db project edit`](#micasa-db-project-edit) -- Edit a project
- [`micasa db project get`](#micasa-db-project-get) -- Get a project by ID
- [`micasa db project list`](#micasa-db-project-list) -- List projects
- [`micasa db project restore`](#micasa-db-project-restore) -- Restore a deleted project

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db project add

Add a project.

### Usage

```
micasa db project add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa db project`](#micasa-db-project) -- Manage projects

## micasa db project delete

Delete a project.

### Usage

```
micasa db project delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa db project`](#micasa-db-project) -- Manage projects

## micasa db project edit

Edit a project.

### Usage

```
micasa db project edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa db project`](#micasa-db-project) -- Manage projects

## micasa db project get

Get a project by ID.

### Usage

```
micasa db project get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa db project`](#micasa-db-project) -- Manage projects

## micasa db project list

List projects.

### Usage

```
micasa db project list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db project`](#micasa-db-project) -- Manage projects

## micasa db project restore

Restore a deleted project.

### Usage

```
micasa db project restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa db project`](#micasa-db-project) -- Manage projects

## micasa db project-type

Manage project types.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for project-type |

### Subcommands

- [`micasa db project-type list`](#micasa-db-project-type-list) -- List project types

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db project-type list

List project types.

### Usage

```
micasa db project-type list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db project-type`](#micasa-db-project-type) -- Manage project types

## micasa db quote

Manage quotes.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for quote |

### Subcommands

- [`micasa db quote add`](#micasa-db-quote-add) -- Add a quote
- [`micasa db quote delete`](#micasa-db-quote-delete) -- Delete a quote
- [`micasa db quote edit`](#micasa-db-quote-edit) -- Edit a quote
- [`micasa db quote get`](#micasa-db-quote-get) -- Get a quote by ID
- [`micasa db quote list`](#micasa-db-quote-list) -- List quotes
- [`micasa db quote restore`](#micasa-db-quote-restore) -- Restore a deleted quote

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db quote add

Add a quote.

### Usage

```
micasa db quote add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa db quote`](#micasa-db-quote) -- Manage quotes

## micasa db quote delete

Delete a quote.

### Usage

```
micasa db quote delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa db quote`](#micasa-db-quote) -- Manage quotes

## micasa db quote edit

Edit a quote.

### Usage

```
micasa db quote edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa db quote`](#micasa-db-quote) -- Manage quotes

## micasa db quote get

Get a quote by ID.

### Usage

```
micasa db quote get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa db quote`](#micasa-db-quote) -- Manage quotes

## micasa db quote list

List quotes.

### Usage

```
micasa db quote list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db quote`](#micasa-db-quote) -- Manage quotes

## micasa db quote restore

Restore a deleted quote.

### Usage

```
micasa db quote restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa db quote`](#micasa-db-quote) -- Manage quotes

## micasa db service-log

Manage service log entrys.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for service-log |

### Subcommands

- [`micasa db service-log add`](#micasa-db-service-log-add) -- Add a service log entry
- [`micasa db service-log delete`](#micasa-db-service-log-delete) -- Delete a service log entry
- [`micasa db service-log edit`](#micasa-db-service-log-edit) -- Edit a service log entry
- [`micasa db service-log get`](#micasa-db-service-log-get) -- Get a service log entry by ID
- [`micasa db service-log list`](#micasa-db-service-log-list) -- List service log entrys
- [`micasa db service-log restore`](#micasa-db-service-log-restore) -- Restore a deleted service log entry

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db service-log add

Add a service log entry.

### Usage

```
micasa db service-log add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa db service-log`](#micasa-db-service-log) -- Manage service log entrys

## micasa db service-log delete

Delete a service log entry.

### Usage

```
micasa db service-log delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa db service-log`](#micasa-db-service-log) -- Manage service log entrys

## micasa db service-log edit

Edit a service log entry.

### Usage

```
micasa db service-log edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa db service-log`](#micasa-db-service-log) -- Manage service log entrys

## micasa db service-log get

Get a service log entry by ID.

### Usage

```
micasa db service-log get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa db service-log`](#micasa-db-service-log) -- Manage service log entrys

## micasa db service-log list

List service log entrys.

### Usage

```
micasa db service-log list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db service-log`](#micasa-db-service-log) -- Manage service log entrys

## micasa db service-log restore

Restore a deleted service log entry.

### Usage

```
micasa db service-log restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa db service-log`](#micasa-db-service-log) -- Manage service log entrys

## micasa db vendor

Manage vendors.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for vendor |

### Subcommands

- [`micasa db vendor add`](#micasa-db-vendor-add) -- Add a vendor
- [`micasa db vendor delete`](#micasa-db-vendor-delete) -- Delete a vendor
- [`micasa db vendor edit`](#micasa-db-vendor-edit) -- Edit a vendor
- [`micasa db vendor get`](#micasa-db-vendor-get) -- Get a vendor by ID
- [`micasa db vendor list`](#micasa-db-vendor-list) -- List vendors
- [`micasa db vendor restore`](#micasa-db-vendor-restore) -- Restore a deleted vendor

### See also

- [`micasa db`](#micasa-db) -- Read and write entity data

## micasa db vendor add

Add a vendor.

### Usage

```
micasa db vendor add [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with field values |
| `--data-file` | - | Path to JSON file with field values |
| `-h`, `--help` | - | help for add |

### See also

- [`micasa db vendor`](#micasa-db-vendor) -- Manage vendors

## micasa db vendor delete

Delete a vendor.

### Usage

```
micasa db vendor delete <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for delete |

### See also

- [`micasa db vendor`](#micasa-db-vendor) -- Manage vendors

## micasa db vendor edit

Edit a vendor.

### Usage

```
micasa db vendor edit <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--data` | - | JSON object with fields to update |
| `--data-file` | - | Path to JSON file with fields to update |
| `-h`, `--help` | - | help for edit |

### See also

- [`micasa db vendor`](#micasa-db-vendor) -- Manage vendors

## micasa db vendor get

Get a vendor by ID.

### Usage

```
micasa db vendor get <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for get |
| `--table` | - | Output as table |

### See also

- [`micasa db vendor`](#micasa-db-vendor) -- Manage vendors

## micasa db vendor list

List vendors.

### Usage

```
micasa db vendor list [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--deleted` | - | Include soft-deleted rows |
| `-h`, `--help` | - | help for list |
| `--table` | - | Output as table |

### See also

- [`micasa db vendor`](#micasa-db-vendor) -- Manage vendors

## micasa db vendor restore

Restore a deleted vendor.

### Usage

```
micasa db vendor restore <id> [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for restore |

### See also

- [`micasa db vendor`](#micasa-db-vendor) -- Manage vendors

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

## micasa eval

Parent command for chat-quality evaluations. See subcommands.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | - | help for eval |

### Subcommands

- [`micasa eval fts`](#micasa-eval-fts) -- Run the FTS context-enrichment chat benchmark

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

## micasa eval fts

Run the FTS chat benchmark against the default fixture DB or a
user-supplied SQLite file. Each question runs twice (FTS on and FTS off) and
is graded by a deterministic regex rubric, with an optional LLM judge pass.

The eval uses the chat config from the user's config file; --provider and
--model override specific fields. Pointing --db at a real micasa DB sends
prompts derived from household data to the configured provider -- if that
provider is a cloud service, the data leaves the machine.

### Usage

```
micasa eval fts [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--db` | - | path to a micasa SQLite DB (default: fixture) |
| `--format` | - | report format: table (default when TTY), markdown, or json |
| `-h`, `--help` | - | help for fts |
| `--judge-model` | - | model for the LLM judge (default: same as --model) |
| `--model` | - | override chat model from config |
| `--no-ab` | - | run each question once (FTS on) instead of twice |
| `--output` | - | write report to this file instead of stdout |
| `--provider` | - | override chat provider from config |
| `--questions` | `[]` | comma-separated names of questions to run (default: all) |
| `--skip-judge` | - | deterministic rubric only; skip the LLM judge |
| `--strict` | - | exit non-zero on per-question rubric regression (completed on both arms) |

### See also

- [`micasa eval`](#micasa-eval) -- Run chat-quality benchmarks against a fixture or user DB

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

## micasa status

Print items that need attention and exit with code 2 if any
are found. Exit 0 means everything is on track. Useful for cron jobs,
shell prompts, and status bar widgets.

### Usage

```
micasa status [database-path] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--days` | `30` | Look-ahead window for upcoming items (1-365) |
| `-h`, `--help` | - | help for status |
| `--json` | - | Output JSON instead of human-readable text |

### See also

- [`micasa`](#micasa) -- A terminal UI for tracking everything about your home

