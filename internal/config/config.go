// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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
	Currency string `toml:"currency"`
}

// LLM holds settings for the LLM inference backend.
type LLM struct {
	// Provider selects which LLM provider to use. Supported values:
	// ollama, anthropic, openai, openrouter, deepseek, gemini, groq,
	// mistral, llamacpp, llamafile. Auto-detected when empty.
	Provider string `toml:"provider"`

	// BaseURL is the base URL for the provider's API.
	// Default varies by provider (e.g. http://localhost:11434 for Ollama).
	// No /v1 suffix needed -- the provider handles path construction.
	BaseURL string `toml:"base_url" default:"http://localhost:11434"`

	// Model is the model identifier passed in chat requests.
	// Default: qwen3
	Model string `toml:"model" default:"qwen3"`

	// APIKey is the authentication credential. Required for cloud
	// providers (Anthropic, OpenAI, OpenRouter, etc.). Leave empty for local
	// servers like Ollama that don't require authentication.
	APIKey string `toml:"api_key"` //nolint:gosec // config field, not a hardcoded credential

	// ExtraContext is custom text appended to all system prompts.
	// Useful for domain-specific details: house style, location, etc.
	// Currency is handled by [locale] section. Optional; defaults to empty.
	ExtraContext string `toml:"extra_context"`

	// Timeout is the maximum time for a single LLM response (including
	// streaming). Go duration string, e.g. "5m", "10m". Default: "5m".
	// Quick operations (ping, model listing) use a shorter fixed deadline.
	Timeout string `toml:"timeout" default:"5m"`

	// Thinking controls the model's reasoning effort level. Supported values:
	// none, low, medium, high, auto. Empty string = don't send (server default).
	Thinking string `toml:"thinking,omitempty"`

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
	Provider string `toml:"provider"`
	BaseURL  string `toml:"base_url"`
	Model    string `toml:"model"`
	APIKey   string `toml:"api_key"` //nolint:gosec // config field, not a hardcoded credential
	Timeout  string `toml:"timeout"`
	Thinking string `toml:"thinking,omitempty"`
}

// LLMExtractionOverride holds optional per-pipeline overrides for the
// extraction pipeline. Empty fields inherit from the parent [llm] section.
type LLMExtractionOverride struct {
	Provider string `toml:"provider"`
	BaseURL  string `toml:"base_url"`
	Model    string `toml:"model"`
	APIKey   string `toml:"api_key"` //nolint:gosec // config field, not a hardcoded credential
	Timeout  string `toml:"timeout"`
	Thinking string `toml:"thinking,omitempty"`
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
	MaxFileSize ByteSize `toml:"max_file_size" default:"52428800"`

	// CacheTTL is the preferred cache lifetime setting. Accepts unitized
	// strings ("30d", "720h") or bare integers (seconds). Default: 30d.
	CacheTTL *Duration `toml:"cache_ttl,omitempty"`

	// CacheTTLDays is deprecated; use CacheTTL instead. Kept for backward
	// compatibility. Bare integer interpreted as days.
	CacheTTLDays *int `toml:"cache_ttl_days,omitempty"`

	// FilePickerDir is the starting directory for the document file picker.
	// Default: the system Downloads folder (e.g. ~/Downloads).
	FilePickerDir string `toml:"file_picker_dir"`
}

// ResolvedFilePickerDir returns the starting directory for the file picker.
// Uses the configured value if set and the directory exists, otherwise falls
// back to the system Downloads folder, then the current working directory.
func (d Documents) ResolvedFilePickerDir() string {
	if d.FilePickerDir != "" {
		if info, err := os.Stat(d.FilePickerDir); err == nil && info.IsDir() {
			return d.FilePickerDir
		}
	}
	if dir := xdg.UserDirs.Download; dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	if dir, err := os.Getwd(); err == nil {
		return dir
	}
	return "."
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
	Model string `toml:"model"`

	// MaxPages is the maximum number of pages for async extraction of scanned
	// documents. 0 means no limit (all pages). Default: 0.
	MaxPages int `toml:"max_pages"`

	// Enable controls whether LLM-powered structured extraction runs when
	// a document is uploaded. When disabled, no structured data is extracted
	// from documents. OCR and pdftotext still run independently (controlled
	// by [extraction.ocr]) to populate the document's stored text. Default: true.
	Enable *bool `toml:"enable,omitempty"`

	// Enabled is the deprecated spelling; migrated to Enable on load.
	Enabled *bool `toml:"enabled,omitempty"`

	// LLMTimeout is the maximum time to wait for the LLM extraction
	// inference step. Go duration string, e.g. "5m", "90s". Default: "5m".
	LLMTimeout string `toml:"llm_timeout"`

	// Thinking controls the model's reasoning effort level for extraction.
	// Supported values: none, low, medium, high, auto.
	// Empty string = don't send (server default). Default: empty.
	Thinking string `toml:"thinking,omitempty"`

	// OCR holds settings for the OCR sub-pipeline.
	OCR OCR `toml:"ocr" doc:"OCR sub-pipeline. Requires tesseract and pdftocairo."`
}

// OCR holds settings for the OCR sub-pipeline within extraction.
type OCR struct {
	// Enable controls whether OCR runs on uploaded documents.
	// When disabled, scanned pages and images produce no text. Default: true.
	Enable *bool `toml:"enable,omitempty"`

	// ConfidenceThreshold is the minimum tesseract word confidence (0-100)
	// to keep in OCR output. Words below this threshold are dropped.
	// 0 means no filtering (all words kept). Default: 0.
	ConfidenceThreshold int `toml:"confidence_threshold"`

	// TSV sends spatial layout annotations (line-level bounding boxes
	// and confidence scores) from tesseract OCR to the LLM alongside text.
	// This helps extraction accuracy for invoices and forms with tabular
	// data, at ~2x token overhead. Default: true.
	TSV *bool `toml:"tsv,omitempty"`

	// SpatialConfThresholdVal is the confidence threshold (0-100) below
	// which OCR confidence annotations are included in spatial layout
	// output. Lines with min confidence >= this value omit the score to
	// save tokens. Set to 0 to never show confidence. Default: 70.
	SpatialConfThresholdVal *int `toml:"spatial_conf_threshold,omitempty"`
}

// IsEnabled returns whether LLM extraction is enabled. Defaults to true
// when the field is unset.
func (e Extraction) IsEnabled() bool {
	if e.Enable != nil {
		return *e.Enable
	}
	return true
}

// IsOCREnabled returns whether OCR is enabled. Defaults to true when
// the field is unset.
func (e Extraction) IsOCREnabled() bool {
	if e.OCR.Enable != nil {
		return *e.OCR.Enable
	}
	return true
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

// IsOCRTSV returns whether spatial layout annotations from tesseract OCR
// should be sent to the LLM alongside text. Defaults to true.
func (e Extraction) IsOCRTSV() bool {
	if e.OCR.TSV != nil {
		return *e.OCR.TSV
	}
	return true
}

// OCRSpatialConfThreshold returns the confidence threshold below which
// OCR confidence annotations appear in spatial output. Defaults to 70.
func (e Extraction) OCRSpatialConfThreshold() int {
	if e.OCR.SpatialConfThresholdVal != nil {
		return *e.OCR.SpatialConfThresholdVal
	}
	return 70
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
	DefaultLLMTimeout           = 5 * time.Minute
	DefaultLLMExtractionTimeout = DefaultLLMTimeout
	DefaultCacheTTL             = 30 * 24 * time.Hour // 30 days
	DefaultMaxPages             = 0
	configRelPath               = "micasa/config.toml"
)

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
	var cfg Config
	data.ApplyDefaults(&cfg)

	if _, err := os.Stat(path); err == nil {
		md, err := toml.DecodeFile(path, &cfg)
		if err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
		migrateRenamedKeys(&cfg, md, path)
	}

	envOverrides := migrateRenamedEnvVars(&cfg)

	if err := applyEnvOverrides(&cfg, envOverrides); err != nil {
		return cfg, err
	}

	// Clear deprecated Enabled again: applyEnvOverrides may have
	// repopulated it from MICASA_EXTRACTION_ENABLED.
	if cfg.Extraction.Enabled != nil {
		if cfg.Extraction.Enable == nil {
			cfg.Extraction.Enable = cfg.Extraction.Enabled
		}
		cfg.Extraction.Enabled = nil
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
				"llm.timeout: invalid duration %q -- use Go syntax like \"5m\" or \"10m\"",
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
		cfg.Warnings = append(
			cfg.Warnings,
			"documents.cache_ttl_days is deprecated -- use documents.cache_ttl (e.g. \"30d\") instead",
		)
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

	if cfg.Extraction.LLMTimeout != "" {
		d, err := time.ParseDuration(cfg.Extraction.LLMTimeout)
		if err != nil {
			return cfg, fmt.Errorf(
				"extraction.llm_timeout: invalid duration %q -- use Go syntax like \"5m\" or \"90s\"",
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

	if cfg.Extraction.MaxPages < 0 {
		return cfg, fmt.Errorf(
			"extraction.max_pages must be non-negative, got %d",
			cfg.Extraction.MaxPages,
		)
	}

	if cfg.Extraction.OCR.ConfidenceThreshold < 0 || cfg.Extraction.OCR.ConfidenceThreshold > 100 {
		return cfg, fmt.Errorf(
			"extraction.ocr.confidence_threshold must be 0-100, got %d",
			cfg.Extraction.OCR.ConfidenceThreshold,
		)
	}

	if t := cfg.Extraction.OCRSpatialConfThreshold(); t < 0 || t > 100 {
		return cfg, fmt.Errorf(
			"extraction.ocr.spatial_conf_threshold must be 0-100, got %d", t,
		)
	}

	checkFilePermissions(&cfg, path)

	return cfg, nil
}

// applyEnvOverrides walks the Config struct and applies environment variable
// overrides. Env var names are derived from the dotted TOML path via
// [EnvVarName]. The extra map supplies values migrated from deprecated env
// var names (checked when the canonical env var is unset).
func applyEnvOverrides(cfg *Config, extra map[string]string) error {
	return walkEnvFields(reflect.ValueOf(cfg).Elem(), "", extra)
}

func walkEnvFields(v reflect.Value, prefix string, extra map[string]string) error {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		fv := v.Field(i)

		tomlName := tomlTagName(f)
		if tomlName == "" {
			continue
		}

		key := tomlName
		if prefix != "" {
			key = prefix + "." + tomlName
		}

		// Recurse into nested config sections (structs whose first
		// field carries a TOML tag).
		if fv.Kind() == reflect.Struct {
			ft := fv.Type()
			if ft.NumField() > 0 && tomlTagName(ft.Field(0)) != "" {
				if err := walkEnvFields(fv, key, extra); err != nil {
					return err
				}
				continue
			}
		}

		envVar := EnvVarName(key)
		val := os.Getenv(envVar)
		if val == "" {
			val = extra[envVar]
		}
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
// dot-delimited config keys. Env var names are derived from dotted paths
// via [EnvVarName].
func EnvVars() map[string]string {
	m := make(map[string]string)
	collectEnvVars(reflect.TypeOf(Config{}), "", m)
	return m
}

func collectEnvVars(t reflect.Type, prefix string, m map[string]string) {
	for i := range t.NumField() {
		f := t.Field(i)
		tomlTag := tomlTagName(f)
		if tomlTag == "" {
			continue
		}
		key := tomlTag
		if prefix != "" {
			key = prefix + "." + tomlTag
		}
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		if ft.Kind() == reflect.Struct && ft.NumField() > 0 && tomlTagName(ft.Field(0)) != "" {
			collectEnvVars(ft, key, m)
		} else {
			m[EnvVarName(key)] = key
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

// EnvVarName derives the environment variable name from a dot-delimited
// config key. The dotted path is the single source of truth:
//
//	MICASA_ + UPPER(key with "." replaced by "_")
//
// For example "llm.model" becomes "MICASA_LLM_MODEL".
func EnvVarName(key string) string {
	return "MICASA_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
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

// hasAPIKeys reports whether any API key field is set in the config.
func (c Config) hasAPIKeys() bool {
	return c.LLM.APIKey != "" ||
		c.LLM.Chat.APIKey != "" ||
		c.LLM.Extraction.APIKey != ""
}

// checkFilePermissions appends a warning if the config file contains API
// keys and has permissions more open than owner-only (0600). Skipped on
// Windows where Unix file permissions do not apply.
func checkFilePermissions(cfg *Config, path string) {
	if runtime.GOOS == "windows" {
		return
	}
	if !cfg.hasAPIKeys() {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	const ownerOnly fs.FileMode = 0o600
	if perm := info.Mode().Perm(); perm&^ownerOnly != 0 {
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
			"%s has permissions %04o -- config files with API keys should be %04o; "+
				"fix with: chmod 600 %s",
			path, perm, ownerOnly, path,
		))
	}
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
	// extraction.max_ocr_pages -> extraction.max_pages (v1.47, updated v1.77)
	if md.IsDefined("extraction", "max_ocr_pages") {
		var raw struct {
			Extraction struct {
				MaxOCRPages int `toml:"max_ocr_pages"`
			} `toml:"extraction"`
		}
		if _, err := toml.DecodeFile(path, &raw); err == nil && raw.Extraction.MaxOCRPages > 0 {
			cfg.Extraction.MaxPages = raw.Extraction.MaxOCRPages
		}
		cfg.Warnings = append(cfg.Warnings,
			"extraction.max_ocr_pages is deprecated -- use extraction.max_pages instead",
		)
	}

	// extraction.max_extract_pages -> extraction.max_pages (v1.77)
	if md.IsDefined("extraction", "max_extract_pages") && !md.IsDefined("extraction", "max_pages") {
		var raw struct {
			Extraction struct {
				MaxExtractPages int `toml:"max_extract_pages"`
			} `toml:"extraction"`
		}
		if _, err := toml.DecodeFile(path, &raw); err == nil {
			cfg.Extraction.MaxPages = raw.Extraction.MaxExtractPages
		}
		cfg.Warnings = append(cfg.Warnings,
			"extraction.max_extract_pages is deprecated -- use extraction.max_pages instead",
		)
	}

	// extraction.enabled -> extraction.enable (v1.78)
	if md.IsDefined("extraction", "enabled") {
		if !md.IsDefined("extraction", "enable") {
			cfg.Extraction.Enable = cfg.Extraction.Enabled
		}
		cfg.Warnings = append(cfg.Warnings,
			"extraction.enabled is deprecated -- use extraction.enable instead",
		)
	}
	cfg.Extraction.Enabled = nil // never propagate the deprecated field

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

// envRenames maps deprecated environment variable names to their canonical
// replacements. Processed newest-first so that the most recent intermediate
// name wins when multiple generations of the same variable are set.
var envRenames = []struct{ old, canonical string }{
	// v1.78: extraction.enabled -> extraction.enable.
	{"MICASA_EXTRACTION_ENABLED", "MICASA_EXTRACTION_ENABLE"},

	// v1.77: env var names now derived from dotted config paths.
	{"MICASA_CURRENCY", "MICASA_LOCALE_CURRENCY"},
	{"MICASA_MAX_DOCUMENT_SIZE", "MICASA_DOCUMENTS_MAX_FILE_SIZE"},
	{"MICASA_CACHE_TTL", "MICASA_DOCUMENTS_CACHE_TTL"},
	{"MICASA_CACHE_TTL_DAYS", "MICASA_DOCUMENTS_CACHE_TTL_DAYS"},
	{"MICASA_FILE_PICKER_DIR", "MICASA_DOCUMENTS_FILE_PICKER_DIR"},
	{"MICASA_EXTRACTION_MAX_EXTRACT_PAGES", "MICASA_EXTRACTION_MAX_PAGES"},
	{"MICASA_MAX_EXTRACT_PAGES", "MICASA_EXTRACTION_MAX_PAGES"},

	// v1.59
	{"MICASA_EXTRACTION_MODEL", "MICASA_LLM_EXTRACTION_MODEL"},
	{"MICASA_EXTRACTION_THINKING", "MICASA_LLM_EXTRACTION_THINKING"},

	// v1.47
	{"MICASA_MAX_OCR_PAGES", "MICASA_EXTRACTION_MAX_PAGES"},
}

// migrateRenamedEnvVars checks for deprecated environment variable names and
// returns a map of canonical env var -> value for [applyEnvOverrides] to
// consume. Does not modify the process environment. Appends deprecation
// warnings to cfg.Warnings.
func migrateRenamedEnvVars(cfg *Config) map[string]string {
	overrides := make(map[string]string)
	for _, r := range envRenames {
		val := os.Getenv(r.old)
		if val == "" {
			continue
		}
		if os.Getenv(r.canonical) != "" || overrides[r.canonical] != "" {
			continue
		}
		overrides[r.canonical] = val
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
			"%s is deprecated -- use %s instead", r.old, r.canonical,
		))
	}
	return overrides
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

# Maximum time for a single LLM response (including streaming).
# Go duration syntax: "5m", "10m", etc. Default: "5m".
# Increase for very slow models or complex queries.
# timeout = "5m"

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

# Starting directory for the document file picker.
# Default: system Downloads folder (~/Downloads on most systems).
# file_picker_dir = "/home/user/Documents"

[extraction]
# Set to false to disable LLM-powered structured extraction. OCR and pdftotext
# still run (see [extraction.ocr]) to populate document text for search/display.
# enable = true

# Timeout for LLM extraction inference. Go duration syntax: "5m", "90s", etc.
# Default: "5m". Increase for slow local models or complex documents.
# llm_timeout = "5m"

# Maximum pages for async extraction of scanned documents. 0 = no limit. Default: 0.
# max_pages = 0

# [extraction.ocr]
# Set to false to disable OCR on uploaded documents. When disabled, scanned
# pages and images produce no text. Default: true.
# enable = true

# Minimum tesseract word confidence (0-100) to keep. Words below this
# threshold are dropped. 0 = no filtering. Default: 0.
# confidence_threshold = 0

[extraction.ocr]
# Send spatial layout annotations (line-level bounding boxes) from tesseract
# OCR to the LLM alongside text. Improves extraction accuracy for invoices
# and forms with tabular data, at ~2x token overhead. Default: true.
# tsv = true

# Confidence threshold (0-100) for spatial annotations. Lines with OCR
# confidence below this threshold include a confidence score; lines above
# omit it to save tokens. Set to 0 to never show confidence. Default: 70.
# confidence_threshold = 70

[locale]
# ISO 4217 currency code. Stored in the database on first run; after that the
# database value is authoritative. Override: MICASA_LOCALE_CURRENCY env var.
# Auto-detected from system locale if not set. Default: USD.
# currency = "USD"
`
}
