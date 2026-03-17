// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"context"

	"github.com/cpcloud/micasa/internal/sync"
)

// Store defines the persistence interface for the relay server.
// Implementations handle storage of encrypted operations and device
// registration without needing to decrypt any user data.
type Store interface {
	// Push stores a batch of encrypted operations, assigning
	// server-side sequence numbers. Returns confirmations.
	Push(ctx context.Context, ops []sync.Envelope) ([]sync.PushConfirmation, error)

	// Pull returns encrypted operations for the given household
	// after the specified sequence number, excluding ops from the
	// given device. Returns up to limit ops.
	Pull(
		ctx context.Context,
		householdID, excludeDeviceID string,
		afterSeq int64,
		limit int,
	) ([]sync.Envelope, bool, error)

	// CreateHousehold creates a new household and registers the
	// first device. Returns the household, device ID, and a raw
	// bearer token (not hashed).
	CreateHousehold(
		ctx context.Context,
		req sync.CreateHouseholdRequest,
	) (sync.CreateHouseholdResponse, error)

	// RegisterDevice registers a new device in an existing household.
	// Returns the device ID and a raw bearer token.
	RegisterDevice(
		ctx context.Context,
		req sync.RegisterDeviceRequest,
	) (sync.RegisterDeviceResponse, error)

	// AuthenticateDevice verifies a bearer token and returns the
	// associated device. Returns an error if auth fails.
	AuthenticateDevice(ctx context.Context, token string) (sync.Device, error)

	// Close releases any resources held by the store.
	Close() error
}
