// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// optionalFilePath is a test-only helper used by TestOptionalFilePathExpandsTilde
// to exercise data.ExpandHome inside a validator-shaped closure. Production
// code uses huh.FilePicker, which performs its own path/dir checks, so this
// helper is not wired up to any form.
func optionalFilePath() func(string) error {
	return func(input string) error {
		path := strings.TrimSpace(input)
		if path == "" {
			return nil
		}
		path = data.ExpandHome(path)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("file not found: %s", path)
		}
		if info.IsDir() {
			return errors.New("path is a directory, not a file")
		}
		return nil
	}
}

func TestFormDataAsSuccess(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.fs.formData = &projectFormData{}
	v, err := formDataAs[projectFormData](m)
	require.NoError(t, err)
	assert.NotNil(t, v)
}

func TestFormDataAsWrongType(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.fs.formData = &vendorFormData{}
	_, err := formDataAs[projectFormData](m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected form data")
}

func TestFormDataAsNilFormData(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.fs.formData = nil
	_, err := formDataAs[projectFormData](m)
	require.Error(t, err)
}

func TestParseFormDataWrongType(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	wrong := &houseFormData{}

	m.fs.formData = wrong
	_, err := m.parseProjectFormData()
	require.Error(t, err, "parseProjectFormData")

	m.fs.formData = wrong
	_, err = m.parseIncidentFormData()
	require.Error(t, err, "parseIncidentFormData")

	m.fs.formData = wrong
	_, err = m.parseApplianceFormData()
	require.Error(t, err, "parseApplianceFormData")

	m.fs.formData = wrong
	_, err = m.parseVendorFormData()
	require.Error(t, err, "parseVendorFormData")

	m.fs.formData = wrong
	_, _, err = m.parseServiceLogFormData()
	require.Error(t, err, "parseServiceLogFormData")

	m.fs.formData = wrong
	_, _, err = m.parseQuoteFormData()
	require.Error(t, err, "parseQuoteFormData")

	m.fs.formData = wrong
	_, err = m.parseMaintenanceFormData()
	require.Error(t, err, "parseMaintenanceFormData")

	m.fs.formData = &projectFormData{}
	err = m.submitHouseForm()
	require.Error(t, err, "submitHouseForm")

	m.fs.formData = wrong
	_, err = m.parseDocumentFormData()
	require.Error(t, err, "parseDocumentFormData")
}

func TestOptionalFilePathExpandsTilde(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// Create a temp file inside home to test with a real tilde path.
	tmp := filepath.Join(home, ".micasa-test-file")
	require.NoError(t, os.WriteFile(tmp, []byte("test"), 0o600))
	t.Cleanup(func() { _ = os.Remove(tmp) })

	validate := optionalFilePath()
	assert.NoError(t, validate("~/.micasa-test-file"))
	assert.NoError(t, validate(tmp))
	assert.NoError(t, validate(""))
	assert.Error(t, validate("~/nonexistent-file-abc123"))
}
