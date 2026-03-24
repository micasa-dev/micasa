// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHouseFormPostalCodeIsSecondField(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	openHouseForm(m)

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)

	// Focus starts on Nickname (field 0). Advance to the second field.
	m.Update(huh.NextField())

	// Type into the focused field. If postal code is correctly the
	// second field, the keystrokes should appear in values.PostalCode.
	for _, ch := range "90210" {
		sendKey(m, string(ch))
	}
	assert.Equal(t, "90210", values.PostalCode,
		"second field should be postal code, got value in wrong field")
	assert.Empty(t, values.AddressLine1,
		"address line 1 should still be empty")
}

// drainBatchCmds executes a tea.Cmd (which may be a tea.Batch) and
// collects all resulting messages.
func drainBatchCmds(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, drainBatchCmds(c)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func TestPostalCodeLookupDispatchedOnThirdChar(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"post_code": "90210",
			"places": [{"place name": "Beverly Hills", "state abbreviation": "CA"}]
		}`))
	}))
	defer srv.Close()

	m := newTestModelWithStore(t)
	m.addressClient = srv.Client()
	m.addressBaseURL = srv.URL
	m.addressCountry = "us"
	m.addressAutofill = true

	openHouseForm(m)

	// Navigate to the postal code field.
	m.Update(huh.NextField())
	require.Equal(t, m.fs.postalCodeField, m.fs.form.GetFocusedField())

	// Type first two chars — no lookup yet (below minimum length).
	sendKey(m, "9")
	sendKey(m, "0")

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)
	require.Equal(t, "90", values.PostalCode)

	// Type the third char — this should trigger the lookup.
	_, cmd := m.Update(keyPress("2"))
	require.NotNil(t, cmd)

	msgs := drainBatchCmds(cmd)
	var lookupMsg *postalCodeLookupMsg
	for _, msg := range msgs {
		if m, ok := msg.(postalCodeLookupMsg); ok {
			lookupMsg = &m
			break
		}
	}
	require.NotNil(t, lookupMsg, "lookup should dispatch on 3rd character")
	require.True(t, called, "should hit the mock HTTP server")
	require.NoError(t, lookupMsg.Err)

	// Feed the result back.
	m.Update(*lookupMsg)

	assert.Equal(t, "Beverly Hills", values.City)
	assert.Equal(t, "CA", values.State)
}

func TestPostalCodeChangeUpdatesAutofilledCityState(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/us/902":
			_, _ = w.Write(
				[]byte(`{"places": [{"place name": "Los Angeles", "state abbreviation": "CA"}]}`),
			)
		case "/us/9021":
			_, _ = w.Write(
				[]byte(`{"places": [{"place name": "Beverly Hills", "state abbreviation": "CA"}]}`),
			)
		case "/us/100":
			_, _ = w.Write(
				[]byte(`{"places": [{"place name": "New York", "state abbreviation": "NY"}]}`),
			)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := newTestModelWithStore(t)
	m.addressClient = srv.Client()
	m.addressBaseURL = srv.URL
	m.addressCountry = "us"
	m.addressAutofill = true

	openHouseForm(m)
	m.Update(huh.NextField())

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)

	// Type "902" — first lookup fills city/state.
	sendKey(m, "9")
	sendKey(m, "0")
	_, cmd := m.Update(keyPress("2"))
	for _, msg := range drainBatchCmds(cmd) {
		m.Update(msg)
	}
	assert.Equal(t, "Los Angeles", values.City)
	assert.Equal(t, "CA", values.State)

	// Type "1" making it "9021" — city/state should update because they
	// were auto-filled, not user-typed.
	_, cmd = m.Update(keyPress("1"))
	for _, msg := range drainBatchCmds(cmd) {
		m.Update(msg)
	}
	assert.Equal(t, "Beverly Hills", values.City,
		"city should update when postal code changes and previous value was auto-filled")
	assert.Equal(t, "CA", values.State)
}

func TestPostalCodeChangeDoesNotOverwriteUserEditedCity(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/us/902":
			_, _ = w.Write(
				[]byte(`{"places": [{"place name": "Los Angeles", "state abbreviation": "CA"}]}`),
			)
		case "/us/9021":
			_, _ = w.Write(
				[]byte(`{"places": [{"place name": "Beverly Hills", "state abbreviation": "CA"}]}`),
			)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := newTestModelWithStore(t)
	m.addressClient = srv.Client()
	m.addressBaseURL = srv.URL
	m.addressCountry = "us"
	m.addressAutofill = true

	openHouseForm(m)
	m.Update(huh.NextField())

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)

	// First lookup fills city.
	sendKey(m, "9")
	sendKey(m, "0")
	_, cmd := m.Update(keyPress("2"))
	for _, msg := range drainBatchCmds(cmd) {
		m.Update(msg)
	}
	require.Equal(t, "Los Angeles", values.City)

	// User manually changes city — must also update huh's buffer
	// since direct struct assignment gets overwritten by huh's sync.
	values.City = "Custom City"
	if m.fs.cityInput != nil {
		m.fs.cityInput.Value(&values.City)
	}

	// Postal code changes — should NOT overwrite the user's edit.
	_, cmd = m.Update(keyPress("1"))
	for _, msg := range drainBatchCmds(cmd) {
		m.Update(msg)
	}
	assert.Equal(t, "Custom City", values.City,
		"user-edited city should not be overwritten by autofill")
}

func TestPostalCodeAutofillDoesNotOverwriteExistingValues(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"post_code": "90210",
			"places": [{"place name": "Beverly Hills", "state abbreviation": "CA"}]
		}`))
	}))
	defer srv.Close()

	m := newTestModelWithStore(t)
	m.addressClient = srv.Client()
	m.addressBaseURL = srv.URL
	m.addressCountry = "us"
	m.addressAutofill = true

	openHouseForm(m)

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)
	values.PostalCode = "90210"
	values.City = "Custom City"
	values.State = "XX"

	cmd := lookupPostalCodeCmd(m.addressClient, m.addressBaseURL, m.addressCountry, "90210")
	msg := cmd()
	m.Update(msg)

	assert.Equal(t, "Custom City", values.City)
	assert.Equal(t, "XX", values.State)
}

func TestPostalCodeAutofillDisabledByConfig(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.addressAutofill = false

	openHouseForm(m)
	m.Update(huh.NextField())

	// Type a full postal code.
	for _, ch := range "90210" {
		sendKey(m, string(ch))
	}

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)
	// No lookup should have been dispatched.
	assert.Empty(t, values.City)
	assert.Empty(t, values.State)
}

func TestPostalCodeInvalidClearsAutofilledValues(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/us/23490":
			_, _ = w.Write(
				[]byte(
					`{"places": [{"place name": "Virginia Beach", "state abbreviation": "VA"}]}`,
				),
			)
		default:
			// Unknown postal code — return 404.
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := newTestModelWithStore(t)
	m.addressClient = srv.Client()
	m.addressBaseURL = srv.URL
	m.addressCountry = "us"
	m.addressAutofill = true

	openHouseForm(m)
	m.Update(huh.NextField())

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)

	// Type "23490" — valid, fills city/state.
	for _, ch := range "23490" {
		_, cmd := m.Update(keyPress(string(ch)))
		for _, msg := range drainBatchCmds(cmd) {
			m.Update(msg)
		}
	}
	require.Equal(t, "Virginia Beach", values.City)
	require.Equal(t, "VA", values.State)

	// User deletes the "0" making it "2349" — invalid, should clear.
	_, cmd := m.Update(keyPress("backspace"))
	for _, msg := range drainBatchCmds(cmd) {
		m.Update(msg)
	}
	assert.Empty(t, values.City,
		"city should be cleared when postal code becomes invalid")
	assert.Empty(t, values.State,
		"state should be cleared when postal code becomes invalid")
}

func TestPostalCodeTooShortSkipsLookup(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.addressAutofill = true

	openHouseForm(m)
	m.Update(huh.NextField())

	// Type only 2 chars — below minimum.
	sendKey(m, "9")
	sendKey(m, "0")

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)
	assert.Empty(t, values.City)
	assert.Empty(t, values.State)
}
