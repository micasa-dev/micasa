// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/charmbracelet/bubbles/table"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMagFormatMoneyWithUnit(t *testing.T) {
	// Used by magCents for dashboard (input still has $ from FormatCents).
	// No internal padding; rendering layer handles alignment.
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"thousands", "$5,234.23", "$ \U0001F8214"},
		{"hundreds", "$500.00", "$ \U0001F8213"},
		{"millions", "$1,000,000.00", "$ \U0001F8216"},
		{"zero", "$0.00", "$ \U0001F821-\u221E"},
		{"negative", "-$5.00", "-$ \U0001F8211"},
		{"negative large", "-$12,345.00", "-$ \U0001F8214"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cell{Value: tt.value, Kind: cellMoney}
			assert.Equal(t, tt.want, magFormat(c, true, "$"))
		})
	}
}

func TestMagFormatBareMoney(t *testing.T) {
	// Table cells carry $ from FormatCents. With includeUnit=false
	// the mag output strips the $ (header carries the unit instead).
	// No internal padding; table renderer handles alignment.
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"thousands", "$5,234.23", "\U0001F8214"},
		{"hundreds", "$500.00", "\U0001F8213"},
		{"millions", "$1,000,000.00", "\U0001F8216"},
		{"tens", "$42.00", "\U0001F8212"},
		{"single digit", "$7.50", "\U0001F8211"},
		{"sub-dollar", "$0.50", "\U0001F8210"},
		{"zero", "$0.00", "\U0001F821-\u221E"},
		{"negative", "-$5.00", "-\U0001F8211"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cell{Value: tt.value, Kind: cellMoney}
			assert.Equal(t, tt.want, magFormat(c, false, "$"))
		})
	}
}

func TestMagFormatDrilldown(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"count", "42", "\U0001F8212"},
		{"zero", "0", "0"},
		{"large", "1000", "\U0001F8213"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cell{Value: tt.value, Kind: cellDrilldown}
			assert.Equal(t, tt.want, magFormat(c, false, "$"))
		})
	}
}

func TestMagFormatSkipsReadonly(t *testing.T) {
	c := cell{Value: "42", Kind: cellReadonly}
	assert.Equal(t, "42", magFormat(c, false, "$"))
}

func TestMagFormatSkipsNonNumericKinds(t *testing.T) {
	tests := []struct {
		name  string
		value string
		kind  cellKind
	}{
		{"text name", "Kitchen Remodel", cellText},
		{"status", "underway", cellStatus},
		{"date", "2026-02-12", cellDate},
		{"warranty date", "2027-06-15", cellWarranty},
		{"urgency date", "2026-03-01", cellUrgency},
		{"notes", "Some long note", cellNotes},
		{"empty text", "", cellText},
		{"dash money", "\u2014", cellMoney},
		{"readonly id", "7", cellReadonly},

		// Numeric-looking cellText values that must NOT be transformed:
		// phone numbers, serial numbers, model numbers, zip codes.
		{"phone number", "5551234567", cellText},
		{"formatted phone", "(555) 123-4567", cellText},
		{"serial number", "123456789", cellText},
		{"model number", "12345", cellText},
		{"zip code", "90210", cellText},
		{"interval", "3m", cellText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cell{Value: tt.value, Kind: tt.kind}
			assert.Equal(t, tt.value, magFormat(c, false, "$"), "value should be unchanged")
		})
	}
}

func TestMagTransformCells(t *testing.T) {
	rows := [][]cell{
		{
			{Value: "1", Kind: cellReadonly},
			{Value: "Kitchen Remodel", Kind: cellText},
			{Value: "$5,234.23", Kind: cellMoney},
			{Value: "3", Kind: cellDrilldown},
		},
		{
			{Value: "2", Kind: cellReadonly},
			{Value: "Deck", Kind: cellText},
			{Value: "$100.00", Kind: cellMoney},
			{Value: "0", Kind: cellDrilldown},
		},
	}
	out := magTransformCells(rows, "$")

	// ID cells unchanged.
	assert.Equal(t, "1", out[0][0].Value)
	assert.Equal(t, "2", out[1][0].Value)

	// Text cells unchanged.
	assert.Equal(t, "Kitchen Remodel", out[0][1].Value)
	assert.Equal(t, "Deck", out[1][1].Value)

	// Money cells: magnitude only ($ stripped by transform).
	assert.Equal(t, "\U0001F8214", out[0][2].Value)
	assert.Equal(t, "\U0001F8212", out[1][2].Value)

	// Drilldown: non-zero count transformed, zero left alone.
	assert.Equal(t, "\U0001F8210", out[0][3].Value)
	assert.Equal(t, "0", out[1][3].Value)

	// Original rows are not modified.
	assert.Equal(t, "$5,234.23", rows[0][2].Value)
}

func TestMagTransformCellsPreservesNull(t *testing.T) {
	rows := [][]cell{
		{
			{Value: "", Kind: cellMoney, Null: true},
			{Value: "Kitchen", Kind: cellText},
		},
	}
	out := magTransformCells(rows, "$")
	assert.True(t, out[0][0].Null, "Null flag should be preserved through mag transform")
	assert.Equal(t, cellMoney, out[0][0].Kind)
}

func TestMagTransformText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"dollar amount",
			"You spent $5,234.23 on kitchen.",
			"You spent $ \U0001F8214 on kitchen.",
		},
		{
			"multiple amounts",
			"Budget is $10,000.00 and actual is $8,500.00.",
			"Budget is $ \U0001F8214 and actual is $ \U0001F8214.",
		},
		{
			"negative amount",
			"Loss of -$500.00 this month.",
			"Loss of -$ \U0001F8213 this month.",
		},
		{
			"no amounts or numbers",
			"The project is underway.",
			"The project is underway.",
		},
		{
			"small amount",
			"Just $5.00.",
			"Just $ \U0001F8211.",
		},
		{
			"bare count",
			"There is 1 flooring project.",
			"There is \U0001F8210 flooring project.",
		},
		{
			"larger bare count",
			"You have 42 maintenance items.",
			"You have \U0001F8212 maintenance items.",
		},
		{
			"bare number with commas",
			"Total is 1,000 items.",
			"Total is \U0001F8213 items.",
		},
		{
			"mixed dollars and bare numbers",
			"Found 3 projects totaling $15,000.00.",
			"Found \U0001F8210 projects totaling $ \U0001F8214.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, magTransformText(tt.input, "$"))
		})
	}
}

func TestMagModeToggle(t *testing.T) {
	m := newTestModel()
	seedMoneyCells(m)

	// Initially off: compact money format visible ($ in header, "5k" in cell).
	assert.False(t, m.magMode)
	view := m.buildView()
	assert.Contains(t, view, "5k",
		"compact money should appear with mag mode off")
	assert.NotContains(t, view, magArrow,
		"magnitude notation should not appear with mag mode off")

	// Toggle on: compact replaced by magnitude notation.
	sendKey(m, "ctrl+o")
	assert.True(t, m.magMode)
	view = m.buildView()
	assert.Contains(t, view, magArrow,
		"magnitude notation should appear with mag mode on")

	// Toggle off: compact money restored.
	sendKey(m, "ctrl+o")
	assert.False(t, m.magMode)
	view = m.buildView()
	assert.Contains(t, view, "5k",
		"compact money should reappear after toggling mag mode off")
	assert.NotContains(t, view, magArrow,
		"magnitude notation should disappear after toggling off")
}

func TestMagModeWorksInEditMode(t *testing.T) {
	m := newTestModel()
	seedMoneyCells(m)
	sendKey(m, "i")

	// Initially off in edit mode: compact money format visible.
	assert.False(t, m.magMode)
	view := m.buildView()
	assert.Contains(t, view, "5k",
		"compact money should appear with mag mode off in edit mode")

	// Toggle on in edit mode.
	sendKey(m, "ctrl+o")
	assert.True(t, m.magMode)
	view = m.buildView()
	assert.Contains(t, view, magArrow,
		"magnitude notation should appear with mag mode on in edit mode")
}

// seedTabMoneyCells populates the given tab with money cell rows and wires
// the Full* fields so pin translation works.
func seedTabMoneyCells(tab *Tab, amounts []string) {
	cellRows := make([][]cell, len(amounts))
	rows := make([]table.Row, len(amounts))
	meta := make([]rowMeta, len(amounts))
	for i, amt := range amounts {
		cr := make([]cell, len(tab.Specs))
		r := make(table.Row, len(tab.Specs))
		for j, spec := range tab.Specs {
			switch spec.Kind {
			case cellMoney:
				cr[j] = cell{Value: amt, Kind: cellMoney}
			case cellReadonly:
				cr[j] = cell{Value: "1", Kind: cellReadonly}
			default:
				cr[j] = cell{Value: "test", Kind: spec.Kind}
			}
			r[j] = cr[j].Value
		}
		cellRows[i] = cr
		rows[i] = r
		meta[i] = rowMeta{ID: uint(i + 1)} //nolint:gosec // i bounded by slice length
	}
	tab.CellRows = cellRows
	tab.FullCellRows = cellRows
	tab.FullRows = rows
	tab.FullMeta = meta
	tab.Rows = meta
	tab.Table.SetRows(rows)
}

func TestMagModeTranslatesPinsOnAllTabs(t *testing.T) {
	m := newTestModel()
	m.showDashboard = false

	// Seed money cells on the active tab (Projects, index 0) and a
	// non-active tab (Quotes, index 1).
	quotesIdx := tabIndex(tabQuotes)
	require.Greater(t, len(m.tabs), quotesIdx)

	seedTabMoneyCells(&m.tabs[m.active], []string{"$5,000.00", "$1,234.00"})
	seedTabMoneyCells(&m.tabs[quotesIdx], []string{"$1,234.00", "$50.00"})

	// Find the first money column on each tab.
	projectsMoneyCol := -1
	for i, s := range m.tabs[m.active].Specs {
		if s.Kind == cellMoney {
			projectsMoneyCol = i
			break
		}
	}
	quotesMoneyCol := -1
	for i, s := range m.tabs[quotesIdx].Specs {
		if s.Kind == cellMoney {
			quotesMoneyCol = i
			break
		}
	}
	require.NotEqual(t, -1, projectsMoneyCol)
	require.NotEqual(t, -1, quotesMoneyCol)

	// Pin "$1,234.00" on the non-active quotes tab (raw mode pin).
	togglePin(&m.tabs[quotesIdx], quotesMoneyCol, "$1,234.00")
	require.True(t, hasPins(&m.tabs[quotesIdx]))
	require.True(t, m.tabs[quotesIdx].Pins[0].Values["$1,234.00"],
		"raw pin should exist before toggle")

	// Toggle mag mode via ctrl+o.
	sendKey(m, "ctrl+o")
	require.True(t, m.magMode)

	// The non-active tab's pin should be translated to the mag
	// representation. $1,234 -> log10(1234) ~ 3.09 -> rounds to 3.
	assert.False(t, m.tabs[quotesIdx].Pins[0].Values["$1,234.00"],
		"raw pin should be gone after mag toggle")
	assert.True(t, m.tabs[quotesIdx].Pins[0].Values[magArrow+"3"],
		"mag pin should exist on non-active tab after toggle")

	// Toggle mag mode off.
	sendKey(m, "ctrl+o")
	require.False(t, m.magMode)

	// Pin should be translated back to raw.
	assert.True(t, m.tabs[quotesIdx].Pins[0].Values["$1,234.00"],
		"raw pin should be restored after toggling mag off")
	assert.False(t, m.tabs[quotesIdx].Pins[0].Values[magArrow+"3"],
		"mag pin should be gone after toggling mag off")
}

// seedMoneyCells populates the active tab with a row containing a money cell
// so that buildView renders dollar values affected by mag mode toggling.
func seedMoneyCells(m *Model) {
	tab := m.effectiveTab()
	row := make([]cell, len(tab.Specs))
	for i, spec := range tab.Specs {
		switch spec.Kind {
		case cellMoney:
			row[i] = cell{Value: "$5,000.00", Kind: cellMoney}
		case cellReadonly:
			row[i] = cell{Value: "1", Kind: cellReadonly}
		default:
			row[i] = cell{Value: "test", Kind: spec.Kind}
		}
	}
	tab.CellRows = [][]cell{row}
	tab.FullCellRows = tab.CellRows
}

func TestMagCentsIncludesUnit(t *testing.T) {
	cur := locale.DefaultCurrency()
	assert.Equal(t, "$ \U0001F8214", magCents(523423, cur))
	assert.Equal(t, "$ \U0001F8213", magCents(50000, cur))
	assert.Equal(t, "$ \U0001F8210", magCents(100, cur))
}

func TestMagOptionalCentsNil(t *testing.T) {
	cur := locale.DefaultCurrency()
	assert.Equal(t, "", magOptionalCents(nil, cur))
}

func TestMagOptionalCentsPresent(t *testing.T) {
	cur := locale.DefaultCurrency()
	cents := int64(100000)
	assert.Equal(t, "$ \U0001F8213", magOptionalCents(&cents, cur))
}
