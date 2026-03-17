// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeCLI runs the CLI in-process with the given args and returns
// captured stdout and any error.
func executeCLI(args ...string) (string, error) {
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// testBin is a lazily-built binary for the few tests that need subprocess
// isolation (env vars, VCS info). Built once via sync.Once.
var (
	testBin     string
	testBinOnce sync.Once
	testBinErr  error
)

func getTestBin(t *testing.T) string {
	t.Helper()
	testBinOnce.Do(func() {
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		dir, err := os.MkdirTemp("", "micasa-test-*")
		if err != nil {
			testBinErr = fmt.Errorf("create temp dir: %w", err)
			return
		}
		// Register cleanup at process exit is not possible with sync.Once,
		// but the OS temp dir is cleaned up eventually. The binary is small.
		bin := filepath.Join(dir, "micasa"+ext)
		cmd := exec.CommandContext(context.Background(), "go", "build", "-o", bin, ".")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			testBinErr = fmt.Errorf("build: %w\n%s", err, out)
			return
		}
		testBin = bin
	})
	require.NoError(t, testBinErr, "building test binary")
	return testBin
}

func TestResolveDBPath_ExplicitPath(t *testing.T) {
	t.Parallel()
	opts := runOpts{dbPath: "/custom/path.db"}
	got, err := opts.resolveDBPath()
	require.NoError(t, err)
	assert.Equal(t, "/custom/path.db", got)
}

func TestResolveDBPath_ExplicitPathWithDemo(t *testing.T) {
	t.Parallel()
	// Explicit path takes precedence even when --demo is set.
	opts := runOpts{dbPath: "/tmp/demo.db", demo: true}
	got, err := opts.resolveDBPath()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/demo.db", got)
}

func TestResolveDBPath_DemoNoPath(t *testing.T) {
	t.Parallel()
	opts := runOpts{demo: true}
	got, err := opts.resolveDBPath()
	require.NoError(t, err)
	assert.Equal(t, ":memory:", got)
}

func TestResolveDBPath_Default(t *testing.T) {
	// With no flags, resolveDBPath falls through to DefaultDBPath.
	// Clear the env override so the platform default is used.
	t.Setenv("MICASA_DB_PATH", "")
	opts := runOpts{}
	got, err := opts.resolveDBPath()
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
	opts := runOpts{}
	got, err := opts.resolveDBPath()
	require.NoError(t, err)
	assert.Equal(t, "/env/override.db", got)
}

func TestResolveDBPath_ExplicitPathBeatsEnv(t *testing.T) {
	// Positional arg takes precedence over env var.
	t.Setenv("MICASA_DB_PATH", "/env/override.db")
	opts := runOpts{dbPath: "/explicit/wins.db"}
	got, err := opts.resolveDBPath()
	require.NoError(t, err)
	assert.Equal(t, "/explicit/wins.db", got)
}

func TestVersion_DevShowsCommitHash(t *testing.T) {
	t.Parallel()
	// Skip when there is no .git directory (e.g. Nix sandbox builds from a
	// source tarball), since Go won't embed VCS info without one.
	if _, err := os.Stat(".git"); err != nil {
		t.Skip("no .git directory; VCS info unavailable (e.g. Nix sandbox)")
	}
	bin := getTestBin(t)
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
	t.Parallel()
	// Test the versionString logic directly: when version is set (as
	// -ldflags would do at build time), it is returned as-is.
	old := version
	t.Cleanup(func() { version = old })
	version = "1.2.3"
	assert.Equal(t, "1.2.3", versionString())
}

func TestConfigCmd(t *testing.T) {
	t.Parallel()

	t.Run("GetScalar", func(t *testing.T) {
		t.Parallel()
		out, err := executeCLI("config", "get", ".chat.llm.model")
		require.NoError(t, err)
		got := strings.TrimSpace(out)
		assert.NotEmpty(t, got)
		assert.NotContains(t, got, `"`, "scalar should not be JSON-quoted")
	})

	t.Run("GetSection", func(t *testing.T) {
		t.Parallel()
		out, err := executeCLI("config", "get", ".chat.llm")
		require.NoError(t, err)
		assert.Contains(t, out, "model =")
		assert.Contains(t, out, "provider =")
		assert.NotContains(t, out, "api_key")
	})

	t.Run("GetNull", func(t *testing.T) {
		t.Parallel()
		out, err := executeCLI("config", "get", ".bogus")
		require.NoError(t, err)
		assert.Equal(t, "null\n", out)
	})

	t.Run("GetKeys", func(t *testing.T) {
		t.Parallel()
		out, err := executeCLI("config", "get", ".chat.llm | keys")
		require.NoError(t, err)
		assert.Contains(t, out, `"model"`)
	})

	t.Run("GetDefaultShowConfig", func(t *testing.T) {
		t.Parallel()
		out, err := executeCLI("config", "get")
		require.NoError(t, err)
		assert.Contains(t, out, "[chat.llm]")
		assert.Contains(t, out, "model =")
	})

	t.Run("GetDefaultViaConfig", func(t *testing.T) {
		t.Parallel()
		out, err := executeCLI("config")
		require.NoError(t, err)
		assert.Contains(t, out, "[chat.llm]")
		assert.Contains(t, out, "model =")
	})
}

func TestConfigEditCreatesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	t.Setenv("EDITOR", noopEditor())
	t.Setenv("VISUAL", noopEditor())
	require.NoError(t, runConfigEdit(configPath))

	info, statErr := os.Stat(configPath)
	require.NoError(t, statErr, "config file should have been created")
	assert.Positive(t, info.Size(), "config file should not be empty")
}

func TestConfigEditExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	original := "[locale]\ncurrency = \"EUR\"\n"
	require.NoError(t, os.WriteFile(configPath, []byte(original), 0o600))

	t.Setenv("EDITOR", noopEditor())
	t.Setenv("VISUAL", noopEditor())
	require.NoError(t, runConfigEdit(configPath))

	content, readErr := os.ReadFile(configPath) //nolint:gosec // test reads its own temp file
	require.NoError(t, readErr)
	assert.Equal(t, original, string(content), "existing config should be untouched")
}

func TestCompletionCmd(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()
			out, err := executeCLI("completion", shell)
			require.NoError(t, err)
			assert.NotEmpty(t, out)
			assert.Contains(t, out, "micasa", "completion script should reference the app name")
		})
	}
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
	t.Parallel()

	t.Run("ExplicitDest", func(t *testing.T) {
		t.Parallel()
		src := createTestDB(t)
		dest := filepath.Join(t.TempDir(), "backup.db")
		out, err := executeCLI("backup", "--source", src, dest)
		require.NoError(t, err)

		got := strings.TrimSpace(out)
		assert.True(t, filepath.IsAbs(got), "expected absolute path, got %q", got)

		_, statErr := os.Stat(dest)
		assert.NoError(t, statErr, "destination file should exist")
	})

	t.Run("DefaultDest", func(t *testing.T) {
		t.Parallel()
		src := createTestDB(t)
		out, err := executeCLI("backup", "--source", src)
		require.NoError(t, err)

		wantPath, absErr := filepath.Abs(src + ".backup")
		require.NoError(t, absErr)
		assert.Equal(t, wantPath, strings.TrimSpace(out))

		_, statErr := os.Stat(src + ".backup")
		assert.NoError(t, statErr, "default destination should exist")
	})

	t.Run("SourceFromEnv", func(t *testing.T) {
		t.Parallel()
		src := createTestDB(t)
		dest := filepath.Join(t.TempDir(), "env-backup.db")
		var buf bytes.Buffer
		err := runBackup(&buf, &backupOpts{dest: dest, envDBPath: src})
		require.NoError(t, err)

		_, statErr := os.Stat(dest)
		assert.NoError(t, statErr, "destination file should exist")
	})

	t.Run("ProducesValidDB", func(t *testing.T) {
		t.Parallel()
		src := createTestDB(t)
		dest := filepath.Join(t.TempDir(), "valid-backup.db")
		_, err := executeCLI("backup", "--source", src, dest)
		require.NoError(t, err)

		backup, openErr := data.Open(dest)
		require.NoError(t, openErr, "backup should be a valid SQLite database")
		t.Cleanup(func() { _ = backup.Close() })
	})

	t.Run("MemorySourceRejected", func(t *testing.T) {
		t.Parallel()
		dest := filepath.Join(t.TempDir(), "backup.db")
		_, err := executeCLI("backup", "--source", ":memory:", dest)
		require.Error(t, err)
		assert.ErrorContains(t, err, "in-memory")
	})

	t.Run("DestAlreadyExists", func(t *testing.T) {
		t.Parallel()
		src := createTestDB(t)
		dest := filepath.Join(t.TempDir(), "existing.db")
		require.NoError(t, os.WriteFile(dest, []byte("x"), 0o600))

		_, err := executeCLI("backup", "--source", src, dest)
		require.Error(t, err)
		assert.ErrorContains(t, err, "already exists")
	})

	t.Run("SourceNotFound", func(t *testing.T) {
		t.Parallel()
		dest := filepath.Join(t.TempDir(), "backup.db")
		_, err := executeCLI("backup", "--source", "/nonexistent/path.db", dest)
		require.Error(t, err)
		assert.ErrorContains(t, err, "not found")
	})

	t.Run("InvalidDestPath", func(t *testing.T) {
		t.Parallel()
		src := createTestDB(t)
		_, err := executeCLI("backup", "--source", src, "file:///tmp/backup.db?mode=rwc")
		require.Error(t, err)
		assert.ErrorContains(t, err, "invalid destination")
	})

	t.Run("SourceNotMicasaDB", func(t *testing.T) {
		t.Parallel()
		// Create a valid SQLite database that isn't a micasa database.
		src := filepath.Join(t.TempDir(), "other.db")
		otherStore, err := data.Open(src)
		require.NoError(t, err)
		require.NoError(t, otherStore.Close())

		dest := filepath.Join(t.TempDir(), "backup.db")
		_, err = executeCLI("backup", "--source", src, dest)
		require.Error(t, err)
		assert.ErrorContains(t, err, "not a micasa database")
	})
}

// noopEditor returns an editor command that exits 0 without modifying
// any files. On Windows this uses "cmd /c echo" (ignores extra args
// safely); on Unix it uses "true".
func noopEditor() string {
	if runtime.GOOS == "windows" {
		return "cmd /c echo"
	}
	return "true"
}
