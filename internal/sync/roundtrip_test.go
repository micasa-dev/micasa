// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync_test

import (
	"encoding/base64"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/crypto"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/relay"
	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// --- 1. Start in-process relay ---
	ms := relay.NewMemStore()
	ms.SetEncryptionKey([]byte("test-encryption-key-exactly-32b!"))
	handler := relay.NewHandler(ms, slog.Default(), relay.WithSelfHosted())
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Generate a shared household key.
	hhKey, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	// --- 2. Create household (device A) ---
	kpA, err := crypto.GenerateDeviceKeyPair()
	require.NoError(t, err)

	hhResp, err := ms.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: "device-a",
		PublicKey:  kpA.PublicKey[:],
	})
	require.NoError(t, err)

	householdID := hhResp.HouseholdID
	tokenA := hhResp.DeviceToken
	deviceIDA := hhResp.DeviceID

	// --- 3. Open local SQLite A, migrate, seed defaults + demo data ---
	storeA := openRoundTripStore(t, srv.URL, householdID, deviceIDA)
	require.NoError(t, storeA.SeedDefaults())
	require.NoError(t, storeA.SeedDemoData())

	// --- 4. Create sync engine A, push all data ---
	clientA := sync.NewClient(srv.URL, tokenA, hhKey)
	engineA := sync.NewEngine(storeA, clientA, householdID)

	resultA, err := engineA.Sync(ctx)
	require.NoError(t, err)
	require.Positive(t, resultA.Pushed, "device A should push demo data ops")

	// --- 5. Register device B on the relay ---
	kpB, err := crypto.GenerateDeviceKeyPair()
	require.NoError(t, err)

	regResp, err := ms.RegisterDevice(ctx, sync.RegisterDeviceRequest{
		HouseholdID: householdID,
		Name:        "device-b",
		PublicKey:   kpB.PublicKey[:],
	})
	require.NoError(t, err)

	tokenB := regResp.DeviceToken
	deviceIDB := regResp.DeviceID

	// --- 6. Open local SQLite B, migrate (no seed: lookup tables arrive via sync) ---
	storeB := openRoundTripStore(t, srv.URL, householdID, deviceIDB)

	// --- 7. Create sync engine B, pull all data ---
	clientB := sync.NewClient(srv.URL, tokenB, hhKey)
	engineB := sync.NewEngine(storeB, clientB, householdID)

	resultB, err := engineB.Sync(ctx)
	require.NoError(t, err)
	require.Positive(t, resultB.Pulled, "device B should pull ops from device A")

	// --- 8. Compare every entity table between A and B ---
	compareHouseProfiles(t, storeA, storeB)
	compareProjects(t, storeA, storeB)
	compareProjectTypes(t, storeA, storeB)
	compareMaintenanceCategories(t, storeA, storeB)
	compareMaintenanceItems(t, storeA, storeB)
	compareServiceLogEntries(t, storeA, storeB)
	compareAppliances(t, storeA, storeB)
	compareIncidents(t, storeA, storeB)
	compareVendors(t, storeA, storeB)
	compareQuotes(t, storeA, storeB)
	compareDocuments(t, storeA, storeB)
}

// openRoundTripStore opens a file-backed SQLite store (for WAL support)
// with the SyncDevice row pre-created.
func openRoundTripStore(t *testing.T, relayURL, householdID, deviceID string) *data.Store {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "micasa.db")
	store, err := data.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate())
	t.Cleanup(func() { _ = store.Close() })

	err = store.GormDB().Create(&data.SyncDevice{
		ID:          deviceID,
		Name:        "test-device",
		HouseholdID: householdID,
		RelayURL:    relayURL,
		LastSeq:     0,
	}).Error
	require.NoError(t, err)

	store.SetDeviceID(deviceID)
	return store
}

// --- House Profiles ---

func compareHouseProfiles(t *testing.T, a, b *data.Store) {
	t.Helper()

	hpA, err := a.HouseProfile()
	require.NoError(t, err)
	hpB, err := b.HouseProfile()
	require.NoError(t, err)

	assert.Equal(t, hpA.ID, hpB.ID, "house_profiles: ID mismatch")
	assert.Equal(t, hpA.Nickname, hpB.Nickname, "house_profiles: Nickname")
	assert.Equal(t, hpA.AddressLine1, hpB.AddressLine1, "house_profiles: AddressLine1")
	assert.Equal(t, hpA.AddressLine2, hpB.AddressLine2, "house_profiles: AddressLine2")
	assert.Equal(t, hpA.City, hpB.City, "house_profiles: City")
	assert.Equal(t, hpA.State, hpB.State, "house_profiles: State")
	assert.Equal(t, hpA.PostalCode, hpB.PostalCode, "house_profiles: PostalCode")
	assert.Equal(t, hpA.YearBuilt, hpB.YearBuilt, "house_profiles: YearBuilt")
	assert.Equal(t, hpA.SquareFeet, hpB.SquareFeet, "house_profiles: SquareFeet")
	assert.Equal(t, hpA.LotSquareFeet, hpB.LotSquareFeet, "house_profiles: LotSquareFeet")
	assert.Equal(t, hpA.Bedrooms, hpB.Bedrooms, "house_profiles: Bedrooms")
	assert.InDelta(t, hpA.Bathrooms, hpB.Bathrooms, 0.01, "house_profiles: Bathrooms")
	assert.Equal(t, hpA.FoundationType, hpB.FoundationType, "house_profiles: FoundationType")
	assert.Equal(t, hpA.WiringType, hpB.WiringType, "house_profiles: WiringType")
	assert.Equal(t, hpA.RoofType, hpB.RoofType, "house_profiles: RoofType")
	assert.Equal(t, hpA.ExteriorType, hpB.ExteriorType, "house_profiles: ExteriorType")
	assert.Equal(t, hpA.HeatingType, hpB.HeatingType, "house_profiles: HeatingType")
	assert.Equal(t, hpA.CoolingType, hpB.CoolingType, "house_profiles: CoolingType")
	assert.Equal(t, hpA.WaterSource, hpB.WaterSource, "house_profiles: WaterSource")
	assert.Equal(t, hpA.SewerType, hpB.SewerType, "house_profiles: SewerType")
	assert.Equal(t, hpA.ParkingType, hpB.ParkingType, "house_profiles: ParkingType")
	assert.Equal(t, hpA.BasementType, hpB.BasementType, "house_profiles: BasementType")
	assert.Equal(t, hpA.InsuranceCarrier, hpB.InsuranceCarrier, "house_profiles: InsuranceCarrier")
	assert.Equal(t, hpA.InsurancePolicy, hpB.InsurancePolicy, "house_profiles: InsurancePolicy")
	assertTimePtr(t, hpA.InsuranceRenewal, hpB.InsuranceRenewal, "house_profiles: InsuranceRenewal")
	assert.Equal(t, hpA.PropertyTaxCents, hpB.PropertyTaxCents, "house_profiles: PropertyTaxCents")
	assert.Equal(t, hpA.HOAName, hpB.HOAName, "house_profiles: HOAName")
	assert.Equal(t, hpA.HOAFeeCents, hpB.HOAFeeCents, "house_profiles: HOAFeeCents")
}

// --- Project Types ---

func compareProjectTypes(t *testing.T, a, b *data.Store) {
	t.Helper()

	var ptA, ptB []data.ProjectType
	require.NoError(t, a.GormDB().Order("id").Find(&ptA).Error)
	require.NoError(t, b.GormDB().Order("id").Find(&ptB).Error)

	require.Len(t, ptB, len(ptA), "project_types: row count mismatch")
	for i := range ptA {
		assert.Equal(t, ptA[i].ID, ptB[i].ID, "project_types[%d]: ID", i)
		assert.Equal(t, ptA[i].Name, ptB[i].Name, "project_types[%d]: Name", i)
	}
}

// --- Maintenance Categories ---

func compareMaintenanceCategories(t *testing.T, a, b *data.Store) {
	t.Helper()

	var mcA, mcB []data.MaintenanceCategory
	require.NoError(t, a.GormDB().Order("id").Find(&mcA).Error)
	require.NoError(t, b.GormDB().Order("id").Find(&mcB).Error)

	require.Len(t, mcB, len(mcA), "maintenance_categories: row count mismatch")
	for i := range mcA {
		assert.Equal(t, mcA[i].ID, mcB[i].ID, "maintenance_categories[%d]: ID", i)
		assert.Equal(t, mcA[i].Name, mcB[i].Name, "maintenance_categories[%d]: Name", i)
	}
}

// --- Projects ---

func compareProjects(t *testing.T, a, b *data.Store) {
	t.Helper()

	projA, err := a.ListProjects(false)
	require.NoError(t, err)
	projB, err := b.ListProjects(false)
	require.NoError(t, err)

	sortByID(projA, func(p data.Project) string { return p.ID })
	sortByID(projB, func(p data.Project) string { return p.ID })

	require.Len(t, projB, len(projA), "projects: row count mismatch")
	for i := range projA {
		pa, pb := projA[i], projB[i]
		assert.Equal(t, pa.ID, pb.ID, "projects[%d]: ID", i)
		assert.Equal(t, pa.Title, pb.Title, "projects[%d]: Title", i)
		assert.Equal(t, pa.ProjectTypeID, pb.ProjectTypeID, "projects[%d]: ProjectTypeID", i)
		assert.Equal(t, pa.Status, pb.Status, "projects[%d]: Status", i)
		assert.Equal(t, pa.Description, pb.Description, "projects[%d]: Description", i)
		assertTimePtr(t, pa.StartDate, pb.StartDate, "projects[%d]: StartDate", i)
		assertTimePtr(t, pa.EndDate, pb.EndDate, "projects[%d]: EndDate", i)
		assert.Equal(t, pa.BudgetCents, pb.BudgetCents, "projects[%d]: BudgetCents", i)
		assert.Equal(t, pa.ActualCents, pb.ActualCents, "projects[%d]: ActualCents", i)
	}
}

// --- Maintenance Items ---

func compareMaintenanceItems(t *testing.T, a, b *data.Store) {
	t.Helper()

	miA, err := a.ListMaintenance(false)
	require.NoError(t, err)
	miB, err := b.ListMaintenance(false)
	require.NoError(t, err)

	sortByID(miA, func(m data.MaintenanceItem) string { return m.ID })
	sortByID(miB, func(m data.MaintenanceItem) string { return m.ID })

	require.Len(t, miB, len(miA), "maintenance_items: row count mismatch")
	for i := range miA {
		ma, mb := miA[i], miB[i]
		assert.Equal(t, ma.ID, mb.ID, "maintenance_items[%d]: ID", i)
		assert.Equal(t, ma.Name, mb.Name, "maintenance_items[%d]: Name", i)
		assert.Equal(t, ma.CategoryID, mb.CategoryID, "maintenance_items[%d]: CategoryID", i)
		assert.Equal(t, ma.ApplianceID, mb.ApplianceID, "maintenance_items[%d]: ApplianceID", i)
		assert.Equal(t, ma.Season, mb.Season, "maintenance_items[%d]: Season", i)
		assertTimePtr(
			t,
			ma.LastServicedAt,
			mb.LastServicedAt,
			"maintenance_items[%d]: LastServicedAt",
			i,
		)
		assert.Equal(
			t,
			ma.IntervalMonths,
			mb.IntervalMonths,
			"maintenance_items[%d]: IntervalMonths",
			i,
		)
		assertTimePtr(t, ma.DueDate, mb.DueDate, "maintenance_items[%d]: DueDate", i)
		assert.Equal(t, ma.ManualURL, mb.ManualURL, "maintenance_items[%d]: ManualURL", i)
		assert.Equal(t, ma.ManualText, mb.ManualText, "maintenance_items[%d]: ManualText", i)
		assert.Equal(t, ma.Notes, mb.Notes, "maintenance_items[%d]: Notes", i)
		assert.Equal(t, ma.CostCents, mb.CostCents, "maintenance_items[%d]: CostCents", i)
	}
}

// --- Service Log Entries ---

func compareServiceLogEntries(t *testing.T, a, b *data.Store) {
	t.Helper()

	slA, err := a.ListAllServiceLogEntries(false)
	require.NoError(t, err)
	slB, err := b.ListAllServiceLogEntries(false)
	require.NoError(t, err)

	sortByID(slA, func(s data.ServiceLogEntry) string { return s.ID })
	sortByID(slB, func(s data.ServiceLogEntry) string { return s.ID })

	require.Len(t, slB, len(slA), "service_log_entries: row count mismatch")
	for i := range slA {
		sa, sb := slA[i], slB[i]
		assert.Equal(t, sa.ID, sb.ID, "service_log_entries[%d]: ID", i)
		assert.Equal(
			t,
			sa.MaintenanceItemID,
			sb.MaintenanceItemID,
			"service_log_entries[%d]: MaintenanceItemID",
			i,
		)
		assertTimeTrunc(t, sa.ServicedAt, sb.ServicedAt, "service_log_entries[%d]: ServicedAt", i)
		assert.Equal(t, sa.VendorID, sb.VendorID, "service_log_entries[%d]: VendorID", i)
		assert.Equal(t, sa.CostCents, sb.CostCents, "service_log_entries[%d]: CostCents", i)
		assert.Equal(t, sa.Notes, sb.Notes, "service_log_entries[%d]: Notes", i)
	}
}

// --- Appliances ---

func compareAppliances(t *testing.T, a, b *data.Store) {
	t.Helper()

	appA, err := a.ListAppliances(false)
	require.NoError(t, err)
	appB, err := b.ListAppliances(false)
	require.NoError(t, err)

	sortByID(appA, func(a data.Appliance) string { return a.ID })
	sortByID(appB, func(a data.Appliance) string { return a.ID })

	require.Len(t, appB, len(appA), "appliances: row count mismatch")
	for i := range appA {
		aa, ab := appA[i], appB[i]
		assert.Equal(t, aa.ID, ab.ID, "appliances[%d]: ID", i)
		assert.Equal(t, aa.Name, ab.Name, "appliances[%d]: Name", i)
		assert.Equal(t, aa.Brand, ab.Brand, "appliances[%d]: Brand", i)
		assert.Equal(t, aa.ModelNumber, ab.ModelNumber, "appliances[%d]: ModelNumber", i)
		assert.Equal(t, aa.SerialNumber, ab.SerialNumber, "appliances[%d]: SerialNumber", i)
		assertTimePtr(t, aa.PurchaseDate, ab.PurchaseDate, "appliances[%d]: PurchaseDate", i)
		assertTimePtr(t, aa.WarrantyExpiry, ab.WarrantyExpiry, "appliances[%d]: WarrantyExpiry", i)
		assert.Equal(t, aa.Location, ab.Location, "appliances[%d]: Location", i)
		assert.Equal(t, aa.CostCents, ab.CostCents, "appliances[%d]: CostCents", i)
		assert.Equal(t, aa.Notes, ab.Notes, "appliances[%d]: Notes", i)
	}
}

// --- Incidents ---

func compareIncidents(t *testing.T, a, b *data.Store) {
	t.Helper()

	incA, err := a.ListIncidents(false)
	require.NoError(t, err)
	incB, err := b.ListIncidents(false)
	require.NoError(t, err)

	sortByID(incA, func(i data.Incident) string { return i.ID })
	sortByID(incB, func(i data.Incident) string { return i.ID })

	require.Len(t, incB, len(incA), "incidents: row count mismatch")
	for i := range incA {
		ia, ib := incA[i], incB[i]
		assert.Equal(t, ia.ID, ib.ID, "incidents[%d]: ID", i)
		assert.Equal(t, ia.Title, ib.Title, "incidents[%d]: Title", i)
		assert.Equal(t, ia.Description, ib.Description, "incidents[%d]: Description", i)
		assert.Equal(t, ia.Status, ib.Status, "incidents[%d]: Status", i)
		assert.Equal(t, ia.PreviousStatus, ib.PreviousStatus, "incidents[%d]: PreviousStatus", i)
		assert.Equal(t, ia.Severity, ib.Severity, "incidents[%d]: Severity", i)
		assertTimeTrunc(t, ia.DateNoticed, ib.DateNoticed, "incidents[%d]: DateNoticed", i)
		assertTimePtr(t, ia.DateResolved, ib.DateResolved, "incidents[%d]: DateResolved", i)
		assert.Equal(t, ia.Location, ib.Location, "incidents[%d]: Location", i)
		assert.Equal(t, ia.CostCents, ib.CostCents, "incidents[%d]: CostCents", i)
		assert.Equal(t, ia.ApplianceID, ib.ApplianceID, "incidents[%d]: ApplianceID", i)
		assert.Equal(t, ia.VendorID, ib.VendorID, "incidents[%d]: VendorID", i)
		assert.Equal(t, ia.Notes, ib.Notes, "incidents[%d]: Notes", i)
	}
}

// --- Vendors ---

func compareVendors(t *testing.T, a, b *data.Store) {
	t.Helper()

	vA, err := a.ListVendors(false)
	require.NoError(t, err)
	vB, err := b.ListVendors(false)
	require.NoError(t, err)

	sortByID(vA, func(v data.Vendor) string { return v.ID })
	sortByID(vB, func(v data.Vendor) string { return v.ID })

	require.Len(t, vB, len(vA), "vendors: row count mismatch")
	for i := range vA {
		va, vb := vA[i], vB[i]
		assert.Equal(t, va.ID, vb.ID, "vendors[%d]: ID", i)
		assert.Equal(t, va.Name, vb.Name, "vendors[%d]: Name", i)
		assert.Equal(t, va.ContactName, vb.ContactName, "vendors[%d]: ContactName", i)
		assert.Equal(t, va.Email, vb.Email, "vendors[%d]: Email", i)
		assert.Equal(t, va.Phone, vb.Phone, "vendors[%d]: Phone", i)
		assert.Equal(t, va.Website, vb.Website, "vendors[%d]: Website", i)
		assert.Equal(t, va.Notes, vb.Notes, "vendors[%d]: Notes", i)
	}
}

// --- Quotes ---

func compareQuotes(t *testing.T, a, b *data.Store) {
	t.Helper()

	qA, err := a.ListQuotes(false)
	require.NoError(t, err)
	qB, err := b.ListQuotes(false)
	require.NoError(t, err)

	sortByID(qA, func(q data.Quote) string { return q.ID })
	sortByID(qB, func(q data.Quote) string { return q.ID })

	require.Len(t, qB, len(qA), "quotes: row count mismatch")
	for i := range qA {
		qa, qb := qA[i], qB[i]
		assert.Equal(t, qa.ID, qb.ID, "quotes[%d]: ID", i)
		assert.Equal(t, qa.ProjectID, qb.ProjectID, "quotes[%d]: ProjectID", i)
		assert.Equal(t, qa.VendorID, qb.VendorID, "quotes[%d]: VendorID", i)
		assert.Equal(t, qa.TotalCents, qb.TotalCents, "quotes[%d]: TotalCents", i)
		assert.Equal(t, qa.LaborCents, qb.LaborCents, "quotes[%d]: LaborCents", i)
		assert.Equal(t, qa.MaterialsCents, qb.MaterialsCents, "quotes[%d]: MaterialsCents", i)
		assert.Equal(t, qa.OtherCents, qb.OtherCents, "quotes[%d]: OtherCents", i)
		assertTimePtr(t, qa.ReceivedDate, qb.ReceivedDate, "quotes[%d]: ReceivedDate", i)
		assert.Equal(t, qa.Notes, qb.Notes, "quotes[%d]: Notes", i)
	}
}

// --- Documents ---
// Data and ExtractData are excluded: both have json:"-" tags and are
// transferred as encrypted blobs via the relay, not as oplog fields.
// The blob round-trip is tested separately via blob upload/download.

func compareDocuments(t *testing.T, a, b *data.Store) {
	t.Helper()

	docA, err := a.ListDocuments(false)
	require.NoError(t, err)
	docB, err := b.ListDocuments(false)
	require.NoError(t, err)

	sortByID(docA, func(d data.Document) string { return d.ID })
	sortByID(docB, func(d data.Document) string { return d.ID })

	require.Len(t, docB, len(docA), "documents: row count mismatch")
	for i := range docA {
		da, db := docA[i], docB[i]
		assert.Equal(t, da.ID, db.ID, "documents[%d]: ID", i)
		assert.Equal(t, da.Title, db.Title, "documents[%d]: Title", i)
		assert.Equal(t, da.FileName, db.FileName, "documents[%d]: FileName", i)
		assert.Equal(t, da.EntityKind, db.EntityKind, "documents[%d]: EntityKind", i)
		assert.Equal(t, da.EntityID, db.EntityID, "documents[%d]: EntityID", i)
		assert.Equal(t, da.MIMEType, db.MIMEType, "documents[%d]: MIMEType", i)
		assert.Equal(t, da.SizeBytes, db.SizeBytes, "documents[%d]: SizeBytes", i)
		assert.Equal(t, da.ChecksumSHA256, db.ChecksumSHA256, "documents[%d]: ChecksumSHA256", i)
		assert.Equal(t, da.ExtractedText, db.ExtractedText, "documents[%d]: ExtractedText", i)
		assert.Equal(t, da.ExtractionModel, db.ExtractionModel, "documents[%d]: ExtractionModel", i)
		assertBytesEquivalent(
			t,
			da.ExtractionOps,
			db.ExtractionOps,
			"documents[%d]: ExtractionOps",
			i,
		)
		assert.Equal(t, da.Notes, db.Notes, "documents[%d]: Notes", i)
	}
}

// --- Helpers ---

// sortByID sorts a slice by the ID field extracted via getID.
func sortByID[T any](s []T, getID func(T) string) {
	sort.Slice(s, func(i, j int) bool {
		return getID(s[i]) < getID(s[j])
	})
}

// assertTimePtr compares two *time.Time values, truncating to seconds
// to tolerate SQLite's time resolution.
func assertTimePtr(t *testing.T, a, b *time.Time, msgAndArgs ...any) {
	t.Helper()
	if a == nil && b == nil {
		return
	}
	if a == nil || b == nil {
		assert.Fail(t, "time pointer mismatch: one is nil", msgAndArgs...)
		return
	}
	assert.WithinDuration(t, *a, *b, time.Second, msgAndArgs...)
}

// assertTimeTrunc compares two time.Time values, truncating to seconds.
func assertTimeTrunc(t *testing.T, a, b time.Time, msgAndArgs ...any) {
	t.Helper()
	assert.WithinDuration(t, a, b, time.Second, msgAndArgs...)
}

// assertBytesEquivalent compares two byte slices, accounting for the
// fact that []byte round-tripping through JSON gets base64-encoded.
// If raw comparison fails, it tries base64-decoding each side.
func assertBytesEquivalent(t *testing.T, a, b []byte, msgAndArgs ...any) {
	t.Helper()
	if assert.ObjectsAreEqual(a, b) {
		return
	}
	decA := decodeIfBase64(a)
	decB := decodeIfBase64(b)
	assert.Equal(t, decA, decB, msgAndArgs...)
}

func decodeIfBase64(data []byte) []byte {
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return data
	}
	return decoded
}
