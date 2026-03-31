// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"time"

	"gorm.io/gorm"
)

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
			if err := writeOplogEntryRaw(tx, TableIncidents, id, OpDelete); err != nil {
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
			if err := writeOplogEntryRaw(tx, TableIncidents, id, OpRestore); err != nil {
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
		// Detach linked documents and remove DeletionRecords.
		if err := detachDocumentsAndCleanup(tx,
			DocumentEntityIncident, DeletionEntityIncident, id); err != nil {
			return err
		}

		// Write oplog "delete" entry before the hard-delete.
		if !isSyncApplying(tx) {
			if err := writeOplogEntryRaw(tx, TableIncidents, id, OpDelete); err != nil {
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

func (s *Store) CountIncidentsByVendor(vendorIDs []string) (map[string]int, error) {
	return s.countByFK(&Incident{}, ColVendorID, vendorIDs)
}
