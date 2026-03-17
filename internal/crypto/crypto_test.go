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
	assert.Error(t, err)

	// Exactly nonce length, no ciphertext.
	_, err = Decrypt(key, make([]byte, NonceSize))
	assert.Error(t, err)
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

	for _, name := range []string{HouseholdKeyFile, DevicePrivateKeyFile} {
		info, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
			"%s should have 0600 permissions", name)
	}
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

// --- KeysDir ---

func TestKeysDirDefault(t *testing.T) {
	t.Parallel()
	dir, err := KeysDir()
	require.NoError(t, err)
	assert.Contains(t, dir, "micasa")
	assert.Contains(t, dir, "keys")
}
