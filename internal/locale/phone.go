// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package locale

import (
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// FormatPhoneNumber formats a phone number string for display.
// regionCode is an uppercase ISO 3166-1 alpha-2 code (e.g. "US").
// Returns the original string unmodified if parsing fails.
func FormatPhoneNumber(number, regionCode string) string {
	trimmed := strings.TrimSpace(number)
	if trimmed == "" {
		return ""
	}
	parsed, err := phonenumbers.Parse(trimmed, regionCode)
	if err != nil {
		return number
	}
	// Prefer region comparison (distinguishes US/CA under shared +1 code).
	// Fall back to country-code comparison for fictional numbers (e.g. 555-xxxx)
	// that parse successfully but return an empty region.
	parsedRegion := phonenumbers.GetRegionCodeForNumber(parsed)
	if parsedRegion != "" {
		if parsedRegion == regionCode {
			return phonenumbers.Format(parsed, phonenumbers.NATIONAL)
		}
		return phonenumbers.Format(parsed, phonenumbers.INTERNATIONAL)
	}
	if int(parsed.GetCountryCode()) == phonenumbers.GetCountryCodeForRegion(regionCode) {
		return phonenumbers.Format(parsed, phonenumbers.NATIONAL)
	}
	return phonenumbers.Format(parsed, phonenumbers.INTERNATIONAL)
}
