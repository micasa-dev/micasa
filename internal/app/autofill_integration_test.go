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

// TestAutofillIntegrationFullLifecycle verifies the complete autofill flow
// using the value-change trigger: type postal code chars into the focused
// field, and when the value reaches 3+ chars the lookup fires automatically.
func TestAutofillIntegrationFullLifecycle(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"post_code": "90210",
			"country": "United States",
			"country abbreviation": "US",
			"places": [{"place name": "Beverly Hills", "longitude": "-118.4065", "latitude": "34.0901", "state": "California", "state abbreviation": "CA"}]
		}`))
	}))
	defer srv.Close()

	m := newTestModelWithStore(t)
	m.addressClient = srv.Client()
	m.addressBaseURL = srv.URL
	m.addressCountry = "us"
	m.addressAutofill = true

	// Open the house form via key sequence.
	sendKey(m, "i")
	sendKey(m, "p")
	require.Equal(t, modeForm, m.mode)
	require.NotNil(t, m.fs.postalCodeField)

	// Advance to postal code field using huh's NextField message.
	m.Update(huh.NextField())

	// Type postal code characters. On the 3rd char (when value reaches
	// minimum length), the lookup is dispatched as a tea.Cmd.
	sendKey(m, "9")
	sendKey(m, "0")
	// The 3rd char triggers the lookup — capture the cmd.
	_, cmd := m.Update(keyPress("2"))
	require.NotNil(t, cmd)

	// Execute all commands and feed results back (including the lookup).
	processCmd(t, m, cmd)

	// Continue typing the rest.
	sendKey(m, "1")
	sendKey(m, "0")

	values, ok := m.fs.formData.(*houseFormData)
	require.True(t, ok)

	assert.Equal(t, "Beverly Hills", values.City,
		"city struct field should be auto-filled")
	assert.Equal(t, "CA", values.State,
		"state struct field should be auto-filled")

	// Verify the rendered View actually shows the autofilled values.
	// This catches huh buffer desync issues where the struct is updated
	// but the form renders stale empty fields.
	view := m.View()
	assert.Contains(t, view.Content, "Beverly Hills",
		"rendered view should show auto-filled city")
	assert.Contains(t, view.Content, "CA",
		"rendered view should show auto-filled state")
}

// processCmd executes a tea.Cmd and feeds all resulting messages back through
// m.Update, recursively processing any commands those produce. Limits depth
// to prevent infinite loops.
func processCmd(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	processWithDepth(t, m, cmd, 10)
}

func processWithDepth(t *testing.T, m *Model, cmd tea.Cmd, depth int) {
	t.Helper()
	if cmd == nil || depth <= 0 {
		return
	}
	msg := cmd()
	if msg == nil {
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			processWithDepth(t, m, c, depth-1)
		}
		return
	}
	// Skip infinite-loop messages.
	switch msg.(type) {
	case tea.WindowSizeMsg, tea.BackgroundColorMsg:
		return
	}
	_, nextCmd := m.Update(msg)
	processWithDepth(t, m, nextCmd, depth-1)
}
