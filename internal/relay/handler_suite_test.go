// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

//nolint:noctx // test file uses httptest.NewRequest which sets context internally
package relay

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

func TestHandlerSuiteMemStore(t *testing.T) {
	t.Parallel()
	suite.Run(t, &HandlerSuite{
		newHandler: func(_ *testing.T, opts ...HandlerOption) (*Handler, Store) {
			s := NewMemStore()
			s.SetEncryptionKey(defaultTestEncryptionKey)
			return NewHandler(s, slog.Default(), opts...), s
		},
	})
}

func TestHandlerSuitePgStore(t *testing.T) {
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
	require.NoError(
		t,
		store.UpdateSubscription(t.Context(), hh.HouseholdID, "sub_1", sync.SubscriptionCanceled),
	)

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/sync/push"},
		{"GET", "/sync/pull"},
		{
			"PUT",
			"/blobs/" + hh.HouseholdID + "/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},
		{
			"GET",
			"/blobs/" + hh.HouseholdID + "/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},
		{
			"HEAD",
			"/blobs/" + hh.HouseholdID + "/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},
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

	require.NoError(
		t,
		store.UpdateSubscription(t.Context(), hh.HouseholdID, "sub_1", sync.SubscriptionCanceled),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull", nil, hh.DeviceToken))
	require.Equal(t, http.StatusPaymentRequired, rec.Code)

	require.NoError(
		t,
		store.UpdateSubscription(t.Context(), hh.HouseholdID, "sub_1", sync.SubscriptionActive),
	)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func (s *HandlerSuite) TestSelfHostedBypassesGating() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t, WithSelfHosted())
	hh := suiteCreateHousehold(t, store)
	require.NoError(
		t,
		store.UpdateSubscription(t.Context(), hh.HouseholdID, "sub_1", sync.SubscriptionCanceled),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/sync/pull", nil, hh.DeviceToken))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func (s *HandlerSuite) TestNullSubscriptionAllowed() {
	t := s.T()
	t.Parallel()
	h, store := s.newHandler(t)
	hh := suiteCreateHousehold(t, store)

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

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authRequest("GET", "/households/"+hh2.HouseholdID+"/devices",
		nil, hh1.DeviceToken))
	assert.Equal(t, http.StatusForbidden, rec.Code)

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
	require.NoError(
		t,
		store.UpdateSubscription(
			t.Context(),
			hh.HouseholdID,
			"sub_replay",
			sync.SubscriptionActive,
		),
	)

	event := fmt.Sprintf(
		`{"id":"evt_1","type":"customer.subscription.updated","data":{"object":{"id":"sub_replay","status":"active"}}}`,
	)
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

	payload := make([]byte, maxRequestBody)
	copy(payload, []byte(`{"id":"evt_1","type":"charge.succeeded","data":{}}`))
	sig := signWebhook(payload, secret)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", sig)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func (s *HandlerSuite) TestWebhook1MiBPlus1() {
	t := s.T()
	t.Parallel()
	h, _ := s.newHandler(t, WithWebhookSecret("whsec_test"))

	payload := make([]byte, maxRequestBody+1)
	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", "t=123,v1=fake")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
