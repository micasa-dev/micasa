// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func projectEntityDef() entityDef[data.Project] {
	return entityDef[data.Project]{
		name:        "project",
		singular:    "project",
		tableHeader: "PROJECTS",
		cols:        projectCols,
		toMap:       projectToMap,
		list: func(s *data.Store, deleted bool) ([]data.Project, error) {
			return s.ListProjects(deleted)
		},
		get: func(s *data.Store, id string) (data.Project, error) {
			return s.GetProject(id)
		},
		decodeAndCreate: projectCreate,
		decodeAndUpdate: projectUpdate,
		del: func(s *data.Store, id string) error {
			return s.DeleteProject(id)
		},
		restore: func(s *data.Store, id string) error {
			return s.RestoreProject(id)
		},
		deletedAt: func(p data.Project) gorm.DeletedAt {
			return p.DeletedAt
		},
	}
}

func newProjectCmd() *cobra.Command {
	return buildEntityCmd(projectEntityDef())
}

func projectCreate(store *data.Store, raw json.RawMessage) (data.Project, error) {
	fields, err := parseFields(raw)
	if err != nil {
		return data.Project{}, err
	}

	var p data.Project
	for _, pair := range []struct {
		key string
		dst any
	}{
		{data.ColTitle, &p.Title},
		{data.ColProjectTypeID, &p.ProjectTypeID},
		{data.ColStatus, &p.Status},
		{data.ColDescription, &p.Description},
		{data.ColBudgetCents, &p.BudgetCents},
		{data.ColActualCents, &p.ActualCents},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.Project{}, err
		}
	}

	for _, datePair := range []struct {
		key string
		dst **time.Time
	}{
		{data.ColStartDate, &p.StartDate},
		{data.ColEndDate, &p.EndDate},
	} {
		if dateStr, ok := stringField(fields, datePair.key); ok {
			parsed, dateErr := data.ParseOptionalDate(dateStr)
			if dateErr != nil {
				return data.Project{}, fmt.Errorf("%s: %w", datePair.key, dateErr)
			}
			*datePair.dst = parsed
		}
	}

	if p.Title == "" {
		return data.Project{}, errors.New("title is required")
	}
	if p.ProjectTypeID == "" {
		return data.Project{}, errors.New("project_type_id is required")
	}
	if p.Status == "" {
		p.Status = data.ProjectStatusPlanned
	}
	if err := store.CreateProject(&p); err != nil {
		return data.Project{}, err
	}
	return store.GetProject(p.ID)
}

func projectUpdate(store *data.Store, id string, raw json.RawMessage) (data.Project, error) {
	existing, err := store.GetProject(id)
	if err != nil {
		return data.Project{}, fmt.Errorf("get project: %w", err)
	}

	fields, err := parseFields(raw)
	if err != nil {
		return data.Project{}, err
	}

	for _, pair := range []struct {
		key string
		dst any
	}{
		{data.ColTitle, &existing.Title},
		{data.ColProjectTypeID, &existing.ProjectTypeID},
		{data.ColStatus, &existing.Status},
		{data.ColDescription, &existing.Description},
		{data.ColBudgetCents, &existing.BudgetCents},
		{data.ColActualCents, &existing.ActualCents},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.Project{}, err
		}
	}

	for _, datePair := range []struct {
		key string
		dst **time.Time
	}{
		{data.ColStartDate, &existing.StartDate},
		{data.ColEndDate, &existing.EndDate},
	} {
		if dateStr, ok := stringField(fields, datePair.key); ok {
			parsed, dateErr := data.ParseOptionalDate(dateStr)
			if dateErr != nil {
				return data.Project{}, fmt.Errorf("%s: %w", datePair.key, dateErr)
			}
			*datePair.dst = parsed
		} else if _, present := fields[datePair.key]; present {
			*datePair.dst = nil
		}
	}

	if err := store.UpdateProject(existing); err != nil {
		return data.Project{}, err
	}
	return store.GetProject(id)
}
