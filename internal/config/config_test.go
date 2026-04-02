// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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
	assert.Equal(t, DefaultBaseURL, cfg.Chat.LLM.BaseURL)
	assert.Equal(t, DefaultModel, cfg.Chat.LLM.Model)
	assert.Equal(t, DefaultBaseURL, cfg.Extraction.LLM.BaseURL)
	assert.Equal(t, DefaultModel, cfg.Extraction.LLM.Model)
}

func TestLoadFromFile(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
base_url = "http://myhost:8080"
model = "llama3"
extra_context = "My house is old."
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "http://myhost:8080", cfg.Chat.LLM.BaseURL)
	assert.Equal(t, "llama3", cfg.Chat.LLM.Model)
	assert.Equal(t, "My house is old.", cfg.Chat.LLM.ExtraContext)
}

func TestPartialConfigUsesDefaults(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
model = "phi3"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, DefaultBaseURL, cfg.Chat.LLM.BaseURL)
	assert.Equal(t, "phi3", cfg.Chat.LLM.Model)
}

func TestEnvOverridesConfig(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
base_url = "http://file-host:1234"
model = "from-file"
`)
	t.Setenv("MICASA_CHAT_LLM_BASE_URL", "http://env-host:5678")
	t.Setenv("MICASA_CHAT_LLM_MODEL", "from-env")

	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "http://env-host:5678", cfg.Chat.LLM.BaseURL)
	assert.Equal(t, "from-env", cfg.Chat.LLM.Model)
}

func TestBaseURLV1SuffixStripped(t *testing.T) {
	t.Run("chat", func(t *testing.T) {
		path := writeConfig(t, `[chat.llm]
base_url = "http://myhost:11434/v1"
`)
		cfg, err := LoadFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, "http://myhost:11434", cfg.Chat.LLM.BaseURL,
			"/v1 suffix should be stripped")
	})
	t.Run("extraction", func(t *testing.T) {
		path := writeConfig(t, `[extraction.llm]
base_url = "http://myhost:11434/v1"
`)
		cfg, err := LoadFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, "http://myhost:11434", cfg.Extraction.LLM.BaseURL,
			"/v1 suffix should be stripped")
	})
}

func TestTrailingSlashStripped(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
base_url = "http://localhost:11434/"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:11434", cfg.Chat.LLM.BaseURL)
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
	path := writeConfig(t, `[chat.llm]
provider = "anthropic"
api_key = "sk-ant-test"
model = "claude-sonnet-4-5-20250929"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.Chat.LLM.Provider)
}

func TestProviderEnvOverride(t *testing.T) {
	t.Setenv("MICASA_CHAT_LLM_PROVIDER", "openai")
	t.Setenv("MICASA_CHAT_LLM_API_KEY", "sk-test")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.Chat.LLM.Provider)
}

func TestProviderInvalidReturnsError(t *testing.T) {
	t.Run("chat", func(t *testing.T) {
		path := writeConfig(t, `[chat.llm]
provider = "bogus"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chat.llm.provider")
		assert.Contains(t, err.Error(), "bogus")
		assert.Contains(t, err.Error(), "supported")
	})
	t.Run("extraction", func(t *testing.T) {
		path := writeConfig(t, `[extraction.llm]
provider = "bogus"
`)
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extraction.llm.provider")
		assert.Contains(t, err.Error(), "bogus")
	})
}

func TestExampleTOML(t *testing.T) {
	example := ExampleTOML()
	assert.Contains(t, example, "[chat]")
	assert.Contains(t, example, "[chat.llm]")
	assert.Contains(t, example, "[extraction]")
	assert.Contains(t, example, "[extraction.llm]")
	assert.Contains(t, example, "[extraction.ocr]")
	assert.Contains(t, example, "[extraction.ocr.tsv]")
	assert.Contains(t, example, "[documents]")
	assert.Contains(t, example, "[locale]")
	assert.Contains(t, example, "base_url")
	assert.Contains(t, example, "model")
	assert.Contains(t, example, "timeout")
	assert.Contains(t, example, "max_file_size")
	assert.Contains(t, example, "cache_ttl")
	assert.Contains(t, example, "max_pages")
	assert.Contains(t, example, "extra_context")
	assert.Contains(t, example, "confidence_threshold")
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
	assert.Equal(t, uint64(50<<20), cfg.Documents.MaxFileSize.Bytes())
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
	t.Setenv("MICASA_DOCUMENTS_MAX_FILE_SIZE", "2097152")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, uint64(2097152), cfg.Documents.MaxFileSize.Bytes())
}

func TestMaxDocumentSizeEnvOverrideUnitized(t *testing.T) {
	t.Setenv("MICASA_DOCUMENTS_MAX_FILE_SIZE", "100 MiB")
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
	t.Setenv("MICASA_DOCUMENTS_CACHE_TTL", "14d")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, 14*24*time.Hour, cfg.Documents.CacheTTLDuration())
}

func TestCacheTTLEnvOverrideSeconds(t *testing.T) {
	t.Setenv("MICASA_DOCUMENTS_CACHE_TTL", "86400")
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

func TestCacheTTLDaysRemovedReturnsError(t *testing.T) {
	path := writeConfig(t, "[documents]\ncache_ttl_days = 7\n")
	_, err := LoadFromPath(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache_ttl_days")
	assert.Contains(t, err.Error(), "removed")
	assert.Contains(t, err.Error(), "cache_ttl")
	assert.Contains(t, err.Error(), "integer days become duration strings")
}

// --- API Keys ---

func TestAPIKeyFromFile(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
api_key = "sk-ant-test-key"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-test-key", cfg.Chat.LLM.APIKey)
}

func TestAPIKeyDefaultEmpty(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Empty(t, cfg.Chat.LLM.APIKey)
	assert.Empty(t, cfg.Extraction.LLM.APIKey)
}

func TestAPIKeyEnvOverride(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
api_key = "from-file"
`)
	t.Setenv("MICASA_CHAT_LLM_API_KEY", "from-env")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "from-env", cfg.Chat.LLM.APIKey)
}

func TestCloudProviderConfig(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
base_url = "https://api.anthropic.com/v1"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-api03-secret"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "https://api.anthropic.com", cfg.Chat.LLM.BaseURL,
		"/v1 suffix should be stripped")
	assert.Equal(t, "claude-sonnet-4-5-20250929", cfg.Chat.LLM.Model)
	assert.Equal(t, "sk-ant-api03-secret", cfg.Chat.LLM.APIKey)
}

// --- Chat LLM Timeout ---

func TestChatLLMTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg, err := LoadFromPath(noConfig(t))
		require.NoError(t, err)
		assert.Equal(t, DefaultLLMTimeout, cfg.Chat.LLM.TimeoutDuration())
	})

	t.Run("from file", func(t *testing.T) {
		path := writeConfig(t, "[chat.llm]\ntimeout = \"10s\"\n")
		cfg, err := LoadFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, 10*time.Second, cfg.Chat.LLM.TimeoutDuration())
	})

	t.Run("sub-second", func(t *testing.T) {
		path := writeConfig(t, "[chat.llm]\ntimeout = \"500ms\"\n")
		cfg, err := LoadFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, 500*time.Millisecond, cfg.Chat.LLM.TimeoutDuration())
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("MICASA_CHAT_LLM_TIMEOUT", "15s")
		cfg, err := LoadFromPath(noConfig(t))
		require.NoError(t, err)
		assert.Equal(t, 15*time.Second, cfg.Chat.LLM.TimeoutDuration())
	})

	t.Run("rejects invalid", func(t *testing.T) {
		path := writeConfig(t, "[chat.llm]\ntimeout = \"not-a-duration\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid duration")
	})

	t.Run("rejects negative", func(t *testing.T) {
		path := writeConfig(t, "[chat.llm]\ntimeout = \"-1s\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	})
}

// --- Extraction LLM Timeout ---

func TestExtractionLLMTimeout(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg, err := LoadFromPath(noConfig(t))
		require.NoError(t, err)
		assert.Equal(t, DefaultLLMTimeout, cfg.Extraction.LLM.TimeoutDuration())
	})

	t.Run("from file", func(t *testing.T) {
		path := writeConfig(t, "[extraction.llm]\ntimeout = \"90s\"\n")
		cfg, err := LoadFromPath(path)
		require.NoError(t, err)
		assert.Equal(t, 90*time.Second, cfg.Extraction.LLM.TimeoutDuration())
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("MICASA_EXTRACTION_LLM_TIMEOUT", "3m")
		cfg, err := LoadFromPath(noConfig(t))
		require.NoError(t, err)
		assert.Equal(t, 3*time.Minute, cfg.Extraction.LLM.TimeoutDuration())
	})

	t.Run("rejects invalid", func(t *testing.T) {
		path := writeConfig(t, "[extraction.llm]\ntimeout = \"not-a-duration\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid duration")
	})

	t.Run("rejects negative", func(t *testing.T) {
		path := writeConfig(t, "[extraction.llm]\ntimeout = \"-1s\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	})
}

// --- Extraction ---

func TestExtractionDefaults(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxPages, cfg.Extraction.MaxPages)
	assert.True(t, cfg.Extraction.LLM.IsEnabled())
	assert.Equal(t, DefaultModel, cfg.Extraction.LLM.Model)
}

func TestExtractionFromFile(t *testing.T) {
	path := writeConfig(t, `[extraction]
max_pages = 10

[extraction.llm]
model = "qwen2.5:7b"
enable = false
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "qwen2.5:7b", cfg.Extraction.LLM.Model)
	assert.Equal(t, 10, cfg.Extraction.MaxPages)
	assert.False(t, cfg.Extraction.LLM.IsEnabled())
}

func TestExtractionEnvOverrides(t *testing.T) {
	t.Setenv("MICASA_EXTRACTION_LLM_MODEL", "phi3")
	t.Setenv("MICASA_EXTRACTION_MAX_PAGES", "5")
	t.Setenv("MICASA_EXTRACTION_LLM_ENABLE", "false")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "phi3", cfg.Extraction.LLM.Model)
	assert.Equal(t, 5, cfg.Extraction.MaxPages)
	assert.False(t, cfg.Extraction.LLM.IsEnabled())
}

func TestExtractionRejectsNegativePages(t *testing.T) {
	path := writeConfig(t, "[extraction]\nmax_pages = -1\n")
	_, err := LoadFromPath(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}

// --- Chat enable ---

func TestChatEnableDefault(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.True(t, cfg.Chat.IsEnabled())
}

func TestChatEnableFromFile(t *testing.T) {
	path := writeConfig(t, `[chat]
enable = false
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.False(t, cfg.Chat.IsEnabled())
}

func TestChatEnableFromEnv(t *testing.T) {
	t.Setenv("MICASA_CHAT_ENABLE", "false")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.False(t, cfg.Chat.IsEnabled())
}

// --- Invalid env values ---

func TestInvalidEnvVarReturnsError(t *testing.T) {
	tests := []struct {
		envVar  string
		value   string
		wantMsg string
	}{
		{"MICASA_EXTRACTION_MAX_PAGES", "not-a-number", "expected integer"},
		{"MICASA_EXTRACTION_LLM_ENABLE", "maybe", "expected true or false"},
		{"MICASA_DOCUMENTS_MAX_FILE_SIZE", "lots", "expected byte size"},
		{"MICASA_DOCUMENTS_CACHE_TTL", "forever", "expected duration"},
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

func TestInvalidEffortLevelReturnsError(t *testing.T) {
	t.Run("chat.llm.effort", func(t *testing.T) {
		path := writeConfig(t, "[chat.llm]\neffort = \"dunno\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chat.llm.effort")
		assert.Contains(t, err.Error(), "invalid level")
		assert.Contains(t, err.Error(), "dunno")
	})
	t.Run("extraction.llm.effort", func(t *testing.T) {
		path := writeConfig(t, "[extraction.llm]\neffort = \"yolo\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extraction.llm.effort")
		assert.Contains(t, err.Error(), "invalid level")
		assert.Contains(t, err.Error(), "yolo")
	})
	t.Run("valid levels", func(t *testing.T) {
		for _, level := range []string{"none", "low", "medium", "high", "auto"} {
			path := writeConfig(t, "[chat.llm]\neffort = \""+level+"\"\n")
			cfg, err := LoadFromPath(path)
			require.NoError(t, err, "level %q should be valid", level)
			assert.Equal(t, level, cfg.Chat.LLM.Effort)
		}
	})
}

func TestInvalidTimeoutReturnsError(t *testing.T) {
	t.Run("chat invalid", func(t *testing.T) {
		path := writeConfig(t, "[chat.llm]\ntimeout = \"nope\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chat.llm.timeout")
	})
	t.Run("chat negative", func(t *testing.T) {
		path := writeConfig(t, "[chat.llm]\ntimeout = \"-1s\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	})
	t.Run("extraction invalid", func(t *testing.T) {
		path := writeConfig(t, "[extraction.llm]\ntimeout = \"nope\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extraction.llm.timeout")
	})
	t.Run("extraction negative", func(t *testing.T) {
		path := writeConfig(t, "[extraction.llm]\ntimeout = \"-1s\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be positive")
	})
}

// --- Config Get ---

func TestConfigGet(t *testing.T) {
	cfg := Config{
		Chat: Chat{
			LLM: ChatLLM{
				BaseURL:      "http://localhost:11434/v1",
				Model:        "qwen3",
				ExtraContext: "my house",
				Timeout:      "10s",
			},
		},
		Extraction: Extraction{
			LLM: ExtractionLLM{
				Model:   "phi3",
				Timeout: "3m",
			},
		},
		Documents: Documents{
			MaxFileSize: ByteSize(1024),
		},
	}

	tests := []struct {
		key  string
		want string
	}{
		{"chat.llm.base_url", "http://localhost:11434/v1"},
		{"chat.llm.model", "qwen3"},
		{"chat.llm.extra_context", "my house"},
		{"chat.llm.timeout", "10s"},
		{"extraction.llm.model", "phi3"},
		{"extraction.llm.timeout", "3m"},
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
	assert.Contains(t, keys, "chat.llm.model")
	assert.Contains(t, keys, "chat.llm.base_url")
	assert.Contains(t, keys, "chat.llm.extra_context")
	assert.Contains(t, keys, "extraction.llm.model")
	assert.Contains(t, keys, "extraction.llm.enable")
	assert.Contains(t, keys, "extraction.max_pages")
	assert.Contains(t, keys, "extraction.ocr.enable")
	assert.Contains(t, keys, "extraction.ocr.tsv.enable")
	assert.Contains(t, keys, "extraction.ocr.tsv.confidence_threshold")
	assert.Contains(t, keys, "documents.max_file_size")
	assert.Contains(t, keys, "locale.currency")

	// Verify every key is resolvable against a default config.
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
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
		"MICASA_CHAT_ENABLE":            "chat.enable",
		"MICASA_CHAT_LLM_PROVIDER":      "chat.llm.provider",
		"MICASA_CHAT_LLM_BASE_URL":      "chat.llm.base_url",
		"MICASA_CHAT_LLM_MODEL":         "chat.llm.model",
		"MICASA_CHAT_LLM_API_KEY":       "chat.llm.api_key",
		"MICASA_CHAT_LLM_TIMEOUT":       "chat.llm.timeout",
		"MICASA_CHAT_LLM_EFFORT":        "chat.llm.effort",
		"MICASA_CHAT_LLM_EXTRA_CONTEXT": "chat.llm.extra_context",

		"MICASA_EXTRACTION_MAX_PAGES":                    "extraction.max_pages",
		"MICASA_EXTRACTION_LLM_ENABLE":                   "extraction.llm.enable",
		"MICASA_EXTRACTION_LLM_PROVIDER":                 "extraction.llm.provider",
		"MICASA_EXTRACTION_LLM_BASE_URL":                 "extraction.llm.base_url",
		"MICASA_EXTRACTION_LLM_MODEL":                    "extraction.llm.model",
		"MICASA_EXTRACTION_LLM_API_KEY":                  "extraction.llm.api_key",
		"MICASA_EXTRACTION_LLM_TIMEOUT":                  "extraction.llm.timeout",
		"MICASA_EXTRACTION_LLM_EFFORT":                   "extraction.llm.effort",
		"MICASA_EXTRACTION_OCR_ENABLE":                   "extraction.ocr.enable",
		"MICASA_EXTRACTION_OCR_TSV_ENABLE":               "extraction.ocr.tsv.enable",
		"MICASA_EXTRACTION_OCR_TSV_CONFIDENCE_THRESHOLD": "extraction.ocr.tsv.confidence_threshold",

		"MICASA_DOCUMENTS_MAX_FILE_SIZE":   "documents.max_file_size",
		"MICASA_DOCUMENTS_CACHE_TTL":       "documents.cache_ttl",
		"MICASA_DOCUMENTS_FILE_PICKER_DIR": "documents.file_picker_dir",

		"MICASA_LOCALE_CURRENCY": "locale.currency",

		"MICASA_ADDRESS_AUTOFILL": "address.autofill",
	}
	assert.Equal(t, want, m)
}

func TestEnvVarName(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"chat.llm.model", "MICASA_CHAT_LLM_MODEL"},
		{"extraction.llm.timeout", "MICASA_EXTRACTION_LLM_TIMEOUT"},
		{"documents.max_file_size", "MICASA_DOCUMENTS_MAX_FILE_SIZE"},
		{"locale.currency", "MICASA_LOCALE_CURRENCY"},
		{
			"extraction.ocr.tsv.confidence_threshold",
			"MICASA_EXTRACTION_OCR_TSV_CONFIDENCE_THRESHOLD",
		},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			assert.Equal(t, tt.want, EnvVarName(tt.key))
		})
	}
}

func TestEnvVarsCoverAllKeys(t *testing.T) {
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

// --- OCR config ---

func TestOCRDefaults(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.True(t, cfg.Extraction.OCR.IsEnabled())
	assert.Equal(t, 70, cfg.Extraction.OCR.TSV.Threshold())
}

func TestOCRFromFile(t *testing.T) {
	path := writeConfig(t, `[extraction.ocr]
enable = false

[extraction.ocr.tsv]
confidence_threshold = 50
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.False(t, cfg.Extraction.OCR.IsEnabled())
	assert.Equal(t, 50, cfg.Extraction.OCR.TSV.Threshold())
}

func TestOCREnvOverrides(t *testing.T) {
	t.Setenv("MICASA_EXTRACTION_OCR_ENABLE", "false")
	t.Setenv("MICASA_EXTRACTION_OCR_TSV_CONFIDENCE_THRESHOLD", "80")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.False(t, cfg.Extraction.OCR.IsEnabled())
	assert.Equal(t, 80, cfg.Extraction.OCR.TSV.Threshold())
}

func TestOCRConfidenceThresholdValidation(t *testing.T) {
	t.Run("rejects negative", func(t *testing.T) {
		path := writeConfig(t, "[extraction.ocr.tsv]\nconfidence_threshold = -1\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "confidence_threshold must be 0-100")
	})
	t.Run("rejects over 100", func(t *testing.T) {
		path := writeConfig(t, "[extraction.ocr.tsv]\nconfidence_threshold = 101\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "confidence_threshold must be 0-100")
	})
}

// --- OCR TSV ---

func TestOCRTSVDefaultTrue(t *testing.T) {
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.True(t, cfg.Extraction.OCR.TSV.IsEnabled())
}

func TestOCRTSVFromTOML(t *testing.T) {
	path := writeConfig(t, "[extraction.ocr.tsv]\nenable = true\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.True(t, cfg.Extraction.OCR.TSV.IsEnabled())
}

func TestOCRTSVFromTOMLFalse(t *testing.T) {
	path := writeConfig(t, "[extraction.ocr.tsv]\nenable = false\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.False(t, cfg.Extraction.OCR.TSV.IsEnabled())
}

func TestOCRTSVFromEnv(t *testing.T) {
	t.Setenv("MICASA_EXTRACTION_OCR_TSV_ENABLE", "false")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.False(t, cfg.Extraction.OCR.TSV.IsEnabled())
}

func TestOCRTSVEnvInvalidReturnsError(t *testing.T) {
	t.Setenv("MICASA_EXTRACTION_OCR_TSV_ENABLE", "maybe")
	_, err := LoadFromPath(noConfig(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MICASA_EXTRACTION_OCR_TSV_ENABLE")
	assert.Contains(t, err.Error(), "expected true or false")
}

// --- Independent pipelines ---

func TestChatAndExtractionAreIndependent(t *testing.T) {
	path := writeConfig(t, `[chat.llm]
provider = "anthropic"
model = "claude-sonnet-4-5-20250929"
api_key = "sk-ant-chat"
timeout = "10s"
effort = "high"
extra_context = "Portland house."

[extraction.llm]
provider = "ollama"
model = "qwen2.5:7b"
timeout = "3m"
effort = "low"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	assert.Equal(t, "anthropic", cfg.Chat.LLM.Provider)
	assert.Equal(t, "claude-sonnet-4-5-20250929", cfg.Chat.LLM.Model)
	assert.Equal(t, "sk-ant-chat", cfg.Chat.LLM.APIKey)
	assert.Equal(t, 10*time.Second, cfg.Chat.LLM.TimeoutDuration())
	assert.Equal(t, "high", cfg.Chat.LLM.Effort)
	assert.Equal(t, "Portland house.", cfg.Chat.LLM.ExtraContext)

	assert.Equal(t, "ollama", cfg.Extraction.LLM.Provider)
	assert.Equal(t, "qwen2.5:7b", cfg.Extraction.LLM.Model)
	assert.Empty(t, cfg.Extraction.LLM.APIKey)
	assert.Equal(t, 3*time.Minute, cfg.Extraction.LLM.TimeoutDuration())
	assert.Equal(t, "low", cfg.Extraction.LLM.Effort)
}

func TestExtractionAutoDetectsProvider(t *testing.T) {
	path := writeConfig(t, `[extraction.llm]
base_url = "https://api.anthropic.com"
model = "claude-haiku-3-5-20241022"
api_key = "sk-ant-test"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.Extraction.LLM.Provider,
		"provider should be auto-detected from extraction base_url")
}

func TestExtractionBaseURLNormalized(t *testing.T) {
	path := writeConfig(t, `[extraction.llm]
base_url = "https://api.example.com/"
api_key = "key"
`)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com", cfg.Extraction.LLM.BaseURL,
		"trailing slash should be stripped")
}

func TestExtractionEnvVars(t *testing.T) {
	t.Setenv("MICASA_EXTRACTION_LLM_PROVIDER", "anthropic")
	t.Setenv("MICASA_EXTRACTION_LLM_MODEL", "claude-haiku-3-5-20241022")
	t.Setenv("MICASA_EXTRACTION_LLM_API_KEY", "sk-ant-from-env")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.Extraction.LLM.Provider)
	assert.Equal(t, "claude-haiku-3-5-20241022", cfg.Extraction.LLM.Model)
	assert.Equal(t, "sk-ant-from-env", cfg.Extraction.LLM.APIKey)
}

func TestChatEnvVars(t *testing.T) {
	t.Setenv("MICASA_CHAT_LLM_PROVIDER", "openai")
	t.Setenv("MICASA_CHAT_LLM_MODEL", "gpt-4o")
	t.Setenv("MICASA_CHAT_LLM_API_KEY", "sk-openai-from-env")

	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.Chat.LLM.Provider)
	assert.Equal(t, "gpt-4o", cfg.Chat.LLM.Model)
	assert.Equal(t, "sk-openai-from-env", cfg.Chat.LLM.APIKey)
}

// --- File permissions ---

func writeConfigPerm(t *testing.T, content string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), perm))
	return path
}

func TestPermissionWarningWithAPIKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Chmod is a no-op on Windows")
	}
	path := writeConfigPerm(t, `[chat.llm]
api_key = "sk-ant-test"
`, 0o644)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	var found bool
	for _, w := range cfg.Warnings {
		if findAll(w, "permissions", "0644") {
			found = true
			assert.Contains(t, w, "chmod 600")
			assert.Contains(t, w, path)
		}
	}
	assert.True(t, found, "expected a permission warning for 0644 config with API key")
}

func TestPermissionWarningExtractionAPIKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Chmod is a no-op on Windows")
	}
	path := writeConfigPerm(t, `[extraction.llm]
api_key = "sk-ant-ext"
provider = "anthropic"
`, 0o604)
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)

	var found bool
	for _, w := range cfg.Warnings {
		if findAll(w, "permissions") {
			found = true
		}
	}
	assert.True(t, found, "expected a permission warning for config with extraction.llm.api_key")
}

func TestNoPermissionWarningWhenSecure(t *testing.T) {
	path := writeConfigPerm(t, `[chat.llm]
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
	path := writeConfigPerm(t, `[chat.llm]
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

// --- FilePickerDir ---

func TestResolvedFilePickerDir_ConfiguredDirExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d := Documents{FilePickerDir: dir}
	assert.Equal(t, dir, d.ResolvedFilePickerDir())
}

func TestResolvedFilePickerDir_ConfiguredDirMissing(t *testing.T) {
	t.Parallel()
	d := Documents{FilePickerDir: "/nonexistent/path/that/does/not/exist"}
	result := d.ResolvedFilePickerDir()
	assert.NotEqual(t, "/nonexistent/path/that/does/not/exist", result)
	assert.NotEmpty(t, result)
}

func TestResolvedFilePickerDir_EmptyFallsBackToDownloadsOrCwd(t *testing.T) {
	t.Parallel()
	d := Documents{}
	result := d.ResolvedFilePickerDir()
	assert.NotEmpty(t, result)
}

func TestFilePickerDir_FromTOML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeConfig(t, "[documents]\nfile_picker_dir = '"+dir+"'\n")
	cfg, err := LoadFromPath(path)
	require.NoError(t, err)
	assert.Equal(t, dir, cfg.Documents.FilePickerDir)
}

func TestFilePickerDir_FromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MICASA_DOCUMENTS_FILE_PICKER_DIR", dir)
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Equal(t, dir, cfg.Documents.FilePickerDir)
}

// --- Deprecated key detection ---

func TestThinkingRemovedReturnsError(t *testing.T) {
	t.Run("chat.llm", func(t *testing.T) {
		path := writeConfig(t, "[chat.llm]\nthinking = \"medium\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chat.llm.thinking")
		assert.Contains(t, err.Error(), "removed")
		assert.Contains(t, err.Error(), "chat.llm.effort")
	})
	t.Run("extraction.llm", func(t *testing.T) {
		path := writeConfig(t, "[extraction.llm]\nthinking = \"low\"\n")
		_, err := LoadFromPath(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extraction.llm.thinking")
		assert.Contains(t, err.Error(), "removed")
		assert.Contains(t, err.Error(), "extraction.llm.effort")
	})
}

func TestThinkingEnvVarRemovedReturnsError(t *testing.T) {
	t.Setenv("MICASA_CHAT_LLM_THINKING", "high")
	_, err := LoadFromPath(noConfig(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MICASA_CHAT_LLM_THINKING")
	assert.Contains(t, err.Error(), "removed")
	assert.Contains(t, err.Error(), "MICASA_CHAT_LLM_EFFORT")
}

func TestBothDeprecatedAndNewEnvVarSetErrorsOnDeprecated(t *testing.T) {
	t.Setenv("MICASA_CHAT_LLM_THINKING", "high")
	t.Setenv("MICASA_CHAT_LLM_EFFORT", "medium")
	_, err := LoadFromPath(noConfig(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MICASA_CHAT_LLM_THINKING")
}

func TestEmptyDeprecatedEnvVarDoesNotError(t *testing.T) {
	t.Setenv("MICASA_CHAT_LLM_THINKING", "")
	cfg, err := LoadFromPath(noConfig(t))
	require.NoError(t, err)
	assert.Empty(t, cfg.Chat.LLM.Effort)
}

func TestDeprecatedEnvVarDeterministicOrder(t *testing.T) {
	t.Setenv("MICASA_CHAT_LLM_THINKING", "high")
	t.Setenv("MICASA_EXTRACTION_LLM_THINKING", "low")
	_, err := LoadFromPath(noConfig(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MICASA_CHAT_LLM_THINKING")
}

func TestCacheTTLDaysEnvVarRemovedReturnsError(t *testing.T) {
	t.Setenv("MICASA_DOCUMENTS_CACHE_TTL_DAYS", "30")
	_, err := LoadFromPath(noConfig(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MICASA_DOCUMENTS_CACHE_TTL_DAYS")
	assert.Contains(t, err.Error(), "removed")
	assert.Contains(t, err.Error(), "MICASA_DOCUMENTS_CACHE_TTL")
	assert.Contains(t, err.Error(), "integer days become duration strings")
}

// findAll reports whether s contains all of the given substrings.
func findAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
