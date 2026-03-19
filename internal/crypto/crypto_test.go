// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package crypto

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Encryption round-trip ---

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()
	key, err := GenerateHouseholdKey()
	require.NoError(t, err)

	plaintext := []byte("hello world, this is a sync operation payload")
	sealed, err := Encrypt(key, plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, sealed)

	decrypted, err := Decrypt(key, sealed)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptDecryptEmptyPlaintext(t *testing.T) {
	t.Parallel()
	key, err := GenerateHouseholdKey()
	require.NoError(t, err)

	sealed, err := Encrypt(key, []byte{})
	require.NoError(t, err)

	decrypted, err := Decrypt(key, sealed)
	require.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestEncryptDecryptLargePayload(t *testing.T) {
	t.Parallel()
	key, err := GenerateHouseholdKey()
	require.NoError(t, err)

	plaintext := make([]byte, 1<<20) // 1 MiB
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}
	sealed, err := Encrypt(key, plaintext)
	require.NoError(t, err)

	decrypted, err := Decrypt(key, sealed)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestDecryptWrongKeyFails(t *testing.T) {
	t.Parallel()
	key1, err := GenerateHouseholdKey()
	require.NoError(t, err)
	key2, err := GenerateHouseholdKey()
	require.NoError(t, err)

	sealed, err := Encrypt(key1, []byte("secret data"))
	require.NoError(t, err)

	_, err = Decrypt(key2, sealed)
	assert.Error(t, err, "decrypting with wrong key should fail")
}

func TestDecryptTamperedCiphertextFails(t *testing.T) {
	t.Parallel()
	key, err := GenerateHouseholdKey()
	require.NoError(t, err)

	sealed, err := Encrypt(key, []byte("original"))
	require.NoError(t, err)

	// Flip a byte in the ciphertext (after the nonce).
	tampered := make([]byte, len(sealed))
	copy(tampered, sealed)
	tampered[len(tampered)-1] ^= 0xFF

	_, err = Decrypt(key, tampered)
	assert.Error(t, err, "decrypting tampered ciphertext should fail")
}

func TestDecryptTruncatedFails(t *testing.T) {
	t.Parallel()
	key, err := GenerateHouseholdKey()
	require.NoError(t, err)

	// Too short to contain a nonce.
	_, err = Decrypt(key, []byte("short"))
	require.Error(t, err)

	// Exactly nonce length, no ciphertext.
	_, err = Decrypt(key, make([]byte, NonceSize))
	require.Error(t, err)
}

func TestEncryptProducesUniqueNonces(t *testing.T) {
	t.Parallel()
	key, err := GenerateHouseholdKey()
	require.NoError(t, err)

	sealed1, err := Encrypt(key, []byte("same plaintext"))
	require.NoError(t, err)
	sealed2, err := Encrypt(key, []byte("same plaintext"))
	require.NoError(t, err)

	// Different nonces mean different ciphertexts.
	assert.NotEqual(t, sealed1, sealed2)
}

// --- Key generation ---

func TestGenerateHouseholdKeyUnique(t *testing.T) {
	t.Parallel()
	key1, err := GenerateHouseholdKey()
	require.NoError(t, err)
	key2, err := GenerateHouseholdKey()
	require.NoError(t, err)

	assert.NotEqual(t, key1, key2, "each generated key should be unique")
}

func TestGenerateDeviceKeyPairUnique(t *testing.T) {
	t.Parallel()
	kp1, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	kp2, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	assert.NotEqual(t, kp1.PublicKey, kp2.PublicKey)
	assert.NotEqual(t, kp1.PrivateKey, kp2.PrivateKey)
}

func TestDeviceKeyPairPublicPrivateDiffer(t *testing.T) {
	t.Parallel()
	kp, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	assert.NotEqual(t, kp.PublicKey, kp.PrivateKey)
}

// --- Key persistence ---

func TestHouseholdKeySaveLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	key, err := GenerateHouseholdKey()
	require.NoError(t, err)

	require.NoError(t, SaveHouseholdKey(dir, key))

	loaded, err := LoadHouseholdKey(dir)
	require.NoError(t, err)
	assert.Equal(t, key, loaded)
}

func TestDeviceKeyPairSaveLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	kp, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	require.NoError(t, SaveDeviceKeyPair(dir, kp))

	loaded, err := LoadDeviceKeyPair(dir)
	require.NoError(t, err)
	assert.Equal(t, kp, loaded)
}

func TestKeyFilePermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	key, err := GenerateHouseholdKey()
	require.NoError(t, err)
	require.NoError(t, SaveHouseholdKey(dir, key))

	kp, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	require.NoError(t, SaveDeviceKeyPair(dir, kp))

	if runtime.GOOS == "windows" {
		t.Skip("NTFS does not support Unix file permissions")
	}
	for _, name := range []string{HouseholdKeyFile, DevicePrivateKeyFile} {
		info, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
			"%s should have 0600 permissions", name)
	}

	pubInfo, err := os.Stat(filepath.Join(dir, DevicePublicKeyFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), pubInfo.Mode().Perm(),
		"%s should have 0644 permissions", DevicePublicKeyFile)
}

func TestLoadHouseholdKeyNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := LoadHouseholdKey(dir)
	assert.Error(t, err)
}

func TestLoadDeviceKeyPairNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := LoadDeviceKeyPair(dir)
	assert.Error(t, err)
}

func TestHouseholdKeyFileTruncated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, HouseholdKeyFile),
		[]byte("too short"),
		0o600,
	))

	_, err := LoadHouseholdKey(dir)
	assert.Error(t, err, "loading truncated key file should fail")
}

func TestLoadDeviceKeyPairRejectsMismatchedKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Generate two keypairs and cross-pollinate them.
	kp1, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	kp2, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	// Write kp1's private key but kp2's public key.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, DevicePrivateKeyFile), kp1.PrivateKey[:], 0o600,
	))
	require.NoError(t, os.WriteFile( //nolint:gosec // test file, 0644 is intentional for public key
		filepath.Join(dir, DevicePublicKeyFile), kp2.PublicKey[:], 0o644,
	))

	_, err = LoadDeviceKeyPair(dir)
	require.Error(t, err, "mismatched pub/priv keys should fail validation")
	assert.Contains(t, err.Error(), "does not match")
}

func TestHouseholdKeyStringer(t *testing.T) {
	t.Parallel()
	key, err := GenerateHouseholdKey()
	require.NoError(t, err)
	assert.Equal(t, "[REDACTED]", key.String())
}

func TestDeviceKeyPairStringer(t *testing.T) {
	t.Parallel()
	kp, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	assert.Equal(t, "[REDACTED]", kp.String())
}

func TestHouseholdKeyFileOversized(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, HouseholdKeyFile),
		make([]byte, 64),
		0o600,
	))

	_, err := LoadHouseholdKey(dir)
	assert.Error(t, err, "oversized key file should fail")
}

// --- atomicWriteFile error paths ---

func TestAtomicWriteFileNonExistentDirectory(t *testing.T) {
	t.Parallel()
	badPath := filepath.Join(t.TempDir(), "no", "such", "dir", "key.dat")
	err := atomicWriteFile(badPath, []byte("data"), 0o600)
	require.Error(t, err, "writing to a non-existent directory should fail")
	assert.Contains(t, err.Error(), "create temp file")
}

func TestSaveDeviceKeyPairBadDirectory(t *testing.T) {
	t.Parallel()
	kp, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	err = SaveDeviceKeyPair("/no/such/directory", kp)
	require.Error(t, err, "saving to a non-existent directory should fail")
	assert.Contains(t, err.Error(), "save device private key")
}

func TestSaveDeviceKeyPairPublicKeyWriteFailure(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("permission-based write prevention not reliable on Windows")
	}

	dir := t.TempDir()
	kp, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	// Write the private key first, then make the directory read-only to
	// prevent the public key write.
	require.NoError(t, atomicWriteFile(
		filepath.Join(dir, DevicePrivateKeyFile), kp.PrivateKey[:], 0o600,
	))
	require.NoError(t, os.Chmod(dir, 0o500))       //nolint:gosec // intentional read-only for test
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // restore perms

	err = SaveDeviceKeyPair(dir, kp)
	assert.Error(t, err, "should fail when directory is read-only")
}

func TestSaveHouseholdKeyBadDirectory(t *testing.T) {
	t.Parallel()
	key, err := GenerateHouseholdKey()
	require.NoError(t, err)
	err = SaveHouseholdKey("/no/such/directory", key)
	require.Error(t, err, "saving to a non-existent directory should fail")
}

// --- GenerateHouseholdKey / GenerateDeviceKeyPair uncovered error paths ---
//
// The remaining uncovered lines in GenerateHouseholdKey and
// GenerateDeviceKeyPair are crypto/rand.Read failures. These are untestable
// in normal conditions: crypto/rand reads from the OS entropy source
// (/dev/urandom or equivalent) and only fails if the OS itself is broken.
// Injecting a faulty reader would require changing the global rand.Reader,
// which is not safe in parallel tests and would not test real behavior.

// --- readBoundedFile ---

func TestReadBoundedFileExceedsMaxSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "toobig.key")
	require.NoError(t, os.WriteFile(bigFile, make([]byte, maxKeyFileSize+1), 0o600))

	_, err := readBoundedFile(bigFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestReadBoundedFileEmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	emptyFile := filepath.Join(dir, "empty.key")
	require.NoError(t, os.WriteFile(emptyFile, []byte{}, 0o600))

	data, err := readBoundedFile(emptyFile)
	require.NoError(t, err, "reading an empty file should not error")
	assert.Empty(t, data, "empty file should return empty data")
}

func TestReadBoundedFileNotFound(t *testing.T) {
	t.Parallel()
	_, err := readBoundedFile(filepath.Join(t.TempDir(), "missing.key"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open key file")
}

func TestReadBoundedFileExactMaxSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exactFile := filepath.Join(dir, "exact.key")
	require.NoError(t, os.WriteFile(exactFile, make([]byte, maxKeyFileSize), 0o600))

	data, err := readBoundedFile(exactFile)
	require.NoError(t, err, "file at exactly maxKeyFileSize should succeed")
	assert.Len(t, data, maxKeyFileSize)
}

// --- LoadDeviceKeyPair additional error paths ---

func TestLoadDeviceKeyPairPublicKeyWrongSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	kp, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	// Write valid private key, but truncated public key.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, DevicePrivateKeyFile), kp.PrivateKey[:], 0o600,
	))
	require.NoError(t, os.WriteFile( //nolint:gosec // test file, 0644 is intentional for public key
		filepath.Join(dir, DevicePublicKeyFile), []byte("short"), 0o644,
	))

	_, err = LoadDeviceKeyPair(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "device public key")
}

func TestLoadDeviceKeyPairPrivateKeyWrongSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, DevicePrivateKeyFile), []byte("short"), 0o600,
	))
	require.NoError(t, os.WriteFile( //nolint:gosec // test file, 0644 is intentional for public key
		filepath.Join(dir, DevicePublicKeyFile), make([]byte, KeySize), 0o644,
	))

	_, err := LoadDeviceKeyPair(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "device private key")
}

// --- SecretsDir ---

func TestSecretsDirDefault(t *testing.T) {
	t.Parallel()
	dir, err := SecretsDir()
	require.NoError(t, err)
	assert.Contains(t, dir, "micasa")
	assert.Contains(t, dir, "secrets")
}
