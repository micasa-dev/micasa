// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/fake"
	"github.com/stretchr/testify/require"
)

func benchStore(b *testing.B, seed uint64) *Store {
	b.Helper()
	path := filepath.Join(b.TempDir(), "bench.db")
	require.NoError(b, os.WriteFile(path, templateBytes, 0o600))
	store, err := Open(path)
	require.NoError(b, err)
	b.Cleanup(func() { _ = store.Close() })
	require.NoError(b, store.SeedDemoDataFrom(fake.New(seed)))
	return store
}

func BenchmarkListMaintenanceWithSchedule(b *testing.B) {
	store := benchStore(b, 42)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListMaintenanceWithSchedule()
	}
}

func BenchmarkListActiveProjects(b *testing.B) {
	store := benchStore(b, 42)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListActiveProjects()
	}
}

func BenchmarkListProjects(b *testing.B) {
	store := benchStore(b, 42)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListProjects(false)
	}
}

func BenchmarkListMaintenance(b *testing.B) {
	store := benchStore(b, 42)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListMaintenance(false)
	}
}

func BenchmarkListVendors(b *testing.B) {
	store := benchStore(b, 42)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListVendors(false)
	}
}

func BenchmarkListExpiringWarranties(b *testing.B) {
	store := benchStore(b, 42)
	now := time.Now()
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListExpiringWarranties(now, 30*24*time.Hour, 90*24*time.Hour)
	}
}

func BenchmarkYTDServiceSpendCents(b *testing.B) {
	store := benchStore(b, 42)
	yearStart := time.Date(time.Now().Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.YTDServiceSpendCents(yearStart)
	}
}
