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

func quoteEntityDef() entityDef[data.Quote] {
	return entityDef[data.Quote]{
		name:        "quote",
		singular:    "quote",
		tableHeader: "QUOTES",
		cols:        quoteCols,
		toMap:       quoteToMap,
		list: func(s *data.Store, deleted bool) ([]data.Quote, error) {
			return s.ListQuotes(deleted)
		},
		get: func(s *data.Store, id string) (data.Quote, error) {
			return s.GetQuote(id)
		},
		decodeAndCreate: quoteCreate,
		decodeAndUpdate: quoteUpdate,
		del: func(s *data.Store, id string) error {
			return s.DeleteQuote(id)
		},
		restore: func(s *data.Store, id string) error {
			return s.RestoreQuote(id)
		},
		deletedAt: func(q data.Quote) gorm.DeletedAt {
			return q.DeletedAt
		},
	}
}

func newQuoteCmd() *cobra.Command {
	return buildEntityCmd(quoteEntityDef())
}

// resolveVendorInput extracts vendor from raw JSON fields. Returns the
// vendor to pass to store methods. vendorID takes precedence over
// vendorName. If neither is present, returns zero Vendor (caller decides
// how to handle).
func resolveVendorInput(
	store *data.Store,
	fields map[string]json.RawMessage,
) (data.Vendor, bool, error) {
	var vendorID string
	var vendorName string

	if raw, ok := fields["vendor_id"]; ok {
		if err := json.Unmarshal(raw, &vendorID); err != nil {
			return data.Vendor{}, false, fmt.Errorf("field vendor_id: %w", err)
		}
		if vendorID != "" {
			v, err := store.GetVendor(vendorID)
			if err != nil {
				return data.Vendor{}, false, fmt.Errorf("get vendor: %w", err)
			}
			return v, true, nil
		}
	}

	if raw, ok := fields["vendor_name"]; ok {
		if err := json.Unmarshal(raw, &vendorName); err != nil {
			return data.Vendor{}, false, fmt.Errorf("field vendor_name: %w", err)
		}
		if vendorName != "" {
			return data.Vendor{Name: vendorName}, true, nil
		}
	}

	return data.Vendor{}, false, nil
}

func quoteCreate(store *data.Store, raw json.RawMessage) (data.Quote, error) {
	fields, err := parseFields(raw)
	if err != nil {
		return data.Quote{}, err
	}

	var q data.Quote
	for _, pair := range []struct {
		key string
		dst any
	}{
		{"project_id", &q.ProjectID},
		{"total_cents", &q.TotalCents},
		{"labor_cents", &q.LaborCents},
		{"materials_cents", &q.MaterialsCents},
		{"other_cents", &q.OtherCents},
		{"notes", &q.Notes},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.Quote{}, err
		}
	}

	if dateStr, ok := stringField(fields, "received_date"); ok {
		parsed, dateErr := data.ParseOptionalDate(dateStr)
		if dateErr != nil {
			return data.Quote{}, fmt.Errorf("received_date: %w", dateErr)
		}
		q.ReceivedDate = parsed
	}

	if q.ProjectID == "" {
		return data.Quote{}, errors.New("project_id is required")
	}

	vendor, hasVendor, err := resolveVendorInput(store, fields)
	if err != nil {
		return data.Quote{}, err
	}
	if !hasVendor {
		return data.Quote{}, errors.New("vendor_id or vendor_name is required")
	}

	if err := store.CreateQuote(&q, vendor); err != nil {
		return data.Quote{}, err
	}
	return store.GetQuote(q.ID)
}

func quoteUpdate(store *data.Store, id string, raw json.RawMessage) (data.Quote, error) {
	existing, err := store.GetQuote(id)
	if err != nil {
		return data.Quote{}, fmt.Errorf("get quote: %w", err)
	}

	fields, err := parseFields(raw)
	if err != nil {
		return data.Quote{}, err
	}

	for _, pair := range []struct {
		key string
		dst any
	}{
		{"project_id", &existing.ProjectID},
		{"total_cents", &existing.TotalCents},
		{"labor_cents", &existing.LaborCents},
		{"materials_cents", &existing.MaterialsCents},
		{"other_cents", &existing.OtherCents},
		{"notes", &existing.Notes},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.Quote{}, err
		}
	}

	if dateStr, ok := stringField(fields, "received_date"); ok {
		parsed, dateErr := data.ParseOptionalDate(dateStr)
		if dateErr != nil {
			return data.Quote{}, fmt.Errorf("received_date: %w", dateErr)
		}
		existing.ReceivedDate = parsed
	} else if _, present := fields["received_date"]; present {
		existing.ReceivedDate = nil
	}

	vendor, hasVendor, err := resolveVendorInput(store, fields)
	if err != nil {
		return data.Quote{}, err
	}
	if !hasVendor {
		// Use preloaded vendor from GetQuote (works even if vendor is
		// soft-deleted, since GetQuote preloads with unscopedPreload).
		vendor = existing.Vendor
	}

	if err := store.UpdateQuote(existing, vendor); err != nil {
		return data.Quote{}, err
	}
	return store.GetQuote(id)
}
