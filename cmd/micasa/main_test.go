// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDBPath_ExplicitPath(t *testing.T) {
	cmd := runCmd{DBPath: "/custom/path.db"}
	got, err := cmd.resolveDBPath()
	require.NoError(t, err)
	assert.Equal(t, "/custom/path.db", got)
}

func TestResolveDBPath_ExplicitPathWithDemo(t *testing.T) {
	// Explicit path takes precedence even when --demo is set.
	cmd := runCmd{DBPath: "/tmp/demo.db", Demo: true}
	got, err := cmd.resolveDBPath()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/demo.db", got)
}

func TestResolveDBPath_DemoNoPath(t *testing.T) {
	cmd := runCmd{Demo: true}
	got, err := cmd.resolveDBPath()
	require.NoError(t, err)
	assert.Equal(t, ":memory:", got)
}

func TestResolveDBPath_Default(t *testing.T) {
	// With no flags, resolveDBPath falls through to DefaultDBPath.
	// Clear the env override so the platform default is used.
	t.Setenv("MICASA_DB_PATH", "")
	cmd := runCmd{}
	got, err := cmd.resolveDBPath()
	require.NoError(t, err)
	assert.NotEmpty(t, got)
	assert.True(
		t,
		strings.HasSuffix(got, "micasa.db"),
		"expected path ending in micasa.db, got %q",
		got,
	)
}

func TestResolveDBPath_EnvOverride(t *testing.T) {
	// MICASA_DB_PATH env var is honored when no positional arg is given.
	t.Setenv("MICASA_DB_PATH", "/env/override.db")
	cmd := runCmd{}
	got, err := cmd.resolveDBPath()
	require.NoError(t, err)
	assert.Equal(t, "/env/override.db", got)
}

func TestResolveDBPath_ExplicitPathBeatsEnv(t *testing.T) {
	// Positional arg takes precedence over env var.
	t.Setenv("MICASA_DB_PATH", "/env/override.db")
	cmd := runCmd{DBPath: "/explicit/wins.db"}
	got, err := cmd.resolveDBPath()
	require.NoError(t, err)
	assert.Equal(t, "/explicit/wins.db", got)
}

// Version tests use exec.Command("go", "build") because debug.ReadBuildInfo()
// only embeds VCS revision info in binaries built with go build, not go test,
// and -ldflags -X injection likewise requires a real build step.

func buildTestBinary(t *testing.T) string {
	t.Helper()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	bin := filepath.Join(t.TempDir(), "micasa"+ext)
	cmd := exec.CommandContext(t.Context(),
		"go",
		"build",
		"-o",
		bin,
		".",
	)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed:\n%s", out)
	return bin
}

func TestVersion_DevShowsCommitHash(t *testing.T) {
	// Skip when there is no .git directory (e.g. Nix sandbox builds from a
	// source tarball), since Go won't embed VCS info without one.
	if _, err := os.Stat(".git"); err != nil {
		t.Skip("no .git directory; VCS info unavailable (e.g. Nix sandbox)")
	}
	bin := buildTestBinary(t)
	verCmd := exec.CommandContext(
		t.Context(),
		bin,
		"--version",
	)
	out, err := verCmd.Output()
	require.NoError(t, err, "--version failed")
	got := strings.TrimSpace(string(out))
	// Built inside a git repo: expect a hex hash, possibly with -dirty.
	assert.NotEqual(t, "dev", got, "expected commit hash, got bare dev")
	assert.Regexp(t, `^[0-9a-f]+(-dirty)?$`, got, "expected hex hash, got %q", got)
}

func TestVersion_Injected(t *testing.T) {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	bin := filepath.Join(t.TempDir(), "micasa"+ext)
	cmd := exec.CommandContext(t.Context(), "go", "build",
		"-ldflags", "-X main.version=1.2.3",
		"-o", bin, ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed:\n%s", out)
	verCmd := exec.CommandContext(
		t.Context(),
		bin,
		"--version",
	)
	verOut, err := verCmd.Output()
	require.NoError(t, err, "--version failed")
	assert.Equal(t, "1.2.3", strings.TrimSpace(string(verOut)))
}

func TestConfigCmd(t *testing.T) {
	bin := buildTestBinary(t)

	t.Run("LLMModel", func(t *testing.T) {
		cmd := exec.CommandContext(
			t.Context(),
			bin,
			"config",
			"llm.model",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "config llm.model failed: %s", out)
		got := strings.TrimSpace(string(out))
		assert.NotEmpty(t, got)
	})

	t.Run("UnknownKey", func(t *testing.T) {
		cmd := exec.CommandContext(
			t.Context(),
			bin,
			"config",
			"bogus.key",
		)
		out, err := cmd.CombinedOutput()
		require.Error(t, err)
		assert.Contains(t, string(out), "unknown config key")
	})

	t.Run("MissingKey", func(t *testing.T) {
		cmd := exec.CommandContext(t.Context(), bin, "config")
		out, err := cmd.CombinedOutput()
		require.Error(t, err)
		assert.Contains(t, string(out), "provide a config key or use --dump")
	})
}

// createTestDB creates a migrated, seeded SQLite database file and returns
// its path. The file lives in a test-scoped temp directory.
func createTestDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())
	require.NoError(t, store.Close())
	return path
}

func TestBackupCmd(t *testing.T) {
	bin := buildTestBinary(t)

	t.Run("ExplicitDest", func(t *testing.T) {
		src := createTestDB(t)
		dest := filepath.Join(t.TempDir(), "backup.db")
		cmd := exec.CommandContext(
			t.Context(),
			bin,
			"backup",
			"--source",
			src,
			dest,
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "backup failed: %s", out)

		got := strings.TrimSpace(string(out))
		assert.True(t, filepath.IsAbs(got), "expected absolute path, got %q", got)

		_, statErr := os.Stat(dest)
		assert.NoError(t, statErr, "destination file should exist")
	})

	t.Run("DefaultDest", func(t *testing.T) {
		src := createTestDB(t)
		cmd := exec.CommandContext(
			t.Context(),
			bin,
			"backup",
			"--source",
			src,
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "backup failed: %s", out)

		wantPath, absErr := filepath.Abs(src + ".backup")
		require.NoError(t, absErr)
		assert.Equal(t, wantPath, strings.TrimSpace(string(out)))

		_, statErr := os.Stat(src + ".backup")
		assert.NoError(t, statErr, "default destination should exist")
	})

	t.Run("SourceFromEnv", func(t *testing.T) {
		src := createTestDB(t)
		dest := filepath.Join(t.TempDir(), "env-backup.db")
		cmd := exec.CommandContext(t.Context(), bin, "backup", dest)
		cmd.Env = append(os.Environ(), "MICASA_DB_PATH="+src)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "backup via MICASA_DB_PATH failed: %s", out)

		_, statErr := os.Stat(dest)
		assert.NoError(t, statErr, "destination file should exist")
	})

	t.Run("ProducesValidDB", func(t *testing.T) {
		src := createTestDB(t)
		dest := filepath.Join(t.TempDir(), "valid-backup.db")
		cmd := exec.CommandContext(
			t.Context(),
			bin,
			"backup",
			"--source",
			src,
			dest,
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "backup failed: %s", out)

		backup, openErr := data.Open(dest)
		require.NoError(t, openErr, "backup should be a valid SQLite database")
		t.Cleanup(func() { _ = backup.Close() })
	})

	t.Run("MemorySourceRejected", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "backup.db")
		cmd := exec.CommandContext(t.Context(),
			bin,
			"backup",
			"--source",
			":memory:",
			dest,
		)
		out, err := cmd.CombinedOutput()
		require.Error(t, err)
		assert.Contains(t, string(out), "in-memory")
	})

	t.Run("DestAlreadyExists", func(t *testing.T) {
		src := createTestDB(t)
		dest := filepath.Join(t.TempDir(), "existing.db")
		require.NoError(t, os.WriteFile(dest, []byte("x"), 0o600))

		cmd := exec.CommandContext(
			t.Context(),
			bin,
			"backup",
			"--source",
			src,
			dest,
		)
		out, err := cmd.CombinedOutput()
		require.Error(t, err)
		assert.Contains(t, string(out), "already exists")
	})

	t.Run("SourceNotFound", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "backup.db")
		cmd := exec.CommandContext(t.Context(),
			bin,
			"backup",
			"--source",
			"/nonexistent/path.db",
			dest,
		)
		out, err := cmd.CombinedOutput()
		require.Error(t, err)
		assert.Contains(t, string(out), "not found")
	})

	t.Run("InvalidDestPath", func(t *testing.T) {
		src := createTestDB(t)
		cmd := exec.CommandContext(t.Context(),
			bin,
			"backup",
			"--source",
			src,
			"file:///tmp/backup.db?mode=rwc",
		)
		out, err := cmd.CombinedOutput()
		require.Error(t, err)
		assert.Contains(t, string(out), "invalid destination")
	})

	t.Run("SourceNotMicasaDB", func(t *testing.T) {
		// Create a valid SQLite database that isn't a micasa database.
		src := filepath.Join(t.TempDir(), "other.db")
		otherStore, err := data.Open(src)
		require.NoError(t, err)
		require.NoError(t, otherStore.Close())

		dest := filepath.Join(t.TempDir(), "backup.db")
		cmd := exec.CommandContext(
			t.Context(),
			bin,
			"backup",
			"--source",
			src,
			dest,
		)
		out, err := cmd.CombinedOutput()
		require.Error(t, err)
		assert.Contains(t, string(out), "not a micasa database")
	})
}
