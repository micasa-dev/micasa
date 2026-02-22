// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// Compact intervals
// ---------------------------------------------------------------------------

func TestFormatInterval(t *testing.T) {
	tests := []struct {
		name   string
		months int
		want   string
	}{
		{"zero", 0, ""},
		{"negative", -3, ""},
		{"one month", 1, "1m"},
		{"three months", 3, "3m"},
		{"six months", 6, "6m"},
		{"eleven months", 11, "11m"},
		{"one year", 12, "1y"},
		{"two years", 24, "2y"},
		{"year and a half", 18, "1y 6m"},
		{"complex", 27, "2y 3m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatInterval(tt.months))
		})
	}
}

// ---------------------------------------------------------------------------
// Status labels
// ---------------------------------------------------------------------------

func TestStatusLabels(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"ideating", "idea"},
		{"planned", "plan"},
		{"quoted", "bid"},
		{"underway", "wip"},
		{"delayed", "hold"},
		{"completed", "done"},
		{"abandoned", "drop"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, statusLabel(tt.status))
		})
	}
}

func TestStatusLabelsAreDistinct(t *testing.T) {
	seen := make(map[string]string)
	for status, label := range statusLabels {
		if prev, ok := seen[label]; ok {
			t.Errorf("duplicate label %q for %q and %q", label, prev, status)
		}
		seen[label] = status
	}
}

func TestStatusStylesExistForAll(t *testing.T) {
	styles := DefaultStyles()
	for status := range statusLabels {
		_, ok := styles.StatusStyle(status)
		assert.True(t, ok, "missing StatusStyle for %q", status)
	}
}

// ---------------------------------------------------------------------------
// Compact money
// ---------------------------------------------------------------------------

func TestCompactMoneyValue(t *testing.T) {
	cur := locale.DefaultCurrency()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"small stays full", "$500.00", "500.00"},
		{"thousands", "$5,234.23", "5.2k"},
		{"round thousands", "$45,000.00", "45k"},
		{"millions", "$1,300,000.00", "1.3M"},
		{"empty", "", ""},
		{"dash", "—", "—"},
		{"unparseable", "not money", "not money"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, compactMoneyValue(tt.input, cur))
		})
	}
}

func TestCompactMoneyCells(t *testing.T) {
	cur := locale.DefaultCurrency()
	rows := [][]cell{
		{
			{Value: "1", Kind: cellReadonly},
			{Value: "Kitchen", Kind: cellText},
			{Value: "$5,234.23", Kind: cellMoney},
			{Value: "3", Kind: cellDrilldown},
		},
		{
			{Value: "2", Kind: cellReadonly},
			{Value: "Deck", Kind: cellText},
			{Value: "$100.00", Kind: cellMoney},
			{Value: "", Kind: cellMoney},
		},
	}
	out := compactMoneyCells(rows, cur)

	// Non-money cells unchanged.
	assert.Equal(t, "1", out[0][0].Value)
	assert.Equal(t, "Kitchen", out[0][1].Value)
	assert.Equal(t, "3", out[0][3].Value)

	// Money cells compacted, $ stripped (header carries the unit).
	assert.Equal(t, "5.2k", out[0][2].Value)
	assert.Equal(t, "100.00", out[1][2].Value)

	// Empty money cell stays empty.
	assert.Equal(t, "", out[1][3].Value)

	// Original rows not modified.
	assert.Equal(t, "$5,234.23", rows[0][2].Value)
}

func TestCompactMoneyCellsPreservesNull(t *testing.T) {
	cur := locale.DefaultCurrency()
	rows := [][]cell{
		{
			{Value: "", Kind: cellMoney, Null: true},
			{Value: "Kitchen", Kind: cellText},
		},
	}
	out := compactMoneyCells(rows, cur)
	assert.True(t, out[0][0].Null, "Null flag should be preserved through compact transform")
	assert.Equal(t, cellMoney, out[0][0].Kind)
}

func TestAnnotateMoneyHeaders(t *testing.T) {
	cur := locale.DefaultCurrency()
	specs := []columnSpec{
		{Title: "Name", Kind: cellText},
		{Title: "Budget", Kind: cellMoney},
		{Title: "Actual", Kind: cellMoney},
		{Title: "ID", Kind: cellReadonly},
	}
	out := annotateMoneyHeaders(specs, cur)

	// Non-money columns unchanged.
	assert.Equal(t, "Name", out[0].Title)
	assert.Equal(t, "ID", out[3].Title)

	// Money columns get styled "$" suffix.
	assert.Contains(t, out[1].Title, "Budget")
	assert.Contains(t, out[1].Title, "$")
	assert.Contains(t, out[2].Title, "Actual")
	assert.Contains(t, out[2].Title, "$")

	// Original specs unmodified.
	assert.Equal(t, "Budget", specs[1].Title)
}
