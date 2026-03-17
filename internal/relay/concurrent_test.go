// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	gosync "sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentCreateInviteRespectsCap verifies that concurrent invite
// creation for the same household never exceeds maxActiveInvites (3).
// Ten goroutines race to create invites; exactly 3 must receive 201 and
// the rest must receive 409 (conflict / quota full).
func TestConcurrentCreateInviteRespectsCap(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	const goroutines = 10

	codes := make([]int, goroutines)
	var wg gosync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			req := authRequest(
				"POST",
				"/households/"+hh.HouseholdID+"/invite",
				nil,
				hh.DeviceToken,
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			codes[idx] = rec.Code
		}(i)
	}

	wg.Wait()

	var created, conflict int
	for _, code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusBadRequest:
			// The handler returns 400 when CreateInvite errors (max reached).
			conflict++
		default:
			t.Errorf("unexpected status code %d", code)
		}
	}

	assert.Equal(
		t,
		maxActiveInvites,
		created,
		"expected exactly maxActiveInvites=3 successful creations",
	)
	assert.Equal(
		t,
		goroutines-maxActiveInvites,
		conflict,
		"remaining goroutines should have been rejected",
	)
}

// TestConcurrentPutBlobRespectsQuota verifies that concurrent blob uploads
// to the same household never exceed the configured quota. Five goroutines
// each try to upload a distinct 400-byte blob against a 1000-byte quota;
// at most two uploads should succeed, and the stored bytes must not exceed
// the quota.
func TestConcurrentPutBlobRespectsQuota(t *testing.T) {
	t.Parallel()

	const (
		quota      int64 = 1000
		blobSize   int64 = 400 // bytes; two blobs fit (800 < 1000), third does not
		goroutines int   = 5
	)

	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithBlobQuota(quota))
	hh := createTestHousehold(t, h)

	// Activate subscription so the blob endpoint accepts requests.
	require.NoError(t, store.UpdateSubscription(
		context.Background(), hh.HouseholdID, "sub_concurrent_test", "active",
	))

	// Build goroutines distinct blobs and their real SHA-256 hashes.
	type blobEntry struct {
		hash string
		data []byte
	}
	blobs := make([]blobEntry, goroutines)
	for i := range goroutines {
		data := bytes.Repeat(fmt.Appendf(nil, "%02d", i), int(blobSize/2))
		h256 := sha256.Sum256(data)
		blobs[i] = blobEntry{
			hash: hex.EncodeToString(h256[:]),
			data: data,
		}
	}

	codes := make([]int, goroutines)
	var wg gosync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			url := "/blobs/" + hh.HouseholdID + "/" + blobs[idx].hash
			req := httptest.NewRequest("PUT", url, bytes.NewReader(blobs[idx].data))
			req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			codes[idx] = rec.Code
		}(i)
	}

	wg.Wait()

	// Tally results.
	var created, quotaExceeded int
	for _, code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusRequestEntityTooLarge:
			quotaExceeded++
		case http.StatusConflict:
			// Duplicate hash -- counts as "not stored".
			quotaExceeded++
		default:
			t.Errorf("unexpected status code %d", code)
		}
	}

	require.Greater(t, created, 0, "at least one blob should have been stored")
	assert.Equal(
		t,
		goroutines,
		created+quotaExceeded,
		"every request must produce a definitive outcome",
	)

	// Verify stored bytes never exceeded quota.
	used, err := store.BlobUsage(context.Background(), hh.HouseholdID)
	require.NoError(t, err)
	assert.LessOrEqual(t, used, quota, "stored bytes must not exceed quota")
}
