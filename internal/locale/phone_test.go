// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package locale_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/micasa-dev/micasa/internal/locale"
)

func TestFormatPhoneNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		number string
		region string
		want   string
	}{
		{"US national", "5551234567", "US", "(555) 123-4567"},
		{"UK national", "02079460958", "GB", "020 7946 0958"},
		{"international prefix", "+442079460958", "US", "+44 20 7946 0958"},
		{"same-region prefix real", "+12025551234", "US", "(202) 555-1234"},
		{"shared-code fictional prefix", "+15551234567", "US", "+1 555-123-4567"},
		{"already formatted", "(555) 123-4567", "US", "(555) 123-4567"},
		{"cross-border shared code", "+16135551234", "US", "+1 613-555-1234"},
		{"garbage passthrough", "not a phone", "US", "not a phone"},
		{"empty string", "", "US", ""},
		{"whitespace only", "   ", "US", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := locale.FormatPhoneNumber(tt.number, tt.region)
			assert.Equal(t, tt.want, got)
		})
	}
}
