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
	for i := 0; i < 3; i++ {
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
	joinBody, _ := json.Marshal(joinReq)
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
		PublicKey:  []byte("key"),
	}
	joinBody, _ := json.Marshal(joinReq)
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
		PublicKey:  []byte("key"),
	}
	joinBody, _ := json.Marshal(joinReq)
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
	joinBody, _ = json.Marshal(joinReq)
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
	joinBody, _ := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		PublicKey:  []byte("key"),
	})
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

	joinBody, _ := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
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

	joinBody, _ := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
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
	joinBody, _ := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
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
	joinBody, _ = json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "tablet",
		PublicKey:  []byte("another-key-32-bytes-padding!!!!"),
	})
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
	regResp, err := store.RegisterDevice(nil, sync.RegisterDeviceRequest{
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

	joinBody, _ := json.Marshal(sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "phone",
		PublicKey:  []byte("key-32-bytes-of-padding-here!!!!"),
	})
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
		nil, hh.HouseholdID, "sub_123", sync.SubscriptionCanceled,
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
		nil, hh.HouseholdID, "sub_456", sync.SubscriptionCanceled,
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
		nil, hh.HouseholdID, "sub_789", sync.SubscriptionActive,
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
		nil, hh.HouseholdID, "sub_pd", sync.SubscriptionPastDue,
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
	secret := "whsec_test_secret"
	h, store := newTestHandlerWithWebhook(secret)

	// Create household and set an initial subscription.
	hh := createTestHousehold(t, h)
	require.NoError(t, store.UpdateSubscription(
		nil, hh.HouseholdID, "sub_webhook_1", sync.SubscriptionActive,
	))

	// Send a subscription.deleted webhook event.
	event := StripeEvent{
		ID:   "evt_test_1",
		Type: "customer.subscription.deleted",
		Data: json.RawMessage(`{"object":{"id":"sub_webhook_1","status":"canceled"}}`),
	}
	body, _ := json.Marshal(event)
	sigHeader := makeSignatureHeader(body, secret, time.Now())

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sigHeader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify household subscription status updated.
	household, err := store.GetHousehold(nil, hh.HouseholdID)
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
	body, _ := json.Marshal(event)
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
	secret := "whsec_ignore_test"
	h, _ := newTestHandlerWithWebhook(secret)

	event := StripeEvent{
		ID:   "evt_charge",
		Type: "charge.succeeded",
		Data: json.RawMessage(`{"object":{"id":"ch_123"}}`),
	}
	body, _ := json.Marshal(event)
	sigHeader := makeSignatureHeader(body, secret, time.Now())

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sigHeader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ignored")
}

func TestStripeWebhookNoSecretSkipsVerification(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler() // No webhook secret.

	hh := createTestHousehold(t, h)
	require.NoError(t, store.UpdateSubscription(
		nil, hh.HouseholdID, "sub_nosec", sync.SubscriptionActive,
	))

	event := StripeEvent{
		ID:   "evt_nosec",
		Type: "customer.subscription.updated",
		Data: json.RawMessage(`{"object":{"id":"sub_nosec","status":"past_due"}}`),
	}
	body, _ := json.Marshal(event)

	// No Stripe-Signature header -- should still work when secret is empty.
	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	household, err := store.GetHousehold(nil, hh.HouseholdID)
	require.NoError(t, err)
	assert.Equal(t, sync.SubscriptionPastDue, household.StripeStatus)
}

// --- Status endpoint ---

func TestStatusEndpoint(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	// Set subscription status.
	require.NoError(t, store.UpdateSubscription(
		nil, hh.HouseholdID, "sub_status", sync.SubscriptionActive,
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
	h, store := newTestHandler()
	hh := createTestHousehold(t, h)

	// Override quota to something tiny (100 bytes).
	store.SetBlobQuota(100)

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
		nil, hh.HouseholdID, "sub_blob", sync.SubscriptionCanceled,
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
	assert.True(t, status.BlobStorage.QuotaBytes > 0)
}
