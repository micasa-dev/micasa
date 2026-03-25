// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func fmtMoney(cents *int64) string {
	if cents == nil {
		return "-"
	}
	return fmt.Sprintf("$%.2f", float64(*cents)/100)
}

func fmtDate(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

func fmtInt(n int) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

func fmtFloat(f float64) string {
	if f == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", f)
}

// --- show command ---

func newShowCmd() *cobra.Command {
	var jsonFlag bool
	var deletedFlag bool

	cmd := &cobra.Command{
		Use:   "show <entity>",
		Short: "Display data as text or JSON",
		Long: `Print entity data to stdout. Entities: house, projects, project-types,
quotes, vendors, maintenance, maintenance-categories, service-log,
appliances, incidents, documents, all.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output as JSON")
	cmd.PersistentFlags().BoolVar(&deletedFlag, "deleted", false, "Include soft-deleted rows")

	cmd.AddCommand(newShowHouseCmd(&jsonFlag, &deletedFlag))

	return cmd
}

func newShowHouseCmd(jsonFlag, deletedFlag *bool) *cobra.Command {
	return &cobra.Command{
		Use:           "house [database-path]",
		Short:         "Show house profile",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			return runShow(cmd.OutOrStdout(), store, "house", *jsonFlag, *deletedFlag)
		},
	}
}

// validEntities lists entity names for error messages.
var validEntities = []string{
	"house", "projects", "project-types", "quotes", "vendors",
	"maintenance", "maintenance-categories", "service-log",
	"appliances", "incidents", "documents", "all",
}

func runShow(w io.Writer, store *data.Store, entity string, asJSON, _ bool) error {
	switch entity {
	case "house":
		return showHouse(w, store, asJSON)
	default:
		return fmt.Errorf("unknown entity %q; valid entities: %s",
			entity, strings.Join(validEntities, ", "))
	}
}

func showHouse(w io.Writer, store *data.Store, asJSON bool) error {
	h, err := store.HouseProfile()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load house profile: %w", err)
	}

	if asJSON {
		return showHouseJSON(w, h)
	}
	return showHouseText(w, h)
}

func showHouseText(w io.Writer, h data.HouseProfile) error {
	if _, err := fmt.Fprintln(w, "=== HOUSE ==="); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	var writeErr error
	kv := func(label, value string) {
		if writeErr != nil {
			return
		}
		if value != "" && value != "-" {
			_, writeErr = fmt.Fprintf(tw, "%s:\t%s\n", label, value)
		}
	}

	kv("Nickname", h.Nickname)
	kv("Address", formatAddress(h))
	kv("Year Built", fmtInt(h.YearBuilt))
	kv("Square Feet", fmtInt(h.SquareFeet))
	kv("Lot Size", fmtInt(h.LotSquareFeet))
	kv("Bedrooms", fmtInt(h.Bedrooms))
	kv("Bathrooms", fmtFloat(h.Bathrooms))
	kv("Foundation", h.FoundationType)
	kv("Wiring", h.WiringType)
	kv("Roof", h.RoofType)
	kv("Exterior", h.ExteriorType)
	kv("Heating", h.HeatingType)
	kv("Cooling", h.CoolingType)
	kv("Water Source", h.WaterSource)
	kv("Sewer", h.SewerType)
	kv("Parking", h.ParkingType)
	kv("Basement", h.BasementType)
	kv("Insurance Carrier", h.InsuranceCarrier)
	kv("Insurance Policy", h.InsurancePolicy)
	kv("Insurance Renewal", fmtDate(h.InsuranceRenewal))
	kv("Property Tax", fmtMoney(h.PropertyTaxCents))
	kv("HOA", h.HOAName)
	kv("HOA Fee", fmtMoney(h.HOAFeeCents))

	if writeErr != nil {
		return fmt.Errorf("write field: %w", writeErr)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush table: %w", err)
	}
	return nil
}

func formatAddress(h data.HouseProfile) string {
	var lines []string
	if h.AddressLine1 != "" {
		lines = append(lines, h.AddressLine1)
	}
	if h.AddressLine2 != "" {
		lines = append(lines, h.AddressLine2)
	}
	var cityState []string
	if h.City != "" {
		cityState = append(cityState, h.City)
	}
	if h.State != "" {
		cityState = append(cityState, h.State)
	}
	csStr := strings.Join(cityState, ", ")
	if csStr != "" && h.PostalCode != "" {
		csStr += " " + h.PostalCode
	} else if h.PostalCode != "" {
		csStr = h.PostalCode
	}
	if csStr != "" {
		lines = append(lines, csStr)
	}
	return strings.Join(lines, "\n                   ")
}

func showHouseJSON(w io.Writer, h data.HouseProfile) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(h); err != nil {
		return fmt.Errorf("encode house JSON: %w", err)
	}
	return nil
}
