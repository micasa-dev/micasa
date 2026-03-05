// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/adrg/xdg"

	"github.com/cpcloud/micasa/internal/data"
)

// Config is the top-level application configuration, loaded from a TOML file.
type Config struct {
	LLM        LLM        `toml:"llm"        doc:"LLM provider, model, and connection settings."`
	Documents  Documents  `toml:"documents"  doc:"Document attachment limits and caching."`
	Extraction Extraction `toml:"extraction" doc:"Document extraction pipeline. Requires an LLM; OCR and pdftotext are internal steps, not standalone features."`
	Locale     Locale     `toml:"locale"     doc:"Locale and currency settings."`

	// Warnings collects non-fatal messages (e.g. deprecations) during load.
	// Not serialized; the caller decides how to display them.
	Warnings []string `toml:"-"`
}

// Locale holds locale-related settings.
type Locale struct {
	// Currency is the ISO 4217 code (e.g. "USD", "EUR", "GBP").
	// Used as the default when the database has no currency set yet.
	Currency string `toml:"currency" env:"MICASA_CURRENCY"`
}

// LLM holds settings for the LLM inference backend.
type LLM struct {
	// Provider selects which LLM provider to use. Supported values:
	// ollama, anthropic, openai, openrouter, deepseek, gemini, groq,
	// mistral, llamacpp, llamafile. Auto-detected when empty.
	Provider string `toml:"provider" env:"MICASA_LLM_PROVIDER"`

	// BaseURL is the base URL for the provider's API.
	// Default varies by provider (e.g. http://localhost:11434 for Ollama).
	// No /v1 suffix needed -- the provider handles path construction.
	BaseURL string `toml:"base_url" env:"MICASA_LLM_BASE_URL"`

	// Model is the model identifier passed in chat requests.
	// Default: qwen3
	Model string `toml:"model" env:"MICASA_LLM_MODEL"`

	// APIKey is the authentication credential. Required for cloud
	// providers (Anthropic, OpenAI, OpenRouter, etc.). Leave empty for local
	// servers like Ollama that don't require authentication.
	APIKey string `toml:"api_key" env:"MICASA_LLM_API_KEY"` //nolint:gosec // config field, not a hardcoded credential

	// ExtraContext is custom text appended to all system prompts.
	// Useful for domain-specific details: house style, location, etc.
	// Currency is handled by [locale] section. Optional; defaults to empty.
	ExtraContext string `toml:"extra_context" env:"MICASA_LLM_EXTRA_CONTEXT"`

	// Timeout is the maximum time to wait for quick LLM server operations
	// (ping, model listing, auto-detect). Go duration string, e.g. "5s",
	// "10s", "500ms". Default: "5s".
	Timeout string `toml:"timeout" env:"MICASA_LLM_TIMEOUT"`

	// Thinking controls the model's reasoning effort level. Supported values:
	// none, low, medium, high, auto. Empty string = don't send (server default).
	Thinking string `toml:"thinking,omitempty" env:"MICASA_LLM_THINKING"`

	// Chat holds per-pipeline overrides for the chat (NL-to-SQL) pipeline.
	// Non-empty fields take precedence over the base values above.
	Chat LLMChatOverride `toml:"chat" doc:"Per-pipeline LLM overrides for chat. Inherits from [llm]."`

	// Extraction holds per-pipeline overrides for the document extraction
	// pipeline. Non-empty fields take precedence over the base values above.
	Extraction LLMExtractionOverride `toml:"extraction" doc:"Per-pipeline LLM overrides for extraction. Inherits from [llm]."`
}

// LLMChatOverride holds optional per-pipeline overrides for the chat
// pipeline. Empty fields inherit from the parent [llm] section.
type LLMChatOverride struct {
	Provider string `toml:"provider"           env:"MICASA_LLM_CHAT_PROVIDER"`
	BaseURL  string `toml:"base_url"           env:"MICASA_LLM_CHAT_BASE_URL"`
	Model    string `toml:"model"              env:"MICASA_LLM_CHAT_MODEL"`
	APIKey   string `toml:"api_key"            env:"MICASA_LLM_CHAT_API_KEY"` //nolint:gosec // config field, not a hardcoded credential
	Timeout  string `toml:"timeout"            env:"MICASA_LLM_CHAT_TIMEOUT"`
	Thinking string `toml:"thinking,omitempty" env:"MICASA_LLM_CHAT_THINKING"`
}

// LLMExtractionOverride holds optional per-pipeline overrides for the
// extraction pipeline. Empty fields inherit from the parent [llm] section.
type LLMExtractionOverride struct {
	Provider string `toml:"provider"           env:"MICASA_LLM_EXTRACTION_PROVIDER"`
	BaseURL  string `toml:"base_url"           env:"MICASA_LLM_EXTRACTION_BASE_URL"`
	Model    string `toml:"model"              env:"MICASA_LLM_EXTRACTION_MODEL"`
	APIKey   string `toml:"api_key"            env:"MICASA_LLM_EXTRACTION_API_KEY"` //nolint:gosec // config field, not a hardcoded credential
	Timeout  string `toml:"timeout"            env:"MICASA_LLM_EXTRACTION_TIMEOUT"`
	Thinking string `toml:"thinking,omitempty" env:"MICASA_LLM_EXTRACTION_THINKING"`
}

// ResolvedLLM is a fully-resolved LLM configuration for a single pipeline.
// All fields are populated -- no empty-means-inherit semantics.
type ResolvedLLM struct {
	Provider     string
	BaseURL      string
	Model        string
	APIKey       string //nolint:gosec // resolved config field, not a hardcoded credential
	ExtraContext string
	Timeout      time.Duration
	Thinking     string
}

// TimeoutDuration returns the parsed LLM timeout, falling back to
// DefaultLLMTimeout if the value is empty or unparseable.
func (l LLM) TimeoutDuration() time.Duration {
	if l.Timeout == "" {
		return DefaultLLMTimeout
	}
	d, err := time.ParseDuration(l.Timeout)
	if err != nil {
		return DefaultLLMTimeout
	}
	return d
}

// ChatConfig returns the fully-resolved LLM configuration for the chat
// pipeline. Fields from [llm.chat] override the base [llm] values.
func (l LLM) ChatConfig() ResolvedLLM {
	return l.resolvePipeline(
		l.Chat.Provider, l.Chat.BaseURL, l.Chat.Model,
		l.Chat.APIKey, l.Chat.Timeout, l.Chat.Thinking,
	)
}

// ExtractionConfig returns the fully-resolved LLM configuration for the
// extraction pipeline. Fields from [llm.extraction] override the base
// [llm] values.
func (l LLM) ExtractionConfig() ResolvedLLM {
	return l.resolvePipeline(
		l.Extraction.Provider, l.Extraction.BaseURL, l.Extraction.Model,
		l.Extraction.APIKey, l.Extraction.Timeout, l.Extraction.Thinking,
	)
}

// resolvePipeline merges per-pipeline overrides with the base LLM config.
func (l LLM) resolvePipeline(
	provider, baseURL, model, apiKey, timeout, thinking string,
) ResolvedLLM {
	resolvedProvider := coalesce(provider, l.Provider)
	resolvedBaseURL := coalesce(baseURL, l.BaseURL)
	resolvedAPIKey := coalesce(apiKey, l.APIKey)

	// Re-detect provider when the pipeline has its own connection
	// settings but no explicit provider.
	if provider == "" && (baseURL != "" || apiKey != "") {
		resolvedProvider = detectProvider(resolvedBaseURL, resolvedAPIKey)
	}

	return ResolvedLLM{
		Provider:     resolvedProvider,
		BaseURL:      resolvedBaseURL,
		Model:        coalesce(model, l.Model),
		APIKey:       resolvedAPIKey,
		ExtraContext: l.ExtraContext,
		Timeout:      parseDurationOr(coalesce(timeout, l.Timeout), DefaultLLMTimeout),
		Thinking:     coalesce(thinking, l.Thinking),
	}
}

func coalesce(override, base string) string {
	if override != "" {
		return override
	}
	return base
}

func parseDurationOr(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}

// normalizeBaseURL strips a trailing slash and /v1 suffix from a base URL.
func normalizeBaseURL(u string) string {
	u = strings.TrimRight(u, "/")
	u = strings.TrimSuffix(u, "/v1")
	return u
}

// Documents holds settings for document attachments.
type Documents struct {
	// MaxFileSize is the largest file that can be imported as a document
	// attachment. Accepts unitized strings ("50 MiB") or bare integers
	// (bytes). Default: 50 MiB.
	MaxFileSize ByteSize `toml:"max_file_size" env:"MICASA_MAX_DOCUMENT_SIZE"`

	// CacheTTL is the preferred cache lifetime setting. Accepts unitized
	// strings ("30d", "720h") or bare integers (seconds). Default: 30d.
	CacheTTL *Duration `toml:"cache_ttl,omitempty" env:"MICASA_CACHE_TTL"`

	// CacheTTLDays is deprecated; use CacheTTL instead. Kept for backward
	// compatibility. Bare integer interpreted as days.
	CacheTTLDays *int `toml:"cache_ttl_days,omitempty" env:"MICASA_CACHE_TTL_DAYS"`
}

// CacheTTLDuration returns the resolved cache TTL as a time.Duration.
// CacheTTL takes precedence over CacheTTLDays. Returns 0 to disable.
func (d Documents) CacheTTLDuration() time.Duration {
	if d.CacheTTL != nil {
		return d.CacheTTL.Duration
	}
	if d.CacheTTLDays != nil {
		return time.Duration(*d.CacheTTLDays) * 24 * time.Hour
	}
	return DefaultCacheTTL
}

// Extraction holds settings for the document extraction pipeline
// (LLM-powered structured pre-fill).
type Extraction struct {
	// Model overrides llm.model for extraction. Extraction wants a small,
	// fast model optimized for structured JSON output. Defaults to the
	// chat model if empty.
	Model string `toml:"model" env:"MICASA_EXTRACTION_MODEL"`

	// MaxExtractPages is the maximum number of pages for async extraction of scanned
	// documents. Front-loaded info (specs, warranty) is typically in the
	// first pages. Default: 20.
	MaxExtractPages int `toml:"max_extract_pages" env:"MICASA_MAX_EXTRACT_PAGES"`

	// Enabled controls whether LLM-powered extraction runs when a document
	// is uploaded. When disabled, no structured data is extracted -- OCR and
	// pdftotext are internal pipeline steps, not standalone features. Default: true.
	Enabled *bool `toml:"enabled,omitempty" env:"MICASA_EXTRACTION_ENABLED"`

	// TextTimeout is the maximum time to wait for pdftotext. Go duration
	// string, e.g. "30s", "1m". Default: "30s".
	TextTimeout string `toml:"text_timeout" env:"MICASA_TEXT_TIMEOUT"`

	// LLMTimeout is the maximum time to wait for the LLM extraction
	// inference step. Go duration string, e.g. "2m", "90s". Default: "2m".
	LLMTimeout string `toml:"llm_timeout" env:"MICASA_EXTRACTION_LLM_TIMEOUT"`

	// Thinking controls the model's reasoning effort level for extraction.
	// Supported values: none, low, medium, high, auto.
	// Empty string = don't send (server default). Default: empty.
	Thinking string `toml:"thinking,omitempty" env:"MICASA_EXTRACTION_THINKING"`
}

// IsEnabled returns whether LLM extraction is enabled. Defaults to true
// when the field is unset.
func (e Extraction) IsEnabled() bool {
	if e.Enabled != nil {
		return *e.Enabled
	}
	return true
}

// TextTimeoutDuration returns the parsed text extraction timeout, falling
// back to DefaultTextTimeout if the value is empty or unparseable.
func (e Extraction) TextTimeoutDuration() time.Duration {
	if e.TextTimeout == "" {
		return DefaultTextTimeout
	}
	d, err := time.ParseDuration(e.TextTimeout)
	if err != nil {
		return DefaultTextTimeout
	}
	return d
}

// LLMTimeoutDuration returns the parsed LLM extraction timeout, falling
// back to DefaultLLMExtractionTimeout if the value is empty or unparseable.
func (e Extraction) LLMTimeoutDuration() time.Duration {
	if e.LLMTimeout == "" {
		return DefaultLLMExtractionTimeout
	}
	d, err := time.ParseDuration(e.LLMTimeout)
	if err != nil {
		return DefaultLLMExtractionTimeout
	}
	return d
}

// ThinkingLevel returns the reasoning effort string for extraction.
// Returns empty string when unset (server default).
func (e Extraction) ThinkingLevel() string {
	return e.Thinking
}

// ResolvedModel returns the extraction model, falling back to the given
// chat model if no extraction-specific model is configured.
func (e Extraction) ResolvedModel(chatModel string) string {
	if e.Model != "" {
		return e.Model
	}
	return chatModel
}

const (
	DefaultBaseURL              = "http://localhost:11434"
	DefaultModel                = "qwen3"
	DefaultProvider             = "ollama"
	DefaultLLMTimeout           = 5 * time.Second
	DefaultLLMExtractionTimeout = 2 * time.Minute
	DefaultCacheTTL             = 30 * 24 * time.Hour // 30 days
	DefaultMaxExtractPages      = 20
	DefaultTextTimeout          = 30 * time.Second
	configRelPath               = "micasa/config.toml"
)

// defaults returns a Config with all default values populated.
func defaults() Config {
	return Config{
		LLM: LLM{
			BaseURL: DefaultBaseURL,
			Model:   DefaultModel,
			Timeout: DefaultLLMTimeout.String(),
		},
		Documents: Documents{
			MaxFileSize: ByteSize(data.MaxDocumentSize),
		},
		Extraction: Extraction{
			MaxExtractPages: DefaultMaxExtractPages,
		},
	}
}

// Path returns the expected config file path (XDG_CONFIG_HOME/micasa/config.toml).
func Path() string {
	return filepath.Join(xdg.ConfigHome, configRelPath)
}

// Load reads the TOML config file from the default path if it exists, falls
// back to defaults for any unset fields, and applies environment variable
// overrides last.
func Load() (Config, error) {
	return LoadFromPath(Path())
}

// LoadFromPath reads the TOML config file at the given path if it exists,
// falls back to defaults for any unset fields, and applies environment
// variable overrides last.
func LoadFromPath(path string) (Config, error) {
	cfg := defaults()

	if _, err := os.Stat(path); err == nil {
		md, err := toml.DecodeFile(path, &cfg)
		if err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
		migrateRenamedKeys(&cfg, md, path)
	}

	migrateRenamedEnvVars(&cfg)

	if err := applyEnvOverrides(&cfg); err != nil {
		return cfg, err
	}

	// Normalize base URLs: strip trailing slash and /v1 suffix --
	// providers handle their own path construction.
	cfg.LLM.BaseURL = normalizeBaseURL(cfg.LLM.BaseURL)
	cfg.LLM.Chat.BaseURL = normalizeBaseURL(cfg.LLM.Chat.BaseURL)
	cfg.LLM.Extraction.BaseURL = normalizeBaseURL(cfg.LLM.Extraction.BaseURL)

	// Auto-detect provider from base_url and api_key when not explicitly set.
	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = detectProvider(cfg.LLM.BaseURL, cfg.LLM.APIKey)
	}

	// Validate base provider name.
	if !validProvider(cfg.LLM.Provider) {
		return cfg, fmt.Errorf(
			"llm.provider: unknown provider %q -- supported: %s",
			cfg.LLM.Provider, strings.Join(providerNames(), ", "),
		)
	}

	// Validate per-pipeline provider overrides.
	if cfg.LLM.Chat.Provider != "" && !validProvider(cfg.LLM.Chat.Provider) {
		return cfg, fmt.Errorf(
			"llm.chat.provider: unknown provider %q -- supported: %s",
			cfg.LLM.Chat.Provider, strings.Join(providerNames(), ", "),
		)
	}
	if cfg.LLM.Extraction.Provider != "" && !validProvider(cfg.LLM.Extraction.Provider) {
		return cfg, fmt.Errorf(
			"llm.extraction.provider: unknown provider %q -- supported: %s",
			cfg.LLM.Extraction.Provider, strings.Join(providerNames(), ", "),
		)
	}

	// Validate thinking levels.
	if cfg.LLM.Thinking != "" && !validThinkingLevel(cfg.LLM.Thinking) {
		return cfg, fmt.Errorf(
			"llm.thinking: invalid level %q -- supported: none, low, medium, high, auto",
			cfg.LLM.Thinking,
		)
	}
	if cfg.LLM.Chat.Thinking != "" && !validThinkingLevel(cfg.LLM.Chat.Thinking) {
		return cfg, fmt.Errorf(
			"llm.chat.thinking: invalid level %q -- supported: none, low, medium, high, auto",
			cfg.LLM.Chat.Thinking,
		)
	}
	if cfg.LLM.Extraction.Thinking != "" && !validThinkingLevel(cfg.LLM.Extraction.Thinking) {
		return cfg, fmt.Errorf(
			"llm.extraction.thinking: invalid level %q -- supported: none, low, medium, high, auto",
			cfg.LLM.Extraction.Thinking,
		)
	}
	if cfg.Extraction.Thinking != "" && !validThinkingLevel(cfg.Extraction.Thinking) {
		return cfg, fmt.Errorf(
			"extraction.thinking: invalid level %q -- supported: none, low, medium, high, auto",
			cfg.Extraction.Thinking,
		)
	}

	// Validate timeouts.
	if cfg.LLM.Timeout != "" {
		d, err := time.ParseDuration(cfg.LLM.Timeout)
		if err != nil {
			return cfg, fmt.Errorf(
				"llm.timeout: invalid duration %q -- use Go syntax like \"5s\" or \"10s\"",
				cfg.LLM.Timeout,
			)
		}
		if d <= 0 {
			return cfg, fmt.Errorf("llm.timeout must be positive, got %s", cfg.LLM.Timeout)
		}
	}
	if err := validateOverrideTimeout(cfg.LLM.Chat.Timeout, "llm.chat"); err != nil {
		return cfg, err
	}
	if err := validateOverrideTimeout(cfg.LLM.Extraction.Timeout, "llm.extraction"); err != nil {
		return cfg, err
	}

	if cfg.Documents.MaxFileSize == 0 {
		return cfg, fmt.Errorf("documents.max_file_size must be positive")
	}

	if cfg.Documents.CacheTTL != nil && cfg.Documents.CacheTTLDays != nil {
		return cfg, fmt.Errorf(
			"documents.cache_ttl and documents.cache_ttl_days cannot both be set -- " +
				"remove cache_ttl_days (deprecated) and use cache_ttl instead",
		)
	}

	if cfg.Documents.CacheTTLDays != nil {
		deprecated := "documents.cache_ttl_days"
		replacement := "documents.cache_ttl"
		if os.Getenv("MICASA_CACHE_TTL_DAYS") != "" {
			deprecated = "MICASA_CACHE_TTL_DAYS"
			replacement = "MICASA_CACHE_TTL"
		}
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
			"%s is deprecated -- use %s (e.g. \"30d\") instead",
			deprecated, replacement,
		))
		if *cfg.Documents.CacheTTLDays < 0 {
			return cfg, fmt.Errorf(
				"documents.cache_ttl_days must be non-negative, got %d",
				*cfg.Documents.CacheTTLDays,
			)
		}
	}

	if cfg.Documents.CacheTTL != nil && cfg.Documents.CacheTTL.Duration < 0 {
		return cfg, fmt.Errorf(
			"documents.cache_ttl must be non-negative, got %s",
			cfg.Documents.CacheTTL.Duration,
		)
	}

	if cfg.Extraction.TextTimeout != "" {
		d, err := time.ParseDuration(cfg.Extraction.TextTimeout)
		if err != nil {
			return cfg, fmt.Errorf(
				"extraction.text_timeout: invalid duration %q -- use Go syntax like \"30s\" or \"1m\"",
				cfg.Extraction.TextTimeout,
			)
		}
		if d <= 0 {
			return cfg, fmt.Errorf(
				"extraction.text_timeout must be positive, got %s",
				cfg.Extraction.TextTimeout,
			)
		}
	}

	if cfg.Extraction.LLMTimeout != "" {
		d, err := time.ParseDuration(cfg.Extraction.LLMTimeout)
		if err != nil {
			return cfg, fmt.Errorf(
				"extraction.llm_timeout: invalid duration %q -- use Go syntax like \"2m\" or \"90s\"",
				cfg.Extraction.LLMTimeout,
			)
		}
		if d <= 0 {
			return cfg, fmt.Errorf(
				"extraction.llm_timeout must be positive, got %s",
				cfg.Extraction.LLMTimeout,
			)
		}
	}

	if cfg.Extraction.MaxExtractPages < 0 {
		return cfg, fmt.Errorf(
			"extraction.max_extract_pages must be non-negative, got %d",
			cfg.Extraction.MaxExtractPages,
		)
	}
	if cfg.Extraction.MaxExtractPages == 0 {
		cfg.Extraction.MaxExtractPages = DefaultMaxExtractPages
	}

	return cfg, nil
}

// applyEnvOverrides walks the Config struct and applies environment variable
// overrides for every field carrying an `env` struct tag. Returns an error
// if an env var is set but its value cannot be parsed for the target type.
func applyEnvOverrides(cfg *Config) error {
	return walkEnvFields(reflect.ValueOf(cfg).Elem())
}

func walkEnvFields(v reflect.Value) error {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		fv := v.Field(i)

		envVar := f.Tag.Get("env")
		if envVar == "" {
			// No env tag -- recurse into nested config sections.
			if fv.Kind() == reflect.Struct && tomlTagName(f) != "" {
				if err := walkEnvFields(fv); err != nil {
					return err
				}
			}
			continue
		}
		val := os.Getenv(envVar)
		if val == "" {
			continue
		}
		if err := setFieldFromEnv(fv, envVar, val); err != nil {
			return err
		}
	}
	return nil
}

// setFieldFromEnv assigns a string value from an environment variable to a
// reflected config field, converting types as needed.
func setFieldFromEnv(fv reflect.Value, envVar, val string) error {
	switch fv.Kind() { //nolint:exhaustive // only config-relevant kinds
	case reflect.String:
		fv.SetString(val)
	case reflect.Int:
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("%s=%q: expected integer", envVar, val)
		}
		fv.SetInt(int64(n))
	case reflect.Uint64:
		parsed, err := ParseByteSize(val)
		if err != nil {
			return fmt.Errorf("%s=%q: expected byte size (e.g. \"50 MiB\" or 1048576)", envVar, val)
		}
		fv.SetUint(uint64(parsed))
	case reflect.Pointer:
		return setFieldFromEnvPtr(fv, envVar, val)
	}
	return nil
}

func setFieldFromEnvPtr(fv reflect.Value, envVar, val string) error {
	switch fv.Type().Elem().Kind() {
	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("%s=%q: expected true or false", envVar, val)
		}
		fv.Set(reflect.ValueOf(&b))
	case reflect.Int:
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("%s=%q: expected integer", envVar, val)
		}
		fv.Set(reflect.ValueOf(&n))
	default:
		if fv.Type() == reflect.TypeFor[*Duration]() {
			parsed, err := ParseDuration(val)
			if err != nil {
				return fmt.Errorf(
					"%s=%q: expected duration (e.g. \"30d\", \"720h\", or seconds)",
					envVar, val,
				)
			}
			d := Duration{parsed}
			fv.Set(reflect.ValueOf(&d))
			return nil
		}
		return fmt.Errorf("%s: unsupported pointer type %s", envVar, fv.Type())
	}
	return nil
}

// EnvVars returns a mapping from environment variable names to their
// dot-delimited config keys, derived from the `env` struct tags.
func EnvVars() map[string]string {
	m := make(map[string]string)
	collectEnvVars(reflect.TypeOf(Config{}), "", m)
	return m
}

func collectEnvVars(t reflect.Type, prefix string, m map[string]string) {
	for i := range t.NumField() {
		f := t.Field(i)
		toml := tomlTagName(f)
		if toml == "" {
			continue
		}
		key := toml
		if prefix != "" {
			key = prefix + "." + toml
		}
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		if envVar := f.Tag.Get("env"); envVar != "" {
			m[envVar] = key
		} else if ft.Kind() == reflect.Struct && ft.NumField() > 0 && tomlTagName(ft.Field(0)) != "" {
			collectEnvVars(ft, key, m)
		}
	}
}

// Get resolves a dot-delimited config key to its string representation.
// Keys mirror the TOML structure (e.g. "llm.model", "documents.max_file_size").
func (c Config) Get(key string) (string, error) {
	return getField(reflect.ValueOf(c), key)
}

// getField walks a struct value using dot-delimited TOML tag names and returns
// the leaf value as a string.
func getField(v reflect.Value, key string) (string, error) {
	parts := strings.SplitN(key, ".", 2)
	tag := parts[0]

	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		tomlTag := tomlTagName(f)
		if tomlTag != tag {
			continue
		}
		fv := v.Field(i)

		// Recurse into nested structs.
		if len(parts) == 2 {
			if fv.Kind() == reflect.Struct {
				return getField(fv, parts[1])
			}
			if fv.Kind() == reflect.Pointer && !fv.IsNil() && fv.Elem().Kind() == reflect.Struct {
				return getField(fv.Elem(), parts[1])
			}
			return "", fmt.Errorf("key %q: %q is not a section", key, tag)
		}

		// Leaf field — format the value.
		return formatValue(fv)
	}
	return "", fmt.Errorf("unknown config key %q", key)
}

// tomlTagName extracts the TOML field name from a struct tag, ignoring
// options like "omitempty".
func tomlTagName(f reflect.StructField) string {
	tag := f.Tag.Get("toml")
	if tag == "" || tag == "-" {
		return ""
	}
	if i := strings.IndexByte(tag, ','); i >= 0 {
		return tag[:i]
	}
	return tag
}

// formatValue converts a reflected config field to its string representation.
func formatValue(v reflect.Value) (string, error) {
	// Dereference pointers.
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "", nil
		}
		v = v.Elem()
	}

	// Handle known types by interface.
	iface := v.Interface()
	switch val := iface.(type) {
	case ByteSize:
		return strconv.FormatUint(val.Bytes(), 10), nil
	case Duration:
		return val.String(), nil
	case fmt.Stringer:
		return val.String(), nil
	}

	switch v.Kind() {
	case reflect.String:
		return v.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.Slice:
		var lines []string
		for i := range v.Len() {
			s, err := formatValue(v.Index(i))
			if err != nil {
				return "", err
			}
			lines = append(lines, s)
		}
		return strings.Join(lines, "\n"), nil
	default:
		return fmt.Sprintf("%v", iface), nil
	}
}

// Keys returns the sorted list of valid dot-delimited config key names
// by reflecting on the Config struct's TOML tags.
func Keys() []string {
	keys := collectKeys(reflect.TypeOf(Config{}), "")
	slices.Sort(keys)
	return keys
}

func collectKeys(t reflect.Type, prefix string) []string {
	var keys []string
	for i := range t.NumField() {
		f := t.Field(i)
		tag := tomlTagName(f)
		if tag == "" {
			continue
		}
		full := tag
		if prefix != "" {
			full = prefix + "." + tag
		}
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() != "" {
			// Nested config section — but only recurse into our own types,
			// not stdlib types like time.Duration.
			if _, isBytes := reflect.New(ft).Interface().(*ByteSize); isBytes {
				keys = append(keys, full)
			} else if _, isDur := reflect.New(ft).Interface().(*Duration); isDur {
				keys = append(keys, full)
			} else if ft.NumField() > 0 && tomlTagName(ft.Field(0)) != "" {
				keys = append(keys, collectKeys(ft, full)...)
			} else {
				keys = append(keys, full)
			}
		} else {
			keys = append(keys, full)
		}
	}
	return keys
}

// validateOverrideTimeout validates a per-pipeline timeout string.
func validateOverrideTimeout(timeout, prefix string) error {
	if timeout == "" {
		return nil
	}
	d, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf(
			"%s.timeout: invalid duration %q -- use Go syntax like \"5s\" or \"10s\"",
			prefix, timeout,
		)
	}
	if d <= 0 {
		return fmt.Errorf("%s.timeout must be positive, got %s", prefix, timeout)
	}
	return nil
}

// migrateRenamedKeys checks for deprecated TOML keys and migrates their
// values to the new field names, appending deprecation warnings.
func migrateRenamedKeys(cfg *Config, md toml.MetaData, path string) {
	// extraction.max_ocr_pages -> extraction.max_extract_pages (v1.47)
	if md.IsDefined("extraction", "max_ocr_pages") {
		var raw struct {
			Extraction struct {
				MaxOCRPages int `toml:"max_ocr_pages"`
			} `toml:"extraction"`
		}
		if _, err := toml.DecodeFile(path, &raw); err == nil && raw.Extraction.MaxOCRPages > 0 {
			cfg.Extraction.MaxExtractPages = raw.Extraction.MaxOCRPages
		}
		cfg.Warnings = append(cfg.Warnings,
			"extraction.max_ocr_pages is deprecated -- use extraction.max_extract_pages instead",
		)
	}

	// extraction.model -> llm.extraction.model (v1.59)
	if md.IsDefined("extraction", "model") && !md.IsDefined("llm", "extraction", "model") {
		cfg.LLM.Extraction.Model = cfg.Extraction.Model
		cfg.Warnings = append(cfg.Warnings,
			"extraction.model is deprecated -- use llm.extraction.model instead",
		)
	}

	// extraction.thinking -> llm.extraction.thinking (v1.59)
	if md.IsDefined("extraction", "thinking") && !md.IsDefined("llm", "extraction", "thinking") {
		cfg.LLM.Extraction.Thinking = cfg.Extraction.Thinking
		cfg.Warnings = append(cfg.Warnings,
			"extraction.thinking is deprecated -- use llm.extraction.thinking instead",
		)
	}
}

// migrateRenamedEnvVars checks for deprecated environment variables and
// migrates their values directly into the config struct, appending deprecation
// warnings. Does not modify the process environment.
func migrateRenamedEnvVars(cfg *Config) {
	// MICASA_MAX_OCR_PAGES -> MICASA_MAX_EXTRACT_PAGES (v1.47)
	if val := os.Getenv("MICASA_MAX_OCR_PAGES"); val != "" {
		if os.Getenv("MICASA_MAX_EXTRACT_PAGES") == "" {
			n, err := strconv.Atoi(val)
			if err == nil && n > 0 {
				cfg.Extraction.MaxExtractPages = n
			}
			cfg.Warnings = append(cfg.Warnings,
				"MICASA_MAX_OCR_PAGES is deprecated -- use MICASA_MAX_EXTRACT_PAGES instead",
			)
		}
	}

	// MICASA_EXTRACTION_MODEL -> MICASA_LLM_EXTRACTION_MODEL (v1.59)
	if val := os.Getenv("MICASA_EXTRACTION_MODEL"); val != "" {
		if os.Getenv("MICASA_LLM_EXTRACTION_MODEL") == "" {
			cfg.LLM.Extraction.Model = val
			cfg.Warnings = append(cfg.Warnings,
				"MICASA_EXTRACTION_MODEL is deprecated -- use MICASA_LLM_EXTRACTION_MODEL instead",
			)
		}
	}

	// MICASA_EXTRACTION_THINKING -> MICASA_LLM_EXTRACTION_THINKING (v1.59)
	if val := os.Getenv("MICASA_EXTRACTION_THINKING"); val != "" {
		if os.Getenv("MICASA_LLM_EXTRACTION_THINKING") == "" {
			cfg.LLM.Extraction.Thinking = val
			cfg.Warnings = append(
				cfg.Warnings,
				"MICASA_EXTRACTION_THINKING is deprecated -- use MICASA_LLM_EXTRACTION_THINKING instead",
			)
		}
	}
}

// providers lists every supported provider name.
var providers = []string{
	"ollama",
	"anthropic",
	"openai",
	"openrouter",
	"deepseek",
	"gemini",
	"groq",
	"mistral",
	"llamacpp",
	"llamafile",
}

func providerNames() []string { return providers }

func validProvider(name string) bool {
	for _, p := range providers {
		if p == name {
			return true
		}
	}
	return false
}

var thinkingLevels = map[string]bool{
	"none": true, "low": true, "medium": true, "high": true, "auto": true,
}

func validThinkingLevel(level string) bool {
	return thinkingLevels[level]
}

// detectProvider infers the provider from the base URL and API key.
func detectProvider(baseURL, apiKey string) string {
	if apiKey != "" {
		lower := strings.ToLower(baseURL)
		switch {
		case strings.Contains(lower, "anthropic"):
			return "anthropic"
		case strings.Contains(lower, "openrouter"):
			return "openrouter"
		case strings.Contains(lower, "deepseek"):
			return "deepseek"
		case strings.Contains(lower, "googleapis") || strings.Contains(lower, "generativelanguage"):
			return "gemini"
		case strings.Contains(lower, "groq"):
			return "groq"
		case strings.Contains(lower, "mistral"):
			return "mistral"
		case strings.Contains(lower, "openai"):
			return "openai"
		default:
			// API key but unrecognized URL -- assume OpenAI-compatible.
			return "openai"
		}
	}
	return DefaultProvider
}

// ExampleTOML returns a commented config file suitable for writing as a
// starter config. Not written automatically -- offered to the user on demand.
func ExampleTOML() string {
	return `# micasa configuration
# Place this file at: ` + Path() + `

[llm]
# Base LLM settings. Both chat and extraction pipelines inherit these
# unless overridden in [llm.chat] or [llm.extraction] below.

# LLM provider. Supported: ollama, anthropic, openai, openrouter,
# deepseek, gemini, groq, mistral, llamacpp, llamafile.
# Auto-detected from base_url and api_key when not set.
# provider = "ollama"

# Base URL for the provider's API. No /v1 suffix needed.
# Ollama (default): http://localhost:11434
# llama.cpp:        http://localhost:8080
# LM Studio:        http://localhost:1234
# base_url = "` + DefaultBaseURL + `"

# Model name passed in chat requests.
model = "` + DefaultModel + `"

# API key for cloud providers.
# Not needed for local servers like Ollama.
# api_key = ""

# Optional: custom context appended to all system prompts.
# Use this to inject domain-specific details about your house, region, etc.
# extra_context = "My house is a 1920s craftsman in Portland, OR."

# Timeout for quick LLM server operations (ping, model listing).
# Go duration syntax: "5s", "10s", "500ms", etc. Default: "5s".
# Increase if your LLM server is slow to respond.
# timeout = "5s"

# Model reasoning effort level. Supported: none, low, medium, high, auto.
# Empty = don't send (server default).
# thinking = "medium"

# [llm.chat]
# Per-pipeline overrides for the chat (NL-to-SQL) pipeline.
# Any field here takes precedence over the base [llm] value.
# provider = "anthropic"
# base_url = "https://api.anthropic.com"
# model = "claude-sonnet-4-5-20250929"
# api_key = "sk-ant-..."
# timeout = "10s"
# thinking = "medium"

# [llm.extraction]
# Per-pipeline overrides for document extraction.
# Extraction wants a fast model optimized for structured JSON output.
# provider = "anthropic"
# base_url = "https://api.anthropic.com"
# model = "claude-haiku-3-5-20241022"
# api_key = "sk-ant-..."
# timeout = "15s"
# thinking = "low"

[documents]
# Maximum file size for document imports. Accepts unitized strings or bare
# integers (bytes). Default: 50 MiB.
# max_file_size = "50 MiB"

# How long to keep extracted document cache entries before evicting on startup.
# Accepts "30d", "720h", or bare integers (seconds). Set to "0s" to disable.
# Default: 30d.
# cache_ttl = "30d"

[extraction]
# Timeout for pdftotext. Go duration syntax: "30s", "1m", etc. Default: "30s".
# Increase if you routinely process very large PDFs.
# text_timeout = "30s"

# Timeout for LLM extraction inference. Go duration syntax: "2m", "90s", etc.
# Default: "2m". Increase for slow local models or complex documents.
# llm_timeout = "2m"

# Maximum pages for async extraction of scanned documents. Default: 20.
# max_extract_pages = 20

# Set to false to disable LLM-powered extraction even when LLM is configured.
# When disabled, no structured data is extracted from documents.
# enabled = true

[locale]
# ISO 4217 currency code. Stored in the database on first run; after that the
# database value is authoritative. Override: MICASA_CURRENCY env var.
# Auto-detected from system locale if not set. Default: USD.
# currency = "USD"
`
}
