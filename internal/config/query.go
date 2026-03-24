// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/itchyny/gojq"
	"github.com/micasa-dev/micasa/internal/safeconv"
)

const (
	queryTimeout  = 2 * time.Second
	maxFilterLen  = 4096
	defaultFilter = "."
)

// Query runs a jq filter against the config and writes the result to w.
// An identity filter (".") uses ShowConfig for canonical TOML formatting.
// Otherwise: scalars print bare, objects encode as TOML, arrays as JSON.
func (c Config) Query(w io.Writer, filter string) error {
	filter = strings.TrimSpace(filter)
	if filter == "" || filter == defaultFilter {
		return c.ShowConfig(w)
	}
	if len(filter) > maxFilterLen {
		return fmt.Errorf("filter too long (%d bytes, max %d)", len(filter), maxFilterLen)
	}

	query, err := gojq.Parse(filter)
	if err != nil {
		return fmt.Errorf("parse filter: %w", err)
	}

	// Compile without WithEnvironLoader, WithModuleLoader, or
	// WithInputIter to prevent env/file/input access.
	code, err := gojq.Compile(query)
	if err != nil {
		return fmt.Errorf("compile filter: %w", err)
	}

	input, err := configToJQ(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	iter := code.RunWithContext(ctx, input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			var haltErr *gojq.HaltError
			if errors.As(err, &haltErr) {
				if haltErr.ExitCode() == 0 {
					return nil
				}
				return fmt.Errorf("filter halted with exit status %d", haltErr.ExitCode())
			}
			return fmt.Errorf("filter: %w", err)
		}
		if err := writeValue(w, v); err != nil {
			return err
		}
	}
	return nil
}

// configToJQ serializes the config through TOML (to get snake_case key
// names from toml struct tags) then deserializes into a map[string]any
// suitable for gojq input. Sensitive fields (api_key) and empty string
// values are removed.
func configToJQ(c Config) (any, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c.forDisplay()); err != nil {
		return nil, fmt.Errorf("serialize config: %w", err)
	}
	var raw map[string]any
	if _, err := toml.NewDecoder(&buf).Decode(&raw); err != nil {
		return nil, fmt.Errorf("deserialize config: %w", err)
	}
	return sanitizeMap(raw)
}

// sanitizeMap recursively:
//   - converts int64 to int (gojq expects int, not int64)
//   - removes sensitive keys (api_key)
//   - removes empty string map values (array elements are kept)
//   - removes maps that become empty after sanitization
func sanitizeMap(v any) (any, error) {
	switch val := v.(type) {
	case map[string]any:
		for k, vv := range val {
			if k == "api_key" {
				delete(val, k)
				continue
			}
			if s, ok := vv.(string); ok && s == "" {
				delete(val, k)
				continue
			}
			cleaned, err := sanitizeMap(vv)
			if err != nil {
				return nil, err
			}
			if m, ok := cleaned.(map[string]any); ok && len(m) == 0 {
				delete(val, k)
				continue
			}
			val[k] = cleaned
		}
		return val, nil
	case []any:
		for i, vv := range val {
			cleaned, err := sanitizeMap(vv)
			if err != nil {
				return nil, err
			}
			val[i] = cleaned
		}
		return val, nil
	case int64:
		n, err := safeconv.Int(val)
		if err != nil {
			return nil, fmt.Errorf("config integer overflow: %w", err)
		}
		return n, nil
	default:
		return v, nil
	}
}

// writeValue formats a single jq result value. Scalars print bare,
// objects encode as TOML, arrays encode as JSON (no top-level TOML
// representation for bare arrays).
func writeValue(w io.Writer, v any) error {
	switch val := v.(type) {
	case map[string]any:
		return writeTOML(w, val)
	case []any:
		return writeJSON(w, val)
	case nil:
		if _, err := fmt.Fprintln(w, "null"); err != nil {
			return fmt.Errorf("write null: %w", err)
		}
		return nil
	case string:
		if _, err := fmt.Fprintln(w, val); err != nil {
			return fmt.Errorf("write string: %w", err)
		}
		return nil
	default:
		if _, err := fmt.Fprintln(w, v); err != nil {
			return fmt.Errorf("write value: %w", err)
		}
		return nil
	}
}

// writeTOML encodes a map as flat TOML (no indentation).
func writeTOML(w io.Writer, m map[string]any) error {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(m); err != nil {
		return fmt.Errorf("encode TOML: %w", err)
	}
	out := bytes.TrimRight(buf.Bytes(), "\n")
	if _, err := w.Write(out); err != nil {
		return fmt.Errorf("write TOML: %w", err)
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("write TOML newline: %w", err)
	}
	return nil
}

// writeJSON encodes an array as JSON.
func writeJSON(w io.Writer, v []any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode array: %w", err)
	}
	if _, err = fmt.Fprintln(w, string(data)); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}
