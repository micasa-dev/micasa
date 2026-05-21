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

func vendorEntityDef() entityDef[data.Vendor] {
	return entityDef[data.Vendor]{
		name:        entityVendor,
		singular:    entityVendor,
		tableHeader: "VENDORS",
		cols:        vendorCols,
		toMap:       vendorToMap,
		list: func(s *data.Store, deleted bool) ([]data.Vendor, error) {
			return s.ListVendors(deleted)
		},
		get: func(s *data.Store, id string) (data.Vendor, error) {
			return s.GetVendor(id)
		},
		decodeAndCreate: vendorCreate,
		decodeAndUpdate: vendorUpdate,
		del: func(s *data.Store, id string) error {
			return s.DeleteVendor(id)
		},
		restore: func(s *data.Store, id string) error {
			return s.RestoreVendor(id)
		},
		deletedAt: func(v data.Vendor) gorm.DeletedAt {
			return v.DeletedAt
		},
	}
}

func newVendorCmd() *cobra.Command {
	return buildEntityCmd(vendorEntityDef())
}

func vendorCreate(store *data.Store, raw json.RawMessage) (data.Vendor, error) {
	var v data.Vendor
	if err := json.Unmarshal(raw, &v); err != nil {
		return data.Vendor{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if v.Name == "" {
		return data.Vendor{}, errors.New("name is required")
	}
	found, err := store.FindOrCreateVendor(v)
	if err != nil {
		return data.Vendor{}, err
	}
	return found, nil
}

func vendorUpdate(store *data.Store, id string, raw json.RawMessage) (data.Vendor, error) {
	existing, err := store.GetVendor(id)
	if err != nil {
		return data.Vendor{}, fmt.Errorf("get vendor: %w", err)
	}

	fields, err := parseFields(raw)
	if err != nil {
		return data.Vendor{}, err
	}

	for _, pair := range []struct {
		key string
		dst any
	}{
		{data.ColName, &existing.Name},
		{data.ColContactName, &existing.ContactName},
		{data.ColEmail, &existing.Email},
		{data.ColPhone, &existing.Phone},
		{data.ColWebsite, &existing.Website},
		{data.ColNotes, &existing.Notes},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.Vendor{}, err
		}
	}

	if err := store.UpdateVendor(existing); err != nil {
		return data.Vendor{}, err
	}
	return existing, nil
}
