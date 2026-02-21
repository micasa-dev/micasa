// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"os"
	"path/filepath"
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
base_url = "http://myhost:8080/v1"
model = "llama3"
extra_context = "My house is old."
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "http://myhost:8080/v1", cfg.LLM.BaseURL)
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
base_url = "http://file-host:1234/v1"
model = "from-file"
`)
	t.Setenv("OLLAMA_HOST", "http://env-host:5678")
	t.Setenv("MICASA_LLM_MODEL", "from-env")

	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "http://env-host:5678/v1", cfg.LLM.BaseURL)
	assert.Equal(t, "from-env", cfg.LLM.Model)
}

func TestOllamaHostAppendsV1(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://myhost:11434")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "http://myhost:11434/v1", cfg.LLM.BaseURL)
}

func TestOllamaHostAlreadyHasV1(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "http://myhost:11434/v1")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "http://myhost:11434/v1", cfg.LLM.BaseURL)
}

func TestTrailingSlashStripped(t *testing.T) {
	path := writeConfig(t, `[llm]
base_url = "http://localhost:11434/v1/"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:11434/v1", cfg.LLM.BaseURL)
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
	assert.Contains(t, example, "max_ocr_pages")
}

func TestMalformedConfigReturnsError(t *testing.T) {
	path := writeConfig(t, "{{not toml")

	_, err := LoadFromPath(path)
	assert.Error(t, err)
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

func TestExtractionDefaults(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxOCRPages, cfg.Extraction.MaxOCRPages)
	assert.True(t, cfg.Extraction.IsEnabled())
	assert.Empty(t, cfg.Extraction.Model)
}

func TestExtractionFromFile(t *testing.T) {
	path := writeConfig(t, `[extraction]
model = "qwen2.5:7b"
max_ocr_pages = 10
enabled = false
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "qwen2.5:7b", cfg.Extraction.Model)
	assert.Equal(t, 10, cfg.Extraction.MaxOCRPages)
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
	t.Setenv("MICASA_MAX_OCR_PAGES", "5")
	t.Setenv("MICASA_EXTRACTION_ENABLED", "false")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "phi3", cfg.Extraction.Model)
	assert.Equal(t, 5, cfg.Extraction.MaxOCRPages)
	assert.False(t, cfg.Extraction.IsEnabled())
}

func TestExtractionRejectsNegativePages(t *testing.T) {
	path := writeConfig(t, "[extraction]\nmax_ocr_pages = -1\n")
	_, err := LoadFromPath(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}
