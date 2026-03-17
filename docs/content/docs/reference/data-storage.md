+++
title = "Data Storage"
weight = 3
description = "SQLite database file, schema, backup, and portability."
linkTitle = "Data Storage"
+++

micasa stores everything in a single SQLite database file. This page covers
how data is stored and how to manage it.

## Database file

By default, the database lives in your platform's data directory:

| Platform | Default path |
|----------|-------------|
| Linux    | `~/.local/share/micasa/micasa.db` |
| macOS    | `~/Library/Application Support/micasa/micasa.db` |
| Windows  | `%LOCALAPPDATA%\micasa\micasa.db` |

See [Configuration]({{< ref "/docs/reference/configuration" >}}) for how to customize the path.

The database path is shown in the tab row (right-aligned) so you always know
which file you're working with.

## Schema

micasa uses [GORM](https://gorm.io) for database access. The database is
created on first run, and new tables or columns are added automatically on
startup. See [Upgrades](#upgrades) for what this covers and what it doesn't.

### Tables

| Table                  | Description |
|------------------------|-------------|
| `house_profiles`       | Single row with your home's details |
| `projects`             | Home improvement projects |
| `project_types`        | Pre-seeded project categories |
| `quotes`               | Vendor quotes linked to projects |
| `vendors`              | Shared vendor records |
| `maintenance_items`    | Recurring maintenance tasks |
| `maintenance_categories` | Pre-seeded maintenance categories |
| `incidents`            | Household issues and repairs |
| `appliances`           | Physical equipment |
| `service_log_entries`  | Service history per maintenance item |
| `documents`            | File metadata + attachments linked to records |
| `documents_fts`        | FTS5 full-text search index for documents (virtual table) |
| `deletion_records`     | Audit trail for soft deletes/restores |

### Pre-seeded data

On first run, micasa seeds default **project types** (Renovation, Repair,
Landscaping, etc.) and **maintenance categories** (HVAC, Plumbing, Electrical,
etc.). These are reference data used in select dropdowns.

## Backup

Use the built-in `backup` command to create a consistent snapshot:

```sh
micasa backup ~/backups/micasa-$(date +%F).db
```

This uses SQLite's [Online Backup API](https://www.sqlite.org/backup.html)
to produce a safe copy even while micasa is running. To back up a database
at a non-default path, pass `--source`:

```sh
micasa backup --source /path/to/micasa.db ~/backups/micasa-$(date +%F).db
```

## Soft delete

micasa uses GORM's soft delete feature. When you delete an item, it sets a
`deleted_at` timestamp rather than removing the row. This means:

- Deleted items can be restored (press <kbd>d</kbd> on a deleted item in Edit mode)
- The `deletion_records` table tracks when items were deleted and restored
- Toggle <kbd>x</kbd> in Edit mode to show/hide deleted items
- Soft deletions persist across runs -- quit and reopen, and your deleted items
  are still hidden (but restorable). Nothing is ever permanently lost unless
  you edit the database file directly

### Referential integrity guards

Soft deletion respects foreign key relationships:

- **Delete guards**: you cannot delete a parent record that has active
  children. For example, deleting a project that still has quotes is refused
  with an actionable error message ("delete its quotes first").
- **Restore guards**: you cannot restore a child record whose parent is
  deleted. For example, restoring a quote whose project is deleted is refused
  ("restore the project first").
- These guards apply at every FK level: projects/quotes,
  maintenance/service logs, appliances/maintenance, and appliances/incidents
  (including nullable links where a value was set).

## Portability

The database is a standard SQLite file. You can:

- Open it with any SQLite client (`sqlite3`, DB Browser for SQLite, etc.)
- Move it between machines by copying the file
- Query it directly with SQL if needed

The file uses a pure-Go SQLite driver (no CGO), so the binary has zero
native dependencies.

## Upgrades

micasa does not yet have a schema migration system. New columns and tables
added in future versions will appear automatically when you upgrade (GORM's
`AutoMigrate` handles that), but some kinds of schema changes are not
supported by this mechanism:

- **Renaming** a column or table
- **Dropping** a column that's no longer used
- **Changing** a column's type or nullability
- **Modifying** foreign key constraints

If a future release requires one of these changes, we'll ship a migration
tool or document the manual steps. Until then, upgrading micasa will never
lose your data -- the worst case is a column that sticks around after it
stops being used.

### What you should do

Back up before upgrading:

```sh
micasa backup ~/backups/micasa-$(date +%F).db
```

If an upgrade goes wrong, restore from the copy and pin the previous version
until a fix is released.

### What we'll do

If a future major version (2.0) requires breaking schema changes, it will
include a migration tool that handles the upgrade automatically. The goal:
you upgrade the binary, launch it, and everything just works -- or you get
a clear error telling you what to do.

## A note on scale

micasa manages one house. Yours. The expected dataset is something like 1 house
profile, a few dozen projects, a handful of appliances, and maybe a few hundred
maintenance entries if you've been religiously logging every generator spark
plug, oil change, and leaf you've raked since the Clinton administration. The entire database will comfortably
fit on a floppy disk, assuming you can find one. Attach enough PDFs of
appliance manuals and warranty certificates and you might graduate to two
floppy disks. Exciting times.

There is no sharding strategy. There is no read replica. The query planner's
hardest decision is whether to use the index. We have not load-tested the
application to determine peak concurrent homeowner throughput, but we're
confident the answer is "one" and the bottleneck is "the homeowner going to get
a snack."

SQLite caps out at 281 terabytes. If your maintenance records are approaching
even a fraction of that, you do not have a database problem -- you have a house
problem. That is less "home maintenance" and more "the Winchester Mystery House
is my primary residence and I am writing down every nail."

If you're managing enough houses to worry about database performance, you're a
property management company, and you should probably use property management
software.

## LLM data exposure

If you enable the optional [LLM chat]({{< ref "/docs/guide/llm-chat" >}}),
micasa sends database contents to the configured LLM endpoint. By default this
is localhost (Ollama), so data stays on your machine. If you configure a remote
endpoint, your home data travels over the network. See the
[Data and privacy]({{< ref "/docs/guide/llm-chat#data-and-privacy" >}})
section of the LLM chat guide for details on exactly what is sent.

## Demo mode

`micasa demo` creates an in-memory database populated with fictitious sample
data. To persist demo data, pass a file path:

```sh
micasa demo /tmp/demo.db
```

Demo data includes sample addresses, phone numbers (all `555-xxxx`), and
`example.com` email addresses.
