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
	"github.com/micasa-dev/micasa/internal/data/datatest"
	"github.com/micasa-dev/micasa/internal/fake"
)

// testSeed is the base seed for all faker instances in this package's tests.
// Set via MICASA_TEST_SEED env var, or generated randomly if unset.
var testSeed uint64

// templateBytes holds a pre-migrated, pre-seeded SQLite database image.
// Every test constructor loads these bytes instead of running AutoMigrate +
// SeedDefaults per test (~150ms -> ~1ms).
var templateBytes []byte

// testDBPath is the database path shown in the UI of models whose store is
// in memory. Nothing opens it; it keeps the header the same as for a
// file-backed database.
var testDBPath = filepath.Join(os.TempDir(), "micasa-test", "test.db")

// houseTemplateBytes returns templateBytes plus the house profile that
// non-demo test models need, so each test does not write it again.
var houseTemplateBytes = sync.OnceValues(func() ([]byte, error) {
	return datatest.Snapshot(templateBytes, func(store *data.Store) error {
		return store.CreateHouseProfile(data.HouseProfile{Nickname: "Test House"})
	})
})

// demoTemplates memoizes, per faker seed, an image of templateBytes with
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
			return datatest.Snapshot(templateBytes, func(store *data.Store) error {
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

	var err error
	if templateBytes, err = datatest.Template(); err != nil {
		fmt.Fprintf(os.Stderr, "create template db: %v\n", err)
		os.Exit(2)
	}

	os.Exit(m.Run())
}
