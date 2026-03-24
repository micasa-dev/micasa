// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
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
	store, err := data.Open(path)
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
