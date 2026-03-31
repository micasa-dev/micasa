// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package crypto

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const DeviceTokenFile = "device.token"

// SaveDeviceToken writes the device bearer token to dir/device.token
// with restrictive permissions (0600).
func SaveDeviceToken(dir, token string) error {
	if token == "" {
		return errors.New("save device token: token is empty")
	}
	return atomicWriteFile(filepath.Join(dir, DeviceTokenFile), []byte(token), 0o600)
}

// deviceTokenLen is the expected length of a device token (64-char hex = 256 bits).
const deviceTokenLen = 64

// LoadDeviceToken reads the device bearer token from dir/device.token.
// Validates that the token is a 64-character lowercase hex string.
func LoadDeviceToken(dir string) (string, error) {
	data, err := os.ReadFile( //nolint:gosec // path is caller-controlled
		filepath.Join(dir, DeviceTokenFile),
	)
	if err != nil {
		return "", fmt.Errorf("load device token: %w", err)
	}
	// Best-effort: clears the []byte copy; the returned string is immutable
	// and cannot be zeroized by the caller.
	defer zeroize(data)
	if len(data) == 0 {
		return "", errors.New("device token file is empty")
	}
	token := string(data)
	if !validDeviceToken(token) {
		return "", fmt.Errorf(
			"invalid device token format: expected %d lowercase hex characters",
			deviceTokenLen,
		)
	}
	return token, nil
}

// validDeviceToken returns true if s is a 64-character lowercase hex string.
func validDeviceToken(s string) bool {
	if len(s) != deviceTokenLen {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
