// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"net/http"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/cpcloud/micasa/internal/address"
)

const (
	postalCodeMinLength     = 3
	postalCodeLookupTimeout = 3 * time.Second
	postalCodeAPIBaseURL    = "https://api.zippopotam.us"
)

type postalCodeLookupMsg struct {
	City  string
	State string
	Err   error
}

func lookupPostalCodeCmd(client *http.Client, baseURL, country, postalCode string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), postalCodeLookupTimeout)
		defer cancel()

		result, err := address.Lookup(ctx, client, baseURL, country, postalCode)
		if err != nil {
			return postalCodeLookupMsg{Err: err}
		}
		if result == nil {
			return postalCodeLookupMsg{}
		}
		return postalCodeLookupMsg{
			City:  result.City,
			State: result.State,
		}
	}
}
