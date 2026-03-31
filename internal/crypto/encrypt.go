// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"runtime"

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
	// key is already a by-value copy (Go passes arrays by value),
	// so we zeroize it directly without an intermediate variable.
	defer zeroize(key[:])
	sealed := secretbox.Seal(nonce[:], plaintext, &nonce, (*[KeySize]byte)(&key))
	return sealed, nil
}

// Decrypt decrypts a sealed message (nonce || ciphertext) with the
// household key. Returns the plaintext or an error if decryption fails
// (wrong key, tampered data, or malformed input).
func Decrypt(key HouseholdKey, sealed []byte) ([]byte, error) {
	if len(sealed) < NonceSize+secretbox.Overhead {
		return nil, fmt.Errorf(
			"ciphertext too short: %d bytes (minimum %d)",
			len(sealed),
			NonceSize+secretbox.Overhead,
		)
	}
	var nonce [NonceSize]byte
	copy(nonce[:], sealed[:NonceSize])

	defer zeroize(key[:])
	plaintext, ok := secretbox.Open(nil, sealed[NonceSize:], &nonce, (*[KeySize]byte)(&key))
	if !ok {
		return nil, errors.New("decryption failed: invalid key or tampered ciphertext")
	}
	return plaintext, nil
}

// zeroize overwrites a byte slice with zeros.
// runtime.KeepAlive prevents the compiler from eliding clear() since the
// slice appears unused after this call. Note: Go's GC does not guarantee
// that copies on the stack (e.g. the caller's key parameter) are zeroed;
// this is a best-effort defense for the local copy.
func zeroize(b []byte) {
	clear(b)
	runtime.KeepAlive(b)
}
