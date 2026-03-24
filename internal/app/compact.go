// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"

	"github.com/micasa-dev/micasa/internal/locale"
)

// statusLabels maps full status names to short display labels.
var statusLabels = map[string]string{
	// Project statuses.
	"ideating":  "idea",
	"planned":   "plan",
	"quoted":    "bid",
	"underway":  "wip",
	"delayed":   "hold",
	"completed": "done",
	"abandoned": "drop",
	// Incident statuses.
	"open":        "open",
	"in_progress": "act",
	"resolved":    "res",
	// Incident severities.
	"urgent":   "urg",
	"soon":     "soon",
	"whenever": "low",
	// Seasons.
	"spring": "spr",
	"summer": "sum",
	"fall":   "fall",
	"winter": "win",
}

// statusLabel returns the short display label for a status value.
func statusLabel(status string) string {
	if label, ok := statusLabels[status]; ok {
		return label
	}
	return status
}

// annotateMoneyHeaders returns a copy of specs with the currency symbol
// appended to money column titles. The unit lives in the header so cell
// values can be bare numbers.
func annotateMoneyHeaders(specs []columnSpec, cur locale.Currency) []columnSpec {
	out := make([]columnSpec, len(specs))
	copy(out, specs)
	for i, spec := range out {
		if spec.Kind == cellMoney {
			out[i].Title = spec.Title + " " + appStyles.Money().Render(cur.Symbol())
		}
	}
	return out
}

// compactMoneyCells returns a copy of the cell grid with money values
// replaced by their compact representation (e.g. "1.2k") without the
// currency symbol (which lives in the column header). The original cells
// are not modified so sorting continues to work on full-precision values.
func compactMoneyCells(rows [][]cell, cur locale.Currency) [][]cell {
	return transformCells(rows, func(c cell) cell {
		if c.Kind != cellMoney {
			return c
		}
		c.Value = compactMoneyValue(c.Value, cur)
		return c
	})
}

// compactMoneyValue converts a full-precision money string to compact form
// without the currency symbol (e.g. "5.2k", "100.00"). The symbol is
// handled by the column header annotation instead.
func compactMoneyValue(v string, cur locale.Currency) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "—" {
		return v
	}
	cents, err := cur.ParseRequiredCents(v)
	if err != nil {
		return v
	}
	compact := cur.FormatCompactCents(cents)
	compact = strings.TrimPrefix(compact, cur.Symbol())
	compact = strings.TrimSuffix(compact, cur.Symbol())
	compact = strings.TrimSpace(compact)
	compact = strings.Trim(compact, "\u00a0")
	return compact
}
