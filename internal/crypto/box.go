// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package crypto

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/nacl/box"
)

const boxOverhead = box.Overhead

// BoxSeal encrypts plaintext using NaCl box (Curve25519 + XSalsa20-Poly1305).
// The sender's private key and recipient's public key are used.
// Returns nonce || ciphertext.
func BoxSeal(
	senderPrivateKey, recipientPublicKey [KeySize]byte,
	plaintext []byte,
) ([]byte, error) {
	// Best-effort: only clears this function's stack copy.
	defer zeroize(senderPrivateKey[:])
	var nonce [NonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	sealed := box.Seal(nonce[:], plaintext, &nonce, &recipientPublicKey, &senderPrivateKey)
	return sealed, nil
}

// BoxOpen decrypts a sealed message (nonce || ciphertext) using NaCl box.
// The recipient's private key and sender's public key are used.
func BoxOpen(
	recipientPrivateKey, senderPublicKey [KeySize]byte,
	sealed []byte,
) ([]byte, error) {
	// Best-effort: only clears this function's stack copy.
	defer zeroize(recipientPrivateKey[:])
	if len(sealed) < NonceSize+boxOverhead {
		return nil, fmt.Errorf(
			"sealed message too short: %d bytes (minimum %d)",
			len(sealed),
			NonceSize+boxOverhead,
		)
	}
	var nonce [NonceSize]byte
	copy(nonce[:], sealed[:NonceSize])

	plaintext, ok := box.Open(
		nil,
		sealed[NonceSize:],
		&nonce,
		&senderPublicKey,
		&recipientPrivateKey,
	)
	if !ok {
		return nil, fmt.Errorf("box open failed: invalid keys or tampered ciphertext")
	}
	return plaintext, nil
}
