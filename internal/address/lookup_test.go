// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package address

import (
	"context"
	"net/http"
	"net/http/httptest"
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
