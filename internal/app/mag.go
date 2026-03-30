// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/micasa-dev/micasa/internal/locale"
)

const magArrow = "\U0001F821" // 🠡

// magFormat converts a numeric cell value to order-of-magnitude notation.
// When includeUnit is false the currency prefix is stripped (table cells get
// the unit from the column header instead). currencySymbol is the symbol to
// strip from input and optionally include in output.
func magFormat(c cell, includeUnit bool, currencySymbol string) string {
	value := strings.TrimSpace(c.Value)
	if value == "" || value == symEmDash || value == "0" {
		return value
	}

	// Only transform kinds that carry meaningful numeric data.
	// cellText is excluded because it covers serial numbers,
	// model numbers, and other identifiers that happen to look numeric.
	switch c.Kind {
	case cellMoney, cellDrilldown, cellOps:
		// Definitely numeric; continue to parsing below.
	case cellText, cellReadonly, cellDate, cellStatus, cellWarranty,
		cellUrgency, cellNotes, cellEntity, cellTelephoneNumber:
		return value
	default:
		panic(fmt.Sprintf("unhandled cellKind: %d", c.Kind))
	}

	sign := ""
	numStr := value

	// Strip negative sign, then currency symbol (prefix or suffix).
	if strings.HasPrefix(numStr, "-") {
		sign = "-"
		numStr = numStr[1:]
	}
	numStr = strings.TrimPrefix(numStr, currencySymbol)
	numStr = strings.TrimSuffix(numStr, currencySymbol)
	numStr = strings.TrimSpace(numStr)
	numStr = strings.Trim(numStr, "\u00a0")
	numStr = strings.ReplaceAll(numStr, ",", "")

	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return value
	}

	if f < 0 {
		sign = "-"
	}

	unit := ""
	if includeUnit && c.Kind == cellMoney {
		unit = currencySymbol + " "
	}

	if f == 0 {
		return fmt.Sprintf("%s%s%s-%s", sign, unit, magArrow, symInfinity)
	}
	mag := int(math.Round(math.Log10(math.Abs(f))))
	return fmt.Sprintf("%s%s%s%d", sign, unit, magArrow, mag)
}

// magCents converts a cent amount to magnitude notation with the currency
// symbol included (for use outside of table columns, e.g. dashboard).
func magCents(cents int64, cur locale.Currency) string {
	return magFormat(cell{Value: cur.FormatCents(cents), Kind: cellMoney}, true, cur.Symbol())
}

// magOptionalCents converts an optional cent amount to magnitude notation.
func magOptionalCents(cents *int64, cur locale.Currency) string {
	if cents == nil {
		return ""
	}
	return magCents(*cents, cur)
}

// magTransformText replaces currency amounts and bare numbers in free-form
// text with magnitude notation. Used to post-process LLM responses when
// mag mode is on. currencySymbol is used for stripping and re-prefixing.
func magTransformText(s string, currencySymbol string) string {
	re := magTextReForSymbol(currencySymbol)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		if strings.Contains(match, currencySymbol) {
			return magFormat(cell{Value: match, Kind: cellMoney}, true, currencySymbol)
		}
		return magFormat(cell{Value: match, Kind: cellDrilldown}, false, currencySymbol)
	})
}

// magTextReForSymbol builds a regex matching currency amounts (with the given
// symbol as prefix or suffix) and standalone bare numbers.
func magTextReForSymbol(symbol string) *regexp.Regexp {
	escaped := regexp.QuoteMeta(symbol)
	// Match: -?<symbol><digits> or <digits><symbol>, then bare numbers.
	pattern := fmt.Sprintf(
		`-?%s[\d,]+(?:\.\d+)?|[\d,]+(?:\.\d+)?%s|\b\d[\d,]*(?:\.\d+)?\b`,
		escaped, escaped,
	)
	return regexp.MustCompile(pattern)
}

// magTransformCells returns a copy of the cell grid with numeric values
// replaced by their order-of-magnitude representation. Currency symbols are
// stripped because the column header carries the unit annotation instead.
func magTransformCells(rows [][]cell, currencySymbol string) [][]cell {
	return transformCells(rows, func(c cell) cell {
		c.Value = magFormat(c, false, currencySymbol)
		return c
	})
}
