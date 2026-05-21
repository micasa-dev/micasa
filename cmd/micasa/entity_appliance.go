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

func applianceEntityDef() entityDef[data.Appliance] {
	return entityDef[data.Appliance]{
		name:        entityAppliance,
		singular:    entityAppliance,
		tableHeader: "APPLIANCES",
		cols:        applianceCols,
		toMap:       applianceToMap,
		list: func(s *data.Store, deleted bool) ([]data.Appliance, error) {
			return s.ListAppliances(deleted)
		},
		get: func(s *data.Store, id string) (data.Appliance, error) {
			return s.GetAppliance(id)
		},
		decodeAndCreate: applianceCreate,
		decodeAndUpdate: applianceUpdate,
		del: func(s *data.Store, id string) error {
			return s.DeleteAppliance(id)
		},
		restore: func(s *data.Store, id string) error {
			return s.RestoreAppliance(id)
		},
		deletedAt: func(a data.Appliance) gorm.DeletedAt {
			return a.DeletedAt
		},
	}
}

func newApplianceCmd() *cobra.Command {
	return buildEntityCmd(applianceEntityDef())
}

func applianceCreate(store *data.Store, raw json.RawMessage) (data.Appliance, error) {
	fields, err := parseFields(raw)
	if err != nil {
		return data.Appliance{}, err
	}

	var a data.Appliance
	for _, pair := range []struct {
		key string
		dst any
	}{
		{data.ColName, &a.Name},
		{data.ColBrand, &a.Brand},
		{data.ColModelNumber, &a.ModelNumber},
		{data.ColSerialNumber, &a.SerialNumber},
		{data.ColLocation, &a.Location},
		{data.ColCostCents, &a.CostCents},
		{data.ColNotes, &a.Notes},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.Appliance{}, err
		}
	}

	for _, datePair := range []struct {
		key string
		dst **time.Time
	}{
		{data.ColPurchaseDate, &a.PurchaseDate},
		{data.ColWarrantyExpiry, &a.WarrantyExpiry},
	} {
		if dateStr, ok := stringField(fields, datePair.key); ok {
			parsed, dateErr := data.ParseOptionalDate(dateStr)
			if dateErr != nil {
				return data.Appliance{}, fmt.Errorf("%s: %w", datePair.key, dateErr)
			}
			*datePair.dst = parsed
		}
	}

	if a.Name == "" {
		return data.Appliance{}, errors.New("name is required")
	}
	if err := store.CreateAppliance(&a); err != nil {
		return data.Appliance{}, err
	}
	return a, nil
}

func applianceUpdate(store *data.Store, id string, raw json.RawMessage) (data.Appliance, error) {
	existing, err := store.GetAppliance(id)
	if err != nil {
		return data.Appliance{}, fmt.Errorf("get appliance: %w", err)
	}

	fields, err := parseFields(raw)
	if err != nil {
		return data.Appliance{}, err
	}

	for _, pair := range []struct {
		key string
		dst any
	}{
		{data.ColName, &existing.Name},
		{data.ColBrand, &existing.Brand},
		{data.ColModelNumber, &existing.ModelNumber},
		{data.ColSerialNumber, &existing.SerialNumber},
		{data.ColLocation, &existing.Location},
		{data.ColCostCents, &existing.CostCents},
		{data.ColNotes, &existing.Notes},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.Appliance{}, err
		}
	}

	for _, datePair := range []struct {
		key string
		dst **time.Time
	}{
		{data.ColPurchaseDate, &existing.PurchaseDate},
		{data.ColWarrantyExpiry, &existing.WarrantyExpiry},
	} {
		if dateStr, ok := stringField(fields, datePair.key); ok {
			parsed, dateErr := data.ParseOptionalDate(dateStr)
			if dateErr != nil {
				return data.Appliance{}, fmt.Errorf("%s: %w", datePair.key, dateErr)
			}
			*datePair.dst = parsed
		} else if _, present := fields[datePair.key]; present {
			*datePair.dst = nil
		}
	}

	if err := store.UpdateAppliance(existing); err != nil {
		return data.Appliance{}, err
	}
	return existing, nil
}
