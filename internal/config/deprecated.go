// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
)

//go:generate go run ./cmd/gendeprecations -output ../../docs/data/deprecations.json

// Deprecation describes a config key that was renamed.
type Deprecation struct {
	OldPath   string // e.g. "chat.llm.thinking"
	NewPath   string // e.g. "chat.llm.effort"
	Transform string // e.g. "days_to_duration" (empty = no transform)
}

// HintText returns the human-readable transform hint, or empty string
// if no transform is registered.
func (d Deprecation) HintText() string {
	if d.Transform == "" {
		return ""
	}
	return transformHints[d.Transform]
}

// transformHints maps transform names to human-readable hint strings
// appended to error messages for value-changing renames.
var transformHints = map[string]string{
	"days_to_duration": "integer days become duration strings, e.g. 30 becomes 30d",
}

// Deprecations walks the Config struct via reflection and returns all
// deprecation entries derived from `deprecated` struct tags. Entries are
// returned in struct-field reflection order (depth-first walk), which is
// deterministic. Panics on duplicate OldPath entries or unregistered
// deprecated_transform names.
func Deprecations() []Deprecation {
	var deps []Deprecation
	seen := make(map[string]bool)
	walkDeprecatedTags(reflect.TypeFor[Config](), "", &deps, seen)
	return deps
}

func walkDeprecatedTags(t reflect.Type, prefix string, deps *[]Deprecation, seen map[string]bool) {
	for f := range t.Fields() {
		tomlName := tomlTagName(f)
		if tomlName == "" {
			continue
		}

		path := tomlName
		if prefix != "" {
			path = prefix + "." + tomlName
		}

		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		if isConfigSection(ft) {
			walkDeprecatedTags(ft, path, deps, seen)
			continue
		}

		oldKey := f.Tag.Get("deprecated")
		if oldKey == "" {
			continue
		}

		oldPath := oldKey
		if prefix != "" {
			oldPath = prefix + "." + oldKey
		}

		if seen[oldPath] {
			panic(fmt.Sprintf("duplicate deprecated old path: %q", oldPath))
		}
		seen[oldPath] = true

		transform := f.Tag.Get("deprecated_transform")
		if transform != "" {
			if _, ok := transformHints[transform]; !ok {
				panic(fmt.Sprintf(
					"unregistered deprecated_transform %q on %s",
					transform, path,
				))
			}
		}

		*deps = append(*deps, Deprecation{
			OldPath:   oldPath,
			NewPath:   path,
			Transform: transform,
		})
	}
}

// derivedDeprecations holds maps derived from struct tags at package
// load time. Computed once via package-level var initialization.
type derivedDeprecations struct {
	// removedKeys maps old TOML path -> error message.
	removedKeys map[string]string
	// envVars maps old env var name -> Deprecation metadata.
	envVars map[string]Deprecation
	// envVarOrder preserves Deprecations() slice order for deterministic
	// iteration in checkDeprecatedEnvVars.
	envVarOrder []string
}

var derived = buildDerived()

func buildDerived() derivedDeprecations {
	deps := Deprecations()

	removed := make(map[string]string, len(deps))
	for _, d := range deps {
		msg := d.OldPath + " was removed -- use " + d.NewPath + " instead"
		if d.Transform != "" {
			msg += " (" + transformHints[d.Transform] + ")"
		}
		removed[d.OldPath] = msg
	}

	envVars, order := buildDeprecatedEnvVars(deps, EnvVars())
	return derivedDeprecations{
		removedKeys: removed,
		envVars:     envVars,
		envVarOrder: order,
	}
}

// buildDeprecatedEnvVars constructs the deprecated env var map and
// ordered key slice. Panics on duplicate env var names or collisions
// with canonical env vars. Extracted from init() for testability.
func buildDeprecatedEnvVars(deps []Deprecation, canonical map[string]string) (
	map[string]Deprecation, []string,
) {
	envVars := make(map[string]Deprecation, len(deps))
	var order []string
	seen := make(map[string]bool, len(deps))

	for _, d := range deps {
		oldEnv := EnvVarName(d.OldPath)
		if seen[oldEnv] {
			panic("duplicate deprecated env var: " + oldEnv)
		}
		if _, ok := canonical[oldEnv]; ok {
			panic(fmt.Sprintf(
				"deprecated env var %s collides with canonical env var",
				oldEnv,
			))
		}
		seen[oldEnv] = true
		envVars[oldEnv] = d
		order = append(order, oldEnv)
	}
	return envVars, order
}

// checkDeprecatedEnvVars scans deprecated env var names and returns a
// hard error if any non-empty deprecated env var is set. Iterates in
// Deprecations() slice order for deterministic error messages.
func checkDeprecatedEnvVars() error {
	for _, oldEnv := range derived.envVarOrder {
		if val := os.Getenv(oldEnv); val != "" {
			d := derived.envVars[oldEnv]
			newEnv := EnvVarName(d.NewPath)
			msg := fmt.Sprintf(
				"%s was removed -- use %s instead",
				oldEnv, newEnv,
			)
			if d.Transform != "" {
				msg += " (" + transformHints[d.Transform] + ")"
			}
			return errors.New(msg)
		}
	}
	return nil
}
