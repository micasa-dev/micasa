// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllTabsHaveHandlers(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	for i, tab := range m.tabs {
		require.NotNilf(t, tab.Handler, "tab %d (%s) has nil handler", i, tab.Name)
	}
}

func TestHandlerForFormKind(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	cases := []struct {
		kind FormKind
		name string
	}{
		{formProject, "project"},
		{formQuote, "quote"},
		{formMaintenance, "maintenance"},
		{formAppliance, "appliance"},
	}

	for _, tc := range cases {
		handler := m.handlerForFormKind(tc.kind)
		require.NotNilf(t, handler, "expected handler for %s", tc.name)
		assert.Equalf(
			t,
			tc.kind,
			handler.FormKind(),
			"handler for %s returned wrong FormKind",
			tc.name,
		)
	}
}

func TestHandlerForFormKindReturnsNilForNonTabKinds(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	for _, kind := range []FormKind{formHouse, formNone} {
		assert.Nilf(t, m.handlerForFormKind(kind), "expected nil handler for %v", kind)
	}
}

func TestHandlerFormKindMatchesTabKind(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	expected := map[TabKind]FormKind{
		tabProjects:    formProject,
		tabQuotes:      formQuote,
		tabMaintenance: formMaintenance,
		tabAppliances:  formAppliance,
	}
	for _, tab := range m.tabs {
		want, ok := expected[tab.Kind]
		if !ok {
			continue
		}
		assert.Equalf(t, want, tab.Handler.FormKind(),
			"tab %s handler FormKind() mismatch", tab.Name)
	}
}
