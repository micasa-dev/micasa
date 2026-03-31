// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Editor returns the user's preferred editor command string, resolved from
// $VISUAL, $EDITOR, or common fallbacks. The returned string may contain
// arguments (e.g. "code --wait").
func Editor() (string, error) {
	if e := os.Getenv("VISUAL"); e != "" {
		return e, nil
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e, nil
	}
	for _, name := range editorFallbacks() {
		if _, err := exec.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", errors.New("no editor found -- set $VISUAL or $EDITOR")
}

// EditorCommand returns the editor executable and its arguments, splitting
// the editor string to handle values like "code --wait" or editors with
// quoted paths. On Windows, SplitEditorCommand uses Windows-native argv
// parsing with a whitespace-based fallback on parse error; on Unix, POSIX
// shell-style splitting is used.
func EditorCommand(configPath string) (string, []string, error) {
	editor, err := Editor()
	if err != nil {
		return "", nil, err
	}
	parts, err := SplitEditorCommand(editor)
	if err != nil {
		return "", nil, fmt.Errorf("parse editor command %q: %w", editor, err)
	}
	if len(parts) == 0 {
		return "", nil, errors.New("editor command resolved to empty")
	}
	args := make([]string, len(parts)-1, len(parts))
	copy(args, parts[1:])
	args = append(args, configPath)
	return parts[0], args, nil
}

// EnsureConfigFile creates the config file at path with example content if
// it does not already exist, creating the parent directory as needed. Uses
// O_CREATE|O_EXCL to atomically check-and-create, avoiding TOCTOU races.
func EnsureConfigFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory %s: %w", dir, err)
	}
	f, err := os.OpenFile( //nolint:gosec // internal; callers pass trusted paths (config.Path())
		path,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o600,
	)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("create config file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(ExampleTOML()); err != nil {
		return fmt.Errorf("write initial config: %w", err)
	}
	return nil
}
