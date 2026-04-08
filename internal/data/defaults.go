// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"reflect"
	"strconv"
	"time"
)

// ApplyDefaults sets zero-valued fields on the struct pointed to by v
// to the values specified in their `default` struct tags. Fields that
// are already non-zero are left untouched. Nested structs without a
// default tag are recursed into automatically.
//
// Supported field types and tag values:
//   - string: literal value; "today" is replaced with time.Now() formatted as DateLayout
//   - int, int64, uint, uint64, float64: parsed via strconv
//   - time.Time: "now" is replaced with time.Now()
//   - Named types with int/uint underlying kind: parsed as the underlying integer
//   - Nested structs: recursed into (no tag needed on the struct field itself)
func ApplyDefaults(v any) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return
	}

	applyDefaultsValue(rv.Elem())
}

func applyDefaultsValue(rv reflect.Value) {
	rt := rv.Type()

	for i := range rt.NumField() {
		field := rv.Field(i)
		if !field.CanSet() {
			continue
		}

		sf := rt.Field(i)
		tag := sf.Tag.Get("default")

		// Recurse into nested structs that don't have a default tag
		// and aren't time.Time (handled as a leaf).
		if tag == "" && field.Kind() == reflect.Struct &&
			field.Type() != reflect.TypeFor[time.Time]() {
			applyDefaultsValue(field)
			continue
		}

		if tag == "" {
			continue
		}

		if !field.IsZero() {
			continue
		}

		setFieldDefault(field, tag)
	}
}

// StructDefault returns the default tag value for the named field on
// the given struct (or pointer-to-struct). Returns "" if the field
// does not exist or has no default tag.
func StructDefault[T any](fieldName string) string {
	var zero T
	rt := reflect.TypeOf(zero)
	if rt == nil {
		return ""
	}
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return ""
	}

	f, ok := rt.FieldByName(fieldName)
	if !ok {
		return ""
	}

	return f.Tag.Get("default")
}

func setFieldDefault(field reflect.Value, tag string) {
	//exhaustive:ignore // only handle types used in model/config defaults
	switch field.Kind() {
	case reflect.String:
		if tag == "today" {
			field.SetString(time.Now().Format(DateLayout))
		} else {
			field.SetString(tag)
		}

	case reflect.Int, reflect.Int64:
		if n, err := strconv.ParseInt(tag, 10, 64); err == nil {
			field.SetInt(n)
		}

	case reflect.Uint, reflect.Uint64:
		if n, err := strconv.ParseUint(tag, 10, 64); err == nil {
			field.SetUint(n)
		}

	case reflect.Float64:
		if f, err := strconv.ParseFloat(tag, 64); err == nil {
			field.SetFloat(f)
		}

	case reflect.Struct:
		if field.Type() == reflect.TypeFor[time.Time]() && tag == "now" {
			field.Set(reflect.ValueOf(time.Now()))
		}
	}
}
