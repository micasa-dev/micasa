// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import "fmt"

// EntityRow is a lightweight (id, name) pair for FK context in LLM prompts.
type EntityRow struct {
	ID   string
	Name string
}

// EntityRowContext provides (id, name) tuples for all FK-referenceable tables
// so the LLM can generate correct foreign key values in SQL output.
type EntityRowContext struct {
	Vendors               []EntityRow
	Projects              []EntityRow
	Appliances            []EntityRow
	MaintenanceItems      []EntityRow
	MaintenanceCategories []EntityRow
	ProjectTypes          []EntityRow
}

// EntityRows returns (id, name) pairs for all active entities that an
// extraction prompt might reference as foreign keys.
func (s *Store) EntityRows() (EntityRowContext, error) {
	var ctx EntityRowContext
	var err error

	ctx.Vendors, err = s.entityRows(&Vendor{}, ColName)
	if err != nil {
		return EntityRowContext{}, fmt.Errorf("vendor rows: %w", err)
	}
	ctx.Projects, err = s.entityRows(&Project{}, ColTitle)
	if err != nil {
		return EntityRowContext{}, fmt.Errorf("project rows: %w", err)
	}
	ctx.Appliances, err = s.entityRows(&Appliance{}, ColName)
	if err != nil {
		return EntityRowContext{}, fmt.Errorf("appliance rows: %w", err)
	}
	ctx.MaintenanceItems, err = s.entityRows(&MaintenanceItem{}, ColName)
	if err != nil {
		return EntityRowContext{}, fmt.Errorf("maintenance item rows: %w", err)
	}
	ctx.MaintenanceCategories, err = s.entityRowsUnscoped(&MaintenanceCategory{}, ColName)
	if err != nil {
		return EntityRowContext{}, fmt.Errorf("maintenance category rows: %w", err)
	}
	ctx.ProjectTypes, err = s.entityRowsUnscoped(&ProjectType{}, ColName)
	if err != nil {
		return EntityRowContext{}, fmt.Errorf("project type rows: %w", err)
	}
	return ctx, nil
}

// entityRows queries (id, name_col) from a soft-deletable model, returning
// only active (non-deleted) rows ordered by the name column.
func (s *Store) entityRows(model any, nameCol string) ([]EntityRow, error) {
	var rows []EntityRow
	err := s.db.Model(model).
		Select(ColID + ", " + nameCol + " AS name").
		Where("deleted_at IS NULL").
		Order(nameCol + " ASC, " + ColID + " DESC").
		Find(&rows).Error
	return rows, err
}

// entityRowsUnscoped queries (id, name_col) from a model without soft-delete
// support, ordered by the name column.
func (s *Store) entityRowsUnscoped(model any, nameCol string) ([]EntityRow, error) {
	var rows []EntityRow
	err := s.db.Model(model).
		Select(ColID + ", " + nameCol + " AS name").
		Order(nameCol + " ASC, " + ColID + " DESC").
		Find(&rows).Error
	return rows, err
}
