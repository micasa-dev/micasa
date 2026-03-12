// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGeneratedColumnCountsMatchDefs ensures the generated iota constants
// stay in sync with the declarative columnDef slices. If a column is added
// to or removed from a defs slice without re-running go generate, the last
// iota constant will no longer equal len(defs)-1.
func TestGeneratedColumnCountsMatchDefs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		lastCol int
		defs    []columnDef
	}{
		{"project", int(projectColDocs), projectColumnDefs},
		{"quote", int(quoteColDocs), quoteColumnDefs},
		{"maintenance", int(maintenanceColDocs), maintenanceColumnDefs},
		{"incident", int(incidentColDocs), incidentColumnDefs},
		{"appliance", int(applianceColDocs), applianceColumnDefs},
		{"vendor", int(vendorColDocs), vendorColumnDefs},
		{"serviceLog", int(serviceLogColDocs), serviceLogColumnDefs},
		{"vendorJobs", int(vendorJobsColNotes), vendorJobsColumnDefs},
		{"document", int(documentColUpdated), documentColumnDefs},
	}

	for _, tc := range cases {
		assert.Equalf(
			t,
			len(tc.defs)-1,
			tc.lastCol,
			"%s: last iota constant (%d) does not match len(defs)-1 (%d); re-run go generate ./internal/app/",
			tc.name,
			tc.lastCol,
			len(tc.defs)-1,
		)
	}
}

// TestGeneratedColumnNamesMatchDefs validates that each iota constant's
// position matches its name in the columnDef slice. Catches reordering
// mismatches between the defs and the generated constants.
func TestGeneratedColumnNamesMatchDefs(t *testing.T) {
	t.Parallel()

	// Spot-check representative constants across entities. Each check
	// verifies bounds first so a stale constant produces a clear failure
	// instead of a panic.
	assertColDef(t, projectColumnDefs, int(projectColType), 1, "Type", "projectColType")
	assertColDef(t, quoteColumnDefs, int(quoteColVendor), 2, "Vendor", "quoteColVendor")
	assertColDef(
		t,
		maintenanceColumnDefs,
		int(maintenanceColSeason),
		3,
		"Season",
		"maintenanceColSeason",
	)
	assertColDef(
		t,
		incidentColumnDefs,
		int(incidentColLocation),
		4,
		"Location",
		"incidentColLocation",
	)
	assertColDef(
		t,
		applianceColumnDefs,
		int(applianceColPurchased),
		6,
		"Purchased",
		"applianceColPurchased",
	)
	assertColDef(t, vendorColumnDefs, int(vendorColEmail), 3, "Email", "vendorColEmail")
	assertColDef(
		t,
		serviceLogColumnDefs,
		int(serviceLogColPerformedBy),
		2,
		"PerformedBy",
		"serviceLogColPerformedBy",
	)
	assertColDef(t, vendorJobsColumnDefs, int(vendorJobsColItem), 1, "Item", "vendorJobsColItem")
	assertColDef(t, documentColumnDefs, int(documentColEntity), 2, "Entity", "documentColEntity")
}

// assertColDef checks that a generated column index is within range, then
// verifies both its numeric position and column name. Prevents panics when
// generated constants are stale relative to the columnDef declarations.
func assertColDef(
	t *testing.T,
	defs []columnDef,
	idx, expectedPos int,
	expectedName, label string,
) {
	t.Helper()

	if !assert.Greaterf(
		t, len(defs), idx,
		"%s: index %d out of range for len(defs)=%d; re-run go generate ./internal/app/",
		label, idx, len(defs),
	) {
		return
	}

	assert.Equalf(t, expectedPos, idx, "%s position", label)
	assert.Equalf(t, expectedName, defs[idx].name, "%s name", label)
}
