// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Package termio provides thin TTY-detection helpers shared across
// CLI surfaces. The standard library has no IsTerminal; golang.org/x/term
// does, but only for file descriptors. This package wraps it for io.Writer.
package termio

import (
	"io"
	"os"

	"github.com/charmbracelet/x/term"
)

// IsTerminal reports whether w is an *os.File backed by a terminal.
// Any non-file writer (bytes.Buffer, io.Pipe, io.Discard, custom writers)
// is treated as non-terminal so styled output never leaks to non-TTY
// destinations.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}
