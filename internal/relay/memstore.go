// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	gosync "sync"
	"time"

	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/micasa-dev/micasa/internal/uid"
)

// newCryptoToken returns a 256-bit (32-byte) crypto-random hex string.
// Used for exchange IDs where unpredictability matters more than
// time-sortability (unlike entity ULIDs).
func newCryptoToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// MemStore is an in-memory implementation of Store for testing.
type MemStore struct {
	mu            gosync.Mutex
	encryptionKey []byte
	ops           []sync.Envelope
	households    map[string]sync.Household
	devices       map[string]deviceRecord
	tokenIndex    map[string]string // sha256(raw_token) hex -> device_id
	seqs          map[string]int64  // household_id -> last_seq
	invites       map[string]*inviteRecord
	exchanges     map[string]*keyExchangeRecord
	blobs         map[string]map[string][]byte // household_id -> hash -> data
}

type deviceRecord struct {
	device   sync.Device
	tokenSHA string // sha256(raw_token) hex — used as tokenIndex key
	revoked  bool
}

type inviteRecord struct {
	code         string
	householdID  string
	inviterDevID string
	expiresAt    time.Time
	maxAttempts  int
	usedAttempts int
	consumed     bool
}

type keyExchangeRecord struct {
	id              string
	householdID     string
	inviteCode      string
	joinerName      string
	joinerPublicKey []byte
	encryptedKey    []byte
	deviceID        string
	deviceToken     string
	createdAt       time.Time
	completed       bool
}

// NewMemStore creates a new in-memory relay store.
func NewMemStore() *MemStore {
	return &MemStore{
		households: make(map[string]sync.Household),
		devices:    make(map[string]deviceRecord),
		tokenIndex: make(map[string]string),
		seqs:       make(map[string]int64),
		invites:    make(map[string]*inviteRecord),
		exchanges:  make(map[string]*keyExchangeRecord),
		blobs:      make(map[string]map[string][]byte),
	}
}

func (m *MemStore) SetEncryptionKey(key []byte) {
	if len(key) != 32 {
		panic(fmt.Sprintf("encryption key must be exactly 32 bytes, got %d", len(key)))
	}
	m.encryptionKey = key
}

func (m *MemStore) Push(_ context.Context, ops []sync.Envelope) ([]sync.PushConfirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	confirmed := make([]sync.PushConfirmation, 0, len(ops))
	for _, op := range ops {
		if _, ok := m.households[op.HouseholdID]; !ok {
			return nil, fmt.Errorf("household %s not found", op.HouseholdID)
		}
		m.seqs[op.HouseholdID]++
		seq := m.seqs[op.HouseholdID]
		op.Seq = seq
		m.ops = append(m.ops, op)
		confirmed = append(confirmed, sync.PushConfirmation{
			ID:  op.ID,
			Seq: seq,
		})
	}
	return confirmed, nil
}

func (m *MemStore) Pull(
	_ context.Context,
	householdID, excludeDeviceID string,
	afterSeq int64,
	limit int,
) ([]sync.Envelope, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limit <= 0 {
		limit = 100
	}

	var result []sync.Envelope
	for _, op := range m.ops {
		if op.HouseholdID != householdID {
			continue
		}
		if op.DeviceID == excludeDeviceID {
			continue
		}
		if op.Seq <= afterSeq {
			continue
		}
		result = append(result, op)
		if len(result) >= limit+1 {
			break
		}
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, hasMore, nil
}

func (m *MemStore) CreateHousehold(
	_ context.Context,
	req sync.CreateHouseholdRequest,
) (sync.CreateHouseholdResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hhID := uid.New()
	m.households[hhID] = sync.Household{
		ID:        hhID,
		CreatedAt: time.Now(),
	}

	devID := uid.New()
	token, tokenHash, err := generateToken()
	if err != nil {
		return sync.CreateHouseholdResponse{}, err
	}

	dev := sync.Device{
		ID:          devID,
		HouseholdID: hhID,
		Name:        req.DeviceName,
		PublicKey:   req.PublicKey,
		CreatedAt:   time.Now(),
	}
	m.devices[devID] = deviceRecord{device: dev, tokenSHA: tokenHash}
	m.tokenIndex[tokenHash] = devID

	return sync.CreateHouseholdResponse{
		HouseholdID: hhID,
		DeviceID:    devID,
		DeviceToken: token,
	}, nil
}

func (m *MemStore) RegisterDevice(
	_ context.Context,
	req sync.RegisterDeviceRequest,
) (sync.RegisterDeviceResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.households[req.HouseholdID]; !ok {
		return sync.RegisterDeviceResponse{}, fmt.Errorf("household %s not found", req.HouseholdID)
	}

	devID := uid.New()
	token, tokenHash, err := generateToken()
	if err != nil {
		return sync.RegisterDeviceResponse{}, err
	}

	dev := sync.Device{
		ID:          devID,
		HouseholdID: req.HouseholdID,
		Name:        req.Name,
		PublicKey:   req.PublicKey,
		CreatedAt:   time.Now(),
	}
	m.devices[devID] = deviceRecord{device: dev, tokenSHA: tokenHash}
	m.tokenIndex[tokenHash] = devID

	return sync.RegisterDeviceResponse{
		DeviceID:    devID,
		DeviceToken: token,
	}, nil
}

func (m *MemStore) AuthenticateDevice(_ context.Context, token string) (sync.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sha := tokenSHA256(token)
	devID, ok := m.tokenIndex[sha]
	if !ok {
		return sync.Device{}, fmt.Errorf("invalid token")
	}
	rec := m.devices[devID]
	if rec.revoked {
		return sync.Device{}, fmt.Errorf("invalid token")
	}
	return rec.device, nil
}

func (m *MemStore) CreateInvite(
	_ context.Context,
	householdID, deviceID string,
) (sync.InviteCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.households[householdID]; !ok {
		return sync.InviteCode{}, fmt.Errorf("household %s not found", householdID)
	}

	// Max 3 active invites per household.
	active := 0
	for _, inv := range m.invites {
		if inv.householdID == householdID && !inv.consumed && time.Now().Before(inv.expiresAt) {
			active++
		}
	}
	if active >= maxActiveInvites {
		return sync.InviteCode{}, fmt.Errorf("max active invites reached (%d)", maxActiveInvites)
	}

	code, err := generateInviteCode()
	if err != nil {
		return sync.InviteCode{}, err
	}
	m.invites[code] = &inviteRecord{
		code:         code,
		householdID:  householdID,
		inviterDevID: deviceID,
		expiresAt:    time.Now().Add(inviteExpiry),
		maxAttempts:  maxInviteAttempts,
	}

	return sync.InviteCode{
		Code:      code,
		ExpiresAt: m.invites[code].expiresAt,
	}, nil
}

func (m *MemStore) StartJoin(
	_ context.Context,
	householdID, code string,
	req sync.JoinRequest,
) (sync.JoinResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	inv, ok := m.invites[code]
	if !ok {
		return sync.JoinResponse{}, fmt.Errorf("invite code not found")
	}
	if inv.householdID != householdID {
		return sync.JoinResponse{}, fmt.Errorf("invite code not found")
	}
	if inv.consumed {
		return sync.JoinResponse{}, fmt.Errorf("invite code already consumed")
	}
	if time.Now().After(inv.expiresAt) {
		return sync.JoinResponse{}, fmt.Errorf("invite code expired")
	}
	if inv.usedAttempts >= inv.maxAttempts {
		inv.consumed = true
		return sync.JoinResponse{}, fmt.Errorf("invite code max attempts exceeded")
	}

	// Find inviter's public key.
	inviterDev, ok := m.devices[inv.inviterDevID]
	if !ok {
		return sync.JoinResponse{}, fmt.Errorf("inviter device not found")
	}

	exchangeID := newCryptoToken()
	m.exchanges[exchangeID] = &keyExchangeRecord{
		id:              exchangeID,
		householdID:     inv.householdID,
		inviteCode:      code,
		joinerName:      req.DeviceName,
		joinerPublicKey: req.PublicKey,
		createdAt:       time.Now(),
	}

	// Increment after successful key exchange creation so that
	// valid joins don't consume brute-force attempt slots.
	inv.usedAttempts++

	return sync.JoinResponse{
		ExchangeID:       exchangeID,
		HouseholdID:      inv.householdID,
		InviterPublicKey: inviterDev.device.PublicKey,
	}, nil
}

func (m *MemStore) GetPendingExchanges(
	_ context.Context,
	householdID string,
) ([]sync.PendingKeyExchange, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []sync.PendingKeyExchange
	for _, ex := range m.exchanges {
		if ex.householdID == householdID && !ex.completed &&
			time.Since(ex.createdAt) <= keyExchangeExpiry {
			result = append(result, sync.PendingKeyExchange{
				ID:              ex.id,
				JoinerPublicKey: ex.joinerPublicKey,
				JoinerName:      ex.joinerName,
				CreatedAt:       ex.createdAt,
			})
		}
	}
	return result, nil
}

func (m *MemStore) CompleteKeyExchange(
	_ context.Context,
	householdID, exchangeID string,
	encryptedKey []byte,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ex, ok := m.exchanges[exchangeID]
	if !ok {
		return fmt.Errorf("key exchange %s not found", exchangeID)
	}
	if ex.householdID != householdID {
		return fmt.Errorf("key exchange does not belong to this household")
	}
	if ex.completed {
		return fmt.Errorf("key exchange already completed")
	}

	// Register the joiner as a device.
	devID := uid.New()
	token, tokenHash, err := generateToken()
	if err != nil {
		return err
	}
	encToken, err := encryptToken(m.encryptionKey, token)
	if err != nil {
		return fmt.Errorf("encrypt device token: %w", err)
	}

	dev := sync.Device{
		ID:          devID,
		HouseholdID: householdID,
		Name:        ex.joinerName,
		PublicKey:   ex.joinerPublicKey,
		CreatedAt:   time.Now(),
	}
	m.devices[devID] = deviceRecord{device: dev, tokenSHA: tokenHash}
	m.tokenIndex[tokenHash] = devID

	ex.encryptedKey = encryptedKey
	ex.deviceID = devID
	ex.deviceToken = encToken
	ex.completed = true

	// Consume the invite code.
	if inv, ok := m.invites[ex.inviteCode]; ok {
		inv.consumed = true
	}

	return nil
}

func (m *MemStore) GetKeyExchangeResult(
	_ context.Context,
	exchangeID string,
) (sync.KeyExchangeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ex, ok := m.exchanges[exchangeID]
	if !ok {
		return sync.KeyExchangeResult{}, fmt.Errorf("key exchange %s not found", exchangeID)
	}

	if time.Since(ex.createdAt) > keyExchangeExpiry {
		return sync.KeyExchangeResult{}, fmt.Errorf("key exchange %s not found", exchangeID)
	}

	if !ex.completed {
		return sync.KeyExchangeResult{Ready: false}, nil
	}

	// Credentials are single-use: cleared after first retrieval. If
	// the encrypted key is nil on a completed exchange, a previous
	// retrieval already consumed them and the client must create a
	// new invite.
	if ex.encryptedKey == nil {
		return sync.KeyExchangeResult{}, fmt.Errorf(
			"key exchange %s credentials already consumed; create a new invite", exchangeID,
		)
	}

	plainToken, err := decryptToken(m.encryptionKey, ex.deviceToken)
	if err != nil {
		return sync.KeyExchangeResult{}, fmt.Errorf("decrypt device token: %w", err)
	}

	result := sync.KeyExchangeResult{
		Ready:                 true,
		EncryptedHouseholdKey: ex.encryptedKey,
		DeviceID:              ex.deviceID,
		DeviceToken:           plainToken,
	}

	// Single-use: clear credentials after first retrieval so they
	// cannot be obtained by a second caller. The device ID remains
	// (it's not a secret) but the token and encrypted key are gone.
	ex.encryptedKey = nil
	ex.deviceToken = ""

	return result, nil
}

func (m *MemStore) ListDevices(
	_ context.Context,
	householdID string,
) ([]sync.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []sync.Device
	for _, rec := range m.devices {
		if rec.device.HouseholdID == householdID && !rec.revoked {
			result = append(result, rec.device)
		}
	}
	return result, nil
}

func (m *MemStore) RevokeDevice(
	_ context.Context,
	householdID, deviceID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rec, ok := m.devices[deviceID]
	if !ok {
		return fmt.Errorf("device %s not found", deviceID)
	}
	if rec.device.HouseholdID != householdID {
		return fmt.Errorf("device does not belong to this household")
	}

	// Remove token from index so the device can no longer authenticate.
	delete(m.tokenIndex, rec.tokenSHA)
	// Mark as revoked (matching PgStore behavior — keeps the record).
	rec.revoked = true
	m.devices[deviceID] = rec
	return nil
}

func (m *MemStore) GetHousehold(
	_ context.Context,
	householdID string,
) (sync.Household, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hh, ok := m.households[householdID]
	if !ok {
		return sync.Household{}, fmt.Errorf("household %s not found", householdID)
	}
	return hh, nil
}

func (m *MemStore) UpdateSubscription(
	_ context.Context,
	householdID, subscriptionID, status string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	hh, ok := m.households[householdID]
	if !ok {
		return fmt.Errorf("household %s not found", householdID)
	}
	hh.StripeSubscriptionID = &subscriptionID
	hh.StripeStatus = &status
	m.households[householdID] = hh
	return nil
}

func (m *MemStore) HouseholdBySubscription(
	_ context.Context,
	subscriptionID string,
) (sync.Household, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, hh := range m.households {
		if hh.StripeSubscriptionID != nil && *hh.StripeSubscriptionID == subscriptionID {
			return hh, nil
		}
	}
	return sync.Household{}, fmt.Errorf(
		"no household with subscription %s",
		subscriptionID,
	)
}

func (m *MemStore) UpdateCustomerID(
	_ context.Context,
	householdID, customerID string,
) error {
	if customerID == "" {
		return fmt.Errorf("customer ID must not be empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hh, ok := m.households[householdID]
	if !ok {
		return fmt.Errorf("household %q not found", householdID)
	}
	hh.StripeCustomerID = &customerID
	m.households[householdID] = hh
	return nil
}

func (m *MemStore) HouseholdByCustomer(
	_ context.Context,
	customerID string,
) (sync.Household, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, hh := range m.households {
		if hh.StripeCustomerID != nil && *hh.StripeCustomerID == customerID {
			return hh, nil
		}
	}
	return sync.Household{}, fmt.Errorf("no household with customer %q", customerID)
}

func (m *MemStore) OpsCount(
	_ context.Context,
	householdID string,
) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	for _, op := range m.ops {
		if op.HouseholdID == householdID {
			count++
		}
	}
	return count, nil
}

func (m *MemStore) PutBlob(
	_ context.Context,
	householdID, hash string,
	data []byte,
	quota int64,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	hhBlobs, ok := m.blobs[householdID]
	if !ok {
		hhBlobs = make(map[string][]byte)
		m.blobs[householdID] = hhBlobs
	}

	if _, exists := hhBlobs[hash]; exists {
		return errBlobExists
	}

	// When quota is 0, skip enforcement (unlimited).
	if quota > 0 {
		// Check quota. Written as used > quota-len to avoid overflow on
		// the left side (used+len could wrap). Safe with signed int64:
		// if len(data) > quota the RHS goes negative and the check holds.
		var used int64
		for _, b := range hhBlobs {
			used += int64(len(b))
		}
		if used > quota-int64(len(data)) {
			return errQuotaExceeded
		}
	}

	// Store a copy of the data.
	stored := make([]byte, len(data))
	copy(stored, data)
	hhBlobs[hash] = stored
	return nil
}

func (m *MemStore) GetBlob(_ context.Context, householdID, hash string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hhBlobs, ok := m.blobs[householdID]
	if !ok {
		return nil, errBlobNotFound
	}
	data, ok := hhBlobs[hash]
	if !ok {
		return nil, errBlobNotFound
	}
	// Return a copy to prevent callers from mutating internal state.
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

func (m *MemStore) HasBlob(_ context.Context, householdID, hash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hhBlobs, ok := m.blobs[householdID]
	if !ok {
		return false, nil
	}
	_, exists := hhBlobs[hash]
	return exists, nil
}

func (m *MemStore) BlobUsage(_ context.Context, householdID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var total int64
	for _, b := range m.blobs[householdID] {
		total += int64(len(b))
	}
	return total, nil
}

func (m *MemStore) Close() error { return nil }

func generateInviteCode() (string, error) {
	b := make([]byte, 8) // 8 bytes = ~64 bits entropy
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate invite code: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// tokenSHA256 returns the hex-encoded SHA-256 of a raw token string.
// Used as the O(1) lookup key in tokenIndex. Tokens are 256-bit
// crypto-random so a fast hash is safe (no brute-force risk).
func tokenSHA256(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func generateToken() (raw string, sha string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	raw = hex.EncodeToString(b)
	return raw, tokenSHA256(raw), nil
}
