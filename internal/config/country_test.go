// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
