// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"github.com/micasa-dev/micasa/internal/app"
	"github.com/micasa-dev/micasa/internal/crypto"
	"github.com/micasa-dev/micasa/internal/data"
)

// tryLoadSyncConfig detects a Pro setup and configures background sync
// on the app options. Silently returns on any failure — sync is optional.
func tryLoadSyncConfig(store *data.Store, opts *app.Options) {
	dev, err := store.GetSyncDevice()
	if err != nil || dev.HouseholdID == "" {
		return
	}
	secretDir, err := crypto.SecretsDir()
	if err != nil {
		return
	}
	token, err := crypto.LoadDeviceToken(secretDir)
	if err != nil {
		return
	}
	key, err := crypto.LoadHouseholdKey(secretDir)
	if err != nil {
		return
	}
	opts.SetSync(dev.RelayURL, token, dev.HouseholdID, key)
}
