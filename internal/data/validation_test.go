// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRequiredCents(t *testing.T) {
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
	for _, test := range tests {
		got, err := ParseRequiredCents(test.input)
		require.NoError(t, err, "input=%q", test.input)
		assert.Equal(t, test.want, got, "input=%q", test.input)
	}
}

func TestParseRequiredCentsInvalid(t *testing.T) {
	for _, input := range []string{"", "12.345", "abc", "1.2.3"} {
		_, err := ParseRequiredCents(input)
		assert.Error(t, err, "input=%q", input)
	}
}

func TestParseOptionalCents(t *testing.T) {
	value, err := ParseOptionalCents("")
	require.NoError(t, err)
	assert.Nil(t, value)

	value, err = ParseOptionalCents("5")
	require.NoError(t, err)
	require.NotNil(t, value)
	assert.Equal(t, int64(500), *value)
}

func TestFormatCents(t *testing.T) {
	assert.Equal(t, "$1,234.56", FormatCents(123456))
}

func TestParseOptionalDate(t *testing.T) {
	date, err := ParseOptionalDate("2025-06-11")
	require.NoError(t, err)
	require.NotNil(t, date)
	assert.Equal(t, "2025-06-11", date.Format(DateLayout))

	_, err = ParseOptionalDate("06/11/2025")
	assert.Error(t, err)
}

func TestParseOptionalInt(t *testing.T) {
	value, err := ParseOptionalInt("12")
	require.NoError(t, err)
	assert.Equal(t, 12, value)

	_, err = ParseOptionalInt("-1")
	assert.Error(t, err)
}

func TestParseOptionalFloat(t *testing.T) {
	value, err := ParseOptionalFloat("2.5")
	require.NoError(t, err)
	assert.Equal(t, 2.5, value)

	_, err = ParseOptionalFloat("-1.2")
	assert.Error(t, err)
}

func TestFormatOptionalCents(t *testing.T) {
	assert.Empty(t, FormatOptionalCents(nil))
	cents := int64(123456)
	assert.Equal(t, "$1,234.56", FormatOptionalCents(&cents))
}

func TestFormatCentsNegative(t *testing.T) {
	assert.Equal(t, "-$5.00", FormatCents(-500))
}

func TestParseCentsRejectsNegative(t *testing.T) {
	// Leading-negative inputs return ErrNegativeMoney specifically.
	for _, input := range []string{"-$5.00", "-5.00", "-$1,234.56"} {
		_, err := ParseRequiredCents(input)
		assert.ErrorIs(t, err, ErrNegativeMoney, "input=%q", input)
	}
	// Malformed negatives (sign in wrong position, bare dash) return ErrInvalidMoney.
	for _, input := range []string{"$-100", "--$5", "-", "-$"} {
		_, err := ParseRequiredCents(input)
		assert.Error(t, err, "input=%q should be rejected", input)
	}
}

func TestParseCentsFormatRoundtrip(t *testing.T) {
	// FormatCents output should parse back to the same value.
	values := []int64{0, 1, 99, 100, 123456}
	for _, cents := range values {
		formatted := FormatCents(cents)
		parsed, err := ParseRequiredCents(formatted)
		require.NoError(t, err, "roundtrip failed for %d (formatted=%q)", cents, formatted)
		assert.Equal(t, cents, parsed, "roundtrip mismatch for %d (formatted=%q)", cents, formatted)
	}
}

func TestFormatCentsZero(t *testing.T) {
	assert.Equal(t, "$0.00", FormatCents(0))
}

func TestParseRequiredDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2025-06-11", "2025-06-11"},
		{" 2025-06-11 ", "2025-06-11"},
	}
	for _, tt := range tests {
		got, err := ParseRequiredDate(tt.input)
		require.NoError(t, err, "input=%q", tt.input)
		assert.Equal(t, tt.want, got.Format(DateLayout), "input=%q", tt.input)
	}
}

func TestParseRequiredDateInvalid(t *testing.T) {
	for _, input := range []string{"", "2025-13-01"} {
		_, err := ParseRequiredDate(input)
		assert.Error(t, err, "input=%q", input)
	}
}

func TestParseRequiredDateAtNaturalLanguage(t *testing.T) {
	ref := time.Date(2026, 2, 25, 14, 30, 0, 0, time.UTC)
	tests := []struct {
		input string
		want  string
	}{
		// strict format still works
		{"2025-06-11", "2025-06-11"},
		{" 2025-06-11 ", "2025-06-11"},
		// natural language expressions
		{"today", "2026-02-25"},
		{"yesterday", "2026-02-24"},
		{"2 weeks ago", "2026-02-11"},
		{"last friday", "2026-02-20"},
	}
	for _, tt := range tests {
		got, err := ParseRequiredDateAt(tt.input, ref)
		require.NoError(t, err, "input=%q", tt.input)
		assert.Equal(t, tt.want, got.Format(DateLayout), "input=%q", tt.input)
	}
}

func TestParseRequiredDateAtTruncatesTime(t *testing.T) {
	ref := time.Date(2026, 2, 25, 14, 30, 45, 123, time.UTC)
	got, err := ParseRequiredDateAt("today", ref)
	require.NoError(t, err)
	assert.Equal(t, 0, got.Hour(), "hour should be zero")
	assert.Equal(t, 0, got.Minute(), "minute should be zero")
	assert.Equal(t, 0, got.Second(), "second should be zero")
	assert.Equal(t, 0, got.Nanosecond(), "nanosecond should be zero")
}

func TestParseRequiredDateAtInvalid(t *testing.T) {
	ref := time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC)
	for _, input := range []string{""} {
		_, err := ParseRequiredDateAt(input, ref)
		assert.Error(t, err, "input=%q", input)
	}
}

func TestParseOptionalDateAtNaturalLanguage(t *testing.T) {
	ref := time.Date(2026, 2, 25, 14, 30, 0, 0, time.UTC)
	tests := []struct {
		input string
		want  string
	}{
		{"today", "2026-02-25"},
		{"yesterday", "2026-02-24"},
	}
	for _, tt := range tests {
		got, err := ParseOptionalDateAt(tt.input, ref)
		require.NoError(t, err, "input=%q", tt.input)
		require.NotNil(t, got, "input=%q", tt.input)
		assert.Equal(t, tt.want, got.Format(DateLayout), "input=%q", tt.input)
	}
}

func TestParseOptionalDateAtEmpty(t *testing.T) {
	ref := time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC)
	got, err := ParseOptionalDateAt("", ref)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestFormatDate(t *testing.T) {
	assert.Empty(t, FormatDate(nil))
	d := time.Date(2025, 6, 11, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "2025-06-11", FormatDate(&d))
}

func TestParseRequiredInt(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"42", 42},
		{" 7 ", 7},
		{"0", 0},
	}
	for _, tt := range tests {
		got, err := ParseRequiredInt(tt.input)
		require.NoError(t, err, "input=%q", tt.input)
		assert.Equal(t, tt.want, got, "input=%q", tt.input)
	}
}

func TestParseRequiredIntInvalid(t *testing.T) {
	for _, input := range []string{"", "abc", "-5", "1.5"} {
		_, err := ParseRequiredInt(input)
		assert.Error(t, err, "input=%q", input)
	}
}

func TestParseRequiredFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"2.5", 2.5},
		{" 0 ", 0},
		{"100", 100},
	}
	for _, tt := range tests {
		got, err := ParseRequiredFloat(tt.input)
		require.NoError(t, err, "input=%q", tt.input)
		assert.Equal(t, tt.want, got, "input=%q", tt.input)
	}
}

func TestParseRequiredFloatInvalid(t *testing.T) {
	for _, input := range []string{"", "abc", "-1.5"} {
		_, err := ParseRequiredFloat(input)
		assert.Error(t, err, "input=%q", input)
	}
}

func TestParseOptionalIntEmpty(t *testing.T) {
	got, err := ParseOptionalInt("")
	require.NoError(t, err)
	assert.Zero(t, got)
}

func TestParseOptionalFloatEmpty(t *testing.T) {
	got, err := ParseOptionalFloat("")
	require.NoError(t, err)
	assert.Zero(t, got)
}

func TestParseOptionalDateEmpty(t *testing.T) {
	got, err := ParseOptionalDate("")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestParseOptionalCentsInvalid(t *testing.T) {
	_, err := ParseOptionalCents("abc")
	assert.Error(t, err)
}

func TestComputeNextDue(t *testing.T) {
	last := time.Date(2024, 10, 10, 0, 0, 0, 0, time.UTC)
	next := ComputeNextDue(&last, 6, nil)
	require.NotNil(t, next)
	assert.Equal(t, "2025-04-10", next.Format(DateLayout))
}

func TestComputeNextDueNilDate(t *testing.T) {
	assert.Nil(t, ComputeNextDue(nil, 6, nil))
}

func TestComputeNextDueZeroInterval(t *testing.T) {
	d := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Nil(t, ComputeNextDue(&d, 0, nil))
}

func TestComputeNextDueExplicitDueDate(t *testing.T) {
	due := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
	next := ComputeNextDue(nil, 0, &due)
	require.NotNil(t, next)
	assert.Equal(t, "2025-11-01", next.Format(DateLayout))
}

func TestComputeNextDueDateOverridesInterval(t *testing.T) {
	last := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	due := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	next := ComputeNextDue(&last, 6, &due)
	require.NotNil(t, next)
	assert.Equal(t, "2025-03-15", next.Format(DateLayout))
}

func TestComputeNextDueNeitherSet(t *testing.T) {
	assert.Nil(t, ComputeNextDue(nil, 0, nil))
}

func TestFormatCompactCents(t *testing.T) {
	tests := []struct {
		name  string
		cents int64
		want  string
	}{
		{"zero", 0, "$0.00"},
		{"small", 999, "$9.99"},
		{"hundred", 10000, "$100.00"},
		{"just under 1k", 99999, "$999.99"},
		{"exactly 1k", 100000, "$1k"},
		{"1.2k", 123456, "$1.2k"},
		{"round thousands", 4500000, "$45k"},
		{"thousands with decimal", 5234023, "$52.3k"},
		{"exactly 1M", 100000000, "$1M"},
		{"1.3M", 130000000, "$1.3M"},
		{"round millions", 200000000, "$2M"},
		{"negative small", -500, "-$5.00"},
		{"negative thousands", -250000, "-$2.5k"},
		{"negative millions", -100000000, "-$1M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatCompactCents(tt.cents))
		})
	}
}

func TestFormatCompactOptionalCents(t *testing.T) {
	assert.Empty(t, FormatCompactOptionalCents(nil))
	cents := int64(250000)
	assert.Equal(t, "$2.5k", FormatCompactOptionalCents(&cents))
}

// Overflow and edge case tests added during code audit.

func TestParseCentsOverflow(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"one dollar over", "$92233720368547759.00"},
		{"way over", "$999999999999999999999.99"},
		// wholePart == maxDollars but frac pushes past MaxInt64
		{"frac overflow at boundary", "$92233720368547758.08"},
		{"frac overflow .99 at boundary", "$92233720368547758.99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRequiredCents(tt.input)
			assert.Error(t, err, "should reject overflow: %s", tt.input)
		})
	}
}

func TestParseCentsAtMaxSafeValue(t *testing.T) {
	// Max safe value: 92233720368547758 dollars = 9223372036854775800 cents
	// This is just under int64 max (9223372036854775807) when multiplied by 100.
	cents, err := ParseRequiredCents("$92233720368547758.00")
	require.NoError(t, err)
	assert.Equal(t, int64(9223372036854775800), cents)

	// With cents, we can go up to .07 (max is 9223372036854775807)
	cents, err = ParseRequiredCents("$92233720368547758.07")
	require.NoError(t, err)
	assert.Equal(t, int64(9223372036854775807), cents)
}

func TestFormatCentsMinInt64(t *testing.T) {
	// math.MinInt64 cannot be negated without overflow.
	// We handle this by treating it as MaxInt64 for display.
	formatted := FormatCents(math.MinInt64)
	// Should not panic and should produce a reasonable (if slightly off) result
	assert.Contains(t, formatted, "-$")
	assert.Contains(t, formatted, "92,233,720,368,547,758.07")
}

func TestFormatCompactCentsMinInt64(t *testing.T) {
	formatted := FormatCompactCents(math.MinInt64)
	// Should not panic
	assert.Contains(t, formatted, "-$")
}

func TestAddMonths(t *testing.T) {
	tests := []struct {
		name   string
		start  time.Time
		months int
		want   string
	}{
		{
			"Jan 31 + 1 month = Feb 28 (non-leap year)",
			time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC), 1,
			"2025-02-28",
		},
		{
			"Jan 31 + 1 month = Feb 29 (leap year)",
			time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), 1,
			"2024-02-29",
		},
		{
			"Mar 31 + 1 month = Apr 30",
			time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC), 1,
			"2025-04-30",
		},
		{
			"normal case: Jan 15 + 1 month = Feb 15",
			time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), 1,
			"2025-02-15",
		},
		{
			"multiple months: Jan 31 + 3 months = Apr 30",
			time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC), 3,
			"2025-04-30",
		},
		{
			"year wrap: Nov 30 + 3 months = Feb 28",
			time.Date(2024, 11, 30, 0, 0, 0, 0, time.UTC), 3,
			"2025-02-28",
		},
		{
			"Feb 29 (leap) + 12 months = Feb 28 (non-leap)",
			time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), 12,
			"2025-02-28",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddMonths(tt.start, tt.months)
			assert.Equal(t, tt.want, got.Format(DateLayout))
		})
	}
}

func TestParseIntervalMonths(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		// bare integers
		{"12", 12},
		{"0", 0},
		{"  7  ", 7},
		// month suffix
		{"6m", 6},
		{"6M", 6},
		{" 3m ", 3},
		// year suffix
		{"1y", 12},
		{"2Y", 24},
		{" 1y ", 12},
		// combined
		{"2y 6m", 30},
		{"1y6m", 18},
		{"1Y 3M", 15},
		{"  2y  6m  ", 30},
		// empty
		{"", 0},
		{"   ", 0},
	}
	for _, tt := range tests {
		got, err := ParseIntervalMonths(tt.input)
		require.NoError(t, err, "input=%q", tt.input)
		assert.Equal(t, tt.want, got, "input=%q", tt.input)
	}
}

func TestParseIntervalMonthsInvalid(t *testing.T) {
	for _, input := range []string{"abc", "-1", "1.5m", "1x", "m", "y", "6m 1y"} {
		_, err := ParseIntervalMonths(input)
		assert.Error(t, err, "input=%q should be rejected", input)
	}
}

func TestComputeNextDueMonthEndClamping(t *testing.T) {
	// User scenario: maintenance item serviced Jan 31, interval 1 month.
	// Next due should be Feb 28, not March 3 (the time.AddDate gotcha).
	last := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
	next := ComputeNextDue(&last, 1, nil)
	require.NotNil(t, next)
	assert.Equal(t, "2025-02-28", next.Format(DateLayout))
}
