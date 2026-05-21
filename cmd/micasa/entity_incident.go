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

func incidentEntityDef() entityDef[data.Incident] {
	return entityDef[data.Incident]{
		name:        "incident",
		singular:    "incident",
		tableHeader: "INCIDENTS",
		cols:        incidentCols,
		toMap:       incidentToMap,
		list: func(s *data.Store, deleted bool) ([]data.Incident, error) {
			return s.ListIncidents(deleted)
		},
		get: func(s *data.Store, id string) (data.Incident, error) {
			return s.GetIncident(id)
		},
		decodeAndCreate: incidentCreate,
		decodeAndUpdate: incidentUpdate,
		del: func(s *data.Store, id string) error {
			return s.DeleteIncident(id)
		},
		restore: func(s *data.Store, id string) error {
			return s.RestoreIncident(id)
		},
		deletedAt: func(i data.Incident) gorm.DeletedAt {
			return i.DeletedAt
		},
	}
}

func newIncidentCmd() *cobra.Command {
	return buildEntityCmd(incidentEntityDef())
}

func incidentCreate(store *data.Store, raw json.RawMessage) (data.Incident, error) {
	fields, err := parseFields(raw)
	if err != nil {
		return data.Incident{}, err
	}

	var i data.Incident
	for _, pair := range []struct {
		key string
		dst any
	}{
		{data.ColTitle, &i.Title},
		{data.ColDescription, &i.Description},
		{data.ColStatus, &i.Status},
		{data.ColSeverity, &i.Severity},
		{data.ColLocation, &i.Location},
		{data.ColCostCents, &i.CostCents},
		{data.ColApplianceID, &i.ApplianceID},
		{data.ColVendorID, &i.VendorID},
		{data.ColNotes, &i.Notes},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.Incident{}, err
		}
	}

	if i.Title == "" {
		return data.Incident{}, errors.New("title is required")
	}
	if i.Status == "" {
		i.Status = data.IncidentStatusOpen
	}
	if i.Severity == "" {
		i.Severity = data.IncidentSeveritySoon
	}

	if dateStr, ok := stringField(fields, data.ColDateNoticed); ok {
		parsed, dateErr := data.ParseRequiredDate(dateStr)
		if dateErr != nil {
			return data.Incident{}, fmt.Errorf("date_noticed: %w", dateErr)
		}
		i.DateNoticed = parsed
	} else {
		i.DateNoticed = time.Now().Truncate(24 * time.Hour)
	}

	if dateStr, ok := stringField(fields, data.ColDateResolved); ok {
		parsed, dateErr := data.ParseOptionalDate(dateStr)
		if dateErr != nil {
			return data.Incident{}, fmt.Errorf("date_resolved: %w", dateErr)
		}
		i.DateResolved = parsed
	}

	if err := store.CreateIncident(&i); err != nil {
		return data.Incident{}, err
	}
	return store.GetIncident(i.ID)
}

func incidentUpdate(store *data.Store, id string, raw json.RawMessage) (data.Incident, error) {
	existing, err := store.GetIncident(id)
	if err != nil {
		return data.Incident{}, fmt.Errorf("get incident: %w", err)
	}

	fields, err := parseFields(raw)
	if err != nil {
		return data.Incident{}, err
	}

	for _, pair := range []struct {
		key string
		dst any
	}{
		{data.ColTitle, &existing.Title},
		{data.ColDescription, &existing.Description},
		{data.ColStatus, &existing.Status},
		{data.ColSeverity, &existing.Severity},
		{data.ColLocation, &existing.Location},
		{data.ColCostCents, &existing.CostCents},
		{data.ColApplianceID, &existing.ApplianceID},
		{data.ColVendorID, &existing.VendorID},
		{data.ColNotes, &existing.Notes},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.Incident{}, err
		}
	}

	if dateStr, ok := stringField(fields, data.ColDateNoticed); ok {
		parsed, dateErr := data.ParseRequiredDate(dateStr)
		if dateErr != nil {
			return data.Incident{}, fmt.Errorf("date_noticed: %w", dateErr)
		}
		existing.DateNoticed = parsed
	}

	if dateStr, ok := stringField(fields, data.ColDateResolved); ok && dateStr != "" {
		parsed, dateErr := data.ParseOptionalDate(dateStr)
		if dateErr != nil {
			return data.Incident{}, fmt.Errorf("date_resolved: %w", dateErr)
		}
		existing.DateResolved = parsed
	} else if _, ok := fields[data.ColDateResolved]; ok {
		existing.DateResolved = nil
	}

	if err := store.UpdateIncident(existing); err != nil {
		return data.Incident{}, err
	}
	return store.GetIncident(id)
}
