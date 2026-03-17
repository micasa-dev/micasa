// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/sync"
	"github.com/cpcloud/micasa/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHandler() (*Handler, *MemStore) {
	store := NewMemStore()
	log := slog.Default()
	return NewHandler(store, log), store
}

func createTestHousehold(t *testing.T, h *Handler) sync.CreateHouseholdResponse {
	t.Helper()
	body, _ := json.Marshal(sync.CreateHouseholdRequest{
		DeviceName: "test-desktop",
		PublicKey:  []byte("fake-public-key-32-bytes-padding!"),
	})
	req := httptest.NewRequest("POST", "/households", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp sync.CreateHouseholdResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	return resp
}

func authRequest(method, path string, body any, token string) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// --- Health ---

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"ok"`)
}

// --- Create Household ---

func TestCreateHousehold(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	resp := createTestHousehold(t, h)
	assert.NotEmpty(t, resp.HouseholdID)
	assert.NotEmpty(t, resp.DeviceID)
	assert.NotEmpty(t, resp.DeviceToken)
}

func TestCreateHouseholdMissingName(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	body, _ := json.Marshal(sync.CreateHouseholdRequest{})
	req := httptest.NewRequest("POST", "/households", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- Auth middleware ---

func TestPushRequiresAuth(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	req := httptest.NewRequest("POST", "/sync/push", bytes.NewReader([]byte("{}")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPullRequiresAuth(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	req := httptest.NewRequest("GET", "/sync/pull", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestInvalidTokenRejected(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	req := authRequest(
		"POST",
		"/sync/push",
		sync.PushRequest{Ops: []sync.Envelope{{}}},
		"bogus-token",
	)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- Push ---

func TestPushAndPull(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	// Create household with device A.
	hhResp := createTestHousehold(t, h)
	tokenA := hhResp.DeviceToken

	// Push an op from device A.
	op := sync.Envelope{
		ID:         uid.New(),
		Nonce:      []byte("test-nonce-24-bytes!!!!"),
		Ciphertext: []byte("encrypted-payload"),
		CreatedAt:  time.Now(),
	}
	pushReq := sync.PushRequest{Ops: []sync.Envelope{op}}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("POST", "/sync/push", pushReq, tokenA))
	require.Equal(t, http.StatusOK, rec.Code)

	var pushResp sync.PushResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&pushResp))
	require.Len(t, pushResp.Confirmed, 1)
	assert.Equal(t, op.ID, pushResp.Confirmed[0].ID)
	assert.Equal(t, int64(1), pushResp.Confirmed[0].Seq)

	// Device A pulls -- should see no ops (own ops excluded).
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?after=0", nil, tokenA))
	require.Equal(t, http.StatusOK, rec.Code)

	var pullResp sync.PullResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&pullResp))
	assert.Empty(t, pullResp.Ops, "device should not see its own ops")
}

func TestPushEmptyOps(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	hhResp := createTestHousehold(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("POST", "/sync/push", sync.PushRequest{}, hhResp.DeviceToken))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- Two-device sync ---

func TestTwoDeviceSync(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()

	// Device A creates household.
	hhResp := createTestHousehold(t, h)
	tokenA := hhResp.DeviceToken

	// Device B registers in same household.
	regBody, _ := json.Marshal(sync.RegisterDeviceRequest{
		HouseholdID: hhResp.HouseholdID,
		Name:        "test-laptop",
		PublicKey:   []byte("device-b-pubkey-32-bytes-padding"),
	})
	rec := httptest.NewRecorder()
	// Register via store directly (no HTTP endpoint exposed in MVP).
	regResp, err := store.RegisterDevice(nil, sync.RegisterDeviceRequest{
		HouseholdID: hhResp.HouseholdID,
		Name:        "test-laptop",
		PublicKey:   regBody,
	})
	require.NoError(t, err)
	tokenB := regResp.DeviceToken

	// Device A pushes an op.
	op := sync.Envelope{
		ID:         uid.New(),
		Nonce:      []byte("nonce-for-op-24bytes!!!!"),
		Ciphertext: []byte("encrypted-data-from-A"),
		CreatedAt:  time.Now(),
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/sync/push", sync.PushRequest{Ops: []sync.Envelope{op}}, tokenA),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	// Device B pulls -- should see device A's op.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?after=0", nil, tokenB))
	require.Equal(t, http.StatusOK, rec.Code)

	var pullResp sync.PullResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&pullResp))
	require.Len(t, pullResp.Ops, 1)
	assert.Equal(t, op.ID, pullResp.Ops[0].ID)
	assert.Equal(t, int64(1), pullResp.Ops[0].Seq)
}

// --- Pull pagination ---

func TestPullPagination(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()

	hhResp := createTestHousehold(t, h)
	tokenA := hhResp.DeviceToken

	// Register device B.
	regResp, err := store.RegisterDevice(nil, sync.RegisterDeviceRequest{
		HouseholdID: hhResp.HouseholdID,
		Name:        "device-b",
	})
	require.NoError(t, err)
	tokenB := regResp.DeviceToken

	// Push 5 ops from device A.
	for i := 0; i < 5; i++ {
		op := sync.Envelope{
			ID:         uid.New(),
			Nonce:      []byte("nonce-24-bytes-padding!!"),
			Ciphertext: []byte("data"),
			CreatedAt:  time.Now(),
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(
			rec,
			authRequest("POST", "/sync/push", sync.PushRequest{Ops: []sync.Envelope{op}}, tokenA),
		)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// Device B pulls with limit=2.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?after=0&limit=2", nil, tokenB))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp1 sync.PullResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp1))
	assert.Len(t, resp1.Ops, 2)
	assert.True(t, resp1.HasMore)

	// Pull next page.
	lastSeq := resp1.Ops[len(resp1.Ops)-1].Seq
	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"GET",
			"/sync/pull?after="+strconv.FormatInt(lastSeq, 10)+"&limit=2",
			nil,
			tokenB,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp2 sync.PullResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp2))
	assert.Len(t, resp2.Ops, 2)
	assert.True(t, resp2.HasMore)

	// Pull last page.
	lastSeq = resp2.Ops[len(resp2.Ops)-1].Seq
	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"GET",
			"/sync/pull?after="+strconv.FormatInt(lastSeq, 10)+"&limit=2",
			nil,
			tokenB,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp3 sync.PullResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp3))
	assert.Len(t, resp3.Ops, 1)
	assert.False(t, resp3.HasMore)
}

// --- Sequence ordering ---

func TestPushAssignsMonotonicSequences(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	hhResp := createTestHousehold(t, h)

	ops := make([]sync.Envelope, 3)
	for i := range ops {
		ops[i] = sync.Envelope{
			ID:         uid.New(),
			Nonce:      []byte("nonce-24-bytes-padding!!"),
			Ciphertext: []byte("data"),
			CreatedAt:  time.Now(),
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/sync/push", sync.PushRequest{Ops: ops}, hhResp.DeviceToken),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp sync.PushResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Confirmed, 3)

	assert.Equal(t, int64(1), resp.Confirmed[0].Seq)
	assert.Equal(t, int64(2), resp.Confirmed[1].Seq)
	assert.Equal(t, int64(3), resp.Confirmed[2].Seq)
}
