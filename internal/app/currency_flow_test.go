// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestModelWithCurrency creates a store-backed Model whose database uses
// the given ISO 4217 currency code and explicit formatting locale. The
// currency code is persisted to SQLite so subsequent ResolveCurrency calls
// honour the DB value; the locale tag controls number formatting.
func newTestModelWithCurrency(t *testing.T, code string, tag language.Tag) *Model {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())

	cur := locale.MustResolve(code, tag)
	store.SetCurrency(cur)
	require.NoError(t, store.PutCurrency(code))

	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{
		Nickname: "Test House",
	}))

	m, err := NewModel(store, Options{DBPath: path})
	require.NoError(t, err)
	m.width = 120
	m.height = 40
	m.showDashboard = false
	return m
}

// seedProject creates a project and returns its ID.
func seedProject(t *testing.T, m *Model) uint {
	t.Helper()
	types, err := m.store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, types)
	p := data.Project{
		Title:         "Kitchen Reno",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, m.store.CreateProject(&p))
	return p.ID
}

// testLocales maps currency codes to their canonical test formatting locales.
var testLocales = map[string]language.Tag{
	"USD": language.AmericanEnglish,
	"EUR": language.German,
	"GBP": language.BritishEnglish,
	"JPY": language.Japanese,
	"CAD": language.MustParse("en-CA"),
	"AUD": language.MustParse("en-AU"),
}

func testLocale(code string) language.Tag {
	if tag, ok := testLocales[code]; ok {
		return tag
	}
	return language.AmericanEnglish
}

// allTestCurrencies is the set of currencies exercised across most
// table-driven tests.
var allTestCurrencies = []string{"USD", "EUR", "GBP", "JPY"}

// ---------------------------------------------------------------------------
// 1. Column headers show currency symbol
// ---------------------------------------------------------------------------

func TestCurrencyFlow_ColumnHeaders(t *testing.T) {
	for _, code := range []string{"USD", "EUR"} {
		t.Run(code, func(t *testing.T) {
			m := newTestModelWithCurrency(t, code, testLocale(code))

			// Create a project with a budget so a money cell appears.
			budget := int64(100000)
			types, _ := m.store.ProjectTypes()
			require.NoError(t, m.store.CreateProject(&data.Project{
				Title:         "Test",
				ProjectTypeID: types[0].ID,
				Status:        data.ProjectStatusPlanned,
				BudgetCents:   &budget,
			}))
			m.active = tabIndex(tabProjects)
			require.NoError(t, m.reloadActiveTab())
			// The rendered view should annotate money headers with the symbol.
			view := m.View()
			assert.Contains(t, view, m.cur.Symbol())
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Quote create round-trip: form input -> cents -> cell display
// ---------------------------------------------------------------------------

func TestCurrencyFlow_QuoteRoundTrip(t *testing.T) {
	tests := []struct {
		code      string
		input     string
		wantCents int64
	}{
		{"USD", "1,500.00", 150000},
		{"EUR", "2.500,00", 250000},
		{"GBP", "750.00", 75000},
		{"JPY", "15000", 1500000},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			m := newTestModelWithCurrency(t, tt.code, testLocale(tt.code))
			seedProject(t, m)

			// User navigates to Quotes tab and opens the add form.
			m.active = tabIndex(tabQuotes)
			openAddForm(m)

			values, ok := m.formData.(*quoteFormData)
			require.True(t, ok)
			values.VendorName = "TestCo"
			values.Total = tt.input
			sendKey(m, "ctrl+s")

			// Verify persisted cents.
			quotes, err := m.store.ListQuotes(false)
			require.NoError(t, err)
			require.Len(t, quotes, 1)
			assert.Equal(t, tt.wantCents, quotes[0].TotalCents)

			// Verify displayed value uses locale formatting after reload.
			sendKey(m, "esc")
			require.NoError(t, m.reloadActiveTab())
			tab := m.activeTab()
			require.NotEmpty(t, tab.CellRows)
			cell := tab.CellRows[0][int(quoteColTotal)].Value
			assert.Equal(t, m.cur.FormatCents(tt.wantCents), cell)
		})
	}
}

// ---------------------------------------------------------------------------
// 3. Formatting and compact notation
// ---------------------------------------------------------------------------

func TestCurrencyFlow_Formatting(t *testing.T) {
	tests := []struct {
		code     string
		cents    int64
		contains []string
	}{
		{"EUR", 123456, []string{"1.234", ",56"}},
		{"GBP", 123456, []string{"1,234.56"}},
		{"JPY", 150000, []string{"1,500"}},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			cur := locale.MustResolve(tt.code, testLocale(tt.code))
			formatted := cur.FormatCents(tt.cents)
			assert.Contains(t, formatted, cur.Symbol())
			for _, s := range tt.contains {
				assert.Contains(t, formatted, s)
			}
		})
	}
}

func TestCurrencyFlow_CompactNotation(t *testing.T) {
	for _, code := range []string{"USD", "EUR"} {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			// Large value should compress to compact form with symbol.
			compact := cur.FormatCompactCents(1234500)
			assert.Contains(t, compact, "k")
		})
	}
}

// ---------------------------------------------------------------------------
// 4. DB portability: EUR database opened with USD env still shows EUR
// ---------------------------------------------------------------------------

func TestCurrencyFlow_DBPortability(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portable.db")

	// First session: user sets up EUR database.
	store1, err := data.Open(path)
	require.NoError(t, err)
	require.NoError(t, store1.AutoMigrate())
	require.NoError(t, store1.SeedDefaults())
	require.NoError(t, store1.ResolveCurrency("EUR"))
	require.NoError(t, store1.CreateHouseProfile(data.HouseProfile{Nickname: "Euro House"}))

	types, _ := store1.ProjectTypes()
	require.NoError(t, store1.CreateProject(&data.Project{
		Title:         "Balcony",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))
	require.NoError(t, store1.Close())

	// Second session: different user opens with USD default.
	store2, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store2.Close() })
	require.NoError(t, store2.AutoMigrate())

	// ResolveCurrency should find "EUR" in the DB and ignore the "USD" arg.
	require.NoError(t, store2.ResolveCurrency("USD"))

	assert.Equal(t, "EUR", store2.Currency().Code(),
		"DB-persisted currency must be authoritative")
}

// ---------------------------------------------------------------------------
// 5. Resolution order: config > env > auto-detect > fallback
// ---------------------------------------------------------------------------

func TestCurrencyFlow_ResolutionOrder_ConfiguredCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())

	// ResolveCurrency with a configured code should use it.
	require.NoError(t, store.ResolveCurrency("GBP"))
	assert.Equal(t, "GBP", store.Currency().Code())

	// Verify it was persisted.
	code, err := store.GetCurrency()
	require.NoError(t, err)
	assert.Equal(t, "GBP", code)
}

func TestCurrencyFlow_ResolutionOrder_DBWinsOverConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())

	// First resolve: persists EUR.
	require.NoError(t, store.ResolveCurrency("EUR"))
	assert.Equal(t, "EUR", store.Currency().Code())

	// Second resolve: config says GBP but DB already has EUR.
	require.NoError(t, store.ResolveCurrency("GBP"))
	assert.Equal(t, "EUR", store.Currency().Code(),
		"DB currency must take precedence over config")
}

func TestCurrencyFlow_ResolutionOrder_EmptyFallsBackToUSD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())

	// Empty config and no env/locale -> USD fallback.
	require.NoError(t, store.ResolveCurrency(""))
	assert.Equal(t, "USD", store.Currency().Code())
}

// ---------------------------------------------------------------------------
// 6. Form validation: parsing with different currencies
// ---------------------------------------------------------------------------

func TestCurrencyFlow_ParseUSD(t *testing.T) {
	cur := locale.MustResolve("USD", language.AmericanEnglish)
	tests := []struct {
		name  string
		input string
		cents int64
	}{
		{"bare number", "1234.56", 123456},
		{"with symbol", "$1,234.56", 123456},
		{"whole dollars", "500", 50000},
		{"with grouping", "1,000", 100000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cur.ParseRequiredCents(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.cents, got)
		})
	}
}

func TestCurrencyFlow_ParseEUR(t *testing.T) {
	cur := locale.MustResolve("EUR", language.German)
	tests := []struct {
		name  string
		input string
		cents int64
	}{
		// In German locale, period is the grouping separator and comma is decimal.
		// "1234.56" = "123456" (dot stripped as grouping) = 12345600 cents.
		{"bare number dot treated as grouping", "1234.56", 12345600},
		{"locale comma decimal", "1234,56", 123456},
		{"with grouping", "1.234,56", 123456},
		{"whole amount", "500", 50000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cur.ParseRequiredCents(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.cents, got)
		})
	}
}

func TestCurrencyFlow_ParseGBP(t *testing.T) {
	cur := locale.MustResolve("GBP", language.BritishEnglish)
	tests := []struct {
		name  string
		input string
		cents int64
	}{
		{"bare number", "750.00", 75000},
		{"with symbol", cur.Symbol() + "750.00", 75000},
		{"with grouping", cur.Symbol() + "1,234.56", 123456},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cur.ParseRequiredCents(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.cents, got)
		})
	}
}

func TestCurrencyFlow_ParseRejectsNegative(t *testing.T) {
	for _, code := range []string{"USD", "EUR", "GBP"} {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			_, err := cur.ParseRequiredCents("-50.00")
			assert.ErrorIs(t, err, locale.ErrNegativeMoney)
		})
	}
}

func TestCurrencyFlow_ParseRejectsGarbage(t *testing.T) {
	for _, code := range []string{"USD", "EUR", "GBP"} {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			_, err := cur.ParseRequiredCents("abc")
			assert.ErrorIs(t, err, locale.ErrInvalidMoney)
		})
	}
}

func TestCurrencyFlow_ParseOptionalEmpty(t *testing.T) {
	for _, code := range []string{"USD", "EUR", "GBP"} {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			cents, err := cur.ParseOptionalCents("")
			require.NoError(t, err)
			assert.Nil(t, cents, "empty input should return nil for optional")
		})
	}
}

// ---------------------------------------------------------------------------
// 7. House profile financial display
// ---------------------------------------------------------------------------

func TestCurrencyFlow_HouseProfile(t *testing.T) {
	for _, code := range []string{"USD", "EUR"} {
		t.Run(code, func(t *testing.T) {
			m := newTestModelWithCurrency(t, code, testLocale(code))
			tax := int64(450000)
			m.house.PropertyTaxCents = &tax
			m.showHouse = true
			view := m.View()
			assert.Contains(t, view, m.cur.FormatOptionalCents(&tax))
		})
	}
}

func TestCurrencyFlow_HouseProfile_NilTax(t *testing.T) {
	m := newTestModelWithCurrency(t, "USD", language.AmericanEnglish)
	m.house.PropertyTaxCents = nil
	m.showHouse = true
	view := m.View()
	// Nil tax should not render "$0.00".
	assert.NotContains(t, view, "$0.00",
		"nil property tax should not show $0.00")
}

// ---------------------------------------------------------------------------
// 8. Row rendering with different currencies
// ---------------------------------------------------------------------------

func TestCurrencyFlow_ProjectRows(t *testing.T) {
	for _, code := range allTestCurrencies {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			budget := int64(250000)
			projects := []data.Project{
				{ID: 1, Title: "Test", Status: data.ProjectStatusPlanned, BudgetCents: &budget},
			}
			_, _, cells := projectRows(projects, nil, nil, cur)
			require.Len(t, cells, 1)
			assert.Equal(t, cur.FormatCents(250000), cells[0][4].Value)
		})
	}
}

func TestCurrencyFlow_QuoteRows(t *testing.T) {
	for _, code := range allTestCurrencies {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			quotes := []data.Quote{
				{
					ID: 1, ProjectID: 1, VendorID: 1,
					Project:    data.Project{Title: "Test"},
					Vendor:     data.Vendor{Name: "Co"},
					TotalCents: 75000,
				},
			}
			_, _, cells := quoteRows(quotes, nil, cur)
			require.Len(t, cells, 1)
			assert.Equal(t, cur.FormatCents(75000), cells[0][int(quoteColTotal)].Value)
		})
	}
}

func TestCurrencyFlow_ApplianceRows(t *testing.T) {
	for _, code := range allTestCurrencies {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			cost := int64(89900)
			now := time.Now()
			items := []data.Appliance{{ID: 1, Name: "Test", CostCents: &cost}}
			_, _, cells := applianceRows(items, nil, nil, now, cur)
			require.Len(t, cells, 1)
			assert.Equal(t, cur.FormatCents(89900), cells[0][9].Value)
		})
	}
}

// ---------------------------------------------------------------------------
// 9. Compact money cells strip symbol correctly
// ---------------------------------------------------------------------------

func TestCurrencyFlow_CompactMoneyCells(t *testing.T) {
	for _, code := range []string{"USD", "EUR"} {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			formatted := cur.FormatCents(523423)
			rows := [][]cell{
				{
					{Value: formatted, Kind: cellMoney},
					{Value: "Text", Kind: cellText},
				},
			}
			out := compactMoneyCells(rows, cur)
			// Symbol should be stripped; header carries the unit.
			assert.NotContains(t, out[0][0].Value, cur.Symbol())
			assert.Contains(t, out[0][0].Value, "k")
		})
	}
}

// ---------------------------------------------------------------------------
// 10. Header annotation with different currencies
// ---------------------------------------------------------------------------

func TestCurrencyFlow_AnnotateMoneyHeaders(t *testing.T) {
	for _, code := range []string{"USD", "EUR", "GBP"} {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			specs := []columnSpec{
				{Title: "Name", Kind: cellText},
				{Title: "Budget", Kind: cellMoney},
			}
			out := annotateMoneyHeaders(specs, cur)
			assert.Equal(t, "Name", out[0].Title)
			assert.Contains(t, out[1].Title, cur.Symbol())
		})
	}
}

// ---------------------------------------------------------------------------
// 11. Form value population with currency
// ---------------------------------------------------------------------------

func TestCurrencyFlow_IncidentFormValues(t *testing.T) {
	for _, code := range []string{"USD", "EUR", "GBP"} {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			cost := int64(15000)
			item := data.Incident{
				Title:       "Test",
				Status:      data.IncidentStatusOpen,
				Severity:    data.IncidentSeverityUrgent,
				DateNoticed: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
				CostCents:   &cost,
			}
			fd := incidentFormValues(item, cur)
			assert.Equal(t, cur.FormatCents(15000), fd.Cost)
		})
	}
}

func TestCurrencyFlow_ServiceLogFormValues(t *testing.T) {
	for _, code := range []string{"USD", "EUR", "GBP"} {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			cost := int64(5000)
			entry := data.ServiceLogEntry{
				MaintenanceItemID: 1,
				ServicedAt:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				CostCents:         &cost,
			}
			fd := serviceLogFormValues(entry, cur)
			assert.Equal(t, cur.FormatCents(5000), fd.Cost)
		})
	}
}

func TestCurrencyFlow_QuoteFormValues(t *testing.T) {
	for _, code := range []string{"USD", "EUR", "GBP"} {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			quote := data.Quote{
				ProjectID:  1,
				TotalCents: 250000,
				Vendor:     data.Vendor{Name: "TestCo"},
			}
			fd := quoteFormValues(quote, cur)
			assert.Equal(t, cur.FormatCents(250000), fd.Total)
		})
	}
}

// ---------------------------------------------------------------------------
// 12. Pin and filter with currency-formatted values
// ---------------------------------------------------------------------------

func TestCurrencyFlow_PinFilter_USD(t *testing.T) {
	m := newTestModelWithCurrency(t, "USD", language.AmericanEnglish)
	projID := seedProject(t, m)

	for _, total := range []int64{50000, 75000} {
		require.NoError(t, m.store.CreateQuote(&data.Quote{
			ProjectID:  projID,
			TotalCents: total,
		}, data.Vendor{Name: "TestCo"}))
	}

	m.active = tabIndex(tabQuotes)
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	require.Len(t, tab.Rows, 2, "should have 2 quotes")

	totalCol := int(quoteColTotal)
	tab.Table.SetCursor(0)
	tab.ColCursor = totalCol

	sendKey(m, keyN)
	sendKey(m, keyShiftN)
	assert.True(t, tab.FilterActive, "filter should be active after N")
	assert.Len(t, tab.Rows, 1, "should filter to 1 quote")

	sendKey(m, keyCtrlN)
	assert.Len(t, tab.Rows, 2, "all quotes restored after clearing pins")
}

// ---------------------------------------------------------------------------
// 13. Invalid currency code
// ---------------------------------------------------------------------------

func TestCurrencyFlow_InvalidCode(t *testing.T) {
	_, err := locale.Resolve("NOPE", language.AmericanEnglish)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown currency")
}

func TestCurrencyFlow_InvalidCode_ResolveCurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())

	// Manually put an invalid code in the DB.
	require.NoError(t, store.PutCurrency("NOPE"))

	err = store.ResolveCurrency("USD")
	assert.Error(t, err, "ResolveCurrency should fail for invalid DB code")
}

// ---------------------------------------------------------------------------
// 14. Format -> Parse round-trip for multiple currencies
// ---------------------------------------------------------------------------

func TestCurrencyFlow_FormatParseRoundTrip(t *testing.T) {
	// CHF excluded: uses apostrophe grouping (10'000) which the parser
	// does not yet handle. See plans/locale-aware-currency.md.
	codes := []string{"USD", "EUR", "GBP", "JPY", "CAD", "AUD"}
	amounts := []int64{0, 100, 12345, 99999, 1000000, 123456789}

	for _, code := range codes {
		cur := locale.MustResolve(code, testLocale(code))
		for _, cents := range amounts {
			t.Run(code+"/"+cur.FormatCents(cents), func(t *testing.T) {
				formatted := cur.FormatCents(cents)
				parsed, err := cur.ParseRequiredCents(formatted)
				require.NoError(t, err, "round-trip parse failed for %s %q", code, formatted)
				assert.Equal(
					t,
					cents,
					parsed,
					"round-trip mismatch for %s: formatted %q parsed to %d",
					code,
					formatted,
					parsed,
				)
			})
		}
	}
}

// ---------------------------------------------------------------------------
// 15. End-to-end: create quote via form, edit it, verify display
// ---------------------------------------------------------------------------

func TestCurrencyFlow_EndToEnd_QuoteEditCycle(t *testing.T) {
	// Test with input that's unambiguous for each locale: use the locale's
	// own formatted output as form input (the realistic "edit existing" path).
	for _, code := range []string{"USD", "EUR", "GBP"} {
		t.Run(code, func(t *testing.T) {
			tag := testLocale(code)
			m := newTestModelWithCurrency(t, code, tag)
			seedProject(t, m)
			cur := m.cur

			// User navigates to Quotes tab and creates a quote via form.
			m.active = tabIndex(tabQuotes)
			openAddForm(m)

			createValues, ok := m.formData.(*quoteFormData)
			require.True(t, ok, "expected quoteFormData")
			createValues.VendorName = "TestCo"
			createValues.Total = cur.FormatCents(150000)
			sendKey(m, "ctrl+s")
			sendKey(m, "esc")

			// Verify persisted cents.
			quotes, err := m.store.ListQuotes(false)
			require.NoError(t, err)
			require.Len(t, quotes, 1)
			assert.Equal(t, int64(150000), quotes[0].TotalCents)

			// Reload table and open the edit form via user interaction.
			require.NoError(t, m.reloadActiveTab())
			tab := m.activeTab()
			require.NotEmpty(t, tab.Rows)
			tab.Table.SetCursor(0)

			sendKey(m, "i")
			tab.ColCursor = int(quoteColID)
			sendKey(m, "e")
			require.Equal(t, modeForm, m.mode, "should open full edit form")

			// Verify form pre-population uses correct currency format.
			editValues, ok := m.formData.(*quoteFormData)
			require.True(t, ok, "expected quoteFormData for edit")
			assert.Equal(t, cur.FormatCents(150000), editValues.Total)

			// User changes the total.
			editValues.Total = cur.FormatCents(200000)
			sendKey(m, "ctrl+s")
			sendKey(m, "esc")

			// Verify update persisted.
			quote, err := m.store.GetQuote(quotes[0].ID)
			require.NoError(t, err)
			assert.Equal(t, int64(200000), quote.TotalCents)

			// Verify cell rendering after reload.
			require.NoError(t, m.reloadActiveTab())
			tab = m.activeTab()
			require.NotEmpty(t, tab.CellRows)
			assert.Equal(t, cur.FormatCents(200000), tab.CellRows[0][int(quoteColTotal)].Value)
		})
	}
}

// ---------------------------------------------------------------------------
// 16. Mag mode with currency
// ---------------------------------------------------------------------------

func TestCurrencyFlow_MagModeToggle(t *testing.T) {
	m := newTestModelWithCurrency(t, "USD", language.AmericanEnglish)

	budget := int64(150000)
	types, _ := m.store.ProjectTypes()
	require.NoError(t, m.store.CreateProject(&data.Project{
		Title:         "Deck",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
		BudgetCents:   &budget,
	}))

	m.active = tabIndex(tabProjects)
	require.NoError(t, m.reloadActiveTab())

	sendKey(m, keyCtrlO)
	assert.True(t, m.magMode, "mag mode should be active after ctrl+o")

	sendKey(m, keyCtrlO)
	assert.False(t, m.magMode, "mag mode should be off after second ctrl+o")
}

// ---------------------------------------------------------------------------
// 17. CentsCell and CentsValue helpers
// ---------------------------------------------------------------------------

func TestCurrencyFlow_CentsHelpers(t *testing.T) {
	for _, code := range allTestCurrencies {
		t.Run(code, func(t *testing.T) {
			cur := locale.MustResolve(code, testLocale(code))
			v := int64(123456)
			c := centsCell(&v, cur)
			assert.False(t, c.Null)
			assert.Equal(t, cellMoney, c.Kind)
			assert.Equal(t, cur.FormatCents(v), c.Value)

			val := centsValue(&v, cur)
			assert.Equal(t, cur.FormatCents(v), val)
		})
	}
}

// ---------------------------------------------------------------------------
// 18. Locale-driven formatting via environment variables
// ---------------------------------------------------------------------------

func TestCurrencyFlow_FrenchLocale_FormAndView(t *testing.T) {
	// Simulate: LANG=fr_FR.UTF-8 micasa
	// User expects EUR with French conventions (space grouping, comma decimal).
	t.Setenv("LC_MONETARY", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "fr_FR.UTF-8")

	path := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())

	// ResolveCurrency detects EUR from fr_FR locale, formatting from LANG.
	require.NoError(t, store.ResolveCurrency(""))
	assert.Equal(t, "EUR", store.Currency().Code())

	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{Nickname: "Maison"}))

	m, err := NewModel(store, Options{DBPath: path})
	require.NoError(t, err)
	m.width = 120
	m.height = 40
	m.showDashboard = false

	// User creates a project with a budget via form.
	types, _ := store.ProjectTypes()
	require.NotEmpty(t, types)
	m.active = tabIndex(tabProjects)
	openAddForm(m)

	pf, ok := m.formData.(*projectFormData)
	require.True(t, ok)
	pf.Title = "Terrasse"
	pf.ProjectTypeID = types[0].ID
	pf.Status = data.ProjectStatusPlanned
	pf.Budget = "2500,00" // French: comma decimal, no grouping (users rarely type NBSP)
	sendKey(m, "ctrl+s")

	// Verify cents stored correctly.
	projects, err := store.ListProjects(false)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.NotNil(t, projects[0].BudgetCents)
	assert.Equal(t, int64(250000), *projects[0].BudgetCents)

	// View should render with French formatting (comma decimal, not period).
	// Compact notation uses comma as decimal: "2,5k" (French) vs "2.5k" (US).
	sendKey(m, "esc")
	require.NoError(t, m.reloadActiveTab())
	view := m.View()
	assert.Contains(t, view, "2,5k",
		"French locale should use comma decimal in compact notation")
}

func TestCurrencyFlow_GermanLocale_FormAndView(t *testing.T) {
	// Simulate: LANG=de_DE.UTF-8 micasa
	// User expects EUR with German conventions (period grouping, comma decimal).
	t.Setenv("LC_MONETARY", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "de_DE.UTF-8")

	path := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())

	require.NoError(t, store.ResolveCurrency(""))
	assert.Equal(t, "EUR", store.Currency().Code())

	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{Nickname: "Haus"}))

	m, err := NewModel(store, Options{DBPath: path})
	require.NoError(t, err)
	m.width = 120
	m.height = 40
	m.showDashboard = false

	// User creates a quote with German formatting.
	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Dach",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))

	m.active = tabIndex(tabQuotes)
	openAddForm(m)

	qf, ok := m.formData.(*quoteFormData)
	require.True(t, ok)
	qf.VendorName = "DachCo"
	qf.Total = "3.500,00" // German: period grouping, comma decimal
	sendKey(m, "ctrl+s")

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, int64(350000), quotes[0].TotalCents)

	// View should show period grouping.
	sendKey(m, "esc")
	require.NoError(t, m.reloadActiveTab())
	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)
	totalCell := tab.CellRows[0][int(quoteColTotal)].Value
	assert.Contains(t, totalCell, "3.500")
	assert.Contains(t, totalCell, ",00")
}

func TestCurrencyFlow_DBPortability_DifferentLocale(t *testing.T) {
	// Simulate: EUR database created on a German machine, opened on an
	// American machine. Currency stays EUR; formatting switches to en-US.
	path := filepath.Join(t.TempDir(), "portable.db")

	// First session: German user creates EUR data.
	t.Setenv("LANG", "de_DE.UTF-8")
	t.Setenv("LC_MONETARY", "")
	t.Setenv("LC_ALL", "")

	store1, err := data.Open(path)
	require.NoError(t, err)
	require.NoError(t, store1.AutoMigrate())
	require.NoError(t, store1.SeedDefaults())
	require.NoError(t, store1.ResolveCurrency(""))
	require.Equal(t, "EUR", store1.Currency().Code())

	require.NoError(t, store1.CreateHouseProfile(data.HouseProfile{Nickname: "Haus"}))
	types, _ := store1.ProjectTypes()
	budget := int64(250000)
	require.NoError(t, store1.CreateProject(&data.Project{
		Title:         "Balkon",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
		BudgetCents:   &budget,
	}))
	require.NoError(t, store1.Close())

	// Second session: American user opens the same database.
	t.Setenv("LANG", "en_US.UTF-8")

	store2, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store2.Close() })
	require.NoError(t, store2.AutoMigrate())
	require.NoError(t, store2.ResolveCurrency(""))

	// Currency code stays EUR (from DB), but formatting is now American.
	assert.Equal(t, "EUR", store2.Currency().Code())

	m, err := NewModel(store2, Options{DBPath: path})
	require.NoError(t, err)
	m.width = 120
	m.height = 40
	m.showDashboard = false

	m.active = tabIndex(tabProjects)
	require.NoError(t, m.reloadActiveTab())

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)
	// American formatting: comma grouping, period decimal.
	budgetCell := tab.CellRows[0][4].Value
	assert.Contains(t, budgetCell, "2,500",
		"American locale should use comma grouping for EUR")
	assert.Contains(t, budgetCell, ".00",
		"American locale should use period decimal for EUR")
}

func TestCurrencyFlow_EnvCurrency_FirstRun(t *testing.T) {
	// Simulate: MICASA_CURRENCY=GBP micasa (first run, no DB currency yet).
	t.Setenv("MICASA_CURRENCY", "GBP")
	t.Setenv("LC_MONETARY", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "en_GB.UTF-8")

	path := filepath.Join(t.TempDir(), "test.db")
	store, err := data.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.AutoMigrate())
	require.NoError(t, store.SeedDefaults())

	// ResolveCurrency with empty config should pick up MICASA_CURRENCY.
	require.NoError(t, store.ResolveCurrency(""))
	assert.Equal(t, "GBP", store.Currency().Code())
	cur := store.Currency()
	assert.NotEmpty(t, cur.Symbol())

	// Verify it was persisted to DB.
	code, err := store.GetCurrency()
	require.NoError(t, err)
	assert.Equal(t, "GBP", code)

	// User creates data and sees pound formatting.
	require.NoError(t, store.CreateHouseProfile(data.HouseProfile{Nickname: "Home"}))
	m, err := NewModel(store, Options{DBPath: path})
	require.NoError(t, err)
	m.width = 120
	m.height = 40
	m.showDashboard = false

	types, _ := store.ProjectTypes()
	require.NotEmpty(t, types)
	m.active = tabIndex(tabQuotes)

	require.NoError(t, store.CreateProject(&data.Project{
		Title:         "Garden",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}))

	openAddForm(m)
	qf, ok := m.formData.(*quoteFormData)
	require.True(t, ok)
	qf.VendorName = "GardenCo"
	qf.Total = "500.00"
	sendKey(m, "ctrl+s")

	quotes, err := store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, int64(50000), quotes[0].TotalCents)

	// View should show pound symbol.
	sendKey(m, "esc")
	require.NoError(t, m.reloadActiveTab())
	view := m.View()
	assert.Contains(t, view, cur.Symbol(),
		"GBP from MICASA_CURRENCY should render pound symbol")
}
