// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package uid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReturnsValidULID(t *testing.T) {
	t.Parallel()
	id := New()
	assert.Len(t, id, 26)
	assert.True(t, IsValid(id))
}

func TestNewIsUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool, 1000)
	for range 1000 {
		id := New()
		require.False(t, seen[id], "duplicate ULID: %s", id)
		seen[id] = true
	}
}

func TestNewIsTimeSorted(t *testing.T) {
	t.Parallel()
	prev := New()
	for range 100 {
		time.Sleep(time.Millisecond)
		cur := New()
		assert.LessOrEqual(t, prev, cur, "ULIDs should be lexicographically non-decreasing")
		prev = cur
	}
}

func TestIsValid(t *testing.T) {
	t.Parallel()
	assert.True(t, IsValid(New()))
	assert.False(t, IsValid(""))
	assert.False(t, IsValid("too-short"))
	assert.False(t, IsValid("this-is-exactly-26-chars!!"))
}
