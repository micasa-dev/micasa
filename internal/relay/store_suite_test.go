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
