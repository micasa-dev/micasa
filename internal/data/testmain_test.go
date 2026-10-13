// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// testSeed is the base seed for all faker instances in this package's tests.
// Set via MICASA_TEST_SEED env var, or generated randomly if unset.
var testSeed uint64

// templateBytes holds a pre-migrated, pre-seeded SQLite database snapshot.
// Every test constructor writes these bytes to a fresh temp file instead of
// running AutoMigrate + SeedDefaults per test (~150ms -> ~1ms).
var templateBytes []byte

func createTemplateBytes() error {
	dir, err := os.MkdirTemp("", "micasa-template-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "template.db")
	store, err := Open(path)
	if err != nil {
		return fmt.Errorf("open template db: %w", err)
	}
	if err := store.AutoMigrate(); err != nil {
		_ = store.Close()
		return fmt.Errorf("auto migrate: %w", err)
	}
	if err := store.SeedDefaults(); err != nil {
		_ = store.Close()
		return fmt.Errorf("seed defaults: %w", err)
	}
	if err := store.WalCheckpoint(); err != nil {
		_ = store.Close()
		return fmt.Errorf("wal checkpoint: %w", err)
	}
	if err := store.Close(); err != nil {
		return fmt.Errorf("close template db: %w", err)
	}

	templateBytes, err = os.ReadFile(path) //nolint:gosec // path is constructed locally
	if err != nil {
		return fmt.Errorf("read template bytes: %w", err)
	}
	return nil
}

// openMemoryStore returns a store backed by a private in-memory copy of
// image. It mirrors datatest.OpenMemory, which this package's own tests
// cannot import without an import cycle.
func openMemoryStore(ctx context.Context, image []byte) (*Store, error) {
	store, err := Open(":memory:") //nolint:contextcheck // Open takes no context
	if err != nil {
		return nil, err
	}
	sqlDB, err := store.db.DB()
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
		d, ok := driverConn.(interface{ Deserialize(image []byte) error })
		if !ok {
			return fmt.Errorf("driver connection %T cannot deserialize", driverConn)
		}
		// An in-memory database cannot use WAL. Header bytes 18 and 19
		// are the file format versions: 2 is WAL, 1 is rollback journal.
		img := append([]byte(nil), image...)
		img[18], img[19] = 1, 1
		return d.Deserialize(img)
	})
	err = errors.Join(err, conn.Close())
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("load image: %w", err)
	}
	return store, nil
}

func TestMain(m *testing.M) {
	if s := os.Getenv("MICASA_TEST_SEED"); s != "" {
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid MICASA_TEST_SEED=%q: %v\n", s, err)
			os.Exit(2)
		}
		testSeed = v
	} else {
		testSeed = rand.Uint64() //nolint:gosec // test seed, not crypto
	}
	fmt.Fprintf(os.Stderr, "MICASA_TEST_SEED=%d\n", testSeed)

	if err := createTemplateBytes(); err != nil {
		fmt.Fprintf(os.Stderr, "create template db: %v\n", err)
		os.Exit(2)
	}

	os.Exit(m.Run())
}
