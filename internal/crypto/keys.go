// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package crypto

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"golang.org/x/crypto/curve25519"
)

const (
	KeySize              = 32
	HouseholdKeyFile     = "household.key"
	DevicePublicKeyFile  = "device.pub"
	DevicePrivateKeyFile = "device.key"
)

// HouseholdKey is a 256-bit symmetric key for NaCl secretbox encryption.
type HouseholdKey [KeySize]byte

// DeviceKeyPair is a Curve25519 keypair for asymmetric key exchange.
type DeviceKeyPair struct {
	PublicKey  [KeySize]byte
	PrivateKey [KeySize]byte
}

// GenerateHouseholdKey creates a new random 256-bit household key.
func GenerateHouseholdKey() (HouseholdKey, error) {
	var key HouseholdKey
	if _, err := rand.Read(key[:]); err != nil {
		return key, fmt.Errorf("generate household key: %w", err)
	}
	return key, nil
}

// GenerateDeviceKeyPair creates a new Curve25519 keypair.
func GenerateDeviceKeyPair() (DeviceKeyPair, error) {
	var kp DeviceKeyPair
	if _, err := rand.Read(kp.PrivateKey[:]); err != nil {
		return kp, fmt.Errorf("generate device key: %w", err)
	}
	pub, err := curve25519.X25519(kp.PrivateKey[:], curve25519.Basepoint)
	if err != nil {
		return kp, fmt.Errorf("derive public key: %w", err)
	}
	copy(kp.PublicKey[:], pub)
	return kp, nil
}

// KeysDir returns the platform-appropriate directory for key storage.
func KeysDir() (string, error) {
	return xdg.DataFile(filepath.Join("micasa", "keys", "."))
}

// SaveHouseholdKey writes the household key to dir/household.key with
// restrictive permissions (0600).
func SaveHouseholdKey(dir string, key HouseholdKey) error {
	return os.WriteFile(filepath.Join(dir, HouseholdKeyFile), key[:], 0o600)
}

// LoadHouseholdKey reads a household key from dir/household.key.
func LoadHouseholdKey(dir string) (HouseholdKey, error) {
	var key HouseholdKey
	data, err := os.ReadFile(filepath.Join(dir, HouseholdKeyFile))
	if err != nil {
		return key, fmt.Errorf("load household key: %w", err)
	}
	if len(data) != KeySize {
		return key, fmt.Errorf("household key: expected %d bytes, got %d", KeySize, len(data))
	}
	copy(key[:], data)
	return key, nil
}

// SaveDeviceKeyPair writes the device keypair to dir/ with restrictive
// permissions on the private key (0600).
func SaveDeviceKeyPair(dir string, kp DeviceKeyPair) error {
	if err := os.WriteFile(
		filepath.Join(dir, DevicePrivateKeyFile), kp.PrivateKey[:], 0o600,
	); err != nil {
		return fmt.Errorf("save device private key: %w", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, DevicePublicKeyFile), kp.PublicKey[:], 0o644,
	); err != nil {
		return fmt.Errorf("save device public key: %w", err)
	}
	return nil
}

// LoadDeviceKeyPair reads the device keypair from dir/.
func LoadDeviceKeyPair(dir string) (DeviceKeyPair, error) {
	var kp DeviceKeyPair

	priv, err := os.ReadFile(filepath.Join(dir, DevicePrivateKeyFile))
	if err != nil {
		return kp, fmt.Errorf("load device private key: %w", err)
	}
	if len(priv) != KeySize {
		return kp, fmt.Errorf("device private key: expected %d bytes, got %d", KeySize, len(priv))
	}
	copy(kp.PrivateKey[:], priv)

	pub, err := os.ReadFile(filepath.Join(dir, DevicePublicKeyFile))
	if err != nil {
		return kp, fmt.Errorf("load device public key: %w", err)
	}
	if len(pub) != KeySize {
		return kp, fmt.Errorf("device public key: expected %d bytes, got %d", KeySize, len(pub))
	}
	copy(kp.PublicKey[:], pub)

	return kp, nil
}
