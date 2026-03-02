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
	m := newTestModel()
	m.formData = &projectFormData{}
	v, err := formDataAs[projectFormData](m)
	require.NoError(t, err)
	assert.NotNil(t, v)
}

func TestFormDataAsWrongType(t *testing.T) {
	m := newTestModel()
	m.formData = &vendorFormData{}
	_, err := formDataAs[projectFormData](m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected form data")
}

func TestFormDataAsNilFormData(t *testing.T) {
	m := newTestModel()
	m.formData = nil
	_, err := formDataAs[projectFormData](m)
	require.Error(t, err)
}

func TestParseFormDataWrongType(t *testing.T) {
	m := newTestModel()
	wrong := &houseFormData{}

	m.formData = wrong
	_, err := m.parseProjectFormData()
	assert.Error(t, err, "parseProjectFormData")

	m.formData = wrong
	_, err = m.parseIncidentFormData()
	assert.Error(t, err, "parseIncidentFormData")

	m.formData = wrong
	_, err = m.parseApplianceFormData()
	assert.Error(t, err, "parseApplianceFormData")

	m.formData = wrong
	_, err = m.parseVendorFormData()
	assert.Error(t, err, "parseVendorFormData")

	m.formData = wrong
	_, _, err = m.parseServiceLogFormData()
	assert.Error(t, err, "parseServiceLogFormData")

	m.formData = wrong
	_, _, err = m.parseQuoteFormData()
	assert.Error(t, err, "parseQuoteFormData")

	m.formData = wrong
	_, err = m.parseMaintenanceFormData()
	assert.Error(t, err, "parseMaintenanceFormData")

	m.formData = &projectFormData{}
	err = m.submitHouseForm()
	assert.Error(t, err, "submitHouseForm")

	m.formData = wrong
	_, err = m.parseDocumentFormData()
	assert.Error(t, err, "parseDocumentFormData")
}

func TestOptionalFilePathExpandsTilde(t *testing.T) {
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
