// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"encoding"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cpcloud/micasa/internal/locale"
)

// hiddenPaths lists TOML key paths excluded from ShowConfig output.
var hiddenPaths = map[string]bool{
	"llm.api_key":            true,
	"llm.chat.api_key":       true,
	"llm.extraction.api_key": true,
}

// deprecatedPaths maps deprecated TOML key paths to the modern replacement
// key. When a user sets a deprecated field, the dump shows it with a
// "DEPRECATED: use <replacement>" warning.
var deprecatedPaths = map[string]string{
	"documents.cache_ttl_days": "documents.cache_ttl",
	"extraction.enabled":       "extraction.enable",
	"extraction.llm_timeout":   "llm.extraction.timeout",
	"extraction.model":         "llm.extraction.model",
	"extraction.thinking":      "llm.extraction.thinking",
}

// ShowConfig writes the fully resolved configuration as valid TOML to w,
// annotating each field with its env var name and marking active overrides.
func (c Config) ShowConfig(w io.Writer) error {
	dc := c.forDisplay()

	envMap := EnvVars() // env_var -> config_key
	envByKey := make(map[string]string, len(envMap))
	for ev, key := range envMap {
		envByKey[key] = ev
	}

	// Build fallback env vars from the rename table so that
	// deprecated env var names still show as "src(env): OLD_NAME".
	fallbacks := make(map[string][]string)
	for _, r := range envRenames {
		if key, ok := envMap[r.canonical]; ok {
			fallbacks[key] = append(fallbacks[key], r.old)
		}
	}
	// Cross-field: deprecated cache_ttl_days env vars feed cache_ttl display.
	fallbacks["documents.cache_ttl"] = append(fallbacks["documents.cache_ttl"],
		"MICASA_DOCUMENTS_CACHE_TTL_DAYS", "MICASA_CACHE_TTL_DAYS",
	)

	v := reflect.ValueOf(dc)
	allValues := make(map[string]string)
	collectValues(v, "", allValues)

	blocks := walkSections(v, "", "", envByKey, fallbacks, allValues)
	return renderBlocks(w, blocks)
}

// forDisplay returns a copy with deprecated fields resolved into their
// modern equivalents and nil optionals populated to effective values.
// User-set deprecated fields are preserved so the dump can warn about them.
func (c Config) forDisplay() Config {
	d := c
	if d.Documents.CacheTTL == nil {
		dur := d.Documents.CacheTTLDuration()
		d.Documents.CacheTTL = &Duration{dur}
	}
	// CacheTTLDays preserved when user-set so the dump warns about it.
	if d.Extraction.Enable == nil {
		t := true
		d.Extraction.Enable = &t
	}
	if d.Extraction.OCR.Enable == nil {
		t := true
		d.Extraction.OCR.Enable = &t
	}
	if d.Locale.Currency == "" {
		d.Locale.Currency = detectCurrencyCode()
	}
	return d
}

// detectCurrencyCode returns the effective currency code by resolving
// the same env/locale chain as the runtime currency detection.
func detectCurrencyCode() string {
	c, err := locale.ResolveDefault("")
	if err != nil {
		return "USD"
	}
	return c.Code()
}

// sectionBlock groups the lines belonging to a single TOML table.
type sectionBlock struct {
	header   string
	override bool   // path contains "." (e.g. llm.chat)
	doc      string // section description from doc struct tag
	lines    []annotatedLine
}

type annotatedLine struct {
	kv      string // key = value (no comment)
	comment string // inline comment including leading "# "
	empty   bool   // value is zero-ish ("", 0, false)
}

// collectValues builds a flat map of every leaf's formatted TOML value,
// keyed by fully-qualified dot path. Used by deprecated field warnings
// to reference replacement key values.
func collectValues(v reflect.Value, prefix string, vals map[string]string) {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		fv := v.Field(i)

		tomlName := tomlTagName(f)
		if tomlName == "" {
			continue
		}

		path := tomlName
		if prefix != "" {
			path = prefix + "." + tomlName
		}

		ft := f.Type
		val := fv
		if ft.Kind() == reflect.Pointer {
			if val.IsNil() {
				continue
			}
			ft = ft.Elem()
			val = val.Elem()
		}

		if isConfigSection(ft) {
			collectValues(val, path, vals)
			continue
		}

		if formatted, ok := formatTOMLValue(val); ok {
			vals[path] = formatted
		}
	}
}

// walkSections reflects over a config struct and builds section blocks
// with annotated lines, handling hidden paths, deprecated fields,
// omitempty, and env var comments.
func walkSections(
	v reflect.Value, prefix, doc string,
	envByKey map[string]string,
	fallbackEnvVars map[string][]string,
	allValues map[string]string,
) []sectionBlock {
	t := v.Type()

	cur := sectionBlock{
		header:   prefix,
		override: strings.Contains(prefix, "."),
		doc:      doc,
	}

	var nested []sectionBlock

	for i := range t.NumField() {
		f := t.Field(i)
		fv := v.Field(i)

		tomlName := tomlTagName(f)
		if tomlName == "" {
			continue
		}

		path := tomlName
		if prefix != "" {
			path = prefix + "." + tomlName
		}

		ft := f.Type
		val := fv
		isPtr := ft.Kind() == reflect.Pointer
		if isPtr {
			ft = ft.Elem()
		}

		if isConfigSection(ft) {
			if isPtr {
				if val.IsNil() {
					continue
				}
				val = val.Elem()
			}
			fieldDoc := f.Tag.Get("doc")
			nested = append(nested,
				walkSections(val, path, fieldDoc, envByKey, fallbackEnvVars, allValues)...)
			continue
		}

		if hiddenPaths[path] {
			continue
		}

		if hasOmitEmpty(f) && shouldOmitValue(fv) {
			continue
		}

		if isPtr {
			if val.IsNil() {
				continue
			}
			val = val.Elem()
		}

		formatted, ok := formatTOMLValue(val)
		if !ok {
			continue
		}

		empty := isEmptyValue(fv)

		if replacement, depOk := deprecatedPaths[path]; depOk {
			if empty {
				continue
			}
			hint := replacement
			if rv, rvOk := allValues[replacement]; rvOk {
				hint = replacement + " = " + rv
			}
			cur.lines = append(cur.lines, annotatedLine{
				kv:      tomlName + " = " + formatted,
				comment: "DEPRECATED: use " + hint,
			})
			continue
		}

		comment := envComment(envByKey[path], fallbackEnvVars[path])
		cur.lines = append(cur.lines, annotatedLine{
			kv:      tomlName + " = " + formatted,
			comment: comment,
			empty:   empty,
		})
	}

	return append([]sectionBlock{cur}, nested...)
}

// renderBlocks writes section blocks as annotated TOML, skipping empty
// override sections and adding section header comments from doc tags.
func renderBlocks(w io.Writer, blocks []sectionBlock) error {
	first := true
	for _, blk := range blocks {
		if blk.header == "" && len(blk.lines) == 0 {
			continue
		}

		if blk.override {
			hasContent := false
			for _, al := range blk.lines {
				if !al.empty {
					hasContent = true
					break
				}
			}
			if !hasContent {
				continue
			}
		}

		if !first {
			if _, err := fmt.Fprintln(w); err != nil {
				return fmt.Errorf("write config: %w", err)
			}
		}
		if blk.header != "" {
			if blk.doc != "" {
				if _, err := fmt.Fprintf(w, "[%s] # %s\n", blk.header, blk.doc); err != nil {
					return fmt.Errorf("write config: %w", err)
				}
			} else {
				if _, err := fmt.Fprintf(w, "[%s]\n", blk.header); err != nil {
					return fmt.Errorf("write config: %w", err)
				}
			}
		}
		if err := writeAligned(w, blk); err != nil {
			return err
		}
		first = false
	}

	return nil
}

// writeAligned writes a block's lines with comments aligned to the same
// column (the longest key=value width in the block, plus one space).
func writeAligned(w io.Writer, blk sectionBlock) error {
	maxKV := 0
	for _, al := range blk.lines {
		if blk.override && al.empty {
			continue
		}
		if al.comment != "" && len(al.kv) > maxKV {
			maxKV = len(al.kv)
		}
	}

	for _, al := range blk.lines {
		if blk.override && al.empty {
			continue
		}
		if al.comment == "" {
			if _, err := fmt.Fprintln(w, al.kv); err != nil {
				return fmt.Errorf("write config: %w", err)
			}
			continue
		}
		pad := maxKV - len(al.kv)
		if pad < 0 {
			pad = 0
		}
		if _, err := fmt.Fprintf(
			w, "%s%s # %s\n", al.kv, strings.Repeat(" ", pad), al.comment,
		); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}
	return nil
}

// FormatDuration formats a duration in a human-friendly way, using
// clean notation for whole-unit multiples (days, hours, minutes).
func FormatDuration(d time.Duration) string {
	switch {
	case d == 0:
		return "0s"
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	default:
		return d.String()
	}
}

// formatTOMLValue formats a reflected value as a TOML value string.
func formatTOMLValue(v reflect.Value) (string, bool) {
	if tm, ok := v.Interface().(encoding.TextMarshaler); ok {
		text, err := tm.MarshalText()
		if err != nil {
			return "", false
		}
		return strconv.Quote(string(text)), true
	}

	switch v.Kind() { //nolint:exhaustive // only config-relevant kinds
	case reflect.String:
		return strconv.Quote(v.String()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), true
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), true
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64), true
	default:
		return "", false
	}
}

// isConfigSection reports whether t is a struct type representing a
// config section (has TOML-tagged fields) rather than a scalar value
// type like Duration.
func isConfigSection(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	if t.Implements(reflect.TypeFor[encoding.TextMarshaler]()) {
		return false
	}
	for i := range t.NumField() {
		if tomlTagName(t.Field(i)) != "" {
			return true
		}
	}
	return false
}

// isEmptyValue reports whether a reflected config field holds a
// zero-ish value (empty string, 0, false, nil pointer to zero).
func isEmptyValue(v reflect.Value) bool {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true
		}
		return isEmptyValue(v.Elem())
	}

	switch v.Kind() { //nolint:exhaustive // only config-relevant kinds
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Bool:
		return !v.Bool()
	default:
		return false
	}
}

// shouldOmitValue reports whether a value should be omitted per TOML
// omitempty semantics: nil pointers and zero-valued non-pointers.
func shouldOmitValue(v reflect.Value) bool {
	if v.Kind() == reflect.Pointer {
		return v.IsNil()
	}
	return v.IsZero()
}

// hasOmitEmpty reports whether a struct field's toml tag includes
// the "omitempty" option.
func hasOmitEmpty(f reflect.StructField) bool {
	tag := f.Tag.Get("toml")
	_, after, found := strings.Cut(tag, ",")
	return found && strings.Contains(after, "omitempty")
}

// envComment returns a TOML inline comment indicating the env var for a
// config field. If the primary or a fallback env var is actively set, it
// returns "src(env): VAR". Otherwise it returns "env: VAR" as a hint.
// Returns empty string when no env var is associated.
func envComment(primary string, fallbacks []string) string {
	if primary != "" && os.Getenv(primary) != "" {
		return "src(env): " + primary
	}
	for _, alt := range fallbacks {
		if os.Getenv(alt) != "" {
			return "src(env): " + alt
		}
	}
	if primary != "" {
		return "env: " + primary
	}
	if len(fallbacks) > 0 {
		return "env: " + fallbacks[0]
	}
	return ""
}
