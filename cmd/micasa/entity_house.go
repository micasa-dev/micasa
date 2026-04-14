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

func newHouseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "house",
		Short:         "Manage house profile",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddCommand(newHouseGetCmd())
	cmd.AddCommand(newHouseAddCmd())
	cmd.AddCommand(newHouseEditCmd())

	return cmd
}

func newHouseGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "get [database-path]",
		Short:         "Get house profile",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			return houseGet(cmd, store)
		},
	}
}

func houseGet(cmd *cobra.Command, store *data.Store) error {
	h, err := store.HouseProfile()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if _, werr := fmt.Fprintln(cmd.OutOrStdout(), "{}"); werr != nil {
				return fmt.Errorf("write empty house: %w", werr)
			}
			return nil
		}
		return fmt.Errorf("get house profile: %w", err)
	}
	return encodeJSON(cmd.OutOrStdout(), h)
}

func newHouseAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "add [database-path]",
		Short:         "Add house profile",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			raw, err := readInputData(cmd)
			if err != nil {
				return err
			}

			h, err := houseCreate(store, raw)
			if err != nil {
				return err
			}
			return encodeJSON(cmd.OutOrStdout(), h)
		},
	}

	cmd.Flags().String("data", "", "JSON object with field values")
	cmd.Flags().String("data-file", "", "Path to JSON file with field values")
	return cmd
}

func houseCreate(store *data.Store, raw json.RawMessage) (data.HouseProfile, error) {
	fields, err := parseFields(raw)
	if err != nil {
		return data.HouseProfile{}, err
	}

	var h data.HouseProfile
	for _, pair := range []struct {
		key string
		dst any
	}{
		{"nickname", &h.Nickname},
		{"address_line1", &h.AddressLine1},
		{"address_line2", &h.AddressLine2},
		{"city", &h.City},
		{"state", &h.State},
		{"postal_code", &h.PostalCode},
		{"year_built", &h.YearBuilt},
		{"square_feet", &h.SquareFeet},
		{"lot_square_feet", &h.LotSquareFeet},
		{"bedrooms", &h.Bedrooms},
		{"bathrooms", &h.Bathrooms},
		{"foundation_type", &h.FoundationType},
		{"wiring_type", &h.WiringType},
		{"roof_type", &h.RoofType},
		{"exterior_type", &h.ExteriorType},
		{"heating_type", &h.HeatingType},
		{"cooling_type", &h.CoolingType},
		{"water_source", &h.WaterSource},
		{"sewer_type", &h.SewerType},
		{"parking_type", &h.ParkingType},
		{"basement_type", &h.BasementType},
		{"insurance_carrier", &h.InsuranceCarrier},
		{"insurance_policy", &h.InsurancePolicy},
		{"property_tax_cents", &h.PropertyTaxCents},
		{"hoa_name", &h.HOAName},
		{"hoa_fee_cents", &h.HOAFeeCents},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.HouseProfile{}, err
		}
	}

	if dateStr, ok := stringField(fields, "insurance_renewal"); ok {
		parsed, dateErr := data.ParseOptionalDate(dateStr)
		if dateErr != nil {
			return data.HouseProfile{}, fmt.Errorf("insurance_renewal: %w", dateErr)
		}
		h.InsuranceRenewal = parsed
	}

	if err := store.CreateHouseProfile(h); err != nil {
		return data.HouseProfile{}, err
	}
	return store.HouseProfile()
}

func newHouseEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "edit [database-path]",
		Short:         "Edit house profile",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			raw, err := readInputData(cmd)
			if err != nil {
				return err
			}

			h, err := houseUpdate(store, raw)
			if err != nil {
				return err
			}
			return encodeJSON(cmd.OutOrStdout(), h)
		},
	}

	cmd.Flags().String("data", "", "JSON object with fields to update")
	cmd.Flags().String("data-file", "", "Path to JSON file with fields to update")
	return cmd
}

func houseUpdate(store *data.Store, raw json.RawMessage) (data.HouseProfile, error) {
	existing, err := store.HouseProfile()
	if err != nil {
		return data.HouseProfile{}, fmt.Errorf("get house profile: %w", err)
	}

	fields, err := parseFields(raw)
	if err != nil {
		return data.HouseProfile{}, err
	}

	for _, pair := range []struct {
		key string
		dst any
	}{
		{"nickname", &existing.Nickname},
		{"address_line1", &existing.AddressLine1},
		{"address_line2", &existing.AddressLine2},
		{"city", &existing.City},
		{"state", &existing.State},
		{"postal_code", &existing.PostalCode},
		{"year_built", &existing.YearBuilt},
		{"square_feet", &existing.SquareFeet},
		{"lot_square_feet", &existing.LotSquareFeet},
		{"bedrooms", &existing.Bedrooms},
		{"bathrooms", &existing.Bathrooms},
		{"foundation_type", &existing.FoundationType},
		{"wiring_type", &existing.WiringType},
		{"roof_type", &existing.RoofType},
		{"exterior_type", &existing.ExteriorType},
		{"heating_type", &existing.HeatingType},
		{"cooling_type", &existing.CoolingType},
		{"water_source", &existing.WaterSource},
		{"sewer_type", &existing.SewerType},
		{"parking_type", &existing.ParkingType},
		{"basement_type", &existing.BasementType},
		{"insurance_carrier", &existing.InsuranceCarrier},
		{"insurance_policy", &existing.InsurancePolicy},
		{"property_tax_cents", &existing.PropertyTaxCents},
		{"hoa_name", &existing.HOAName},
		{"hoa_fee_cents", &existing.HOAFeeCents},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.HouseProfile{}, err
		}
	}

	if dateStr, ok := stringField(fields, "insurance_renewal"); ok {
		parsed, dateErr := data.ParseOptionalDate(dateStr)
		if dateErr != nil {
			return data.HouseProfile{}, fmt.Errorf("insurance_renewal: %w", dateErr)
		}
		existing.InsuranceRenewal = parsed
	} else if _, present := fields["insurance_renewal"]; present {
		existing.InsuranceRenewal = nil
	}

	if err := store.UpdateHouseProfile(existing); err != nil {
		return data.HouseProfile{}, err
	}
	return store.HouseProfile()
}
