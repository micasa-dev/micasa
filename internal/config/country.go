// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"os"
	"strings"
)

// DetectCountry returns the ISO 3166-1 alpha-2 country code derived from
// the system locale. Falls back to "us" when the locale cannot be parsed.
func DetectCountry() string {
	for _, env := range []string{"LC_ALL", "LANG"} {
		if val := os.Getenv(env); val != "" {
			if c := detectCountryFromLang(val); c != "us" || strings.Contains(val, "_US") {
				return c
			}
		}
	}
	return "us"
}

// detectCountryFromLang extracts a lowercase country code from a locale
// string like "en_US.UTF-8". Returns "us" if the locale is unparseable.
func detectCountryFromLang(lang string) string {
	if i := strings.IndexByte(lang, '.'); i >= 0 {
		lang = lang[:i]
	}
	if i := strings.IndexByte(lang, '_'); i >= 0 && i+1 < len(lang) {
		return strings.ToLower(lang[i+1:])
	}
	return "us"
}
