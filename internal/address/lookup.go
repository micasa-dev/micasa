// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package address

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Result holds the city and state resolved from a postal code lookup.
type Result struct {
	City  string
	State string
}

// response mirrors the zippopotam.us JSON structure.
type response struct {
	Places []place `json:"places"`
}

type place struct {
	PlaceName         string `json:"place name"`
	State             string `json:"state"`
	StateAbbreviation string `json:"state abbreviation"`
}

// Lookup queries the postal code API for city/state. Returns nil (no error)
// when the postal code is not found (404). The baseURL parameter allows
// test injection; production callers pass "https://api.zippopotam.us".
func Lookup(
	ctx context.Context,
	client *http.Client,
	baseURL, country, postalCode string,
) (*Result, error) {
	url := fmt.Sprintf("%s/%s/%s", baseURL, country, postalCode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("postal code lookup: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("postal code lookup: unexpected status %d", resp.StatusCode)
	}

	var data response
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode postal code response: %w", err)
	}

	if len(data.Places) == 0 {
		return nil, nil
	}

	p := data.Places[0]
	return &Result{
		City:  p.PlaceName,
		State: p.StateAbbreviation,
	}, nil
}
