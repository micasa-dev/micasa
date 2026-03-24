// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"os"
	"strings"
)

// DetectCountry returns the ISO 3166-1 alpha-2 country code derived from
// the system locale. Respects POSIX precedence: LC_ALL overrides LANG.
// Falls back to "us" when the locale cannot be parsed.
func DetectCountry() string {
	// LC_ALL overrides LANG per POSIX. If set, use it unconditionally.
	if val := os.Getenv("LC_ALL"); val != "" {
		return detectCountryFromLang(val)
	}
	if val := os.Getenv("LANG"); val != "" {
		return detectCountryFromLang(val)
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
