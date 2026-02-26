// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testValidatorDate = "2025-06-15"

func TestRequiredTextRejectsEmpty(t *testing.T) {
	validate := requiredText("title")
	assert.Error(t, validate(""))
	assert.Error(t, validate("  "))
}

func TestRequiredTextAcceptsNonEmpty(t *testing.T) {
	validate := requiredText("title")
	assert.NoError(t, validate("hello"))
}

func TestOptionalIntAcceptsValid(t *testing.T) {
	validate := optionalInt("months")
	for _, input := range []string{"", "0", "12", "  7  "} {
		assert.NoErrorf(t, validate(input), "optionalInt(%q)", input)
	}
}

func TestOptionalIntRejectsInvalid(t *testing.T) {
	validate := optionalInt("months")
	for _, input := range []string{"abc", "-1", "1.5"} {
		assert.Errorf(t, validate(input), "optionalInt(%q) expected error", input)
	}
}

func TestOptionalIntervalAcceptsValid(t *testing.T) {
	validate := optionalInterval("interval")
	for _, input := range []string{"", "0", "12", "6m", "1y", "2y 6m", "1y6m", "  1Y  "} {
		assert.NoErrorf(t, validate(input), "optionalInterval(%q)", input)
	}
}

func TestOptionalIntervalRejectsInvalid(t *testing.T) {
	validate := optionalInterval("interval")
	for _, input := range []string{"abc", "-1", "1x", "m", "y"} {
		err := validate(input)
		assert.Errorf(t, err, "optionalInterval(%q) expected error", input)
		if err != nil {
			assert.Contains(t, err.Error(), "6m, 1y, 2y 6m", "error should be actionable")
		}
	}
}

func TestOptionalFloatAcceptsValid(t *testing.T) {
	validate := optionalFloat("bathrooms")
	for _, input := range []string{"", "0", "2.5", "  3  "} {
		assert.NoErrorf(t, validate(input), "optionalFloat(%q)", input)
	}
}

func TestOptionalFloatRejectsInvalid(t *testing.T) {
	validate := optionalFloat("bathrooms")
	for _, input := range []string{"abc", "-1.5"} {
		assert.Errorf(t, validate(input), "optionalFloat(%q) expected error", input)
	}
}

func TestOptionalDateAcceptsValid(t *testing.T) {
	validate := optionalDate("start date")
	for _, input := range []string{"", "2025-06-11"} {
		assert.NoErrorf(t, validate(input), "optionalDate(%q)", input)
	}
}

func TestOptionalDateRejectsInvalid(t *testing.T) {
	validate := optionalDate("start date")
	for _, input := range []string{"06/11/2025", "not-a-date"} {
		assert.Errorf(t, validate(input), "optionalDate(%q) expected error", input)
	}
}

func TestEndDateAfterStart(t *testing.T) {
	cases := []struct {
		name    string
		start   string
		end     string
		wantErr string // empty means no error expected
	}{
		{
			"rejects earlier end",
			testValidatorDate,
			"2025-06-10",
			"end date must not be before start date",
		},
		{"accepts same day", testValidatorDate, testValidatorDate, ""},
		{"accepts later end", "2025-06-10", testValidatorDate, ""},
		{"accepts empty end", testValidatorDate, "", ""},
		{"accepts empty start", "", testValidatorDate, ""},
		{"accepts both empty", "", "", ""},
		{"rejects invalid end format", testValidatorDate, "not-a-date", "YYYY-MM-DD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := tc.start, tc.end
			validate := endDateAfterStart(&start, &end)
			err := validate(end)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestOptionalMoneyAcceptsValid(t *testing.T) {
	validate := optionalMoney("budget")
	for _, input := range []string{"", "100", "1250.00", "$5,000.50"} {
		assert.NoErrorf(t, validate(input), "optionalMoney(%q)", input)
	}
}

func TestOptionalMoneyRejectsInvalid(t *testing.T) {
	validate := optionalMoney("budget")
	assert.Error(t, validate("abc"))
}

func TestRequiredMoneyAcceptsValid(t *testing.T) {
	validate := requiredMoney("total")
	assert.NoError(t, validate("1250.00"))
}

func TestRequiredMoneyRejectsEmpty(t *testing.T) {
	validate := requiredMoney("total")
	assert.Error(t, validate(""))
}

func TestIntToString(t *testing.T) {
	assert.Empty(t, intToString(0))
	assert.Equal(t, "42", intToString(42))
}

func TestProjectFormValues(t *testing.T) {
	budget := int64(500000)
	start := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	project := data.Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: 1,
		Status:        data.ProjectStatusInProgress,
		BudgetCents:   &budget,
		StartDate:     &start,
		Description:   "Full gut renovation",
	}
	got := projectFormValues(project)
	assert.Equal(t, "Kitchen Remodel", got.Title)
	assert.Equal(t, "$5,000.00", got.Budget)
	assert.Equal(t, "2025-03-01", got.StartDate)
	assert.Empty(t, got.Actual)
}

func TestVendorFormValues(t *testing.T) {
	vendor := data.Vendor{
		Name:        "HVAC Pros",
		ContactName: "Alice",
		Email:       "alice@hvac.com",
		Phone:       "555-1234",
		Website:     "https://hvac.com",
		Notes:       "vendor notes",
	}
	got := vendorFormValues(vendor)
	assert.Equal(t, "HVAC Pros", got.Name)
	assert.Equal(t, "Alice", got.ContactName)
	assert.Equal(t, "alice@hvac.com", got.Email)
	assert.Equal(t, "555-1234", got.Phone)
	assert.Equal(t, "https://hvac.com", got.Website)
	assert.Equal(t, "vendor notes", got.Notes)
}

func TestQuoteFormValues(t *testing.T) {
	labor := int64(10000)
	quote := data.Quote{
		ProjectID:  1,
		TotalCents: 50000,
		LaborCents: &labor,
		Vendor:     data.Vendor{Name: "ContractorCo"},
	}
	got := quoteFormValues(quote)
	assert.Equal(t, "$500.00", got.Total)
	assert.Equal(t, "$100.00", got.Labor)
	assert.Empty(t, got.Materials)
	assert.Equal(t, "ContractorCo", got.VendorName)
}

func TestMaintenanceFormValues(t *testing.T) {
	appID := uint(3)
	item := data.MaintenanceItem{
		Name:           "HVAC Filter",
		CategoryID:     1,
		ApplianceID:    &appID,
		IntervalMonths: 3,
	}
	got := maintenanceFormValues(item)
	assert.Equal(t, "HVAC Filter", got.Name)
	assert.Equal(t, uint(3), got.ApplianceID)
	assert.Equal(t, "3m", got.IntervalMonths)
}

func TestMaintenanceFormValuesNoAppliance(t *testing.T) {
	item := data.MaintenanceItem{
		Name:       "Smoke Detectors",
		CategoryID: 1,
	}
	got := maintenanceFormValues(item)
	assert.Zero(t, got.ApplianceID)
}

func TestMaintenanceFormValuesDueDate(t *testing.T) {
	due := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
	item := data.MaintenanceItem{
		Name:       "Inspect Roof",
		CategoryID: 1,
		DueDate:    &due,
	}
	got := maintenanceFormValues(item)
	assert.Equal(t, "2025-11-01", got.DueDate)
	assert.Empty(t, got.IntervalMonths)
}

func TestApplianceFormValues(t *testing.T) {
	cost := int64(89900)
	purchase := time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)
	appliance := data.Appliance{
		Name:         "Fridge",
		Brand:        "Samsung",
		ModelNumber:  "RF28R7351SR",
		PurchaseDate: &purchase,
		CostCents:    &cost,
	}
	got := applianceFormValues(appliance)
	assert.Equal(t, "Fridge", got.Name)
	assert.Equal(t, "Samsung", got.Brand)
	assert.Equal(t, "2023-06-15", got.PurchaseDate)
	assert.Equal(t, "$899.00", got.Cost)
}

func TestHouseFormValues(t *testing.T) {
	profile := data.HouseProfile{
		Nickname:  "Home",
		YearBuilt: 1995,
		Bedrooms:  3,
		Bathrooms: 2.5,
	}
	got := houseFormValues(profile)
	assert.Equal(t, "Home", got.Nickname)
	assert.Equal(t, "1995", got.YearBuilt)
	assert.Equal(t, "3", got.Bedrooms)
	assert.Equal(t, "2.5", got.Bathrooms)
}

func TestServiceLogFormValues(t *testing.T) {
	cost := int64(15000)
	vendorID := uint(1)
	entry := data.ServiceLogEntry{
		ServicedAt: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
		VendorID:   &vendorID,
		CostCents:  &cost,
		Notes:      "replaced filter",
	}
	got := serviceLogFormValues(entry)
	assert.Equal(t, "2025-01-15", got.ServicedAt)
	assert.Equal(t, "$150.00", got.Cost)
	assert.Equal(t, uint(1), got.VendorID)
}

func TestServiceLogFormValuesNoVendor(t *testing.T) {
	entry := data.ServiceLogEntry{
		ServicedAt: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	got := serviceLogFormValues(entry)
	assert.Zero(t, got.VendorID)
	assert.Empty(t, got.Cost)
}

func TestFormDirtyDetectionUserFlow(t *testing.T) {
	m := newTestModel()
	m.mode = modeForm

	// Simulate: user opens an appliance edit form with pre-filled values.
	values := &applianceFormData{
		Name:  "Fridge",
		Brand: "Samsung",
		Cost:  "$899.00",
	}
	m.formData = values
	m.snapshotForm()

	// User hasn't changed anything yet — status should show "saved".
	m.checkFormDirty()
	status := m.statusView()
	assert.Contains(t, status, "saved", "status should show saved before any edits")
	assert.NotContains(t, status, "unsaved", "status should not show unsaved before edits")

	// User edits the brand field.
	values.Brand = "LG"
	m.checkFormDirty()
	assert.Contains(t, m.statusView(), "unsaved",
		"status should show unsaved after editing a field")

	// User reverts the edit back to the original value.
	values.Brand = "Samsung"
	m.checkFormDirty()
	status = m.statusView()
	assert.Contains(t, status, "saved", "status should show saved after reverting")
	assert.NotContains(t, status, "unsaved", "status should not show unsaved after reverting")
}

func TestOversizedDocumentShowsHumanReadableSize(t *testing.T) {
	m := newTestModelWithStore(t)

	// Set a small max to avoid writing a large temp file.
	require.NoError(t, m.store.SetMaxDocumentSize(1024))

	// Create a temp file that exceeds the limit.
	bigFile := filepath.Join(t.TempDir(), "big.pdf")
	require.NoError(t, os.WriteFile(bigFile, make([]byte, 2048), 0o600))

	// Simulate the user filling out the document form with the oversized file.
	m.formData = &documentFormData{
		Title:    "Too Big",
		FilePath: bigFile,
	}
	_, err := m.parseDocumentFormData()
	require.Error(t, err)

	// Error should contain human-readable sizes, not raw byte counts.
	assert.Contains(t, err.Error(), "2.0 KB")
	assert.Contains(t, err.Error(), "1.0 KB")
	assert.NotContains(t, err.Error(), "2048")
	assert.NotContains(t, err.Error(), "1024")
}

// TestFormDataStructsHaveNoReferenceFields ensures cloneFormData's shallow
// copy is safe. If any form data struct gains a pointer, slice, or map
// field, this test will catch it -- the snapshot would share that reference
// and dirty-detection via reflect.DeepEqual would silently break.
func TestFormDataStructsHaveNoReferenceFields(t *testing.T) {
	structs := []any{
		projectFormData{},
		applianceFormData{},
		maintenanceFormData{},
		vendorFormData{},
		quoteFormData{},
		serviceLogFormData{},
		documentFormData{},
		houseFormData{},
	}
	for _, s := range structs {
		rt := reflect.TypeOf(s)
		t.Run(rt.Name(), func(t *testing.T) {
			for i := range rt.NumField() {
				f := rt.Field(i)
				switch f.Type.Kind() { //nolint:exhaustive // only reference kinds matter here
				case reflect.Ptr, reflect.Slice, reflect.Map,
					reflect.Chan, reflect.Func, reflect.Interface:
					t.Errorf(
						"field %s.%s is %s -- cloneFormData requires value-only fields",
						rt.Name(), f.Name, f.Type.Kind(),
					)
				}
			}
		})
	}
}
