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
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db              *gorm.DB
	maxDocumentSize uint64
	currency        locale.Currency
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
	return &Store{db: db, maxDocumentSize: MaxDocumentSize}, nil
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

// Close closes the underlying database connection.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("get underlying db: %w", err)
	}
	return sqlDB.Close()
}

// IsMicasaDB returns true if the database contains the core micasa tables.
func (s *Store) IsMicasaDB() (bool, error) {
	var count int64
	err := s.db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN (?, ?, ?)`,
		"vendors", "projects", "appliances",
	).Scan(&count).Error
	if err != nil {
		return false, err
	}
	return count == 3, nil
}

func (s *Store) AutoMigrate() error {
	return s.db.AutoMigrate(
		&HouseProfile{},
		&ProjectType{},
		&Vendor{},
		&Project{},
		&Quote{},
		&MaintenanceCategory{},
		&Appliance{},
		&MaintenanceItem{},
		&ServiceLogEntry{},
		&Incident{},
		&Document{},
		&DeletionRecord{},
		&Setting{},
		&ChatInput{},
	)
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
	typeID := func(name string) uint {
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
	catID := func(name string) uint {
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
		for q := 0; q < nQuotes; q++ {
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
		for j := 0; j < nItems; j++ {
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
		for j := 0; j < nEntries; j++ {
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
	for i := 0; i < 3; i++ {
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
		entityID                    uint
	}
	docSeeds := []docSeed{
		{"Invoice", "invoice.pdf", "application/pdf", DocumentEntityProject, projects[0].ID},
		{"Contract", "contract.pdf", "application/pdf", DocumentEntityProject, projects[1].ID},
		{
			"Warranty Card",
			"warranty-card.jpg",
			"image/jpeg",
			DocumentEntityAppliance,
			appliances[0].ID,
		},
		{
			"User Manual",
			"user-manual.pdf",
			"application/pdf",
			DocumentEntityAppliance,
			appliances[1].ID,
		},
		{
			"Incident Photo",
			"incident-photo.jpg",
			"image/jpeg",
			DocumentEntityIncident,
			incidents[0].ID,
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
	return s.db.Model(&existing).Select("*").Updates(profile).Error
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
	var vendors []Vendor
	db := s.db.Order(ColName + " ASC, " + ColID + " DESC")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&vendors).Error; err != nil {
		return nil, err
	}
	return vendors, nil
}

func (s *Store) GetVendor(id uint) (Vendor, error) {
	var vendor Vendor
	if err := s.db.First(&vendor, id).Error; err != nil {
		return Vendor{}, err
	}
	return vendor, nil
}

func (s *Store) CreateVendor(vendor *Vendor) error {
	return s.db.Create(vendor).Error
}

func (s *Store) UpdateVendor(vendor Vendor) error {
	return s.updateByID(&Vendor{}, vendor.ID, vendor)
}

// CountQuotesByVendor returns the number of non-deleted quotes per vendor ID.
func (s *Store) CountQuotesByVendor(vendorIDs []uint) (map[uint]int, error) {
	return s.countByFK(&Quote{}, ColVendorID, vendorIDs)
}

// CountServiceLogsByVendor returns the number of non-deleted service log entries per vendor ID.
func (s *Store) CountServiceLogsByVendor(vendorIDs []uint) (map[uint]int, error) {
	return s.countByFK(&ServiceLogEntry{}, ColVendorID, vendorIDs)
}

// CountQuotesByProject returns the number of non-deleted quotes per project ID.
func (s *Store) CountQuotesByProject(projectIDs []uint) (map[uint]int, error) {
	return s.countByFK(&Quote{}, ColProjectID, projectIDs)
}

// ListQuotesByVendor returns all quotes for a specific vendor.
func (s *Store) ListQuotesByVendor(
	vendorID uint,
	includeDeleted bool,
) ([]Quote, error) {
	var quotes []Quote
	db := s.db.Where(ColVendorID+" = ?", vendorID).
		Preload("Vendor", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Preload("Project", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Order(ColReceivedDate + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&quotes).Error; err != nil {
		return nil, err
	}
	return quotes, nil
}

// ListQuotesByProject returns all quotes for a specific project.
func (s *Store) ListQuotesByProject(
	projectID uint,
	includeDeleted bool,
) ([]Quote, error) {
	var quotes []Quote
	db := s.db.Where(ColProjectID+" = ?", projectID).
		Preload("Vendor", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Preload("Project", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Order(ColReceivedDate + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&quotes).Error; err != nil {
		return nil, err
	}
	return quotes, nil
}

// ListServiceLogsByVendor returns all service log entries for a specific vendor.
func (s *Store) ListServiceLogsByVendor(
	vendorID uint,
	includeDeleted bool,
) ([]ServiceLogEntry, error) {
	var entries []ServiceLogEntry
	db := s.db.Where(ColVendorID+" = ?", vendorID).
		Preload("Vendor", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Preload("MaintenanceItem", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Order(ColServicedAt + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) ListProjects(includeDeleted bool) ([]Project, error) {
	var projects []Project
	db := s.db.Preload("ProjectType").Order(ColUpdatedAt + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (s *Store) ListQuotes(includeDeleted bool) ([]Quote, error) {
	var quotes []Quote
	db := s.db.Preload("Vendor", func(q *gorm.DB) *gorm.DB {
		return q.Unscoped()
	})
	db = db.Preload("Project", func(q *gorm.DB) *gorm.DB {
		return q.Unscoped().Preload("ProjectType")
	})
	db = db.Order(ColUpdatedAt + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&quotes).Error; err != nil {
		return nil, err
	}
	return quotes, nil
}

func (s *Store) ListMaintenance(includeDeleted bool) ([]MaintenanceItem, error) {
	var items []MaintenanceItem
	db := s.db.Preload("Category")
	db = db.Preload("Appliance", func(q *gorm.DB) *gorm.DB {
		return q.Unscoped()
	})
	db = db.Order(ColUpdatedAt + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) ListMaintenanceByAppliance(
	applianceID uint,
	includeDeleted bool,
) ([]MaintenanceItem, error) {
	var items []MaintenanceItem
	db := s.db.Preload("Category").
		Where(ColApplianceID+" = ?", applianceID).
		Order(ColUpdatedAt + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) GetProject(id uint) (Project, error) {
	var project Project
	err := s.db.Preload("ProjectType").First(&project, id).Error
	return project, err
}

func (s *Store) CreateProject(project *Project) error {
	return s.db.Create(project).Error
}

func (s *Store) UpdateProject(project Project) error {
	return s.updateByID(&Project{}, project.ID, project)
}

func (s *Store) GetQuote(id uint) (Quote, error) {
	var quote Quote
	err := s.db.Preload("Vendor", func(q *gorm.DB) *gorm.DB {
		return q.Unscoped()
	}).Preload("Project", func(q *gorm.DB) *gorm.DB {
		return q.Unscoped().Preload("ProjectType")
	}).
		First(&quote, id).Error
	return quote, err
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
		return updateByIDWith(tx, &Quote{}, quote.ID, quote)
	})
}

func (s *Store) GetMaintenance(id uint) (MaintenanceItem, error) {
	var item MaintenanceItem
	err := s.db.Preload("Category").
		Preload("Appliance", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		First(&item, id).Error
	return item, err
}

func (s *Store) CreateMaintenance(item *MaintenanceItem) error {
	return s.db.Create(item).Error
}

func (s *Store) UpdateMaintenance(item MaintenanceItem) error {
	return s.updateByID(&MaintenanceItem{}, item.ID, item)
}

func (s *Store) ListAppliances(includeDeleted bool) ([]Appliance, error) {
	var items []Appliance
	db := s.db.Order(ColUpdatedAt + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) GetAppliance(id uint) (Appliance, error) {
	var item Appliance
	err := s.db.First(&item, id).Error
	return item, err
}

func (s *Store) CreateAppliance(item *Appliance) error {
	return s.db.Create(item).Error
}

func (s *Store) UpdateAppliance(item Appliance) error {
	return s.updateByID(&Appliance{}, item.ID, item)
}

// ---------------------------------------------------------------------------
// ServiceLogEntry CRUD
// ---------------------------------------------------------------------------

func (s *Store) ListServiceLog(
	maintenanceItemID uint,
	includeDeleted bool,
) ([]ServiceLogEntry, error) {
	var entries []ServiceLogEntry
	db := s.db.Where(ColMaintenanceItemID+" = ?", maintenanceItemID).
		Preload("Vendor", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Order(ColServicedAt + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) GetServiceLog(id uint) (ServiceLogEntry, error) {
	var entry ServiceLogEntry
	err := s.db.Preload("Vendor", func(q *gorm.DB) *gorm.DB {
		return q.Unscoped()
	}).First(&entry, id).Error
	return entry, err
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
		return tx.Create(entry).Error
	})
}

func (s *Store) UpdateServiceLog(entry ServiceLogEntry, vendor Vendor) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if strings.TrimSpace(vendor.Name) != "" {
			found, err := findOrCreateVendor(tx, vendor)
			if err != nil {
				return err
			}
			entry.VendorID = &found.ID
		} else {
			entry.VendorID = nil
		}
		return updateByIDWith(tx, &ServiceLogEntry{}, entry.ID, entry)
	})
}

func (s *Store) DeleteServiceLog(id uint) error {
	return s.softDelete(&ServiceLogEntry{}, DeletionEntityServiceLog, id)
}

func (s *Store) RestoreServiceLog(id uint) error {
	var entry ServiceLogEntry
	if err := s.db.Unscoped().First(&entry, id).Error; err != nil {
		return err
	}
	if err := s.requireParentAlive(&MaintenanceItem{}, entry.MaintenanceItemID); err != nil {
		return parentRestoreError("maintenance item", err)
	}
	if entry.VendorID != nil {
		if err := s.requireParentAlive(&Vendor{}, *entry.VendorID); err != nil {
			return parentRestoreError("vendor", err)
		}
	}
	return s.restoreEntity(&ServiceLogEntry{}, DeletionEntityServiceLog, id)
}

// CountServiceLogs returns the number of non-deleted service log entries per
// maintenance item ID for the given set of IDs.
func (s *Store) CountServiceLogs(itemIDs []uint) (map[uint]int, error) {
	return s.countByFK(&ServiceLogEntry{}, ColMaintenanceItemID, itemIDs)
}

// CountMaintenanceByAppliance returns the count of non-deleted maintenance
// items for each appliance ID.
func (s *Store) CountMaintenanceByAppliance(applianceIDs []uint) (map[uint]int, error) {
	return s.countByFK(&MaintenanceItem{}, ColApplianceID, applianceIDs)
}

// ---------------------------------------------------------------------------
// Incident CRUD
// ---------------------------------------------------------------------------

func (s *Store) ListIncidents(includeDeleted bool) ([]Incident, error) {
	var items []Incident
	db := s.db.
		Preload("Appliance", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		Preload("Vendor", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		Order(ColUpdatedAt + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	return items, db.Find(&items).Error
}

func (s *Store) GetIncident(id uint) (Incident, error) {
	var item Incident
	err := s.db.
		Preload("Appliance", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		Preload("Vendor", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		First(&item, id).Error
	return item, err
}

func (s *Store) CreateIncident(item *Incident) error {
	return s.db.Create(item).Error
}

func (s *Store) UpdateIncident(item Incident) error {
	return s.updateByID(&Incident{}, item.ID, item)
}

func (s *Store) DeleteIncident(id uint) error {
	return s.softDelete(&Incident{}, DeletionEntityIncident, id)
}

func (s *Store) RestoreIncident(id uint) error {
	var item Incident
	if err := s.db.Unscoped().First(&item, id).Error; err != nil {
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
	return s.restoreEntity(&Incident{}, DeletionEntityIncident, id)
}

func (s *Store) CountIncidentsByAppliance(applianceIDs []uint) (map[uint]int, error) {
	return s.countByFK(&Incident{}, ColApplianceID, applianceIDs)
}

func (s *Store) CountIncidentsByVendor(vendorIDs []uint) (map[uint]int, error) {
	return s.countByFK(&Incident{}, ColVendorID, vendorIDs)
}

// ---------------------------------------------------------------------------
// Document CRUD
// ---------------------------------------------------------------------------

// listDocumentColumns are the columns selected when listing documents to
// avoid loading the potentially large Data BLOB.
var listDocumentColumns = []string{
	ColID, ColTitle, ColFileName, ColEntityKind, ColEntityID,
	ColMIMEType, ColSizeBytes, ColChecksum, ColNotes,
	ColCreatedAt, ColUpdatedAt, ColDeletedAt,
}

func (s *Store) ListDocuments(includeDeleted bool) ([]Document, error) {
	var docs []Document
	db := s.db.Select(listDocumentColumns).Order(ColUpdatedAt + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&docs).Error; err != nil {
		return nil, err
	}
	return docs, nil
}

// ListDocumentsByEntity returns documents scoped to a specific entity,
// excluding the BLOB data.
func (s *Store) ListDocumentsByEntity(
	entityKind string,
	entityID uint,
	includeDeleted bool,
) ([]Document, error) {
	var docs []Document
	db := s.db.Select(listDocumentColumns).
		Where(ColEntityKind+" = ? AND "+ColEntityID+" = ?", entityKind, entityID).
		Order(ColUpdatedAt + " desc, " + ColID + " desc")
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&docs).Error; err != nil {
		return nil, err
	}
	return docs, nil
}

// CountDocumentsByEntity counts non-deleted documents grouped by entity_id
// where entity_kind matches. Uses a custom query because documents use
// two-column polymorphic keys that countByFK can't handle.
func (s *Store) CountDocumentsByEntity(
	entityKind string,
	entityIDs []uint,
) (map[uint]int, error) {
	if len(entityIDs) == 0 {
		return map[uint]int{}, nil
	}
	type row struct {
		FK    uint `gorm:"column:fk"`
		Count int  `gorm:"column:cnt"`
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
	counts := make(map[uint]int, len(results))
	for _, r := range results {
		counts[r.FK] = r.Count
	}
	return counts, nil
}

func (s *Store) GetDocument(id uint) (Document, error) {
	var doc Document
	if err := s.db.First(&doc, id).Error; err != nil {
		return Document{}, err
	}
	return doc, nil
}

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
			ColChecksum, ColData,
		)
	}
	return s.db.Model(&Document{}).Where(ColID+" = ?", doc.ID).
		Select("*").
		Omit(omit...).
		Updates(doc).Error
}

// UpdateDocumentExtraction persists async extraction results on a document
// without touching other fields. Called from the extraction overlay after
// async extraction completes.
func (s *Store) UpdateDocumentExtraction(id uint, text string, data []byte) error {
	updates := map[string]any{
		ColExtractData: data,
	}
	if text != "" {
		updates[ColExtractedText] = text
	}
	return s.db.Model(&Document{}).Where(ColID+" = ?", id).Updates(updates).Error
}

func (s *Store) DeleteDocument(id uint) error {
	return s.softDelete(&Document{}, DeletionEntityDocument, id)
}

func (s *Store) RestoreDocument(id uint) error {
	var doc Document
	if err := s.db.Unscoped().First(&doc, id).Error; err != nil {
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

func (s *Store) DeleteVendor(id uint) error {
	n, err := s.countDependents(&Quote{}, ColVendorID, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("vendor has %d active quote(s) -- delete them first", n)
	}
	ni, err := s.countDependents(&Incident{}, ColVendorID, id)
	if err != nil {
		return err
	}
	if ni > 0 {
		return fmt.Errorf("vendor has %d active incident(s) -- delete them first", ni)
	}
	return s.softDelete(&Vendor{}, DeletionEntityVendor, id)
}

func (s *Store) RestoreVendor(id uint) error {
	return s.restoreEntity(&Vendor{}, DeletionEntityVendor, id)
}

func (s *Store) DeleteProject(id uint) error {
	n, err := s.countDependents(&Quote{}, ColProjectID, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("project has %d active quote(s) -- delete them first", n)
	}
	return s.softDelete(&Project{}, DeletionEntityProject, id)
}

func (s *Store) DeleteQuote(id uint) error {
	return s.softDelete(&Quote{}, DeletionEntityQuote, id)
}

func (s *Store) DeleteMaintenance(id uint) error {
	n, err := s.countDependents(&ServiceLogEntry{}, ColMaintenanceItemID, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("maintenance item has %d service log(s) -- delete them first", n)
	}
	return s.softDelete(&MaintenanceItem{}, DeletionEntityMaintenance, id)
}

func (s *Store) DeleteAppliance(id uint) error {
	n, err := s.countDependents(&MaintenanceItem{}, ColApplianceID, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf(
			"appliance has %d active maintenance item(s) -- delete or reassign them first",
			n,
		)
	}
	ni, err := s.countDependents(&Incident{}, ColApplianceID, id)
	if err != nil {
		return err
	}
	if ni > 0 {
		return fmt.Errorf("appliance has %d active incident(s) -- delete them first", ni)
	}
	return s.softDelete(&Appliance{}, DeletionEntityAppliance, id)
}

func (s *Store) RestoreProject(id uint) error {
	var project Project
	if err := s.db.Unscoped().First(&project, id).Error; err != nil {
		return err
	}
	return s.restoreEntity(&Project{}, DeletionEntityProject, id)
}

func (s *Store) RestoreQuote(id uint) error {
	var quote Quote
	if err := s.db.Unscoped().First(&quote, id).Error; err != nil {
		return err
	}
	if err := s.requireParentAlive(&Project{}, quote.ProjectID); err != nil {
		return parentRestoreError("project", err)
	}
	if err := s.requireParentAlive(&Vendor{}, quote.VendorID); err != nil {
		return parentRestoreError("vendor", err)
	}
	return s.restoreEntity(&Quote{}, DeletionEntityQuote, id)
}

func (s *Store) RestoreMaintenance(id uint) error {
	var item MaintenanceItem
	if err := s.db.Unscoped().First(&item, id).Error; err != nil {
		return err
	}
	if item.ApplianceID != nil {
		if err := s.requireParentAlive(&Appliance{}, *item.ApplianceID); err != nil {
			return parentRestoreError("appliance", err)
		}
	}
	return s.restoreEntity(&MaintenanceItem{}, DeletionEntityMaintenance, id)
}

func (s *Store) RestoreAppliance(id uint) error {
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
func (s *Store) requireParentAlive(model any, id uint) error {
	err := s.db.First(model, id).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// Distinguish soft-deleted from truly missing.
	if err := s.db.Unscoped().First(model, id).Error; err != nil {
		return ErrParentNotFound
	}
	return ErrParentDeleted
}

// parentRestoreError returns a user-facing error message for a failed parent
// alive check, distinguishing soft-deleted parents (restorable) from missing
// parents (permanently gone).
func parentRestoreError(entity string, err error) error {
	if errors.Is(err, ErrParentNotFound) {
		return fmt.Errorf("%s no longer exists", entity)
	}
	return fmt.Errorf("%s is deleted -- restore it first", entity)
}

// countDependents counts non-deleted rows in model where fkColumn equals id.
// GORM's soft-delete scope automatically excludes deleted rows.
func (s *Store) countDependents(model any, fkColumn string, id uint) (int64, error) {
	var count int64
	err := s.db.Model(model).Where(fkColumn+" = ?", id).Count(&count).Error
	return count, err
}

func (s *Store) softDelete(model any, entity string, id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Delete(model, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		record := DeletionRecord{
			Entity:    entity,
			TargetID:  id,
			DeletedAt: time.Now(),
		}
		return tx.Create(&record).Error
	})
}

func (s *Store) restoreEntity(model any, entity string, id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Model(model).
			Where(ColID+" = ?", id).
			Update(ColDeletedAt, nil).Error; err != nil {
			return err
		}
		restoredAt := time.Now()
		return tx.Model(&DeletionRecord{}).
			Where(
				ColEntity+" = ? AND "+ColTargetID+" = ? AND "+ColRestoredAt+" IS NULL",
				entity, id,
			).
			Update(ColRestoredAt, restoredAt).Error
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
func (s *Store) countByFK(model any, fkColumn string, ids []uint) (map[uint]int, error) {
	if len(ids) == 0 {
		return map[uint]int{}, nil
	}
	type row struct {
		FK    uint `gorm:"column:fk"`
		Count int  `gorm:"column:cnt"`
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
	counts := make(map[uint]int, len(results))
	for _, r := range results {
		counts[r.FK] = r.Count
	}
	return counts, nil
}

// updateByIDWith updates a record by ID, preserving id, created_at, and
// deleted_at. Works with both Store.db and transaction handles.
func updateByIDWith(db *gorm.DB, model any, id uint, values any) error {
	return db.Model(model).Where(ColID+" = ?", id).
		Select("*").
		Omit(ColID, ColCreatedAt, ColDeletedAt).
		Updates(values).Error
}

func (s *Store) updateByID(model any, id uint, values any) error {
	return updateByIDWith(s.db, model, id, values)
}

func findOrCreateVendor(tx *gorm.DB, vendor Vendor) (Vendor, error) {
	if strings.TrimSpace(vendor.Name) == "" {
		return Vendor{}, fmt.Errorf("vendor name is required")
	}
	// Search unscoped so we find soft-deleted vendors too -- the unique
	// index on name spans all rows regardless of deleted_at.
	var existing Vendor
	err := tx.Unscoped().Where(ColName+" = ?", vendor.Name).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Create(&vendor).Error; err != nil {
			return Vendor{}, err
		}
		return vendor, nil
	}
	if err != nil {
		return Vendor{}, err
	}
	// Restore the vendor if it was soft-deleted, and mark the
	// DeletionRecord as restored so undo doesn't see a stale entry.
	if existing.DeletedAt.Valid {
		if err := tx.Unscoped().Model(&existing).Update(ColDeletedAt, nil).Error; err != nil {
			return Vendor{}, err
		}
		existing.DeletedAt.Valid = false
		restoredAt := time.Now()
		if err := tx.Model(&DeletionRecord{}).
			Where(
				ColEntity+" = ? AND "+ColTargetID+" = ? AND "+ColRestoredAt+" IS NULL",
				DeletionEntityVendor, existing.ID,
			).
			Update(ColRestoredAt, restoredAt).Error; err != nil {
			return Vendor{}, err
		}
	}
	// Unconditionally overwrite contact fields so callers can clear them
	// (e.g. user blanks a phone number in the quote form).
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
	// Re-read so the returned struct reflects the persisted state,
	// not the potentially stale snapshot from the initial First query.
	if err := tx.First(&existing, existing.ID).Error; err != nil {
		return Vendor{}, err
	}
	return existing, nil
}
