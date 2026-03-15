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

	assert.Contains(t, out, "[chat]")
	assert.Contains(t, out, "[chat.llm]")
	assert.Contains(t, out, "[extraction]")
	assert.Contains(t, out, "[extraction.llm]")
	assert.Contains(t, out, "[extraction.ocr]")
	assert.Contains(t, out, "[extraction.ocr.tsv]")
	assert.Contains(t, out, "[documents]")
	assert.Contains(t, out, "[locale]")

	assert.Contains(t, out, `model = "qwen3:0.6b"`)
	assert.Contains(t, out, `base_url = "http://localhost:11434"`)
	assert.Contains(t, out, `timeout = "5m"`)
	assert.Contains(t, out, `max_file_size = "50 MiB"`)
	assert.Contains(t, out, `cache_ttl = "30d"`)
	assert.Contains(t, out, "max_pages = 0")
	assert.Contains(t, out, "enable = true")
}

func TestShowConfigOutputIsValidTOML(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
provider = "anthropic"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-test"
timeout = "10s"
thinking = "medium"

[extraction.llm]
model = "qwen2.5:7b"

[documents]
max_file_size = "100 MiB"
cache_ttl = "7d"

[extraction]
max_pages = 10
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	var parsed map[string]any
	_, err = toml.Decode(out, &parsed)
	assert.NoError(t, err, "output must be valid TOML:\n%s", out)
}

func TestShowConfigRoundTrip(t *testing.T) {
	path := writeConfig(t, `[chat]
enable = true

[chat.llm]
provider = "anthropic"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-test"
timeout = "10s"
thinking = "medium"
extra_context = "My house is old."

[extraction]
max_pages = 10

[extraction.llm]
enable = false
provider = "ollama"
model = "qwen2.5:7b"

[documents]
max_file_size = "100 MiB"
cache_ttl = "7d"
`)
	orig, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, orig)

	tmpPath := writeConfig(t, out)
	parsed, err := LoadFromPath(tmpPath)
	require.NoError(t, err)

	// Non-hidden fields must survive the roundtrip.
	assert.Equal(t, orig.Chat.LLM.Provider, parsed.Chat.LLM.Provider)
	assert.Equal(t, orig.Chat.LLM.Model, parsed.Chat.LLM.Model)
	assert.Equal(t, orig.Chat.LLM.BaseURL, parsed.Chat.LLM.BaseURL)
	assert.Equal(t, orig.Chat.LLM.Timeout, parsed.Chat.LLM.Timeout)
	assert.Equal(t, orig.Chat.LLM.Thinking, parsed.Chat.LLM.Thinking)
	assert.Equal(t, orig.Chat.LLM.ExtraContext, parsed.Chat.LLM.ExtraContext)
	assert.Equal(t, orig.Extraction.LLM.Provider, parsed.Extraction.LLM.Provider)
	assert.Equal(t, orig.Extraction.LLM.Model, parsed.Extraction.LLM.Model)
	assert.Equal(t, orig.Extraction.LLM.IsEnabled(), parsed.Extraction.LLM.IsEnabled())
	assert.Equal(t, orig.Documents.MaxFileSize, parsed.Documents.MaxFileSize)
	assert.Equal(t,
		orig.Documents.CacheTTLDuration(),
		parsed.Documents.CacheTTLDuration())
	assert.Equal(t, orig.Extraction.MaxPages, parsed.Extraction.MaxPages)

	// API keys are hidden -- the parsed config must NOT have them.
	assert.Empty(t, parsed.Chat.LLM.APIKey)
	assert.Empty(t, parsed.Extraction.LLM.APIKey)
}

func TestShowConfigEnvOverride(t *testing.T) {
	t.Setenv("MICASA_DOCUMENTS_MAX_FILE_SIZE", "100 MiB")
	t.Setenv("MICASA_CHAT_LLM_MODEL", "llama3")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Regexp(
		t,
		`max_file_size = "100 MiB"\s+# src\(env\): MICASA_DOCUMENTS_MAX_FILE_SIZE`,
		out,
	)
	assert.Regexp(t, `model = "llama3"\s+# src\(env\): MICASA_CHAT_LLM_MODEL`, out)
}

func TestShowConfigCurrencyEnv(t *testing.T) {
	t.Setenv("MICASA_LOCALE_CURRENCY", "EUR")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Regexp(t, `currency = "EUR"\s+# src\(env\): MICASA_LOCALE_CURRENCY`, out)
}

func TestShowConfigFromFile(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
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
	path := writeConfig(t, `[chat.llm]
thinking = "high"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Contains(t, out, `thinking = "high"`)
}

func TestShowConfigShowsConfidenceThreshold(t *testing.T) {
	path := writeConfig(t, `[extraction.ocr.tsv]
confidence_threshold = 50
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.Contains(t, out, "confidence_threshold = 50")
}

func TestShowConfigOmitsDefaultConfidenceThreshold(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.NotContains(t, out, "confidence_threshold",
		"omitempty confidence_threshold should be omitted when not set")
}

func TestShowConfigOmitsAPIKeys(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
api_key = "sk-ant-secret-key"

[extraction.llm]
api_key = "sk-ext-secret"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	out := showConfig(t, cfg)

	assert.NotContains(t, out, "sk-ant-secret-key")
	assert.NotContains(t, out, "sk-ext-secret")
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
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "90m"},
		{2 * time.Hour, "2h"},
		{30 * 24 * time.Hour, "30d"},
		{7 * 24 * time.Hour, "7d"},
		{90*time.Minute + 30*time.Second, "1h30m30s"},
		{500 * time.Millisecond, "500ms"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatDuration(tt.d))
		})
	}
}
