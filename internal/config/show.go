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

	"github.com/micasa-dev/micasa/internal/locale"
)

// hiddenPaths lists TOML key paths excluded from ShowConfig output.
var hiddenPaths = map[string]bool{
	"chat.llm.api_key":       true,
	"extraction.llm.api_key": true,
}

// deprecatedPaths maps deprecated TOML key paths to a human-readable
// replacement hint shown in ShowConfig output.
var deprecatedPaths = map[string]string{}

// ShowConfig writes the fully resolved configuration as valid TOML to w,
// annotating each field with its env var name and marking active overrides.
func (c Config) ShowConfig(w io.Writer) error {
	dc := c.forDisplay()

	envMap := EnvVars() // env_var -> config_key
	envByKey := make(map[string]string, len(envMap))
	for ev, key := range envMap {
		envByKey[key] = ev
	}

	v := reflect.ValueOf(dc)
	blocks := walkSections(v, "", "", envByKey)
	return renderBlocks(w, blocks)
}

// forDisplay returns a copy with nil optionals populated to effective values.
func (c Config) forDisplay() Config {
	d := c
	if d.Documents.CacheTTL == nil {
		dur := d.Documents.CacheTTLDuration()
		d.Documents.CacheTTL = &Duration{dur}
	}
	if d.Chat.Enable == nil {
		t := true
		d.Chat.Enable = &t
	}
	if d.Extraction.LLM.Enable == nil {
		t := true
		d.Extraction.LLM.Enable = &t
	}
	if d.Extraction.OCR.Enable == nil {
		t := true
		d.Extraction.OCR.Enable = &t
	}
	if d.Extraction.OCR.TSV.Enable == nil {
		t := true
		d.Extraction.OCR.TSV.Enable = &t
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
	override bool   // path contains "." (e.g. chat.llm)
	doc      string // section description from doc struct tag
	lines    []annotatedLine
}

type annotatedLine struct {
	kv      string // key = value (no comment)
	comment string // inline comment including leading "# "
	empty   bool   // value is zero-ish ("", 0, false)
}

// walkSections reflects over a config struct and builds section blocks
// with annotated lines, handling hidden paths, omitempty, and env var
// comments.
func walkSections(
	v reflect.Value, prefix, doc string,
	envByKey map[string]string,
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
				walkSections(val, path, fieldDoc, envByKey)...)
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

		comment := envComment(envByKey[path])
		if replacement, ok := deprecatedPaths[path]; ok {
			dep := fmt.Sprintf("DEPRECATED: use %s", replacement)
			if comment != "" {
				comment = comment + "; " + dep
			} else {
				comment = dep
			}
		}
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
// zero-ish value (empty string, 0, false, nil pointer). A non-nil
// pointer to a zero value is NOT empty -- it was explicitly set.
func isEmptyValue(v reflect.Value) bool {
	if v.Kind() == reflect.Pointer {
		return v.IsNil()
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
// config field. If the env var is actively set, it returns "src(env): VAR".
// Otherwise it returns "env: VAR" as a hint.
func envComment(envVar string) string {
	if envVar == "" {
		return ""
	}
	if os.Getenv(envVar) != "" {
		return "src(env): " + envVar
	}
	return "env: " + envVar
}
