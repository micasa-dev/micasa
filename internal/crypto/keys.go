// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package crypto

import (
	"crypto/rand"
	"crypto/subtle"
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

// String returns a redacted placeholder. Prevents accidental key leakage
// through fmt, slog, or any other Stringer consumer.
func (HouseholdKey) String() string { return "[REDACTED]" }

// DeviceKeyPair is a Curve25519 keypair for asymmetric key exchange.
type DeviceKeyPair struct {
	PublicKey  [KeySize]byte
	PrivateKey [KeySize]byte
}

// String returns a redacted placeholder.
func (DeviceKeyPair) String() string { return "[REDACTED]" }

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
		return DeviceKeyPair{}, fmt.Errorf("generate device key: %w", err)
	}
	pub, err := curve25519.X25519(kp.PrivateKey[:], curve25519.Basepoint)
	if err != nil {
		return DeviceKeyPair{}, fmt.Errorf("derive public key: %w", err)
	}
	copy(kp.PublicKey[:], pub)
	return kp, nil
}

// SecretsDir returns the platform-appropriate directory for key and
// credential storage. All files in this directory are sensitive and
// must never be committed, backed up to the cloud, or included in
// SQLite backups.
func SecretsDir() (string, error) {
	return xdg.DataFile(filepath.Join("micasa", "secrets", "."))
}

// SaveHouseholdKey writes the household key to dir/household.key with
// restrictive permissions (0600).
func SaveHouseholdKey(dir string, key HouseholdKey) error {
	return atomicWriteFile(filepath.Join(dir, HouseholdKeyFile), key[:], 0o600)
}

// LoadHouseholdKey reads a household key from dir/household.key.
func LoadHouseholdKey(dir string) (HouseholdKey, error) {
	var key HouseholdKey
	data, err := os.ReadFile(filepath.Join(dir, HouseholdKeyFile))
	if err != nil {
		return key, fmt.Errorf("load household key: %w", err)
	}
	defer zeroize(data)
	if len(data) != KeySize {
		return key, fmt.Errorf("household key: expected %d bytes, got %d", KeySize, len(data))
	}
	copy(key[:], data)
	return key, nil
}

// SaveDeviceKeyPair writes the device keypair to dir/ with restrictive
// permissions on the private key (0600). Uses write-then-rename to
// avoid leaving partial files on failure.
func SaveDeviceKeyPair(dir string, kp DeviceKeyPair) error {
	if err := atomicWriteFile(
		filepath.Join(dir, DevicePrivateKeyFile), kp.PrivateKey[:], 0o600,
	); err != nil {
		return fmt.Errorf("save device private key: %w", err)
	}
	if err := atomicWriteFile(
		filepath.Join(dir, DevicePublicKeyFile), kp.PublicKey[:], 0o644,
	); err != nil {
		return fmt.Errorf("save device public key: %w", err)
	}
	return nil
}

// atomicWriteFile writes data to a temporary file then renames it to
// the target path. This prevents partial writes from corrupting the
// destination file.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // clean up on any failure path

	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadDeviceKeyPair reads the device keypair from dir/ and validates
// that the public key matches the private key.
func LoadDeviceKeyPair(dir string) (DeviceKeyPair, error) {
	var kp DeviceKeyPair

	priv, err := os.ReadFile(filepath.Join(dir, DevicePrivateKeyFile))
	if err != nil {
		return DeviceKeyPair{}, fmt.Errorf("load device private key: %w", err)
	}
	defer zeroize(priv)
	if len(priv) != KeySize {
		return DeviceKeyPair{}, fmt.Errorf(
			"device private key: expected %d bytes, got %d",
			KeySize,
			len(priv),
		)
	}
	copy(kp.PrivateKey[:], priv)

	pub, err := os.ReadFile(filepath.Join(dir, DevicePublicKeyFile))
	if err != nil {
		return DeviceKeyPair{}, fmt.Errorf("load device public key: %w", err)
	}
	if len(pub) != KeySize {
		return DeviceKeyPair{}, fmt.Errorf(
			"device public key: expected %d bytes, got %d",
			KeySize,
			len(pub),
		)
	}
	copy(kp.PublicKey[:], pub)

	// Verify public key is consistent with private key.
	derived, err := curve25519.X25519(kp.PrivateKey[:], curve25519.Basepoint)
	if err != nil {
		return DeviceKeyPair{}, fmt.Errorf("validate device keypair: %w", err)
	}
	if subtle.ConstantTimeCompare(derived, kp.PublicKey[:]) != 1 {
		return DeviceKeyPair{}, fmt.Errorf("device public key does not match private key")
	}

	return kp, nil
}
