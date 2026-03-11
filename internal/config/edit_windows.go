// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

//go:build windows

package config

import (
	"strings"

	"golang.org/x/sys/windows"
)

// SplitEditorCommand splits an editor command string into argv using
// Windows-native command-line parsing so that quoted paths and spaces
// are handled correctly.
func SplitEditorCommand(cmd string) ([]string, error) {
	if cmd == "" {
		return []string{}, nil
	}
	argv, err := windows.DecomposeCommandLine(cmd)
	if err != nil {
		return strings.Fields(cmd), nil
	}
	return argv, nil
}

// editorFallbacks returns Windows-appropriate fallback editor names.
func editorFallbacks() []string {
	return []string{"notepad"}
}
