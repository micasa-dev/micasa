// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidSHA256Hash(t *testing.T) {
	t.Parallel()

	valid := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	assert.True(t, validSHA256Hash(valid))

	assert.False(t, validSHA256Hash(""), "empty string")
	assert.False(t, validSHA256Hash("abc"), "too short")
	assert.False(
		t,
		validSHA256Hash("E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855"),
		"uppercase",
	)
	assert.False(
		t,
		validSHA256Hash("g3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
		"invalid hex char",
	)
	assert.False(t, validSHA256Hash(valid+"aa"), "too long")
}
