<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# micasa Pro — Product & Technical Spec

> This document captures the full product vision, architecture, and build plan
> for micasa Pro. It is designed to be used as context for Claude Code sessions.

---

## 1. Product Overview

### What is micasa Pro?

micasa is free, local, and yours forever. Pro is the infrastructure you'd
have to build yourself to get sync, sharing, and backup.

The free product is complete. Every feature — projects, maintenance,
incidents, appliances, vendors, quotes, document extraction, LLM chat,
full-text search — stays free. Pro doesn't gate features. Pro sells ops.

### What Pro provides

One infrastructure layer that solves three problems:

- **Household sharing.** Invite your partner. Both of you see the same home
  data, from your own devices, with your own micasa installs.
- **Multi-device sync.** Your desktop and your laptop stay in sync. Edit on
  one, see it on the other.
- **Encrypted backup.** Your data lives on our server as an encrypted blob we
  can't read. Your disk dies, your data doesn't.

### What Pro does NOT do

- Gate any existing free feature
- Require an internet connection to use micasa
- Give us the ability to read your data
- Change how the local app works

### Target user

The same person already using micasa — technical, privacy-conscious,
runs a TUI to manage their house. They want their partner to have access,
or they work across multiple machines, or they want peace of mind that
their data survives a disk failure. They'd build this themselves if they
had the time.

### Pricing

- $9/month or $89/year (anchor on annual)
- Founding member price: $69/year, locked in permanently
- Free tier: the complete micasa app, forever, no sync

---

## 2. Architecture Overview

### The sync model

The server is an encrypted relay. It stores data it cannot read. All
merge logic runs on the client.

```
┌──────────────┐         ┌──────────────┐
│  Client A    │         │  Client B    │
│  (desktop)   │         │  (laptop)    │
│              │         │              │
│  SQLite DB   │         │  SQLite DB   │
│  Private key │         │  Private key │
└──────┬───────┘         └──────┬───────┘
       │                        │
       │   encrypted ops        │   encrypted ops
       ▼                        ▼
┌─────────────────────────────────────────┐
│            Sync Relay Server            │
│                                         │
│  Stores encrypted operation log         │
│  Cannot read, merge, or interpret data  │
│  Delivers ops to other household        │
│  members / devices                      │
│  Holds encrypted backup state           │
│                                         │
└─────────────────────────────────────────┘
```

### Why a relay and not a source-of-truth server?

Because micasa is local-first and that should mean something. The app works
fully offline. The server is a convenience, not a dependency. If the relay
goes down, nothing breaks — you just don't sync until it's back. If you
cancel Pro, you keep your local database. Nothing is lost.

And because the server can't read the data, there's nothing to breach. The
worst case in a server compromise is encrypted blobs that are useless
without the household's private key.

---

## 3. Data Model Context

### What we're syncing

micasa stores everything in a single SQLite file managed by GORM. The
tables are:

| Table | Typical scale | Sync notes |
|-------|--------------|------------|
| `house_profiles` | 1 row | Singleton, rarely changes. See "Singleton merge" below. |
| `projects` | 10-50 rows | Moderate edit frequency |
| `project_types` | ~10 rows (pre-seeded) | Reference data, rarely modified. Not soft-deletable. |
| `quotes` | 10-100 rows | Created often during active projects |
| `vendors` | 10-50 rows | Unique name constraint. Shared across quotes/service logs/incidents. |
| `maintenance_items` | 10-100 rows | Core entity, moderate edits |
| `maintenance_categories` | ~10 rows (pre-seeded) | Reference data. Not soft-deletable. |
| `incidents` | 5-50 rows | Bursty creation |
| `appliances` | 5-30 rows | Infrequent changes |
| `service_log_entries` | 10-500 rows | Append-mostly. CASCADE on MaintenanceItem delete. |
| `documents` | 10-100 rows | BLOBs up to 50MB each (configurable via `documents.max_file_size`) |

**Excluded from sync (local-only):**

| Table | Reason |
|-------|--------|
| `documents_fts` | Virtual table, rebuilt from `ExtractedText` after sync |
| `deletion_records` | Local audit trail. Redundant with oplog for sync; rebuilt locally from applied ops. |
| `settings` | Per-device preferences (e.g., last-used LLM model) |
| `chat_inputs` | Per-device chat history |
| `sync_oplog_entries` | The sync mechanism itself |
| `sync_device` | Local-only device identity and sync cursor |

### Key properties

- **Small dataset.** Total row count across all tables is in the hundreds,
  not thousands. Without document BLOBs, the database fits on a floppy disk.
- **Soft deletes.** GORM sets `deleted_at` rather than removing rows.
  Deletions must sync. Not all tables use soft deletes — `project_types`,
  `maintenance_categories`, `house_profiles`, `settings`, and `chat_inputs`
  lack a `DeletedAt` field.
- **GORM timestamps.** Every row has `created_at`, `updated_at`,
  `deleted_at` (where applicable). These are our change detection mechanism.
- **Polymorphic FKs.** Documents use `EntityKind` + `EntityID` to link to
  any entity type (project, quote, vendor, etc.). The `EntityKind` value is
  the polymorphic discriminator (e.g., "vendor", "project"), mapped to table
  names at init time via `BuildEntityKindToTable()`.
- **Document BLOBs.** Files stored inline in SQLite (`Data []byte`). Up to
  50MB each (configurable). These are the only large data and need special
  treatment in sync.
- **FTS index.** `documents_fts` is a virtual table rebuilt from
  `ExtractedText`. It must be rebuilt locally after sync, never shipped.
- **Unique constraints.** `Vendor.Name` and `ProjectType.Name` have unique
  indices. Sync apply must handle unique constraint violations gracefully
  (see Section 10 "Unique constraint conflicts").
- **CASCADE deletes.** `ServiceLogEntry` has `ON DELETE CASCADE` from
  `MaintenanceItem`. Cascaded deletes happen at the SQLite level, bypassing
  GORM hooks. The oplog must handle this explicitly (see Section 5
  "CASCADE handling").
- **Find-or-create pattern.** Vendors, Appliances, and MaintenanceItems
  use a find-or-create-with-restore pattern: search including soft-deleted
  records, restore if found deleted, create if not found. This compound
  operation produces different op types depending on runtime state.
- **Singleton: HouseProfile.** Only one row (ID=1). When a device joins
  a household, the household's HouseProfile takes precedence. The joiner's
  local HouseProfile is preserved in a backup but overwritten.
- **No migration system.** GORM AutoMigrate adds columns/tables. No rename
  or drop support yet. Sync must tolerate schema drift across client
  versions.

---

## 4. Primary Key Strategy

### The problem

GORM uses auto-increment `uint` primary keys. Two devices operating
independently will assign the same IDs to different rows. Device A creates
project ID=5 ("Deck rebuild"), device B creates project ID=5 ("Kitchen
remodel"). When ops sync, both inserts target the same ID — collision.

Worse, FK references cascade the problem. If device A creates quote ID=3
with `ProjectID=5`, that FK now points at the wrong project on device B.

### Decision: migrate to ULID text primary keys

ULIDs (Universally Unique Lexicographically Sortable Identifiers) solve
ID collisions while preserving time-sortability. They're 26-character
base32 strings, globally unique without coordination.

```
Current: ID uint `gorm:"primaryKey"` — auto-increment, collision-prone
New:     ID string `gorm:"primaryKey;size:26"` — ULID, globally unique
```

### Why ULIDs over UUIDs?

- **Time-sortable.** ULID's first 48 bits encode millisecond-precision
  time. `ORDER BY id` produces chronological order. This preserves the
  behavior users expect from auto-increment IDs.
- **Already in the oplog.** The `sync_oplog_entries.id` field already uses ULIDs.
  Using ULIDs for row IDs unifies the ID strategy.
- **Compact.** 26 chars vs UUID's 36. Smaller indices.
- **No coordination.** Each device generates ULIDs independently with
  negligible collision probability (48-bit timestamp + 80-bit randomness).

### Migration plan

This is the most invasive prerequisite for Pro. It must ship in the free
product before any sync code:

1. **Add ULID generation.** `github.com/oklog/ulid/v2` for Go.
2. **GORM hook: `BeforeCreate`.** If `ID` is empty, assign a new ULID.
   This means all existing code that creates records (`tx.Create(&item)`)
   continues to work unchanged — GORM calls the hook before insert.
3. **Schema migration.** For each table with a `uint` PK:
   - Create new table with `TEXT` PK
   - Copy rows, generating ULIDs for existing IDs
   - Maintain an `old_id → new_ulid` mapping table (temporary)
   - Update all FK references using the mapping
   - Drop old tables, rename new ones
   - Drop mapping table
4. **Update all code referencing `uint` IDs to `string`.** This touches
   handlers, forms, Store methods, test helpers, and the TUI (row
   selection, drilldown, undo snapshots). It's a large but mechanical
   change.
5. **Update `DeletionRecord.TargetID`** from `uint` to `string`.

### FK reference integrity in oplog payloads

Because IDs are globally unique, FK references in oplog payloads (e.g.,
`Quote.ProjectID`, `ServiceLogEntry.MaintenanceItemID`) are valid on any
device. No remapping needed. The receiving device applies the payload
as-is.

### Backward compatibility

The ULID migration is a one-way schema change. Users upgrading to the
ULID-enabled version get an automatic migration. There is no downgrade
path — this is acceptable because micasa already has no downgrade support
(GORM AutoMigrate is additive-only).

### Alternative considered: ID remapping

An alternative is to keep `uint` PKs and maintain a mapping table
(`remote_device_id, remote_row_id → local_row_id`) on each device. This
avoids the schema migration but introduces ongoing complexity:

- Every FK reference in every incoming op must be remapped
- The mapping table grows with every row
- Polymorphic FK resolution (`EntityKind` + `EntityID`) becomes fragile
- The code complexity is permanent, not one-time

The ULID migration is more work upfront but eliminates an entire class of
bugs permanently.

---

## 5. Change Tracking

### Approach: row-level operation log

Every mutation (insert, update, soft-delete, restore) is captured as an
operation in a local append-only log before it's applied to the database.
This log is the unit of sync.

### New table: `sync_oplog_entries`

```sql
CREATE TABLE sync_oplog_entries (
    id TEXT PRIMARY KEY,           -- ULID (time-sortable, globally unique)
    table_name TEXT NOT NULL,      -- e.g. "projects", "maintenance_items"
    row_id TEXT NOT NULL,          -- ULID PK of the affected row (see Section 4)
    op_type TEXT NOT NULL,         -- "insert", "update", "delete", "restore"
    payload TEXT NOT NULL,         -- JSON: full row snapshot (insert/update)
                                   --   or empty object (delete/restore)
    device_id TEXT NOT NULL,       -- identifies which device produced this op
    created_at TEXT NOT NULL,      -- ISO 8601 timestamp with ms precision
    applied_at TEXT,               -- when this op was applied to the live DB
                                   --   NULL = received but not applied (conflict loser)
    synced_at TEXT                 -- set when successfully pushed to relay
);

CREATE INDEX idx_oplog_synced ON sync_oplog_entries(synced_at);
CREATE INDEX idx_oplog_table_row ON sync_oplog_entries(table_name, row_id);
CREATE INDEX idx_oplog_unapplied ON sync_oplog_entries(applied_at) WHERE applied_at IS NULL;
```

**Op lifecycle:**
- Local mutation: `applied_at` = now (applied immediately), `synced_at` = NULL
- After push: `synced_at` = now
- Remote op received and applied: `applied_at` = now, `synced_at` = now
- Remote op received, lost conflict: `applied_at` = NULL, `synced_at` = now

This mirrors the CouchDB model: all revisions are kept, a deterministic
winner is picked, and losing revisions are recoverable. No data is ever
discarded.

### New table: `sync_device`

Local-only. Stores this device's identity and sync cursor.

```sql
CREATE TABLE sync_device (
    id TEXT PRIMARY KEY,           -- ULID, generated once during `micasa pro init`
    name TEXT NOT NULL,            -- human-readable (hostname or user-chosen)
    household_id TEXT,             -- set after init or join
    last_seq INTEGER DEFAULT 0,   -- highest relay seq number received
    created_at TEXT NOT NULL
);
```

Only one row exists locally. The table (not a flat file) because it
participates in the same SQLite backup as everything else.

### Why an oplog instead of diff-based sync?

- **Works with E2E encryption.** The server stores encrypted ops it can't
  read. If we did diff-based sync, the server would need to understand
  the data to compute diffs.
- **Conflict detection.** Two ops on the same (table, row_id) from different
  devices is a conflict. Easy to detect, easy to resolve.
- **Auditability.** The oplog is a history of every change. Useful for
  debugging sync issues.
- **Append-only.** Never need to modify existing ops. Simple, reliable.

### What gets logged

Every GORM hook (AfterCreate, AfterUpdate, AfterDelete) writes to the
oplog. The payload is a JSON snapshot of the full row at the time of the
operation.

**Applying remote ops must NOT trigger oplog writes.** When the sync
pull applies a remote op to the local DB, it must bypass the GORM hooks
that write to the oplog — otherwise the applied op would be re-logged
locally and pushed back to the relay in an infinite loop.

**Mechanism:** Use a context flag. The GORM hooks check for a
`sync.applying` key in the `gorm.DB`'s context:

```go
func (p Project) AfterCreate(tx *gorm.DB) error {
    if tx.Statement.Context.Value(syncApplyingKey{}) != nil {
        return nil // remote apply — don't log
    }
    return writeOplogEntry(tx, "projects", p.ID, "insert", p)
}
```

The sync apply code sets this flag before calling `tx.Create()`/
`tx.Save()`/etc. Normal local mutations don't set it, so hooks fire
as usual.

**Excluded from oplog** (matches the local-only table list in Section 3):
- `documents_fts` (virtual table, rebuilt locally)
- `deletion_records` (local audit trail, rebuilt from oplog)
- `settings` (per-device preferences)
- `chat_inputs` (per-device chat history)
- `sync_oplog_entries` itself
- `sync_device` (local-only device identity)

**Special handling for documents:**
- The `payload` for document inserts/updates includes metadata but NOT the
  BLOB content (`Data` field omitted from JSON). BLOBs sync separately
  (see Section 8).
- A `blob_ref` field in the payload points to the content-addressed blob
  in the blob store (the SHA-256 hash already computed for integrity, stored
  in `ChecksumSHA256`).
- When applying remote document ops, the `blob_ref` key is stripped from
  the payload before writing to the database — it has no corresponding DB
  column and exists only to coordinate blob sync. Similarly, `deleted_at`
  is always stripped to prevent a malicious relay from injecting
  soft-deletes via crafted payloads.

**Deterministic delete convergence:**
- When applying a remote "delete" op, the client sets `deleted_at` to the
  op's `created_at` timestamp (not `time.Now()`). This ensures all devices
  converge on the same `deleted_at` value for the same delete operation,
  regardless of when each device processes it.

### CASCADE handling

`ServiceLogEntry` has `ON DELETE CASCADE` from `MaintenanceItem`. When a
MaintenanceItem is soft-deleted, SQLite doesn't cascade (soft delete is an
UPDATE, not a DELETE). But if a hard-delete ever occurs (future
consideration), the cascaded ServiceLogEntry deletes happen at the SQLite
level, bypassing GORM hooks entirely — no oplog entries would be generated
for the children.

**Solution:** The oplog write hook must explicitly enumerate CASCADE
children before writing a delete op. For MaintenanceItem deletes:

1. Query all ServiceLogEntries with `maintenance_item_id = <id>`
2. Write a "delete" oplog entry for each child
3. Write the "delete" oplog entry for the parent
4. All in the same transaction

This is currently only one relationship (`ServiceLogEntry → MaintenanceItem`)
but the pattern must be followed for any future CASCADE FKs.

### Restoration hook gap

GORM's `AfterUpdate` hook fires on standard updates, but restoration uses
`db.Unscoped().Model(&item).Update("deleted_at", nil)` — a raw column
update that may not trigger model-level hooks consistently.

**Solution:** Don't rely on GORM hooks for restoration. Instead, the
Store's `Restore()` method explicitly writes a "restore" oplog entry
after successfully clearing `deleted_at`. This is already the pattern for
`DeletionRecord` updates (the Store method handles both the restore and
the audit trail write).

### DeletionRecord interaction

With the oplog capturing all deletes and restores, the existing
`deletion_records` table becomes redundant for sync purposes. However, it
remains useful locally as a user-facing audit trail ("when was this item
deleted? was it restored?").

**Strategy:** Don't sync `deletion_records`. Each device rebuilds its own
`deletion_records` from the oplog entries it applies. When a "delete" op
is applied, insert a local `DeletionRecord`. When a "restore" op is
applied, set `RestoredAt` on the matching `DeletionRecord`.

---

## 6. Encryption

### Algorithm

NaCl secretbox (XSalsa20 + Poly1305) via `golang.org/x/crypto/nacl/secretbox`
for symmetric encryption of sync data.

### Key hierarchy

```
Household Key (HK)
  └── 256-bit random key, generated once when household is created
  └── Used to encrypt/decrypt all sync operations
  └── Shared with household members via key exchange (see Section 9)

Device Key (DK)
  └── Curve25519 keypair, generated per device during `micasa pro init`
  └── Used for key exchange when joining a household
  └── Private key never leaves the device
```

### Key storage

Keys are stored following the application's XDG convention:

```
$XDG_DATA_HOME/micasa/secrets/
  household.key      # 256-bit household symmetric key
  device.pub         # Curve25519 public key
  device.key         # Curve25519 private key (0600 permissions)
  device.token       # bearer token for relay API auth (0600 permissions)
```

On Linux this defaults to `~/.local/share/micasa/secrets/`. On macOS,
`~/Library/Application Support/micasa/secrets/`. This matches the existing
DB path convention (`xdg.DataFile`).

Keys and credentials are NOT stored in the SQLite database. Rationale:
the DB is the data being synced. Keys and tokens are the mechanism for
syncing. Keeping them separate means `micasa backup backup.db` doesn't
export credentials (a security property: backups are data-only). An
attacker with a backup file gets plaintext data (which they'd have
anyway from the local DB) but cannot impersonate the device to the relay.

### Why symmetric (secretbox) for sync data?

The household is a small trust group (typically 2 people). Every member
has the same household key. Symmetric encryption is faster and simpler
than encrypting per-recipient. When a new member joins, they receive the
household key via an asymmetric key exchange (NaCl box).

### Encryption envelope

Every operation pushed to the relay is wrapped in an `Envelope`:

```go
type Envelope struct {
    ID         string    `json:"id"`
    Nonce      []byte    `json:"nonce"`       // 24 bytes (XSalsa20)
    Ciphertext []byte    `json:"ciphertext"`  // secretbox-sealed oplog entry
    CreatedAt  time.Time `json:"created_at"`
    Seq        int64     `json:"seq"`          // assigned by relay on push
}
```

The client constructs `sealed = nonce || ciphertext` for encryption/
decryption via `crypto.Encrypt`/`crypto.Decrypt`. The nonce and
ciphertext are split for JSON transport and stored separately on the
relay.

The server sees: id, nonce, ciphertext, timestamp, and sequence number.
It cannot read the ciphertext. It uses the authenticated device's
household_id to route ops and seq for ordering. The `household_id` and
`device_id` are not in the envelope itself — they come from the
authenticated device context.

---

## 7. Sync Protocol

### Push (client → relay)

1. Client queries `sync_oplog_entries WHERE synced_at IS NULL` (unsynced ops)
2. For each op: encrypt payload with household key → envelope
3. `POST /sync/push` with batch of envelopes
4. Server stores envelopes, returns confirmed sequence numbers
5. Client sets `synced_at` on confirmed ops

### Pull (relay → client)

1. Client tracks `last_seq` — the highest sequence number it has received
   from the relay
2. `GET /sync/pull?after={last_seq}` → server returns all envelopes with
   seq > last_seq, excluding ops from the requesting device_id
3. Client decrypts each envelope → oplog entry
4. Client applies ops to local database (see merge below)
5. Client updates `last_seq`

### Sync trigger

- **On startup:** pull, then push
- **Periodic:** every 60 seconds while app is running (configurable)
- **On mutation:** push immediately after any local write (debounced 2s)
- **Manual:** `micasa pro sync` command for explicit sync

### Transaction safety

The oplog write and the data mutation MUST occur in the same SQLite
transaction. If either fails, both roll back. This guarantees the oplog
never diverges from the actual data state.

```go
// Pseudocode for a mutation with oplog
err := store.Transaction(func(tx *gorm.DB) error {
    if err := tx.Save(&project).Error; err != nil {
        return err
    }
    return writeOplogEntry(tx, "projects", project.ID, "update", project)
})
```

The existing `Store.Transaction()` method already wraps GORM transactions.
Oplog writes use the same `tx` handle.

**Apply-side transactions (pull):** When applying a remote op, the data
write and the local `DeletionRecord` update (for deletes/restores) must
also be in the same transaction. If the apply fails (e.g., FK constraint
violation), the op is marked as failed and retried on the next pull cycle.

### Concurrency

If the TUI is writing locally while a background sync pull is applying
remote ops, both hit the same SQLite database. SQLite's WAL mode (already
enabled by micasa) supports concurrent reads but serializes writes.
The sync pull goroutine must acquire the same write lock as local mutations.

**Solution:** Route all writes through the Store's transaction method,
which serializes on SQLite's write lock. The sync goroutine is just
another caller — no additional locking needed beyond what SQLite provides.

### Client safety

- **Bounded error reads:** when the relay returns an error response, the
  client reads at most 4096 bytes of the body (`maxErrorBody`) to prevent
  a malicious relay from exhausting client memory with an unbounded
  response.
- **Table allowlist:** the sync apply layer maintains an explicit allowlist
  of syncable tables (`allowedSyncTable`). Remote ops targeting unknown
  tables (e.g., `sync_oplog_entries`, `settings`, `sync_device`) are
  rejected before any database write.
- **Insert ID validation:** remote insert payloads must contain a string
  `id` field matching the op's `row_id`. Mismatches are rejected. This
  prevents a crafted op from inserting a row with an unexpected ID.

### Offline behavior

If the relay is unreachable, ops accumulate locally in `sync_oplog_entries` with
`synced_at = NULL`. Next successful connection pushes the backlog. The app
is fully functional offline — sync is opportunistic, never blocking.

---

## 8. Document BLOB Sync

### The problem

Document BLOBs can be up to 50MB each. Shipping them inline in the oplog
would be wasteful (every device downloads every version of every document)
and slow.

### Content-addressed blob store

BLOBs are stored separately from the oplog, addressed by their SHA-256 hash
(which micasa already computes for integrity checks).

```
Client has document with hash abc123...
  → Encrypts BLOB with household key
  → PUT /blobs/{household_id}/{hash}
  → Server stores encrypted blob

Other client sees oplog entry referencing blob_ref=abc123...
  → GET /blobs/{household_id}/{hash}
  → Decrypts locally
  → Writes to local documents table
```

### Deduplication

If two clients attach the same file, the hash matches and only one
encrypted copy is stored. The oplog entries both reference the same
blob_ref.

### Lazy pull

Clients don't download all BLOBs eagerly. On pull, the oplog entry is
applied with document metadata but `Data` set to nil. A document with
`ChecksumSHA256 != "" && Data == nil` is a pending blob — no new field
needed, the existing columns encode the state.

The actual BLOB is fetched on demand (when the user tries to open the
document) or during a background sync pass.

This keeps the sync fast for the common case (metadata changes) and avoids
downloading 50MB PDFs on every device if they're never opened there.

### Storage quotas

Pro includes a household storage quota for BLOBs:

- **Starter:** 1 GB (covers ~20 large PDFs or hundreds of invoices)
- **Future tier:** adjustable, but 1 GB is plenty for v1

Quota is enforced server-side on `PUT /blobs`. The client shows remaining
quota in `micasa pro status`.

---

## 9. Household Management

### Creating a household

```bash
micasa pro init
```

1. Generates device keypair (Curve25519)
2. Generates household key (256-bit random)
3. Stores both locally in `$XDG_DATA_HOME/micasa/secrets/` (see Section 6)
4. Registers device with the relay server
5. Creates household on the relay
6. Performs initial full push (all existing data → oplog → encrypted → relay)

### Inviting a member

```bash
micasa pro invite
```

1. Generates a one-time invite code (13-character base32 string, expires
   in 24h)
2. Displays the code for the user to share (text, verbally, etc.)

The invite code encodes enough information for the joining device to:
- Find the relay server
- Identify the household
- Initiate a key exchange

**Invite code format:**
- 8 bytes of `crypto/rand` entropy → base32-encoded without padding →
  13-character string (A-Z, 2-7) = 64 bits of entropy
- Expires after 24 hours

**Invite code security:**
- Rate-limited: max 5 join attempts per invite code, then invalidated
- Rate-limited: max 3 active (unexpired, unconsumed) invite codes per
  household at any time, enforced with `FOR UPDATE` row locking under
  Postgres to prevent TOCTOU races
- Server-side: join attempts from unknown device IDs are rate-limited to
  1 per second per source IP to prevent brute force (post-v1, handled at
  infrastructure level)

### Joining a household

```bash
micasa pro join <invite-code>
```

1. Generates device keypair
2. Contacts relay, identifies household from invite code
3. Relay facilitates key exchange:
   - Joiner's public key is sent to the inviter's next sync
   - Inviter's client encrypts household key with joiner's public key
   - Encrypted household key is relayed to joiner
4. Joiner decrypts household key, stores locally
5. Joiner performs initial full pull (downloads all encrypted ops, applies)
6. Invite code is consumed (one-time use)

**Important:** The relay never sees the household key. It only facilitates
the exchange of public keys and encrypted messages between devices.

**Single-use credential retrieval:** When the joiner polls
`GET /key-exchange/{id}`, the response includes the encrypted household
key and the device bearer token. These credentials are cleared from the
relay after the first successful retrieval — a second GET returns
`ready: true` but with empty credentials. This prevents a leaked
exchange ID from being exploited after the joiner has already retrieved
their credentials.

### Joining with existing data (post-v1)

If the joining device already has a micasa database (they've been using
micasa solo), the join process must handle merge:

- Pull all remote ops first → build the "household" state
- Compare local state against household state
- For conflicts: the household version wins (the inviter's data takes
  precedence on first join)
- Local-only records that don't conflict are pushed as new ops

This is the one scenario where data could be lost (local edits overridden
by household state). The join flow should warn the user and create a
local backup before proceeding.

**Status:** post-v1. For v1, `micasa pro join` requires a fresh database
(no existing data). Users with existing data should back up first, then
start with a clean database.

### Device management

```bash
micasa pro devices          # list devices in household
micasa pro devices revoke   # remove a device (e.g., lost laptop)
```

Revoking a device sets a `revoked` flag on the device record (the record
is not deleted, preserving the audit trail). Revoked devices fail
authentication on all subsequent API calls — they cannot push, pull,
or access blobs. Revocation does NOT rotate the household key (the
revoked device retains its copy). For true cryptographic revocation
after a compromised device, see Section 11 "Key rotation".

---

## 10. Conflict Resolution

### Strategy: last-write-wins per row

For a household of 1-2 people managing one house, sophisticated conflict
resolution (CRDTs, three-way merge) is overkill. The realistic conflict
scenario — two people editing the same maintenance item at the same time
while both offline — is vanishingly rare.

### How it works

When applying pulled ops, the client checks:

1. Does a local unsynced op exist for the same (table_name, row_id)?
2. If yes: compare the `created_at` timestamps on the two oplog entries.
   The op with the later `created_at` wins. Tiebreaker: lexicographic
   device_id. This applies to all op types — insert, update, delete,
   restore. The oplog `created_at` is always present regardless of
   payload contents.
3. The winning op is applied (`applied_at` = now). The losing op stays
   in the oplog with `applied_at = NULL` — present for audit and
   recovery but not reflected in the live tables.

**Recovering a conflict loser:** `micasa pro conflicts` lists ops where
`applied_at IS NULL AND synced_at IS NOT NULL`. These are ops (either
local or remote) that lost the LWW comparison — they were recorded in
the oplog for audit purposes but their effect was not applied to the
live tables. Specifically:
- A **local** op loses when a newer remote op for the same row wins.
  The local op's `applied_at` is cleared (set to NULL) and `synced_at`
  remains NULL (it was never pushed). All local unsynced ops for that
  row are cleared, not just the latest, to ensure row state consistency.
- A **remote** op loses when an older remote op arrives but the local
  device already has a newer unsynced op. The remote op is recorded
  with `applied_at = NULL` and `synced_at = now`.

The user can review the losing payload and choose to apply it instead,
which flips the `applied_at` values (sets the loser's, clears the
winner's) and reapplies the row.

### Conflict notification

When a conflict is resolved, the client shows a status bar message:

```
⚠ Sync conflict: "HVAC filter replacement" — Laptop version kept (3:45 PM > 3:42 PM)
```

### Delete/edit conflicts

If one client deletes a row and another edits it:
- The later `created_at` wins via standard LWW. If the delete is newer,
  the edit is discarded. If the edit is newer, the delete is discarded.
- Deletes are soft-deletes, so even if the delete wins, the row is
  recoverable via restore.
- Delete ops use deterministic convergence: all devices set `deleted_at`
  to the op's `created_at` (not local wall-clock time), ensuring
  identical `deleted_at` values across devices.

If one client deletes and another restores:
- The later timestamp wins via standard LWW.

### Document conflicts

For document metadata: LWW same as any other row.
For document BLOBs: if two clients attach different files to the same
document, both BLOBs are stored (content-addressed, different hashes).
The metadata LWW determines which blob_ref is active. The other blob
is retained for 30 days then garbage-collected.

### Unique constraint conflicts

`Vendor.Name` and `ProjectType.Name` have unique indices. Two devices
could independently create a vendor named "Bob's Plumbing". With ULID
PKs, these are different rows (different IDs) but the unique constraint
prevents both from existing.

**Resolution:** When applying a remote insert that violates a unique
constraint:

1. Find the existing local row with the conflicting unique value
2. Compare timestamps — the older row is the "canonical" one
3. The newer row's data is merged into the older row (LWW per field)
4. The duplicate row is soft-deleted. Future ops referencing the
   duplicate's ID are applied to the canonical row by checking the
   oplog for the merge record.
5. Notify the user: "Vendor 'Bob's Plumbing' was created on both
   Desktop and Laptop — merged into one record"

This is a narrow case (only Vendor and ProjectType have unique constraints)
and the resolution is deterministic.

### Find-or-create conflicts

The find-or-create-with-restore pattern (used for Vendors, Appliances,
MaintenanceItems during document extraction) can produce different op
types on different devices for the same logical operation. Device A finds
a soft-deleted "Bob's Plumbing" and restores it; Device B doesn't have
that vendor and creates a new one.

**Resolution:** This is a variant of the unique constraint conflict above.
The existing restored vendor and the newly created vendor collide on the
unique name. Same merge logic applies.

---

## 11. Security Considerations

### Threat model

- **Server compromise:** attacker gets encrypted blobs they can't read.
  No plaintext data is ever stored server-side. The relay stores only
  ciphertext, nonces, and metadata (household_id, device_id, timestamps,
  sequence numbers). Attacker gains traffic analysis metadata (who syncs
  when, payload sizes) but no content.
- **Device theft:** attacker gets the local SQLite DB (plaintext) and the
  household key. Mitigation: the local DB was always plaintext (that's
  the current state without Pro). The household key allows decrypting
  relay-stored data and future sync traffic until the key is rotated.
  **Response:** other household members should run `micasa pro keys rotate`
  to generate a new household key and revoke the stolen device (see
  "Key rotation" below).
- **Network interception:** all client-relay communication is over TLS.
  Even without TLS, data is E2E encrypted with the household key.
- **Relay MITM during key exchange:** the relay facilitates NaCl box key
  exchange between inviter and joiner. A compromised relay could
  substitute its own public key during the exchange, performing a
  man-in-the-middle attack to intercept the household key. **Mitigation
  (post-v1):** out-of-band key fingerprint verification — both parties
  compare a short fingerprint derived from the exchanged public keys.
  For v1, the risk is accepted: the relay is our own infrastructure, and
  the invite code's 64-bit entropy + 24h expiry + 5-attempt limit
  constrains the attack window.

### Known limitations

- **Device revocation is relay-level only.** Revoking a device prevents
  it from authenticating to the relay (push/pull/blob operations fail).
  However, the revoked device retains the household key and its local
  copy of all data. It can decrypt any previously-downloaded ops and
  any ops it intercepts by other means. Full cryptographic revocation
  requires key rotation (see below).
- **LWW clock fragility.** Conflict resolution uses `created_at`
  timestamps from the originating device. Devices with significantly
  skewed clocks can cause incorrect conflict winners. NTP-synced clocks
  are assumed. In practice, modern operating systems keep clocks within
  milliseconds via NTP, and the conflict scenario (two people editing
  the same row while both offline) is already rare for a 1-2 person
  household. If clock skew becomes a problem, a future version could
  use the relay's monotonic `seq` as the tiebreaker instead of device
  timestamps.
- **Max device enforcement gap.** The 4-device limit is checked at join
  time. If a household somehow exceeds the limit (e.g., concurrent joins
  racing), excess devices are not retroactively removed. This is
  acceptable for v1 — the 3-invite-code limit and the key exchange flow
  make concurrent joins unlikely.
- **Key exchange credential window.** Between when the inviter completes
  the key exchange and when the joiner retrieves the credentials, the
  encrypted household key and device token sit on the relay. Credentials
  are single-use (cleared after first retrieval) but persist until then.
  The key exchange record should expire (e.g., 1 hour) to bound this
  window. **Status:** not yet implemented, tracked for v1.

### Auth model

- **Token format:** 256-bit crypto-random hex string, generated during
  device registration.
- **Storage:** client stores the raw token in `device.token` (0600
  permissions). Relay stores only the SHA-256 hash in `devices.token_sha`.
- **Lookup:** relay computes `SHA256(token)` and does an indexed lookup on
  `token_sha` for O(1) authentication. SHA-256 was chosen over bcrypt
  because: (a) tokens are 256-bit random (not human-chosen passwords),
  making rainbow tables and brute force infeasible; (b) constant-time
  indexed lookup is critical for every API request; (c) bcrypt's
  intentional slowness would add latency to every sync operation.
- **Subscription gating:** after authentication, push/pull/blob endpoints
  check `households.stripe_status = 'active'`. Non-active returns 402.
  Management endpoints (status, devices, invite) work regardless of
  subscription status.

### Key rotation

If a device is compromised:

```bash
micasa pro keys rotate
```

**Protocol (post-v1):**

1. Caller generates a new 256-bit household key locally
2. Pull all ops from the relay, decrypt with the old key
3. Re-encrypt every op with the new key, push replacement batch
4. Re-encrypt all stored blobs: download each, decrypt with old key,
   re-encrypt with new key, upload (replacing the old ciphertext)
5. For each non-revoked device (other than the caller): initiate a key
   exchange — encrypt the new household key with the target device's
   NaCl box public key and store on the relay
6. Other devices pick up the new key on their next sync, switch to it
7. Revoke the compromised device

**Open design questions:**
- **In-flight ops:** ops encrypted with the old key that arrive after
  rotation must be handled. Options: (a) reject them (device must
  re-encrypt and re-push), (b) the relay stores a "key generation"
  counter and clients include it in envelopes.
- **Partial failure:** if rotation fails midway (e.g., network error
  after re-encrypting half the ops), the relay has a mix of old-key and
  new-key ciphertext. The client must be able to resume rotation.
- **Blob re-encryption cost:** for a household at the 1 GB quota limit,
  rotation means downloading, decrypting, re-encrypting, and re-uploading
  1 GB. This should run as a background operation with progress reporting.

This is expensive (re-encrypts everything) and should be rare. But
without it, a compromised device retains the ability to decrypt any
future ops it intercepts — revoking only prevents relay access, not
cryptographic access. Key rotation closes that gap.

**Status:** key rotation is post-v1. For v1, device revocation (relay-level)
is the only response to compromise. Users are advised to treat the
household key as compromised if a device is lost and to re-create the
household if they need cryptographic assurance.

### What the server knows

The relay server knows:
- Which households exist
- Which devices belong to each household
- When ops were created (timestamps)
- Size of encrypted payloads (reveals rough data volume)
- Sync frequency patterns (reveals activity level)
- Blob count and sizes per household (reveals document usage)

The relay server does NOT know:
- Any home data (addresses, costs, vendors, projects, etc.)
- What type of operation occurred (insert vs update vs delete)
- Which tables were affected
- Document contents or filenames
- The household encryption key

---

## 12. Relay Server API

All endpoints require device authentication (device_id + bearer token
from `device.token`, issued during registration).

The server verifies that the authenticated device belongs to the
household specified in the request. A device can only push/pull ops
for its own household — the `household_id` in the envelope must match
the device's registered household.

### Sync endpoints

#### `POST /sync/push`

Push a batch of encrypted operations.

```json
// Request (household_id and device_id are inferred from the auth token)
{
  "ops": [
    {
      "id": "01J...",
      "nonce": "<base64>",
      "ciphertext": "<base64>",
      "created_at": "2026-03-16T10:30:00.000Z"
    }
  ]
}

// Response
{
  "confirmed": [
    { "id": "01J...", "seq": 4827 }
  ]
}
```

#### `GET /sync/pull?after={seq}&limit={n}`

Pull operations from the relay. Returns ops from all devices in the
household except the requesting device. The `limit` parameter caps
the batch size (default: all available).

```json
// Response
{
  "ops": [
    {
      "id": "01J...",
      "nonce": "<base64>",
      "ciphertext": "<base64>",
      "created_at": "2026-03-16T10:30:00.000Z",
      "seq": 4828
    }
  ],
  "has_more": false
}
```

The `has_more` field indicates whether more ops exist beyond the
returned batch. The client should continue pulling until `has_more`
is false.

### Blob endpoints

All blob endpoints require auth + active subscription.

#### `PUT /blobs/{household_id}/{hash}` (auth + subscription)

Upload an encrypted document BLOB.

- Request body: raw encrypted bytes (application/octet-stream)
- Hash must be exactly 64 lowercase hex characters (validated)
- Authenticated device must belong to the specified household
- Max blob size: 50 MB (`maxBlobSize`), enforced via `io.LimitReader`
- Household quota: 1 GB (`defaultBlobQuota`), enforced server-side
- Returns 201 on success, 409 if hash already exists (dedup — client
  treats as success), 413 if quota exceeded (response includes current
  usage and quota for actionable error display)

#### `GET /blobs/{household_id}/{hash}` (auth + subscription)

Download an encrypted document BLOB. Hash is validated (64 lowercase
hex). Returns raw bytes as `application/octet-stream`, or 404 if not
found.

#### `HEAD /blobs/{household_id}/{hash}` (auth + subscription)

Check if a blob exists without downloading. Hash is validated. Returns
200 if exists, 404 if not.

### Household endpoints

#### `POST /households`

Create a new household and register the first device. Called during
`micasa pro init`. Returns household_id, device_id, and bearer token.

#### `POST /households/{id}/invite` (auth required)

Generate an invite code. Enforces max 3 active (unexpired, unconsumed)
invites per household.

#### `POST /households/{id}/join` (unauthenticated)

Join a household with an invite code. Validates the code, increments
attempt counter, creates a key exchange record. Returns the exchange ID
for polling.

### Key exchange endpoints

#### `GET /households/{id}/pending-exchanges` (auth required)

List pending key exchanges for the household. Used by the inviter to
discover joiners waiting for the household key.

#### `POST /key-exchange/{id}/complete` (auth required)

Complete a key exchange. The inviter sends the household key encrypted
with the joiner's NaCl box public key, plus a freshly-generated device
token for the joiner.

#### `GET /key-exchange/{id}` (unauthenticated)

Poll for key exchange completion. The joiner calls this to retrieve the
encrypted household key and device token. **Single-use:** credentials
are cleared after the first successful retrieval — subsequent GETs
return `ready: true` with empty credential fields.

Intentionally unauthenticated because the joiner doesn't have a device
token yet. The exchange ID is a 256-bit crypto-random hex string, making
brute-force infeasible.

### Device endpoints

#### `GET /households/{id}/devices` (auth required)

List non-revoked devices in the household.

#### `DELETE /households/{id}/devices/{device_id}` (auth required)

Revoke a device. Sets the `revoked` flag (does not delete the device
record, preserving audit trail). Revoked devices fail authentication
on subsequent API calls.

### Status endpoint

#### `GET /status` (auth required)

Returns sync status, blob storage usage, and device list.

```json
{
  "household_id": "hh_...",
  "devices": [
    { "id": "dev_...", "name": "Desktop", "created_at": "2026-03-16T..." },
    { "id": "dev_...", "name": "Laptop", "created_at": "2026-03-15T..." }
  ],
  "blob_storage": {
    "used_bytes": 52428800,
    "quota_bytes": 1073741824
  },
  "ops_count": 4827,
  "stripe_status": "active"
}
```

### Health endpoint

#### `GET /health` (unauthenticated)

Returns 200 OK. Used for Cloud Run health checks.

### Webhook endpoint

#### `POST /webhooks/stripe` (unauthenticated, HMAC-verified)

Receives Stripe webhook events. Verifies the signature using a
pre-shared webhook secret (HMAC-SHA256, manual verification — not the
Stripe SDK). Returns 503 if no webhook secret is configured (prevents
silent signature bypass). Updates `households.stripe_status` and
`households.stripe_subscription_id` based on event type.

---

## 13. CLI Commands

### New commands for Pro

```bash
# Initialize Pro — create household, generate keys, register device
micasa pro init

# Check sync status
micasa pro status

# Force immediate sync
micasa pro sync

# Invite a household member
micasa pro invite

# Join an existing household
micasa pro join <invite-code>

# List devices
micasa pro devices

# Revoke a device
micasa pro devices revoke <device-id>

# View and recover conflict losers
micasa pro conflicts

# Show blob storage usage
micasa pro storage

# Rotate household key (after device compromise)
micasa pro keys rotate
```

### TUI integration

- **Status bar:** shows sync status icon
  - `◈` synced (all ops confirmed)
  - `◉` syncing (push/pull in progress)
  - `○` offline (relay unreachable)
  - `!` conflict (unresolved conflicts exist)
- **On startup:** automatic pull → push cycle, non-blocking
- **Background sync:** periodic poll, non-blocking
- **Mutation hook:** after any edit, debounced push

---

## 14. Relay Server Infrastructure

### Code location

The relay lives in this repo at `cmd/relay/main.go`. Shared types
(encryption envelope, oplog entry JSON schema) live in an `internal/sync`
package importable by both `cmd/micasa` and `cmd/relay`. The relay handler
and store implementations live in `internal/relay/`.

**Current state:** `cmd/relay/main.go` uses `MemStore` for local
development. Production wiring to `PgStore` (via `DATABASE_URL` env var)
and Stripe webhook secret configuration are remaining v1 tasks.

### Stack: GCP (Cloud Run + Cloud SQL + Cloud Storage)

GCP was chosen because the operator (Phillip) knows it well. Everything
stays within GCP's IAM/OAuth perimeter — no external credentials to
manage.

#### Cloud Run — compute

The relay is a single Go binary deployed as a container to Cloud Run.

- **Scales to zero** when no clients are syncing (minimal cost at launch)
- **Scales up** automatically when traffic increases (no capacity planning)
- **TLS termination** handled by Cloud Run (no cert management)
- **Custom domain** via Cloud Run domain mapping (e.g., `sync.micasa.dev`)
- **Min instances: 0** for v1 (accept cold start latency of ~1-2s; sync
  is not latency-sensitive). Bump to 1 when paying customers depend on it.
- **Multi-instance:** Cloud Run can run multiple instances concurrently
  since Postgres handles write coordination.

Deployment: `gcloud run deploy relay --source .` from `cmd/relay/`.

#### Cloud SQL (Postgres) — relay metadata and encrypted ops

Cloud SQL (Postgres) stores all relay state. SQL is the natural fit for
the relay's access patterns (insert ops, range query by seq, CRUD on
devices/households) and provides the same query language used throughout
the codebase.

**Schema:**

```sql
CREATE TABLE households (
    id          TEXT PRIMARY KEY,
    seq_counter BIGINT NOT NULL DEFAULT 0,
    stripe_subscription_id TEXT,
    stripe_status TEXT,  -- 'active', 'past_due', 'canceled', ''
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE devices (
    id           TEXT PRIMARY KEY,
    household_id TEXT NOT NULL REFERENCES households(id),
    name         TEXT NOT NULL,
    public_key   BYTEA,          -- NaCl box public key for key exchange
    token_sha    TEXT NOT NULL,   -- SHA-256 hex of bearer token (not bcrypt: tokens are
                                  --   256-bit random, so SHA-256 suffices and enables O(1) index lookup)
    last_seen    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked      BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX idx_devices_household ON devices(household_id);
CREATE INDEX idx_devices_token_sha ON devices(token_sha);

CREATE TABLE ops (
    seq          BIGINT NOT NULL,
    household_id TEXT NOT NULL REFERENCES households(id),
    id           TEXT NOT NULL,  -- ULID from client
    device_id    TEXT NOT NULL,  -- no FK: device revocation must not cascade-delete ops
    nonce        BYTEA NOT NULL,
    ciphertext   BYTEA NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, seq)
);
CREATE UNIQUE INDEX idx_ops_dedup ON ops(household_id, id);

CREATE TABLE invites (
    code         TEXT PRIMARY KEY,
    household_id TEXT NOT NULL REFERENCES households(id),
    created_by   TEXT NOT NULL,  -- device_id (no FK: revocation must not cascade)
    expires_at   TIMESTAMPTZ NOT NULL,
    consumed     BOOLEAN NOT NULL DEFAULT false,
    attempts     INTEGER NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE key_exchanges (
    id                      TEXT PRIMARY KEY,
    household_id            TEXT NOT NULL REFERENCES households(id),
    invite_code             TEXT,
    joiner_name             TEXT,           -- device name before device is created
    joiner_public_key       BYTEA,
    encrypted_household_key BYTEA,
    device_token            TEXT,
    device_id               TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed               BOOLEAN NOT NULL DEFAULT false
);

CREATE TABLE blobs (
    household_id TEXT NOT NULL REFERENCES households(id),
    hash         TEXT NOT NULL,  -- SHA-256 hex
    data         BYTEA NOT NULL,
    size_bytes   BIGINT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, hash)
);
```

**Why Cloud SQL (Postgres):**

- Same SQL used everywhere else in the codebase — no learning curve
- Full ACID transactions for atomic seq increment
- Rich tooling: `psql`, `pg_dump`, ad-hoc queries for debugging
- Predictable pricing — fixed instance cost, no per-operation surprises
- Multi-instance Cloud Run works natively via Cloud SQL Auth Proxy
- Local development uses plain Postgres (docker or nix)
- Stays within GCP IAM — Cloud Run service account authenticates
  automatically via Cloud SQL Auth Proxy, no connection strings needed
- The `database/sql` + `pgx` Go ecosystem is mature and well-known

**Instance sizing for v1:**

`db-f1-micro` (shared-core, 0.6 GB RAM, 10 GB SSD) at ~$7-10/month.
Even 1 paying subscriber ($9/month) covers the infrastructure. Upgrade
to `db-g1-small` when connection count or query volume warrants it.

**Connection from Cloud Run:**

Cloud Run connects to Cloud SQL via the built-in Cloud SQL Auth Proxy
sidecar. The connection string uses a Unix socket:

```go
dsn := fmt.Sprintf(
    "host=/cloudsql/%s dbname=%s user=%s sslmode=disable",
    instanceConnectionName, dbName, dbUser,
)
db, err := sql.Open("pgx", dsn)
```

No passwords in environment variables — IAM database authentication
uses the Cloud Run service account identity.

**Monotonic sequence counter:**

```sql
-- Atomic increment using Postgres advisory lock or UPDATE ... RETURNING
UPDATE households
SET seq_counter = seq_counter + 1
WHERE id = $1
RETURNING seq_counter;
```

Single statement, atomic, no application-level locking needed.

**Pull query:**

```sql
SELECT seq, id, device_id, nonce, ciphertext, created_at
FROM ops
WHERE household_id = $1 AND seq > $2
ORDER BY seq ASC
LIMIT $3;
```

The `(household_id, seq)` primary key makes this an efficient index
range scan.

#### Cloud Storage — encrypted document BLOBs (future)

For v1, encrypted BLOBs are stored in the `blobs` Postgres table (see
schema above). This keeps the infrastructure simple — one database for
everything. At the 50 MB max document size and 1 GB household quota,
Postgres handles this comfortably.

If blob storage outgrows Postgres (many households with large document
libraries), migrate to a Cloud Storage bucket keyed by
`{household_id}/{sha256_hash}`. The Store interface abstracts this — the
handler and client code don't change.

For the future Cloud Storage path:

- **Bucket:** `micasa-relay-blobs` (single bucket, objects keyed by
  `{household_id}/{sha256_hash}`)
- **Storage class:** Standard (frequent access during active sync)
- **Lifecycle rule:** Objects in soft-deleted households are moved to
  Nearline after 30 days and deleted after 90 days
- **Encryption:** Objects are already E2E encrypted by the client before
  upload. Google's server-side encryption adds a second layer (defense
  in depth) but is not relied upon.
- **Access:** The Cloud Run service uses its service account — no API keys
  to manage. Clients never talk to Cloud Storage directly; all blob
  operations go through the relay API.

**Quota enforcement:**

Quota is enforced inside `Store.PutBlob()`. The store computes current
usage (`BlobUsage`) and checks whether adding the new blob would exceed
the 1 GB household quota. The check uses `used > quota - newSize`
(safe with signed int64, avoids overflow from `used + newSize > quota`).
Returns `errQuotaExceeded` with current usage and quota values so the
handler can return an actionable 413 response.

### Auth

See Section 11 "Auth model" for full details on token format, storage,
and the rationale for SHA-256 hashing.

- **Device registration:** `micasa pro init` calls `POST /households`,
  relay generates a 256-bit crypto-random hex bearer token, returns it
  to the client, stores only the SHA-256 hash in `devices.token_sha`.
- **Request auth:** every API call includes `Authorization: Bearer <token>`.
  Relay computes `SHA256(token)` for O(1) indexed lookup.
- **Subscription gating:** push/pull/blob endpoints check
  `households.stripe_status = 'active'`. Non-active returns 402 with
  message "Subscription inactive — sync paused." Management endpoints
  (status, devices, invite) work regardless of subscription status.

### Cost model

At launch (0-50 households, low sync frequency):

| Component | Estimated monthly cost |
|-----------|----------------------|
| Cloud Run | $0 (within free tier — 2M requests/month, scales to zero) |
| Cloud SQL | ~$7-10 (`db-f1-micro`, always on) |
| **Total** | **~$7-10/month** |

At moderate scale (500 households):

| Component | Estimated monthly cost |
|-----------|----------------------|
| Cloud Run | $5-15 (beyond free tier, still minimal) |
| Cloud SQL | ~$15-25 (`db-g1-small` + storage) |
| **Total** | **~$20-40/month** |

The $89/year price point per household is >90% margin even at moderate
scale. At 500 households that's ~$44K ARR against ~$500/year in infra.
Even at launch, a single subscriber covers the Cloud SQL cost.

### Monitoring

For v1, use GCP's built-in tooling:

- **Cloud Run metrics:** request count, latency, error rate (built-in)
- **Cloud SQL metrics:** connections, query latency, CPU, storage (built-in)
- **Cloud Logging:** structured logs from the relay binary (automatic
  with Cloud Run)
- **Alerting:** Cloud Monitoring alerts on error rate > 1% and
  Cloud SQL connection count approaching limits

Skip Prometheus/Grafana until the built-in dashboards are insufficient.

### Infrastructure

**Region:** `us-east1` (South Carolina). Single region is fine for v1 —
the data is small and the sync protocol tolerates latency.

**Container builds:** Use Cloud Build or `docker build` locally and push
to GCR/Artifact Registry. The Cloud Run service pulls the image on deploy.

Infrastructure-as-code tooling TBD — will be sorted out during actual
deployment.

---

## 15. Build Plan

### Done

All core library/API code is implemented and tested:

- **ULID migration** — `internal/uid`, GORM hooks, schema migration
- **Change tracking** — `sync_oplog_entries` table, GORM hooks, CASCADE
  handling, context-flag suppression of re-logging during remote apply
- **Encryption** — NaCl secretbox (symmetric), NaCl box (key exchange),
  `internal/crypto` package with Encrypt/Decrypt/NonceSize
- **Relay server API** — full HTTP handler (`internal/relay/handler.go`):
  push/pull, auth (SHA-256 token hashing), household CRUD, invite/join,
  key exchange (single-use credential retrieval), device management
  (revocation sets flag, doesn't delete), subscription gating, Stripe
  webhook (HMAC-SHA256 verification, 503 when no secret configured)
- **Relay Store interface + two implementations**:
  - `MemStore` — in-memory, used for tests and local dev
  - `PgStore` — production Postgres implementation with all 22 Store
    methods, atomic seq increment via `UPDATE ... RETURNING`, `FOR UPDATE`
    locking on invite count, GORM + pgx driver
- **Client sync engine** — push/pull with E2E encryption, LWW conflict
  resolution (timestamp + device ID tiebreaker), transactional
  conflict-check-and-apply, deterministic delete convergence
  (`op.CreatedAt` not `time.Now()`), bounded error body reads
  (`maxErrorBody = 4096`)
- **Document BLOB sync** — content-addressed blob store (SHA-256 key),
  relay endpoints (`PUT`/`GET`/`HEAD /blobs/{household_id}/{hash}`),
  client methods (`UploadBlob`/`DownloadBlob`/`HasBlob`), lazy pull,
  1 GB household quota enforced server-side, 50 MB per-blob limit,
  dedup (409 on existing hash), hash validation (64 lowercase hex chars)
- **CLI commands** (`cmd/micasa/pro.go`): `pro init`, `pro status`,
  `pro sync`, `pro invite`, `pro join <code>`, `pro devices`,
  `pro devices revoke <id>`, `pro conflicts`, `pro storage`. All wired
  to real relay HTTP calls via the sync Client.
- **Payment gating** — 402 on push/pull when subscription inactive,
  webhook signature verification (manual HMAC-SHA256)

### Remaining (v1)

- **Relay binary wiring** — `cmd/relay/main.go` currently uses MemStore.
  Wire to PgStore via `DATABASE_URL` env var, read Stripe webhook secret
  from env/Secret Manager, configure graceful shutdown.
- **TUI sync integration** — background sync goroutine (startup pull+push,
  periodic 60s, debounced push on mutation), status bar indicator
  (`◈` synced, `◉` syncing, `○` offline, `!` conflict).
- **Infrastructure** — Cloud SQL instance, Cloud Run service, IAM, Secret
  Manager, container image build. Deploy to `us-east1`. IaC tooling TBD.
- **Stripe account setup** — create products/prices, configure webhook
  endpoint URL, test end-to-end subscription flow.

### Post-v1

- **Key rotation** — `micasa pro keys rotate` (see Section 11 for protocol
  details). Expensive operation that re-encrypts all relay data.
- **Join-with-existing-data merge** — household HouseProfile wins, local
  backup created before merge, local-only records pushed as new ops
- **Rate limiting** — relay endpoint rate limiting handled at GCP
  infrastructure level (Cloud Armor / API Gateway), not application code
- **Oplog compaction** — prune old oplog entries after all devices have
  confirmed receipt, reducing storage and pull times
- **Data retention automation** — automated cleanup of encrypted data for
  lapsed subscribers after the 90-day grace period
- **Landing page** — marketing site and email waitlist

---

## 16. What to NOT Build

- Web or mobile client (sync is between micasa TUI instances only)
- Real-time collaboration (sync is eventually consistent, not live)
- Granular permissions (all household members see everything)
- Multi-household support (one household per subscription)
- Proactive alerts/notifications (future feature, separate service)

---

## 17. Privacy Messaging

### For the landing page

> micasa Pro syncs your home data between your devices, encrypted so we
> can't read it. Share with your household. Back up without trusting us.
> Your data is yours — we just move it.

### For the technical docs

> micasa Pro uses NaCl symmetric encryption (XSalsa20-Poly1305) with a
> household key that never leaves your devices. Every sync operation is
> encrypted client-side before transmission. The relay server stores
> opaque ciphertext it cannot decrypt. Document files are encrypted
> separately and stored in content-addressed blob storage. The server
> knows that sync is happening, but not what is being synced.
>
> When you invite a household member, the household key is transferred
> via an asymmetric key exchange (Curve25519). The relay facilitates the
> exchange but never sees the key. If a device is compromised, you can
> rotate the household key — all relay data is re-encrypted client-side
> and the compromised device is revoked.

---

## 18. Open Questions

- **Schema drift:** If two devices run different micasa versions with
  different schemas, ops may reference columns that don't exist on the
  other device. Current approach: ops carry full row snapshots as JSON.
  The receiving client applies via `map[string]any`, so unknown columns
  are silently ignored by GORM (only struct-mapped columns are written).
  Unknown keys are preserved in the oplog `payload` field for audit.
  A post-v1 improvement would unmarshal into typed model structs instead
  of `map[string]any`, making unknown-key handling explicit.

- **Max household size:** 4 devices max for v1. Enforced at join time
  by counting non-revoked devices. Real households rarely exceed 2-3
  people, each with 1-2 devices.

- **Subscription expiry and data retention:** When payment lapses, sync
  stops (402 on push/pull) but local data is unaffected. The relay holds
  encrypted data for 90 days (grace period). Re-subscribing within 90
  days resumes sync with no data loss. After 90 days, all household data
  on the relay (ops, blobs, device records, invites) is permanently
  deleted. **Post-v1:** automated cleanup job. For v1, manual purge via
  admin tooling.

- **Async key exchange UX:** The join flow requires the inviter's client
  to be online to complete the key exchange (the inviter must encrypt
  the household key with the joiner's public key). Set a clear
  expectation in the UX ("Have both devices online when joining"). The
  CLI `pro invite` command polls for the joiner and completes the
  exchange interactively. Add a timeout with a clear error message if
  the joiner doesn't appear within 15 minutes.

- **Key exchange record expiry:** Key exchange records on the relay
  should expire (e.g., 1 hour after creation) to limit the window
  during which encrypted credentials sit on the server. Not yet
  implemented.

- **Oplog compaction:** The oplog grows indefinitely. For long-lived
  households, old entries should be prunable once all devices have
  confirmed receipt (i.e., all devices' `last_seq` exceeds the entry's
  `seq`). **Post-v1:** implement a compaction command or automatic
  background compaction with a configurable retention period.
