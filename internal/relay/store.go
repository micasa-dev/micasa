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

	// CreateInvite generates a one-time invite code for a household.
	// Max 3 active invites per household. Code expires in 24 hours.
	CreateInvite(ctx context.Context, householdID, deviceID string) (sync.InviteCode, error)

	// StartJoin validates an invite code and creates a pending key
	// exchange. Returns the exchange ID and the inviter's public key.
	StartJoin(ctx context.Context, code string, req sync.JoinRequest) (sync.JoinResponse, error)

	// GetPendingExchanges returns incomplete key exchanges for a household.
	GetPendingExchanges(ctx context.Context, householdID string) ([]sync.PendingKeyExchange, error)

	// CompleteKeyExchange stores the encrypted household key and
	// registers the joiner as a device. Only the inviting device
	// (same household) may complete an exchange.
	CompleteKeyExchange(
		ctx context.Context,
		householdID, exchangeID string,
		encryptedKey []byte,
	) error

	// GetKeyExchangeResult returns the key exchange status. When
	// complete, includes the encrypted key and device credentials.
	GetKeyExchangeResult(ctx context.Context, exchangeID string) (sync.KeyExchangeResult, error)

	// ListDevices returns all active devices in a household.
	ListDevices(ctx context.Context, householdID string) ([]sync.Device, error)

	// RevokeDevice removes a device from a household, preventing
	// further authentication.
	RevokeDevice(ctx context.Context, householdID, deviceID string) error

	// GetHousehold returns the household with its subscription status.
	GetHousehold(ctx context.Context, householdID string) (sync.Household, error)

	// UpdateSubscription sets the Stripe subscription ID and status
	// on a household.
	UpdateSubscription(
		ctx context.Context,
		householdID, subscriptionID, status string,
	) error

	// HouseholdBySubscription finds a household by its Stripe
	// subscription ID. Used by webhook processing.
	HouseholdBySubscription(
		ctx context.Context,
		subscriptionID string,
	) (sync.Household, error)

	// OpsCount returns the total number of ops stored for a household.
	OpsCount(ctx context.Context, householdID string) (int64, error)

	// Close releases any resources held by the store.
	Close() error
}
