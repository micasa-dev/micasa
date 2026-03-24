// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package address

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupValidPostalCode(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/us/90210", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"post_code": "90210",
			"country": "United States",
			"country_abbreviation": "US",
			"places": [{"place name": "Beverly Hills", "state": "California", "state abbreviation": "CA"}]
		}`))
	}))
	defer srv.Close()

	result, err := Lookup(context.Background(), srv.Client(), srv.URL, "us", "90210")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Beverly Hills", result.City)
	assert.Equal(t, "CA", result.State)
}

func TestLookupUnknownPostalCode(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	result, err := Lookup(context.Background(), srv.Client(), srv.URL, "us", "00000")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestLookupTimeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := Lookup(ctx, srv.Client(), srv.URL, "us", "90210")
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestLookupMalformedJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	result, err := Lookup(context.Background(), srv.Client(), srv.URL, "us", "90210")
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestLookupMultiplePlacesUsesFirst(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"post_code": "02134",
			"country": "United States",
			"country_abbreviation": "US",
			"places": [
				{"place name": "Allston", "state": "Massachusetts", "state abbreviation": "MA"},
				{"place name": "Brighton", "state": "Massachusetts", "state abbreviation": "MA"}
			]
		}`))
	}))
	defer srv.Close()

	result, err := Lookup(context.Background(), srv.Client(), srv.URL, "us", "02134")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Allston", result.City)
	assert.Equal(t, "MA", result.State)
}

func TestLookupUnexpectedStatusCode(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	result, err := Lookup(context.Background(), srv.Client(), srv.URL, "us", "90210")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestLookupRejectsInvalidPostalCodeCharacters(t *testing.T) {
	t.Parallel()
	for _, code := range []string{"../us", "90210?q=1", "90210#frag", "hello\nworld"} {
		result, err := Lookup(context.Background(), &http.Client{}, "http://unused", "us", code)
		require.Error(t, err, "expected error for postal code %q", code)
		assert.Contains(t, err.Error(), "invalid postal code character")
		assert.Nil(t, result)
	}
}

func TestLookupAcceptsValidPostalCodeFormats(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(
			[]byte(
				`{"places": [{"place name": "Test", "state": "Test", "state abbreviation": "TS"}]}`,
			),
		)
	}))
	defer srv.Close()

	for _, code := range []string{"90210", "SW1A 1AA", "H0H-0H0", "1010"} {
		result, err := Lookup(context.Background(), srv.Client(), srv.URL, "us", code)
		require.NoError(t, err, "unexpected error for postal code %q", code)
		require.NotNil(t, result, "unexpected nil result for postal code %q", code)
	}
}

func TestLookupSetsUserAgent(t *testing.T) {
	t.Parallel()
	var called atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, "micasa", r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"places": []}`))
	}))
	defer srv.Close()

	_, err := Lookup(context.Background(), srv.Client(), srv.URL, "us", "90210")
	require.NoError(t, err)
	assert.True(t, called.Load(), "handler was never invoked")
}

func TestLookupEmptyPlaces(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"places": []}`))
	}))
	defer srv.Close()

	result, err := Lookup(context.Background(), srv.Client(), srv.URL, "us", "99999")
	require.NoError(t, err)
	assert.Nil(t, result)
}
