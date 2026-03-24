// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"errors"
	"fmt"

	"github.com/micasa-dev/micasa/internal/locale"
)

// hintError wraps an error with a user-facing hint message.
// The hint becomes the error's string representation so that
// setStatusError(err.Error()) displays actionable text.
// The original error is preserved for errors.Is / errors.As.
type hintError struct {
	hint string
	err  error
}

func (e *hintError) Error() string { return e.hint }
func (e *hintError) Unwrap() error { return e.err }

// WithHint wraps err with a user-facing hint message.
// Returns nil when err is nil.
func WithHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	return &hintError{hint: hint, err: err}
}

// Hint extracts the user-facing hint from an error chain.
// Returns "" if no hint is found.
func Hint(err error) string {
	var h *hintError
	if errors.As(err, &h) {
		return h.hint
	}
	return ""
}

// FieldError wraps a validation sentinel error with a field-specific,
// user-friendly message. The sentinel is preserved in the error chain.
func FieldError(label string, err error) error {
	switch {
	case errors.Is(err, locale.ErrNegativeMoney):
		return WithHint(err, fmt.Sprintf("%s must be a positive amount", label))
	case errors.Is(err, locale.ErrInvalidMoney):
		return WithHint(err, fmt.Sprintf("%s should look like 1250.00", label))
	case errors.Is(err, ErrInvalidDate):
		return WithHint(err, fmt.Sprintf(
			"%s should be YYYY-MM-DD or a relative date like 'yesterday'", label))
	case errors.Is(err, ErrInvalidInt):
		return WithHint(err, fmt.Sprintf("%s should be a whole number", label))
	case errors.Is(err, ErrInvalidFloat):
		return WithHint(err, fmt.Sprintf("%s should be a number like 2.5", label))
	case errors.Is(err, ErrInvalidInterval):
		return WithHint(err, fmt.Sprintf(
			"%s should be months (6), or a duration like 6m, 1y, 2y 6m", label))
	case errors.Is(err, ErrIntervalAndDueDate):
		return WithHint(err, err.Error())
	default:
		return WithHint(err, fmt.Sprintf("%s: %s", label, err.Error()))
	}
}
