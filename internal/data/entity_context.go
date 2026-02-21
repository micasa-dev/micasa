// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

// EntityNames returns the names of all active (non-deleted) vendors,
// projects, and appliances. Used to provide context for LLM extraction
// so it can match extracted references against known entities.
func (s *Store) EntityNames() (vendors, projects, appliances []string, err error) {
	if err := s.db.Model(&Vendor{}).
		Where("deleted_at IS NULL").
		Order("name ASC").
		Pluck("name", &vendors).Error; err != nil {
		return nil, nil, nil, err
	}
	if err := s.db.Model(&Project{}).
		Where("deleted_at IS NULL").
		Order("title ASC").
		Pluck("title", &projects).Error; err != nil {
		return nil, nil, nil, err
	}
	if err := s.db.Model(&Appliance{}).
		Where("deleted_at IS NULL").
		Order("name ASC").
		Pluck("name", &appliances).Error; err != nil {
		return nil, nil, nil, err
	}
	return vendors, projects, appliances, nil
}
