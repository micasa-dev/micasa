// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

//nolint:noctx // test file uses httptest.NewRequest which sets context internally
package relay

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
