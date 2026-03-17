// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"context"
	"crypto/rand"
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
}

type deviceRecord struct {
	device    sync.Device
	tokenHash string
}

// NewMemStore creates a new in-memory relay store.
func NewMemStore() *MemStore {
	return &MemStore{
		households: make(map[string]sync.Household),
		devices:    make(map[string]deviceRecord),
		tokenIndex: make(map[string]string),
		seqs:       make(map[string]int64),
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

func (m *MemStore) Close() error { return nil }

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
