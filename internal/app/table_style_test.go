// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWarrantyStyleDateOnlyComparison(t *testing.T) {
	t.Parallel()
	// Warranty expires "2026-02-20". A user in UTC-5 at 23:00 local on
	// Feb 20 is still on the expiry date, so the warranty should be active.
	// But the absolute instant is Feb 21 04:00 UTC, which is After
	// midnight UTC Feb 20 -- the old code incorrectly shows expired.
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 2, 20, 23, 0, 0, 0, loc) // Feb 20 23:00 local

	style := warrantyStyleAt("2026-02-20", now)
	assert.Equal(t, warrantyActive, style,
		"warranty expiring today should be active, not expired")
}

func TestWarrantyStyleExpiredNextDay(t *testing.T) {
	t.Parallel()
	// Same warranty, but now it's Feb 21 local -- should be expired.
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 2, 21, 1, 0, 0, 0, loc)

	style := warrantyStyleAt("2026-02-20", now)
	assert.Equal(t, warrantyExpired, style,
		"warranty should be expired the day after expiry")
}

func TestUrgencyStyleUsesProvidedNow(t *testing.T) {
	t.Parallel()
	// Verify urgencyStyleAt uses the provided now, not real time.Now().
	// Set now to the day after the target -- should be overdue regardless
	// of what the real clock says.
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	style := urgencyStyleAt("2026-06-14", now)
	assert.Equal(t, urgencyOverdue, style,
		"item due yesterday (per provided now) should be overdue")
}

func TestUrgencyStyleDateOnlyComparison(t *testing.T) {
	t.Parallel()
	// A maintenance item due "2026-02-20". User in UTC-5 at 23:00 local
	// on Feb 19 -- locally that's 1 day away. But the absolute instant
	// is Feb 20 04:00 UTC, past midnight of the due date. The function
	// should compare local dates, not absolute instants.
	loc := time.FixedZone("UTC-5", -5*3600)
	now := time.Date(2026, 2, 19, 23, 0, 0, 0, loc) // Feb 19 23:00 local

	style := urgencyStyleAt("2026-02-20", now)
	assert.Equal(t, urgencySoon, style,
		"item due tomorrow (locally) should be 'soon', not overdue")
}
