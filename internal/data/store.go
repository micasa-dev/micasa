// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/dustin/go-humanize"
	"github.com/iancoleman/strcase"

	"github.com/cpcloud/micasa/internal/data/sqlite"
	"github.com/cpcloud/micasa/internal/fake"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/cpcloud/micasa/internal/safeconv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db              *gorm.DB
	maxDocumentSize uint64
	currency        locale.Currency
	deviceCell      *deviceIDCell
}

func unscopedPreload(q *gorm.DB) *gorm.DB { return q.Unscoped() }

func identity(db *gorm.DB) *gorm.DB { return db }

func listQuery[T any](
	s *Store,
	includeDeleted bool,
	prepare func(*gorm.DB) *gorm.DB,
) ([]T, error) {
	var items []T
	db := prepare(s.db)
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func getByID[T any](s *Store, id string, prepare func(*gorm.DB) *gorm.DB) (T, error) {
	var item T
	err := prepare(s.db).First(&item, "id = ?", id).Error
	return item, err
}

// findOrCreate looks up a record (including soft-deleted) using the given
// predicate. If not found, creates it. If found and soft-deleted, restores
// it and marks the DeletionRecord as restored.
func findOrCreate[T any](
	tx *gorm.DB,
	item T,
	name string,
	nameLabel string,
	lookup func(*gorm.DB) *gorm.DB,
	entity string,
	id func(T) string,
	deleted func(T) bool,
) (T, error) {
	if strings.TrimSpace(name) == "" {
		var zero T
		return zero, fmt.Errorf("%s is required", nameLabel)
	}
	var existing T
	err := lookup(tx.Unscoped()).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Create(&item).Error; err != nil {
			var zero T
			return zero, err
		}
		return item, nil
	}
	if err != nil {
		var zero T
		return zero, err
	}
	if deleted(existing) {
		if err := restoreSoftDeleted(tx, new(T), entity, id(existing)); err != nil {
			var zero T
			return zero, err
		}
	}
	return existing, nil
}

type dependencyCheck struct {
	model  any
	fkCol  string
	errFmt string
}

func (s *Store) checkDependencies(id string, checks []dependencyCheck) error {
	for _, c := range checks {
		n, err := s.countDependents(c.model, c.fkCol, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf(c.errFmt, n)
		}
	}
	return nil
}

func Open(path string) (*Store, error) {
	if err := ValidateDBPath(path); err != nil {
		return nil, err
	}
	db, err := gorm.Open(
		sqlite.Open(path,
			"PRAGMA foreign_keys = ON",
			"PRAGMA journal_mode = WAL",
			"PRAGMA synchronous = NORMAL",
			"PRAGMA busy_timeout = 5000",
		),
		&gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	cell := &deviceIDCell{}
	db = db.WithContext(withDeviceIDCell(db.Statement.Context, cell))
	return &Store{db: db, deviceCell: cell}, nil
}

// GormDB returns the underlying *gorm.DB for use by sync.ApplyOps,
// which needs direct GORM access to apply remote operations generically.
func (s *Store) GormDB() *gorm.DB {
	return s.db
}

// MaxDocumentSize returns the configured maximum file size for document imports.
func (s *Store) MaxDocumentSize() uint64 {
	return s.maxDocumentSize
}

// SetMaxDocumentSize overrides the maximum allowed file size for document
// imports. The value must be positive; zero is rejected.
func (s *Store) SetMaxDocumentSize(n uint64) error {
	if n == 0 {
		return fmt.Errorf("max document size must be positive, got 0")
	}
	s.maxDocumentSize = n
	return nil
}

// Currency returns the resolved currency for this store.
func (s *Store) Currency() locale.Currency {
	return s.currency
}

// SetCurrency directly sets the cached currency without touching the database.
// Intended for tests that need a currency but don't require full resolution.
func (s *Store) SetCurrency(cur locale.Currency) {
	s.currency = cur
}

// ResolveCurrency determines the currency to use. The database value is
// authoritative for the currency CODE; if unset, resolves from
// configured/env/locale and persists the code for portability. The
// formatting locale is always detected from the environment (never
// persisted) -- like displaying a UTC timestamp in the local timezone.
func (s *Store) ResolveCurrency(configured string) error {
	tag := locale.DetectLocale()
	code, err := s.GetCurrency()
	if err != nil {
		return fmt.Errorf("read currency from database: %w", err)
	}
	if code != "" {
		cur, err := locale.Resolve(code, tag)
		if err != nil {
			return err
		}
		s.currency = cur
		return nil
	}
	cur, err := locale.ResolveDefault(configured)
	if err != nil {
		return err
	}
	if err := s.PutCurrency(cur.Code()); err != nil {
		return fmt.Errorf("persist currency to database: %w", err)
	}
	s.currency = cur
	return nil
}

// ExpandHome replaces a leading "~" or "~/" with the current user's home
// directory. Other forms like "~user/" are left as-is because os/user.Lookup
// requires cgo on macOS.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// ValidateDBPath rejects paths that could be interpreted as URIs by the
// SQLite driver. The underlying go-sqlite driver passes query strings
// through net/url.ParseQuery (subject to CVE GO-2026-4341) and enables
// SQLITE_OPEN_URI, so both file:// URIs and scheme://... URLs must be
// blocked. Only plain filesystem paths and the special ":memory:" value
// are accepted.
func ValidateDBPath(path string) error {
	if path == "" {
		return fmt.Errorf("database path must not be empty")
	}
	if path == ":memory:" {
		return nil
	}
	// Reject anything with a URI scheme (letters followed by "://").
	if i := strings.Index(path, "://"); i > 0 {
		scheme := path[:i]
		if isLetterOnly(scheme) {
			return fmt.Errorf(
				"database path %q looks like a URI (%s://); only filesystem paths are accepted",
				path, scheme,
			)
		}
	}
	// Reject "file:" prefix -- even without "//", SQLite interprets
	// "file:path?query" as a URI when SQLITE_OPEN_URI is set.
	if strings.HasPrefix(path, "file:") {
		return fmt.Errorf(
			"database path %q uses the file: scheme; pass a plain filesystem path instead",
			path,
		)
	}
	// Reject paths containing '?' -- the go-sqlite driver splits on '?'
	// and feeds the remainder to url.ParseQuery.
	if strings.ContainsRune(path, '?') {
		return fmt.Errorf(
			"database path %q contains '?' which would be interpreted as query parameters",
			path,
		)
	}
	return nil
}

func isLetterOnly(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// WalCheckpoint runs a WAL checkpoint that flushes the WAL into the main
// database file and truncates the WAL. This ensures the .db file is
// self-contained with no -wal or -shm sidecars.
func (s *Store) WalCheckpoint() error {
	return s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("get underlying db: %w", err)
	}
	return sqlDB.Close()
}

// Transaction executes fn inside a database transaction. The callback
// receives a transactional Store that shares all methods but operates on the
// transaction. If fn returns an error the transaction is rolled back;
// otherwise it is committed.
func (s *Store) Transaction(fn func(tx *Store) error) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		txStore := &Store{
			db:              tx,
			maxDocumentSize: s.maxDocumentSize,
			currency:        s.currency,
		}
		return fn(txStore)
	})
}

// IsMicasaDB returns true if the database contains the core micasa tables.
func (s *Store) IsMicasaDB() (bool, error) {
	var count int64
	err := s.db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN (?, ?, ?)`,
		TableVendors, TableProjects, TableAppliances,
	).Scan(&count).Error
	if err != nil {
		return false, err
	}
	return count == 3, nil
}

func (s *Store) AutoMigrate() error {
	if err := s.db.AutoMigrate(Models()...); err != nil {
		return err
	}
	return s.setupFTS()
}

func (s *Store) SeedDefaults() error {
	if err := s.seedProjectTypes(); err != nil {
		return err
	}
	return s.seedMaintenanceCategories()
}

// SeedDemoData populates the database with realistic demo data using a fixed
// seed so the demo always looks the same. Skips if data already exists.
func (s *Store) SeedDemoData() error {
	return s.SeedDemoDataFrom(fake.New(42))
}

// SeedDemoDataFrom populates the database with demo data generated by the
// given HomeFaker. Callers can pass different seeds for varied test data.
func (s *Store) SeedDemoDataFrom(h *fake.HomeFaker) error {
	var count int64
	if err := s.db.Model(&HouseProfile{}).Count(&count).Error; err != nil {
		return fmt.Errorf("check existing data: %w", err)
	}
	if count > 0 {
		return nil
	}

	fh := h.HouseProfile()
	house := HouseProfile{
		Nickname:         fh.Nickname,
		AddressLine1:     fh.AddressLine1,
		City:             fh.City,
		State:            fh.State,
		PostalCode:       fh.PostalCode,
		YearBuilt:        fh.YearBuilt,
		SquareFeet:       fh.SquareFeet,
		LotSquareFeet:    fh.LotSquareFeet,
		Bedrooms:         fh.Bedrooms,
		Bathrooms:        fh.Bathrooms,
		FoundationType:   fh.FoundationType,
		WiringType:       fh.WiringType,
		RoofType:         fh.RoofType,
		ExteriorType:     fh.ExteriorType,
		HeatingType:      fh.HeatingType,
		CoolingType:      fh.CoolingType,
		WaterSource:      fh.WaterSource,
		SewerType:        fh.SewerType,
		ParkingType:      fh.ParkingType,
		BasementType:     fh.BasementType,
		InsuranceCarrier: fh.InsuranceCarrier,
		InsurancePolicy:  fh.InsurancePolicy,
		InsuranceRenewal: fh.InsuranceRenewal,
		PropertyTaxCents: fh.PropertyTaxCents,
		HOAName:          fh.HOAName,
		HOAFeeCents:      fh.HOAFeeCents,
	}
	if err := s.db.Create(&house).Error; err != nil {
		return fmt.Errorf("seed house: %w", err)
	}

	// Lookup helpers that panic on missing seed data. SeedDefaults must run
	// before SeedDemoDataFrom; a missing type/category is a programming error.
	typeID := func(name string) string {
		var pt ProjectType
		if err := s.db.Where(ColName+" = ?", name).First(&pt).Error; err != nil {
			panic(
				fmt.Sprintf(
					"seed: project type %q not found (run SeedDefaults first): %v",
					name,
					err,
				),
			)
		}
		return pt.ID
	}
	catID := func(name string) string {
		var mc MaintenanceCategory
		if err := s.db.Where(ColName+" = ?", name).First(&mc).Error; err != nil {
			panic(
				fmt.Sprintf(
					"seed: maintenance category %q not found (run SeedDefaults first): %v",
					name,
					err,
				),
			)
		}
		return mc.ID
	}

	// Vendors: one per trade.
	trades := fake.VendorTrades()
	vendors := make([]Vendor, len(trades))
	for i, trade := range trades {
		fv := h.VendorForTrade(trade)
		vendors[i] = Vendor{
			Name:        fv.Name,
			ContactName: fv.ContactName,
			Phone:       fv.Phone,
			Email:       fv.Email,
			Website:     fv.Website,
		}
		if err := s.db.Create(&vendors[i]).Error; err != nil {
			return fmt.Errorf("seed vendor %s: %w", vendors[i].Name, err)
		}
	}

	// Projects: one per project type.
	projectTypeNames := fake.ProjectTypes()
	projects := make([]Project, len(projectTypeNames))
	for i, typeName := range projectTypeNames {
		fp := h.Project(typeName)
		projects[i] = Project{
			Title:         fp.Title,
			ProjectTypeID: typeID(typeName),
			Status:        fp.Status,
			Description:   fp.Description,
			StartDate:     fp.StartDate,
			EndDate:       fp.EndDate,
			BudgetCents:   fp.BudgetCents,
			ActualCents:   fp.ActualCents,
		}
		if err := s.db.Create(&projects[i]).Error; err != nil {
			return fmt.Errorf("seed project %s: %w", projects[i].Title, err)
		}
	}

	// Quotes: 1-2 for each non-ideating, non-abandoned project.
	for i := range projects {
		if projects[i].Status == ProjectStatusIdeating ||
			projects[i].Status == ProjectStatusAbandoned {
			continue
		}
		nQuotes := 1 + h.IntN(2)
		for range nQuotes {
			vi := h.IntN(len(vendors))
			fq := h.Quote()
			quote := Quote{
				ProjectID:      projects[i].ID,
				VendorID:       vendors[vi].ID,
				TotalCents:     fq.TotalCents,
				LaborCents:     fq.LaborCents,
				MaterialsCents: fq.MaterialsCents,
				ReceivedDate:   fq.ReceivedDate,
				Notes:          fq.Notes,
			}
			if err := s.db.Create(&quote).Error; err != nil {
				return fmt.Errorf("seed quote: %w", err)
			}
		}
	}

	// Appliances: 5-8 items.
	nAppliances := 5 + h.IntN(4)
	appliances := make([]Appliance, nAppliances)
	for i := range appliances {
		fa := h.Appliance()
		appliances[i] = Appliance{
			Name:           fa.Name,
			Brand:          fa.Brand,
			ModelNumber:    fa.ModelNumber,
			SerialNumber:   fa.SerialNumber,
			Location:       fa.Location,
			PurchaseDate:   fa.PurchaseDate,
			WarrantyExpiry: fa.WarrantyExpiry,
			CostCents:      fa.CostCents,
		}
		if err := s.db.Create(&appliances[i]).Error; err != nil {
			return fmt.Errorf("seed appliance %s: %w", appliances[i].Name, err)
		}
	}

	// Maintenance items: 1-2 per category.
	categoryNames := fake.MaintenanceCategories()
	var maintItems []MaintenanceItem
	for _, catName := range categoryNames {
		nItems := 1 + h.IntN(2)
		for range nItems {
			fm := h.MaintenanceItem(catName)
			item := MaintenanceItem{
				Name:           fm.Name,
				CategoryID:     catID(catName),
				IntervalMonths: fm.IntervalMonths,
				Notes:          fm.Notes,
				LastServicedAt: fm.LastServicedAt,
				CostCents:      fm.CostCents,
			}
			// Link appliance-related items to a random appliance.
			if catName == "Appliance" || catName == "HVAC" {
				ai := h.IntN(len(appliances))
				item.ApplianceID = &appliances[ai].ID
			}
			if err := s.db.Create(&item).Error; err != nil {
				return fmt.Errorf("seed maintenance %s: %w", item.Name, err)
			}
			maintItems = append(maintItems, item)
		}
	}

	// Service log entries: first 3 always get entries, rest ~60% chance.
	for i := range maintItems {
		if i >= 3 && h.IntN(10) >= 6 {
			continue
		}
		nEntries := 1 + h.IntN(3)
		for range nEntries {
			fe := h.ServiceLogEntry()
			entry := ServiceLogEntry{
				MaintenanceItemID: maintItems[i].ID,
				ServicedAt:        fe.ServicedAt,
				CostCents:         fe.CostCents,
				Notes:             fe.Notes,
			}
			if h.IntN(10) < 3 {
				vi := h.IntN(len(vendors))
				entry.VendorID = &vendors[vi].ID
			}
			if err := s.db.Create(&entry).Error; err != nil {
				return fmt.Errorf("seed service log: %w", err)
			}
		}
	}

	// Incidents: 2-3 items linked to appliances/vendors.
	incidents := make([]Incident, 0, 3)
	for i := range 3 {
		fi := h.Incident()
		inc := Incident{
			Title:       fi.Title,
			Description: fi.Description,
			Status:      fi.Status,
			Severity:    fi.Severity,
			DateNoticed: fi.DateNoticed,
			Location:    fi.Location,
			CostCents:   fi.CostCents,
		}
		if i < len(appliances) {
			inc.ApplianceID = &appliances[i].ID
		}
		if i < len(vendors) {
			inc.VendorID = &vendors[i].ID
		}
		if err := s.db.Create(&inc).Error; err != nil {
			return fmt.Errorf("seed incident %s: %w", inc.Title, err)
		}
		incidents = append(incidents, inc)
	}

	// Documents: attach a couple to projects, appliances, and an incident.
	type docSeed struct {
		title, fileName, mime, kind string
		entityID                    string
		ops                         []byte // pre-built extraction ops JSON
	}
	docSeeds := []docSeed{
		{
			"Invoice", "invoice.pdf", "application/pdf",
			DocumentEntityProject, projects[0].ID,
			demoExtractionOps("invoice"),
		},
		{
			"Contract", "contract.pdf", "application/pdf",
			DocumentEntityProject, projects[1].ID,
			demoExtractionOps("contract"),
		},
		{
			"Warranty Card",
			"warranty-card.jpg",
			"image/jpeg",
			DocumentEntityAppliance,
			appliances[0].ID,
			nil,
		},
		{
			"User Manual",
			"user-manual.pdf",
			"application/pdf",
			DocumentEntityAppliance,
			appliances[1].ID,
			nil,
		},
		{
			"Incident Photo",
			"incident-photo.jpg",
			"image/jpeg",
			DocumentEntityIncident,
			incidents[0].ID,
			nil,
		},
	}
	for _, ds := range docSeeds {
		// Small placeholder content -- demo only.
		content := []byte(ds.title + " placeholder content")
		doc := Document{
			Title:          ds.title,
			FileName:       ds.fileName,
			EntityKind:     ds.kind,
			EntityID:       ds.entityID,
			MIMEType:       ds.mime,
			SizeBytes:      int64(len(content)),
			ChecksumSHA256: fmt.Sprintf("%x", sha256.Sum256(content)),
			Data:           content,
			ExtractionOps:  ds.ops,
		}
		if err := s.db.Create(&doc).Error; err != nil {
			return fmt.Errorf("seed document %s: %w", ds.title, err)
		}
	}

	return nil
}

func (s *Store) HouseProfile() (HouseProfile, error) {
	var profile HouseProfile
	err := s.db.First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return HouseProfile{}, gorm.ErrRecordNotFound
	}
	return profile, err
}

func (s *Store) CreateHouseProfile(profile HouseProfile) error {
	var count int64
	if err := s.db.Model(&HouseProfile{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count house profiles: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("house profile already exists")
	}
	return s.db.Create(&profile).Error
}

func (s *Store) UpdateHouseProfile(profile HouseProfile) error {
	var existing HouseProfile
	if err := s.db.First(&existing).Error; err != nil {
		return err
	}
	profile.ID = existing.ID
	profile.CreatedAt = existing.CreatedAt
	if err := s.db.Model(&existing).Select("*").Updates(profile).Error; err != nil {
		return err
	}
	if !isSyncApplying(s.db) {
		return writeOplogEntry(s.db, TableHouseProfiles, profile.ID, OpUpdate, profile)
	}
	return nil
}

func (s *Store) ProjectTypes() ([]ProjectType, error) {
	var types []ProjectType
	if err := s.db.Order(ColName + " ASC, " + ColID + " DESC").Find(&types).Error; err != nil {
		return nil, err
	}
	return types, nil
}

func (s *Store) MaintenanceCategories() ([]MaintenanceCategory, error) {
	var categories []MaintenanceCategory
	if err := s.db.Order(ColName + " ASC, " + ColID + " DESC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

func (s *Store) ListVendors(includeDeleted bool) ([]Vendor, error) {
	return listQuery[Vendor](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Order(ColName + " ASC, " + ColID + " DESC")
	})
}

func (s *Store) GetVendor(id string) (Vendor, error) {
	return getByID[Vendor](s, id, identity)
}

func (s *Store) CreateVendor(vendor *Vendor) error {
	return s.db.Create(vendor).Error
}

// FindOrCreateVendor looks up a vendor by name. If found, updates its contact
// fields and returns it. If not found, creates a new one. Soft-deleted vendors
// with the same name are restored.
func (s *Store) FindOrCreateVendor(vendor Vendor) (Vendor, error) {
	return findOrCreateVendor(s.db, vendor)
}

// MaxIDs returns the current maximum ID (lexicographic for ULIDs) for each of
// the named tables. Tables with no rows are omitted from the result.
func (s *Store) MaxIDs(tables ...string) (map[string]string, error) {
	result := make(map[string]string, len(tables))
	for _, table := range tables {
		var maxID *string
		if err := s.db.Table(table).Select("MAX(id)").Scan(&maxID).Error; err != nil {
			return nil, fmt.Errorf("max id for %s: %w", table, err)
		}
		if maxID != nil {
			result[table] = *maxID
		}
	}
	return result, nil
}

// RowCounts returns the number of non-deleted rows for each of the named
// tables. Tables with no rows return 0.
func (s *Store) RowCounts(tables ...string) (map[string]int, error) {
	result := make(map[string]int, len(tables))
	for _, table := range tables {
		var count int64
		if err := s.db.Table(table).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("row count for %s: %w", table, err)
		}
		n, err := safeconv.Int(count)
		if err != nil {
			return nil, fmt.Errorf("row count for %s: %w", table, err)
		}
		result[table] = n
	}
	return result, nil
}

func (s *Store) UpdateVendor(vendor Vendor) error {
	return s.updateByID(TableVendors, &Vendor{}, vendor.ID, vendor)
}

// CountQuotesByVendor returns the number of non-deleted quotes per vendor ID.
func (s *Store) CountQuotesByVendor(vendorIDs []string) (map[string]int, error) {
	return s.countByFK(&Quote{}, ColVendorID, vendorIDs)
}

// CountServiceLogsByVendor returns the number of non-deleted service log entries per vendor ID.
func (s *Store) CountServiceLogsByVendor(vendorIDs []string) (map[string]int, error) {
	return s.countByFK(&ServiceLogEntry{}, ColVendorID, vendorIDs)
}

// CountQuotesByProject returns the number of non-deleted quotes per project ID.
func (s *Store) CountQuotesByProject(projectIDs []string) (map[string]int, error) {
	return s.countByFK(&Quote{}, ColProjectID, projectIDs)
}

// ListQuotesByVendor returns all quotes for a specific vendor.
func (s *Store) ListQuotesByVendor(vendorID string, includeDeleted bool) ([]Quote, error) {
	return listQuery[Quote](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Where(ColVendorID+" = ?", vendorID).
			Preload("Vendor", unscopedPreload).
			Preload("Project", unscopedPreload).
			Order(ColReceivedDate + " desc, " + ColID + " desc")
	})
}

// ListQuotesByProject returns all quotes for a specific project.
func (s *Store) ListQuotesByProject(projectID string, includeDeleted bool) ([]Quote, error) {
	return listQuery[Quote](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Where(ColProjectID+" = ?", projectID).
			Preload("Vendor", unscopedPreload).
			Preload("Project", unscopedPreload).
			Order(ColReceivedDate + " desc, " + ColID + " desc")
	})
}

// ListServiceLogsByVendor returns all service log entries for a specific vendor.
func (s *Store) ListServiceLogsByVendor(
	vendorID string,
	includeDeleted bool,
) ([]ServiceLogEntry, error) {
	return listQuery[ServiceLogEntry](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Where(ColVendorID+" = ?", vendorID).
			Preload("Vendor", unscopedPreload).
			Preload("MaintenanceItem", unscopedPreload).
			Order(ColServicedAt + " desc, " + ColID + " desc")
	})
}

func (s *Store) ListProjects(includeDeleted bool) ([]Project, error) {
	return listQuery[Project](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Preload("ProjectType").Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

func (s *Store) ListQuotes(includeDeleted bool) ([]Quote, error) {
	return listQuery[Quote](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Vendor", unscopedPreload).
			Preload("Project", func(q *gorm.DB) *gorm.DB {
				return q.Unscoped().Preload("ProjectType")
			}).
			Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

func (s *Store) ListMaintenance(includeDeleted bool) ([]MaintenanceItem, error) {
	return listQuery[MaintenanceItem](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Category").
			Preload("Appliance", unscopedPreload).
			Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

func (s *Store) ListMaintenanceByAppliance(
	applianceID string,
	includeDeleted bool,
) ([]MaintenanceItem, error) {
	return listQuery[MaintenanceItem](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Category").
			Where(ColApplianceID+" = ?", applianceID).
			Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

func (s *Store) GetProject(id string) (Project, error) {
	return getByID[Project](s, id, func(db *gorm.DB) *gorm.DB {
		return db.Preload("ProjectType")
	})
}

func (s *Store) CreateProject(project *Project) error {
	return s.db.Create(project).Error
}

func (s *Store) UpdateProject(project Project) error {
	return s.updateByID(TableProjects, &Project{}, project.ID, project)
}

func (s *Store) GetQuote(id string) (Quote, error) {
	return getByID[Quote](s, id, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Vendor", unscopedPreload).
			Preload("Project", func(q *gorm.DB) *gorm.DB {
				return q.Unscoped().Preload("ProjectType")
			})
	})
}

func (s *Store) CreateQuote(quote *Quote, vendor Vendor) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		foundVendor, err := findOrCreateVendor(tx, vendor)
		if err != nil {
			return err
		}
		quote.VendorID = foundVendor.ID
		return tx.Create(quote).Error
	})
}

func (s *Store) UpdateQuote(quote Quote, vendor Vendor) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		foundVendor, err := findOrCreateVendor(tx, vendor)
		if err != nil {
			return err
		}
		quote.VendorID = foundVendor.ID
		return updateByIDWith(tx, TableQuotes, &Quote{}, quote.ID, quote)
	})
}

func (s *Store) GetMaintenance(id string) (MaintenanceItem, error) {
	return getByID[MaintenanceItem](s, id, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Category").Preload("Appliance", unscopedPreload)
	})
}

func (s *Store) CreateMaintenance(item *MaintenanceItem) error {
	return s.db.Create(item).Error
}

// FindOrCreateMaintenance looks up a maintenance item by name and category.
// If found, returns it. If not found, creates a new one. Soft-deleted items
// with the same name+category are restored.
func (s *Store) FindOrCreateMaintenance(item MaintenanceItem) (MaintenanceItem, error) {
	return findOrCreate(s.db, item, item.Name, "maintenance item name",
		func(db *gorm.DB) *gorm.DB {
			return db.Where(ColName+" = ? AND "+ColCategoryID+" = ?", item.Name, item.CategoryID)
		},
		DeletionEntityMaintenance,
		func(m MaintenanceItem) string { return m.ID },
		func(m MaintenanceItem) bool { return m.DeletedAt.Valid },
	)
}

func (s *Store) UpdateMaintenance(item MaintenanceItem) error {
	return s.updateByID(TableMaintenanceItems, &MaintenanceItem{}, item.ID, item)
}

func (s *Store) ListAppliances(includeDeleted bool) ([]Appliance, error) {
	return listQuery[Appliance](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

func (s *Store) GetAppliance(id string) (Appliance, error) {
	return getByID[Appliance](s, id, identity)
}

func (s *Store) CreateAppliance(item *Appliance) error {
	return s.db.Create(item).Error
}

// FindOrCreateAppliance looks up an appliance by name. If found, returns it.
// If not found, creates a new one. Soft-deleted appliances with the same name
// are restored.
func (s *Store) FindOrCreateAppliance(item Appliance) (Appliance, error) {
	return findOrCreate(s.db, item, item.Name, "appliance name",
		func(db *gorm.DB) *gorm.DB {
			return db.Where(ColName+" = ?", item.Name)
		},
		DeletionEntityAppliance,
		func(a Appliance) string { return a.ID },
		func(a Appliance) bool { return a.DeletedAt.Valid },
	)
}

func (s *Store) UpdateAppliance(item Appliance) error {
	return s.updateByID(TableAppliances, &Appliance{}, item.ID, item)
}

// ---------------------------------------------------------------------------
// ServiceLogEntry CRUD
// ---------------------------------------------------------------------------

// syncLastServiced sets a maintenance item's LastServicedAt to the most recent
// ServicedAt from its non-deleted service log entries. If no entries exist the
// field is left unchanged, preserving any manually-set value.
func syncLastServiced(tx *gorm.DB, maintenanceItemID string) error {
	var latest ServiceLogEntry
	err := tx.Where(ColMaintenanceItemID+" = ?", maintenanceItemID).
		Order(ColServicedAt + " desc, " + ColID + " desc").
		First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return tx.Model(&MaintenanceItem{}).
		Where(ColID+" = ?", maintenanceItemID).
		Update(ColLastServicedAt, latest.ServicedAt).Error
}

func (s *Store) ListServiceLog(
	maintenanceItemID string,
	includeDeleted bool,
) ([]ServiceLogEntry, error) {
	return listQuery[ServiceLogEntry](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Where(ColMaintenanceItemID+" = ?", maintenanceItemID).
			Preload("Vendor", unscopedPreload).
			Order(ColServicedAt + " desc, " + ColID + " desc")
	})
}

func (s *Store) GetServiceLog(id string) (ServiceLogEntry, error) {
	return getByID[ServiceLogEntry](s, id, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Vendor", unscopedPreload)
	})
}

func (s *Store) CreateServiceLog(entry *ServiceLogEntry, vendor Vendor) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if strings.TrimSpace(vendor.Name) != "" {
			found, err := findOrCreateVendor(tx, vendor)
			if err != nil {
				return err
			}
			entry.VendorID = &found.ID
		}
		if err := tx.Create(entry).Error; err != nil {
			return err
		}
		return syncLastServiced(tx, entry.MaintenanceItemID)
	})
}

func (s *Store) UpdateServiceLog(entry ServiceLogEntry, vendor Vendor) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Fetch old entry to detect parent change.
		var old ServiceLogEntry
		if err := tx.First(&old, "id = ?", entry.ID).Error; err != nil {
			return err
		}
		if strings.TrimSpace(vendor.Name) != "" {
			found, err := findOrCreateVendor(tx, vendor)
			if err != nil {
				return err
			}
			entry.VendorID = &found.ID
		} else {
			entry.VendorID = nil
		}
		if err := updateByIDWith(tx, TableServiceLogEntries, &ServiceLogEntry{}, entry.ID, entry); err != nil {
			return err
		}
		// If the entry moved to a different parent, sync both.
		if old.MaintenanceItemID != entry.MaintenanceItemID {
			if err := syncLastServiced(tx, old.MaintenanceItemID); err != nil {
				return err
			}
		}
		return syncLastServiced(tx, entry.MaintenanceItemID)
	})
}

func (s *Store) DeleteServiceLog(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var entry ServiceLogEntry
		if err := tx.First(&entry, "id = ?", id).Error; err != nil {
			return err
		}
		if err := softDeleteWith(tx, &ServiceLogEntry{}, DeletionEntityServiceLog, id); err != nil {
			return err
		}
		return syncLastServiced(tx, entry.MaintenanceItemID)
	})
}

func (s *Store) RestoreServiceLog(id string) error {
	var entry ServiceLogEntry
	if err := s.db.Unscoped().First(&entry, "id = ?", id).Error; err != nil {
		return err
	}
	if err := s.checkParentsAlive([]parentCheck{
		{&MaintenanceItem{}, &entry.MaintenanceItemID, "maintenance item"},
		{&Vendor{}, entry.VendorID, "vendor"},
	}); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := restoreSoftDeleted(tx, &ServiceLogEntry{}, DeletionEntityServiceLog, id); err != nil {
			return err
		}
		return syncLastServiced(tx, entry.MaintenanceItemID)
	})
}

// CountServiceLogs returns the number of non-deleted service log entries per
// maintenance item ID for the given set of IDs.
func (s *Store) CountServiceLogs(itemIDs []string) (map[string]int, error) {
	return s.countByFK(&ServiceLogEntry{}, ColMaintenanceItemID, itemIDs)
}

// CountMaintenanceByAppliance returns the count of non-deleted maintenance
// items for each appliance ID.
func (s *Store) CountMaintenanceByAppliance(applianceIDs []string) (map[string]int, error) {
	return s.countByFK(&MaintenanceItem{}, ColApplianceID, applianceIDs)
}

// ---------------------------------------------------------------------------
// Incident CRUD
// ---------------------------------------------------------------------------

func (s *Store) ListIncidents(includeDeleted bool) ([]Incident, error) {
	return listQuery[Incident](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Appliance", unscopedPreload).
			Preload("Vendor", unscopedPreload).
			Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

func (s *Store) GetIncident(id string) (Incident, error) {
	return getByID[Incident](s, id, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Appliance", unscopedPreload).Preload("Vendor", unscopedPreload)
	})
}

func (s *Store) CreateIncident(item *Incident) error {
	return s.db.Create(item).Error
}

func (s *Store) UpdateIncident(item Incident) error {
	return s.updateByID(TableIncidents, &Incident{}, item.ID, item)
}

func (s *Store) DeleteIncident(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Read the full incident so we can log it and restore status later.
		var current Incident
		if err := tx.First(&current, "id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Model(&Incident{}).
			Where(ColID+" = ?", id).
			Updates(map[string]any{
				ColPreviousStatus: current.Status,
				ColStatus:         IncidentStatusResolved,
			}).Error; err != nil {
			return err
		}
		current.PreviousStatus = current.Status
		current.Status = IncidentStatusResolved
		result := tx.Delete(&current)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if !isSyncApplying(tx) {
			if err := writeOplogEntryRaw(tx, TableIncidents, id, OpDelete, "{}"); err != nil {
				return err
			}
		}
		return tx.Create(&DeletionRecord{
			Entity:    DeletionEntityIncident,
			TargetID:  id,
			DeletedAt: time.Now(),
		}).Error
	})
}

func (s *Store) RestoreIncident(id string) error {
	var item Incident
	if err := s.db.Unscoped().First(&item, "id = ?", id).Error; err != nil {
		return err
	}
	if item.ApplianceID != nil {
		if err := s.requireParentAlive(&Appliance{}, *item.ApplianceID); err != nil {
			return parentRestoreError("appliance", err)
		}
	}
	if item.VendorID != nil {
		if err := s.requireParentAlive(&Vendor{}, *item.VendorID); err != nil {
			return parentRestoreError("vendor", err)
		}
	}
	restoreStatus := item.PreviousStatus
	if restoreStatus == "" {
		restoreStatus = StructDefault[Incident]("Status")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Model(&Incident{}).
			Where(ColID+" = ?", id).
			Updates(map[string]any{
				ColDeletedAt:      nil,
				ColStatus:         restoreStatus,
				ColPreviousStatus: "",
			}).Error; err != nil {
			return err
		}

		// Write oplog "restore" entry explicitly (Unscoped().Updates()
		// does not fire model-level AfterUpdate hooks).
		if !isSyncApplying(tx) {
			if err := writeOplogEntryRaw(tx, TableIncidents, id, OpRestore, "{}"); err != nil {
				return err
			}
		}

		restoredAt := time.Now()
		return tx.Model(&DeletionRecord{}).
			Where(
				ColEntity+" = ? AND "+ColTargetID+" = ? AND "+ColRestoredAt+" IS NULL",
				DeletionEntityIncident, id,
			).
			Update(ColRestoredAt, restoredAt).Error
	})
}

func (s *Store) HardDeleteIncident(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Log oplog entries for detached documents before modifying them.
		if !isSyncApplying(tx) {
			var docs []Document
			if err := tx.Unscoped().
				Where(ColEntityKind+" = ? AND "+ColEntityID+" = ?", DocumentEntityIncident, id).
				Find(&docs).Error; err != nil {
				return err
			}
			for i := range docs {
				docs[i].EntityKind = DocumentEntityNone
				docs[i].EntityID = ""
				if err := writeOplogEntry(tx, TableDocuments, docs[i].ID, OpUpdate,
					newDocumentOplogPayload(docs[i])); err != nil {
					return err
				}
			}
		}

		// Detach linked documents (including soft-deleted ones) so they
		// survive the incident removal. Documents have intrinsic value
		// independent of the entity they were filed under.
		if err := tx.Unscoped().Model(&Document{}).
			Where(ColEntityKind+" = ? AND "+ColEntityID+" = ?", DocumentEntityIncident, id).
			Updates(map[string]any{
				ColEntityKind: DocumentEntityNone,
				ColEntityID:   "",
			}).Error; err != nil {
			return err
		}
		// Delete deletion records for this incident.
		if err := tx.
			Where(ColEntity+" = ? AND "+ColTargetID+" = ?", DeletionEntityIncident, id).
			Delete(&DeletionRecord{}).Error; err != nil {
			return err
		}

		// Write oplog "delete" entry before the hard-delete (which uses
		// an empty model instance that won't trigger hooks properly).
		if !isSyncApplying(tx) {
			if err := writeOplogEntryRaw(tx, TableIncidents, id, OpDelete, "{}"); err != nil {
				return err
			}
		}

		// Permanently remove the incident row.
		result := tx.Unscoped().Where("id = ?", id).Delete(&Incident{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *Store) CountIncidentsByAppliance(applianceIDs []string) (map[string]int, error) {
	return s.countByFK(&Incident{}, ColApplianceID, applianceIDs)
}

func (s *Store) CountIncidentsByVendor(vendorIDs []string) (map[string]int, error) {
	return s.countByFK(&Incident{}, ColVendorID, vendorIDs)
}

// ---------------------------------------------------------------------------
// Document CRUD
// ---------------------------------------------------------------------------

// listDocumentColumns are the columns selected when listing documents to
// avoid loading the potentially large Data BLOB.
var listDocumentColumns = []string{
	ColID, ColTitle, ColFileName, ColEntityKind, ColEntityID,
	ColMIMEType, ColSizeBytes, ColChecksumSHA256, ColExtractionModel,
	ColExtractionOps,
	ColNotes, ColCreatedAt, ColUpdatedAt, ColDeletedAt,
}

func (s *Store) ListDocuments(includeDeleted bool) ([]Document, error) {
	return listQuery[Document](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Select(listDocumentColumns).Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

// ListDocumentsByEntity returns documents scoped to a specific entity,
// excluding the BLOB data.
func (s *Store) ListDocumentsByEntity(
	entityKind string,
	entityID string,
	includeDeleted bool,
) ([]Document, error) {
	return listQuery[Document](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Select(listDocumentColumns).
			Where(ColEntityKind+" = ? AND "+ColEntityID+" = ?", entityKind, entityID).
			Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

// CountDocumentsByEntity counts non-deleted documents grouped by entity_id
// where entity_kind matches. Uses a custom query because documents use
// two-column polymorphic keys that countByFK can't handle.
func (s *Store) CountDocumentsByEntity(
	entityKind string,
	entityIDs []string,
) (map[string]int, error) {
	if len(entityIDs) == 0 {
		return map[string]int{}, nil
	}
	type row struct {
		FK    string `gorm:"column:fk"`
		Count int    `gorm:"column:cnt"`
	}
	var results []row
	err := s.db.Model(&Document{}).
		Select(ColEntityID+" as fk, count(*) as cnt").
		Where(ColEntityKind+" = ? AND "+ColEntityID+" IN ?", entityKind, entityIDs).
		Group(ColEntityID).
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(results))
	for _, r := range results {
		counts[r.FK] = r.Count
	}
	return counts, nil
}

func (s *Store) GetDocument(id string) (Document, error) {
	return getByID[Document](s, id, identity)
}

// GetDocumentMetadata loads a document by ID without the Data BLOB,
// ExtractData, or other heavy columns. Use GetDocument when you need
// the file bytes.
func (s *Store) GetDocumentMetadata(id string) (Document, error) {
	return getByID[Document](s, id, func(db *gorm.DB) *gorm.DB {
		return db.Select(metadataDocumentColumns)
	})
}

// PendingBlobDocuments returns documents that have a SHA-256 checksum
// (meaning they had file data at some point) but currently have no Data
// (blob not yet fetched from the relay). These are candidates for blob
// download during sync pull. Soft-deleted documents are automatically
// excluded by GORM's DeletedAt scoping (Document uses gorm.DeletedAt).
func (s *Store) PendingBlobDocuments() ([]Document, error) {
	var docs []Document
	err := s.db.Select(listDocumentColumns).
		Where("sha256 != '' AND data IS NULL").
		Order("updated_at DESC, id DESC").
		Find(&docs).Error
	return docs, err
}

// UpdateDocumentData sets the Data blob on an existing document by ID.
func (s *Store) UpdateDocumentData(id string, data []byte) error {
	return s.db.Model(&Document{}).Where("id = ?", id).Update("data", data).Error
}

// metadataDocumentColumns includes everything listDocumentColumns has
// plus ExtractedText, which callers like afterDocumentSave need for
// extraction decisions.
var metadataDocumentColumns = append(
	append([]string(nil), listDocumentColumns...),
	ColExtractedText,
)

func (s *Store) CreateDocument(doc *Document) error {
	if doc.SizeBytes > 0 &&
		uint64(doc.SizeBytes) > s.maxDocumentSize { //nolint:gosec // SizeBytes is non-negative here
		return fmt.Errorf(
			"file is too large (%s) -- maximum allowed is %s",
			humanize.IBytes(
				uint64(doc.SizeBytes), //nolint:gosec // SizeBytes checked positive above
			),
			humanize.IBytes(s.maxDocumentSize),
		)
	}
	return s.db.Create(doc).Error
}

// UpdateDocument persists changes to a document. Entity linkage (EntityID,
// EntityKind) is always preserved -- callers must use a dedicated method to
// re-link a document. When Data is empty the existing BLOB and file metadata
// columns are also preserved, so metadata-only edits don't erase the file.
func (s *Store) UpdateDocument(doc Document) error {
	omit := []string{ColID, ColCreatedAt, ColDeletedAt}
	if len(doc.Data) == 0 {
		omit = append(omit,
			ColFileName, ColMIMEType, ColSizeBytes,
			ColChecksumSHA256, ColData,
		)
	}
	if err := s.db.Model(&Document{}).Where(ColID+" = ?", doc.ID).
		Select("*").
		Omit(omit...).
		Updates(doc).Error; err != nil {
		return err
	}
	if !isSyncApplying(s.db) {
		return writeOplogEntry(s.db, TableDocuments, doc.ID, OpUpdate, newDocumentOplogPayload(doc))
	}
	return nil
}

// UpdateDocumentExtraction persists async extraction results on a document
// without touching other fields. Called from the extraction overlay after
// async extraction completes.
func (s *Store) UpdateDocumentExtraction(
	id string,
	text string,
	data []byte,
	model string,
	ops []byte,
) error {
	updates := make(map[string]any)
	if text != "" {
		updates[ColExtractedText] = text
	}
	if len(data) > 0 {
		updates[ColExtractData] = data
	}
	if model != "" {
		updates[ColExtractionModel] = model
	}
	if len(ops) > 0 {
		updates[ColExtractionOps] = ops
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.db.Unscoped().Model(&Document{}).Where(ColID+" = ?", id).Updates(updates).Error; err != nil {
		return err
	}
	// Extraction updates are logged so they sync to other devices.
	if !isSyncApplying(s.db) {
		var doc Document
		if err := s.db.Unscoped().First(&doc, "id = ?", id).Error; err != nil {
			return fmt.Errorf("re-read document for oplog: %w", err)
		}
		return writeOplogEntry(s.db, TableDocuments, id, OpUpdate, newDocumentOplogPayload(doc))
	}
	return nil
}

// EnsureDocumentAlive restores a soft-deleted document if necessary.
// Returns nil if the document is already alive or was successfully restored.
// Returns an error if the document does not exist or restoration fails
// (e.g. parent entity is also deleted).
func (s *Store) EnsureDocumentAlive(id string) error {
	var doc Document
	if err := s.db.First(&doc, "id = ?", id).Error; err == nil {
		return nil // already alive
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("check document %s: %w", id, err)
	}
	return s.RestoreDocument(id)
}

func (s *Store) DeleteDocument(id string) error {
	return s.softDelete(&Document{}, DeletionEntityDocument, id)
}

func (s *Store) RestoreDocument(id string) error {
	var doc Document
	if err := s.db.Unscoped().First(&doc, "id = ?", id).Error; err != nil {
		return err
	}
	if err := s.validateDocumentParent(doc); err != nil {
		return err
	}
	return s.restoreEntity(&Document{}, DeletionEntityDocument, id)
}

// validateDocumentParent checks that the document's parent entity is alive.
func (s *Store) validateDocumentParent(doc Document) error {
	switch doc.EntityKind {
	case DocumentEntityProject:
		if err := s.requireParentAlive(&Project{}, doc.EntityID); err != nil {
			return parentRestoreError("project", err)
		}
	case DocumentEntityAppliance:
		if err := s.requireParentAlive(&Appliance{}, doc.EntityID); err != nil {
			return parentRestoreError("appliance", err)
		}
	case DocumentEntityVendor:
		if err := s.requireParentAlive(&Vendor{}, doc.EntityID); err != nil {
			return parentRestoreError("vendor", err)
		}
	case DocumentEntityQuote:
		if err := s.requireParentAlive(&Quote{}, doc.EntityID); err != nil {
			return parentRestoreError("quote", err)
		}
	case DocumentEntityMaintenance:
		if err := s.requireParentAlive(&MaintenanceItem{}, doc.EntityID); err != nil {
			return parentRestoreError("maintenance item", err)
		}
	case DocumentEntityServiceLog:
		if err := s.requireParentAlive(&ServiceLogEntry{}, doc.EntityID); err != nil {
			return parentRestoreError("service log", err)
		}
	case DocumentEntityIncident:
		if err := s.requireParentAlive(&Incident{}, doc.EntityID); err != nil {
			return parentRestoreError("incident", err)
		}
	}
	return nil
}

// TitleFromFilename derives a human-friendly title from a filename by
// stripping extensions (including compound ones like .tar.gz), splitting on
// word boundaries via strcase, and title-casing each word.
func TitleFromFilename(name string) string {
	name = strings.TrimSpace(name)

	// Always strip the outermost extension (every file has one).
	if ext := filepath.Ext(name); ext != "" && ext != name {
		name = strings.TrimSuffix(name, ext)
	}

	// Continue stripping known compound-extension intermediaries
	// (e.g. .tar in .tar.gz, .tar.bz2, .tar.xz).
	for {
		ext := filepath.Ext(name)
		if ext == "" || ext == name {
			break
		}
		lower := strings.ToLower(ext)
		if lower != ".tar" {
			break
		}
		name = strings.TrimSuffix(name, ext)
	}

	// Split on word boundaries (camelCase, snake_case, kebab, dots).
	name = strcase.ToDelimited(name, ' ')
	name = strings.Join(strings.Fields(name), " ")

	// Title-case each word.
	runes := []rune(name)
	wordStart := true
	for i, r := range runes {
		if unicode.IsSpace(r) {
			wordStart = true
			continue
		}
		if wordStart {
			runes[i] = unicode.ToUpper(r)
		}
		wordStart = false
	}
	return string(runes)
}

func (s *Store) DeleteVendor(id string) error {
	if err := s.checkDependencies(id, []dependencyCheck{
		{&Quote{}, ColVendorID, "vendor has %d active quote(s) -- delete them first"},
		{&Incident{}, ColVendorID, "vendor has %d active incident(s) -- delete them first"},
	}); err != nil {
		return err
	}
	return s.softDelete(&Vendor{}, DeletionEntityVendor, id)
}

func (s *Store) RestoreVendor(id string) error {
	return s.restoreEntity(&Vendor{}, DeletionEntityVendor, id)
}

func (s *Store) DeleteProject(id string) error {
	if err := s.checkDependencies(id, []dependencyCheck{
		{&Quote{}, ColProjectID, "project has %d active quote(s) -- delete them first"},
	}); err != nil {
		return err
	}
	return s.softDelete(&Project{}, DeletionEntityProject, id)
}

func (s *Store) DeleteQuote(id string) error {
	return s.softDelete(&Quote{}, DeletionEntityQuote, id)
}

func (s *Store) DeleteMaintenance(id string) error {
	if err := s.checkDependencies(id, []dependencyCheck{
		{&ServiceLogEntry{}, ColMaintenanceItemID, "maintenance item has %d service log(s) -- delete them first"},
	}); err != nil {
		return err
	}
	return s.softDelete(&MaintenanceItem{}, DeletionEntityMaintenance, id)
}

// HardDeleteMaintenance permanently removes a maintenance item and its child
// service log entries. Before deleting, it writes oplog entries for each child
// and detaches any documents linked to service logs (documents have intrinsic
// value and survive parent deletion). Mirrors HardDeleteIncident.
func (s *Store) HardDeleteMaintenance(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Find all child service log entries (including soft-deleted).
		var logs []ServiceLogEntry
		if err := tx.Unscoped().
			Where(ColMaintenanceItemID+" = ?", id).
			Find(&logs).Error; err != nil {
			return err
		}

		if !isSyncApplying(tx) {
			// Detach documents linked to each service log and write oplog
			// entries for the detachment, then write delete entries for each
			// service log.
			for _, log := range logs {
				var docs []Document
				if err := tx.Unscoped().
					Where(ColEntityKind+" = ? AND "+ColEntityID+" = ?",
						DocumentEntityServiceLog, log.ID).
					Find(&docs).Error; err != nil {
					return err
				}
				for i := range docs {
					docs[i].EntityKind = DocumentEntityNone
					docs[i].EntityID = ""
					if err := writeOplogEntry(tx, TableDocuments, docs[i].ID, OpUpdate,
						newDocumentOplogPayload(docs[i])); err != nil {
						return err
					}
				}
				if err := writeOplogEntryRaw(tx, TableServiceLogEntries, log.ID, OpDelete, "{}"); err != nil {
					return err
				}
			}
		}

		// Detach documents from all service logs (persist the detachment).
		for _, log := range logs {
			if err := tx.Unscoped().Model(&Document{}).
				Where(ColEntityKind+" = ? AND "+ColEntityID+" = ?",
					DocumentEntityServiceLog, log.ID).
				Updates(map[string]any{
					ColEntityKind: DocumentEntityNone,
					ColEntityID:   "",
				}).Error; err != nil {
				return err
			}
		}

		// Delete deletion records for the maintenance item and its children.
		if err := tx.
			Where(ColEntity+" = ? AND "+ColTargetID+" = ?", DeletionEntityMaintenance, id).
			Delete(&DeletionRecord{}).Error; err != nil {
			return err
		}
		for _, log := range logs {
			if err := tx.
				Where(ColEntity+" = ? AND "+ColTargetID+" = ?", DeletionEntityServiceLog, log.ID).
				Delete(&DeletionRecord{}).Error; err != nil {
				return err
			}
		}

		// Write oplog delete entry for the maintenance item itself.
		if !isSyncApplying(tx) {
			if err := writeOplogEntryRaw(tx, TableMaintenanceItems, id, OpDelete, "{}"); err != nil {
				return err
			}
		}

		// Permanently remove the maintenance item (CASCADE deletes children).
		result := tx.Unscoped().Where("id = ?", id).Delete(&MaintenanceItem{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *Store) DeleteAppliance(id string) error {
	if err := s.checkDependencies(id, []dependencyCheck{
		{&MaintenanceItem{}, ColApplianceID, "appliance has %d active maintenance item(s) -- delete or reassign them first"},
		{&Incident{}, ColApplianceID, "appliance has %d active incident(s) -- delete them first"},
	}); err != nil {
		return err
	}
	return s.softDelete(&Appliance{}, DeletionEntityAppliance, id)
}

func (s *Store) RestoreProject(id string) error {
	return s.restoreEntity(&Project{}, DeletionEntityProject, id)
}

func (s *Store) RestoreQuote(id string) error {
	var quote Quote
	if err := s.db.Unscoped().First(&quote, "id = ?", id).Error; err != nil {
		return err
	}
	if err := s.checkParentsAlive([]parentCheck{
		{&Project{}, &quote.ProjectID, "project"},
		{&Vendor{}, &quote.VendorID, "vendor"},
	}); err != nil {
		return err
	}
	return s.restoreEntity(&Quote{}, DeletionEntityQuote, id)
}

func (s *Store) RestoreMaintenance(id string) error {
	var item MaintenanceItem
	if err := s.db.Unscoped().First(&item, "id = ?", id).Error; err != nil {
		return err
	}
	if err := s.checkParentsAlive([]parentCheck{
		{&Appliance{}, item.ApplianceID, "appliance"},
	}); err != nil {
		return err
	}
	return s.restoreEntity(&MaintenanceItem{}, DeletionEntityMaintenance, id)
}

func (s *Store) RestoreAppliance(id string) error {
	return s.restoreEntity(&Appliance{}, DeletionEntityAppliance, id)
}

var (
	// ErrParentDeleted indicates the parent record exists but is soft-deleted.
	ErrParentDeleted = errors.New("parent record is deleted")
	// ErrParentNotFound indicates the parent record doesn't exist at all.
	ErrParentNotFound = errors.New("parent record not found")
)

// requireParentAlive returns ErrParentDeleted if the parent record is
// soft-deleted, or ErrParentNotFound if it doesn't exist at all. Returns nil
// when the parent is alive.
func (s *Store) requireParentAlive(model any, id string) error {
	err := s.db.First(model, "id = ?", id).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// Distinguish soft-deleted from truly missing.
	if err := s.db.Unscoped().First(model, "id = ?", id).Error; err != nil {
		return ErrParentNotFound
	}
	return ErrParentDeleted
}

// parentRestoreError returns a user-facing error message for a failed parent
// alive check, distinguishing soft-deleted parents (restorable) from missing
// parents (permanently gone).
// parentCheck describes a parent FK that must be alive before a child can be
// restored. A nil id means the FK is optional and unset; the check is skipped.
type parentCheck struct {
	model any // GORM model pointer -- typed as any because gorm.DB.First accepts any
	id    *string
	label string
}

func (s *Store) checkParentsAlive(checks []parentCheck) error {
	for _, c := range checks {
		if c.id == nil {
			continue
		}
		if err := s.requireParentAlive(c.model, *c.id); err != nil {
			return parentRestoreError(c.label, err)
		}
	}
	return nil
}

func parentRestoreError(entity string, err error) error {
	if errors.Is(err, ErrParentNotFound) {
		return fmt.Errorf("%s no longer exists", entity)
	}
	return fmt.Errorf("%s is deleted -- restore it first", entity)
}

// countDependents counts non-deleted rows in model where fkColumn equals id.
// GORM's soft-delete scope automatically excludes deleted rows.
func (s *Store) countDependents(model any, fkColumn string, id string) (int64, error) {
	var count int64
	err := s.db.Model(model).Where(fkColumn+" = ?", id).Count(&count).Error
	return count, err
}

func softDeleteWith(tx *gorm.DB, model any, entity string, id string) error {
	result := tx.Where("id = ?", id).Delete(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	// Write oplog "delete" entry for the soft-deleted entity.
	if table := deletionEntityToTable[entity]; table != "" && !isSyncApplying(tx) {
		if err := writeOplogEntryRaw(tx, table, id, OpDelete, "{}"); err != nil {
			return err
		}
	}

	record := DeletionRecord{
		Entity:    entity,
		TargetID:  id,
		DeletedAt: time.Now(),
	}
	return tx.Create(&record).Error
}

func (s *Store) softDelete(model any, entity string, id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return softDeleteWith(tx, model, entity, id)
	})
}

// restoreSoftDeleted clears a record's soft-delete timestamp, marks the
// corresponding DeletionRecord as restored, and writes a "restore" oplog
// entry. Callers that need transactional guarantees should pass a *gorm.DB
// obtained from db.Transaction.
func restoreSoftDeleted(tx *gorm.DB, model any, entity string, id string) error {
	if err := tx.Unscoped().Model(model).
		Where(ColID+" = ?", id).
		Update(ColDeletedAt, nil).Error; err != nil {
		return err
	}

	// Write oplog "restore" entry. GORM's Unscoped().Update() does not
	// trigger model-level AfterUpdate hooks, so we must do this explicitly.
	if table := deletionEntityToTable[entity]; table != "" && !isSyncApplying(tx) {
		if err := writeOplogEntryRaw(tx, table, id, OpRestore, "{}"); err != nil {
			return err
		}
	}

	restoredAt := time.Now()
	return tx.Model(&DeletionRecord{}).
		Where(
			ColEntity+" = ? AND "+ColTargetID+" = ? AND "+ColRestoredAt+" IS NULL",
			entity, id,
		).
		Update(ColRestoredAt, restoredAt).Error
}

func (s *Store) restoreEntity(model any, entity string, id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return restoreSoftDeleted(tx, model, entity, id)
	})
}

func (s *Store) LastDeletion(entity string) (DeletionRecord, error) {
	var record DeletionRecord
	err := s.db.
		Where(ColEntity+" = ? AND "+ColRestoredAt+" IS NULL", entity).
		Order(ColID + " desc").
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DeletionRecord{}, gorm.ErrRecordNotFound
	}
	return record, err
}

func (s *Store) seedProjectTypes() error {
	types := []ProjectType{
		{Name: "Appliance"},
		{Name: "Electrical"},
		{Name: "Exterior"},
		{Name: "Flooring"},
		{Name: "HVAC"},
		{Name: "Landscaping"},
		{Name: "Painting"},
		{Name: "Plumbing"},
		{Name: "Remodel"},
		{Name: "Roof"},
		{Name: "Structural"},
		{Name: "Windows"},
	}
	for _, projectType := range types {
		if err := s.db.FirstOrCreate(&projectType, ColName+" = ?", projectType.Name).
			Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedMaintenanceCategories() error {
	categories := []MaintenanceCategory{
		{Name: "Appliance"},
		{Name: "Electrical"},
		{Name: "Exterior"},
		{Name: "HVAC"},
		{Name: "Interior"},
		{Name: "Landscaping"},
		{Name: "Plumbing"},
		{Name: "Safety"},
		{Name: "Structural"},
	}
	for _, category := range categories {
		if err := s.db.FirstOrCreate(&category, ColName+" = ?", category.Name).
			Error; err != nil {
			return err
		}
	}
	return nil
}

// countByFK groups rows in model by fkColumn and returns a count per FK value.
// Only non-deleted rows are counted (soft-delete scope applies automatically).
func (s *Store) countByFK(model any, fkColumn string, ids []string) (map[string]int, error) {
	if len(ids) == 0 {
		return map[string]int{}, nil
	}
	type row struct {
		FK    string `gorm:"column:fk"`
		Count int    `gorm:"column:cnt"`
	}
	var results []row
	err := s.db.Model(model).
		Select(fkColumn+" as fk, count(*) as cnt").
		Where(fkColumn+" IN ?", ids).
		Group(fkColumn).
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(results))
	for _, r := range results {
		counts[r.FK] = r.Count
	}
	return counts, nil
}

// updateByIDWith updates a record by ID, preserving id, created_at, and
// deleted_at. Works with both Store.db and transaction handles.
// Writes an "update" oplog entry with the new values as payload.
func updateByIDWith(db *gorm.DB, table string, model any, id string, values any) error {
	if err := db.Model(model).Where(ColID+" = ?", id).
		Select("*").
		Omit(ColID, ColCreatedAt, ColDeletedAt).
		Updates(values).Error; err != nil {
		return err
	}
	if !isSyncApplying(db) {
		return writeOplogEntry(db, table, id, OpUpdate, values)
	}
	return nil
}

func (s *Store) updateByID(table string, model any, id string, values any) error {
	return updateByIDWith(s.db, table, model, id, values)
}

func findOrCreateVendor(tx *gorm.DB, vendor Vendor) (Vendor, error) {
	existing, err := findOrCreate(tx, vendor, vendor.Name, "vendor name",
		func(db *gorm.DB) *gorm.DB { return db.Where(ColName+" = ?", vendor.Name) },
		DeletionEntityVendor,
		func(v Vendor) string { return v.ID },
		func(v Vendor) bool { return v.DeletedAt.Valid },
	)
	if err != nil {
		return Vendor{}, err
	}
	// Overwrite contact fields so callers can clear them (e.g. user
	// blanks a phone number in the quote form). For newly created
	// vendors this is a no-op since the fields already match.
	updates := map[string]any{
		ColContactName: vendor.ContactName,
		ColEmail:       vendor.Email,
		ColPhone:       vendor.Phone,
		ColWebsite:     vendor.Website,
		ColNotes:       vendor.Notes,
	}
	if err := tx.Model(&existing).Updates(updates).Error; err != nil {
		return Vendor{}, err
	}
	// Re-read so the returned struct reflects the persisted state.
	if err := tx.First(&existing, "id = ?", existing.ID).Error; err != nil {
		return Vendor{}, err
	}
	return existing, nil
}
