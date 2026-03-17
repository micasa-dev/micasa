// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import "time"

// Envelope wraps an encrypted oplog entry for transit between client
// and relay. The relay stores and routes envelopes without decrypting.
type Envelope struct {
	ID          string    `json:"id"`
	HouseholdID string    `json:"household_id"`
	DeviceID    string    `json:"device_id"`
	Nonce       []byte    `json:"nonce"`
	Ciphertext  []byte    `json:"ciphertext"`
	CreatedAt   time.Time `json:"created_at"`
	Seq         int64     `json:"seq,omitempty"`
}

// PushRequest is the body of POST /sync/push.
type PushRequest struct {
	Ops []Envelope `json:"ops"`
}

// PushConfirmation pairs a client op ID with its server-assigned sequence.
type PushConfirmation struct {
	ID  string `json:"id"`
	Seq int64  `json:"seq"`
}

// PushResponse is the response of POST /sync/push.
type PushResponse struct {
	Confirmed []PushConfirmation `json:"confirmed"`
}

// PullResponse is the response of GET /sync/pull.
type PullResponse struct {
	Ops     []Envelope `json:"ops"`
	HasMore bool       `json:"has_more"`
}

// Subscription status constants.
const (
	SubscriptionActive   = "active"
	SubscriptionPastDue  = "past_due"
	SubscriptionCanceled = "canceled"
)

// Household represents a sync household on the relay.
type Household struct {
	ID                   string    `json:"id"`
	CreatedAt            time.Time `json:"created_at"`
	StripeSubscriptionID string    `json:"stripe_subscription_id,omitempty"`
	StripeStatus         string    `json:"stripe_status,omitempty"`
}

// StatusResponse is the response of GET /status.
type StatusResponse struct {
	HouseholdID  string   `json:"household_id"`
	Devices      []Device `json:"devices"`
	OpsCount     int64    `json:"ops_count"`
	StripeStatus string   `json:"stripe_status"`
}

// Device represents a registered device on the relay.
type Device struct {
	ID          string    `json:"id"`
	HouseholdID string    `json:"household_id"`
	Name        string    `json:"name"`
	PublicKey   []byte    `json:"public_key,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateHouseholdRequest is the body of POST /households.
type CreateHouseholdRequest struct {
	DeviceName string `json:"device_name"`
	PublicKey  []byte `json:"public_key"`
}

// CreateHouseholdResponse is the response of POST /households.
type CreateHouseholdResponse struct {
	HouseholdID string `json:"household_id"`
	DeviceID    string `json:"device_id"`
	DeviceToken string `json:"device_token"`
}

// RegisterDeviceRequest is the body of POST /devices.
type RegisterDeviceRequest struct {
	HouseholdID string `json:"household_id"`
	Name        string `json:"device_name"`
	PublicKey   []byte `json:"public_key"`
}

// RegisterDeviceResponse is the response of POST /devices.
type RegisterDeviceResponse struct {
	DeviceID    string `json:"device_id"`
	DeviceToken string `json:"device_token"`
}

// InviteCode represents a one-time invite code for joining a household.
type InviteCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// JoinRequest is the body of POST /households/{id}/join.
type JoinRequest struct {
	InviteCode string `json:"invite_code"`
	DeviceName string `json:"device_name"`
	PublicKey  []byte `json:"public_key"`
}

// JoinResponse is returned when a joiner initiates a key exchange.
type JoinResponse struct {
	ExchangeID       string `json:"exchange_id"`
	HouseholdID      string `json:"household_id"`
	InviterPublicKey []byte `json:"inviter_public_key"`
}

// PendingKeyExchange represents a key exchange awaiting inviter completion.
type PendingKeyExchange struct {
	ID              string    `json:"id"`
	JoinerPublicKey []byte    `json:"joiner_public_key"`
	JoinerName      string    `json:"joiner_name"`
	CreatedAt       time.Time `json:"created_at"`
}

// CompleteKeyExchangeRequest is the body of POST /key-exchange/{id}/complete.
type CompleteKeyExchangeRequest struct {
	EncryptedHouseholdKey []byte `json:"encrypted_household_key"`
}

// KeyExchangeResult is returned when a joiner polls for their key exchange.
type KeyExchangeResult struct {
	Ready                 bool   `json:"ready"`
	EncryptedHouseholdKey []byte `json:"encrypted_household_key,omitempty"`
	DeviceID              string `json:"device_id,omitempty"`
	DeviceToken           string `json:"device_token,omitempty"`
}
