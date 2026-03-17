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

	token := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	require.NoError(t, SaveDeviceToken(dir, token))

	loaded, err := LoadDeviceToken(dir)
	require.NoError(t, err)
	assert.Equal(t, token, loaded)
}

func TestDeviceTokenFilePermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	require.NoError(
		t,
		SaveDeviceToken(dir, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
	)

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

	first := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	second := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	require.NoError(t, SaveDeviceToken(dir, first))
	require.NoError(t, SaveDeviceToken(dir, second))

	loaded, err := LoadDeviceToken(dir)
	require.NoError(t, err)
	assert.Equal(t, second, loaded)
}

func TestLoadDeviceTokenRejectsNonHex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write a token that is not a valid 64-char hex string.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, DeviceTokenFile),
		[]byte("not-a-hex-token"),
		0o600,
	))

	_, err := LoadDeviceToken(dir)
	require.Error(t, err, "non-hex token should be rejected")
	assert.Contains(t, err.Error(), "invalid device token format")
}

func TestLoadDeviceTokenRejectsWrongLength(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write a valid hex string but wrong length (32 chars instead of 64).
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, DeviceTokenFile),
		[]byte("abcdef0123456789abcdef0123456789"),
		0o600,
	))

	_, err := LoadDeviceToken(dir)
	require.Error(t, err, "wrong-length hex token should be rejected")
	assert.Contains(t, err.Error(), "invalid device token format")
}

func TestLoadDeviceTokenAcceptsValid64CharHex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	validToken := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, DeviceTokenFile),
		[]byte(validToken),
		0o600,
	))

	loaded, err := LoadDeviceToken(dir)
	require.NoError(t, err)
	assert.Equal(t, validToken, loaded)
}
