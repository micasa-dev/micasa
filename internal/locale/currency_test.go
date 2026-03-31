// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package locale

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"
)

const (
	symbolEuro  = "\u20ac" // €
	symbolPound = "\u00a3" // £
	symbolYen   = "\uffe5" // ¥
)

func TestResolveUSD(t *testing.T) {
	t.Parallel()
	c, err := Resolve("USD", language.AmericanEnglish)
	require.NoError(t, err)
	assert.Equal(t, "USD", c.Code())
	assert.Equal(t, "$", c.Symbol())
}

func TestResolveEUR(t *testing.T) {
	t.Parallel()
	c, err := Resolve("EUR", language.German)
	require.NoError(t, err)
	assert.Equal(t, "EUR", c.Code())
	assert.Equal(t, symbolEuro, c.Symbol())
}

func TestResolveGBP(t *testing.T) {
	t.Parallel()
	c, err := Resolve("GBP", language.BritishEnglish)
	require.NoError(t, err)
	assert.Equal(t, "GBP", c.Code())
	assert.Equal(t, symbolPound, c.Symbol())
}

func TestResolveJPY(t *testing.T) {
	t.Parallel()
	c, err := Resolve("JPY", language.Japanese)
	require.NoError(t, err)
	assert.Equal(t, "JPY", c.Code())
	assert.Equal(t, symbolYen, c.Symbol())
}

func TestResolveInvalid(t *testing.T) {
	t.Parallel()
	_, err := Resolve("NOPE", language.AmericanEnglish)
	assert.Error(t, err)
}

func TestResolveCaseInsensitive(t *testing.T) {
	t.Parallel()
	c, err := Resolve("eur", language.German)
	require.NoError(t, err)
	assert.Equal(t, "EUR", c.Code())
}

func TestResolveEmpty(t *testing.T) {
	t.Parallel()
	c, err := Resolve("", language.AmericanEnglish)
	require.NoError(t, err)
	assert.Equal(t, "USD", c.Code())
}

func TestDefaultCurrency(t *testing.T) {
	t.Parallel()
	c := DefaultCurrency()
	assert.Equal(t, "USD", c.Code())
	assert.Equal(t, "$", c.Symbol())
}

func TestFormatCentsUSD(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	tests := []struct {
		name  string
		cents int64
		want  string
	}{
		{"zero", 0, "$0.00"},
		{"small", 99, "$0.99"},
		{"even dollars", 10000, "$100.00"},
		{"typical", 123456, "$1,234.56"},
		{"large", 100000000, "$1,000,000.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, c.FormatCents(tt.cents))
		})
	}
}

func TestFormatCentsNegative(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	assert.Equal(t, "-$5.00", c.FormatCents(-500))
}

func TestFormatCentsMinInt64(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	formatted := c.FormatCents(math.MinInt64)
	assert.Contains(t, formatted, "-")
	assert.Contains(t, formatted, "$")
}

func TestFormatOptionalCentsNil(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	assert.Empty(t, c.FormatOptionalCents(nil))
}

func TestFormatOptionalCentsNonNil(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	cents := int64(123456)
	assert.Equal(t, "$1,234.56", c.FormatOptionalCents(&cents))
}

func TestFormatCentsEUR(t *testing.T) {
	t.Parallel()
	c := MustResolve("EUR", language.German)
	formatted := c.FormatCents(123456)
	assert.Contains(t, formatted, c.Symbol())
	assert.Contains(t, formatted, "1.234,56")
}

func TestStripSymbol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		code     string
		tag      language.Tag
		cents    int64
		wantSym  bool // false = symbol must NOT appear in stripped output
		contains string
	}{
		{"USD prefix", "USD", language.AmericanEnglish, 77653, false, "776.53"},
		{"USD negative", "USD", language.AmericanEnglish, -77653, false, "776.53"},
		{"EUR German suffix", "EUR", language.German, 123456, false, "1.234,56"},
		{"EUR French suffix", "EUR", language.MustParse("fr"), 99900, false, "999,00"},
		{"GBP prefix", "GBP", language.BritishEnglish, 50000, false, "500.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := MustResolve(tt.code, tt.tag)
			formatted := c.FormatCents(tt.cents)
			stripped := c.StripSymbol(formatted)
			assert.NotContains(t, stripped, c.Symbol(), "symbol should be removed")
			assert.Contains(t, stripped, tt.contains, "number should be preserved")
			if !tt.wantSym {
				// Also verify no stray nbsp left over.
				assert.NotContains(t, stripped, "\u00a0", "no stray non-breaking space")
			}
		})
	}
}

func TestFormatCentsGBP(t *testing.T) {
	t.Parallel()
	c := MustResolve("GBP", language.BritishEnglish)
	formatted := c.FormatCents(123456)
	assert.Contains(t, formatted, c.Symbol())
	assert.Contains(t, formatted, "1,234.56")
}

// TestFormatCentsEURFrench verifies that the same EUR currency formats
// differently when the formatting locale is French vs German.
func TestFormatCentsEURFrench(t *testing.T) {
	t.Parallel()
	fr := language.MustParse("fr")
	c := MustResolve("EUR", fr)
	formatted := c.FormatCents(123456)
	assert.Contains(t, formatted, c.Symbol(), "should contain euro sign")
	// French uses non-breaking space as grouping separator and comma
	// as decimal. The symbol is placed after the number.
	assert.Contains(t, formatted, ",56", "French uses comma decimal")
}

func TestFormatCompactCentsUSD(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	tests := []struct {
		name  string
		cents int64
		want  string
	}{
		{"zero", 0, "$0.00"},
		{"small", 999, "$9.99"},
		{"just under 1k", 99999, "$999.99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, c.FormatCompactCents(tt.cents))
		})
	}
}

func TestFormatCompactCentsLargeUSD(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	assert.Equal(t, "$1k", c.FormatCompactCents(100000))
	assert.Equal(t, "$1.2k", c.FormatCompactCents(123456))
	assert.Equal(t, "$45k", c.FormatCompactCents(4500000))
	assert.Equal(t, "$1M", c.FormatCompactCents(100000000))
}

func TestFormatCompactCentsEUR(t *testing.T) {
	t.Parallel()
	c := MustResolve("EUR", language.German)
	assert.Contains(t, c.FormatCompactCents(123456), "1,2k")
	assert.Contains(t, c.FormatCompactCents(130000000), "1,3M")
	assert.Contains(t, c.FormatCompactCents(100000), "1k")
}

func TestFormatCompactOptionalCentsNil(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	assert.Empty(t, c.FormatCompactOptionalCents(nil))
}

func TestParseRequiredCentsUSD(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	tests := []struct {
		input string
		want  int64
	}{
		{"100", 10000},
		{"100.5", 10050},
		{"100.05", 10005},
		{"$1,234.56", 123456},
		{".75", 75},
		{"0.99", 99},
	}
	for _, tt := range tests {
		got, err := c.ParseRequiredCents(tt.input)
		require.NoError(t, err, "input=%q", tt.input)
		assert.Equal(t, tt.want, got, "input=%q", tt.input)
	}
}

func TestParseRequiredCentsInvalid(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	for _, input := range []string{"", "12.345", "abc", "1.2.3"} {
		_, err := c.ParseRequiredCents(input)
		assert.Error(t, err, "input=%q", input)
	}
}

func TestParseCentsRejectsNegative(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	for _, input := range []string{"-$5.00", "-5.00", "-$1,234.56"} {
		_, err := c.ParseRequiredCents(input)
		assert.ErrorIs(t, err, ErrNegativeMoney, "input=%q", input)
	}
}

func TestParseCentsRoundtripUSD(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	values := []int64{0, 1, 99, 100, 123456}
	for _, cents := range values {
		formatted := c.FormatCents(cents)
		parsed, err := c.ParseRequiredCents(formatted)
		require.NoError(t, err, "roundtrip failed for %d (formatted=%q)", cents, formatted)
		assert.Equal(t, cents, parsed, "roundtrip mismatch for %d (formatted=%q)", cents, formatted)
	}
}

func TestParseCentsRoundtripEUR(t *testing.T) {
	t.Parallel()
	c := MustResolve("EUR", language.German)
	values := []int64{0, 1, 99, 100, 123456}
	for _, cents := range values {
		formatted := c.FormatCents(cents)
		parsed, err := c.ParseRequiredCents(formatted)
		require.NoError(t, err, "roundtrip failed for %d (formatted=%q)", cents, formatted)
		assert.Equal(t, cents, parsed, "roundtrip mismatch for %d (formatted=%q)", cents, formatted)
	}
}

func TestParseOptionalCentsEmpty(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	val, err := c.ParseOptionalCents("")
	require.NoError(t, err)
	assert.Nil(t, val)
}

func TestParseOptionalCentsValid(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	val, err := c.ParseOptionalCents("5")
	require.NoError(t, err)
	require.NotNil(t, val)
	assert.Equal(t, int64(500), *val)
}

func TestParseCentsEURFormat(t *testing.T) {
	t.Parallel()
	c := MustResolve("EUR", language.German)
	cents, err := c.ParseRequiredCents("1.234,56")
	require.NoError(t, err)
	assert.Equal(t, int64(123456), cents)
}

func TestCurrencyFromLocaleString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		locale string
		want   string
	}{
		{"en_US.UTF-8", "USD"},
		{"de_DE.UTF-8", "EUR"},
		{"ja_JP.UTF-8", "JPY"},
		{"en_GB.UTF-8", "GBP"},
		{"", ""},
		{"C", ""},
		{"POSIX", ""},
	}
	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			assert.Equal(t, tt.want, currencyFromLocaleString(tt.locale))
		})
	}
}

func TestParseCentsOverflow(t *testing.T) {
	t.Parallel()
	c := MustResolve("USD", language.AmericanEnglish)
	tests := []struct {
		name  string
		input string
	}{
		{"one dollar over", "$92233720368547759.00"},
		{"way over", "$999999999999999999999.99"},
		{"frac overflow at boundary", "$92233720368547758.08"},
		{"frac overflow .99 at boundary", "$92233720368547758.99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.ParseRequiredCents(tt.input)
			assert.Error(t, err, "should reject overflow: %s", tt.input)
		})
	}
}

func TestParseCentsAtMaxSafeValue(t *testing.T) {
	c := MustResolve("USD", language.AmericanEnglish)
	cents, err := c.ParseRequiredCents("$92233720368547758.00")
	require.NoError(t, err)
	assert.Equal(t, int64(9223372036854775800), cents)

	cents, err = c.ParseRequiredCents("$92233720368547758.07")
	require.NoError(t, err)
	assert.Equal(t, int64(9223372036854775807), cents)
}

func TestDetectLocaleDefault(t *testing.T) {
	// With no locale env vars set, should fall back to American English.
	for _, key := range []string{"LC_MONETARY", "LC_ALL", "LANG"} {
		t.Setenv(key, "")
	}
	assert.Equal(t, language.AmericanEnglish, DetectLocale())
}

func TestDetectLocaleFromLANG(t *testing.T) {
	t.Setenv("LC_MONETARY", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "de_DE.UTF-8")
	tag := DetectLocale()
	assert.Equal(t, language.MustParse("de-DE"), tag)
}

func TestDetectLocalePriority(t *testing.T) {
	t.Setenv("LC_MONETARY", "fr_FR.UTF-8")
	t.Setenv("LC_ALL", "de_DE.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	tag := DetectLocale()
	assert.Equal(t, language.MustParse("fr-FR"), tag, "LC_MONETARY should take priority")
}

// TestSameCurrencyDifferentLocales verifies that the same currency code
// produces different formatting when paired with different locales.
func TestSameCurrencyDifferentLocales(t *testing.T) {
	t.Parallel()
	de := MustResolve("EUR", language.German)
	fr := MustResolve("EUR", language.MustParse("fr"))

	deFmt := de.FormatCents(123456)
	frFmt := fr.FormatCents(123456)

	// Both should contain the euro sign.
	assert.Contains(t, deFmt, de.Symbol())
	assert.Contains(t, frFmt, fr.Symbol())

	// German uses period grouping; French uses non-breaking space.
	assert.Contains(t, deFmt, "1.234,56")
	assert.NotContains(t, frFmt, "1.234", "French should not use period grouping")
}
