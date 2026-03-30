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
	parsedRegion := phonenumbers.GetRegionCodeForNumber(parsed)
	if parsedRegion != "" {
		if parsedRegion == regionCode {
			return phonenumbers.Format(parsed, phonenumbers.NATIONAL)
		}
		return phonenumbers.Format(parsed, phonenumbers.INTERNATIONAL)
	}
	// Fictional numbers (e.g. 555-xxxx) parse but have no region. When
	// the input lacks a "+" prefix, the user entered a local number — use
	// NATIONAL if the country code matches. When "+" is present, the user
	// explicitly typed an international code; for shared codes like +1
	// (US/CA) use INTERNATIONAL to avoid misattribution.
	cc := int(parsed.GetCountryCode())
	if cc != phonenumbers.GetCountryCodeForRegion(regionCode) {
		return phonenumbers.Format(parsed, phonenumbers.INTERNATIONAL)
	}
	if strings.HasPrefix(trimmed, "+") &&
		len(phonenumbers.GetRegionCodesForCountryCode(cc)) > 1 {
		return phonenumbers.Format(parsed, phonenumbers.INTERNATIONAL)
	}
	return phonenumbers.Format(parsed, phonenumbers.NATIONAL)
}
