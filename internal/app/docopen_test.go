// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelperProcess is a helper process that exits with code 1.
// It is invoked by fakeExitError via the GO_WANT_HELPER_PROCESS env var.
func TestHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(1)
}

// fakeExitError returns an *exec.ExitError with a non-zero exit code
// by re-executing the test binary with a helper that exits non-zero.
// Works on all platforms (Linux, macOS, Windows).
func fakeExitError(t *testing.T) *exec.ExitError {
	t.Helper()
	cmd := exec.CommandContext( //nolint:gosec // test helper re-execs itself
		t.Context(),
		os.Args[0],
		"-test.run=^TestHelperProcess$",
	)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	err := cmd.Run()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	return exitErr
}

func TestWrapOpenerError_NotFound(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		opener   string
		wantSub  string // substring that must appear
		wantHint string // actionable hint substring
	}{
		{
			name:     "xdg-open",
			opener:   "xdg-open",
			wantSub:  "xdg-open not found",
			wantHint: "xdg-utils",
		},
		{
			name:     "open (macOS)",
			opener:   "open",
			wantSub:  "open not found",
			wantHint: "headless",
		},
		{
			name:     "unknown opener",
			opener:   "something-else",
			wantSub:  "something-else not found",
			wantHint: "no file opener available",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapOpenerError(exec.ErrNotFound, tt.opener)
			require.ErrorContains(t, got, tt.wantSub)
			assert.ErrorContains(t, got, tt.wantHint)
		})
	}
}

func TestWrapOpenerError_PlainError_PassesThrough(t *testing.T) {
	t.Parallel()
	other := errors.New("something weird")
	got := wrapOpenerError(other, "xdg-open")
	assert.Equal(t, other, got, "non-ExitError non-ErrNotFound should pass through unchanged")
}

func TestWrapOpenerError_ExitError_GenericMessage(t *testing.T) {
	t.Parallel()
	exitErr := fakeExitError(t)
	got := wrapOpenerError(exitErr, "open")
	require.ErrorContains(t, got, "open failed")
	require.ErrorContains(t, got, "graphical environment")
}

func TestWrapOpenerError_XdgOpen_NoDisplay(t *testing.T) {
	t.Parallel()
	if hasDisplay() {
		t.Skip("test requires no display server")
	}
	exitErr := fakeExitError(t)
	got := wrapOpenerError(exitErr, "xdg-open")
	require.ErrorContains(t, got, "no display server")
	require.ErrorContains(t, got, "remote or headless")
}

func TestWrapOpenerError_XdgOpen_WithDisplay(t *testing.T) {
	t.Parallel()
	if !hasDisplay() {
		t.Skip("test requires a display server")
	}
	exitErr := fakeExitError(t)
	got := wrapOpenerError(exitErr, "xdg-open")
	require.ErrorContains(t, got, "xdg-open failed")
	require.ErrorContains(t, got, "graphical environment")
}

func TestIsDocumentTab(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		tab  *Tab
		want bool
	}{
		{name: "nil tab", tab: nil, want: false},
		{
			name: "top-level documents tab",
			tab:  &Tab{Kind: tabDocuments},
			want: true,
		},
		{
			name: "entity-scoped document sub-tab",
			tab: &Tab{
				Kind:    tabAppliances,
				Handler: newEntityDocumentHandler("appliance", "01JTEST00000000000000001"),
			},
			want: true,
		},
		{
			name: "non-document tab",
			tab:  &Tab{Kind: tabAppliances, Handler: applianceHandler{}},
			want: false,
		},
		{
			name: "nil handler non-document kind",
			tab:  &Tab{Kind: tabProjects},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.tab.isDocumentTab())
		})
	}
}
