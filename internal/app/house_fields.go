// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"charm.land/huh/v2"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/locale"
)

type houseSection int

const (
	houseSectionIdentity houseSection = iota
	houseSectionStructure
	houseSectionUtilities
	houseSectionFinancial
)

// houseSectionOrder lists sections in the order they appear in the form.
var houseSectionOrder = []houseSection{
	houseSectionIdentity,
	houseSectionStructure,
	houseSectionUtilities,
	houseSectionFinancial,
}

func (s houseSection) title() string {
	switch s {
	case houseSectionIdentity:
		return "Basics"
	case houseSectionStructure:
		return "Structure"
	case houseSectionUtilities:
		return "Utilities"
	case houseSectionFinancial:
		return "Financial"
	default:
		return ""
	}
}

// houseFieldDef describes a single editable field on HouseProfile.
// Used by both the full form and the overlay inline editor.
type houseFieldDef struct {
	key     string
	label   string
	labelFn func(us data.UnitSystem) string // unit-aware label override
	section houseSection
	// build creates a huh.Field bound to the given *string value.
	build func(m *Model, value *string) huh.Field
	// get reads the display value from a HouseProfile.
	get func(p data.HouseProfile, cur locale.Currency, us data.UnitSystem) string
	// ptr returns a pointer to this field's backing string in houseFormData.
	ptr func(fd *houseFormData) *string
	// validate checks a string value for this field. nil = no validation.
	validate func(string) error
	// toggle, if non-nil, means Enter toggles the value instead of opening
	// a textinput. The function flips the value and returns the new string.
	toggle func(current string) string
}

// displayLabel returns the label for the given unit system, using labelFn
// when available and falling back to the static label.
func (d houseFieldDef) displayLabel(us data.UnitSystem) string {
	if d.labelFn != nil {
		return d.labelFn(us)
	}
	return d.label
}

func houseFieldDefs() []houseFieldDef {
	return []houseFieldDef{
		// Identity — ordered to match form tab order (postal code after nickname
		// for autofill, then address lines, city, state).
		{
			key: "nickname", label: "Name", section: houseSectionIdentity,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().
					Title(requiredTitle("Nickname")).
					Description("Ex: Primary Residence").
					Value(v).
					Validate(requiredText("nickname"))
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.Nickname
			},
			ptr:      func(fd *houseFormData) *string { return &fd.Nickname },
			validate: requiredText("nickname"),
		},
		{
			key: "postal_code", label: "ZIP", section: houseSectionIdentity,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Postal code").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.PostalCode
			},
			ptr:      func(fd *houseFormData) *string { return &fd.PostalCode },
			validate: nil,
		},
		{
			key: "address_line1", label: "Addr 1", section: houseSectionIdentity,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Address line 1").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.AddressLine1
			},
			ptr:      func(fd *houseFormData) *string { return &fd.AddressLine1 },
			validate: nil,
		},
		{
			key: "address_line2", label: "Addr 2", section: houseSectionIdentity,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Address line 2").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.AddressLine2
			},
			ptr:      func(fd *houseFormData) *string { return &fd.AddressLine2 },
			validate: nil,
		},
		{
			key: "city", label: "City", section: houseSectionIdentity,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("City").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.City
			},
			ptr:      func(fd *houseFormData) *string { return &fd.City },
			validate: nil,
		},
		{
			key: "state", label: "State", section: houseSectionIdentity,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("State").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.State
			},
			ptr:      func(fd *houseFormData) *string { return &fd.State },
			validate: nil,
		},
		// Structure
		{
			key: "year_built", label: "Year", section: houseSectionStructure,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().
					Title("Year built").
					Placeholder("1998").
					Value(v).
					Validate(optionalInt("year built"))
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return intToString(p.YearBuilt)
			},
			ptr:      func(fd *houseFormData) *string { return &fd.YearBuilt },
			validate: optionalInt("year built"),
		},
		{
			key: "square_feet", label: "Ft\u00B2", section: houseSectionStructure,
			labelFn: func(us data.UnitSystem) string {
				if us == data.UnitsMetric {
					return "m\u00B2"
				}
				return "Ft\u00B2"
			},
			build: func(m *Model, v *string) huh.Field {
				return huh.NewInput().
					Title(data.AreaFormTitle(m.unitSystem)).
					Placeholder(data.AreaPlaceholder(m.unitSystem)).
					Value(v).
					Validate(optionalInt(data.AreaFormTitle(m.unitSystem)))
			},
			get: func(p data.HouseProfile, _ locale.Currency, us data.UnitSystem) string {
				return intToString(data.SqFtToDisplayInt(p.SquareFeet, us))
			},
			ptr:      func(fd *houseFormData) *string { return &fd.SquareFeet },
			validate: optionalInt(data.AreaFormTitle(data.UnitsImperial)),
		},
		{
			key: "lot_square_feet", label: "Lot", section: houseSectionStructure,
			build: func(m *Model, v *string) huh.Field {
				return huh.NewInput().
					Title(data.LotAreaFormTitle(m.unitSystem)).
					Placeholder(data.LotAreaPlaceholder(m.unitSystem)).
					Value(v).
					Validate(optionalInt(data.LotAreaFormTitle(m.unitSystem)))
			},
			get: func(p data.HouseProfile, _ locale.Currency, us data.UnitSystem) string {
				return intToString(data.SqFtToDisplayInt(p.LotSquareFeet, us))
			},
			ptr:      func(fd *houseFormData) *string { return &fd.LotSquareFeet },
			validate: optionalInt(data.LotAreaFormTitle(data.UnitsImperial)),
		},
		{
			key: "bedrooms", label: "Bed", section: houseSectionStructure,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().
					Title("Bedrooms").
					Placeholder("3").
					Value(v).
					Validate(optionalInt("bedrooms"))
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return intToString(p.Bedrooms)
			},
			ptr:      func(fd *houseFormData) *string { return &fd.Bedrooms },
			validate: optionalInt("bedrooms"),
		},
		{
			key: "bathrooms", label: "Bath", section: houseSectionStructure,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().
					Title("Bathrooms").
					Placeholder("2.5").
					Value(v).
					Validate(optionalFloat("bathrooms"))
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return formatFloat(p.Bathrooms)
			},
			ptr:      func(fd *houseFormData) *string { return &fd.Bathrooms },
			validate: optionalFloat("bathrooms"),
		},
		{
			key: "foundation_type", label: "Fndtn", section: houseSectionStructure,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Foundation type").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.FoundationType
			},
			ptr:      func(fd *houseFormData) *string { return &fd.FoundationType },
			validate: nil,
		},
		{
			key: "wiring_type", label: "Wire", section: houseSectionStructure,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Wiring type").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.WiringType
			},
			ptr:      func(fd *houseFormData) *string { return &fd.WiringType },
			validate: nil,
		},
		{
			key: "roof_type", label: "Roof", section: houseSectionStructure,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Roof type").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.RoofType
			},
			ptr:      func(fd *houseFormData) *string { return &fd.RoofType },
			validate: nil,
		},
		{
			key: "exterior_type", label: "Ext", section: houseSectionStructure,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Exterior type").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.ExteriorType
			},
			ptr:      func(fd *houseFormData) *string { return &fd.ExteriorType },
			validate: nil,
		},
		{
			key: "basement_type", label: "Bsmnt", section: houseSectionStructure,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Basement").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				if p.BasementType != "" {
					return "Yes"
				}
				return "No"
			},
			ptr:      func(fd *houseFormData) *string { return &fd.BasementType },
			validate: nil,
			toggle: func(cur string) string {
				if cur != "" {
					return ""
				}
				return "Yes"
			},
		},
		// Utilities
		{
			key: "heating_type", label: "Heat", section: houseSectionUtilities,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Heating type").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.HeatingType
			},
			ptr:      func(fd *houseFormData) *string { return &fd.HeatingType },
			validate: nil,
		},
		{
			key: "cooling_type", label: "Cool", section: houseSectionUtilities,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Cooling type").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.CoolingType
			},
			ptr:      func(fd *houseFormData) *string { return &fd.CoolingType },
			validate: nil,
		},
		{
			key: "water_source", label: "Water", section: houseSectionUtilities,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Water source").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.WaterSource
			},
			ptr:      func(fd *houseFormData) *string { return &fd.WaterSource },
			validate: nil,
		},
		{
			key: "sewer_type", label: "Sewer", section: houseSectionUtilities,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Sewer type").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.SewerType
			},
			ptr:      func(fd *houseFormData) *string { return &fd.SewerType },
			validate: nil,
		},
		{
			key: "parking_type", label: "Parking", section: houseSectionUtilities,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Parking type").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.ParkingType
			},
			ptr:      func(fd *houseFormData) *string { return &fd.ParkingType },
			validate: nil,
		},
		// Financial
		{
			key: "insurance_carrier", label: "Ins carrier", section: houseSectionFinancial,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Insurance carrier").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.InsuranceCarrier
			},
			ptr:      func(fd *houseFormData) *string { return &fd.InsuranceCarrier },
			validate: nil,
		},
		{
			key: "insurance_policy", label: "Ins policy", section: houseSectionFinancial,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("Insurance policy").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.InsurancePolicy
			},
			ptr:      func(fd *houseFormData) *string { return &fd.InsurancePolicy },
			validate: nil,
		},
		{
			key: "insurance_renewal", label: "Ins renewal", section: houseSectionFinancial,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().
					Title("Insurance renewal (YYYY-MM-DD)").
					Value(v).
					Validate(optionalDate("insurance renewal"))
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return data.FormatDate(p.InsuranceRenewal)
			},
			ptr:      func(fd *houseFormData) *string { return &fd.InsuranceRenewal },
			validate: optionalDate("insurance renewal"),
		},
		{
			key: "property_tax", label: "Prop tax", section: houseSectionFinancial,
			build: func(m *Model, v *string) huh.Field {
				return huh.NewInput().
					Title("Property tax (annual)").
					Placeholder("4200.00").
					Value(v).
					Validate(optionalMoney("property tax", m.cur))
			},
			get: func(p data.HouseProfile, cur locale.Currency, _ data.UnitSystem) string {
				return cur.FormatOptionalCents(p.PropertyTaxCents)
			},
			ptr:      func(fd *houseFormData) *string { return &fd.PropertyTax },
			validate: nil, // currency-dependent; validated by saveHouseFormData
		},
		{
			key: "hoa_name", label: "HOA", section: houseSectionFinancial,
			build: func(_ *Model, v *string) huh.Field {
				return huh.NewInput().Title("HOA name").Value(v)
			},
			get: func(p data.HouseProfile, _ locale.Currency, _ data.UnitSystem) string {
				return p.HOAName
			},
			ptr:      func(fd *houseFormData) *string { return &fd.HOAName },
			validate: nil,
		},
		{
			key: "hoa_fee", label: "HOA fee", section: houseSectionFinancial,
			build: func(m *Model, v *string) huh.Field {
				return huh.NewInput().
					Title("HOA fee (monthly)").
					Placeholder("250.00").
					Value(v).
					Validate(optionalMoney("HOA fee", m.cur))
			},
			get: func(p data.HouseProfile, cur locale.Currency, _ data.UnitSystem) string {
				return cur.FormatOptionalCents(p.HOAFeeCents)
			},
			ptr:      func(fd *houseFormData) *string { return &fd.HOAFee },
			validate: nil, // currency-dependent; validated by saveHouseFormData
		},
	}
}
