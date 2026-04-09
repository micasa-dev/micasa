// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/locale"
)

// formatKSqft formats a square footage value with k-suffix for values >= 1000.
// Returns "" for zero, raw number for < 1000, and k-suffix for >= 1000.
// Examples: 0 → "", 850 → "850", 1200 → "1.2k", 2400 → "2.4k", 5000 → "5k".
func formatKSqft(sqft int) string {
	if sqft == 0 {
		return ""
	}
	if sqft < 1000 {
		return strconv.Itoa(sqft)
	}
	rounded := math.Round(float64(sqft)/100) / 10
	if rounded == math.Trunc(rounded) {
		return fmt.Sprintf("%.0fk", rounded)
	}
	return fmt.Sprintf("%.1fk", rounded)
}

// houseEmptyFieldCount returns how many house profile fields have empty display values.
func houseEmptyFieldCount(p data.HouseProfile, cur locale.Currency, us data.UnitSystem) int {
	count := 0
	for _, d := range houseFieldDefs() {
		if d.toggle != nil {
			continue // toggle fields always have a value
		}
		if strings.TrimSpace(d.get(p, cur, us)) == "" {
			count++
		}
	}
	return count
}

// emptyHouseProfile returns a HouseProfile with only the nickname set.
func emptyHouseProfile(nickname string) data.HouseProfile {
	return data.HouseProfile{Nickname: nickname}
}
