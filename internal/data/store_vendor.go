// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import "gorm.io/gorm"

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

func (s *Store) UpdateVendor(vendor Vendor) error {
	return s.updateByID(TableVendors, &Vendor{}, vendor.ID, vendor)
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

// CountQuotesByVendor returns the number of non-deleted quotes per vendor ID.
func (s *Store) CountQuotesByVendor(vendorIDs []string) (map[string]int, error) {
	return s.countByFK(&Quote{}, ColVendorID, vendorIDs)
}

// CountServiceLogsByVendor returns the number of non-deleted service log entries per vendor ID.
func (s *Store) CountServiceLogsByVendor(vendorIDs []string) (map[string]int, error) {
	return s.countByFK(&ServiceLogEntry{}, ColVendorID, vendorIDs)
}

// ListQuotesByVendor returns all quotes for a specific vendor.
func (s *Store) ListQuotesByVendor(vendorID string, includeDeleted bool) ([]Quote, error) {
	return listQuery[Quote](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return prepareQuoteRelations(
			db.Where(ColVendorID+" = ?", vendorID),
		).Order(ColReceivedDate + " desc, " + ColID + " desc")
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

func findOrCreateVendor(tx *gorm.DB, vendor Vendor) (Vendor, error) {
	// Check if the vendor already exists before findOrCreate, so we know
	// whether to write an oplog update for contact-field changes.
	var preExisting Vendor
	wasExisting := tx.Unscoped().
		Where(ColName+" = ?", vendor.Name).
		First(&preExisting).
		Error == nil &&
		!preExisting.DeletedAt.Valid

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
	// Write oplog update for contact-field changes on existing vendors.
	// New vendors already have an insert oplog entry from AfterCreate.
	if wasExisting && !isSyncApplying(tx) {
		if err := writeOplogEntry(tx, TableVendors, existing.ID, OpUpdate, existing); err != nil {
			return Vendor{}, err
		}
	}
	return existing, nil
}
