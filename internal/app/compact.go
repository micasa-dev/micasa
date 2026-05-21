// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/locale"
)

// statusLabelPlanned is the abbreviated display label for the "planned"
// project status. It is also used as a filter-query token in tests.
const statusLabelPlanned = "plan"

// statusLabels maps full status names to short display labels.
var statusLabels = map[string]string{
	// Project statuses.
	data.ProjectStatusIdeating:   "idea",
	data.ProjectStatusPlanned:    statusLabelPlanned,
	data.ProjectStatusQuoted:     "bid",
	data.ProjectStatusInProgress: "wip",
	data.ProjectStatusDelayed:    "hold",
	data.ProjectStatusCompleted:  "done",
	data.ProjectStatusAbandoned:  "drop",
	// Incident statuses.
	data.IncidentStatusOpen:       "open",
	data.IncidentStatusInProgress: "act",
	data.IncidentStatusResolved:   "res",
	// Incident severities.
	data.IncidentSeverityUrgent:   "urg",
	data.IncidentSeveritySoon:     "soon",
	data.IncidentSeverityWhenever: "low",
	// Seasons.
	data.SeasonSpring: "spr",
	data.SeasonSummer: "sum",
	data.SeasonFall:   "fall",
	data.SeasonWinter: "win",
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
