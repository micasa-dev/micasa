// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package uid

import "github.com/oklog/ulid/v2"

// New returns a new ULID string. Uses the process-global monotonic
// entropy source from oklog/ulid, which is thread-safe and guarantees
// strict lexicographic ordering within the same millisecond.
func New() string {
	return ulid.Make().String()
}

// IsValid reports whether s is a valid 26-character ULID.
func IsValid(s string) bool {
	if len(s) != 26 {
		return false
	}
	_, err := ulid.ParseStrict(s)
	return err == nil
}
