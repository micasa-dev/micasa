// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStoreWithScaledData(t *testing.T, seed uint64, years int) (*Store, SeedSummary) {
	t.Helper()
	store := newTestStore(t)
	summary, err := store.SeedScaledDataFrom(fake.New(seed), years)
	require.NoError(t, err)
	return store, summary
}

func TestSeedScaledDataPopulatesAllEntities(t *testing.T) {
	t.Parallel()
	store, summary := newTestStoreWithScaledData(t, testSeed, 3)

	house, err := store.HouseProfile()
	require.NoError(t, err)
	assert.NotEmpty(t, house.Nickname)

	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	assert.NotEmpty(t, vendors)
	assert.Len(t, vendors, summary.Vendors)

	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	assert.NotEmpty(t, projects)
	assert.Len(t, projects, summary.Projects)

	appliances, err := store.ListAppliances(false)
	require.NoError(t, err)
	assert.NotEmpty(t, appliances)
	assert.Len(t, appliances, summary.Appliances)

	maint, err := store.ListMaintenance(false)
	require.NoError(t, err)
	assert.NotEmpty(t, maint)
	assert.Len(t, maint, summary.Maintenance)

	assert.Positive(t, summary.ServiceLogs)
	assert.Positive(t, summary.Quotes)
	assert.Positive(t, summary.Documents)
}

func TestSeedScaledDataDeterministic(t *testing.T) {
	t.Parallel()
	store1, _ := newTestStoreWithScaledData(t, testSeed, 5)
	store2, _ := newTestStoreWithScaledData(t, testSeed, 5)

	// House profile is generated first and doesn't depend on time.Now(),
	// so it's a reliable determinism check (same pattern as the demo test).
	h1, _ := store1.HouseProfile()
	h2, _ := store2.HouseProfile()
	assert.Equal(t, h1.Nickname, h2.Nickname,
		"same seed should produce identical house names")
}

func TestSeedScaledDataGrowsWithYears(t *testing.T) {
	t.Parallel()
	_, summary1 := newTestStoreWithScaledData(t, testSeed, 1)
	_, summary5 := newTestStoreWithScaledData(t, testSeed, 5)
	_, summary10 := newTestStoreWithScaledData(t, testSeed, 10)

	assert.Less(t, summary1.ServiceLogs, summary5.ServiceLogs,
		"service logs at 1yr should be less than 5yr")
	assert.Less(t, summary5.ServiceLogs, summary10.ServiceLogs,
		"service logs at 5yr should be less than 10yr")

	assert.Less(t, summary1.Projects, summary10.Projects,
		"projects should grow with years")
	assert.Less(t, summary1.Vendors, summary10.Vendors,
		"vendors should grow with years")
}

func TestSeedScaledDataServiceLogDateSpread(t *testing.T) {
	t.Parallel()
	store, _ := newTestStoreWithScaledData(t, testSeed, 5)

	// Collect all service logs by querying maintenance items.
	maint, err := store.ListMaintenance(false)
	require.NoError(t, err)

	yearsSeen := make(map[int]bool)
	for _, m := range maint {
		logs, err := store.ListServiceLog(m.ID, false)
		require.NoError(t, err)
		for _, log := range logs {
			yearsSeen[log.ServicedAt.Year()] = true
		}
	}

	currentYear := time.Now().UTC().Year()
	// With 5 years of data, we expect at least 3 distinct years to have logs
	// (some years may have all services skipped due to the 15% miss rate).
	assert.GreaterOrEqual(t, len(yearsSeen), 3,
		"expected service logs spread across multiple years, got years: %v", yearsSeen)
	// The most recent year should have logs.
	assert.True(t, yearsSeen[currentYear],
		"expected service logs in the current year %d", currentYear)
}

func TestSeedScaledDataIdempotent(t *testing.T) {
	t.Parallel()
	store, summary1 := newTestStoreWithScaledData(t, testSeed, 3)

	// Second call should be a no-op.
	summary2, err := store.SeedScaledData(3)
	require.NoError(t, err)

	assert.Equal(t, SeedSummary{}, summary2, "second seed call should return zero summary")
	assert.NotEqual(t, SeedSummary{}, summary1, "first seed should have populated data")
}

func TestSeedScaledDataFKIntegrity(t *testing.T) {
	t.Parallel()
	store, _ := newTestStoreWithScaledData(t, testSeed, 3)

	// All projects should have valid project types.
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	types, err := store.ProjectTypes()
	require.NoError(t, err)
	typeIDs := make(map[string]bool, len(types))
	for _, pt := range types {
		typeIDs[pt.ID] = true
	}
	for _, p := range projects {
		assert.True(t, typeIDs[p.ProjectTypeID],
			"project %q has invalid project type ID %s", p.Title, p.ProjectTypeID)
	}

	// All maintenance items should have valid categories.
	maint, err := store.ListMaintenance(false)
	require.NoError(t, err)
	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	catIDs := make(map[string]bool, len(cats))
	for _, c := range cats {
		catIDs[c.ID] = true
	}
	for _, m := range maint {
		assert.True(t, catIDs[m.CategoryID],
			"maintenance %q has invalid category ID %s", m.Name, m.CategoryID)
	}

	// All service logs should reference valid maintenance items.
	maintIDs := make(map[string]bool, len(maint))
	for _, m := range maint {
		maintIDs[m.ID] = true
	}
	for _, m := range maint {
		logs, err := store.ListServiceLog(m.ID, false)
		require.NoError(t, err)
		for _, log := range logs {
			assert.True(t, maintIDs[log.MaintenanceItemID],
				"service log references invalid maintenance item ID %s", log.MaintenanceItemID)
		}
	}
}

func TestSeedScaledDataSummaryMatchesDB(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, templateBytes, 0o600))
	store, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	summary, err := store.SeedScaledDataFrom(fake.New(testSeed), 5)
	require.NoError(t, err)

	// Verify summary counts match actual DB contents.
	vendors, err := store.ListVendors(false)
	require.NoError(t, err)
	assert.Len(t, vendors, summary.Vendors)

	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	assert.Len(t, projects, summary.Projects)

	appliances, err := store.ListAppliances(false)
	require.NoError(t, err)
	assert.Len(t, appliances, summary.Appliances)

	maint, err := store.ListMaintenance(false)
	require.NoError(t, err)
	assert.Len(t, maint, summary.Maintenance)

	// Count total service logs across all maintenance items.
	totalLogs := 0
	for _, m := range maint {
		logs, err := store.ListServiceLog(m.ID, false)
		require.NoError(t, err)
		totalLogs += len(logs)
	}
	assert.Equal(t, summary.ServiceLogs, totalLogs)
}
