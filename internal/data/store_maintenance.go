// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import "gorm.io/gorm"

func (s *Store) MaintenanceCategories() ([]MaintenanceCategory, error) {
	var categories []MaintenanceCategory
	if err := s.db.Order(ColName + " ASC, " + ColID + " DESC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
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
// and detaches any documents linked to the maintenance item or its service
// logs (documents have intrinsic value and survive parent deletion).
func (s *Store) HardDeleteMaintenance(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Find all child service log entries (including soft-deleted).
		var logs []ServiceLogEntry
		if err := tx.Unscoped().
			Where(ColMaintenanceItemID+" = ?", id).
			Find(&logs).Error; err != nil {
			return err
		}

		// Detach documents and clean up DeletionRecords for the
		// maintenance item itself.
		if err := detachDocumentsAndCleanup(tx,
			DocumentEntityMaintenance, DeletionEntityMaintenance, id); err != nil {
			return err
		}

		// Detach documents and clean up DeletionRecords for each child
		// service log, then write oplog delete entries.
		for _, log := range logs {
			if err := detachDocumentsAndCleanup(tx,
				DocumentEntityServiceLog, DeletionEntityServiceLog, log.ID); err != nil {
				return err
			}
			if !isSyncApplying(tx) {
				if err := writeOplogEntryRaw(tx, TableServiceLogEntries, log.ID, OpDelete, "{}"); err != nil {
					return err
				}
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
