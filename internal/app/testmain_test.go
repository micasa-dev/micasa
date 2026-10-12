// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/fake"
)

// testSeed is the base seed for all faker instances in this package's tests.
// Set via MICASA_TEST_SEED env var, or generated randomly if unset.
var testSeed uint64

// templateBytes holds a pre-migrated, pre-seeded SQLite database snapshot.
// Every test constructor writes these bytes to a fresh temp file instead of
// running AutoMigrate + SeedDefaults per test (~150ms -> ~1ms).
var templateBytes []byte

// snapshotDB opens a scratch database initialised from base (empty when
// nil), runs populate on it, and returns the checkpointed file contents.
func snapshotDB(base []byte, populate func(*data.Store) error) ([]byte, error) {
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

	b, err := os.ReadFile(path) //nolint:gosec // path is constructed locally
	if err != nil {
		return nil, fmt.Errorf("read template bytes: %w", err)
	}
	return b, nil
}

func createTemplateBytes() error {
	b, err := snapshotDB(nil, func(store *data.Store) error {
		if err := store.AutoMigrate(); err != nil {
			return fmt.Errorf("auto migrate: %w", err)
		}
		if err := store.SeedDefaults(); err != nil {
			return fmt.Errorf("seed defaults: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	templateBytes = b
	return nil
}

// demoTemplates memoizes, per faker seed, a snapshot of templateBytes with
// demo data seeded. Seeding issues hundreds of single-row commits, and most
// demo-data tests share a seed, so each seed is seeded once per process and
// later tests copy the resulting bytes.
var (
	demoTemplatesMu sync.Mutex
	demoTemplates   = map[uint64]func() ([]byte, error){}
)

func demoTemplateBytes(seed uint64) ([]byte, error) {
	demoTemplatesMu.Lock()
	snapshot, ok := demoTemplates[seed]
	if !ok {
		snapshot = sync.OnceValues(func() ([]byte, error) {
			return snapshotDB(templateBytes, func(store *data.Store) error {
				if err := store.SetMaxDocumentSize(50 << 20); err != nil {
					return err
				}
				if err := store.SeedDemoDataFrom(fake.New(seed)); err != nil {
					return fmt.Errorf("seed demo data: %w", err)
				}
				return nil
			})
		})
		demoTemplates[seed] = snapshot
	}
	demoTemplatesMu.Unlock()
	return snapshot()
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
