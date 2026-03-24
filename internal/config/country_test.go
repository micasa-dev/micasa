// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectCountryLCAllOverridesLang(t *testing.T) {
	// LC_ALL=C should produce "us" even if LANG=en_GB.UTF-8.
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "en_GB.UTF-8")
	assert.Equal(t, "us", DetectCountry())
}

func TestDetectCountryFallsBackToLang(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "en_GB.UTF-8")
	assert.Equal(t, "gb", DetectCountry())
}

func TestDetectCountryDefaultsToUS(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "")
	assert.Equal(t, "us", DetectCountry())
}

func TestDetectCountryFromLocale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		lang     string
		expected string
	}{
		{"US locale", "en_US.UTF-8", "us"},
		{"GB locale", "en_GB.UTF-8", "gb"},
		{"German locale", "de_DE.UTF-8", "de"},
		{"No underscore", "C", "us"},
		{"Empty", "", "us"},
		{"POSIX", "POSIX", "us"},
		{"Just language", "en", "us"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, detectCountryFromLang(tt.lang))
		})
	}
}
