// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package crypto

import (
	"fmt"
	"os"
	"path/filepath"
)

const DeviceTokenFile = "device.token"

// SaveDeviceToken writes the device bearer token to dir/device.token
// with restrictive permissions (0600).
func SaveDeviceToken(dir, token string) error {
	if token == "" {
		return fmt.Errorf("save device token: token is empty")
	}
	return atomicWriteFile(filepath.Join(dir, DeviceTokenFile), []byte(token), 0o600)
}

// LoadDeviceToken reads the device bearer token from dir/device.token.
func LoadDeviceToken(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, DeviceTokenFile))
	if err != nil {
		return "", fmt.Errorf("load device token: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("device token file is empty")
	}
	return string(data), nil
}
