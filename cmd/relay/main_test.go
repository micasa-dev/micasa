// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"testing"

	"github.com/cpcloud/micasa/internal/relay"
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
