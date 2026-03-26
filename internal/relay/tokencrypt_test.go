// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testEncryptionKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)
	plaintext := "test-device-token-abc123"

	encrypted, err := encryptToken(key, plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, encrypted)

	decrypted, err := decryptToken(key, encrypted)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	t.Parallel()
	key1 := testEncryptionKey(t)
	key2 := testEncryptionKey(t)

	encrypted, err := encryptToken(key1, "secret-token")
	require.NoError(t, err)

	_, err = decryptToken(key2, encrypted)
	assert.Error(t, err)
}

func TestDecryptTamperedCiphertextFails(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)

	encrypted, err := encryptToken(key, "secret-token")
	require.NoError(t, err)

	data, err := base64.StdEncoding.DecodeString(encrypted)
	require.NoError(t, err)
	data[len(data)-1] ^= 0xff
	_, err = decryptToken(key, base64.StdEncoding.EncodeToString(data))
	assert.Error(t, err)
}

func TestDecryptTooShortCiphertext(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)

	short := base64.StdEncoding.EncodeToString([]byte("x"))
	_, err := decryptToken(key, short)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext too short")
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)

	enc1, err := encryptToken(key, "same-token")
	require.NoError(t, err)
	enc2, err := encryptToken(key, "same-token")
	require.NoError(t, err)

	assert.NotEqual(t, enc1, enc2, "different nonces should produce different ciphertexts")
}

func TestEncryptDecryptEmptyPlaintext(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)

	encrypted, err := encryptToken(key, "")
	require.NoError(t, err)
	assert.NotEmpty(t, encrypted)

	decrypted, err := decryptToken(key, encrypted)
	require.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestDecryptEmptyStringFails(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)

	_, err := decryptToken(key, "")
	assert.Error(t, err)
}

func TestDecryptInvalidBase64Fails(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)

	_, err := decryptToken(key, "not-valid-base64!!!")
	assert.Error(t, err)
}

func TestEncryptWithNilKeyFails(t *testing.T) {
	t.Parallel()

	_, err := encryptToken(nil, "token")
	assert.Error(t, err)
}

func TestDecryptWithNilKeyFails(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)

	encrypted, err := encryptToken(key, "token")
	require.NoError(t, err)

	_, err = decryptToken(nil, encrypted)
	assert.Error(t, err)
}

// Confidentiality: ciphertext must not contain the plaintext.
func TestEncryptedOutputDoesNotContainPlaintext(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)
	plaintext := "super-secret-device-token-12345"

	encrypted, err := encryptToken(key, plaintext)
	require.NoError(t, err)

	// Check both the base64 output and the decoded bytes.
	assert.NotContains(t, encrypted, plaintext)
	decoded, err := base64.StdEncoding.DecodeString(encrypted)
	require.NoError(t, err)
	assert.NotContains(t, string(decoded), plaintext)
}

// Integrity: flipping a single bit in the ciphertext must fail.
func TestDecryptSingleBitFlipFails(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)

	encrypted, err := encryptToken(key, "token-for-bitflip")
	require.NoError(t, err)

	data, err := base64.StdEncoding.DecodeString(encrypted)
	require.NoError(t, err)

	// Flip one bit in the middle of the ciphertext (past the nonce).
	mid := len(data)/2 + 6 // well into the ciphertext portion
	data[mid] ^= 0x01
	_, err = decryptToken(key, base64.StdEncoding.EncodeToString(data))
	assert.Error(t, err)
}

// Failed decryption must not return partial plaintext.
func TestDecryptFailureReturnsEmptyString(t *testing.T) {
	t.Parallel()
	key1 := testEncryptionKey(t)
	key2 := testEncryptionKey(t)

	encrypted, err := encryptToken(key1, "secret-data")
	require.NoError(t, err)

	result, err := decryptToken(key2, encrypted)
	require.Error(t, err)
	assert.Empty(t, result, "failed decryption must not leak partial plaintext")
}

// Various plaintext lengths to exercise padding/block boundaries.
func TestEncryptDecryptVariousLengths(t *testing.T) {
	t.Parallel()
	key := testEncryptionKey(t)

	for _, length := range []int{1, 15, 16, 17, 31, 32, 33, 64, 128, 256, 1024} {
		t.Run(fmt.Sprintf("len_%d", length), func(t *testing.T) {
			t.Parallel()
			buf := make([]byte, length)
			for i := range buf {
				buf[i] = 'a' + byte(i%26)
			}
			plaintext := string(buf)

			encrypted, err := encryptToken(key, plaintext)
			require.NoError(t, err)

			decrypted, err := decryptToken(key, encrypted)
			require.NoError(t, err)
			assert.Equal(t, plaintext, decrypted)
		})
	}
}
