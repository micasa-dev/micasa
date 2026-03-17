// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoxRoundTrip(t *testing.T) {
	t.Parallel()
	sender, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	recipient, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	message := []byte("household key material")
	sealed, err := BoxSeal(sender.PrivateKey, recipient.PublicKey, message)
	require.NoError(t, err)

	plaintext, err := BoxOpen(recipient.PrivateKey, sender.PublicKey, sealed)
	require.NoError(t, err)
	assert.Equal(t, message, plaintext)
}

func TestBoxWrongRecipientKey(t *testing.T) {
	t.Parallel()
	sender, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	recipient, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	wrong, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	sealed, err := BoxSeal(sender.PrivateKey, recipient.PublicKey, []byte("secret"))
	require.NoError(t, err)

	_, err = BoxOpen(wrong.PrivateKey, sender.PublicKey, sealed)
	assert.Error(t, err)
}

func TestBoxWrongSenderKey(t *testing.T) {
	t.Parallel()
	sender, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	recipient, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	wrong, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	sealed, err := BoxSeal(sender.PrivateKey, recipient.PublicKey, []byte("secret"))
	require.NoError(t, err)

	_, err = BoxOpen(recipient.PrivateKey, wrong.PublicKey, sealed)
	assert.Error(t, err)
}

func TestBoxTamperedCiphertext(t *testing.T) {
	t.Parallel()
	sender, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	recipient, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	sealed, err := BoxSeal(sender.PrivateKey, recipient.PublicKey, []byte("secret"))
	require.NoError(t, err)

	sealed[len(sealed)-1] ^= 0xff
	_, err = BoxOpen(recipient.PrivateKey, sender.PublicKey, sealed)
	assert.Error(t, err)
}

func TestBoxTooShort(t *testing.T) {
	t.Parallel()
	var priv, pub [KeySize]byte
	_, err := BoxOpen(priv, pub, make([]byte, NonceSize))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestBoxEmptyPlaintext(t *testing.T) {
	t.Parallel()
	sender, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	recipient, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	sealed, err := BoxSeal(sender.PrivateKey, recipient.PublicKey, []byte{})
	require.NoError(t, err)

	plaintext, err := BoxOpen(recipient.PrivateKey, sender.PublicKey, sealed)
	require.NoError(t, err)
	assert.Empty(t, plaintext)
}

func TestBoxUniqueNonces(t *testing.T) {
	t.Parallel()
	sender, err := GenerateDeviceKeyPair()
	require.NoError(t, err)
	recipient, err := GenerateDeviceKeyPair()
	require.NoError(t, err)

	msg := []byte("same message")
	sealed1, err := BoxSeal(sender.PrivateKey, recipient.PublicKey, msg)
	require.NoError(t, err)
	sealed2, err := BoxSeal(sender.PrivateKey, recipient.PublicKey, msg)
	require.NoError(t, err)

	assert.NotEqual(t, sealed1[:NonceSize], sealed2[:NonceSize])
}
