// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Package datatest builds data.Store instances for tests in other packages.
//
// Tests start from a database image (the bytes of a SQLite file) instead of
// migrating a fresh database each time. OpenMemory loads an image into an
// in-memory database, so a test touches no files: no temp directory, no
// write-ahead log, and no lock files. Tests that exercise file behavior
// (paths, backups, reopening) should write the image to disk and call
// data.Open on the path instead.
package datatest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/require"
)

// Migrated returns the image of a database that has been migrated and has
// no rows. It is built once per test process.
var Migrated = sync.OnceValues(func() ([]byte, error) {
	return Snapshot(nil, func(store *data.Store) error {
		if err := store.AutoMigrate(); err != nil {
			return fmt.Errorf("auto migrate: %w", err)
		}
		return nil
	})
})

// Template returns the image of a database that has been migrated and
// seeded with defaults. It is built once per test process.
var Template = sync.OnceValues(func() ([]byte, error) {
	migrated, err := Migrated()
	if err != nil {
		return nil, err
	}
	return Snapshot(migrated, func(store *data.Store) error {
		if err := store.SeedDefaults(); err != nil {
			return fmt.Errorf("seed defaults: %w", err)
		}
		return nil
	})
})

// Snapshot opens a scratch database file that starts from base (empty when
// nil), runs populate on it, and returns the checkpointed file contents.
func Snapshot(base []byte, populate func(*data.Store) error) ([]byte, error) {
	dir, err := os.MkdirTemp("", "micasa-template-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "template.db")
	if base != nil {
		if err := os.WriteFile(path, base, 0o600); err != nil {
			return nil, fmt.Errorf("write base db: %w", err)
		}
	}
	store, err := data.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open template db: %w", err)
	}
	if err := populate(store); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.WalCheckpoint(); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("wal checkpoint: %w", err)
	}
	if err := store.Close(); err != nil {
		return nil, fmt.Errorf("close template db: %w", err)
	}

	image, err := os.ReadFile(path) //nolint:gosec // path is constructed locally
	if err != nil {
		return nil, fmt.Errorf("read template bytes: %w", err)
	}
	return image, nil
}

// OpenMemory returns a store backed by a private in-memory copy of image.
// The store is closed when the test ends.
func OpenMemory(tb testing.TB, image []byte) *data.Store {
	tb.Helper()
	store, err := openMemory(tb.Context(), image)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = store.Close() })
	return store
}

// deserializer is the modernc.org/sqlite connection method that replaces a
// connection's main database with an in-memory copy of an image.
type deserializer interface {
	Deserialize(image []byte) error
}

func openMemory(ctx context.Context, image []byte) (*data.Store, error) {
	store, err := data.Open(":memory:") //nolint:contextcheck // data.Open takes no context
	if err != nil {
		return nil, err
	}
	sqlDB, err := store.GormDB().DB()
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	// Every SQLite connection to ":memory:" has its own empty database, so
	// the pool must hold exactly the one connection that gets the image.
	sqlDB.SetMaxOpenConns(1)

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("get connection: %w", err)
	}
	err = conn.Raw(func(driverConn any) error {
		d, ok := driverConn.(deserializer)
		if !ok {
			return fmt.Errorf("driver connection %T cannot deserialize", driverConn)
		}
		return d.Deserialize(rollbackJournalImage(image))
	})
	err = errors.Join(err, conn.Close())
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("load image: %w", err)
	}
	return store, nil
}

// rollbackJournalImage returns a copy of image marked as a rollback-journal
// database. Images come from files in WAL mode, and an in-memory database
// cannot use WAL. Bytes 18 and 19 of the header hold the file format
// versions: 2 means WAL and 1 means rollback journal. This is the same
// change SQLite makes when a checkpointed database leaves WAL mode.
func rollbackJournalImage(image []byte) []byte {
	out := append([]byte(nil), image...)
	if len(out) > 19 {
		out[18], out[19] = 1, 1
	}
	return out
}
