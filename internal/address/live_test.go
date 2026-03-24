// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package address

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestLookupLiveAPI is a manual smoke test against the real zippopotam.us API.
// Run with: go test ./internal/address/ -run TestLookupLiveAPI -v
// Skipped in normal test runs to avoid network dependency.
func TestLookupLiveAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := Lookup(ctx, &http.Client{}, "https://api.zippopotam.us", "us", "90210")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	t.Logf("City: %q State: %q", result.City, result.State)
	if result.City == "" {
		t.Error("City is empty — JSON keys likely don't match the API response")
	}
	if result.State == "" {
		t.Error("State is empty — JSON keys likely don't match the API response")
	}
}
