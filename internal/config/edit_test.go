// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditor_Visual(t *testing.T) {
	t.Setenv("VISUAL", "code --wait")
	t.Setenv("EDITOR", "vim")
	got, err := Editor()
	require.NoError(t, err)
	assert.Equal(t, "code --wait", got)
}

func TestEditor_EditorEnv(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "nano")
	got, err := Editor()
	require.NoError(t, err)
	assert.Equal(t, "nano", got)
}

func TestEditor_NoEnvFallsBackToPath(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	editor, err := Editor()
	if err != nil {
		assert.Contains(t, err.Error(), "no editor found")
	} else {
		assert.NotEmpty(t, editor)
	}
}

func TestEditorCommand_SplitsArgs(t *testing.T) {
	t.Setenv("VISUAL", "code --wait --new-window")
	t.Setenv("EDITOR", "")
	name, args, err := EditorCommand("/tmp/config.toml")
	require.NoError(t, err)
	assert.Equal(t, "code", name)
	assert.Equal(t, []string{"--wait", "--new-window", "/tmp/config.toml"}, args)
}

func TestEditorCommand_SimpleEditor(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "vim")
	name, args, err := EditorCommand("/tmp/config.toml")
	require.NoError(t, err)
	assert.Equal(t, "vim", name)
	assert.Equal(t, []string{"/tmp/config.toml"}, args)
}

func TestEditorCommand_QuotedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX quoting not applicable on Windows")
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", `'/path/to my/editor' --wait`)
	name, args, err := EditorCommand("/tmp/config.toml")
	require.NoError(t, err)
	assert.Equal(t, "/path/to my/editor", name)
	assert.Equal(t, []string{"--wait", "/tmp/config.toml"}, args)
}

func TestEnsureConfigFile_CreatesNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "micasa", "config.toml")

	err := EnsureConfigFile(path)
	require.NoError(t, err)

	content, err := os.ReadFile(path) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	assert.Contains(t, string(content), "[chat.llm]")
	assert.Contains(t, string(content), "model =")
}

func TestEnsureConfigFile_ExistingFileUntouched(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "micasa")
	require.NoError(t, os.MkdirAll(dir, 0o750))

	path := filepath.Join(dir, "config.toml")
	original := []byte("[locale]\ncurrency = \"EUR\"\n")
	require.NoError(t, os.WriteFile(path, original, 0o600))

	err := EnsureConfigFile(path)
	require.NoError(t, err)

	content, err := os.ReadFile(path) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	assert.Equal(t, original, content)
}
