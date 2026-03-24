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

	"github.com/micasa-dev/micasa/internal/data"
)

// Config is the top-level application configuration, loaded from a TOML file.
// Each section is self-contained; no section's values affect another section.
type Config struct {
	Chat       Chat       `toml:"chat"       doc:"Chat (NL-to-SQL) pipeline and its LLM settings."`
	Extraction Extraction `toml:"extraction" doc:"Document extraction pipeline: LLM, OCR, and pdftotext."`
	Documents  Documents  `toml:"documents"  doc:"Document attachment limits and caching."`
	Locale     Locale     `toml:"locale"     doc:"Locale and currency settings."`
	Address    Address    `toml:"address"    doc:"Postal code auto-fill settings."`

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

// Address holds settings for postal code auto-fill in the house form.
// When enabled, postal codes are sent to api.zippopotam.us (a public,
// third-party API) to resolve city and state. No authentication or
// account is required; the API sees the postal code and your IP address.
type Address struct {
	// Autofill controls whether the app looks up city/state from the
	// postal code via api.zippopotam.us. Default: false.
	Autofill *bool `toml:"autofill,omitempty"`
}

// IsAutofillEnabled returns whether postal code auto-fill is enabled.
// Defaults to false — the user must opt in, consistent with the app's
// principle that no data leaves the machine without explicit consent.
func (a Address) IsAutofillEnabled() bool {
	return a.Autofill != nil && *a.Autofill
}

// Chat holds settings for the chat (NL-to-SQL) pipeline.
type Chat struct {
	// Enable controls whether the chat feature is available in the UI.
	// Default: true.
	Enable *bool `toml:"enable,omitempty"`

	// LLM holds the LLM connection settings for the chat pipeline.
	LLM ChatLLM `toml:"llm" doc:"LLM connection settings for chat."`
}

// IsEnabled returns whether chat is enabled. Defaults to true.
func (c Chat) IsEnabled() bool {
	if c.Enable != nil {
		return *c.Enable
	}
	return true
}

// ChatLLM holds LLM settings for the chat pipeline. Each field has its
// own default; no values are inherited from other config sections.
type ChatLLM struct {
	// Provider selects which LLM provider to use. Supported values:
	// ollama, anthropic, openai, openrouter, deepseek, gemini, groq,
	// mistral, llamacpp, llamafile. Auto-detected from base_url and
	// api_key when empty.
	Provider string `toml:"provider" validate:"provider"`

	// BaseURL is the base URL for the provider's API.
	// No /v1 suffix needed -- the provider handles path construction.
	BaseURL string `toml:"base_url" default:"http://localhost:11434"`

	// Model is the model identifier passed in chat requests.
	Model string `toml:"model" default:"qwen3"`

	// APIKey is the authentication credential. Required for cloud
	// providers; leave empty for local servers like Ollama.
	APIKey string `toml:"api_key"` //nolint:gosec // config field, not a hardcoded credential

	// Timeout is the inference timeout for LLM responses (including
	// streaming). Go duration string, e.g. "5m", "10m". Default: "5m".
	Timeout string `toml:"timeout" default:"5m" validate:"omitempty,positive_duration"`

	// Thinking controls the model's reasoning effort level.
	// Supported: none, low, medium, high, auto. Empty = server default.
	Thinking string `toml:"thinking,omitempty" validate:"omitempty,oneof=none low medium high auto"`

	// ExtraContext is custom text appended to chat system prompts.
	// Useful for domain-specific details: house style, location, etc.
	ExtraContext string `toml:"extra_context"`
}

// TimeoutDuration returns the parsed timeout, falling back to
// DefaultLLMTimeout if the value is empty or unparseable.
func (l ChatLLM) TimeoutDuration() time.Duration {
	return parseDurationOr(l.Timeout, DefaultLLMTimeout)
}

// Extraction holds settings for the document extraction pipeline.
type Extraction struct {
	// MaxPages is the maximum number of pages for async extraction of
	// scanned documents. 0 means no limit. Default: 0.
	MaxPages int `toml:"max_pages" validate:"min=0"`

	// LLM holds the LLM connection settings for the extraction pipeline.
	LLM ExtractionLLM `toml:"llm" doc:"LLM connection settings for extraction."`

	// OCR holds settings for the OCR sub-pipeline.
	OCR OCR `toml:"ocr" doc:"OCR sub-pipeline. Requires tesseract and pdftocairo."`
}

// ExtractionLLM holds LLM settings for the extraction pipeline. Each field
// has its own default; no values are inherited from other config sections.
type ExtractionLLM struct {
	// Enable controls whether LLM-powered structured extraction runs when
	// a document is uploaded. When disabled, OCR and pdftotext still run
	// to populate the document's stored text. Default: true.
	Enable *bool `toml:"enable,omitempty"`

	// Provider selects which LLM provider to use. See ChatLLM.Provider
	// for supported values. Auto-detected when empty.
	Provider string `toml:"provider" validate:"provider"`

	// BaseURL is the base URL for the provider's API.
	BaseURL string `toml:"base_url" default:"http://localhost:11434"`

	// Model is the model identifier for extraction. Extraction wants a
	// small, fast model optimized for structured JSON output.
	Model string `toml:"model" default:"qwen3"`

	// APIKey is the authentication credential.
	APIKey string `toml:"api_key"` //nolint:gosec // config field, not a hardcoded credential

	// Timeout is the inference timeout for extraction LLM responses.
	Timeout string `toml:"timeout" default:"5m" validate:"omitempty,positive_duration"`

	// Thinking controls the model's reasoning effort level.
	// Supported: none, low, medium, high, auto. Empty = server default.
	Thinking string `toml:"thinking,omitempty" validate:"omitempty,oneof=none low medium high auto"`
}

// IsEnabled returns whether LLM extraction is enabled. Defaults to true.
func (e ExtractionLLM) IsEnabled() bool {
	if e.Enable != nil {
		return *e.Enable
	}
	return true
}

// TimeoutDuration returns the parsed timeout, falling back to
// DefaultLLMTimeout if the value is empty or unparseable.
func (e ExtractionLLM) TimeoutDuration() time.Duration {
	return parseDurationOr(e.Timeout, DefaultLLMTimeout)
}

// OCR holds settings for the OCR sub-pipeline within extraction.
type OCR struct {
	// Enable controls whether OCR runs on uploaded documents.
	// When disabled, scanned pages and images produce no text.
	// Default: true.
	Enable *bool `toml:"enable,omitempty"`

	// TSV holds settings for spatial layout annotations from tesseract OCR.
	TSV OCRTSV `toml:"tsv" doc:"Spatial layout annotations from tesseract OCR."`
}

// IsEnabled returns whether OCR is enabled. Defaults to true.
func (o OCR) IsEnabled() bool {
	if o.Enable != nil {
		return *o.Enable
	}
	return true
}

// OCRTSV holds settings for spatial layout annotations (line-level bounding
// boxes and confidence scores) sent from tesseract OCR to the LLM.
type OCRTSV struct {
	// Enable controls whether spatial layout annotations are sent to the
	// LLM alongside text. Improves extraction accuracy for invoices and
	// forms with tabular data, at ~2x token overhead. Default: true.
	Enable *bool `toml:"enable,omitempty"`

	// ConfidenceThreshold is the confidence threshold (0-100) below which
	// OCR confidence annotations are included in spatial layout output.
	// Lines with min confidence >= this value omit the score to save
	// tokens. Set to 0 to never show confidence. Default: 70.
	ConfidenceThreshold *int `toml:"confidence_threshold,omitempty" validate:"omitempty,min=0,max=100"`
}

// IsEnabled returns whether TSV spatial annotations are enabled.
// Defaults to true.
func (t OCRTSV) IsEnabled() bool {
	if t.Enable != nil {
		return *t.Enable
	}
	return true
}

// Threshold returns the confidence threshold below which OCR confidence
// annotations appear in spatial output. Defaults to 70.
func (t OCRTSV) Threshold() int {
	if t.ConfidenceThreshold != nil {
		return *t.ConfidenceThreshold
	}
	return 70
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
	MaxFileSize ByteSize `toml:"max_file_size" default:"52428800" validate:"required"`

	// CacheTTL is the cache lifetime for extracted documents. Accepts
	// unitized strings ("30d", "720h") or bare integers (seconds).
	// Set to "0s" to disable eviction. Default: 30d.
	CacheTTL *Duration `toml:"cache_ttl,omitempty" validate:"omitempty,nonneg_duration"`

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
// Returns 0 to disable eviction.
func (d Documents) CacheTTLDuration() time.Duration {
	if d.CacheTTL != nil {
		return d.CacheTTL.Duration
	}
	return DefaultCacheTTL
}

const (
	DefaultBaseURL    = "http://localhost:11434"
	DefaultModel      = "qwen3"
	DefaultProvider   = "ollama"
	DefaultLLMTimeout = 5 * time.Minute
	DefaultCacheTTL   = 30 * 24 * time.Hour // 30 days
	DefaultMaxPages   = 0
	configRelPath     = "micasa/config.toml"
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
		if err := checkRemovedKeys(md); err != nil {
			return cfg, err
		}
	}

	if err := applyEnvOverrides(&cfg, nil); err != nil {
		return cfg, err
	}

	// Normalize base URLs: strip trailing slash and /v1 suffix --
	// providers handle their own path construction.
	cfg.Chat.LLM.BaseURL = normalizeBaseURL(cfg.Chat.LLM.BaseURL)
	cfg.Extraction.LLM.BaseURL = normalizeBaseURL(cfg.Extraction.LLM.BaseURL)

	// Auto-detect provider from base_url and api_key when not set.
	if cfg.Chat.LLM.Provider == "" {
		cfg.Chat.LLM.Provider = detectProvider(cfg.Chat.LLM.BaseURL, cfg.Chat.LLM.APIKey)
	}
	if cfg.Extraction.LLM.Provider == "" {
		cfg.Extraction.LLM.Provider = detectProvider(
			cfg.Extraction.LLM.BaseURL,
			cfg.Extraction.LLM.APIKey,
		)
	}

	if err := cfg.validate(path); err != nil {
		return cfg, err
	}

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
		if val == "" && extra != nil {
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
// dot-delimited config keys.
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
// Keys mirror the TOML structure (e.g. "chat.llm.model",
// "documents.max_file_size").
func (c Config) Get(key string) (string, error) {
	return getField(reflect.ValueOf(c), key)
}

// getField walks a struct value using dot-delimited TOML tag names and returns
// the leaf value as a string. Returns an error if the key resolves to a
// section (struct) rather than a scalar value.
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

		// Reject sections -- use "config get" (e.g., "micasa config get .") instead.
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if isConfigSection(ft) {
			return "", fmt.Errorf(
				"%q is a config section, not a key -- use \"micasa config get\" or \"micasa config get .\" to see the full config",
				key,
			)
		}

		// Leaf field -- format the value.
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
// For example "chat.llm.model" becomes "MICASA_CHAT_LLM_MODEL".
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
			// Nested config section -- but only recurse into our own types,
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
	return c.Chat.LLM.APIKey != "" ||
		c.Extraction.LLM.APIKey != ""
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
#
# Each section is self-contained. No section's values affect another section.

[chat]
# Set to false to hide the chat feature from the UI.
# enable = true

[chat.llm]
# LLM connection settings for the chat (NL-to-SQL) pipeline.

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

# API key for cloud providers. Not needed for local servers like Ollama.
# api_key = ""

# Inference timeout (including streaming). Go duration syntax: "5m", "10m".
# timeout = "5m"

# Model reasoning effort level. Supported: none, low, medium, high, auto.
# Empty = don't send (server default).
# thinking = "medium"

# Custom context appended to chat system prompts.
# extra_context = "My house is a 1920s craftsman in Portland, OR."

[extraction]
# Maximum pages for async extraction of scanned documents. 0 = no limit.
# max_pages = 0

[extraction.llm]
# LLM connection settings for the document extraction pipeline.
# Extraction wants a fast model optimized for structured JSON output.

# Set to false to disable LLM-powered structured extraction. OCR and
# pdftotext still run to populate document text for search/display.
# enable = true

# provider = "ollama"
# base_url = "` + DefaultBaseURL + `"
model = "` + DefaultModel + `"
# api_key = ""
# timeout = "5m"
# thinking = "low"

[extraction.ocr]
# Set to false to disable OCR on uploaded documents. When disabled, scanned
# pages and images produce no text.
# enable = true

[extraction.ocr.tsv]
# Spatial layout annotations (line-level bounding boxes) from tesseract OCR.
# Improves extraction accuracy for invoices and forms with tabular data,
# at ~2x token overhead.

# Set to false to disable spatial annotations.
# enable = true

# Confidence threshold (0-100). Lines with OCR confidence below this threshold
# include a confidence score; lines above omit it to save tokens.
# Set to 0 to never show confidence.
# confidence_threshold = 70

[documents]
# Maximum file size for document imports. Accepts unitized strings or bare
# integers (bytes). Default: 50 MiB.
# max_file_size = "50 MiB"

# How long to keep extracted document cache entries before evicting on startup.
# Accepts "30d", "720h", or bare integers (seconds). Set to "0s" to disable.
# cache_ttl = "30d"

# Starting directory for the document file picker.
# Default: system Downloads folder (~/Downloads on most systems).
# file_picker_dir = "/home/user/Documents"

[locale]
# ISO 4217 currency code. Stored in the database on first run; after that the
# database value is authoritative. Auto-detected from system locale if not set.
# currency = "USD"

[address]
# Postal code auto-fill: when you type a postal code in the house form,
# micasa queries api.zippopotam.us to fill in city and state. The API
# sees the postal code and your IP address. Disabled by default.
# autofill = false
`
}
