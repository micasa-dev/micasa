// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() Config {
	var cfg Config
	data.ApplyDefaults(&cfg)
	return cfg
}

func TestQuery_Identity(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	require.NoError(t, cfg.Query(context.Background(), &buf, "."))
	assert.Contains(t, buf.String(), "[chat.llm]")
	assert.Contains(t, buf.String(), "model =")
}

func TestQuery_IdentityEmpty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	require.NoError(t, cfg.Query(context.Background(), &buf, ""))
	assert.Contains(t, buf.String(), "[chat.llm]")
}

func TestQuery_IdentityWhitespace(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	require.NoError(t, cfg.Query(context.Background(), &buf, "  .  "))
	assert.Contains(t, buf.String(), "[chat.llm]")
}

func TestQuery_Scalar(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	require.NoError(t, cfg.Query(context.Background(), &buf, ".chat.llm.model"))
	got := strings.TrimSpace(buf.String())
	assert.NotEmpty(t, got)
	assert.NotContains(t, got, `"`)
}

func TestQuery_NullKey(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	require.NoError(t, cfg.Query(context.Background(), &buf, ".nonexistent"))
	assert.Equal(t, "null\n", buf.String())
}

func TestQuery_Keys(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	require.NoError(t, cfg.Query(context.Background(), &buf, ".chat.llm | keys"))
	assert.Contains(t, buf.String(), `"model"`)
}

func TestQuery_InvalidFilter(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	err := cfg.Query(context.Background(), &buf, ".[invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse filter")
}

func TestQuery_OverlongFilter(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	long := strings.Repeat("x", maxFilterLen+1)
	err := cfg.Query(context.Background(), &buf, long)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "filter too long")
}

func TestQuery_HaltZero(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	require.NoError(t, cfg.Query(context.Background(), &buf, "halt"))
}

func TestQuery_HaltError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	err := cfg.Query(context.Background(), &buf, `halt_error(1)`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "halted with exit status 1")
}

func TestQuery_APIKeyStripped(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	cfg.Chat.LLM.APIKey = "secret-key"
	require.NoError(t, cfg.Query(context.Background(), &buf, ".chat.llm.api_key"))
	assert.Equal(t, "null\n", buf.String())
}

func TestQuery_Section(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	cfg := testConfig()
	require.NoError(t, cfg.Query(context.Background(), &buf, ".chat.llm"))
	s := buf.String()
	assert.Contains(t, s, "model =")
	assert.NotContains(t, s, "api_key")
}
