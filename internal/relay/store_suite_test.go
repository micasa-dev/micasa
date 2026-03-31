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
		{
			ID:          "a",
			HouseholdID: hh.HouseholdID,
			DeviceID:    hh.DeviceID,
			Nonce:       []byte("n"),
			Ciphertext:  []byte("c"),
		},
		{
			ID:          "b",
			HouseholdID: hh.HouseholdID,
			DeviceID:    hh.DeviceID,
			Nonce:       []byte("n"),
			Ciphertext:  []byte("c"),
		},
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
		{
			ID:          "op-1",
			HouseholdID: hh.HouseholdID,
			DeviceID:    hh.DeviceID,
			Nonce:       []byte("n"),
			Ciphertext:  []byte("c"),
		},
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
		{
			ID:          "op-1",
			HouseholdID: hh.HouseholdID,
			DeviceID:    hh.DeviceID,
			Nonce:       []byte("n"),
			Ciphertext:  []byte("c"),
		},
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
	var all []sync.PushConfirmation
	all = append(all, confirmed1...)
	all = append(all, confirmed2...)
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
	require.Len(t, page1, 2)
	assert.True(t, hasMore)

	// Page 2: after last seq of page 1.
	page2, hasMore, err := store.Pull(ctx, hh.HouseholdID, "other", page1[1].Seq, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.True(t, hasMore)

	// Page 3: last op.
	page3, hasMore, err := store.Pull(ctx, hh.HouseholdID, "other", page2[1].Seq, 2)
	require.NoError(t, err)
	require.Len(t, page3, 1)
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

	// The 6th attempt should fail — max attempts exceeded.
	_, err = store.StartJoin(ctx, hh.HouseholdID, invite.Code, sync.JoinRequest{
		DeviceName: "one-too-many", PublicKey: testPublicKey,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max attempts")
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
	require.Len(t, pulled, 1)
	assert.Equal(t, "from-joined", pulled[0].ID)
}

// --- Blobs ---

func (s *StoreSuite) TestPutBlobExactlyAtQuota() {
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

	require.NoError(t, store.RevokeDevice(ctx, hh.HouseholdID, dev2ID))
	require.NoError(t, store.RevokeDevice(ctx, hh.HouseholdID, dev3ID))

	dev, err := store.AuthenticateDevice(ctx, hh.DeviceToken)
	require.NoError(t, err)
	assert.Equal(t, hh.DeviceID, dev.ID)

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
	require.Len(t, devices, 1)
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

// --- Backend-specific: Push Duplicate ---

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

	// Second push of same ID fails on PgStore (unique constraint).
	_, err = store.Push(ctx, []sync.Envelope{op})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}
