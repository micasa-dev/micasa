// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package crypto

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeviceTokenSaveLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	token := "abc123hextoken"
	require.NoError(t, SaveDeviceToken(dir, token))

	loaded, err := LoadDeviceToken(dir)
	require.NoError(t, err)
	assert.Equal(t, token, loaded)
}

func TestDeviceTokenFilePermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	require.NoError(t, SaveDeviceToken(dir, "secret-token"))

	info, err := os.Stat(filepath.Join(dir, DeviceTokenFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"device token file should have 0600 permissions")
}

func TestLoadDeviceTokenNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := LoadDeviceToken(dir)
	assert.Error(t, err)
}

func TestLoadDeviceTokenEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, DeviceTokenFile),
		[]byte{},
		0o600,
	))

	_, err := LoadDeviceToken(dir)
	assert.Error(t, err, "loading empty token file should fail")
}

func TestSaveDeviceTokenEmptyRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := SaveDeviceToken(dir, "")
	assert.Error(t, err, "saving empty token should fail")
}

func TestDeviceTokenOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	require.NoError(t, SaveDeviceToken(dir, "first-token"))
	require.NoError(t, SaveDeviceToken(dir, "second-token"))

	loaded, err := LoadDeviceToken(dir)
	require.NoError(t, err)
	assert.Equal(t, "second-token", loaded)
}
