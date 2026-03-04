// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseByteSizeBareInteger(t *testing.T) {
	t.Parallel()
	b, err := ParseByteSize("1024")
	require.NoError(t, err)
	assert.Equal(t, uint64(1024), b.Bytes())
}

func TestParseByteSizeUnitizedString(t *testing.T) {
	t.Parallel()
	b, err := ParseByteSize("50 MiB")
	require.NoError(t, err)
	assert.Equal(t, uint64(50<<20), b.Bytes())
}

func TestParseByteSizeRejectsInvalid(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"", "abc", "50 XiB", "MiB"} {
		t.Run(input, func(t *testing.T) {
			_, err := ParseByteSize(input)
			assert.Error(t, err)
		})
	}
}

func TestByteSizeUnmarshalTOMLInt(t *testing.T) {
	t.Parallel()
	var b ByteSize
	require.NoError(t, b.UnmarshalTOML(int64(1024)))
	assert.Equal(t, uint64(1024), b.Bytes())
}

func TestByteSizeUnmarshalTOMLString(t *testing.T) {
	t.Parallel()
	var b ByteSize
	require.NoError(t, b.UnmarshalTOML("50 MiB"))
	assert.Equal(t, uint64(50<<20), b.Bytes())
}

func TestByteSizeUnmarshalTOMLRejectsOtherTypes(t *testing.T) {
	t.Parallel()
	var b ByteSize
	assert.Error(t, b.UnmarshalTOML(3.14))
}

func TestByteSizeUnmarshalTOMLRejectsNegative(t *testing.T) {
	t.Parallel()
	var b ByteSize
	assert.Error(t, b.UnmarshalTOML(int64(-1)))
}

func TestParseByteSizeRejectsOverflow(t *testing.T) {
	t.Parallel()
	// 10 EiB exceeds math.MaxInt64 (~9.2 EiB).
	_, err := ParseByteSize("10 EiB")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows")
}

func TestParseByteSizeRejectsBareIntegerOverflow(t *testing.T) {
	t.Parallel()
	// math.MaxInt64 + 1 as a bare integer string.
	_, err := ParseByteSize("9223372036854775808")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows")
}
