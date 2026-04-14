// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func serviceLogEntityDef() entityDef[data.ServiceLogEntry] {
	return entityDef[data.ServiceLogEntry]{
		name:        "service-log",
		singular:    "service log entry",
		tableHeader: "SERVICE LOG",
		cols:        serviceLogCols,
		toMap:       serviceLogToMap,
		list: func(s *data.Store, deleted bool) ([]data.ServiceLogEntry, error) {
			return s.ListAllServiceLogEntries(deleted)
		},
		get: func(s *data.Store, id string) (data.ServiceLogEntry, error) {
			return s.GetServiceLog(id)
		},
		decodeAndCreate: serviceLogCreate,
		decodeAndUpdate: serviceLogUpdate,
		del: func(s *data.Store, id string) error {
			return s.DeleteServiceLog(id)
		},
		restore: func(s *data.Store, id string) error {
			return s.RestoreServiceLog(id)
		},
		deletedAt: func(e data.ServiceLogEntry) gorm.DeletedAt {
			return e.DeletedAt
		},
	}
}

func newServiceLogCmd() *cobra.Command {
	return buildEntityCmd(serviceLogEntityDef())
}

func serviceLogCreate(
	store *data.Store,
	raw json.RawMessage,
) (data.ServiceLogEntry, error) {
	fields, err := parseFields(raw)
	if err != nil {
		return data.ServiceLogEntry{}, err
	}

	var e data.ServiceLogEntry
	e.MaintenanceItemID, _ = stringField(fields, "maintenance_item_id")
	if e.MaintenanceItemID == "" {
		return data.ServiceLogEntry{}, errors.New("maintenance_item_id is required")
	}

	servicedAtStr, _ := stringField(fields, "serviced_at")
	if servicedAtStr == "" {
		return data.ServiceLogEntry{}, errors.New("serviced_at is required")
	}
	servicedAt, dateErr := data.ParseRequiredDate(servicedAtStr)
	if dateErr != nil {
		return data.ServiceLogEntry{}, fmt.Errorf("serviced_at: %w", dateErr)
	}
	e.ServicedAt = servicedAt

	if err := mergeField(fields, "cost_cents", &e.CostCents); err != nil {
		return data.ServiceLogEntry{}, err
	}
	if err := mergeField(fields, "notes", &e.Notes); err != nil {
		return data.ServiceLogEntry{}, err
	}

	vendor, _, err := resolveVendorInput(store, fields)
	if err != nil {
		return data.ServiceLogEntry{}, err
	}

	if err := store.CreateServiceLog(&e, vendor); err != nil {
		return data.ServiceLogEntry{}, err
	}
	return store.GetServiceLog(e.ID)
}

func serviceLogUpdate(
	store *data.Store,
	id string,
	raw json.RawMessage,
) (data.ServiceLogEntry, error) {
	existing, err := store.GetServiceLog(id)
	if err != nil {
		return data.ServiceLogEntry{}, fmt.Errorf("get service log: %w", err)
	}

	fields, err := parseFields(raw)
	if err != nil {
		return data.ServiceLogEntry{}, err
	}

	if err := mergeField(fields, "maintenance_item_id", &existing.MaintenanceItemID); err != nil {
		return data.ServiceLogEntry{}, err
	}

	if dateStr, ok := stringField(fields, "serviced_at"); ok {
		parsed, dateErr := data.ParseRequiredDate(dateStr)
		if dateErr != nil {
			return data.ServiceLogEntry{}, fmt.Errorf("serviced_at: %w", dateErr)
		}
		existing.ServicedAt = parsed
	}

	if err := mergeField(fields, "cost_cents", &existing.CostCents); err != nil {
		return data.ServiceLogEntry{}, err
	}
	if err := mergeField(fields, "notes", &existing.Notes); err != nil {
		return data.ServiceLogEntry{}, err
	}

	// Vendor resolution: vendor_id/vendor_name or preserve existing.
	vendor, hasVendor, err := resolveVendorInput(store, fields)
	if err != nil {
		return data.ServiceLogEntry{}, err
	}

	if !hasVendor {
		// Preserve existing vendor. Use preloaded vendor from GetServiceLog
		// (works even if vendor is soft-deleted via unscopedPreload).
		vendor = existing.Vendor
	}

	// Handle explicit null for vendor_id (clear vendor).
	if raw, ok := fields["vendor_id"]; ok {
		if string(raw) == "null" {
			vendor = data.Vendor{}
		}
	}

	if err := store.UpdateServiceLog(existing, vendor); err != nil {
		return data.ServiceLogEntry{}, err
	}
	return store.GetServiceLog(id)
}
