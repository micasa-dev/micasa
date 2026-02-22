// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package locale

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"golang.org/x/text/currency"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Currency holds resolved currency formatting state. The currency unit
// (what money) and formatting locale (how to display numbers) are
// independent concerns -- like a timestamp (UTC) and a timezone (display).
//
// Safe for concurrent read access; treat as immutable after creation.
type Currency struct {
	unit    currency.Unit
	tag     language.Tag
	symbol  string
	prefix  bool // true if symbol comes before the number
	code    string
	group   string // cached grouping separator (e.g. "," or ".")
	decimal string // cached decimal separator (e.g. "." or ",")
}

const nbsp = "\u00a0" // non-breaking space between number and suffix symbol

var (
	ErrInvalidMoney  = errors.New("invalid money value")
	ErrNegativeMoney = errors.New("negative money value")
)

// Resolve creates a Currency from an ISO 4217 code and a formatting locale.
// The code determines the currency unit and symbol; the tag determines
// number grouping, decimal separator, and symbol placement.
func Resolve(code string, tag language.Tag) (Currency, error) {
	if code == "" {
		code = "USD"
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	unit, err := currency.ParseISO(code)
	if err != nil {
		return Currency{}, fmt.Errorf("unknown currency %q: %w", code, err)
	}
	sym, pre := extractSymbol(unit, tag)
	group, dec := deriveSeparators(tag)
	return Currency{
		unit:    unit,
		tag:     tag,
		symbol:  sym,
		prefix:  pre,
		code:    code,
		group:   group,
		decimal: dec,
	}, nil
}

// MustResolve is like Resolve but panics on error.
func MustResolve(code string, tag language.Tag) Currency {
	c, err := Resolve(code, tag)
	if err != nil {
		panic(err)
	}
	return c
}

// DefaultCurrency returns USD with standard US English formatting.
func DefaultCurrency() Currency {
	return MustResolve("USD", language.AmericanEnglish)
}

// ResolveDefault resolves the currency code using the config layering:
// explicit code > MICASA_CURRENCY env > LC_MONETARY/LANG auto-detect > USD.
// The formatting locale is always detected from the environment.
func ResolveDefault(configured string) (Currency, error) {
	code := configured
	if code == "" {
		code = os.Getenv("MICASA_CURRENCY")
	}
	if code == "" {
		code = detectCurrencyFromLocale()
	}
	if code == "" {
		code = "USD"
	}
	return Resolve(code, DetectLocale())
}

// DetectLocale reads the user's formatting locale from the environment.
// Checks LC_MONETARY, LC_ALL, then LANG. Falls back to American English.
func DetectLocale() language.Tag {
	for _, key := range []string{"LC_MONETARY", "LC_ALL", "LANG"} {
		if val := os.Getenv(key); val != "" {
			tag, err := parseLocaleString(val)
			if err == nil {
				return tag
			}
		}
	}
	return language.AmericanEnglish
}

// Code returns the ISO 4217 code (e.g. "USD", "EUR").
func (c Currency) Code() string {
	return c.code
}

// Symbol returns the narrow symbol glyph (e.g. "$", "EUR", "GBP", "JPY").
func (c Currency) Symbol() string {
	return c.symbol
}

// FormatCents formats an int64 cent value as a locale-appropriate currency
// string. Uses the locale's number grouping and decimal separator, with the
// currency symbol placed per locale convention (no extra space).
func (c Currency) FormatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		if cents == math.MinInt64 {
			cents = math.MaxInt64
		} else {
			cents = -cents
		}
	}
	dollars := cents / 100
	remainder := cents % 100
	p := message.NewPrinter(c.tag)
	numStr := p.Sprintf("%d", dollars)
	number := fmt.Sprintf("%s%s%02d", numStr, c.decimal, remainder)
	if c.prefix {
		return sign + c.symbol + number
	}
	return sign + number + nbsp + c.symbol
}

// FormatOptionalCents formats a *int64 cent value, returning "" for nil.
func (c Currency) FormatOptionalCents(cents *int64) string {
	if cents == nil {
		return ""
	}
	return c.FormatCents(*cents)
}

// FormatCompactCents formats cents using abbreviated notation for large
// values (e.g. 1.2k, 45k, 1.3M) with the correct currency symbol.
// Values under 1,000 in the base unit use full precision.
func (c Currency) FormatCompactCents(cents int64) string {
	sign := ""
	absCents := cents
	if cents < 0 {
		sign = "-"
		if cents == math.MinInt64 {
			absCents = math.MaxInt64
		} else {
			absCents = -cents
		}
	}
	dollars := float64(absCents) / 100.0
	if dollars < 1000 {
		if sign != "" {
			return sign + c.FormatCents(absCents)
		}
		return c.FormatCents(cents)
	}
	si := humanize.SIWithDigits(dollars, 1, "")
	si = strings.Replace(si, " ", "", 1)
	if c.decimal != "." {
		si = strings.Replace(si, ".", c.decimal, 1)
	}
	if c.prefix {
		return sign + c.symbol + si
	}
	return sign + si + nbsp + c.symbol
}

// FormatCompactOptionalCents formats optional cents compactly.
func (c Currency) FormatCompactOptionalCents(cents *int64) string {
	if cents == nil {
		return ""
	}
	return c.FormatCompactCents(*cents)
}

// ParseRequiredCents parses a user-entered money string into cents.
// Strips the currency symbol if present; bare numbers always accepted.
func (c Currency) ParseRequiredCents(input string) (int64, error) {
	cents, err := c.parseCents(strings.TrimSpace(input))
	if err != nil {
		return 0, err
	}
	return cents, nil
}

// ParseOptionalCents parses an optional money string. Returns (nil, nil) for
// empty input.
func (c Currency) ParseOptionalCents(input string) (*int64, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}
	cents, err := c.parseCents(trimmed)
	if err != nil {
		return nil, err
	}
	return &cents, nil
}

func (c Currency) parseCents(input string) (int64, error) {
	clean := c.normalizeNumber(input)

	if strings.HasPrefix(clean, "-") {
		return 0, ErrNegativeMoney
	}
	clean = strings.TrimPrefix(clean, c.symbol)
	clean = strings.TrimSuffix(clean, c.symbol)
	clean = strings.TrimSpace(clean)
	// Also strip $ as a universal fallback (for pasted values).
	clean = strings.TrimPrefix(clean, "$")
	if clean == "" {
		return 0, ErrInvalidMoney
	}
	parts := strings.Split(clean, ".")
	if len(parts) > 2 {
		return 0, ErrInvalidMoney
	}
	wholePart, err := parseDigits(parts[0], true)
	if err != nil {
		return 0, ErrInvalidMoney
	}
	const maxDollars = math.MaxInt64 / 100
	if wholePart > maxDollars {
		return 0, ErrInvalidMoney
	}
	frac := int64(0)
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, ErrInvalidMoney
		}
		frac, err = parseDigits(parts[1], false)
		if err != nil {
			return 0, ErrInvalidMoney
		}
		if len(parts[1]) == 1 {
			frac *= 10
		}
	}
	cents := wholePart*100 + frac
	if cents < 0 {
		return 0, ErrInvalidMoney
	}
	return cents, nil
}

// normalizeNumber removes locale-specific grouping separators and replaces
// the locale-specific decimal separator with ".".
func (c Currency) normalizeNumber(input string) string {
	result := strings.ReplaceAll(input, c.group, "")
	if c.decimal != "." {
		result = strings.Replace(result, c.decimal, ".", 1)
	}
	return result
}

// deriveSeparators determines grouping and decimal separators for a locale
// by formatting a known number and inspecting the output. Operates on runes
// to correctly handle multi-byte separators (e.g. U+00A0 non-breaking space
// in French).
func deriveSeparators(tag language.Tag) (group, decimal string) {
	p := message.NewPrinter(tag)
	formatted := p.Sprintf("%.1f", 1234.5)
	runes := []rune(formatted)
	lastNonDigit := -1
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] < '0' || runes[i] > '9' {
			lastNonDigit = i
			break
		}
	}
	if lastNonDigit < 0 {
		return ",", "."
	}
	decimal = string(runes[lastNonDigit])
	for i := 0; i < lastNonDigit; i++ {
		if runes[i] < '0' || runes[i] > '9' {
			return string(runes[i]), decimal
		}
	}
	if decimal == "." {
		return ",", "."
	}
	return ".", decimal
}

func parseDigits(input string, allowEmpty bool) (int64, error) {
	if input == "" {
		if allowEmpty {
			return 0, nil
		}
		return 0, ErrInvalidMoney
	}
	for _, r := range input {
		if r < '0' || r > '9' {
			return 0, ErrInvalidMoney
		}
	}
	return strconv.ParseInt(input, 10, 64)
}

// extractSymbol formats a zero amount and extracts the symbol glyph and its
// position (prefix or suffix) from the CLDR-formatted output.
func extractSymbol(unit currency.Unit, tag language.Tag) (sym string, prefix bool) {
	p := message.NewPrinter(tag)
	formatted := p.Sprint(currency.NarrowSymbol(unit.Amount(0)))
	firstDigit := strings.IndexAny(formatted, "0123456789")
	lastDigit := strings.LastIndexAny(formatted, "0123456789")
	if firstDigit < 0 {
		return unit.String(), true
	}
	pre := strings.TrimSpace(formatted[:firstDigit])
	suf := strings.TrimSpace(formatted[lastDigit+1:])
	if pre != "" {
		return pre, true
	}
	if suf != "" {
		return suf, false
	}
	return unit.String(), true
}

// parseLocaleString normalizes a POSIX locale string (e.g. "fr_FR.UTF-8")
// into a BCP 47 language tag.
func parseLocaleString(loc string) (language.Tag, error) {
	if idx := strings.IndexByte(loc, '.'); idx >= 0 {
		loc = loc[:idx]
	}
	if idx := strings.IndexByte(loc, '@'); idx >= 0 {
		loc = loc[:idx]
	}
	loc = strings.ReplaceAll(loc, "_", "-")
	return language.Parse(loc)
}

// detectCurrencyFromLocale tries to determine the currency from environment
// locale variables (LC_MONETARY, LC_ALL, LANG).
func detectCurrencyFromLocale() string {
	for _, key := range []string{"LC_MONETARY", "LC_ALL", "LANG"} {
		if val := os.Getenv(key); val != "" {
			if code := currencyFromLocaleString(val); code != "" {
				return code
			}
		}
	}
	return ""
}

// currencyFromLocaleString extracts a currency from a locale string like
// "de_DE.UTF-8" by parsing the region and looking up its currency.
func currencyFromLocaleString(loc string) string {
	tag, err := parseLocaleString(loc)
	if err != nil {
		return ""
	}
	unit, conf := currency.FromTag(tag)
	if conf == language.No {
		return ""
	}
	return unit.String()
}
