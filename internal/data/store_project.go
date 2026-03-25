// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import "gorm.io/gorm"

func (s *Store) ProjectTypes() ([]ProjectType, error) {
	var types []ProjectType
	if err := s.db.Order(ColName + " ASC, " + ColID + " DESC").Find(&types).Error; err != nil {
		return nil, err
	}
	return types, nil
}

func (s *Store) ListProjects(includeDeleted bool) ([]Project, error) {
	return listQuery[Project](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Preload("ProjectType").Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

func (s *Store) GetProject(id string) (Project, error) {
	return getByID[Project](s, id, func(db *gorm.DB) *gorm.DB {
		return db.Preload("ProjectType")
	})
}

func (s *Store) CreateProject(project *Project) error {
	return s.db.Create(project).Error
}

func (s *Store) UpdateProject(project Project) error {
	return s.updateByID(TableProjects, &Project{}, project.ID, project)
}

func (s *Store) DeleteProject(id string) error {
	if err := s.checkDependencies(id, []dependencyCheck{
		{&Quote{}, ColProjectID, "project has %d active quote(s) -- delete them first"},
	}); err != nil {
		return err
	}
	return s.softDelete(&Project{}, DeletionEntityProject, id)
}

func (s *Store) RestoreProject(id string) error {
	return s.restoreEntity(&Project{}, DeletionEntityProject, id)
}

// CountQuotesByProject returns the number of non-deleted quotes per project ID.
func (s *Store) CountQuotesByProject(projectIDs []string) (map[string]int, error) {
	return s.countByFK(&Quote{}, ColProjectID, projectIDs)
}
