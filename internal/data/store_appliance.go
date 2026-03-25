// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import "gorm.io/gorm"

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

func (s *Store) DeleteAppliance(id string) error {
	if err := s.checkDependencies(id, []dependencyCheck{
		{&MaintenanceItem{}, ColApplianceID, "appliance has %d active maintenance item(s) -- delete or reassign them first"},
		{&Incident{}, ColApplianceID, "appliance has %d active incident(s) -- delete them first"},
	}); err != nil {
		return err
	}
	return s.softDelete(&Appliance{}, DeletionEntityAppliance, id)
}

func (s *Store) RestoreAppliance(id string) error {
	return s.restoreEntity(&Appliance{}, DeletionEntityAppliance, id)
}

// CountMaintenanceByAppliance returns the count of non-deleted maintenance
// items for each appliance ID.
func (s *Store) CountMaintenanceByAppliance(applianceIDs []string) (map[string]int, error) {
	return s.countByFK(&MaintenanceItem{}, ColApplianceID, applianceIDs)
}

func (s *Store) CountIncidentsByAppliance(applianceIDs []string) (map[string]int, error) {
	return s.countByFK(&Incident{}, ColApplianceID, applianceIDs)
}
