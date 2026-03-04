// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package fake

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestGeneratedTimestampsUseUTC verifies that date-producing generators
// use UTC bounds so that the same seed produces identical output regardless
// of the host's TZ setting. gofakeit.DateRange works on Unix timestamps
// (timezone-independent), but using UTC bounds is defensive best practice.
func TestGeneratedTimestampsUseUTC(t *testing.T) {
	t.Parallel()
	h := New(42)

	t.Run("Appliance", func(t *testing.T) {
		a := h.Appliance()
		if a.PurchaseDate != nil {
			assert.Equal(t, time.UTC, a.PurchaseDate.Location(),
				"Appliance.PurchaseDate should be UTC")
		}
	})

	t.Run("ServiceLogEntry", func(t *testing.T) {
		s := h.ServiceLogEntry()
		assert.Equal(t, time.UTC, s.ServicedAt.Location(),
			"ServiceLogEntry.ServicedAt should be UTC")
	})

	t.Run("Incident", func(t *testing.T) {
		inc := h.Incident()
		assert.Equal(t, time.UTC, inc.DateNoticed.Location(),
			"Incident.DateNoticed should be UTC")
	})
}
