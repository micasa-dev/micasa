// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync_test

import (
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/crypto"
	"github.com/micasa-dev/micasa/internal/relay"
	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/micasa-dev/micasa/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRelay(t *testing.T) (*httptest.Server, *relay.MemStore, string) {
	t.Helper()
	store := relay.NewMemStore()
	handler := relay.NewHandler(store, slog.Default())
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Create a household + device.
	resp, err := store.CreateHousehold(t.Context(), sync.CreateHouseholdRequest{
		DeviceName: "test-device",
		PublicKey:  []byte("test-key-32-bytes-of-padding!!!!"),
	})
	require.NoError(t, err)
	return srv, store, resp.DeviceToken
}

func TestClientPushAndPull(t *testing.T) {
	t.Parallel()
	srv, store, tokenA := setupTestRelay(t)

	key, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	clientA := sync.NewClient(srv.URL, tokenA, key)

	// Push an op.
	ops := []sync.OpPayload{{
		ID:        uid.New(),
		TableName: "vendors",
		RowID:     uid.New(),
		OpType:    "insert",
		Payload:   `{"Name":"Acme"}`,
		DeviceID:  "dev-a",
		CreatedAt: time.Now(),
	}}
	pushResp, err := clientA.Push(t.Context(), ops)
	require.NoError(t, err)
	require.Len(t, pushResp.Confirmed, 1)
	assert.Equal(t, int64(1), pushResp.Confirmed[0].Seq)

	// Register device B and pull.
	devAResp, _ := store.AuthenticateDevice(t.Context(), tokenA)
	regResp, err := store.RegisterDevice(t.Context(), sync.RegisterDeviceRequest{
		HouseholdID: devAResp.HouseholdID,
		Name:        "device-b",
	})
	require.NoError(t, err)

	clientB := sync.NewClient(srv.URL, regResp.DeviceToken, key)
	pullResult, err := clientB.Pull(t.Context(), 0, 100)
	require.NoError(t, err)
	require.Len(t, pullResult.Ops, 1)

	assert.Equal(t, ops[0].TableName, pullResult.Ops[0].Payload.TableName)
	assert.Equal(t, ops[0].RowID, pullResult.Ops[0].Payload.RowID)
	assert.Equal(t, ops[0].OpType, pullResult.Ops[0].Payload.OpType)
}

func TestClientEncryptionRoundTrip(t *testing.T) {
	t.Parallel()
	srv, store, tokenA := setupTestRelay(t)

	key, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	clientA := sync.NewClient(srv.URL, tokenA, key)

	payload := `{"Name":"Test Vendor","Email":"test@example.com"}`
	ops := []sync.OpPayload{{
		ID:        uid.New(),
		TableName: "vendors",
		RowID:     uid.New(),
		OpType:    "insert",
		Payload:   payload,
		DeviceID:  "dev-a",
		CreatedAt: time.Now(),
	}}

	_, err = clientA.Push(t.Context(), ops)
	require.NoError(t, err)

	// Device B pulls and decrypts.
	devAResp, _ := store.AuthenticateDevice(t.Context(), tokenA)
	regResp, err := store.RegisterDevice(t.Context(), sync.RegisterDeviceRequest{
		HouseholdID: devAResp.HouseholdID,
		Name:        "device-b",
	})
	require.NoError(t, err)

	clientB := sync.NewClient(srv.URL, regResp.DeviceToken, key)
	pullResult, err := clientB.Pull(t.Context(), 0, 100)
	require.NoError(t, err)
	require.Len(t, pullResult.Ops, 1)

	// Verify payload survived encrypt/decrypt round-trip.
	assert.Equal(t, payload, pullResult.Ops[0].Payload.Payload)
}

func TestClientWrongKeyCannotDecrypt(t *testing.T) {
	t.Parallel()
	srv, store, tokenA := setupTestRelay(t)

	keyA, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)
	keyB, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	clientA := sync.NewClient(srv.URL, tokenA, keyA)
	ops := []sync.OpPayload{{
		ID:        uid.New(),
		TableName: "vendors",
		RowID:     uid.New(),
		OpType:    "insert",
		Payload:   `{"Name":"Secret"}`,
		DeviceID:  "dev-a",
		CreatedAt: time.Now(),
	}}
	_, err = clientA.Push(t.Context(), ops)
	require.NoError(t, err)

	devAResp, _ := store.AuthenticateDevice(t.Context(), tokenA)
	regResp, err := store.RegisterDevice(t.Context(), sync.RegisterDeviceRequest{
		HouseholdID: devAResp.HouseholdID,
		Name:        "device-b",
	})
	require.NoError(t, err)

	// Device B uses wrong key.
	clientB := sync.NewClient(srv.URL, regResp.DeviceToken, keyB)
	_, err = clientB.Pull(t.Context(), 0, 100)
	assert.Error(t, err, "decryption with wrong key should fail")
}

func TestClientHandlesTrailingSlashInBaseURL(t *testing.T) {
	t.Parallel()
	srv, store, tokenA := setupTestRelay(t)

	key, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	// Use base URL with trailing slash -- should still work after JoinPath.
	clientA := sync.NewClient(srv.URL+"/", tokenA, key)

	ops := []sync.OpPayload{{
		ID:        uid.New(),
		TableName: "vendors",
		RowID:     uid.New(),
		OpType:    "insert",
		Payload:   `{"Name":"Trailing Slash Test"}`,
		DeviceID:  "dev-a",
		CreatedAt: time.Now(),
	}}
	pushResp, err := clientA.Push(t.Context(), ops)
	require.NoError(t, err)
	require.Len(t, pushResp.Confirmed, 1)

	// Pull should also work.
	devAResp, _ := store.AuthenticateDevice(t.Context(), tokenA)
	regResp, err := store.RegisterDevice(t.Context(), sync.RegisterDeviceRequest{
		HouseholdID: devAResp.HouseholdID,
		Name:        "device-b",
	})
	require.NoError(t, err)

	clientB := sync.NewClient(srv.URL+"/", regResp.DeviceToken, key)
	pullResult, err := clientB.Pull(t.Context(), 0, 100)
	require.NoError(t, err)
	require.Len(t, pullResult.Ops, 1)
}

func TestClientPushPayloadPreservesJSON(t *testing.T) {
	t.Parallel()

	key, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	op := sync.OpPayload{
		ID:        uid.New(),
		TableName: "projects",
		RowID:     uid.New(),
		OpType:    "update",
		Payload:   `{"Title":"Kitchen Reno","Status":"in_progress"}`,
		DeviceID:  "dev-x",
		CreatedAt: time.Now(),
	}

	// Serialize, encrypt, decrypt, deserialize.
	plaintext, err := json.Marshal(op)
	require.NoError(t, err)

	sealed, err := crypto.Encrypt(key, plaintext)
	require.NoError(t, err)

	decrypted, err := crypto.Decrypt(key, sealed)
	require.NoError(t, err)

	var restored sync.OpPayload
	require.NoError(t, json.Unmarshal(decrypted, &restored))

	assert.Equal(t, op.TableName, restored.TableName)
	assert.Equal(t, op.Payload, restored.Payload)
}
