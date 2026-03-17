<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Deep Design Review: Micasa Pro Sync System

Date: 2026-03-17

---

## 1. TRUST MODEL

**Trust Boundaries:**

The system defines three trust zones:

- **Client (Local)**: Fully trusted. Owns household key, generates ops.
- **Relay (Server)**: Untrusted by design. Cannot decrypt ops; routes encrypted data only.
- **Network**: Untrusted. Assume TLS mitigates passive observation but not active MITM at the HTTP level.

**What Each Component Trusts:**

1. **Relay trusts:**
   - Device token bearer (authentication) -- but only after salted hash comparison; plaintext tokens never cross the wire
   - Household membership claims from authenticated devices (encoded in token; no re-validation per request)
   - Op sequence numbers it generates internally (via `seq_counter` increments in transaction)
   - Stripe webhooks (signature verification required; can be disabled for testing)

2. **Client trusts:**
   - Relay to maintain causal ordering (seq monotonicity)
   - Relay to store blobs with correct hash keys (no integrity guarantee; client-side verification occurs post-decryption)
   - Other devices' public keys retrieved via household key exchange

3. **Household members trust:**
   - Each other via the shared household key (symmetric, not rotated per device)
   - The inviter to transmit the key securely during key exchange

**Trust Violations & Failure Modes:**

| Violation | Impact |
|-----------|--------|
| Malicious relay replays old ops | Mitigated by seq deduplication (primary key). Duplicate inserts fail; updates applied twice. |
| Malicious relay reorders ops | **HIGH RISK**: Seq ordering is not cryptographically signed. Relay can silently reorder within causal order. This breaks determinism if two ops touch the same row. |
| Malicious relay drops ops | Silent data loss. Clients pull with `after=seq` and have no way to detect dropped sequences. |
| Malicious relay forges device_id | **Mitigated by transaction**: Relay forces `req.Ops[i].DeviceID = dev.ID` and authenticates token first; injected ops cannot spoof another device's ID. |
| Compromised household key | Catastrophic: All ops visible, all blobs decryptable. Single-device compromise leaks household to attacker. |
| Rogue household member | Symmetric key shared; cannot revoke member's access post facto (only via device revocation, which stops future ops). |

---

## 2. CRYPTOGRAPHIC DESIGN

**NaCl Secretbox Usage:**

- **Cipher**: XSalsa20-Poly1305 (via `golang.org/x/crypto/nacl/secretbox`)
- **Key**: 256-bit symmetric household key (generated fresh, stored in `~/.local/share/micasa/secrets/household.key` with 0600 perms)
- **Nonce**: 24-byte random, generated fresh per Encrypt() call via `crypto.Rand`
- **Format**: Envelope stores nonce + ciphertext separately; client reconstructs as `nonce || ciphertext` for decryption

**Cryptographic Strengths:**

1. **Nonce handling is correct**: Each message gets a fresh 24-byte random nonce. The probability of collision is negligible (birthday paradox requires ~2^96 messages). Code explicitly does `rand.Read(nonce[:])` with no reuse.

2. **AEAD authentication** provides integrity + confidentiality. Tampering with ciphertext fails at decryption with `"invalid key or tampered ciphertext"`.

3. **Key zeroization** (via `runtime.KeepAlive` and `clear()`) attempts to prevent key copies in memory, though Go's GC does not guarantee stack copies are zeroed.

**Cryptographic Risks:**

1. **Symmetric key shared across devices**: All devices in a household share one `HouseholdKey`. A compromised device exposes all household data forever. No per-device keying, no forward secrecy, no key rotation mechanism.

2. **Key exchange over plaintext asymmetric crypto**: During onboarding, joiner sends plaintext public key to relay. Inviter retrieves it and computes ECDH. Encrypted household key is sent to relay. **Risk**: A compromised relay could substitute its own public key (MITM). Mitigated in v1 by controlling the relay infrastructure.

3. **No forward secrecy on ops**: All historical ops encrypted with the same household key. If the key is compromised, the attacker can decrypt all history.

4. **Blob hash is plaintext SHA-256**: Relay stores blobs by hash of plaintext. This leaks information (identical blobs deduplicate; attacker can infer whether two households have the same file by comparing hashes).

---

## 3. SYNC PROTOCOL CORRECTNESS

**Core Mechanism:**

- **Local oplog**: Clients maintain `sync_oplog_entries` with (table_name, row_id, op_type, payload, device_id, created_at).
- **Push**: Client encrypts ops, sends to relay with bearer token.
- **Relay seq**: Increments `households.seq_counter` atomically per op in a transaction; assigns this seq to each op.
- **Pull**: Client fetches ops with `after=seq` parameter; relay returns ops with seq > after, in order.

**Potential Data Loss Scenarios:**

1. **Relay silently drops an op**: Seq jumps from 5 to 7. Client calling `Pull(after=5)` gets seq 7. Gap is invisible unless client tracks the last known seq and asserts no gaps. **Risk: Data loss goes undetected.**

2. **Client crashes before pulling**: Op is on relay but client never fetches. On restart, client calls `Pull(after=lastSeq)` (stored locally). If the op has already been dropped by relay (TTL/cleanup), it's lost. **No TTL documented; likely unbounded storage.**

3. **Op deduplication**: Primary key is `(seq, household_id)`. Secondary unique constraint is `(id, household_id)`. If a client retransmits with the same `op.ID`, the second insert fails (duplicate key). Client must implement idempotent push.

**Critical Issue: Causal Order is NOT Guaranteed**

The relay sorts operations by arrival time (implicit in the transaction counter), not by `created_at` timestamp. This can cause:

- **Scenario**: Device A updates project title at 10:00. Device B creates maintenance item for that project at 10:01. If B's op arrives at relay before A's op, seq(B) < seq(A). When applied to a fresh device: B's insert tries to create a maintenance item for a project that doesn't exist yet. Fails with "project not found."
- **Current mitigation**: ApplyOps sorts by seq; `applyOne()` returns an error if the row doesn't exist. Error is collected in `ApplyResult.Errors` but does not halt sync.

**Verdict**: Ops can fail to apply silently. Client gets `ApplyResult.Errors` but has no mechanism to retry or reorder. This is a protocol gap, though in practice the single-household 1-2 person scenario makes this very unlikely.

---

## 4. CONFLICT RESOLUTION

**LWW Implementation:**

```go
func lwwLocalWins(localTime, remoteTime, localDevice, remoteDevice) bool {
  if localTime.Equal(remoteTime) return localDevice >= remoteDevice
  return localTime.After(remoteTime)
}
```

Tiebreaker: lexicographic device ID.

**Conflict Scenarios:**

| Scenario | Outcome | Correctness |
|----------|---------|-------------|
| **Insert/Insert** (same row_id) | Remote insert fails if row exists locally. | OK -- ULID collision is negligible. |
| **Update/Update** (same row_id) | LWW: later created_at wins. Loser's entire payload is discarded. | Correct but coarse-grained (no per-field merge). |
| **Delete/Update** (same row_id) | LWW applied: later timestamp wins. | Correct. |
| **Delete/Restore** (same row_id) | LWW applied: later timestamp wins. | Correct. |
| **Unique constraint** (two devices insert same vendor name) | Not handled by LWW. Second insert fails silently. | **GAP**: Can cause divergent state. |

**Critical: Unique Constraint Violations Are Unhandled in Code**

The spec describes a merge resolution for unique constraints (Section 10), but the implementation does not handle this case. Two devices inserting vendors with the same name will cause one device to see both, another to see only one.

---

## 5. DATA INTEGRITY

**Invariants Maintained:**

1. Per-household seq monotonicity
2. Device membership isolation
3. Op immutability on relay (inserted, never updated)
4. Encryption at rest (client-side before PUT)

**Invariants NOT Maintained:**

1. Global causal order (ops ordered by arrival, not creation)
2. FK referential integrity across devices at apply time
3. Unique constraint uniqueness at apply time
4. Exactly-once application (updates can be applied twice from stale ops)

---

## 6. SCALABILITY CONCERNS

**Storage Projections:**
- 1000 households, 5 ops/day each, 1 year: ~1.8M ops, ~360 MB
- Blob storage: 1000 households * 1 GB = 1 TB (exceeds small server disk)

**Major Concern: No Automatic Data Expiration**
- Relay has no TTL on ops, pending exchanges, or old invites
- Stale data accumulates indefinitely
- Post-v1 compaction mechanism needed

**Concurrency:**
- Blob quota check-then-write race mitigated by transaction
- Seq counter overflow: int64 saturates after ~292 billion years (not a concern)

---

## 7. PRODUCT ARCHITECTURE

**Model Soundness:**
- Local-first: clients are fully functional offline
- Relay is optional convenience, not dependency
- Free tier uses no relay; Pro adds sync infrastructure

**UX Gaps:**
1. No network observability (user can't see "synced up to seq X")
2. No conflict notifications (LWW silently picks winner)
3. No op failure visibility (FK violations logged but not shown)
4. Key exchange requires manual invite code sharing

**Single-User Multi-Device**: Works correctly. Desktop edits sync to laptop via relay.

**Household Sharing**: Works correctly for the 1-2 person, low-conflict scenario the product targets.

---

## 8. CRITICAL DESIGN ISSUES & RECOMMENDATIONS

### Issue #1: Causal Order Is Not Enforced
**Severity**: HIGH (theoretical), LOW (practical for target users)

Ops ordered by relay arrival, not creation time. FK violations possible if ops arrive out of order. In practice, a 1-2 person household editing different rows makes this very unlikely.

**Recommendation**: Accept and document. Add retry logic for failed applies in post-v1.

### Issue #2: Unique Constraint Violations Cause Silent Divergence
**Severity**: HIGH

Two devices can insert rows with the same unique key. Second insert fails silently.

**Recommendation**: Implement the merge resolution described in spec Section 10, or document as known limitation.

### Issue #3: No Op Loss Detection
**Severity**: MEDIUM

Relay can silently drop ops. No gap detection.

**Recommendation**: Client tracks last synced seq and compares against relay's total op count.

### Issue #4: Household Key Compromise Is Catastrophic
**Severity**: HIGH (long-term)

Single symmetric key, no rotation, no per-device keying.

**Recommendation**: Implement key rotation (post-v1, protocol outlined in spec Section 11).

### Issue #5: No Replay Detection
**Severity**: MEDIUM

Stale ops can be re-applied. Updates may execute twice.

**Recommendation**: Client-side dedup by op.ID.

### Issue #6: Pending Key Exchanges Never Expire
**Severity**: LOW

Incomplete exchanges accumulate forever.

**Recommendation**: Add TTL (1 hour) on pending exchanges.

---

## 9. VERDICT

The system is **functionally correct for the target scenario** (1-2 person household, low-conflict, small data). Design gaps exist for concurrent multi-user scenarios, unique constraints, and long-term key management, but these are acceptable for MVP given the product's target user profile.

**Suitable for v1 launch** with documented limitations. Post-v1 priorities: key rotation, unique constraint handling, op loss detection, oplog compaction.
