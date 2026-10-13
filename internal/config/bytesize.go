// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"fmt"
	"math"
	"strconv"

	"github.com/dustin/go-humanize"
)

// ByteSize represents a size in bytes, parseable from unitized strings
// like "50 MiB" or bare integers (interpreted as bytes).
type ByteSize uint64

// Bytes returns the size as uint64.
func (b ByteSize) Bytes() uint64 { return uint64(b) }

// MarshalText implements encoding.TextMarshaler, producing human-readable
// IEC notation like "50 MiB".
func (b ByteSize) MarshalText() ([]byte, error) {
	return []byte(humanize.IBytes(b.Bytes())), nil
}

// UnmarshalTOML implements toml.Unmarshaler for ByteSize,
// accepting both TOML integers (bytes) and strings ("50 MiB").
func (b *ByteSize) UnmarshalTOML(v any) error {
	switch val := v.(type) {
	case int64:
		if val < 0 {
			return fmt.Errorf("max_file_size: must be non-negative, got %d", val)
		}
		*b = ByteSize(val)
		return nil
	case string:
		parsed, err := ParseByteSize(val)
		if err != nil {
			return err
		}
		*b = parsed
		return nil
	default:
		return fmt.Errorf("max_file_size: expected integer or string, got %T", v)
	}
}

// ParseByteSize parses a size string like "50 MiB", "1.5 GiB", or "1024".
// A bare integer is interpreted as bytes.
func ParseByteSize(s string) (ByteSize, error) {
	// Try bare integer first (most common for env vars).
	if n, err := strconv.ParseUint(s, 10, 64); err == nil {
		if n > math.MaxInt64 {
			return 0, fmt.Errorf("byte size %q overflows int64", s)
		}
		return ByteSize(n), nil
	}

	n, err := humanize.ParseBytes(s)
	if err != nil {
		return 0, fmt.Errorf(
			"invalid byte size %q -- use a number with optional unit "+
				"(B, KiB, MiB, GiB, TiB, KB, MB, GB, TB), e.g. \"50 MiB\": %w",
			s, err,
		)
	}

	if n > math.MaxInt64 {
		return 0, fmt.Errorf("byte size %q overflows int64", s)
	}

	return ByteSize(n), nil
}
