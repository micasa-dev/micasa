// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package termio

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTerminal(t *testing.T) {
	t.Parallel()

	// bytes.Buffer is not a file.
	var buf bytes.Buffer
	assert.False(t, IsTerminal(&buf))

	// io.Discard is not a file.
	assert.False(t, IsTerminal(io.Discard))

	// Temp file is an *os.File but not a terminal.
	f, err := os.CreateTemp(t.TempDir(), "tty-test")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	assert.False(t, IsTerminal(f))
}
