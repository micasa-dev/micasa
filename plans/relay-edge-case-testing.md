<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Relay Edge Case Testing

Comprehensive, store-polymorphic edge case test suite for the relay server.
Runs the same tests against MemStore and PgStore to catch behavioral divergence,
plus HTTP-layer edge cases through the Handler.

## Architecture

Two `testify/suite` structs, each run once per store backend:

- **`StoreSuite`** -- Store interface contract tests. Calls store methods
  directly. Catches MemStore/PgStore behavioral drift.
- **`HandlerSuite`** -- HTTP edge cases through `ServeHTTP`. Tests status
  codes, headers, body limits, auth parsing, subscription gating.

PgStore runners skip when `RELAY_POSTGRES_DSN` is unset (same gate as today).
Existing test files stay untouched. The suites are additive.

### File Layout

```
internal/relay/
  store_suite_test.go       StoreSuite struct + all store contract edge cases
  handler_suite_test.go     HandlerSuite struct + all HTTP edge cases
  suite_helpers_test.go     Time manipulation helpers + suite-specific helpers
```

### Suite Struct Design

```go
type StoreSuite struct {
    suite.Suite
    newStore func(t *testing.T) Store  // immutable factory
}

type HandlerSuite struct {
    suite.Suite
    newHandler func(t *testing.T, opts ...HandlerOption) (*Handler, Store) // immutable factory
}
```

No mutable `store` or `handler` fields on the suite. Each test method calls
the factory to get its own instances. This avoids the race condition where
parallel test methods overwrite shared suite fields via `SetupTest`.

### Parallel Safety

Two levels of parallelism:

**Runner level**: MemStore runners use `t.Parallel()` (no shared state).
PgStore runners do NOT (both truncate tables via `openTestPgStore` — running
concurrently would wipe each other's data).

**Method level**: Each test method within a suite captures `t := s.T()`,
calls `t.Parallel()`, and obtains its own store/handler from the factory.
Safe because:

- **MemStore**: Factory returns a fresh `MemStore` per call. Each test gets
  its own store instance -- no shared mutable state.
- **PgStore**: Factory returns a shared `*PgStore` (opened once in the test
  runner, before `suite.Run`). PgStore is thread-safe (GORM manages its own
  connection pool). Each test creates fresh households with unique ULID-based
  IDs, so tests operate on disjoint data within the same DB.
- **HandlerSuite with PgStore**: Factory wraps the shared PgStore in a new
  `Handler` per test (Handler is stateless beyond its store reference).

### Suite Runners

```go
func TestStoreMemStore(t *testing.T) {
    t.Parallel()
    suite.Run(t, &StoreSuite{
        newStore: func(t *testing.T) Store {
            s := NewMemStore()
            s.SetEncryptionKey(defaultTestEncryptionKey)
            return s
        },
    })
}

func TestStorePgStore(t *testing.T) {
    // NOT parallel: openTestPgStore truncates tables. Running two PgStore
    // runners concurrently would cause the second truncation to wipe data
    // created by the first runner's tests.
    dsn := os.Getenv("RELAY_POSTGRES_DSN")
    if dsn == "" {
        t.Skip("RELAY_POSTGRES_DSN not set")
    }
    pgStore := openTestPgStore(t)
    suite.Run(t, &StoreSuite{
        newStore: func(_ *testing.T) Store { return pgStore },
    })
}

func TestHandlerMemStore(t *testing.T) {
    t.Parallel()
    suite.Run(t, &HandlerSuite{
        newHandler: func(t *testing.T, opts ...HandlerOption) (*Handler, Store) {
            s := NewMemStore()
            s.SetEncryptionKey(defaultTestEncryptionKey)
            return NewHandler(s, slog.Default(), opts...), s
        },
    })
}

func TestHandlerPgStore(t *testing.T) {
    // NOT parallel: shares Postgres with TestStorePgStore (see note above).
    dsn := os.Getenv("RELAY_POSTGRES_DSN")
    if dsn == "" {
        t.Skip("RELAY_POSTGRES_DSN not set")
    }
    pgStore := openTestPgStore(t)
    suite.Run(t, &HandlerSuite{
        newHandler: func(_ *testing.T, opts ...HandlerOption) (*Handler, Store) {
            return NewHandler(pgStore, slog.Default(), opts...), pgStore
        },
    })
}
```

Each PgStore runner calls `openTestPgStore` which truncates all tables. Since
the runners are sequential, each starts with a clean DB. Within a runner,
tests create fresh data with unique IDs and don't truncate again.

### Test Method Pattern

**Critical**: capture `t := s.T()` before calling `t.Parallel()`. After
`Parallel()` returns, the suite runner restores the parent T and proceeds
to the next method. Calling `s.T()`, `s.Require()`, or `s.Assert()` after
`Parallel()` may return another test's T (the `sync.RWMutex` in testify
v1.11.1 prevents data races but not this logical race). Use `require` and
`assert` with the captured `t` directly.

```go
func (s *StoreSuite) TestPullLimit1() {
    t := s.T()
    t.Parallel()
    store := s.newStore(t)
    ctx := t.Context()

    resp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{...})
    require.NoError(t, err)
    // ... test logic using t, store, and resp
}

func (s *HandlerSuite) TestEmptyBody() {
    t := s.T()
    t.Parallel()
    h, _ := s.newHandler(t)

    req := httptest.NewRequest("POST", "/households", nil)
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    assert.Equal(t, http.StatusBadRequest, rec.Code)
}
```

### Helpers

**Existing helpers** (already in `handler_test.go`, same package):
- `authRequest(method, path string, body any, token string) *http.Request`
- `createTestHousehold(t *testing.T, h *Handler) CreateHouseholdResponse`

**New standalone helpers** (`suite_helpers_test.go`):

All take `t testing.TB` as first argument to avoid the `s.T()` parallel race.

- `suiteCreateHousehold(t testing.TB, store Store) CreateHouseholdResponse`
- `suiteJoinDevice(t testing.TB, store Store, hhID, inviterDevID string) (deviceID, token string)`
  -- full invite -> join -> complete -> poll flow
- `suiteActivateSubscription(t testing.TB, store Store, hhID string)`

### Time-Dependent Tests

Several edge cases involve expiry (invites: 4h, key exchanges: 15min). Since
neither store exposes a clock interface, tests use backend-agnostic helper
functions that type-switch on the concrete store:

```go
// expireInvite sets an invite's expiry to the past (test-only).
func expireInvite(t testing.TB, store Store, code string) {
    t.Helper()
    switch s := store.(type) {
    case *MemStore:
        s.mu.Lock()
        s.invites[code].expiresAt = time.Now().Add(-time.Hour)
        s.mu.Unlock()
    case *PgStore:
        require.NoError(t, s.db.Exec(
            "UPDATE invites SET expires_at = ? WHERE code = ?",
            time.Now().Add(-time.Hour), code,
        ).Error)
    default:
        t.Fatalf("unsupported store type %T", store)
    }
}

// expireKeyExchange sets an exchange's created_at to >15min ago (test-only).
func expireKeyExchange(t testing.TB, store Store, exchangeID string) { /* analogous */ }
```

These are test-only helpers in `suite_helpers_test.go`. No production code
changes. Each time-dependent test notes "(requires time manipulation)" in
the tables below.

## Known Behavioral Divergences

The store suite is designed to surface these. Document rather than hide them.

| Area | MemStore | PgStore |
|------|----------|---------|
| Push duplicate op IDs | No dedup -- appends blindly | Unique index `(id, household_id)` rejects duplicates |
| AuthenticateDevice | No last_seen update | Updates `last_seen` atomically via `RETURNING` |
| CreateInvite serialization | Mutex (process-wide) | `FOR UPDATE` row lock (per-connection) |
| PutBlob serialization | Mutex | `FOR UPDATE` on household row |

The "push duplicate op IDs" divergence is the most significant. This is
tested as **backend-specific tests** in `store_suite_test.go` (not part of the
shared suite): `TestMemStorePushDuplicateAllowed` and
`TestPgStorePushDuplicateRejected`, each asserting the backend's actual
behavior. This documents the design choice rather than forcing artificial
agreement.

## Store Suite Edge Cases

### Push/Pull

| Test | What it verifies |
|------|-----------------|
| Pull limit=1 | Minimum valid limit, hasMore flag correct |
| Pull limit<=0 silently clamps to 100 | Both stores default to 100 (handler rejects before this reaches store) |
| Pull afterSeq beyond all ops | Returns empty slice, hasMore=false |
| Pull excludes own device | Ops from the requesting device are filtered out |
| Two devices push concurrently | Seqs monotonic, no gaps (goroutines within one test) |
| Pull pagination across pages | Pull with limit=N, then after=lastSeq -- sequences contiguous, no duplicates |
| Joined device pulls history | Sees all ops pushed before its join |
| Push large batch (100 ops) | Ordering preserved in confirmations, seqs sequential |

### Invite/Join

| Test | What it verifies |
|------|-----------------|
| StartJoin wrong household ID | Returns error without incrementing invite attempts |
| StartJoin after invite expires | Rejected (requires time manipulation) |
| Fifth attempt consumes invite | Exactly at maxInviteAttempts boundary, invite marked consumed |
| Concurrent StartJoin same code | At most maxInviteAttempts exchanges created (goroutines, see note below) |
| CreateInvite after active expires | 4th invite allowed once an existing one expires (requires time manipulation) |

**Concurrent StartJoin note**: PgStore's `StartJoin` lacks `FOR UPDATE` on the
invite row. Under READ COMMITTED, two transactions can both read `attempts=4`,
both pass the `< maxInviteAttempts` check, and both create exchanges — resulting
in more exchanges than the limit allows. This test should count exchanges
created (not the attempts counter) and may surface a real race condition that
needs fixing with row-level locking.

### Key Exchange

| Test | What it verifies |
|------|-----------------|
| GetKeyExchangeResult after expiry | Returns error (requires time manipulation on createdAt) |
| GetKeyExchangeResult twice | Second call returns "credentials already consumed" error |
| Concurrent GetKeyExchangeResult | Exactly one caller gets credentials, others get error (goroutines) |
| CompleteKeyExchange already completed | Returns "already completed" error |
| CompleteKeyExchange wrong household | Returns "does not belong" error |
| Joined device push/pull immediately | New device can sync right after exchange completes |

### Blobs

| Test | What it verifies |
|------|-----------------|
| PutBlob exactly at quota (used + data == quota) | Succeeds (boundary: `used > quota-len(data)` is false when equal) |
| PutBlob one byte over quota | Fails with errQuotaExceeded |
| PutBlob quota=0 | Unlimited, quota check skipped entirely |
| PutBlob same hash different households | Both succeed (no cross-household dedup) |
| GetBlob / HasBlob non-existent hash | Returns errBlobNotFound / false |
| BlobUsage empty household | Returns 0 |
| PutBlob zero-length body | Allowed -- both stores accept it, usage increases by 0 |
| GetBlob round-trip | Exact bytes match what was uploaded |
| BlobUsage accuracy after multiple puts | Sum of individual blob sizes matches BlobUsage result |

### Device Management

| Test | What it verifies |
|------|-----------------|
| Revoke then authenticate | AuthenticateDevice fails for revoked device's token |
| Revoke N-1 of N devices | Remaining device still authenticates and can push/pull |
| ListDevices excludes revoked | Only non-revoked devices returned |

### Data Partitioning by Household

The store trusts the caller with householdID -- there is no auth layer at
this level. These tests verify that data is correctly keyed by householdID
so that queries with the wrong ID return empty/not-found, not another
household's data.

| Test | What it verifies |
|------|-----------------|
| GetBlob with wrong householdID | Returns errBlobNotFound |
| Pull with wrong householdID | Returns empty ops slice |
| RevokeDevice with wrong householdID | Returns "does not belong" error |
| CreateInvite for non-existent household | Returns "not found" error |

### Subscription State (Store-Level)

Store-level tests verify data persistence only. The store has NO subscription
gating -- that is handler middleware (`requireSubscription`).

| Test | What it verifies |
|------|-----------------|
| UpdateSubscription round-trip | GetHousehold reflects new status |
| Status transitions: null -> active -> canceled -> active | Each UpdateSubscription persists correctly |
| HouseholdBySubscription unknown ID | Returns error |
| HouseholdByCustomer unknown ID | Returns error |
| UpdateSubscription non-existent household | Returns error |
| UpdateCustomerID empty string | Returns error (both stores validate this) |

### Auth

| Test | What it verifies |
|------|-----------------|
| Same token authenticated repeatedly | Returns same device each time, no error |
| Authenticate empty string | Returns error (SHA-256 of "" not in any token index) |

## Handler Suite Edge Cases

### Request Validation

JSON endpoints (households, push, join, key-exchange/complete) use
`io.LimitReader` which silently truncates at 1 MiB — the truncated body
fails JSON decoding with 400. The webhook and blob endpoints use
`http.MaxBytesReader` which returns a hard 413 on oversized bodies.

| Test | What it verifies |
|------|-----------------|
| POST all JSON endpoints with empty body | 400, no panic (EOF from JSON decoder) |
| POST JSON endpoint with body > 1 MiB | 400 "invalid request body" (LimitReader truncates, JSON decode fails) |
| PUT blob exceeding 50 MiB (maxBlobSize = 50<<20) | 413 via MaxBytesReader |

### Query Parameter Boundaries

| Test | What it verifies |
|------|-----------------|
| Pull after=non-numeric | 400 "invalid after parameter" |
| Pull limit=non-numeric | 400 "limit must be 1-1000" |
| Pull limit=0 | 400 (handler rejects n < 1 before store's silent clamp) |
| Pull limit=1001 | 400 (handler rejects n > 1000) |

### Auth Edge Cases (HTTP)

| Test | What it verifies |
|------|-----------------|
| "Bearer" (no trailing space) | HasPrefix("Bearer ") fails -> 401 |
| "Bearer " (trailing space, no token) | extractBearerToken returns "" -> 401 |
| "bearer token" (lowercase b) | HasPrefix("Bearer ") fails -> 401 |
| "Bearer  token" (double space) | Extracts " token" (leading space), hash not found -> 401 |
| No Authorization header | 401 |
| 10KB bearer token | SHA-256 handles arbitrary length, hash not found -> 401 |

### Blob HTTP Semantics

| Test | What it verifies |
|------|-----------------|
| PUT with uppercase hex hash | 400 (sha256Re requires `[0-9a-f]{64}`) |
| PUT with 63-char hash (one short) | 400 (regex requires exactly 64 chars) |
| HEAD existing blob | 200, empty body |
| HEAD non-existent blob | 404, empty body |
| GET sets Content-Type: application/octet-stream | Header present on successful response |

### Subscription Gating (HTTP)

Gating is handler-only (`requireSubscription` middleware). The store has no
concept of subscription enforcement.

| Test | What it verifies |
|------|-----------------|
| Push/pull/blob with canceled subscription | 402 Payment Required |
| Same endpoints after reactivation to "active" | 200 OK |
| Self-hosted mode bypasses all gating | No 402 on any endpoint |
| Null subscription status (no Stripe configured) | Allowed through (dev/free mode) |

### Cross-Household Access (HTTP)

Unlike store-level partitioning, the handler enforces auth: the device's
householdID comes from `AuthenticateDevice`, not from the URL. These test
that a device can't access another household's resources by putting a
different household ID in the URL path.

| Test | What it verifies |
|------|-----------------|
| PUT/GET/HEAD blob with other household's ID in URL | 403 Forbidden |
| Create invite for other household's ID in URL | 403 Forbidden |
| List/revoke devices for other household | 403 Forbidden |

### Stripe Webhook (HTTP)

All webhook tests require `WithWebhookSecret("test-secret")` on the handler
(otherwise the handler returns 503 before checking signatures). The "body at
1 MiB + 1" test is the exception — MaxBytesReader rejects before the secret
check.

| Test | What it verifies |
|------|-----------------|
| Replay: same valid signature sent twice | Both succeed (idempotent) |
| Non-subscription event type | 200 with `{"status":"ignored"}` |
| Missing Stripe-Signature header | 400 (VerifyWebhookSignature fails on empty header) |
| Body at exactly 1 MiB (1<<20 bytes) | Not rejected for size (response depends on signature/content validity) |
| Body at 1 MiB + 1 byte | 413 via MaxBytesReader (fires before secret check) |
