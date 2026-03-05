// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
