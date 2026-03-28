<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Relay Edge Case Testing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a store-polymorphic edge case test suite that runs ~75 tests against both MemStore and PgStore, plus HTTP-layer edge cases through the Handler.

**Architecture:** Two `testify/suite` structs (`StoreSuite`, `HandlerSuite`) each run once per backend. No mutable suite fields — each test method calls an immutable factory to get its own store/handler instance. Time-dependent tests use type-switch helpers that manipulate MemStore fields or PgStore rows directly.

**Tech Stack:** Go, testify/suite, testify/require+assert, httptest, goroutines for concurrency tests

**Spec:** `plans/relay-edge-case-testing.md`

---

### Task 1: Suite Helpers and Scaffolding

**Files:**
- Create: `internal/relay/suite_helpers_test.go`

This task creates the shared helpers and time manipulation functions used by all later tasks. No tests to run yet — just helpers.

- [ ] **Step 1: Create `suite_helpers_test.go` with all helpers**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/stretchr/testify/require"
)

var testPublicKey = []byte("fake-public-key-32-bytes-paddin!")

// suiteCreateHousehold creates a household + device via the Store interface.
func suiteCreateHousehold(t testing.TB, store Store) sync.CreateHouseholdResponse {
	t.Helper()
	resp, err := store.CreateHousehold(t.Context(), sync.CreateHouseholdRequest{
		DeviceName: "test-device",
		PublicKey:  testPublicKey,
	})
	require.NoError(t, err)
	return resp
}

// suiteJoinDevice runs the full invite -> join -> complete -> poll flow,
// returning the new device's ID and raw token.
func suiteJoinDevice(
	t testing.TB, store Store, hhID, inviterDevID string,
) (deviceID, deviceToken string) {
	t.Helper()
	ctx := t.Context()

	invite, err := store.CreateInvite(ctx, hhID, inviterDevID)
	require.NoError(t, err)

	joinResp, err := store.StartJoin(ctx, hhID, invite.Code, sync.JoinRequest{
		DeviceName: "joined-device",
		PublicKey:  testPublicKey,
	})
	require.NoError(t, err)

	err = store.CompleteKeyExchange(ctx, hhID, joinResp.ExchangeID, []byte("fake-enc-key"))
	require.NoError(t, err)

	result, err := store.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
	require.NoError(t, err)
	require.True(t, result.Ready)

	return result.DeviceID, result.DeviceToken
}

// suiteActivateSubscription sets a household's subscription to "active".
func suiteActivateSubscription(t testing.TB, store Store, hhID string) {
	t.Helper()
	err := store.UpdateSubscription(t.Context(), hhID, "sub_test", sync.SubscriptionActive)
	require.NoError(t, err)
}

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
func expireKeyExchange(t testing.TB, store Store, exchangeID string) {
	t.Helper()
	past := time.Now().Add(-20 * time.Minute)
	switch s := store.(type) {
	case *MemStore:
		s.mu.Lock()
		s.exchanges[exchangeID].createdAt = past
		s.mu.Unlock()
	case *PgStore:
		require.NoError(t, s.db.Exec(
			"UPDATE key_exchanges SET created_at = ? WHERE id = ?",
			past, exchangeID,
		).Error)
	default:
		t.Fatalf("unsupported store type %T", store)
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/relay/`
Expected: no errors (test helpers are only used at test time, `go build` won't compile them, but this verifies the main package is clean)

Run: `go vet ./internal/relay/`
Expected: no errors

- [ ] **Step 3: Commit**

```
test(relay): add suite helpers for polymorphic edge case testing
```

---

### Task 2: StoreSuite Scaffold + Push/Pull Edge Cases

**Files:**
- Create: `internal/relay/store_suite_test.go`

This task creates the `StoreSuite` struct, the four suite runners (MemStore parallel, PgStore sequential), and the first group of tests: Push/Pull edge cases.

- [ ] **Step 1: Create `store_suite_test.go` with struct, runners, and Push/Pull tests**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"fmt"
	"os"
	gosync "sync"
	"testing"

	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// StoreSuite tests the Store interface contract against any backend.
type StoreSuite struct {
	suite.Suite
	newStore func(t *testing.T) Store // immutable factory
}

func TestStoreMemStore(t *testing.T) {
	t.Parallel()
	suite.Run(t, &StoreSuite{
		newStore: func(_ *testing.T) Store {
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
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	pgStore := openTestPgStore(t)
	suite.Run(t, &StoreSuite{
		newStore: func(_ *testing.T) Store { return pgStore },
	})
}

// --- Push/Pull ---

func (s *StoreSuite) TestPullLimit1() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	// Push 3 ops from the household's device.
	ops := make([]sync.Envelope, 3)
	for i := range ops {
		ops[i] = sync.Envelope{
			ID:          fmt.Sprintf("op-%d", i),
			HouseholdID: hh.HouseholdID,
			DeviceID:    hh.DeviceID,
			Nonce:       []byte("n"),
			Ciphertext:  []byte("c"),
		}
	}
	_, err := store.Push(ctx, ops)
	require.NoError(t, err)

	// Pull with limit=1 from a different device.
	pulled, hasMore, err := store.Pull(ctx, hh.HouseholdID, "other-device", 0, 1)
	require.NoError(t, err)
	assert.Len(t, pulled, 1)
	assert.True(t, hasMore, "3 ops with limit=1 should have more")
}

func (s *StoreSuite) TestPullLimitZeroClampsTo100() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	// Push 2 ops.
	ops := []sync.Envelope{
		{ID: "a", HouseholdID: hh.HouseholdID, DeviceID: hh.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c")},
		{ID: "b", HouseholdID: hh.HouseholdID, DeviceID: hh.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c")},
	}
	_, err := store.Push(ctx, ops)
	require.NoError(t, err)

	// limit=0 should clamp to default (100), returning all 2 ops.
	pulled, hasMore, err := store.Pull(ctx, hh.HouseholdID, "other", 0, 0)
	require.NoError(t, err)
	assert.Len(t, pulled, 2)
	assert.False(t, hasMore)

	// limit=-1 should also clamp.
	pulled, hasMore, err = store.Pull(ctx, hh.HouseholdID, "other", 0, -1)
	require.NoError(t, err)
	assert.Len(t, pulled, 2)
	assert.False(t, hasMore)
}

func (s *StoreSuite) TestPullAfterSeqBeyondAllOps() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	_, err := store.Push(ctx, []sync.Envelope{
		{ID: "op-1", HouseholdID: hh.HouseholdID, DeviceID: hh.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c")},
	})
	require.NoError(t, err)

	pulled, hasMore, err := store.Pull(ctx, hh.HouseholdID, "other", 9999, 100)
	require.NoError(t, err)
	assert.Empty(t, pulled)
	assert.False(t, hasMore)
}

func (s *StoreSuite) TestPullExcludesOwnDevice() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	_, err := store.Push(ctx, []sync.Envelope{
		{ID: "op-1", HouseholdID: hh.HouseholdID, DeviceID: hh.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c")},
	})
	require.NoError(t, err)

	// Same device pulls — should see nothing.
	pulled, _, err := store.Pull(ctx, hh.HouseholdID, hh.DeviceID, 0, 100)
	require.NoError(t, err)
	assert.Empty(t, pulled)

	// Different device pulls — should see the op.
	pulled, _, err = store.Pull(ctx, hh.HouseholdID, "other-device", 0, 100)
	require.NoError(t, err)
	assert.Len(t, pulled, 1)
}

func (s *StoreSuite) TestConcurrentPushSeqsMonotonic() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)
	dev2ID, _ := suiteJoinDevice(t, store, hh.HouseholdID, hh.DeviceID)

	const opsPerDevice = 20
	var wg gosync.WaitGroup
	wg.Add(2)

	confirmed1 := make([]sync.PushConfirmation, 0, opsPerDevice)
	confirmed2 := make([]sync.PushConfirmation, 0, opsPerDevice)

	pushN := func(devID string, dest *[]sync.PushConfirmation) {
		defer wg.Done()
		for i := range opsPerDevice {
			c, err := store.Push(ctx, []sync.Envelope{{
				ID:          fmt.Sprintf("%s-op-%d", devID, i),
				HouseholdID: hh.HouseholdID,
				DeviceID:    devID,
				Nonce:       []byte("n"),
				Ciphertext:  []byte("c"),
			}})
			if err == nil {
				*dest = append(*dest, c...)
			}
		}
	}

	go pushN(hh.DeviceID, &confirmed1)
	go pushN(dev2ID, &confirmed2)
	wg.Wait()

	// Collect all seqs and verify monotonic with no gaps.
	all := append(confirmed1, confirmed2...)
	seqs := make(map[int64]bool, len(all))
	for _, c := range all {
		seqs[c.Seq] = true
	}
	require.Len(t, seqs, len(all), "every seq should be unique")

	// Verify contiguous from 1..N.
	for i := int64(1); i <= int64(len(all)); i++ {
		assert.True(t, seqs[i], "missing seq %d", i)
	}
}

func (s *StoreSuite) TestPullPaginationContiguous() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	// Push 5 ops.
	ops := make([]sync.Envelope, 5)
	for i := range ops {
		ops[i] = sync.Envelope{
			ID: fmt.Sprintf("op-%d", i), HouseholdID: hh.HouseholdID,
			DeviceID: hh.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
		}
	}
	_, err := store.Push(ctx, ops)
	require.NoError(t, err)

	// Page 1: limit=2.
	page1, hasMore, err := store.Pull(ctx, hh.HouseholdID, "other", 0, 2)
	require.NoError(t, err)
	assert.Len(t, page1, 2)
	assert.True(t, hasMore)

	// Page 2: after last seq of page 1.
	page2, hasMore, err := store.Pull(ctx, hh.HouseholdID, "other", page1[1].Seq, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
	assert.True(t, hasMore)

	// Page 3: last op.
	page3, hasMore, err := store.Pull(ctx, hh.HouseholdID, "other", page2[1].Seq, 2)
	require.NoError(t, err)
	assert.Len(t, page3, 1)
	assert.False(t, hasMore)

	// Verify contiguous: page2[0].Seq == page1[1].Seq + 1.
	assert.Equal(t, page1[1].Seq+1, page2[0].Seq)
	assert.Equal(t, page2[1].Seq+1, page3[0].Seq)

	// No duplicate IDs across pages.
	seen := make(map[string]bool)
	for _, p := range [][]sync.Envelope{page1, page2, page3} {
		for _, op := range p {
			assert.False(t, seen[op.ID], "duplicate op %s", op.ID)
			seen[op.ID] = true
		}
	}
}

func (s *StoreSuite) TestJoinedDevicePullsHistory() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	// Push ops BEFORE device B joins.
	for i := range 3 {
		_, err := store.Push(ctx, []sync.Envelope{{
			ID: fmt.Sprintf("pre-join-%d", i), HouseholdID: hh.HouseholdID,
			DeviceID: hh.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
		}})
		require.NoError(t, err)
	}

	// Join device B.
	dev2ID, _ := suiteJoinDevice(t, store, hh.HouseholdID, hh.DeviceID)

	// Device B pulls from seq 0 — should see all 3 historical ops.
	pulled, _, err := store.Pull(ctx, hh.HouseholdID, dev2ID, 0, 100)
	require.NoError(t, err)
	assert.Len(t, pulled, 3)
}

func (s *StoreSuite) TestPushLargeBatchOrdering() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	ops := make([]sync.Envelope, 100)
	for i := range ops {
		ops[i] = sync.Envelope{
			ID: fmt.Sprintf("batch-%03d", i), HouseholdID: hh.HouseholdID,
			DeviceID: hh.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
		}
	}

	confirmed, err := store.Push(ctx, ops)
	require.NoError(t, err)
	require.Len(t, confirmed, 100)

	// Confirm ordering matches input and seqs are sequential.
	for i, c := range confirmed {
		assert.Equal(t, ops[i].ID, c.ID)
		assert.Equal(t, int64(i+1), c.Seq)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test -shuffle=on -run 'TestStoreMemStore' ./internal/relay/ -count=1`
Expected: all tests PASS

- [ ] **Step 3: Commit**

```
test(relay): add StoreSuite scaffold with push/pull edge cases
```

---

### Task 3: StoreSuite — Invite/Join + Key Exchange Edge Cases

**Files:**
- Modify: `internal/relay/store_suite_test.go`

- [ ] **Step 1: Add Invite/Join and Key Exchange tests**

Append to `store_suite_test.go`:

```go
// --- Invite/Join ---

func (s *StoreSuite) TestStartJoinWrongHouseholdDoesNotBurnAttempts() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	invite, err := store.CreateInvite(ctx, hh1.HouseholdID, hh1.DeviceID)
	require.NoError(t, err)

	// Try joining with wrong household — should fail.
	_, err = store.StartJoin(ctx, hh2.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "wrong-hh", PublicKey: testPublicKey,
	})
	require.Error(t, err)

	// The invite should still be usable with the correct household.
	resp, err := store.StartJoin(ctx, hh1.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "correct-hh", PublicKey: testPublicKey,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ExchangeID)
}

func (s *StoreSuite) TestStartJoinAfterExpiry() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	invite, err := store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID)
	require.NoError(t, err)

	expireInvite(t, store, invite.Code)

	_, err = store.StartJoin(ctx, hh.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "late-joiner", PublicKey: testPublicKey,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func (s *StoreSuite) TestFifthAttemptConsumesInvite() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	invite, err := store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID)
	require.NoError(t, err)

	// Use maxInviteAttempts (5) join attempts.
	for i := range maxInviteAttempts {
		_, err := store.StartJoin(ctx, hh.HouseholdID, invite.Code, sync.JoinRequest{
			DeviceName: fmt.Sprintf("joiner-%d", i), PublicKey: testPublicKey,
		})
		require.NoError(t, err, "attempt %d should succeed", i)
	}

	// The 6th attempt should fail — invite consumed.
	_, err = store.StartJoin(ctx, hh.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "one-too-many", PublicKey: testPublicKey,
	})
	require.Error(t, err)
}

func (s *StoreSuite) TestConcurrentStartJoinSameCode() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	invite, err := store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID)
	require.NoError(t, err)

	const goroutines = 10
	results := make([]error, goroutines)
	var wg gosync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			_, results[idx] = store.StartJoin(ctx, hh.HouseholdID, invite.Code, sync.JoinRequest{
				DeviceName: fmt.Sprintf("racer-%d", idx), PublicKey: testPublicKey,
			})
		}(i)
	}
	wg.Wait()

	var successes int
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	assert.LessOrEqual(t, successes, maxInviteAttempts,
		"at most %d joins should succeed", maxInviteAttempts)
	assert.Positive(t, successes, "at least one join should succeed")
}

func (s *StoreSuite) TestCreateInviteAfterActiveExpires() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	// Create maxActiveInvites (3) invites.
	codes := make([]string, maxActiveInvites)
	for i := range maxActiveInvites {
		inv, err := store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID)
		require.NoError(t, err, "invite %d", i)
		codes[i] = inv.Code
	}

	// 4th should fail.
	_, err := store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID)
	require.Error(t, err)

	// Expire one invite.
	expireInvite(t, store, codes[0])

	// Now a new invite should succeed.
	_, err = store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID)
	require.NoError(t, err)
}

// --- Key Exchange ---

func (s *StoreSuite) TestGetKeyExchangeResultAfterExpiry() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)

	invite, err := store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID)
	require.NoError(t, err)

	joinResp, err := store.StartJoin(ctx, hh.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "joiner", PublicKey: testPublicKey,
	})
	require.NoError(t, err)

	// Complete the exchange.
	err = store.CompleteKeyExchange(ctx, hh.HouseholdID, joinResp.ExchangeID, []byte("key"))
	require.NoError(t, err)

	// Expire the exchange.
	expireKeyExchange(t, store, joinResp.ExchangeID)

	// Polling should now fail.
	_, err = store.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
	require.Error(t, err)
}

func (s *StoreSuite) TestGetKeyExchangeResultTwice() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)
	invite, err := store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID)
	require.NoError(t, err)

	joinResp, err := store.StartJoin(ctx, hh.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "joiner", PublicKey: testPublicKey,
	})
	require.NoError(t, err)

	err = store.CompleteKeyExchange(ctx, hh.HouseholdID, joinResp.ExchangeID, []byte("key"))
	require.NoError(t, err)

	// First retrieval succeeds.
	result, err := store.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
	require.NoError(t, err)
	assert.True(t, result.Ready)
	assert.NotEmpty(t, result.DeviceToken)

	// Second retrieval fails — credentials consumed.
	_, err = store.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "consumed")
}

func (s *StoreSuite) TestConcurrentGetKeyExchangeResult() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)
	invite, err := store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID)
	require.NoError(t, err)

	joinResp, err := store.StartJoin(ctx, hh.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "joiner", PublicKey: testPublicKey,
	})
	require.NoError(t, err)

	err = store.CompleteKeyExchange(ctx, hh.HouseholdID, joinResp.ExchangeID, []byte("key"))
	require.NoError(t, err)

	const goroutines = 5
	results := make([]sync.KeyExchangeResult, goroutines)
	errs := make([]error, goroutines)
	var wg gosync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = store.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
		}(i)
	}
	wg.Wait()

	var gotCredentials int
	for i := range goroutines {
		if errs[i] == nil && results[i].Ready && results[i].DeviceToken != "" {
			gotCredentials++
		}
	}
	assert.Equal(t, 1, gotCredentials, "exactly one caller should get credentials")
}

func (s *StoreSuite) TestCompleteKeyExchangeAlreadyCompleted() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)
	invite, err := store.CreateInvite(ctx, hh.HouseholdID, hh.DeviceID)
	require.NoError(t, err)

	joinResp, err := store.StartJoin(ctx, hh.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "joiner", PublicKey: testPublicKey,
	})
	require.NoError(t, err)

	err = store.CompleteKeyExchange(ctx, hh.HouseholdID, joinResp.ExchangeID, []byte("key"))
	require.NoError(t, err)

	// Second completion should fail.
	err = store.CompleteKeyExchange(ctx, hh.HouseholdID, joinResp.ExchangeID, []byte("key2"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already completed")
}

func (s *StoreSuite) TestCompleteKeyExchangeWrongHousehold() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	invite, err := store.CreateInvite(ctx, hh1.HouseholdID, hh1.DeviceID)
	require.NoError(t, err)

	joinResp, err := store.StartJoin(ctx, hh1.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "joiner", PublicKey: testPublicKey,
	})
	require.NoError(t, err)

	// Complete with wrong household.
	err = store.CompleteKeyExchange(ctx, hh2.HouseholdID, joinResp.ExchangeID, []byte("key"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong")
}

func (s *StoreSuite) TestJoinedDevicePushPullImmediately() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)
	dev2ID, dev2Token := suiteJoinDevice(t, store, hh.HouseholdID, hh.DeviceID)

	// Joined device can authenticate.
	dev, err := store.AuthenticateDevice(ctx, dev2Token)
	require.NoError(t, err)
	assert.Equal(t, dev2ID, dev.ID)

	// Joined device can push.
	confirmed, err := store.Push(ctx, []sync.Envelope{{
		ID: "from-joined", HouseholdID: hh.HouseholdID,
		DeviceID: dev2ID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}})
	require.NoError(t, err)
	require.Len(t, confirmed, 1)

	// Original device can pull the joined device's op.
	pulled, _, err := store.Pull(ctx, hh.HouseholdID, hh.DeviceID, 0, 100)
	require.NoError(t, err)
	assert.Len(t, pulled, 1)
	assert.Equal(t, "from-joined", pulled[0].ID)
}
```

- [ ] **Step 2: Run tests**

Run: `go test -shuffle=on -run 'TestStoreMemStore' ./internal/relay/ -count=1`
Expected: all PASS

- [ ] **Step 3: Commit**

```
test(relay): add invite/join and key exchange edge cases to StoreSuite
```

---

### Task 4: StoreSuite — Blobs, Device Management, Partitioning, Subscription, Auth

**Files:**
- Modify: `internal/relay/store_suite_test.go`

- [ ] **Step 1: Add remaining Store Suite tests**

Append to `store_suite_test.go`:

```go
// --- Blobs ---

func (s *StoreSuite) TestPutBlobExactlyAtQuota() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	const quota int64 = 100

	// Fill to 60 bytes.
	err := store.PutBlob(ctx, hh.HouseholdID,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		make([]byte, 60), quota)
	require.NoError(t, err)

	// 40 more bytes brings us to exactly 100 (quota). Should succeed.
	err = store.PutBlob(ctx, hh.HouseholdID,
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		make([]byte, 40), quota)
	require.NoError(t, err)

	usage, err := store.BlobUsage(ctx, hh.HouseholdID)
	require.NoError(t, err)
	assert.Equal(t, quota, usage)
}

func (s *StoreSuite) TestPutBlobOneByteOverQuota() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	const quota int64 = 100
	err := store.PutBlob(ctx, hh.HouseholdID,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		make([]byte, 60), quota)
	require.NoError(t, err)

	// 41 bytes would bring total to 101 — exceeds quota.
	err = store.PutBlob(ctx, hh.HouseholdID,
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		make([]byte, 41), quota)
	require.ErrorIs(t, err, errQuotaExceeded)
}

func (s *StoreSuite) TestPutBlobQuotaZeroUnlimited() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	// quota=0 means unlimited.
	err := store.PutBlob(ctx, hh.HouseholdID,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		make([]byte, 10000), 0)
	require.NoError(t, err)
}

func (s *StoreSuite) TestPutBlobSameHashDifferentHouseholds() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	hash := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	data := []byte("same-data")

	err := store.PutBlob(ctx, hh1.HouseholdID, hash, data, 0)
	require.NoError(t, err)

	// Same hash in different household — should succeed (no cross-household dedup).
	err = store.PutBlob(ctx, hh2.HouseholdID, hash, data, 0)
	require.NoError(t, err)
}

func (s *StoreSuite) TestGetBlobNotFound() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	_, err := store.GetBlob(ctx, hh.HouseholdID,
		"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
	require.ErrorIs(t, err, errBlobNotFound)

	exists, err := store.HasBlob(ctx, hh.HouseholdID,
		"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
	require.NoError(t, err)
	assert.False(t, exists)
}

func (s *StoreSuite) TestBlobUsageEmptyHousehold() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	usage, err := store.BlobUsage(ctx, hh.HouseholdID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), usage)
}

func (s *StoreSuite) TestPutBlobZeroLength() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	err := store.PutBlob(ctx, hh.HouseholdID,
		"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		[]byte{}, 0)
	require.NoError(t, err)

	usage, err := store.BlobUsage(ctx, hh.HouseholdID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), usage)
}

func (s *StoreSuite) TestGetBlobRoundTrip() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	data := []byte("exact-content-to-verify")
	hash := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	err := store.PutBlob(ctx, hh.HouseholdID, hash, data, 0)
	require.NoError(t, err)

	got, err := store.GetBlob(ctx, hh.HouseholdID, hash)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func (s *StoreSuite) TestBlobUsageAccuracy() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	sizes := []int{10, 20, 35}
	hashes := []string{
		"1111111111111111111111111111111111111111111111111111111111111111",
		"2222222222222222222222222222222222222222222222222222222222222222",
		"3333333333333333333333333333333333333333333333333333333333333333",
	}
	var expected int64
	for i, size := range sizes {
		err := store.PutBlob(ctx, hh.HouseholdID, hashes[i], make([]byte, size), 0)
		require.NoError(t, err)
		expected += int64(size)
	}

	usage, err := store.BlobUsage(ctx, hh.HouseholdID)
	require.NoError(t, err)
	assert.Equal(t, expected, usage)
}

// --- Device Management ---

func (s *StoreSuite) TestRevokeThenAuthenticate() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)
	dev2ID, dev2Token := suiteJoinDevice(t, store, hh.HouseholdID, hh.DeviceID)

	err := store.RevokeDevice(ctx, hh.HouseholdID, dev2ID)
	require.NoError(t, err)

	_, err = store.AuthenticateDevice(ctx, dev2Token)
	require.Error(t, err)
}

func (s *StoreSuite) TestRevokeAllButOne() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)
	dev2ID, _ := suiteJoinDevice(t, store, hh.HouseholdID, hh.DeviceID)
	dev3ID, _ := suiteJoinDevice(t, store, hh.HouseholdID, hh.DeviceID)

	// Revoke devices 2 and 3.
	require.NoError(t, store.RevokeDevice(ctx, hh.HouseholdID, dev2ID))
	require.NoError(t, store.RevokeDevice(ctx, hh.HouseholdID, dev3ID))

	// Original device still works.
	dev, err := store.AuthenticateDevice(ctx, hh.DeviceToken)
	require.NoError(t, err)
	assert.Equal(t, hh.DeviceID, dev.ID)

	// Can still push.
	_, err = store.Push(ctx, []sync.Envelope{{
		ID: "after-revoke", HouseholdID: hh.HouseholdID,
		DeviceID: hh.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}})
	require.NoError(t, err)
}

func (s *StoreSuite) TestListDevicesExcludesRevoked() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh := suiteCreateHousehold(t, store)
	dev2ID, _ := suiteJoinDevice(t, store, hh.HouseholdID, hh.DeviceID)

	devices, err := store.ListDevices(ctx, hh.HouseholdID)
	require.NoError(t, err)
	assert.Len(t, devices, 2)

	require.NoError(t, store.RevokeDevice(ctx, hh.HouseholdID, dev2ID))

	devices, err = store.ListDevices(ctx, hh.HouseholdID)
	require.NoError(t, err)
	assert.Len(t, devices, 1)
	assert.Equal(t, hh.DeviceID, devices[0].ID)
}

// --- Data Partitioning ---

func (s *StoreSuite) TestGetBlobWrongHousehold() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	hash := "4444444444444444444444444444444444444444444444444444444444444444"
	require.NoError(t, store.PutBlob(ctx, hh1.HouseholdID, hash, []byte("secret"), 0))

	_, err := store.GetBlob(ctx, hh2.HouseholdID, hash)
	require.ErrorIs(t, err, errBlobNotFound)
}

func (s *StoreSuite) TestPullWrongHousehold() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	_, err := store.Push(ctx, []sync.Envelope{{
		ID: "op-1", HouseholdID: hh1.HouseholdID,
		DeviceID: hh1.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}})
	require.NoError(t, err)

	pulled, _, err := store.Pull(ctx, hh2.HouseholdID, "any-device", 0, 100)
	require.NoError(t, err)
	assert.Empty(t, pulled)
}

func (s *StoreSuite) TestRevokeDeviceWrongHousehold() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	err := store.RevokeDevice(ctx, hh2.HouseholdID, hh1.DeviceID)
	require.Error(t, err)
}

func (s *StoreSuite) TestCreateInviteNonExistentHousehold() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()

	_, err := store.CreateInvite(ctx, "nonexistent-hh", "nonexistent-dev")
	require.Error(t, err)
}

// --- Subscription State ---

func (s *StoreSuite) TestUpdateSubscriptionRoundTrip() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	err := store.UpdateSubscription(ctx, hh.HouseholdID, "sub_1", "active")
	require.NoError(t, err)

	got, err := store.GetHousehold(ctx, hh.HouseholdID)
	require.NoError(t, err)
	require.NotNil(t, got.StripeSubscriptionID)
	assert.Equal(t, "sub_1", *got.StripeSubscriptionID)
	require.NotNil(t, got.StripeStatus)
	assert.Equal(t, "active", *got.StripeStatus)
}

func (s *StoreSuite) TestSubscriptionStatusTransitions() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	transitions := []string{"active", "canceled", "active"}
	for _, status := range transitions {
		require.NoError(t, store.UpdateSubscription(ctx, hh.HouseholdID, "sub_1", status))
		got, err := store.GetHousehold(ctx, hh.HouseholdID)
		require.NoError(t, err)
		require.NotNil(t, got.StripeStatus)
		assert.Equal(t, status, *got.StripeStatus)
	}
}

func (s *StoreSuite) TestHouseholdBySubscriptionUnknown() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)

	_, err := store.HouseholdBySubscription(t.Context(), "sub_nonexistent")
	require.Error(t, err)
}

func (s *StoreSuite) TestHouseholdByCustomerUnknown() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)

	_, err := store.HouseholdByCustomer(t.Context(), "cus_nonexistent")
	require.Error(t, err)
}

func (s *StoreSuite) TestUpdateSubscriptionNonExistentHousehold() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)

	err := store.UpdateSubscription(t.Context(), "nonexistent", "sub_1", "active")
	require.Error(t, err)
}

func (s *StoreSuite) TestUpdateCustomerIDEmpty() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	err := store.UpdateCustomerID(ctx, hh.HouseholdID, "")
	require.Error(t, err)
}

// --- Auth ---

func (s *StoreSuite) TestAuthenticateRepeatedly() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)
	ctx := t.Context()
	hh := suiteCreateHousehold(t, store)

	for range 5 {
		dev, err := store.AuthenticateDevice(ctx, hh.DeviceToken)
		require.NoError(t, err)
		assert.Equal(t, hh.DeviceID, dev.ID)
	}
}

func (s *StoreSuite) TestAuthenticateEmptyString() {
	t := s.T()
	t.Parallel()
	store := s.newStore(t)

	_, err := store.AuthenticateDevice(t.Context(), "")
	require.Error(t, err)
}

/// --- Backend-specific: Push Duplicate ---

func TestMemStorePushDuplicateAllowed(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	store.SetEncryptionKey(defaultTestEncryptionKey)

	resp, err := store.CreateHousehold(t.Context(), sync.CreateHouseholdRequest{
		DeviceName: "test", PublicKey: testPublicKey,
	})
	require.NoError(t, err)

	op := sync.Envelope{
		ID: "dup-op", HouseholdID: resp.HouseholdID,
		DeviceID: resp.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}
	_, err = store.Push(t.Context(), []sync.Envelope{op})
	require.NoError(t, err)

	// Second push of same ID succeeds on MemStore (no dedup).
	_, err = store.Push(t.Context(), []sync.Envelope{op})
	require.NoError(t, err)
}

func TestPgStorePushDuplicateRejected(t *testing.T) {
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	store := openTestPgStore(t)
	ctx := t.Context()

	resp, err := store.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "test", PublicKey: testPublicKey,
	})
	require.NoError(t, err)

	op := sync.Envelope{
		ID: "dup-op", HouseholdID: resp.HouseholdID,
		DeviceID: resp.DeviceID, Nonce: []byte("n"), Ciphertext: []byte("c"),
	}
	_, err = store.Push(ctx, []sync.Envelope{op})
	require.NoError(t, err)

	// Second push of same ID fails on PgStore (unique index).
	_, err = store.Push(ctx, []sync.Envelope{op})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests**

Run: `go test -shuffle=on -run 'TestStoreMemStore' ./internal/relay/ -count=1`
Expected: all PASS

- [ ] **Step 3: Commit**

```
test(relay): add blob, device, partitioning, subscription, and auth edge cases to StoreSuite
```

---

### Task 5: HandlerSuite Scaffold + Request Validation, Query Params, Auth Edge Cases

**Files:**
- Create: `internal/relay/handler_suite_test.go`

- [ ] **Step 1: Create `handler_suite_test.go` with struct, runners, and first tests**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

//nolint:noctx // test file uses httptest.NewRequest which sets context internally
package relay

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// HandlerSuite tests HTTP edge cases through ServeHTTP.
type HandlerSuite struct {
	suite.Suite
	newHandler func(t *testing.T, opts ...HandlerOption) (*Handler, Store)
}

func TestHandlerMemStore(t *testing.T) {
	t.Parallel()
	suite.Run(t, &HandlerSuite{
		newHandler: func(_ *testing.T, opts ...HandlerOption) (*Handler, Store) {
			s := NewMemStore()
			s.SetEncryptionKey(defaultTestEncryptionKey)
			return NewHandler(s, slog.Default(), opts...), s
		},
	})
}

func TestHandlerPgStore(t *testing.T) {
	// NOT parallel: shares Postgres with TestStorePgStore.
	if os.Getenv("RELAY_POSTGRES_DSN") == "" {
		t.Skip("RELAY_POSTGRES_DSN not set")
	}
	pgStore := openTestPgStore(t)
	suite.Run(t, &HandlerSuite{
		newHandler: func(_ *testing.T, opts ...HandlerOption) (*Handler, Store) {
			return NewHandler(pgStore, slog.Default(), opts...), pgStore
		},
	})
}

// --- Request Validation ---

func (s *HandlerSuite) TestEmptyBodyAllJSONEndpoints() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)

	hh := suiteCreateHousehold(t, store)

	endpoints := []struct {
		method string
		path   string
		auth   bool
	}{
		{"POST", "/households", false},
		{"POST", "/sync/push", true},
		{"POST", "/households/" + hh.HouseholdID + "/join", false},
		{"POST", "/key-exchange/fake-id/complete", true},
	}

	for _, ep := range endpoints {
		var req *http.Request
		if ep.auth {
			req = httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
		} else {
			req = httptest.NewRequest(ep.method, ep.path, nil)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code,
			"%s %s with nil body should return 400", ep.method, ep.path)
	}
}

func (s *HandlerSuite) TestOversizedJSONBody() {
	t := s.T()
	t.Parallel()
	h, _ := s.newHandler(t)

	// 1 MiB + 1 byte of garbage. io.LimitReader truncates, JSON decode fails -> 400.
	bigBody := bytes.NewReader(make([]byte, maxRequestBody+1))
	req := httptest.NewRequest("POST", "/households", bigBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func (s *HandlerSuite) TestOversizedBlobBody() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)
	suiteActivateSubscription(t, store, hh.HouseholdID)

	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	bigBody := bytes.NewReader(make([]byte, maxBlobSize+1))
	req := httptest.NewRequest("PUT", "/blobs/"+hh.HouseholdID+"/"+hash, bigBody)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

// --- Query Parameter Boundaries ---

func (s *HandlerSuite) TestPullAfterNonNumeric() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?after=abc", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func (s *HandlerSuite) TestPullLimitNonNumeric() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?limit=abc", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func (s *HandlerSuite) TestPullLimitZero() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?limit=0", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func (s *HandlerSuite) TestPullLimit1001() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?limit=1001", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- Auth Edge Cases (HTTP) ---

func (s *HandlerSuite) TestBearerNoTrailingSpace() {
	t := s.T()
	t.Parallel()
	h, _ := s.newHandler(t)

	req := httptest.NewRequest("GET", "/status", nil)
	req.Header.Set("Authorization", "Bearer")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func (s *HandlerSuite) TestBearerTrailingSpaceNoToken() {
	t := s.T()
	t.Parallel()
	h, _ := s.newHandler(t)

	req := httptest.NewRequest("GET", "/status", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func (s *HandlerSuite) TestBearerLowercase() {
	t := s.T()
	t.Parallel()
	h, _ := s.newHandler(t)

	req := httptest.NewRequest("GET", "/status", nil)
	req.Header.Set("Authorization", "bearer some-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func (s *HandlerSuite) TestBearerDoubleSpace() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)

	// "Bearer  <token>" — double space. extractBearerToken returns " <token>".
	req := httptest.NewRequest("GET", "/status", nil)
	req.Header.Set("Authorization", "Bearer  "+hh.DeviceToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func (s *HandlerSuite) TestNoAuthHeader() {
	t := s.T()
	t.Parallel()
	h, _ := s.newHandler(t)

	req := httptest.NewRequest("GET", "/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func (s *HandlerSuite) TestLongBearerToken() {
	t := s.T()
	t.Parallel()
	h, _ := s.newHandler(t)

	longToken := strings.Repeat("a", 10240)
	req := httptest.NewRequest("GET", "/status", nil)
	req.Header.Set("Authorization", "Bearer "+longToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

- [ ] **Step 2: Run tests**

Run: `go test -shuffle=on -run 'TestHandlerMemStore' ./internal/relay/ -count=1`
Expected: all PASS

- [ ] **Step 3: Commit**

```
test(relay): add HandlerSuite scaffold with request validation, query params, and auth edge cases
```

---

### Task 6: HandlerSuite — Blob HTTP, Subscription Gating, Cross-Household, Stripe Webhook

**Files:**
- Modify: `internal/relay/handler_suite_test.go`

- [ ] **Step 1: Add remaining Handler Suite tests**

Append to `handler_suite_test.go`:

```go
// --- Blob HTTP Semantics ---

func (s *HandlerSuite) TestPutBlobUppercaseHash() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)
	suiteActivateSubscription(t, store, hh.HouseholdID)

	hash := "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"
	req := httptest.NewRequest("PUT", "/blobs/"+hh.HouseholdID+"/"+hash,
		bytes.NewReader([]byte("data")))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func (s *HandlerSuite) TestPutBlobShortHash() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)
	suiteActivateSubscription(t, store, hh.HouseholdID)

	hash := strings.Repeat("a", 63) // 63 chars, needs 64
	req := httptest.NewRequest("PUT", "/blobs/"+hh.HouseholdID+"/"+hash,
		bytes.NewReader([]byte("data")))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func (s *HandlerSuite) TestHeadBlobExists() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)
	suiteActivateSubscription(t, store, hh.HouseholdID)

	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	require.NoError(t, store.PutBlob(t.Context(), hh.HouseholdID, hash, []byte("data"), 0))

	req := httptest.NewRequest("HEAD", "/blobs/"+hh.HouseholdID+"/"+hash, nil)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func (s *HandlerSuite) TestHeadBlobNotFound() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)
	suiteActivateSubscription(t, store, hh.HouseholdID)

	hash := "0000000000000000000000000000000000000000000000000000000000000000"
	req := httptest.NewRequest("HEAD", "/blobs/"+hh.HouseholdID+"/"+hash, nil)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func (s *HandlerSuite) TestGetBlobContentType() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)
	suiteActivateSubscription(t, store, hh.HouseholdID)

	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	require.NoError(t, store.PutBlob(t.Context(), hh.HouseholdID, hash, []byte("data"), 0))

	req := httptest.NewRequest("GET", "/blobs/"+hh.HouseholdID+"/"+hash, nil)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
}

// --- Subscription Gating ---

func (s *HandlerSuite) TestSubscriptionGatingCanceled() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)
	require.NoError(t, store.UpdateSubscription(t.Context(), hh.HouseholdID, "sub_1", sync.SubscriptionCanceled))

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/sync/push"},
		{"GET", "/sync/pull"},
		{"PUT", "/blobs/" + hh.HouseholdID + "/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		{"GET", "/blobs/" + hh.HouseholdID + "/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		{"HEAD", "/blobs/" + hh.HouseholdID + "/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
	}

	for _, ep := range endpoints {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authRequest(ep.method, ep.path, nil, hh.DeviceToken))
		assert.Equal(t, http.StatusPaymentRequired, rec.Code,
			"%s %s should return 402 when canceled", ep.method, ep.path)
	}
}

func (s *HandlerSuite) TestSubscriptionGatingReactivation() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)

	// Cancel subscription.
	require.NoError(t, store.UpdateSubscription(t.Context(), hh.HouseholdID, "sub_1", sync.SubscriptionCanceled))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull", nil, hh.DeviceToken))
	require.Equal(t, http.StatusPaymentRequired, rec.Code)

	// Reactivate.
	require.NoError(t, store.UpdateSubscription(t.Context(), hh.HouseholdID, "sub_1", "active"))

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func (s *HandlerSuite) TestSelfHostedBypassesGating() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t, WithSelfHosted())
	hh := suiteCreateHousehold(t, store)
	require.NoError(t, store.UpdateSubscription(t.Context(), hh.HouseholdID, "sub_1", sync.SubscriptionCanceled))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func (s *HandlerSuite) TestNullSubscriptionAllowed() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)
	// No subscription set — StripeStatus is nil.

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- Cross-Household Access ---

func (s *HandlerSuite) TestCrossHouseholdBlobAccess() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)
	suiteActivateSubscription(t, store, hh1.HouseholdID)
	suiteActivateSubscription(t, store, hh2.HouseholdID)

	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	// Device from hh1 tries to access hh2's blob URL.
	for _, method := range []string{"PUT", "GET", "HEAD"} {
		var body *bytes.Reader
		if method == "PUT" {
			body = bytes.NewReader([]byte("data"))
		} else {
			body = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, "/blobs/"+hh2.HouseholdID+"/"+hash, body)
		req.Header.Set("Authorization", "Bearer "+hh1.DeviceToken)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code,
			"%s blob with other household's ID should be 403", method)
	}
}

func (s *HandlerSuite) TestCrossHouseholdInvite() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("POST", "/households/"+hh2.HouseholdID+"/invite",
		nil, hh1.DeviceToken))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func (s *HandlerSuite) TestCrossHouseholdDevices() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh1 := suiteCreateHousehold(t, store)
	hh2 := suiteCreateHousehold(t, store)

	// List devices for other household.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/households/"+hh2.HouseholdID+"/devices",
		nil, hh1.DeviceToken))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Revoke device in other household.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("DELETE",
		"/households/"+hh2.HouseholdID+"/devices/"+hh2.DeviceID,
		nil, hh1.DeviceToken))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- Stripe Webhook ---

// signWebhook creates a valid Stripe webhook signature for testing.
func signWebhook(payload []byte, secret string) string {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(payload)))
	sig := hex.EncodeToString(mac.Sum(nil))
	return "t=" + ts + ",v1=" + sig
}

func (s *HandlerSuite) TestWebhookReplay() {
	t := s.T()
	t.Parallel()
	secret := "whsec_test"
	h, store := s.newHandler(t, WithWebhookSecret(secret))
	hh := suiteCreateHousehold(t, store)
	require.NoError(t, store.UpdateSubscription(t.Context(), hh.HouseholdID, "sub_replay", "active"))

	event := fmt.Sprintf(`{"id":"evt_1","type":"customer.subscription.updated","data":{"object":{"id":"sub_replay","status":"active"}}}`)
	payload := []byte(event)
	sig := signWebhook(payload, secret)

	for range 2 {
		req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(payload))
		req.Header.Set("Stripe-Signature", sig)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
}

func (s *HandlerSuite) TestWebhookNonSubscriptionEvent() {
	t := s.T()
	t.Parallel()
	secret := "whsec_test"
	h, _ := s.newHandler(t, WithWebhookSecret(secret))

	payload := []byte(`{"id":"evt_1","type":"charge.succeeded","data":{}}`)
	sig := signWebhook(payload, secret)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", sig)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ignored")
}

func (s *HandlerSuite) TestWebhookMissingSignatureHeader() {
	t := s.T()
	t.Parallel()
	h, _ := s.newHandler(t, WithWebhookSecret("whsec_test"))

	req := httptest.NewRequest("POST", "/webhooks/stripe",
		bytes.NewReader([]byte(`{"id":"evt_1"}`)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func (s *HandlerSuite) TestWebhookExactly1MiB() {
	t := s.T()
	t.Parallel()
	secret := "whsec_test"
	h, _ := s.newHandler(t, WithWebhookSecret(secret))

	// Body at exactly maxRequestBody bytes. Not rejected for size.
	payload := make([]byte, maxRequestBody)
	copy(payload, []byte(`{"id":"evt_1","type":"charge.succeeded","data":{}}`))
	sig := signWebhook(payload, secret)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", sig)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	// Should NOT be 413. Could be 200 (ignored) or 400 (bad JSON) — either is fine.
	assert.NotEqual(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func (s *HandlerSuite) TestWebhook1MiBPlus1() {
	t := s.T()
	t.Parallel()
	h, _ := s.newHandler(t, WithWebhookSecret("whsec_test"))

	// Body at maxRequestBody + 1 — rejected by MaxBytesReader.
	payload := make([]byte, maxRequestBody+1)
	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", "t=123,v1=fake")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
```

- [ ] **Step 2: Run tests**

Run: `go test -shuffle=on -run 'TestHandlerMemStore' ./internal/relay/ -count=1`
Expected: all PASS

- [ ] **Step 3: Commit**

```
test(relay): add blob HTTP, subscription gating, cross-household, and webhook edge cases to HandlerSuite
```

---

### Task 7: Final Verification

**Files:** None (verification only)

- [ ] **Step 1: Run all relay tests together**

Run: `go test -shuffle=on ./internal/relay/ -count=1`
Expected: all PASS (existing tests + new suites)

- [ ] **Step 2: Run with race detector**

Run: `go test -shuffle=on -race ./internal/relay/ -count=1`
Expected: no race conditions detected

- [ ] **Step 3: Check for lint warnings**

Run: `golangci-lint run ./internal/relay/`
Expected: no new warnings

- [ ] **Step 4: Commit (if any lint fixes needed)**

```
test(relay): fix lint warnings in edge case test suite
```
