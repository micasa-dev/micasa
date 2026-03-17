// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package crypto

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/nacl/secretbox"
)

// NonceSize is the byte length of an XSalsa20 nonce.
const NonceSize = 24

// Encrypt encrypts plaintext with the household key using NaCl secretbox
// (XSalsa20-Poly1305). Returns nonce || ciphertext.
func Encrypt(key HouseholdKey, plaintext []byte) ([]byte, error) {
	var nonce [NonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	k := [KeySize]byte(key)
	sealed := secretbox.Seal(nonce[:], plaintext, &nonce, &k)
	return sealed, nil
}

// Decrypt decrypts a sealed message (nonce || ciphertext) with the
// household key. Returns the plaintext or an error if decryption fails
// (wrong key, tampered data, or malformed input).
func Decrypt(key HouseholdKey, sealed []byte) ([]byte, error) {
	if len(sealed) <= NonceSize {
		return nil, fmt.Errorf("ciphertext too short: %d bytes", len(sealed))
	}
	var nonce [NonceSize]byte
	copy(nonce[:], sealed[:NonceSize])

	k := [KeySize]byte(key)
	plaintext, ok := secretbox.Open(nil, sealed[NonceSize:], &nonce, &k)
	if !ok {
		return nil, fmt.Errorf("decryption failed: invalid key or tampered ciphertext")
	}
	return plaintext, nil
}
