// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func noConfig(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "nope.toml")
}

func TestDefaultsApplied(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, DefaultBaseURL, cfg.LLM.BaseURL)
	assert.Equal(t, DefaultModel, cfg.LLM.Model)
}

func TestLoadFromFile(t *testing.T) {
	path := writeConfig(t, `[llm]
base_url = "http://myhost:8080"
model = "llama3"
extra_context = "My house is old."
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "http://myhost:8080", cfg.LLM.BaseURL)
	assert.Equal(t, "llama3", cfg.LLM.Model)
	assert.Equal(t, "My house is old.", cfg.LLM.ExtraContext)
}

func TestPartialConfigUsesDefaults(t *testing.T) {
	path := writeConfig(t, `[llm]
model = "phi3"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, DefaultBaseURL, cfg.LLM.BaseURL)
	assert.Equal(t, "phi3", cfg.LLM.Model)
}

func TestEnvOverridesConfig(t *testing.T) {
	path := writeConfig(t, `[llm]
base_url = "http://file-host:1234"
model = "from-file"
`)
	t.Setenv("MICASA_LLM_BASE_URL", "http://env-host:5678")
	t.Setenv("MICASA_LLM_MODEL", "from-env")

	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "http://env-host:5678", cfg.LLM.BaseURL)
	assert.Equal(t, "from-env", cfg.LLM.Model)
}

func TestBaseURLV1SuffixStripped(t *testing.T) {
	path := writeConfig(t, `[llm]
base_url = "http://myhost:11434/v1"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "http://myhost:11434", cfg.LLM.BaseURL,
		"/v1 suffix should be stripped -- providers handle path construction")
}

func TestTrailingSlashStripped(t *testing.T) {
	path := writeConfig(t, `[llm]
base_url = "http://localhost:11434/"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:11434", cfg.LLM.BaseURL)
}

func TestProviderAutoDetection(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		apiKey   string
		expected string
	}{
		{"no key defaults to ollama", "", "", DefaultProvider},
		{"anthropic URL", "https://api.anthropic.com", "sk-ant-key", "anthropic"},
		{"openai URL", "https://api.openai.com", "sk-key", "openai"},
		{"openrouter URL", "https://openrouter.ai", "sk-key", "openrouter"},
		{"deepseek URL", "https://api.deepseek.com", "sk-key", "deepseek"},
		{"gemini googleapis URL", "https://generativelanguage.googleapis.com", "key", "gemini"},
		{"groq URL", "https://api.groq.com", "gsk-key", "groq"},
		{"mistral URL", "https://api.mistral.ai", "key", "mistral"},
		{"unknown with key defaults to openai", "https://custom.api.com", "key", "openai"},
		{"empty URL with key defaults to openai", "", "sk-key", "openai"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, detectProvider(tt.baseURL, tt.apiKey))
		})
	}
}

func TestProviderExplicitConfig(t *testing.T) {
	path := writeConfig(t, `[llm]
provider = "anthropic"
api_key = "sk-ant-test"
model = "claude-sonnet-4-5-20250929"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.LLM.Provider)
}

func TestProviderEnvOverride(t *testing.T) {
	t.Setenv("MICASA_LLM_PROVIDER", "openai")
	t.Setenv("MICASA_LLM_API_KEY", "sk-test")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.LLM.Provider)
}

func TestProviderInvalidReturnsError(t *testing.T) {
	path := writeConfig(t, `[llm]
provider = "bogus"
`)
	_, err := LoadFromPath(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, err.Error(), "supported")
}

func TestExampleTOML(t *testing.T) {
	example := ExampleTOML()
	assert.Contains(t, example, "[llm]")
	assert.Contains(t, example, "base_url")
	assert.Contains(t, example, "model")
	assert.Contains(t, example, "timeout")
	assert.Contains(t, example, "[documents]")
	assert.Contains(t, example, "max_file_size")
	assert.Contains(t, example, "cache_ttl")
	assert.Contains(t, example, "[extraction]")
	assert.Contains(t, example, "max_extract_pages")
}

func TestMalformedConfigReturnsError(t *testing.T) {
	path := writeConfig(t, "{{not toml")

	_, err := LoadFromPath(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

// --- MaxFileSize ---

func TestDefaultMaxDocumentSize(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, data.MaxDocumentSize, cfg.Documents.MaxFileSize.Bytes())
}

func TestMaxDocumentSizeFromFileInteger(t *testing.T) {
	path := writeConfig(t, "[documents]\nmax_file_size = 1048576\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, uint64(1048576), cfg.Documents.MaxFileSize.Bytes())
}

func TestMaxDocumentSizeFromFileString(t *testing.T) {
	path := writeConfig(t, "[documents]\nmax_file_size = \"10 MiB\"\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, uint64(10<<20), cfg.Documents.MaxFileSize.Bytes())
}

func TestMaxDocumentSizeFromFileFractional(t *testing.T) {
	path := writeConfig(t, "[documents]\nmax_file_size = \"1.5 GiB\"\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, uint64(1.5*(1<<30)), cfg.Documents.MaxFileSize.Bytes())
}

func TestMaxDocumentSizeEnvOverrideInteger(t *testing.T) {
	t.Setenv("MICASA_MAX_DOCUMENT_SIZE", "2097152")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, uint64(2097152), cfg.Documents.MaxFileSize.Bytes())
}

func TestMaxDocumentSizeEnvOverrideUnitized(t *testing.T) {
	t.Setenv("MICASA_MAX_DOCUMENT_SIZE", "100 MiB")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, uint64(100<<20), cfg.Documents.MaxFileSize.Bytes())
}

func TestMaxDocumentSizeRejectsZero(t *testing.T) {
	path := writeConfig(t, "[documents]\nmax_file_size = 0\n")
	_, err := LoadFromPath(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")
}

func TestMaxDocumentSizeRejectsNegative(t *testing.T) {
	path := writeConfig(t, "[documents]\nmax_file_size = -1\n")
	_, err := LoadFromPath(path)
	require.Error(t, err)
}

// --- CacheTTL ---

func TestDefaultCacheTTL(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, DefaultCacheTTL, cfg.Documents.CacheTTLDuration())
}

func TestCacheTTLFromFileString(t *testing.T) {
	path := writeConfig(t, "[documents]\ncache_ttl = \"7d\"\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, cfg.Documents.CacheTTLDuration())
}

func TestCacheTTLFromFileGoDuration(t *testing.T) {
	path := writeConfig(t, "[documents]\ncache_ttl = \"168h\"\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, 168*time.Hour, cfg.Documents.CacheTTLDuration())
}

func TestCacheTTLFromFileInteger(t *testing.T) {
	path := writeConfig(t, "[documents]\ncache_ttl = 3600\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, time.Hour, cfg.Documents.CacheTTLDuration())
}

func TestCacheTTLZeroDisables(t *testing.T) {
	path := writeConfig(t, "[documents]\ncache_ttl = \"0s\"\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), cfg.Documents.CacheTTLDuration())
}

func TestCacheTTLEnvOverride(t *testing.T) {
	t.Setenv("MICASA_CACHE_TTL", "14d")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, 14*24*time.Hour, cfg.Documents.CacheTTLDuration())
}

func TestCacheTTLEnvOverrideSeconds(t *testing.T) {
	t.Setenv("MICASA_CACHE_TTL", "86400")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, cfg.Documents.CacheTTLDuration())
}

func TestCacheTTLRejectsNegative(t *testing.T) {
	path := writeConfig(t, "[documents]\ncache_ttl = \"-1s\"\n")
	_, err := LoadFromPath(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}

// --- CacheTTLDays (deprecated) ---

func TestCacheTTLDaysStillWorks(t *testing.T) {
	path := writeConfig(t, "[documents]\ncache_ttl_days = 7\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, cfg.Documents.CacheTTLDuration())
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0], "documents.cache_ttl_days")
}

func TestCacheTTLDaysZeroDisables(t *testing.T) {
	path := writeConfig(t, "[documents]\ncache_ttl_days = 0\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), cfg.Documents.CacheTTLDuration())
}

func TestCacheTTLDaysEnvOverride(t *testing.T) {
	t.Setenv("MICASA_CACHE_TTL_DAYS", "14")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, 14*24*time.Hour, cfg.Documents.CacheTTLDuration())
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0], "MICASA_CACHE_TTL_DAYS")
}

func TestCacheTTLDaysRejectsNegative(t *testing.T) {
	path := writeConfig(t, "[documents]\ncache_ttl_days = -1\n")
	_, err := LoadFromPath(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}

func TestCacheTTLAndCacheTTLDaysBothSetFails(t *testing.T) {
	path := writeConfig(t, "[documents]\ncache_ttl = \"30d\"\ncache_ttl_days = 30\n")
	_, err := LoadFromPath(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot both be set")
}

func TestCacheTTLAndCacheTTLDaysEnvBothSetFails(t *testing.T) {
	t.Setenv("MICASA_CACHE_TTL", "30d")
	t.Setenv("MICASA_CACHE_TTL_DAYS", "30")
	_, err := LoadFromPath(noConfig(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot both be set")
}

func TestAPIKeyFromFile(t *testing.T) {
	path := writeConfig(t, `[llm]
api_key = "sk-ant-test-key"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-test-key", cfg.LLM.APIKey)
}

func TestAPIKeyDefaultEmpty(t *testing.T) {
	cfg, err := LoadFromPath(filepath.Join(t.TempDir(), "nope.toml"))
	require.NoError(t, err)
	assert.Empty(t, cfg.LLM.APIKey)
}

func TestAPIKeyEnvOverride(t *testing.T) {
	path := writeConfig(t, `[llm]
api_key = "from-file"
`)
	t.Setenv("MICASA_LLM_API_KEY", "from-env")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "from-env", cfg.LLM.APIKey)
}

func TestCloudProviderConfig(t *testing.T) {
	path := writeConfig(t, `[llm]
base_url = "https://api.anthropic.com/v1"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-api03-secret"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	// /v1 suffix is stripped -- providers handle path construction.
	assert.Equal(t, "https://api.anthropic.com", cfg.LLM.BaseURL)
	assert.Equal(t, "claude-sonnet-4-5-20250929", cfg.LLM.Model)
	assert.Equal(t, "sk-ant-api03-secret", cfg.LLM.APIKey)
}

// --- LLM Timeout ---

func TestLLMTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg, err := LoadFromPath(noConfig(t))
		require.NoError(t, err)
		assert.Equal(t, DefaultLLMTimeout, cfg.LLM.TimeoutDuration())
	})

	t.Run("from file", func(t *testing.T) {
		path := writeConfig(t, "[llm]\ntimeout = \"10s\"\n")
		cfg, err := LoadFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, 10*time.Second, cfg.LLM.TimeoutDuration())
	})

	t.Run("sub-second", func(t *testing.T) {
		path := writeConfig(t, "[llm]\ntimeout = \"500ms\"\n")
		cfg, err := LoadFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, 500*time.Millisecond, cfg.LLM.TimeoutDuration())
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("MICASA_LLM_TIMEOUT", "15s")
		cfg, err := LoadFromPath(noConfig(t))
		require.NoError(t, err)
		assert.Equal(t, 15*time.Second, cfg.LLM.TimeoutDuration())
	})

	t.Run("rejects invalid", func(t *testing.T) {
		path := writeConfig(t, "[llm]\ntimeout = \"not-a-duration\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid duration")
	})

	t.Run("rejects negative", func(t *testing.T) {
		path := writeConfig(t, "[llm]\ntimeout = \"-1s\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	})
}

// --- Extraction ---

func TestExtractionLLMTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg, err := LoadFromPath(noConfig(t))
		require.NoError(t, err)
		assert.Equal(t, DefaultLLMExtractionTimeout, cfg.Extraction.LLMTimeoutDuration())
	})

	t.Run("from file", func(t *testing.T) {
		path := writeConfig(t, "[extraction]\nllm_timeout = \"90s\"\n")
		cfg, err := LoadFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, 90*time.Second, cfg.Extraction.LLMTimeoutDuration())
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("MICASA_EXTRACTION_LLM_TIMEOUT", "3m")
		cfg, err := LoadFromPath(noConfig(t))
		require.NoError(t, err)
		assert.Equal(t, 3*time.Minute, cfg.Extraction.LLMTimeoutDuration())
	})

	t.Run("rejects invalid", func(t *testing.T) {
		path := writeConfig(t, "[extraction]\nllm_timeout = \"not-a-duration\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid duration")
	})

	t.Run("rejects negative", func(t *testing.T) {
		path := writeConfig(t, "[extraction]\nllm_timeout = \"-1s\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	})
}

func TestExtractionDefaults(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxExtractPages, cfg.Extraction.MaxExtractPages)
	assert.True(t, cfg.Extraction.IsEnabled())
	assert.Empty(t, cfg.Extraction.Model)
}

func TestExtractionFromFile(t *testing.T) {
	path := writeConfig(t, `[extraction]
model = "qwen2.5:7b"
max_extract_pages = 10
enabled = false
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "qwen2.5:7b", cfg.Extraction.Model)
	assert.Equal(t, 10, cfg.Extraction.MaxExtractPages)
	assert.False(t, cfg.Extraction.IsEnabled())
}

func TestExtractionResolvedModel(t *testing.T) {
	t.Run("uses extraction model", func(t *testing.T) {
		e := Extraction{Model: "qwen2.5:7b"}
		assert.Equal(t, "qwen2.5:7b", e.ResolvedModel("qwen3"))
	})
	t.Run("falls back to chat model", func(t *testing.T) {
		e := Extraction{}
		assert.Equal(t, "qwen3", e.ResolvedModel("qwen3"))
	})
}

func TestExtractionEnvOverrides(t *testing.T) {
	t.Setenv("MICASA_EXTRACTION_MODEL", "phi3")
	t.Setenv("MICASA_MAX_EXTRACT_PAGES", "5")
	t.Setenv("MICASA_EXTRACTION_ENABLED", "false")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "phi3", cfg.Extraction.Model)
	assert.Equal(t, 5, cfg.Extraction.MaxExtractPages)
	assert.False(t, cfg.Extraction.IsEnabled())
}

func TestExtractionRejectsNegativePages(t *testing.T) {
	path := writeConfig(t, "[extraction]\nmax_extract_pages = -1\n")
	_, err := LoadFromPath(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}

// --- Invalid env values ---

func TestInvalidEnvVarReturnsError(t *testing.T) {
	tests := []struct {
		envVar  string
		value   string
		wantMsg string
	}{
		{"MICASA_MAX_EXTRACT_PAGES", "not-a-number", "expected integer"},
		{"MICASA_EXTRACTION_ENABLED", "maybe", "expected true or false"},
		{"MICASA_MAX_DOCUMENT_SIZE", "lots", "expected byte size"},
		{"MICASA_CACHE_TTL", "forever", "expected duration"},
		{"MICASA_CACHE_TTL_DAYS", "many", "expected integer"},
	}
	for _, tt := range tests {
		t.Run(tt.envVar, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.value)
			_, err := LoadFromPath(noConfig(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.envVar+"=")
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestInvalidThinkingLevelReturnsError(t *testing.T) {
	t.Run("llm.thinking", func(t *testing.T) {
		t.Setenv("MICASA_LLM_THINKING", "dunno")
		_, err := LoadFromPath(noConfig(t))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid level")
		assert.Contains(t, err.Error(), "dunno")
	})
	t.Run("extraction.thinking", func(t *testing.T) {
		t.Setenv("MICASA_EXTRACTION_THINKING", "yolo")
		_, err := LoadFromPath(noConfig(t))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid level")
		assert.Contains(t, err.Error(), "yolo")
	})
	t.Run("valid levels", func(t *testing.T) {
		for _, level := range []string{"none", "low", "medium", "high", "auto"} {
			t.Setenv("MICASA_LLM_THINKING", level)
			cfg, err := LoadFromPath(noConfig(t))
			require.NoError(t, err, "level %q should be valid", level)
			assert.Equal(t, level, cfg.LLM.Thinking)
		}
	})
}

// --- Config Get ---

func TestConfigGet(t *testing.T) {
	cfg := Config{
		LLM: LLM{
			BaseURL:      "http://localhost:11434/v1",
			Model:        "qwen3",
			ExtraContext: "my house",
			Timeout:      "10s",
		},
		Documents: Documents{
			MaxFileSize: ByteSize(1024),
		},
	}

	tests := []struct {
		key  string
		want string
	}{
		{"llm.base_url", "http://localhost:11434/v1"},
		{"llm.model", "qwen3"},
		{"llm.extra_context", "my house"},
		{"llm.timeout", "10s"},
		{"documents.max_file_size", "1024"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := cfg.Get(tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("unknown key", func(t *testing.T) {
		_, err := cfg.Get("no.such.key")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown config key")
	})
}

func TestKeys(t *testing.T) {
	keys := Keys()
	assert.NotEmpty(t, keys)
	assert.Contains(t, keys, "llm.model")
	assert.Contains(t, keys, "llm.base_url")
	assert.Contains(t, keys, "documents.max_file_size")
	assert.Contains(t, keys, "extraction.max_extract_pages")
	// Verify every key is resolvable against defaults.
	cfg := defaults()
	for _, k := range keys {
		_, err := cfg.Get(k)
		assert.NoError(t, err, "key %q should be resolvable", k)
	}
}

// --- EnvVars ---

func TestEnvVars(t *testing.T) {
	m := EnvVars()
	assert.NotEmpty(t, m)

	want := map[string]string{
		"MICASA_LLM_PROVIDER":           "llm.provider",
		"MICASA_LLM_BASE_URL":           "llm.base_url",
		"MICASA_LLM_API_KEY":            "llm.api_key",
		"MICASA_LLM_MODEL":              "llm.model",
		"MICASA_LLM_EXTRA_CONTEXT":      "llm.extra_context",
		"MICASA_LLM_TIMEOUT":            "llm.timeout",
		"MICASA_LLM_THINKING":           "llm.thinking",
		"MICASA_MAX_DOCUMENT_SIZE":      "documents.max_file_size",
		"MICASA_CACHE_TTL":              "documents.cache_ttl",
		"MICASA_CACHE_TTL_DAYS":         "documents.cache_ttl_days",
		"MICASA_EXTRACTION_MODEL":       "extraction.model",
		"MICASA_MAX_EXTRACT_PAGES":      "extraction.max_extract_pages",
		"MICASA_EXTRACTION_ENABLED":     "extraction.enabled",
		"MICASA_TEXT_TIMEOUT":           "extraction.text_timeout",
		"MICASA_EXTRACTION_LLM_TIMEOUT": "extraction.llm_timeout",
		"MICASA_EXTRACTION_THINKING":    "extraction.thinking",
		"MICASA_CURRENCY":               "locale.currency",

		// Per-pipeline chat overrides.
		"MICASA_LLM_CHAT_PROVIDER": "llm.chat.provider",
		"MICASA_LLM_CHAT_BASE_URL": "llm.chat.base_url",
		"MICASA_LLM_CHAT_MODEL":    "llm.chat.model",
		"MICASA_LLM_CHAT_API_KEY":  "llm.chat.api_key",
		"MICASA_LLM_CHAT_TIMEOUT":  "llm.chat.timeout",
		"MICASA_LLM_CHAT_THINKING": "llm.chat.thinking",

		// Per-pipeline extraction overrides.
		"MICASA_LLM_EXTRACTION_PROVIDER": "llm.extraction.provider",
		"MICASA_LLM_EXTRACTION_BASE_URL": "llm.extraction.base_url",
		"MICASA_LLM_EXTRACTION_MODEL":    "llm.extraction.model",
		"MICASA_LLM_EXTRACTION_API_KEY":  "llm.extraction.api_key",
		"MICASA_LLM_EXTRACTION_TIMEOUT":  "llm.extraction.timeout",
		"MICASA_LLM_EXTRACTION_THINKING": "llm.extraction.thinking",
	}
	assert.Equal(t, want, m)
}

func TestEnvVarsCoverAllKeys(t *testing.T) {
	// Every env-mapped key must be a valid config key.
	keys := Keys()
	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[k] = true
	}
	for envVar, configKey := range EnvVars() {
		assert.True(t, keySet[configKey],
			"env var %s maps to %q which is not a valid config key", envVar, configKey)
	}
}

// --- Deprecated key migration ---

func TestMaxOCRPagesTOMLMigration(t *testing.T) {
	path := writeConfig(t, "[extraction]\nmax_ocr_pages = 10\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, 10, cfg.Extraction.MaxExtractPages)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0], "max_ocr_pages")
	assert.Contains(t, cfg.Warnings[0], "max_extract_pages")
}

func TestMaxOCRPagesEnvMigration(t *testing.T) {
	t.Setenv("MICASA_MAX_OCR_PAGES", "15")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, 15, cfg.Extraction.MaxExtractPages)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0], "MICASA_MAX_OCR_PAGES")
}

func TestMaxOCRPagesEnvIgnoredWhenNewEnvSet(t *testing.T) {
	t.Setenv("MICASA_MAX_OCR_PAGES", "15")
	t.Setenv("MICASA_MAX_EXTRACT_PAGES", "25")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, 25, cfg.Extraction.MaxExtractPages)
	assert.Empty(t, cfg.Warnings)
}

// --- Per-pipeline LLM config ---

func TestChatConfigInheritsBase(t *testing.T) {
	path := writeConfig(t, `[llm]
provider = "ollama"
base_url = "http://localhost:11434"
model = "qwen3"
timeout = "5s"
thinking = "medium"
extra_context = "My house is old."
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	chat := cfg.LLM.ChatConfig()
	assert.Equal(t, "ollama", chat.Provider)
	assert.Equal(t, "http://localhost:11434", chat.BaseURL)
	assert.Equal(t, "qwen3", chat.Model)
	assert.Equal(t, 5*time.Second, chat.Timeout)
	assert.Equal(t, "medium", chat.Thinking)
	assert.Equal(t, "My house is old.", chat.ExtraContext)
}

func TestExtractionConfigInheritsBase(t *testing.T) {
	path := writeConfig(t, `[llm]
provider = "ollama"
base_url = "http://localhost:11434"
model = "qwen3"
api_key = ""
timeout = "5s"
extra_context = "Portland house."
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	ex := cfg.LLM.ExtractionConfig()
	assert.Equal(t, "ollama", ex.Provider)
	assert.Equal(t, "http://localhost:11434", ex.BaseURL)
	assert.Equal(t, "qwen3", ex.Model)
	assert.Equal(t, 5*time.Second, ex.Timeout)
	assert.Equal(t, "Portland house.", ex.ExtraContext)
}

func TestChatOverridesTakeEffect(t *testing.T) {
	path := writeConfig(t, `[llm]
provider = "ollama"
model = "qwen3"

[llm.chat]
provider = "anthropic"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-test"
timeout = "10s"
thinking = "high"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	chat := cfg.LLM.ChatConfig()
	assert.Equal(t, "anthropic", chat.Provider)
	assert.Equal(t, "claude-sonnet-4-5-20250929", chat.Model)
	assert.Equal(t, "sk-ant-test", chat.APIKey)
	assert.Equal(t, 10*time.Second, chat.Timeout)
	assert.Equal(t, "high", chat.Thinking)

	// Extraction should still inherit the base.
	ex := cfg.LLM.ExtractionConfig()
	assert.Equal(t, "ollama", ex.Provider)
	assert.Equal(t, "qwen3", ex.Model)
}

func TestExtractionOverridesTakeEffect(t *testing.T) {
	path := writeConfig(t, `[llm]
provider = "ollama"
model = "qwen3"

[llm.extraction]
provider = "anthropic"
model = "claude-haiku-3-5-20241022"
api_key = "sk-ant-test"
timeout = "15s"
thinking = "low"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	ex := cfg.LLM.ExtractionConfig()
	assert.Equal(t, "anthropic", ex.Provider)
	assert.Equal(t, "claude-haiku-3-5-20241022", ex.Model)
	assert.Equal(t, "sk-ant-test", ex.APIKey)
	assert.Equal(t, 15*time.Second, ex.Timeout)
	assert.Equal(t, "low", ex.Thinking)

	// Chat should still inherit the base.
	chat := cfg.LLM.ChatConfig()
	assert.Equal(t, "ollama", chat.Provider)
	assert.Equal(t, "qwen3", chat.Model)
}

func TestBothPipelinesOverridden(t *testing.T) {
	path := writeConfig(t, `[llm]
model = "default-model"

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

	chat := cfg.LLM.ChatConfig()
	assert.Equal(t, "openai", chat.Provider)
	assert.Equal(t, "gpt-4o", chat.Model)
	assert.Equal(t, "sk-openai", chat.APIKey)

	ex := cfg.LLM.ExtractionConfig()
	assert.Equal(t, "anthropic", ex.Provider)
	assert.Equal(t, "claude-haiku-3-5-20241022", ex.Model)
	assert.Equal(t, "sk-ant", ex.APIKey)
}

func TestExtractionAutoDetectsProviderFromOverride(t *testing.T) {
	path := writeConfig(t, `[llm]
provider = "ollama"
model = "qwen3"

[llm.extraction]
base_url = "https://api.anthropic.com"
model = "claude-haiku-3-5-20241022"
api_key = "sk-ant-test"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	ex := cfg.LLM.ExtractionConfig()
	assert.Equal(t, "anthropic", ex.Provider,
		"provider should be auto-detected from extraction base_url")
}

func TestOverrideBaseURLNormalized(t *testing.T) {
	path := writeConfig(t, `[llm]
model = "qwen3"

[llm.chat]
base_url = "http://localhost:8080/v1"

[llm.extraction]
base_url = "https://api.example.com/"
api_key = "key"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:8080", cfg.LLM.Chat.BaseURL,
		"chat override base_url /v1 suffix should be stripped")
	assert.Equal(t, "https://api.example.com", cfg.LLM.Extraction.BaseURL,
		"extraction override trailing slash should be stripped")
}

func TestOverrideInvalidProviderReturnsError(t *testing.T) {
	t.Run("chat", func(t *testing.T) {
		path := writeConfig(t, `[llm.chat]
provider = "bogus"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.chat.provider")
		assert.Contains(t, err.Error(), "bogus")
	})
	t.Run("extraction", func(t *testing.T) {
		path := writeConfig(t, `[llm.extraction]
provider = "bogus"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.extraction.provider")
		assert.Contains(t, err.Error(), "bogus")
	})
}

func TestOverrideInvalidThinkingReturnsError(t *testing.T) {
	t.Run("chat", func(t *testing.T) {
		path := writeConfig(t, `[llm.chat]
thinking = "bogus"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.chat.thinking")
	})
	t.Run("extraction", func(t *testing.T) {
		path := writeConfig(t, `[llm.extraction]
thinking = "bogus"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.extraction.thinking")
	})
}

func TestOverrideInvalidTimeoutReturnsError(t *testing.T) {
	t.Run("chat invalid", func(t *testing.T) {
		path := writeConfig(t, `[llm.chat]
timeout = "nope"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.chat.timeout")
	})
	t.Run("chat negative", func(t *testing.T) {
		path := writeConfig(t, `[llm.chat]
timeout = "-1s"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	})
	t.Run("extraction invalid", func(t *testing.T) {
		path := writeConfig(t, `[llm.extraction]
timeout = "nope"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "llm.extraction.timeout")
	})
}

func TestOverrideEnvVars(t *testing.T) {
	t.Setenv("MICASA_LLM_EXTRACTION_PROVIDER", "anthropic")
	t.Setenv("MICASA_LLM_EXTRACTION_MODEL", "claude-haiku-3-5-20241022")
	t.Setenv("MICASA_LLM_EXTRACTION_API_KEY", "sk-ant-from-env")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.LLM.Extraction.Provider)
	assert.Equal(t, "claude-haiku-3-5-20241022", cfg.LLM.Extraction.Model)
	assert.Equal(t, "sk-ant-from-env", cfg.LLM.Extraction.APIKey)

	ex := cfg.LLM.ExtractionConfig()
	assert.Equal(t, "anthropic", ex.Provider)
	assert.Equal(t, "claude-haiku-3-5-20241022", ex.Model)
}

func TestOverrideChatEnvVars(t *testing.T) {
	t.Setenv("MICASA_LLM_CHAT_PROVIDER", "openai")
	t.Setenv("MICASA_LLM_CHAT_MODEL", "gpt-4o")
	t.Setenv("MICASA_LLM_CHAT_API_KEY", "sk-openai-from-env")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	chat := cfg.LLM.ChatConfig()
	assert.Equal(t, "openai", chat.Provider)
	assert.Equal(t, "gpt-4o", chat.Model)
	assert.Equal(t, "sk-openai-from-env", chat.APIKey)
}

// --- Deprecation: extraction.model -> llm.extraction.model ---

func TestExtractionModelTOMLMigration(t *testing.T) {
	path := writeConfig(t, `[llm]
model = "qwen3"

[extraction]
model = "qwen2.5:7b"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "qwen2.5:7b", cfg.LLM.Extraction.Model)
	require.NotEmpty(t, cfg.Warnings)
	assert.Contains(t, cfg.Warnings[len(cfg.Warnings)-1], "extraction.model")
	assert.Contains(t, cfg.Warnings[len(cfg.Warnings)-1], "llm.extraction.model")
}

func TestExtractionModelTOMLMigrationNotOverrideNew(t *testing.T) {
	path := writeConfig(t, `[llm]
model = "qwen3"

[llm.extraction]
model = "new-model"

[extraction]
model = "old-model"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "new-model", cfg.LLM.Extraction.Model,
		"new config should take precedence over deprecated")
	// No deprecation warning for model since the new key is set.
	for _, w := range cfg.Warnings {
		assert.NotContains(t, w, "extraction.model is deprecated")
	}
}

func TestExtractionModelEnvMigration(t *testing.T) {
	t.Setenv("MICASA_EXTRACTION_MODEL", "old-model")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "old-model", cfg.LLM.Extraction.Model)
	require.NotEmpty(t, cfg.Warnings)
	found := false
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "MICASA_EXTRACTION_MODEL") {
			found = true
		}
	}
	assert.True(t, found, "should warn about deprecated env var")
}

func TestExtractionModelEnvMigrationNotOverrideNew(t *testing.T) {
	t.Setenv("MICASA_EXTRACTION_MODEL", "old")
	t.Setenv("MICASA_LLM_EXTRACTION_MODEL", "new")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "new", cfg.LLM.Extraction.Model)
}

func TestExtractionThinkingTOMLMigration(t *testing.T) {
	path := writeConfig(t, `[extraction]
thinking = "low"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "low", cfg.LLM.Extraction.Thinking)
	require.NotEmpty(t, cfg.Warnings)
	found := false
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "extraction.thinking") &&
			strings.Contains(w, "llm.extraction.thinking") {
			found = true
		}
	}
	assert.True(t, found, "should warn about deprecated TOML key")
}

func TestExtractionThinkingEnvMigration(t *testing.T) {
	t.Setenv("MICASA_EXTRACTION_THINKING", "high")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "high", cfg.LLM.Extraction.Thinking)
	found := false
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "MICASA_EXTRACTION_THINKING") {
			found = true
		}
	}
	assert.True(t, found, "should warn about deprecated env var")
}

func TestConfigGetPipelineKeys(t *testing.T) {
	cfg := Config{
		LLM: LLM{
			Model: "base-model",
			Chat: LLMChatOverride{
				Provider: "openai",
				Model:    "gpt-4o",
			},
			Extraction: LLMExtractionOverride{
				Provider: "anthropic",
				Model:    "claude-haiku",
			},
		},
	}

	got, err := cfg.Get("llm.chat.provider")
	require.NoError(t, err)
	assert.Equal(t, "openai", got)

	got, err = cfg.Get("llm.chat.model")
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", got)

	got, err = cfg.Get("llm.extraction.provider")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", got)

	got, err = cfg.Get("llm.extraction.model")
	require.NoError(t, err)
	assert.Equal(t, "claude-haiku", got)
}

func TestKeysPipelineKeys(t *testing.T) {
	keys := Keys()
	assert.Contains(t, keys, "llm.chat.provider")
	assert.Contains(t, keys, "llm.chat.model")
	assert.Contains(t, keys, "llm.extraction.provider")
	assert.Contains(t, keys, "llm.extraction.model")
}

func TestExampleTOMLPipelineSections(t *testing.T) {
	example := ExampleTOML()
	assert.Contains(t, example, "[llm.chat]")
	assert.Contains(t, example, "[llm.extraction]")
}

func writeConfigPerm(t *testing.T, content string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), perm))
	return path
}

func TestPermissionWarningWithAPIKey(t *testing.T) {
	path := writeConfigPerm(t, `[llm]
api_key = "sk-ant-test"
`, 0o644)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	var found bool
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "permissions") && strings.Contains(w, "0644") {
			found = true
			assert.Contains(t, w, "chmod 600")
			assert.Contains(t, w, path)
		}
	}
	assert.True(t, found, "expected a permission warning for 0644 config with API key")
}

func TestPermissionWarningChatAPIKey(t *testing.T) {
	path := writeConfigPerm(t, `[llm.chat]
api_key = "sk-ant-chat"
provider = "anthropic"
`, 0o640)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	var found bool
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "permissions") {
			found = true
		}
	}
	assert.True(t, found, "expected a permission warning for config with llm.chat.api_key")
}

func TestPermissionWarningExtractionAPIKey(t *testing.T) {
	path := writeConfigPerm(t, `[llm.extraction]
api_key = "sk-ant-ext"
provider = "anthropic"
`, 0o604)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	var found bool
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "permissions") {
			found = true
		}
	}
	assert.True(t, found, "expected a permission warning for config with llm.extraction.api_key")
}

func TestNoPermissionWarningWhenSecure(t *testing.T) {
	path := writeConfigPerm(t, `[llm]
api_key = "sk-ant-test"
`, 0o600)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	for _, w := range cfg.Warnings {
		assert.NotContains(t, w, "permissions",
			"should not warn when permissions are 0600")
	}
}

func TestNoPermissionWarningWithoutAPIKey(t *testing.T) {
	path := writeConfigPerm(t, `[llm]
model = "qwen3"
`, 0o644)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	for _, w := range cfg.Warnings {
		assert.NotContains(t, w, "permissions",
			"should not warn about permissions when no API keys are set")
	}
}

func TestNoPermissionWarningNoFile(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)

	for _, w := range cfg.Warnings {
		assert.NotContains(t, w, "permissions",
			"should not warn about permissions when config file does not exist")
	}
}

func TestNoOverridesBackwardCompatible(t *testing.T) {
	path := writeConfig(t, `[llm]
provider = "anthropic"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-test"
timeout = "10s"
thinking = "high"
extra_context = "Portland house."
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	// Both pipelines should get the same settings.
	chat := cfg.LLM.ChatConfig()
	ex := cfg.LLM.ExtractionConfig()

	assert.Equal(t, chat.Provider, ex.Provider)
	assert.Equal(t, chat.BaseURL, ex.BaseURL)
	assert.Equal(t, chat.Model, ex.Model)
	assert.Equal(t, chat.APIKey, ex.APIKey)
	assert.Equal(t, chat.Timeout, ex.Timeout)
	assert.Equal(t, chat.Thinking, ex.Thinking)
	assert.Equal(t, chat.ExtraContext, ex.ExtraContext)
}
