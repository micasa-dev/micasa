// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Duration wraps time.Duration with parsing support for day-suffixed
// strings ("30d") and bare integers (interpreted as seconds).
type Duration struct{ time.Duration }

// MarshalText implements encoding.TextMarshaler, using day notation for
// whole-day multiples (e.g. "30d") and Go duration syntax otherwise.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(FormatDuration(d.Duration)), nil
}

// UnmarshalTOML implements toml.Unmarshaler for Duration,
// accepting TOML integers (seconds) and strings ("30d", "720h").
func (d *Duration) UnmarshalTOML(v any) error {
	switch val := v.(type) {
	case int64:
		d.Duration = time.Duration(val) * time.Second
		return nil
	case string:
		parsed, err := ParseDuration(val)
		if err != nil {
			return err
		}
		d.Duration = parsed
		return nil
	default:
		return fmt.Errorf("cache_ttl: expected integer or string, got %T", v)
	}
}

var dayDurationRe = regexp.MustCompile(`^\s*([0-9]+)\s*d\s*$`)

// ParseDuration parses a duration string. It extends Go's
// time.ParseDuration with support for a "d" (day) suffix, and treats
// bare integers as seconds.
func ParseDuration(s string) (time.Duration, error) {
	// Try day suffix first: "30d", "7d"
	if m := dayDurationRe.FindStringSubmatch(s); m != nil {
		days, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, fmt.Errorf("invalid day count %q: %w", m[1], err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// Try Go duration syntax: "720h", "5s", "500ms"
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Try bare integer (seconds).
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Duration(n) * time.Second, nil
	}

	return 0, fmt.Errorf(
		"invalid duration %q -- use \"30d\", Go syntax like \"720h\", "+
			"or a bare integer (seconds)",
		s,
	)
}
