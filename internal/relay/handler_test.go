// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

//nolint:noctx // test file uses httptest.NewRequest which sets context internally
package relay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	body, err := json.Marshal(sync.CreateHouseholdRequest{
		DeviceName: "test-desktop",
		PublicKey:  []byte("fake-public-key-32-bytes-paddin!"),
	})
	require.NoError(t, err)
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
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			panic("marshal test body: " + err.Error())
		}
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

	body, err := json.Marshal(sync.CreateHouseholdRequest{})
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/households", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- Auth middleware ---

func TestAuthenticateDeviceMultipleDevices(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	// Create several households, each with a device token.
	tokens := make([]string, 5)
	for i := range tokens {
		hh := createTestHousehold(t, h)
		tokens[i] = hh.DeviceToken
	}

	// Each token should authenticate to the correct device.
	for _, token := range tokens {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authRequest("GET", "/status", nil, token))
		assert.Equal(t, http.StatusOK, rec.Code, "token should authenticate")
	}

	// Invalid token should fail.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/status", nil, "bogus-token"))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

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
	regBody, err := json.Marshal(sync.RegisterDeviceRequest{
		HouseholdID: hhResp.HouseholdID,
		Name:        "test-laptop",
		PublicKey:   []byte("device-b-pubkey-32-bytes-padding"),
	})
	require.NoError(t, err)
	// Register via store directly (no HTTP endpoint exposed in MVP).
	regResp, err := store.RegisterDevice(context.Background(), sync.RegisterDeviceRequest{
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
	rec := httptest.NewRecorder()
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
	regResp, err := store.RegisterDevice(context.Background(), sync.RegisterDeviceRequest{
		HouseholdID: hhResp.HouseholdID,
		Name:        "device-b",
	})
	require.NoError(t, err)
	tokenB := regResp.DeviceToken

	// Push 5 ops from device A.
	for range 5 {
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

// --- Invite and Join ---

func TestCreateInvite(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusCreated, rec.Code)

	var invite sync.InviteCode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))
	assert.NotEmpty(t, invite.Code)
	assert.True(t, invite.ExpiresAt.After(time.Now()))
}

func TestCreateInviteRequiresHouseholdMembership(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Try to create invite for a different household.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/wrong-id/invite", nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCreateInviteMaxActiveLimit(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Create 3 invites (max).
	for range 3 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(
			rec,
			authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
		)
		require.Equal(t, http.StatusCreated, rec.Code)
	}

	// Fourth should fail.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFullKeyExchangeFlow(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Step 1: Inviter creates invite.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	var invite sync.InviteCode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))

	// Step 2: Joiner initiates join (no auth needed).
	joinerPubKey := []byte("joiner-public-key-32-byte-pad!!!")
	joinReq := sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "joiner-phone",
		PublicKey:  joinerPubKey,
	}
	joinBody, err := json.Marshal(joinReq)
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var joinResp sync.JoinResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&joinResp))
	assert.NotEmpty(t, joinResp.ExchangeID)
	// Exchange IDs must be 256-bit crypto-random hex, not ULIDs.
	assert.Len(t, joinResp.ExchangeID, 64, "exchange ID should be 64 hex chars (256-bit)")
	assert.Regexp(t, `^[0-9a-f]{64}$`, joinResp.ExchangeID)
	assert.NotEmpty(t, joinResp.InviterPublicKey)

	// Step 3: Inviter checks pending exchanges.
	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"GET",
			"/households/"+hh.HouseholdID+"/pending-exchanges",
			nil,
			hh.DeviceToken,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var pendingResp struct {
		Exchanges []sync.PendingKeyExchange `json:"exchanges"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&pendingResp))
	require.Len(t, pendingResp.Exchanges, 1)
	assert.Equal(t, joinResp.ExchangeID, pendingResp.Exchanges[0].ID)
	assert.Equal(t, "joiner-phone", pendingResp.Exchanges[0].JoinerName)

	// Step 4: Inviter completes key exchange.
	encryptedKey := []byte("encrypted-household-key-data")
	completeReq := sync.CompleteKeyExchangeRequest{
		EncryptedHouseholdKey: encryptedKey,
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/key-exchange/"+joinResp.ExchangeID+"/complete",
			completeReq,
			hh.DeviceToken,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	// Step 5: Joiner polls for result (no auth).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/key-exchange/"+joinResp.ExchangeID, nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var result sync.KeyExchangeResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&result))
	assert.True(t, result.Ready)
	assert.Equal(t, encryptedKey, result.EncryptedHouseholdKey)
	assert.NotEmpty(t, result.DeviceID)
	assert.NotEmpty(t, result.DeviceToken)
}

func TestJoinInvalidInviteCode(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	joinReq := sync.JoinRequest{
		InviteCode: "INVALID1",
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	}
	joinBody, err := json.Marshal(joinReq)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestJoinWrongHouseholdDoesNotConsumeAttempt(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Create invite.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	var invite sync.InviteCode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))

	// Try to join with a different household ID -- should fail.
	joinReq := sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	}
	joinBody, err := json.Marshal(joinReq)
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(
		"POST",
		"/households/WRONG-HOUSEHOLD-ID/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Join with the correct household ID should still succeed,
	// proving no attempt was consumed by the wrong-ID request.
	joinBody, err = json.Marshal(joinReq)
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestJoinMissingFields(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Create invite.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	var invite sync.InviteCode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))

	// Missing device_name.
	joinBody, err := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestKeyExchangeResultBeforeCompletion(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Create invite and join.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	var invite sync.InviteCode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))

	joinBody, err := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var joinResp sync.JoinResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&joinResp))

	// Poll before inviter completes -- not ready.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/key-exchange/"+joinResp.ExchangeID, nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var result sync.KeyExchangeResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&result))
	assert.False(t, result.Ready)
	assert.Empty(t, result.DeviceToken)
}

func TestKeyExchangeResultClearsCredentialsAfterRetrieval(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Full exchange: invite → join → complete.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	var invite sync.InviteCode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))

	joinBody, err := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var joinResp sync.JoinResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&joinResp))

	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/key-exchange/"+joinResp.ExchangeID+"/complete",
			sync.CompleteKeyExchangeRequest{EncryptedHouseholdKey: []byte("key-data")},
			hh.DeviceToken,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	// First retrieval -- should return credentials.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/key-exchange/"+joinResp.ExchangeID, nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var result sync.KeyExchangeResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&result))
	require.True(t, result.Ready)
	assert.NotEmpty(t, result.DeviceToken)
	assert.NotEmpty(t, result.EncryptedHouseholdKey)

	// Second retrieval -- credentials must be gone.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/key-exchange/"+joinResp.ExchangeID, nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var result2 sync.KeyExchangeResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&result2))
	assert.True(t, result2.Ready)
	assert.Empty(t, result2.DeviceToken, "token must not be returned twice")
	assert.Empty(t, result2.EncryptedHouseholdKey, "encrypted key must not be returned twice")
}

func TestInviteConsumedAfterKeyExchange(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Create invite.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	var invite sync.InviteCode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))

	// Join and complete exchange.
	joinBody, err := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var joinResp sync.JoinResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&joinResp))

	// Complete exchange.
	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/key-exchange/"+joinResp.ExchangeID+"/complete",
			sync.CompleteKeyExchangeRequest{EncryptedHouseholdKey: []byte("key-data")},
			hh.DeviceToken,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	// Try to use invite code again -- should fail.
	joinBody, err = json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "tablet",
		PublicKey:  []byte("another-key-32-bytes-padding!!!!"),
	})
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- Device management ---

func TestListDevices(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("GET", "/households/"+hh.HouseholdID+"/devices", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Devices []sync.Device `json:"devices"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Devices, 1)
	assert.Equal(t, "test-desktop", resp.Devices[0].Name)
}

func TestRevokeDevice(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	// Register a second device.
	regResp, err := store.RegisterDevice(context.Background(), sync.RegisterDeviceRequest{
		HouseholdID: hh.HouseholdID,
		Name:        "device-b",
	})
	require.NoError(t, err)

	// Revoke device B.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"DELETE",
			"/households/"+hh.HouseholdID+"/devices/"+regResp.DeviceID,
			nil,
			hh.DeviceToken,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	// Device B can no longer authenticate.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?after=0", nil, regResp.DeviceToken))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Only 1 device remains.
	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("GET", "/households/"+hh.HouseholdID+"/devices", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Devices []sync.Device `json:"devices"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Len(t, resp.Devices, 1)
}

func TestCannotRevokeSelf(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"DELETE",
			"/households/"+hh.HouseholdID+"/devices/"+hh.DeviceID,
			nil,
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListDevicesWrongHousehold(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("GET", "/households/wrong-id/devices", nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestJoinedDeviceCanPushAndPull(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Full key exchange flow.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	var invite sync.InviteCode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))

	joinBody, err := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var joinResp sync.JoinResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&joinResp))

	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/key-exchange/"+joinResp.ExchangeID+"/complete",
			sync.CompleteKeyExchangeRequest{EncryptedHouseholdKey: []byte("key-data")},
			hh.DeviceToken,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	// Get joiner's token.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/key-exchange/"+joinResp.ExchangeID, nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var result sync.KeyExchangeResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&result))
	require.True(t, result.Ready)

	// Device A pushes an op.
	op := sync.Envelope{
		ID:         uid.New(),
		Nonce:      []byte("nonce-for-op-24bytes!!!!"),
		Ciphertext: []byte("from-device-A"),
		CreatedAt:  time.Now(),
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/sync/push",
			sync.PushRequest{Ops: []sync.Envelope{op}},
			hh.DeviceToken,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	// Joined device pulls -- should see device A's op.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?after=0", nil, result.DeviceToken))
	require.Equal(t, http.StatusOK, rec.Code)

	var pullResp sync.PullResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&pullResp))
	require.Len(t, pullResp.Ops, 1)
	assert.Equal(t, op.ID, pullResp.Ops[0].ID)
}

// --- Subscription gating ---

func TestPushReturns402WhenSubscriptionCanceled(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	// Set subscription to canceled.
	require.NoError(t, store.UpdateSubscription(
		context.Background(), hh.HouseholdID, "sub_123", sync.SubscriptionCanceled,
	))

	op := sync.Envelope{
		ID:         uid.New(),
		Nonce:      []byte("nonce-24-bytes-padding!!"),
		Ciphertext: []byte("data"),
		CreatedAt:  time.Now(),
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/sync/push",
			sync.PushRequest{Ops: []sync.Envelope{op}},
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusPaymentRequired, rec.Code)
	assert.Contains(t, rec.Body.String(), "subscription inactive")
}

func TestPullReturns402WhenSubscriptionCanceled(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	require.NoError(t, store.UpdateSubscription(
		context.Background(), hh.HouseholdID, "sub_456", sync.SubscriptionCanceled,
	))

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("GET", "/sync/pull?after=0", nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusPaymentRequired, rec.Code)
}

func TestPushSucceedsWhenSubscriptionActive(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	require.NoError(t, store.UpdateSubscription(
		context.Background(), hh.HouseholdID, "sub_789", sync.SubscriptionActive,
	))

	op := sync.Envelope{
		ID:         uid.New(),
		Nonce:      []byte("nonce-24-bytes-padding!!"),
		Ciphertext: []byte("data"),
		CreatedAt:  time.Now(),
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/sync/push",
			sync.PushRequest{Ops: []sync.Envelope{op}},
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPushSucceedsWhenNoSubscription(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// No subscription set (empty status) -- should pass (dev/free mode).
	op := sync.Envelope{
		ID:         uid.New(),
		Nonce:      []byte("nonce-24-bytes-padding!!"),
		Ciphertext: []byte("data"),
		CreatedAt:  time.Now(),
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/sync/push",
			sync.PushRequest{Ops: []sync.Envelope{op}},
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPushReturns402WhenSubscriptionPastDue(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	require.NoError(t, store.UpdateSubscription(
		context.Background(), hh.HouseholdID, "sub_pd", sync.SubscriptionPastDue,
	))

	op := sync.Envelope{
		ID:         uid.New(),
		Nonce:      []byte("nonce-24-bytes-padding!!"),
		Ciphertext: []byte("data"),
		CreatedAt:  time.Now(),
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/sync/push",
			sync.PushRequest{Ops: []sync.Envelope{op}},
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusPaymentRequired, rec.Code)
}

// --- Stripe webhook ---

func newTestHandlerWithWebhook(secret string) (*Handler, *MemStore) {
	store := NewMemStore()
	log := slog.Default()
	return NewHandler(store, log, WithWebhookSecret(secret)), store
}

func TestStripeWebhookUpdatesSubscription(t *testing.T) {
	t.Parallel()
	secret := "whsec_test_secret" //nolint:gosec // test credential
	h, store := newTestHandlerWithWebhook(secret)

	// Create household and set an initial subscription.
	hh := createTestHousehold(t, h)
	require.NoError(t, store.UpdateSubscription(
		context.Background(), hh.HouseholdID, "sub_webhook_1", sync.SubscriptionActive,
	))

	// Send a subscription.deleted webhook event.
	event := StripeEvent{
		ID:   "evt_test_1",
		Type: "customer.subscription.deleted",
		Data: json.RawMessage(`{"object":{"id":"sub_webhook_1","status":"canceled"}}`),
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)
	sigHeader := makeSignatureHeader(body, secret, time.Now())

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sigHeader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify household subscription status updated.
	household, err := store.GetHousehold(context.Background(), hh.HouseholdID)
	require.NoError(t, err)
	assert.Equal(t, sync.SubscriptionCanceled, household.StripeStatus)
}

func TestStripeWebhookRejectsInvalidSignature(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandlerWithWebhook("whsec_real_secret")

	event := StripeEvent{
		ID:   "evt_bad",
		Type: "customer.subscription.created",
		Data: json.RawMessage(`{"object":{"id":"sub_1","status":"active"}}`),
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)
	// Sign with wrong secret.
	sigHeader := makeSignatureHeader(body, "wrong_secret", time.Now())

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sigHeader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid signature")
}

func TestStripeWebhookIgnoresNonSubscriptionEvents(t *testing.T) {
	t.Parallel()
	secret := "whsec_ignore_test" //nolint:gosec // test credential
	h, _ := newTestHandlerWithWebhook(secret)

	event := StripeEvent{
		ID:   "evt_charge",
		Type: "charge.succeeded",
		Data: json.RawMessage(`{"object":{"id":"ch_123"}}`),
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)
	sigHeader := makeSignatureHeader(body, secret, time.Now())

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sigHeader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ignored")
}

func TestStripeWebhookNoSecretReturns503(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler() // No webhook secret.

	event := StripeEvent{
		ID:   "evt_nosec",
		Type: "customer.subscription.updated",
		Data: json.RawMessage(`{"object":{"id":"sub_nosec","status":"past_due"}}`),
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)

	// Without a webhook secret, the handler must refuse to process
	// webhooks to prevent accepting arbitrary payloads in production.
	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestStripeWebhookRejectsOversizedBody(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	// Create a body larger than 1 MB.
	oversized := make([]byte, maxRequestBody+1)
	for i := range oversized {
		oversized[i] = 'A'
	}

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(oversized))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// With MaxBytesReader, reading beyond the limit produces an error.
	// The handler should return 400, not silently truncate.
	assert.NotEqual(t, http.StatusOK, rec.Code,
		"oversized webhook body should be rejected, not silently truncated")
}

// --- Status endpoint ---

func TestStatusEndpoint(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	// Set subscription status.
	require.NoError(t, store.UpdateSubscription(
		context.Background(), hh.HouseholdID, "sub_status", sync.SubscriptionActive,
	))

	// Push an op to increment ops count.
	op := sync.Envelope{
		ID:         uid.New(),
		Nonce:      []byte("nonce-24-bytes-padding!!"),
		Ciphertext: []byte("data"),
		CreatedAt:  time.Now(),
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/sync/push",
			sync.PushRequest{Ops: []sync.Envelope{op}},
			hh.DeviceToken,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	// Get status.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/status", nil, hh.DeviceToken))
	require.Equal(t, http.StatusOK, rec.Code)

	var status sync.StatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	assert.Equal(t, hh.HouseholdID, status.HouseholdID)
	assert.Len(t, status.Devices, 1)
	assert.Equal(t, int64(1), status.OpsCount)
	assert.Equal(t, sync.SubscriptionActive, status.StripeStatus)
}

func TestStatusRequiresAuth(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	req := httptest.NewRequest("GET", "/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- Blob endpoints ---

const testHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func blobURL(householdID, hash string) string {
	return "/blobs/" + householdID + "/" + hash
}

func TestBlobUploadAndDownload(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	payload := []byte("encrypted-blob-content")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", blobURL(hh.HouseholdID, testHash), bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Download the blob.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", blobURL(hh.HouseholdID, testHash), nil)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, payload, rec.Body.Bytes())
}

func TestBlobDedup409(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	payload := []byte("encrypted-blob-content")

	// First upload succeeds.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", blobURL(hh.HouseholdID, testHash), bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Second upload returns 409 (dedup).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", blobURL(hh.HouseholdID, testHash), bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestBlobDownloadNotFound(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", blobURL(hh.HouseholdID, testHash), nil)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestBlobHEADExists(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// HEAD before upload -- 404.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("HEAD", blobURL(hh.HouseholdID, testHash), nil)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Upload.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		"PUT",
		blobURL(hh.HouseholdID, testHash),
		bytes.NewReader([]byte("data")),
	)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// HEAD after upload -- 200.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("HEAD", blobURL(hh.HouseholdID, testHash), nil)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestBlobRequiresAuth(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// No auth token.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		"PUT",
		blobURL(hh.HouseholdID, testHash),
		bytes.NewReader([]byte("data")),
	)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBlobWrongHousehold(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Try to upload to a different household.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		"PUT",
		blobURL("wrong-household", testHash),
		bytes.NewReader([]byte("data")),
	)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestBlobInvalidHash(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		"PUT",
		blobURL(hh.HouseholdID, "not-a-hash"),
		bytes.NewReader([]byte("data")),
	)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestBlobQuotaExceeded(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithBlobQuota(100))
	hh := createTestHousehold(t, h)

	// Upload a blob larger than quota.
	bigPayload := make([]byte, 200)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		"PUT",
		blobURL(hh.HouseholdID, testHash),
		bytes.NewReader(bigPayload),
	)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Contains(t, rec.Body.String(), "quota")
}

func TestBlobSubscriptionGating(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	// Cancel subscription.
	require.NoError(t, store.UpdateSubscription(
		context.Background(), hh.HouseholdID, "sub_blob", sync.SubscriptionCanceled,
	))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		"PUT",
		blobURL(hh.HouseholdID, testHash),
		bytes.NewReader([]byte("data")),
	)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusPaymentRequired, rec.Code)
}

func TestStatusIncludesBlobStorage(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Upload a blob.
	payload := []byte("encrypted-blob-content-for-status")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", blobURL(hh.HouseholdID, testHash), bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Get status -- should include blob_storage.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/status", nil, hh.DeviceToken))
	require.Equal(t, http.StatusOK, rec.Code)

	var status sync.StatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	assert.Equal(t, int64(len(payload)), status.BlobStorage.UsedBytes)
	assert.Positive(t, status.BlobStorage.QuotaBytes)
}

func TestKeyExchangeExpiredReturnsError(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	// Create invite and join to get a key exchange record.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	var invite sync.InviteCode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))

	joinBody, err := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var joinResp sync.JoinResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&joinResp))

	// Backdate the exchange record to beyond the expiry window.
	store.mu.Lock()
	ex := store.exchanges[joinResp.ExchangeID]
	require.NotNil(t, ex)
	ex.createdAt = time.Now().Add(-2 * keyExchangeExpiry)
	store.mu.Unlock()

	// GetKeyExchangeResult should now return an error (expired).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/key-exchange/"+joinResp.ExchangeID, nil)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// GetPendingExchanges should exclude the expired exchange.
	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"GET",
			"/households/"+hh.HouseholdID+"/pending-exchanges",
			nil,
			hh.DeviceToken,
		),
	)
	require.Equal(t, http.StatusOK, rec.Code)
	var pendingResp struct {
		Exchanges []sync.PendingKeyExchange `json:"exchanges"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&pendingResp))
	assert.Empty(t, pendingResp.Exchanges, "expired exchange should not appear in pending list")
}

// --- Bearer token edge cases ---

func TestAuthEdgeCases(t *testing.T) {
	t.Parallel()

	// authEndpoint is a representative authenticated endpoint used across subtests.
	const authEndpoint = "/status"

	t.Run("empty bearer token", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler()

		req := httptest.NewRequest("GET", authEndpoint, nil)
		req.Header.Set("Authorization", "Bearer ")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("whitespace-only bearer token", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler()

		req := httptest.NewRequest("GET", authEndpoint, nil)
		req.Header.Set("Authorization", "Bearer    ")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("extremely long bearer token", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler()

		const tokenLen = 10 * 1024
		const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		b := make([]byte, tokenLen)
		for i := range b {
			b[i] = charset[rand.IntN(len(charset))] //nolint:gosec // test helper
		}
		longToken := string(b)

		req := httptest.NewRequest("GET", authEndpoint, nil)
		req.Header.Set("Authorization", "Bearer "+longToken)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("missing Authorization header", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler()

		req := httptest.NewRequest("GET", authEndpoint, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// --- Malformed JSON in request bodies ---

func TestMalformedJSONBodies(t *testing.T) {
	t.Parallel()

	const badJSON = `{invalid json`

	t.Run("POST /households", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler()

		req := httptest.NewRequest("POST", "/households", strings.NewReader(badJSON))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("POST /households/{id}/join", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler()
		hh := createTestHousehold(t, h)

		req := httptest.NewRequest(
			"POST",
			"/households/"+hh.HouseholdID+"/join",
			strings.NewReader(badJSON),
		)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("POST /sync/push", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler()
		hh := createTestHousehold(t, h)

		req := httptest.NewRequest("POST", "/sync/push", strings.NewReader(badJSON))
		req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("POST /key-exchange/{id}/complete", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler()
		hh := createTestHousehold(t, h)

		// Set up a pending key exchange so the route resolves to an existing exchange.
		rec := httptest.NewRecorder()
		h.ServeHTTP(
			rec,
			authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
		)
		require.Equal(t, http.StatusCreated, rec.Code)
		var invite sync.InviteCode
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))

		joinBody, err := json.Marshal(sync.JoinRequest{
			InviteCode: invite.Code,
			DeviceName: "joiner",
			PublicKey:  []byte("joiner-key-32-bytes-padding!!!!!"),
		})
		require.NoError(t, err)
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(
			"POST",
			"/households/"+hh.HouseholdID+"/join",
			bytes.NewReader(joinBody),
		)
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var joinResp sync.JoinResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&joinResp))

		// Now send malformed JSON to the complete endpoint.
		req = httptest.NewRequest(
			"POST",
			"/key-exchange/"+joinResp.ExchangeID+"/complete",
			strings.NewReader(badJSON),
		)
		req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestSelfHostedSubscriptionBypass(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithSelfHosted())
	hh := createTestHousehold(t, h)

	// Set subscription to canceled — cloud mode would return 402.
	require.NoError(t, store.UpdateSubscription(
		context.Background(),
		hh.HouseholdID,
		"sub_test",
		"canceled",
	))

	// In self-hosted mode, push should succeed despite canceled subscription.
	// Empty ops returns 400, not 402 — proves subscription check was bypassed.
	rec := httptest.NewRecorder()
	req := authRequest("POST", "/sync/push", sync.PushRequest{}, hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSelfHostedUnlimitedQuota(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithSelfHosted())
	hh := createTestHousehold(t, h)

	// Status should report quota=0 (unlimited) in self-hosted mode.
	rec := httptest.NewRecorder()
	req := authRequest("GET", "/status", nil, hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var status sync.StatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	assert.Equal(t, int64(0), status.BlobStorage.QuotaBytes)
}

func TestCloudModeDefaultQuota(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default())
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	req := authRequest("GET", "/status", nil, hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var status sync.StatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	assert.Equal(t, DefaultBlobQuota, status.BlobStorage.QuotaBytes)
}

func TestCustomBlobQuota(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithBlobQuota(500))
	hh := createTestHousehold(t, h)

	// Upload a 200-byte blob — should succeed (200 < 500).
	payload := make([]byte, 200)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", blobURL(hh.HouseholdID, testHash), bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	// Upload another 400-byte blob — would exceed 500 quota.
	payload2 := make([]byte, 400)
	hash2 := fmt.Sprintf("%x", sha256.Sum256(payload2))
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("PUT", blobURL(hh.HouseholdID, hash2), bytes.NewReader(payload2))
	req2.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec2.Code)

	// Verify the 413 response reports the custom quota, not DefaultBlobQuota.
	var body map[string]any
	require.NoError(t, json.NewDecoder(rec2.Body).Decode(&body))
	assert.InDelta(t, float64(500), body["quota_bytes"], 0)
}

func TestSelfHostedIntegration(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithSelfHosted())
	hh := createTestHousehold(t, h)

	// Cancel subscription — self-hosted should still work.
	require.NoError(t, store.UpdateSubscription(
		context.Background(),
		hh.HouseholdID,
		"sub_test",
		"canceled",
	))

	// Upload blob despite canceled subscription.
	payload := []byte("self-hosted-integration-test-blob")
	hash := fmt.Sprintf("%x", sha256.Sum256(payload))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", blobURL(hh.HouseholdID, hash), bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Download it back.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", blobURL(hh.HouseholdID, hash), nil)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, payload, rec.Body.Bytes())

	// Status reports quota=0.
	rec = httptest.NewRecorder()
	req = authRequest("GET", "/status", nil, hh.DeviceToken)
	h.ServeHTTP(rec, req)
	var status sync.StatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	assert.Equal(t, int64(0), status.BlobStorage.QuotaBytes)
	assert.Equal(t, int64(len(payload)), status.BlobStorage.UsedBytes)
}

func TestSelfHostedCustomQuota(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithSelfHosted(), WithBlobQuota(100))
	hh := createTestHousehold(t, h)

	// Upload a blob within quota.
	small := []byte("small")
	hash := fmt.Sprintf("%x", sha256.Sum256(small))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", blobURL(hh.HouseholdID, hash), bytes.NewReader(small))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Upload a blob that exceeds the custom quota.
	big := make([]byte, 100)
	bigHash := fmt.Sprintf("%x", sha256.Sum256(big))
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", blobURL(hh.HouseholdID, bigHash), bytes.NewReader(big))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestWithBlobQuotaNegativePanics(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { WithBlobQuota(-1) })
}

// --- failingStore: error-injecting wrapper for coverage of store error paths ---

// failingStore wraps a real Store and lets tests inject errors for specific methods.
type failingStore struct {
	Store
	listDevicesErr      error
	getHouseholdErr     error
	opsCountErr         error
	blobUsageErr        error
	getPendingErr       error
	hasBlobErr          error
	getBlobErr          error
	createHouseholdErr  error
	completeExchangeErr error
	pullErr             error
	pushErr             error
	revokeDeviceErr     error
	putBlobErr          error
}

func (f *failingStore) ListDevices(ctx context.Context, hhID string) ([]sync.Device, error) {
	if f.listDevicesErr != nil {
		return nil, f.listDevicesErr
	}
	return f.Store.ListDevices(ctx, hhID)
}

func (f *failingStore) GetHousehold(ctx context.Context, hhID string) (sync.Household, error) {
	if f.getHouseholdErr != nil {
		return sync.Household{}, f.getHouseholdErr
	}
	return f.Store.GetHousehold(ctx, hhID)
}

func (f *failingStore) OpsCount(ctx context.Context, hhID string) (int64, error) {
	if f.opsCountErr != nil {
		return 0, f.opsCountErr
	}
	return f.Store.OpsCount(ctx, hhID)
}

func (f *failingStore) BlobUsage(ctx context.Context, hhID string) (int64, error) {
	if f.blobUsageErr != nil {
		return 0, f.blobUsageErr
	}
	return f.Store.BlobUsage(ctx, hhID)
}

func (f *failingStore) GetPendingExchanges(
	ctx context.Context,
	hhID string,
) ([]sync.PendingKeyExchange, error) {
	if f.getPendingErr != nil {
		return nil, f.getPendingErr
	}
	return f.Store.GetPendingExchanges(ctx, hhID)
}

func (f *failingStore) HasBlob(ctx context.Context, hhID, hash string) (bool, error) {
	if f.hasBlobErr != nil {
		return false, f.hasBlobErr
	}
	return f.Store.HasBlob(ctx, hhID, hash)
}

func (f *failingStore) GetBlob(ctx context.Context, hhID, hash string) ([]byte, error) {
	if f.getBlobErr != nil {
		return nil, f.getBlobErr
	}
	return f.Store.GetBlob(ctx, hhID, hash)
}

func (f *failingStore) CreateHousehold(
	ctx context.Context,
	req sync.CreateHouseholdRequest,
) (sync.CreateHouseholdResponse, error) {
	if f.createHouseholdErr != nil {
		return sync.CreateHouseholdResponse{}, f.createHouseholdErr
	}
	return f.Store.CreateHousehold(ctx, req)
}

func (f *failingStore) CompleteKeyExchange(
	ctx context.Context,
	hhID, exchangeID string,
	encryptedKey []byte,
) error {
	if f.completeExchangeErr != nil {
		return f.completeExchangeErr
	}
	return f.Store.CompleteKeyExchange(ctx, hhID, exchangeID, encryptedKey)
}

func (f *failingStore) Pull(
	ctx context.Context,
	householdID, excludeDeviceID string,
	afterSeq int64,
	limit int,
) ([]sync.Envelope, bool, error) {
	if f.pullErr != nil {
		return nil, false, f.pullErr
	}
	return f.Store.Pull(ctx, householdID, excludeDeviceID, afterSeq, limit)
}

func (f *failingStore) Push(
	ctx context.Context,
	ops []sync.Envelope,
) ([]sync.PushConfirmation, error) {
	if f.pushErr != nil {
		return nil, f.pushErr
	}
	return f.Store.Push(ctx, ops)
}

func (f *failingStore) RevokeDevice(ctx context.Context, hhID, deviceID string) error {
	if f.revokeDeviceErr != nil {
		return f.revokeDeviceErr
	}
	return f.Store.RevokeDevice(ctx, hhID, deviceID)
}

func (f *failingStore) PutBlob(
	ctx context.Context,
	hhID, hash string,
	data []byte,
	quota int64,
) error {
	if f.putBlobErr != nil {
		return f.putBlobErr
	}
	return f.Store.PutBlob(ctx, hhID, hash, data, quota)
}

// newFailingHandler creates a handler backed by a failingStore wrapping a MemStore.
func newFailingHandler(fs *failingStore) *Handler {
	return NewHandler(fs, slog.Default())
}

// createTestHouseholdDirect creates a household via the underlying MemStore,
// then authenticates the device token through the failingStore. Returns the
// response so tests can use the token for authenticated requests.
func createTestHouseholdDirect(
	t *testing.T,
	ms *MemStore,
	h *Handler,
) sync.CreateHouseholdResponse {
	t.Helper()
	resp, err := ms.CreateHousehold(context.Background(), sync.CreateHouseholdRequest{
		DeviceName: "test-desktop",
		PublicKey:  []byte("fake-public-key-32-bytes-paddin!"),
	})
	require.NoError(t, err)
	return resp
}

// --- handleStatus error paths ---

func TestStatusListDevicesError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.listDevicesErr = fmt.Errorf("database connection lost")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/status", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal error")
}

func TestStatusGetHouseholdError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.getHouseholdErr = fmt.Errorf("household lookup failed")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/status", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal error")
}

func TestStatusOpsCountError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.opsCountErr = fmt.Errorf("ops count query failed")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/status", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal error")
}

func TestStatusBlobUsageError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.blobUsageErr = fmt.Errorf("blob usage query failed")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/status", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal error")
}

// --- handleGetPendingExchanges error paths ---

func TestGetPendingExchangesForbidden(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("GET", "/households/wrong-id/pending-exchanges", nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "does not belong")
}

func TestGetPendingExchangesStoreError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.getPendingErr = fmt.Errorf("database unavailable")

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"GET",
			"/households/"+hh.HouseholdID+"/pending-exchanges",
			nil,
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal error")
}

// --- handleHeadBlob error paths ---

func TestHeadBlobForbidden(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("HEAD", blobURL("wrong-household", testHash), nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHeadBlobInvalidHash(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("HEAD", blobURL(hh.HouseholdID, "not-valid-hash"), nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHeadBlobNotFound(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("HEAD", blobURL(hh.HouseholdID, testHash), nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHeadBlobStoreError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.hasBlobErr = fmt.Errorf("storage backend unreachable")

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("HEAD", blobURL(hh.HouseholdID, testHash), nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// --- handleCreateHousehold error paths ---

func TestCreateHouseholdMissingPublicKey(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	body, err := json.Marshal(sync.CreateHouseholdRequest{
		DeviceName: "test-desktop",
		// PublicKey omitted (nil/empty).
	})
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/households", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "public_key must be exactly 32 bytes")
}

func TestCreateHouseholdPublicKeyWrongSize(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	body, err := json.Marshal(sync.CreateHouseholdRequest{
		DeviceName: "test-desktop",
		PublicKey:  []byte("too-short-key"),
	})
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/households", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "public_key must be exactly 32 bytes")
}

func TestCreateHouseholdDeviceNameTooLong(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()

	longName := strings.Repeat("a", maxDeviceNameLen+1)
	body, err := json.Marshal(sync.CreateHouseholdRequest{
		DeviceName: longName,
		PublicKey:  []byte("fake-public-key-32-bytes-paddin!"),
	})
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/households", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "device_name exceeds maximum length")
}

func TestCreateHouseholdStoreError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms, createHouseholdErr: fmt.Errorf("db write failed")}
	h := newFailingHandler(fs)

	body, err := json.Marshal(sync.CreateHouseholdRequest{
		DeviceName: "test-desktop",
		PublicKey:  []byte("fake-public-key-32-bytes-paddin!"),
	})
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/households", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal error")
}

// --- handleGetBlob error paths ---

func TestGetBlobForbidden(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("GET", blobURL("wrong-household", testHash), nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "does not belong")
}

func TestGetBlobInvalidHash(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("GET", blobURL(hh.HouseholdID, "INVALID"), nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid hash")
}

func TestGetBlobStoreError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.getBlobErr = fmt.Errorf("storage read failure")

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("GET", blobURL(hh.HouseholdID, testHash), nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "get blob failed")
}

// --- handleCompleteKeyExchange error paths ---

func TestCompleteKeyExchangeEmptyEncryptedKey(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	// Set up an exchange.
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("POST", "/households/"+hh.HouseholdID+"/invite", nil, hh.DeviceToken),
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	var invite sync.InviteCode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&invite))

	joinBody, err := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(
		"POST",
		"/households/"+hh.HouseholdID+"/join",
		bytes.NewReader(joinBody),
	)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var joinResp sync.JoinResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&joinResp))

	// Send empty encrypted key.
	rec = httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/key-exchange/"+joinResp.ExchangeID+"/complete",
			sync.CompleteKeyExchangeRequest{EncryptedHouseholdKey: []byte{}},
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "encrypted_household_key is required")
}

func TestCompleteKeyExchangeStoreError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	// Set up an exchange via the underlying MemStore.
	invite, err := ms.CreateInvite(context.Background(), hh.HouseholdID, hh.DeviceID)
	require.NoError(t, err)

	joinResp, err := ms.StartJoin(
		context.Background(),
		hh.HouseholdID,
		invite.Code,
		sync.JoinRequest{
			InviteCode: invite.Code,
			DeviceName: "phone",
			PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
		},
	)
	require.NoError(t, err)

	// Inject the error.
	fs.completeExchangeErr = fmt.Errorf("db constraint violation")

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/key-exchange/"+joinResp.ExchangeID+"/complete",
			sync.CompleteKeyExchangeRequest{EncryptedHouseholdKey: []byte("some-key-data")},
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "key exchange failed")
}

// --- handleRevokeDevice error paths ---

func TestRevokeDeviceWrongHousehold(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	// Register a second device to revoke.
	regResp, err := store.RegisterDevice(context.Background(), sync.RegisterDeviceRequest{
		HouseholdID: hh.HouseholdID,
		Name:        "device-b",
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"DELETE",
			"/households/wrong-id/devices/"+regResp.DeviceID,
			nil,
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "does not belong")
}

func TestRevokeDeviceStoreError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	// Register a second device.
	regResp, err := ms.RegisterDevice(context.Background(), sync.RegisterDeviceRequest{
		HouseholdID: hh.HouseholdID,
		Name:        "device-b",
	})
	require.NoError(t, err)

	fs.revokeDeviceErr = fmt.Errorf("device revocation constraint failure")

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"DELETE",
			"/households/"+hh.HouseholdID+"/devices/"+regResp.DeviceID,
			nil,
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "device revocation failed")
}

// --- handleListDevices error paths ---

func TestListDevicesStoreError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.listDevicesErr = fmt.Errorf("database timeout")

	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest("GET", "/households/"+hh.HouseholdID+"/devices", nil, hh.DeviceToken),
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal error")
}

// --- handlePull error paths ---

func TestPullInvalidAfterParam(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?after=not-a-number", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid after parameter")
}

func TestPullInvalidLimitParam(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	t.Run("non-numeric", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authRequest("GET", "/sync/pull?limit=abc", nil, hh.DeviceToken))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "limit must be 1-1000")
	})

	t.Run("zero", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authRequest("GET", "/sync/pull?limit=0", nil, hh.DeviceToken))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "limit must be 1-1000")
	})

	t.Run("exceeds max", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authRequest("GET", "/sync/pull?limit=1001", nil, hh.DeviceToken))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "limit must be 1-1000")
	})

	t.Run("negative", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authRequest("GET", "/sync/pull?limit=-1", nil, hh.DeviceToken))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "limit must be 1-1000")
	})
}

func TestPullStoreError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.pullErr = fmt.Errorf("query execution failed")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull?after=0", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "pull failed")
}

// --- handlePutBlob error paths ---

func TestPutBlobInvalidHashFormats(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	hh := createTestHousehold(t, h)

	cases := []struct {
		name string
		hash string
	}{
		{"uppercase hex", "E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855"},
		{"too short", "abcdef"},
		{"too long", testHash + "aa"},
		{"non-hex chars", "g3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(
				"PUT",
				blobURL(hh.HouseholdID, tc.hash),
				bytes.NewReader([]byte("data")),
			)
			req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
			h.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "invalid hash")
		})
	}
}

func TestPutBlobStoreError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.putBlobErr = fmt.Errorf("disk full")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		"PUT",
		blobURL(hh.HouseholdID, testHash),
		bytes.NewReader([]byte("data")),
	)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "store blob failed")
}

// --- handlePush error paths ---

func TestPushStoreError(t *testing.T) {
	t.Parallel()
	ms := NewMemStore()
	fs := &failingStore{Store: ms}
	h := newFailingHandler(fs)
	hh := createTestHouseholdDirect(t, ms, h)

	fs.pushErr = fmt.Errorf("write conflict")

	op := sync.Envelope{
		ID:         uid.New(),
		Nonce:      []byte("nonce-24-bytes-padding!!"),
		Ciphertext: []byte("data"),
		CreatedAt:  time.Now(),
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(
		rec,
		authRequest(
			"POST",
			"/sync/push",
			sync.PushRequest{Ops: []sync.Envelope{op}},
			hh.DeviceToken,
		),
	)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "push failed")
}
