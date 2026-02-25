// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
)

// hasPins reports whether the tab has any active pins.
func hasPins(tab *Tab) bool {
	return tab != nil && len(tab.Pins) > 0
}

// togglePin adds or removes a pin for the given column and value. Returns true
// if the value was pinned (added), false if it was unpinned (removed).
func togglePin(tab *Tab, col int, value string) bool {
	key := strings.ToLower(strings.TrimSpace(value))
	for i := range tab.Pins {
		if tab.Pins[i].Col != col {
			continue
		}
		// Column already has pins -- toggle this value.
		if tab.Pins[i].Values[key] {
			delete(tab.Pins[i].Values, key)
			// Remove the column entry entirely if no values remain.
			if len(tab.Pins[i].Values) == 0 {
				tab.Pins = append(tab.Pins[:i], tab.Pins[i+1:]...)
			}
			return false
		}
		tab.Pins[i].Values[key] = true
		return true
	}
	// No existing pin for this column -- create one.
	tab.Pins = append(tab.Pins, filterPin{
		Col:    col,
		Values: map[string]bool{key: true},
	})
	return true
}

// clearPins removes all pins and deactivates the filter.
func clearPins(tab *Tab) {
	tab.Pins = nil
	tab.FilterActive = false
	tab.FilterInverted = false
}

// clearPinsForColumn removes pins on the given column. If no pins remain,
// FilterActive is cleared.
func clearPinsForColumn(tab *Tab, col int) {
	for i := range tab.Pins {
		if tab.Pins[i].Col == col {
			tab.Pins = append(tab.Pins[:i], tab.Pins[i+1:]...)
			break
		}
	}
	if len(tab.Pins) == 0 {
		tab.FilterActive = false
		tab.FilterInverted = false
	}
}

// cellDisplayValue returns the value used for pin matching. In mag mode,
// numeric cells are transformed to their magnitude representation. NULL cells
// return a sentinel key so they can be pinned independently of empty strings.
// Entity cells return the kind name ("project", "vendor") so pinning groups
// by entity type rather than specific entity.
func cellDisplayValue(c cell, magMode bool) string {
	if c.Null {
		return nullPinKey
	}
	if c.Kind == cellEntity && len(c.Value) >= 2 && c.Value[1] == ' ' {
		if kind, ok := entityLetterKind[c.Value[0]]; ok {
			return kind
		}
	}
	if magMode {
		return strings.ToLower(strings.TrimSpace(magFormat(c, false)))
	}
	return strings.ToLower(strings.TrimSpace(c.Value))
}

// matchesAllPins checks whether a cell row satisfies all pin constraints.
// AND across columns, OR (IN) within each column. When magMode is true,
// numeric cells are compared by their magnitude value.
func matchesAllPins(cellRow []cell, pins []filterPin, magMode bool) bool {
	for _, pin := range pins {
		if pin.Col >= len(cellRow) {
			return false
		}
		cellVal := cellDisplayValue(cellRow[pin.Col], magMode)
		if !pin.Values[cellVal] {
			return false
		}
	}
	return true
}

// applyRowFilter updates the displayed rows based on pin state. When
// FilterActive is true, non-matching rows are removed. When pins exist but
// FilterActive is false (preview), all rows remain but non-matching rows are
// marked as dimmed in rowMeta. When no pins exist, displayed data mirrors Full*.
// magMode controls whether numeric cells are compared by magnitude.
func applyRowFilter(tab *Tab, magMode bool) {
	if len(tab.Pins) == 0 {
		tab.Rows = copyMeta(tab.FullMeta)
		tab.CellRows = tab.FullCellRows
		tab.Table.SetRows(tab.FullRows)
		return
	}

	if tab.FilterActive {
		// Active filter: only keep rows that satisfy the pin+invert predicate.
		var filteredRows []table.Row
		var filteredMeta []rowMeta
		var filteredCells [][]cell
		for i := range tab.FullCellRows {
			// XOR: when inverted, keep non-matching rows instead.
			if matchesAllPins(tab.FullCellRows[i], tab.Pins, magMode) != tab.FilterInverted {
				filteredRows = append(filteredRows, tab.FullRows[i])
				filteredMeta = append(filteredMeta, tab.FullMeta[i])
				filteredCells = append(filteredCells, tab.FullCellRows[i])
			}
		}
		tab.Rows = filteredMeta
		tab.CellRows = filteredCells
		tab.Table.SetRows(filteredRows)
		return
	}

	// Preview mode: keep all rows, dim those that would be filtered out.
	meta := copyMeta(tab.FullMeta)
	for i := range tab.FullCellRows {
		// XOR: when inverted, matching rows are dimmed instead.
		if matchesAllPins(tab.FullCellRows[i], tab.Pins, magMode) == tab.FilterInverted {
			meta[i].Dimmed = true
		}
	}
	tab.Rows = meta
	tab.CellRows = tab.FullCellRows
	tab.Table.SetRows(tab.FullRows)
}

// copyMeta returns a shallow copy of the metadata slice so we can set Dimmed
// flags without mutating FullMeta.
func copyMeta(src []rowMeta) []rowMeta {
	dst := make([]rowMeta, len(src))
	copy(dst, src)
	return dst
}

// isPinned reports whether the given column+value is currently pinned.
func isPinned(tab *Tab, col int, value string) bool {
	key := strings.ToLower(strings.TrimSpace(value))
	for _, pin := range tab.Pins {
		if pin.Col == col {
			return pin.Values[key]
		}
	}
	return false
}

// translatePins converts pin values between raw and magnitude representations.
// When magMode is true (just switched TO mag), raw value pins are collapsed to
// their magnitude equivalents. When false (just switched FROM mag), mag pins
// are expanded to the raw cell values that match them in the full data set.
func translatePins(tab *Tab, nowMagMode bool) {
	for i := range tab.Pins {
		pin := &tab.Pins[i]
		newValues := make(map[string]bool)
		if nowMagMode {
			// Switching to mag mode: convert each raw pinned value to its mag.
			for _, row := range tab.FullCellRows {
				if pin.Col >= len(row) {
					continue
				}
				c := row[pin.Col]
				rawKey := cellDisplayValue(c, false)
				if !pin.Values[rawKey] {
					continue
				}
				magKey := cellDisplayValue(c, true)
				newValues[magKey] = true
			}
		} else {
			// Switching from mag mode: expand each mag pin to matching raw values.
			for _, row := range tab.FullCellRows {
				if pin.Col >= len(row) {
					continue
				}
				c := row[pin.Col]
				magKey := cellDisplayValue(c, true)
				if !pin.Values[magKey] {
					continue
				}
				rawKey := cellDisplayValue(c, false)
				newValues[rawKey] = true
			}
		}
		if len(newValues) > 0 {
			pin.Values = newValues
		}
	}
}

// statusColumnIndex returns the index of the "Status" column in the tab specs,
// or -1 if not found.
func statusColumnIndex(specs []columnSpec) int {
	for i, s := range specs {
		if s.Title == "Status" {
			return i
		}
	}
	return -1
}

// hasColumnPins reports whether the tab has any pins on the given column.
func hasColumnPins(tab *Tab, col int) bool {
	for _, pin := range tab.Pins {
		if pin.Col == col {
			return len(pin.Values) > 0
		}
	}
	return false
}

// pinSummary returns a human-readable summary of active pins, e.g.
// "Status: Plan, Active · Vendor: Bob's".
func pinSummary(tab *Tab) string {
	if len(tab.Pins) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tab.Pins))
	for _, pin := range tab.Pins {
		colName := ""
		if pin.Col < len(tab.Specs) {
			colName = tab.Specs[pin.Col].Title
		}
		vals := make([]string, 0, len(pin.Values))
		for v := range pin.Values {
			if v == nullPinKey {
				vals = append(vals, symEmptySet) // ∅
			} else {
				vals = append(vals, v)
			}
		}
		parts = append(parts, colName+": "+strings.Join(vals, ", "))
	}
	return strings.Join(parts, " · ")
}
