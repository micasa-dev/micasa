// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

const AppName = "micasa"

func DefaultDBPath() (string, error) {
	if override := os.Getenv("MICASA_DB_PATH"); override != "" {
		return override, nil
	}
	// xdg.DataFile creates the parent directory and returns the full path.
	// On Linux/WSL: $XDG_DATA_HOME/micasa/micasa.db (default ~/.local/share)
	// On macOS:     ~/Library/Application Support/micasa/micasa.db
	// On Windows:   %LOCALAPPDATA%/micasa/micasa.db
	p, err := xdg.DataFile(filepath.Join(AppName, AppName+".db"))
	if err != nil {
		return "", fmt.Errorf("resolving data file path: %w", err)
	}
	return p, nil
}

// DocumentCacheDir returns the directory used for extracted document BLOBs.
// On Linux: $XDG_CACHE_HOME/micasa/documents (default ~/.cache/micasa/documents).
func DocumentCacheDir() (string, error) {
	dir := filepath.Join(xdg.CacheHome, AppName, "documents")
	// 0o700: owner-only access. Windows ignores Unix permission bits;
	// the directory there inherits the user's default ACL.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating document cache dir: %w", err)
	}
	return dir, nil
}
