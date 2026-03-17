// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	gosync "sync"
	"time"

	"github.com/cpcloud/micasa/internal/sync"
	"github.com/cpcloud/micasa/internal/uid"
	"golang.org/x/crypto/bcrypt"
)

// MemStore is an in-memory implementation of Store for testing.
type MemStore struct {
	mu         gosync.Mutex
	ops        []sync.Envelope
	households map[string]sync.Household
	devices    map[string]deviceRecord
	tokenIndex map[string]string // token_hash -> device_id
	seqs       map[string]int64  // household_id -> last_seq
	invites    map[string]*inviteRecord
	exchanges  map[string]*keyExchangeRecord
}

type deviceRecord struct {
	device    sync.Device
	tokenHash string
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
	}
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
	m.devices[devID] = deviceRecord{device: dev, tokenHash: tokenHash}
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
	m.devices[devID] = deviceRecord{device: dev, tokenHash: tokenHash}
	m.tokenIndex[tokenHash] = devID

	return sync.RegisterDeviceResponse{
		DeviceID:    devID,
		DeviceToken: token,
	}, nil
}

func (m *MemStore) AuthenticateDevice(_ context.Context, token string) (sync.Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for hash, devID := range m.tokenIndex {
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)) == nil {
			rec := m.devices[devID]
			return rec.device, nil
		}
	}
	return sync.Device{}, fmt.Errorf("invalid token")
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
	if active >= 3 {
		return sync.InviteCode{}, fmt.Errorf("max active invites reached (3)")
	}

	code, err := generateInviteCode()
	if err != nil {
		return sync.InviteCode{}, err
	}
	m.invites[code] = &inviteRecord{
		code:         code,
		householdID:  householdID,
		inviterDevID: deviceID,
		expiresAt:    time.Now().Add(24 * time.Hour),
		maxAttempts:  5,
	}

	return sync.InviteCode{
		Code:      code,
		ExpiresAt: m.invites[code].expiresAt,
	}, nil
}

func (m *MemStore) StartJoin(
	_ context.Context,
	code string,
	req sync.JoinRequest,
) (sync.JoinResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	inv, ok := m.invites[code]
	if !ok {
		return sync.JoinResponse{}, fmt.Errorf("invite code not found")
	}
	if inv.consumed {
		return sync.JoinResponse{}, fmt.Errorf("invite code already consumed")
	}
	if time.Now().After(inv.expiresAt) {
		return sync.JoinResponse{}, fmt.Errorf("invite code expired")
	}
	inv.usedAttempts++
	if inv.usedAttempts >= inv.maxAttempts {
		inv.consumed = true
		return sync.JoinResponse{}, fmt.Errorf("invite code max attempts exceeded")
	}

	// Find inviter's public key.
	inviterDev, ok := m.devices[inv.inviterDevID]
	if !ok {
		return sync.JoinResponse{}, fmt.Errorf("inviter device not found")
	}

	exchangeID := uid.New()
	m.exchanges[exchangeID] = &keyExchangeRecord{
		id:              exchangeID,
		householdID:     inv.householdID,
		inviteCode:      code,
		joinerName:      req.DeviceName,
		joinerPublicKey: req.PublicKey,
		createdAt:       time.Now(),
	}

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
		if ex.householdID == householdID && !ex.completed {
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

	dev := sync.Device{
		ID:          devID,
		HouseholdID: householdID,
		Name:        ex.joinerName,
		PublicKey:   ex.joinerPublicKey,
		CreatedAt:   time.Now(),
	}
	m.devices[devID] = deviceRecord{device: dev, tokenHash: tokenHash}
	m.tokenIndex[tokenHash] = devID

	ex.encryptedKey = encryptedKey
	ex.deviceID = devID
	ex.deviceToken = token
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

	if !ex.completed {
		return sync.KeyExchangeResult{Ready: false}, nil
	}

	result := sync.KeyExchangeResult{
		Ready:                 true,
		EncryptedHouseholdKey: ex.encryptedKey,
		DeviceID:              ex.deviceID,
		DeviceToken:           ex.deviceToken,
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
		if rec.device.HouseholdID == householdID {
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

	// Remove token from index.
	delete(m.tokenIndex, rec.tokenHash)
	// Remove device.
	delete(m.devices, deviceID)
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
	hh.StripeSubscriptionID = subscriptionID
	hh.StripeStatus = status
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
		if hh.StripeSubscriptionID == subscriptionID {
			return hh, nil
		}
	}
	return sync.Household{}, fmt.Errorf(
		"no household with subscription %s",
		subscriptionID,
	)
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

func (m *MemStore) Close() error { return nil }

func generateInviteCode() (string, error) {
	b := make([]byte, 8) // 8 bytes = ~64 bits entropy
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate invite code: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

func generateToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	raw = hex.EncodeToString(b)
	hashed, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	if err != nil {
		return "", "", fmt.Errorf("hash token: %w", err)
	}
	return raw, string(hashed), nil
}
