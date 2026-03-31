// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

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
		return errors.New("house profile already exists")
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
	if err := s.db.Model(&existing).Select("*").Updates(profile).Error; err != nil { //nolint:unqueryvet // GORM Select("*") updates all non-omitted columns
		return err
	}
	if !isSyncApplying(s.db) {
		return writeOplogEntry(s.db, TableHouseProfiles, profile.ID, OpUpdate, profile)
	}
	return nil
}
