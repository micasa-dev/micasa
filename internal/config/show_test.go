// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func showConfig(t *testing.T, cfg Config) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, cfg.ShowConfig(&buf))
	return buf.String()
}

func TestShowConfigDefaults(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Contains(t, out, "[llm]")
	assert.Contains(t, out, "[documents]")
	assert.Contains(t, out, "[extraction]")
	assert.Contains(t, out, "[locale]")

	assert.Contains(t, out, `model = "qwen3"`)
	assert.Contains(t, out, `base_url = "http://localhost:11434"`)
	assert.Contains(t, out, `timeout = "5m0s"`)
	assert.Contains(t, out, `max_file_size = "50 MiB"`)
	assert.Contains(t, out, `cache_ttl = "30d"`)
	assert.Contains(t, out, "max_extract_pages = 0")
	assert.Contains(t, out, "enabled = true")
	assert.Contains(t, out, `text_timeout = "30s"`)

	assert.NotContains(t, out, "cache_ttl_days")
	assert.NotContains(t, out, "[llm.chat]")
	assert.NotContains(t, out, "[llm.extraction]")
}

func TestShowConfigOutputIsValidTOML(t *testing.T) {
	path := writeConfig(t, `[llm]
provider = "anthropic"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-test"
timeout = "10s"
thinking = "medium"

[llm.chat]
model = "gpt-4o"

[documents]
max_file_size = "100 MiB"
cache_ttl = "7d"

[extraction]
max_extract_pages = 10
enabled = false
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	var parsed map[string]any
	_, err = toml.Decode(out, &parsed)
	assert.NoError(t, err, "output must be valid TOML:\n%s", out)
}

func TestShowConfigRoundTrip(t *testing.T) {
	path := writeConfig(t, `[llm]
provider = "anthropic"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-test"
timeout = "10s"
thinking = "medium"
extra_context = "My house is old."

[llm.chat]
provider = "openai"
model = "gpt-4o"
api_key = "sk-openai"

[documents]
max_file_size = "100 MiB"
cache_ttl = "7d"

[extraction]
max_extract_pages = 10
enabled = false
text_timeout = "1m"
`)
	orig, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, orig)

	// Re-parse the dump output as a new Config.
	tmpPath := writeConfig(t, out)
	parsed, err := LoadFromPath(tmpPath)
	require.NoError(t, err)

	// Non-hidden fields must survive the roundtrip.
	assert.Equal(t, orig.LLM.Provider, parsed.LLM.Provider)
	assert.Equal(t, orig.LLM.Model, parsed.LLM.Model)
	assert.Equal(t, orig.LLM.BaseURL, parsed.LLM.BaseURL)
	assert.Equal(t, orig.LLM.Timeout, parsed.LLM.Timeout)
	assert.Equal(t, orig.LLM.Thinking, parsed.LLM.Thinking)
	assert.Equal(t, orig.LLM.ExtraContext, parsed.LLM.ExtraContext)
	assert.Equal(t, orig.LLM.Chat.Provider, parsed.LLM.Chat.Provider)
	assert.Equal(t, orig.LLM.Chat.Model, parsed.LLM.Chat.Model)
	assert.Equal(t, orig.Documents.MaxFileSize, parsed.Documents.MaxFileSize)
	assert.Equal(t,
		orig.Documents.CacheTTLDuration(),
		parsed.Documents.CacheTTLDuration())
	assert.Equal(t, orig.Extraction.MaxExtractPages, parsed.Extraction.MaxExtractPages)
	assert.Equal(t, orig.Extraction.IsEnabled(), parsed.Extraction.IsEnabled())
	assert.Equal(t, orig.Extraction.TextTimeout, parsed.Extraction.TextTimeout)

	// API keys are hidden -- the parsed config must NOT have them.
	assert.Empty(t, parsed.LLM.APIKey)
	assert.Empty(t, parsed.LLM.Chat.APIKey)
}

func TestShowConfigEnvOverride(t *testing.T) {
	t.Setenv("MICASA_MAX_DOCUMENT_SIZE", "100 MiB")
	t.Setenv("MICASA_LLM_MODEL", "llama3")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Regexp(t, `max_file_size = "100 MiB"\s+# src\(env\): MICASA_MAX_DOCUMENT_SIZE`, out)
	assert.Regexp(t, `model = "llama3"\s+# src\(env\): MICASA_LLM_MODEL`, out)
}

func TestShowConfigDeprecatedCacheTTLDaysEnv(t *testing.T) {
	t.Setenv("MICASA_CACHE_TTL_DAYS", "14")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Regexp(t, `cache_ttl = "14d"\s+# src\(env\): MICASA_CACHE_TTL_DAYS`, out)
	assert.Regexp(t, `cache_ttl_days = 14\s+# DEPRECATED: use documents\.cache_ttl = "14d"`, out)
}

func TestShowConfigDeprecatedCacheTTLDaysFile(t *testing.T) {
	path := writeConfig(t, `[documents]
cache_ttl_days = 7
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Regexp(t, `cache_ttl_days = 7\s+# DEPRECATED: use documents\.cache_ttl = "7d"`, out)
	assert.Contains(t, out, `cache_ttl = "7d"`)
}

func TestShowConfigCurrencyEnv(t *testing.T) {
	t.Setenv("MICASA_CURRENCY", "EUR")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Regexp(t, `currency = "EUR"\s+# src\(env\): MICASA_CURRENCY`, out)
}

func TestShowConfigPipelineOverrides(t *testing.T) {
	path := writeConfig(t, `[llm]
model = "qwen3"

[llm.chat]
provider = "anthropic"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-test"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Contains(t, out, "[llm.chat]")
	assert.Contains(t, out, `provider = "anthropic"`)
	assert.Contains(t, out, `model = "claude-sonnet-4-5-20250929"`)
}

func TestShowConfigSkipsEmptyOverrideSections(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.NotContains(t, out, "[llm.chat]")
	assert.NotContains(t, out, "[llm.extraction]")
}

func TestShowConfigBothPipelineOverrides(t *testing.T) {
	path := writeConfig(t, `[llm]
model = "default"

[llm.chat]
provider = "openai"
model = "gpt-4o"
api_key = "sk-openai"

[llm.extraction]
provider = "anthropic"
model = "claude-haiku-3-5-20241022"
api_key = "sk-ant"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Contains(t, out, "[llm.chat]")
	assert.Contains(t, out, "[llm.extraction]")
}

func TestShowConfigDeprecatedExtractionFieldsWarned(t *testing.T) {
	path := writeConfig(t, `[extraction]
model = "old-model"
thinking = "low"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Regexp(
		t,
		`model = "old-model"\s+# DEPRECATED: use llm\.extraction\.model = "old-model"`,
		out,
	)
	assert.Regexp(t, `thinking = "low"\s+# DEPRECATED: use llm\.extraction\.thinking = "low"`, out)
	assert.Contains(t, out, "[llm.extraction]")
}

func TestShowConfigFromFile(t *testing.T) {
	path := writeConfig(t, `[llm]
model = "phi3"
extra_context = "My house is old."

[documents]
max_file_size = "10 MiB"
cache_ttl = "7d"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Contains(t, out, `model = "phi3"`)
	assert.Contains(t, out, `extra_context = "My house is old."`)
	assert.Contains(t, out, `max_file_size = "10 MiB"`)
	assert.Contains(t, out, `cache_ttl = "7d"`)
}

func TestShowConfigOmitsEmptyThinking(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.NotContains(t, out, "thinking =",
		"empty thinking fields (omitempty) should be omitted")
}

func TestShowConfigShowsNonEmptyThinking(t *testing.T) {
	path := writeConfig(t, `[llm]
thinking = "high"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Contains(t, out, `thinking = "high"`)
}

func TestShowConfigOmitsAPIKeys(t *testing.T) {
	path := writeConfig(t, `[llm]
api_key = "sk-ant-secret-key"

[llm.chat]
api_key = "sk-openai-secret"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.NotContains(t, out, "sk-ant-secret-key")
	assert.NotContains(t, out, "sk-openai-secret")
	assert.NotContains(t, out, "api_key")
}

func TestShowConfigOmitsEmptyAPIKey(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.NotContains(t, out, "api_key")
}

func TestFormatTOMLValue(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"string", "hello", `"hello"`},
		{"empty_string", "", `""`},
		{"string_with_quotes", `say "hi"`, `"say \"hi\""`},
		{"int", 42, "42"},
		{"zero_int", 0, "0"},
		{"negative_int", -1, "-1"},
		{"bool_true", true, "true"},
		{"bool_false", false, "false"},
		{"bytesize", ByteSize(50 * 1024 * 1024), `"50 MiB"`},
		{"duration", Duration{30 * 24 * time.Hour}, `"30d"`},
		{"duration_seconds", Duration{5 * time.Second}, `"5s"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := formatTOMLValue(reflect.ValueOf(tt.val))
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{30 * 24 * time.Hour, "30d"},
		{7 * 24 * time.Hour, "7d"},
		{90 * time.Minute, "1h30m0s"},
		{500 * time.Millisecond, "500ms"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatDuration(tt.d))
		})
	}
}
