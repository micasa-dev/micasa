// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExtractDocument writes the document's BLOB content to the XDG cache
// directory and returns the resulting filesystem path. If the cached file
// already exists and has the expected size, the extraction is skipped.
func (s *Store) ExtractDocument(id string) (string, error) {
	var doc Document
	err := s.db.Select("data", "file_name", "sha256", "size_bytes").
		First(&doc, "id = ?", id).Error
	if err != nil {
		return "", fmt.Errorf("load document content: %w", err)
	}
	if len(doc.Data) == 0 {
		return "", fmt.Errorf("document has no content")
	}

	cacheDir, err := DocumentCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve cache dir: %w", err)
	}

	name := doc.ChecksumSHA256 + "-" + filepath.Base(doc.FileName)
	cachePath := filepath.Join(cacheDir, name)

	// Cache hit: file exists with correct size. Touch the ModTime so the
	// TTL-based eviction in EvictStaleCache treats it as recently used.
	if info, statErr := os.Stat(cachePath); statErr == nil && info.Size() == doc.SizeBytes {
		now := time.Now()
		// Best-effort: if the file was removed between Stat and Chtimes,
		// we will recreate it on the next call.
		_ = os.Chtimes(cachePath, now, now)
		return cachePath, nil
	}

	// Atomic write: create a temp file in the same directory (guaranteeing
	// same filesystem), write data, then rename into place. The rename is
	// atomic on POSIX, so readers see either the old file or the complete
	// new file -- never a partial write.
	tmp, err := os.CreateTemp(cacheDir, ".micasa-cache-*")
	if err != nil {
		return "", fmt.Errorf("create temp cache file: %w", err)
	}
	tmpPath := tmp.Name()

	// Clean up the temp file on any failure path.
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(doc.Data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write temp cache file: %w", err)
	}
	// 0o600: owner read/write only. Windows ignores Unix permission bits;
	// cached files there inherit the user's default ACL.
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("chmod temp cache file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp cache file: %w", err)
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		return "", fmt.Errorf("rename temp cache file: %w", err)
	}

	ok = true
	return cachePath, nil
}

// EvictStaleCache removes cached document files from dir that haven't been
// modified within the given TTL. A ttl of 0 disables eviction.
// Returns the number of files removed and any error encountered while listing
// the directory (individual file removal errors are skipped).
func EvictStaleCache(dir string, ttl time.Duration) (int, error) {
	if ttl <= 0 || dir == "" {
		return 0, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil // cache dir doesn't exist yet; nothing to evict
		}
		return 0, fmt.Errorf("list cache dir: %w", err)
	}

	cutoff := time.Now().Add(-ttl)
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if os.Remove(filepath.Join(dir, entry.Name())) == nil {
				removed++
			}
		}
	}
	return removed, nil
}
