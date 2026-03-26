// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/relay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRelayMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		selfHosted     string
		webhookSecret  string
		wantSelfHosted bool
		wantErr        bool
	}{
		{
			name:           "cloud mode, no webhook",
			wantSelfHosted: false,
		},
		{
			name:           "cloud mode, webhook set",
			webhookSecret:  "whsec_test",
			wantSelfHosted: false,
		},
		{
			name:           "self-hosted mode",
			selfHosted:     "true",
			wantSelfHosted: true,
		},
		{
			name:           "self-hosted TRUE (uppercase)",
			selfHosted:     "TRUE",
			wantSelfHosted: true,
		},
		{
			name:           "self-hosted 1",
			selfHosted:     "1",
			wantSelfHosted: true,
		},
		{
			name:           "explicit false",
			selfHosted:     "false",
			wantSelfHosted: false,
		},
		{
			name:       "invalid value",
			selfHosted: "maybe",
			wantErr:    true,
		},
		{
			name:          "conflict: both set",
			selfHosted:    "true",
			webhookSecret: "whsec_test",
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			selfHosted, err := resolveRelayMode(tt.selfHosted, tt.webhookSecret)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSelfHosted, selfHosted)
		})
	}
}

func TestParseEncryptionKey(t *testing.T) {
	t.Parallel()

	t.Run("valid 64 hex chars", func(t *testing.T) {
		t.Parallel()
		h := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		key, err := parseEncryptionKey(h)
		require.NoError(t, err)
		assert.Len(t, key, 32)
	})

	t.Run("empty returns error", func(t *testing.T) {
		t.Parallel()
		_, err := parseEncryptionKey("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("wrong length", func(t *testing.T) {
		t.Parallel()
		_, err := parseEncryptionKey("deadbeef")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "64")
	})

	t.Run("invalid hex", func(t *testing.T) {
		t.Parallel()
		_, err := parseEncryptionKey(
			"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
		)
		assert.Error(t, err)
	})
}

func TestParseBlobQuota(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		envVal     string
		selfHosted bool
		want       int64
		wantErr    bool
	}{
		{
			name:       "unset, cloud mode",
			selfHosted: false,
			want:       relay.DefaultBlobQuota,
		},
		{
			name:       "unset, self-hosted",
			selfHosted: true,
			want:       0,
		},
		{
			name:       "explicit value",
			envVal:     "5368709120",
			selfHosted: false,
			want:       5368709120,
		},
		{
			name:       "explicit zero",
			envVal:     "0",
			selfHosted: false,
			want:       0,
		},
		{
			name:    "negative",
			envVal:  "-1",
			wantErr: true,
		},
		{
			name:    "non-integer",
			envVal:  "abc",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseBlobQuota(tt.envVal, tt.selfHosted)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
