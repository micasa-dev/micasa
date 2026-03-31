// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/fake"
	"github.com/stretchr/testify/require"
)

func benchStore(b *testing.B) *Store {
	b.Helper()
	path := filepath.Join(b.TempDir(), "bench.db")
	require.NoError(b, os.WriteFile(path, templateBytes, 0o600))
	store, err := Open(path)
	require.NoError(b, err)
	b.Cleanup(func() { _ = store.Close() })
	require.NoError(b, store.SeedDemoDataFrom(fake.New(42)))
	return store
}

func BenchmarkListMaintenanceWithSchedule(b *testing.B) {
	store := benchStore(b)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListMaintenanceWithSchedule()
	}
}

func BenchmarkListActiveProjects(b *testing.B) {
	store := benchStore(b)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListActiveProjects()
	}
}

func BenchmarkListProjects(b *testing.B) {
	store := benchStore(b)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListProjects(false)
	}
}

func BenchmarkListMaintenance(b *testing.B) {
	store := benchStore(b)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListMaintenance(false)
	}
}

func BenchmarkListVendors(b *testing.B) {
	store := benchStore(b)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListVendors(false)
	}
}

func BenchmarkListExpiringWarranties(b *testing.B) {
	store := benchStore(b)
	now := time.Now()
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.ListExpiringWarranties(now, 30*24*time.Hour, 90*24*time.Hour)
	}
}

func BenchmarkYTDServiceSpendCents(b *testing.B) {
	store := benchStore(b)
	yearStart := time.Date(time.Now().Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.YTDServiceSpendCents(yearStart)
	}
}

func BenchmarkGetDocumentByID(b *testing.B) {
	for _, size := range []int{1 << 10, 1 << 20, 10 << 20, 50 << 20} {
		b.Run(fmt.Sprintf("blob_%dB", size), func(b *testing.B) {
			store := benchStore(b)
			doc := &Document{
				Title:    "bench",
				FileName: "bench.pdf",
				MIMEType: "application/pdf",
				Data:     make([]byte, size),
			}
			require.NoError(b, store.CreateDocument(doc))

			b.Run("Full", func(b *testing.B) {
				for b.Loop() {
					_, _ = store.GetDocument(doc.ID)
				}
			})
			b.Run("Metadata", func(b *testing.B) {
				for b.Loop() {
					_, _ = store.GetDocumentMetadata(doc.ID)
				}
			})
		})
	}
}
