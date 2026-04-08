// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/go-playground/validator/v10"
)

// configValidator is the package-level validator instance, configured once
// at init with custom validators and TOML-based field naming.
var configValidator = newConfigValidator()

func newConfigValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())

	// Use TOML tag names in error namespaces and cross-field references
	// so error messages use the same dotted paths users see in config files.
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := fld.Tag.Get("toml")
		if name == "" || name == "-" {
			return fld.Name
		}
		if idx := strings.IndexByte(name, ','); idx >= 0 {
			name = name[:idx]
		}
		return name
	})

	// Extract Duration's underlying nanosecond count so numeric
	// validators (min=0, nonneg_duration) compare against an int64.
	v.RegisterCustomTypeFunc(func(field reflect.Value) any {
		if d, ok := field.Interface().(Duration); ok {
			return d.Nanoseconds()
		}
		return nil
	}, Duration{})

	mustRegister(v, "provider", func(fl validator.FieldLevel) bool {
		return validProvider(fl.Field().String())
	})

	mustRegister(v, "positive_duration", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		d, err := time.ParseDuration(s)
		return err == nil && d > 0
	})

	mustRegister(v, "nonneg_duration", func(fl validator.FieldLevel) bool {
		field := fl.Field()
		//exhaustive:ignore // only numeric kinds relevant
		switch field.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return field.Int() >= 0
		default:
			return false
		}
	})

	return v
}

func mustRegister(
	v *validator.Validate,
	tag string,
	fn validator.Func,
) {
	if err := v.RegisterValidation(tag, fn); err != nil {
		panic(fmt.Sprintf("register validator %q: %v", tag, err))
	}
}

// checkRemovedKeys inspects decoded TOML metadata for keys that have been
// removed and returns an error directing the user to the replacement.
// The removedKeys map is derived from struct tags in deprecated.go.
func checkRemovedKeys(md toml.MetaData) error {
	for _, key := range md.Undecoded() {
		path := key.String()
		if msg, ok := derived.removedKeys[path]; ok {
			return fmt.Errorf("%s", msg)
		}
	}
	return nil
}

// validate runs struct-tag-driven validation, checks file permissions,
// and returns the first validation error formatted for the user.
func (c *Config) validate(path string) error {
	if err := configValidator.Struct(c); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) && len(ve) > 0 {
			return formatFieldError(ve[0])
		}
		return fmt.Errorf("config validation: %w", err)
	}

	if err := c.validateClaudeCLI(); err != nil {
		return err
	}

	checkFilePermissions(c, path)
	return nil
}

// formatFieldError translates a validator.FieldError into a user-facing
// error message that matches the config's dotted-path conventions.
func formatFieldError(fe validator.FieldError) error {
	ns := strings.TrimPrefix(fe.Namespace(), "Config.")

	switch fe.Tag() {
	case "provider":
		return fmt.Errorf(
			"%s: unknown provider %q -- supported: %s",
			ns, fe.Value(), strings.Join(providerNames(), ", "),
		)

	case "oneof":
		return fmt.Errorf(
			"%s: invalid level %q -- supported: %s",
			ns, fe.Value(), strings.ReplaceAll(fe.Param(), " ", ", "),
		)

	case "positive_duration":
		s, _ := fe.Value().(string)
		if _, err := time.ParseDuration(s); err != nil {
			return fmt.Errorf(
				"%s: invalid duration %q -- use Go syntax like \"5m\" or \"10m\"",
				ns, s,
			)
		}
		return fmt.Errorf("%s must be positive, got %s", ns, s)

	case "required":
		return fmt.Errorf("%s must be positive", ns)

	case "min", "max":
		if strings.HasSuffix(ns, ".confidence_threshold") {
			return fmt.Errorf("%s must be 0-100, got %v", ns, fe.Value())
		}
		return fmt.Errorf("%s must be non-negative, got %v", ns, fe.Value())

	case "nonneg_duration":
		return fmt.Errorf("%s must be non-negative, got %v", ns, fe.Value())
	}

	return fmt.Errorf("%s: validation failed on '%s'", ns, fe.Tag())
}

const providerClaudeCLI = "claude-cli"

// validateClaudeCLI enforces constraints specific to the claude-cli provider.
func (c *Config) validateClaudeCLI() error {
	// claude-cli does not yet support chat (multi-turn transport unverified).
	if c.Chat.LLM.Provider == providerClaudeCLI {
		return errors.New(
			"claude-cli does not yet support chat; use it under [extraction.llm] only",
		)
	}

	// claude-cli requires an explicit model (not the provider-agnostic default).
	// Normalize whitespace so " model " doesn't pass validation but fail at runtime.
	if c.Extraction.LLM.Provider == providerClaudeCLI {
		c.Extraction.LLM.Model = strings.TrimSpace(c.Extraction.LLM.Model)
		if c.Extraction.LLM.Model == "" || c.Extraction.LLM.Model == DefaultModel {
			return fmt.Errorf(
				"claude-cli requires an explicit model (e.g. claude-sonnet-4-5-latest), got %q",
				c.Extraction.LLM.Model,
			)
		}
	}

	return nil
}
