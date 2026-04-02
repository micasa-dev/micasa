// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeprecationsReturnsEntries(t *testing.T) {
	deps := Deprecations()
	require.NotEmpty(t, deps)

	var chatEffort, exEffort bool
	for _, d := range deps {
		switch d.OldPath {
		case "chat.llm.thinking":
			chatEffort = true
			assert.Equal(t, "chat.llm.effort", d.NewPath)
			assert.Empty(t, d.Transform)
		case "extraction.llm.thinking":
			exEffort = true
			assert.Equal(t, "extraction.llm.effort", d.NewPath)
			assert.Empty(t, d.Transform)
		}
	}
	assert.True(t, chatEffort, "expected chat.llm.thinking deprecation")
	assert.True(t, exEffort, "expected extraction.llm.thinking deprecation")
}

func TestDeprecationsIncludesCacheTTLDays(t *testing.T) {
	deps := Deprecations()
	var found bool
	for _, d := range deps {
		if d.OldPath == "documents.cache_ttl_days" {
			found = true
			assert.Equal(t, "documents.cache_ttl", d.NewPath)
			assert.Equal(t, "days_to_duration", d.Transform)
		}
	}
	assert.True(t, found, "expected documents.cache_ttl_days deprecation")
}

func TestRemovedKeysMapBuiltFromTags(t *testing.T) {
	require.NotEmpty(t, derived.removedKeys)

	msg, ok := derived.removedKeys["chat.llm.thinking"]
	require.True(t, ok)
	assert.Contains(t, msg, "chat.llm.effort")

	msg, ok = derived.removedKeys["documents.cache_ttl_days"]
	require.True(t, ok)
	assert.Contains(t, msg, "documents.cache_ttl")
	assert.Contains(t, msg, "integer days become duration strings")
}

func TestDeprecatedEnvVarsMapBuiltFromTags(t *testing.T) {
	require.NotEmpty(t, derived.envVars)

	d, ok := derived.envVars["MICASA_CHAT_LLM_THINKING"]
	require.True(t, ok)
	assert.Equal(t, "chat.llm.effort", d.NewPath)

	d, ok = derived.envVars["MICASA_EXTRACTION_LLM_THINKING"]
	require.True(t, ok)
	assert.Equal(t, "extraction.llm.effort", d.NewPath)

	d, ok = derived.envVars["MICASA_DOCUMENTS_CACHE_TTL_DAYS"]
	require.True(t, ok)
	assert.Equal(t, "documents.cache_ttl", d.NewPath)
	assert.Equal(t, "days_to_duration", d.Transform)
}

func TestDeprecationsOrderIsDeterministic(t *testing.T) {
	d1 := Deprecations()
	d2 := Deprecations()
	require.Equal(t, d1, d2, "Deprecations() must return deterministic order")
}

func TestHintText(t *testing.T) {
	t.Run("with transform", func(t *testing.T) {
		d := Deprecation{Transform: "days_to_duration"}
		assert.Equal(t, "integer days become duration strings, e.g. 30 becomes 30d", d.HintText())
	})
	t.Run("without transform", func(t *testing.T) {
		d := Deprecation{}
		assert.Empty(t, d.HintText())
	})
}

func TestWalkDeprecatedTagsPanicsOnDuplicateOldPath(t *testing.T) {
	type dupSection struct {
		A string `toml:"a" deprecated:"old"`
		B string `toml:"b" deprecated:"old"`
	}
	assert.Panics(t, func() {
		var deps []Deprecation
		seen := make(map[string]bool)
		walkDeprecatedTags(reflect.TypeFor[dupSection](), "test", &deps, seen)
	}, "duplicate old path should panic")
}

func TestWalkDeprecatedTagsPanicsOnUnknownTransform(t *testing.T) {
	type badTransform struct {
		A string `toml:"a" deprecated:"old" deprecated_transform:"nonexistent"`
	}
	assert.Panics(t, func() {
		var deps []Deprecation
		seen := make(map[string]bool)
		walkDeprecatedTags(reflect.TypeFor[badTransform](), "test", &deps, seen)
	}, "unregistered transform should panic")
}

func TestBuildDeprecatedEnvVarsPanicsOnCanonicalCollision(t *testing.T) {
	deps := []Deprecation{{OldPath: "chat.llm.model", NewPath: "chat.llm.new_model"}}
	canonical := EnvVars()
	assert.Panics(t, func() {
		buildDeprecatedEnvVars(deps, canonical)
	}, "deprecated env var colliding with canonical should panic")
}

func TestBuildDeprecatedEnvVarsPanicsOnEnvVarCollision(t *testing.T) {
	deps := []Deprecation{
		{OldPath: "a.b_c", NewPath: "a.new1"},
		{OldPath: "a_b.c", NewPath: "a.new2"},
	}
	assert.Panics(t, func() {
		buildDeprecatedEnvVars(deps, map[string]string{})
	}, "distinct paths colliding to same env var should panic")
}
