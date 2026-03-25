// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

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
