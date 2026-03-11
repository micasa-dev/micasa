// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

//go:build !windows

package config

import (
	"fmt"

	"github.com/google/shlex"
)

// SplitEditorCommand splits an editor command string into argv using POSIX
// shell splitting, correctly handling quoted paths and escape sequences.
func SplitEditorCommand(cmd string) ([]string, error) {
	parts, err := shlex.Split(cmd)
	if err != nil {
		return nil, fmt.Errorf("split editor command: %w", err)
	}
	return parts, nil
}

// editorFallbacks returns Unix-appropriate fallback editor names.
func editorFallbacks() []string {
	return []string{"vi", "nano"}
}
