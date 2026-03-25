// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"

	"charm.land/huh/v2"
)

// inlineEditKind classifies how a column should be edited inline.
type inlineEditKind int

const (
	ieText   inlineEditKind = iota // openInlineInput with text validator
	ieMoney                        // openInlineInput with money validator
	ieDate                         // openDatePicker
	ieNotes                        // openNotesEdit
	ieSelect                       // openInlineEdit with a huh.Select
)

// inlineColSpec describes how a single column should be edited inline.
type inlineColSpec struct {
	kind        inlineEditKind
	title       string // display title for the input/select
	placeholder string // placeholder text (text/money inputs)

	// fieldPtr returns a pointer to the string field in the form values.
	fieldPtr func(formData) *string

	// validate returns a validator for text/money inputs. Nil means no
	// validation. Receives the Model so money validators can access currency.
	validate func(*Model) func(string) error

	// selectOptions builds the huh options for select columns. Receives the
	// Model so it can load dynamic data (appliances, vendors, etc.).
	selectOptions func(*Model) ([]huh.Option[string], error)

	// beforeEdit is an optional hook called before opening the editor. Used
	// for maintenance schedule columns that need to mutate form values.
	beforeEdit func(formData)
}

// mustAssert performs a checked type assertion, panicking on type mismatch.
// Used in inlineColSpec.fieldPtr closures where the spec map guarantees the
// correct concrete type, but the linter requires a checked assertion.
func mustAssert[T any](v any) T {
	t, ok := v.(T)
	if !ok {
		panic(fmt.Sprintf("inline edit: expected %T, got %T", *new(T), v))
	}
	return t
}

// dispatchInlineEdit looks up the column in the spec map and opens the
// appropriate inline editor. Returns true if the column was handled, false
// if the caller should fall through to the full edit form.
func (m *Model) dispatchInlineEdit(
	id string,
	col int,
	specs map[int]inlineColSpec,
	values formData,
) (bool, error) {
	spec, ok := specs[col]
	if !ok {
		return false, nil
	}

	if spec.beforeEdit != nil {
		spec.beforeEdit(values)
	}

	switch spec.kind {
	case ieText, ieMoney:
		var vfn func(string) error
		if spec.validate != nil {
			vfn = spec.validate(m)
		}
		m.openInlineInput(id, spec.title, spec.placeholder, spec.fieldPtr(values), vfn, values)

	case ieDate:
		m.openDatePicker(id, spec.fieldPtr(values), values)

	case ieNotes:
		m.openNotesEdit(id, spec.fieldPtr(values), values)

	case ieSelect:
		if spec.selectOptions == nil {
			return true, fmt.Errorf(
				"inline edit: ieSelect spec for col %d has nil selectOptions",
				col,
			)
		}
		opts, err := spec.selectOptions(m)
		if err != nil {
			return true, err
		}
		field := huh.NewSelect[string]().Title(spec.title).
			Options(opts...).
			Value(spec.fieldPtr(values))
		m.openInlineEdit(id, field, values)
	}

	return true, nil
}
