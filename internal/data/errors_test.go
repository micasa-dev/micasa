// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithHintNil(t *testing.T) {
	assert.Nil(t, WithHint(nil, "should not wrap"))
}

func TestWithHintPreservesChain(t *testing.T) {
	sentinel := errors.New("base")
	wrapped := fmt.Errorf("layer: %w", sentinel)
	hinted := WithHint(wrapped, "user-friendly message")

	require.NotNil(t, hinted)
	assert.Equal(t, "user-friendly message", hinted.Error())
	assert.True(t, errors.Is(hinted, sentinel), "sentinel should be reachable via errors.Is")
	assert.True(t, errors.Is(hinted, wrapped), "intermediate wrap should be reachable")
}

func TestHintExtraction(t *testing.T) {
	err := WithHint(errors.New("raw"), "friendly")
	assert.Equal(t, "friendly", Hint(err))
}

func TestHintExtractionNoHint(t *testing.T) {
	assert.Equal(t, "", Hint(errors.New("no hint")))
	assert.Equal(t, "", Hint(nil))
}

func TestHintNestedExtraction(t *testing.T) {
	inner := WithHint(errors.New("raw"), "inner hint")
	outer := fmt.Errorf("wrap: %w", inner)
	assert.Equal(t, "inner hint", Hint(outer))
}

func TestFieldErrorMappings(t *testing.T) {
	tests := []struct {
		label   string
		err     error
		wantMsg string
		wantIs  error
	}{
		{
			"Budget",
			locale.ErrInvalidMoney,
			"Budget should look like 1250.00",
			locale.ErrInvalidMoney,
		},
		{
			"Budget",
			locale.ErrNegativeMoney,
			"Budget must be a positive amount",
			locale.ErrNegativeMoney,
		},
		{
			"Start Date", ErrInvalidDate,
			"Start Date should be YYYY-MM-DD or a relative date like 'yesterday'", ErrInvalidDate,
		},
		{"Year Built", ErrInvalidInt, "Year Built should be a whole number", ErrInvalidInt},
		{"Bathrooms", ErrInvalidFloat, "Bathrooms should be a number like 2.5", ErrInvalidFloat},
		{
			"Interval", ErrInvalidInterval,
			"Interval should be months (6), or a duration like 6m, 1y, 2y 6m", ErrInvalidInterval,
		},
		{"Schedule", ErrIntervalAndDueDate, ErrIntervalAndDueDate.Error(), ErrIntervalAndDueDate},
	}
	for _, tt := range tests {
		t.Run(tt.label+"_"+tt.err.Error(), func(t *testing.T) {
			result := FieldError(tt.label, tt.err)
			assert.Equal(t, tt.wantMsg, result.Error())
			assert.True(t, errors.Is(result, tt.wantIs),
				"errors.Is should match sentinel %v", tt.wantIs)
		})
	}
}

func TestFieldErrorUnknownSentinel(t *testing.T) {
	custom := errors.New("something unusual")
	result := FieldError("Field", custom)
	assert.Equal(t, "Field: something unusual", result.Error())
	assert.True(t, errors.Is(result, custom))
}
