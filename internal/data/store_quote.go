// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import "gorm.io/gorm"

// prepareQuoteRelationsFull preloads Vendor + Project with nested ProjectType.
func prepareQuoteRelationsFull(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Vendor", unscopedPreload).
		Preload("Project", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped().Preload("ProjectType")
		})
}

// prepareQuoteRelations preloads Vendor + Project (unscoped, no nested types).
func prepareQuoteRelations(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Vendor", unscopedPreload).
		Preload("Project", unscopedPreload)
}

func (s *Store) ListQuotes(includeDeleted bool) ([]Quote, error) {
	return listQuery[Quote](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return prepareQuoteRelationsFull(db).
			Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

func (s *Store) GetQuote(id string) (Quote, error) {
	return getByID[Quote](s, id, prepareQuoteRelationsFull)
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

func (s *Store) DeleteQuote(id string) error {
	return s.softDelete(&Quote{}, DeletionEntityQuote, id)
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

// ListQuotesByProject returns all quotes for a specific project.
func (s *Store) ListQuotesByProject(projectID string, includeDeleted bool) ([]Quote, error) {
	return listQuery[Quote](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return prepareQuoteRelations(
			db.Where(ColProjectID+" = ?", projectID),
		).Order(ColReceivedDate + " desc, " + ColID + " desc")
	})
}
