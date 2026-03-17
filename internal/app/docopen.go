// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

// openFileResultMsg carries the outcome of an OS-viewer launch back to the
// Bubble Tea event loop so the status bar can surface errors.
type openFileResultMsg struct{ Err error }

// isDocumentTab reports whether this tab displays documents, covering both
// the top-level Documents tab and entity-scoped document sub-tabs (e.g.
// Appliances > Docs).
func (t *Tab) isDocumentTab() bool {
	return t != nil && (t.Kind == tabDocuments ||
		(t.Handler != nil && t.Handler.FormKind() == formDocument))
}

// openSelectedDocument extracts the selected document to the cache and
// launches the OS-appropriate viewer. Only operates on document tabs
// (top-level or entity-scoped); returns nil (no-op) on other tabs.
func (m *Model) openSelectedDocument() tea.Cmd {
	if !m.effectiveTab().isDocumentTab() {
		return nil
	}

	meta, ok := m.selectedRowMeta()
	if !ok || meta.Deleted {
		return nil
	}

	cachePath, err := m.store.ExtractDocument(meta.ID)
	if err != nil {
		m.setStatusError(fmt.Sprintf("extract: %s", err))
		return nil
	}

	return openFileCmd(cachePath)
}

// extractSelectedDocument loads the selected document and opens the extraction
// overlay. Only operates on document tabs; returns nil on other tabs.
func (m *Model) extractSelectedDocument() tea.Cmd {
	if !m.effectiveTab().isDocumentTab() {
		return nil
	}

	meta, ok := m.selectedRowMeta()
	if !ok || meta.Deleted {
		return nil
	}

	doc, err := m.store.GetDocument(meta.ID)
	if err != nil {
		m.setStatusError(fmt.Sprintf("load document: %s", err))
		return nil
	}

	cmd := m.startExtractionOverlay(
		doc.ID, doc.FileName, doc.Data, doc.MIMEType, doc.ExtractedText, doc.ExtractData,
	)
	if cmd == nil {
		m.setStatusError("no extraction tools or LLM configured")
		return nil
	}
	return cmd
}

// openFileCmd returns a tea.Cmd that opens the given path with the OS viewer.
// The command runs to completion so exit-status errors (e.g. no handler for
// the MIME type) are captured and returned as an openFileResultMsg.
//
// Only called from openSelectedDocument with a path returned by
// Store.ExtractDocument (always under the XDG cache directory).
func openFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		var openerName string
		switch runtime.GOOS {
		case "darwin":
			openerName = "open"
			cmd = exec.Command( //nolint:gosec,noctx // trusted cache path; no context in tea.Cmd
				"open",
				path,
			)
		case "windows":
			openerName = "cmd"
			cmd = exec.Command( //nolint:gosec,noctx // path from trusted cache directory; no context in tea.Cmd
				"cmd",
				"/c",
				"start",
				"",
				path,
			)
		default:
			openerName = "xdg-open"
			cmd = exec.Command( //nolint:gosec,noctx // trusted cache path; no context in tea.Cmd
				"xdg-open",
				path,
			)
		}
		err := cmd.Run()
		if err != nil {
			err = wrapOpenerError(err, openerName)
		}
		return openFileResultMsg{Err: err}
	}
}

// wrapOpenerError adds an actionable hint when the OS file-opener command
// is missing or fails (e.g. headless/remote server with no display).
func wrapOpenerError(err error, openerName string) error {
	if errors.Is(err, exec.ErrNotFound) {
		switch openerName {
		case "xdg-open":
			return fmt.Errorf(
				"%s not found -- install xdg-utils (e.g. apt install xdg-utils)",
				openerName,
			)
		case "open":
			return fmt.Errorf(
				"%s not found -- expected on macOS; is this a headless environment?",
				openerName,
			)
		default:
			return fmt.Errorf("%s not found -- no file opener available", openerName)
		}
	}

	// The opener command exists but exited non-zero. On headless/remote
	// machines (no display server) this is the typical failure mode.
	// Detect it and surface an actionable message.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if openerName == "xdg-open" && !hasDisplay() {
			return fmt.Errorf(
				"%s failed -- no display server (DISPLAY/WAYLAND_DISPLAY not set); "+
					"running on a remote or headless machine?",
				openerName,
			)
		}
		return fmt.Errorf(
			"%s failed (exit code %d) -- is a graphical environment available?",
			openerName, exitErr.ExitCode(),
		)
	}

	return err
}

// hasDisplay reports whether a graphical display appears to be available
// by checking the standard X11 and Wayland environment variables.
func hasDisplay() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}
