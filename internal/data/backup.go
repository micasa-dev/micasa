// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"context"
	"fmt"
	"os"

	"modernc.org/sqlite"
)

// backupConn is the subset of the modernc.org/sqlite driver connection
// needed for online backups. The driver's conn type is unexported, so we
// assert this interface instead.
type backupConn interface {
	NewBackup(destPath string) (*sqlite.Backup, error)
}

// Backup creates a consistent snapshot of the database at destPath using
// SQLite's Online Backup API. The destination must not already exist.
// After copying, it runs PRAGMA integrity_check on the backup to verify
// internal consistency.
func (s *Store) Backup(ctx context.Context, destPath string) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("get underlying db: %w", err)
	}

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.Raw(func(driverConn any) error {
		b, ok := driverConn.(backupConn)
		if !ok {
			return fmt.Errorf(
				"SQLite driver does not support the backup API -- please report this as a bug",
			)
		}

		bck, err := b.NewBackup(destPath)
		if err != nil {
			return fmt.Errorf("init backup: %w", err)
		}

		// Step(-1) copies all remaining pages. The loop drains the
		// backup even if multiple iterations are needed.
		for more := true; more; {
			more, err = bck.Step(-1)
			if err != nil {
				_ = bck.Finish()
				return fmt.Errorf("backup step: %w", err)
			}
		}

		if err := bck.Finish(); err != nil {
			return fmt.Errorf("finish backup: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := verifyBackup(destPath); err != nil {
		return err
	}

	if err := os.Chmod(destPath, 0o600); err != nil {
		return fmt.Errorf("set backup file permissions: %w", err)
	}

	return nil
}

// verifyBackup opens the backup and runs PRAGMA integrity_check to confirm
// the database is internally consistent.
func verifyBackup(path string) error {
	backup, err := Open(path)
	if err != nil {
		return fmt.Errorf("open backup for verification: %w", err)
	}
	defer func() { _ = backup.Close() }()

	var result string
	if err := backup.db.Raw("PRAGMA integrity_check").Scan(&result).Error; err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("backup integrity check failed: %s", result)
	}
	return nil
}
