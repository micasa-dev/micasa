<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Spec vs Implementation Audit

Date: 2026-03-17

---

## 1. SPEC CLAIMS THAT ARE ACCURATE

- **Encryption**: NaCl secretbox (XSalsa20-Poly1305) via `golang.org/x/crypto/nacl/secretbox` -- `internal/crypto/encrypt.go`
- **ULID PKs**: All models use `gorm:"primaryKey;size:26"` with string IDs -- `internal/uid`
- **Oplog tracking**: Context flag suppression works -- `internal/data/oplog.go`
- **Sync oplog table schema**: `sync_oplog_entries` with correct columns and indices
- **Sync device singleton**: `sync_device` table with ID, name, household_id, relay_url, last_seq
- **E2E encryption envelope**: `internal/sync/types.go` defines Envelope correctly
- **LWW conflict resolution**: `lwwLocalWins()` in `internal/sync/apply.go`
- **Deterministic delete**: `applyDelete()` uses `op.CreatedAt` not `time.Now()`
- **Document blob handling**: Oplog payloads exclude Data field; `blob_ref` stripped before DB write
- **Content-addressed blob store**: SHA-256 key, 50 MB limit, 1 GB quota, dedup via 409
- **Invite code**: Base32-encoded 8 bytes = 13 chars, 24h expiry, rate-limited
- **Device revocation**: `revoked` flag (not deleted), fails auth
- **Auth model**: SHA-256 token hashing, O(1) indexed lookup
- **Subscription gating**: 402 on push/pull when inactive
- **All relay endpoints**: Implemented and wired in handler.go
- **Stripe webhook**: HMAC-SHA256 verification, 503 when no secret
- **All CLI commands**: pro init/status/sync/invite/join/devices/revoke/conflicts/storage
- **Bounded error reads**: `maxErrorBody = 4096`, `readErrorBody()` with LimitReader
- **Table allowlist**: `allowedSyncTable()` in apply.go
- **Insert ID validation**: `validateInsertPayloadID()` in apply.go
- **Transactional apply**: Single tx per op in `applyOne()`
- **PgStore**: All 22 Store interface methods, atomic seq, FOR UPDATE locking
- **MemStore**: Complete in-memory implementation for tests

## 2. SPEC CLAIMS THAT ARE INACCURATE OR STALE

**Issue 1: Relay binary still uses MemStore**
- `cmd/relay/main.go:28` uses `relay.NewMemStore()`
- Not production-ready; all data lost on restart

**Issue 2: SyncDevice has extra RelayURL field**
- `internal/data/models.go` includes `RelayURL` not mentioned in spec
- Non-breaking, but spec is out-of-date

**Issue 3: Envelope includes HouseholdID/DeviceID in JSON struct**
- `internal/sync/types.go:12-13` has these fields in Envelope
- Spec says they come from auth context only
- Handler does override: `req.Ops[i].DeviceID = dev.ID`

**Issue 4: Missing CASCADE handling for ServiceLogEntry**
- Spec Section 5 describes explicit CASCADE child enumeration
- No implementation found in oplog hooks
- Risk: hard-delete of MaintenanceItem loses ServiceLogEntry oplog entries

## 3. IMPLEMENTATION GAPS

**Gap 1: TUI sync integration** -- No background sync goroutine, no status bar indicator

**Gap 2: Key rotation** -- No `pro keys rotate` command (acknowledged post-v1)

**Gap 3: Join-with-existing-data** -- Requires fresh DB (acknowledged post-v1)

**Gap 4: Oplog compaction** -- No cleanup mechanism (acknowledged post-v1)

**Gap 5: Key exchange record expiry** -- No TTL on pending exchanges

## 4. IMPLEMENTATION EXTRAS (not in spec)

- **Device last_seen tracking**: `pgstore.go` pgDevice has `LastSeen *time.Time`
- **Invite consumed flag**: Explicit `Consumed bool` field
- **SyncDevice RelayURL**: For future multi-relay support

## 5. CODE QUALITY CONCERNS

**CRITICAL: Relay main.go hardcoded to MemStore** (`cmd/relay/main.go:28`)

**HIGH: Missing CASCADE handling** -- ServiceLogEntry deletes bypass GORM hooks on hard-delete

**MEDIUM: Key exchange credentials never expire** -- Sit on relay indefinitely

**MEDIUM: No device max enforcement beyond join time** -- Concurrent joins could exceed limit

**LOW: Error responses may leak implementation details**

## 6. DEPLOYMENT READINESS CHECKLIST

### Critical Blockers

- [ ] Wire DATABASE_URL env var in `cmd/relay/main.go`
- [ ] Switch MemStore to PgStore on startup
- [ ] Read Stripe webhook secret from env/Secret Manager
- [ ] Verify handleStripeWebhook returns 503 when secret empty
- [ ] Test connection pooling and timeouts

### Infrastructure

- [ ] Cloud SQL db-f1-micro in us-east1
- [ ] Cloud Run service with Cloud SQL Auth Proxy sidecar
- [ ] IAM service account for Cloud Run -> Cloud SQL
- [ ] Secret Manager for webhook secret
- [ ] Container image build (Dockerfile or Cloud Build)
- [ ] Health check configuration (GET /health)

### Pre-Launch Testing

- [ ] PgStore end-to-end tests
- [ ] Subscription flow: create -> webhook -> gating
- [ ] Invite/join/key-exchange full flow
- [ ] Blob upload/download/quota
- [ ] Device revocation
- [ ] Conflict resolution scenarios

### Post-Launch (v1.1)

- [ ] TUI background sync integration
- [ ] Key exchange expiry
- [ ] Oplog compaction
- [ ] Key rotation
